package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/textract"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
	"github.com/dreamspider42/document-processing-pipeline/src/textractparser"
)

var PIPELINE_STAGE = "ASYNC_PROCESS_TEXTRACT"

// Represents the resources used by the handler
type handler struct {
	pipelineOperationsClient *metadata.PipelineOperationsClient
	documentLineageClient    *metadata.DocumentLineageClient
	s3                       *awshelper.S3Helper
	textractBucketName       string
}

type MessageMap struct {
	JobId            string `json:"JobId"`
	DocumentId       string `json:"JobTag"`
	Status           string `json:"Status"`
	API              string `json:"API"`
	DocumentLocation struct {
		BucketName string `json:"S3Bucket"`
		ObjectName string `json:"S3ObjectName"`
	} `json:"DocumentLocation"`
}

func (h *handler) getJobResults(message MessageMap) ([]byte, error) {
	resultbytes := []byte{}

	textractRawResultsFiles, err := h.s3.ListObjectsInS3(h.textractBucketName, message.DocumentLocation.ObjectName+"/textract-output/"+message.JobId, 1000)
	if err != nil {
		return nil, err
	}
	// skip the s3 access file, which will always appear first
	for _, textractResultFile := range textractRawResultsFiles[1:] {
		result, err := h.s3.ReadFromS3(h.textractBucketName, *textractResultFile)
		if err != nil {
			return nil, err
		}
		resultbytes = append(resultbytes, result...)
	}

	return resultbytes, nil
}

func (h *handler) processRequest(message MessageMap, callerId string) error {
	// Update the pipeline status
	var operationsBody = map[string]interface{}{
		"documentId": message.DocumentId,
		"bucketName": message.DocumentLocation.BucketName,
		"objectName": message.DocumentLocation.ObjectName,
		"stage":      PIPELINE_STAGE,
	}
	if message.Status == "FAILED" {
		err := h.pipelineOperationsClient.StageFailed(operationsBody, fmt.Sprintf("Textract job for document ID %s; bucketName %s fileName %s; failed during Textract analysis. Please double check the document quality", message.DocumentId, message.DocumentLocation.BucketName, message.DocumentLocation.ObjectName))
		return fmt.Errorf("textract analysis didn't complete successfully: %v", err)
	}

	err := h.pipelineOperationsClient.StageInProgress(operationsBody, "")
	if err != nil {
		log.Printf("Error updating pipeline stage for document %s. Error: %s \n", message.DocumentId, err)
		return err
	}

	resultbytes, err := h.getJobResults(message)
	if err != nil {
		failerr := h.pipelineOperationsClient.StageFailed(operationsBody, fmt.Sprintf("Textract job for document ID %s; bucketName %s fileName %s; failed during Textract processing. Could not read Textract output files under job Name %s", message.DocumentId, h.textractBucketName, message.DocumentLocation.ObjectName, message.JobId))
		if failerr != nil {
			log.Printf("Error updating pipeline stage for document %s. Error: %s \n", message.DocumentId, failerr)
		}
		return fmt.Errorf("textract retrieval didn't complete successfully: %v", err)
	}

	var results *textract.AnalyzeDocumentOutput
	err = json.Unmarshal(resultbytes, &results)
	if err != nil {
		failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Could not convert results from Textract into processable object. Try uploading again.")
		if failerr != nil {
			log.Printf("Error updating pipeline stage for document %s. Error: %s \n", message.DocumentId, failerr)
		}
		return err
	}

	detectForms := false
	detectTables := false
	if message.API == "StartDocumentAnalysis" {
		detectForms = true
		detectTables = true
	}

	opg := textractparser.NewOutputGenerator(h.s3, results, message.DocumentId, h.textractBucketName, message.DocumentLocation.ObjectName, detectForms, detectTables)
	tagging := "documentId=" + message.DocumentId
	opg.WriteTextractOutputs(&tagging)

	h.documentLineageClient.RecordLineage(map[string]interface{}{
		"documentId":       message.DocumentId,
		"callerId":         callerId,
		"sourceBucketName": message.DocumentLocation.BucketName,
		"targetBucketName": h.textractBucketName,
		"sourceFileName":   message.DocumentLocation.ObjectName,
		"targetFileName":   message.DocumentLocation.ObjectName,
	})

	output := fmt.Sprintf("Processed -> Document: %s, Object: %s/%s processed.", message.DocumentId, h.textractBucketName, message.DocumentLocation.ObjectName)
	err = h.pipelineOperationsClient.StageSucceeded(operationsBody, "")
	if err != nil {
		return err
	}

	log.Println("Output is: ", output)
	return nil
}

func (h *handler) handleRequest(ctx context.Context, snsEvent events.SNSEvent) error {
	// Print the event
	log.Printf("Async Textract Completed event: {%+v} \n", snsEvent)

	// Get the caller id
	lc, _ := lambdacontext.FromContext(ctx)
	callerId := lc.InvokedFunctionArn

	// Process each record
	for _, record := range snsEvent.Records {
		snsrecord := record.SNS
		log.Println("SNS Record: ", snsrecord.Message)

		// Unmarshall the JSON
		var messageMap MessageMap

		json.Unmarshal([]byte(snsrecord.Message), &messageMap)

		err := h.processRequest(messageMap, callerId)
		if err != nil {
			log.Println("Error processing request: ", err)
			return err
		}
	}
	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	metadataTopic := os.Getenv("METADATA_SNS_TOPIC_ARN")
	textractBucketName := os.Getenv("TEXTRACT_RESULTS_BUCKET_NAME")

	if metadataTopic == "" {
		panic("Missing METADATA_SNS_TOPIC_ARN environment variable.")
	}
	if textractBucketName == "" {
		panic("Missing TEXTRACT_RESULTS_BUCKET_NAME environment variable.")
	}

	// Create S3Helper
	s3helper := awshelper.S3Helper{S3Client: s3.New(awshelper.NewAWSSession())}

	//Create Metadata Clients
	pipelineClient := metadata.NewPipelineOperationsClient(metadataTopic)
	lineageClient := metadata.NewDocumentLineageClient(metadataTopic)

	h := handler{
		pipelineOperationsClient: pipelineClient,
		documentLineageClient:    lineageClient,
		s3:                       &s3helper,
		textractBucketName:       textractBucketName,
	}

	lambda.Start(h.handleRequest)
}
