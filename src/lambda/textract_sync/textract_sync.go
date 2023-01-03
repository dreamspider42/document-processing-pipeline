package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/textract"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
	"github.com/dreamspider42/document-processing-pipeline/src/textractparser"
)

var PIPELINE_STAGE = "SYNC_PROCESS_TEXTRACT"

// Represents the resources used by the handler
type handler struct {
	pipelineOperationsClient *metadata.PipelineOperationsClient
	documentLineageClient    *metadata.DocumentLineageClient
	s3                       *awshelper.S3Helper
	textractBucketName       string
}

func (h *handler) callTextract(bucketName string, objectName string) (*textract.AnalyzeDocumentOutput, error) {
	t := textract.New(awshelper.NewAWSSession())

	response, err := t.AnalyzeDocument(&textract.AnalyzeDocumentInput{
		Document: &textract.Document{
			S3Object: &textract.S3Object{
				Bucket: aws.String(bucketName),
				Name:   aws.String(objectName),
			},
		},
		FeatureTypes: []*string{
			aws.String("TABLES"),
			aws.String("FORMS"),
		},
	})
	if err != nil {
		log.Printf("Failed to call textract. Error: %s \n", err)
		return nil, err
	}

	return response, nil
}

func (h *handler) processImage(documentId string, bucketName string, objectName string, callerId string) error {
	// Call textract
	response, err := h.callTextract(bucketName, objectName)
	if err != nil {
		return err
	}

	// Print the output
	log.Printf("Generating output for documentId: %s \n", documentId)

	// Generate the output
	opg := textractparser.NewOutputGenerator(h.s3, response, documentId, h.textractBucketName, objectName, false, false)

	// Write the output
	tagging := "documentId=" + documentId
	err = opg.WriteTextractOutputs(&tagging)
	if err != nil {
		return err
	}

	// Record the lineage of the copy
	var lineageBody = map[string]interface{}{
		"documentId":       documentId,
		"callerId":         callerId,
		"sourceBucketName": bucketName,
		"targetBucketName": h.textractBucketName,
		"sourceFileName":   objectName,
		"targetFileName":   objectName,
	}
	err = h.documentLineageClient.RecordLineageOfCopy(lineageBody)
	if err != nil {
		log.Printf("Failed to record lineage of copy. Error: %s \n", err)
		return err
	}

	return nil
}

func (h *handler) processRequest(bucketName string, objectName string, callerId string) error {
	// Get the document tags
	tagMap, err := h.s3.GetTagsS3(bucketName, objectName)
	if err != nil {
		return err
	}

	// Get the document id
	documentId := tagMap["documentId"]

	// Initialize the pipeline payload
	var operationsBody = map[string]interface{}{
		"documentId": documentId,
		"bucketName": bucketName,
		"objectName": objectName,
		"stage":      PIPELINE_STAGE,
	}

	// Update the pipeline stage
	err = h.pipelineOperationsClient.StageInProgress(operationsBody, "")
	if err != nil {
		log.Printf("Failed to update pipeline stage. Error: %s \n", err)
		return err
	}

	// Print the task id
	log.Printf("Task ID: %s \n", documentId)

	// Process the image
	if documentId != "" && bucketName != "" && objectName != "" {
		log.Printf("DocumentId: %s, Object: %s/%s \n", documentId, bucketName, objectName)

		err = h.processImage(documentId, bucketName, objectName, callerId)
		if err != nil {
			log.Printf("Failed to process image. Error: %s \n", err)
			return err
		}

		log.Printf("Document: %s, Object: %s/%s processed. \n", documentId, bucketName, objectName)
		err = h.pipelineOperationsClient.StageSucceeded(operationsBody, "")
	} else {
		err = h.pipelineOperationsClient.StageFailed(operationsBody, "Missing documentId, bucketName or objectName.")
	}

	if err != nil {
		log.Printf("Failed to update pipeline stage. Error: %s \n", err)
		return err
	}
	return nil
}

func (h *handler) handleRequest(ctx context.Context, s3Event events.S3Event) error {
	// Print the caller id
	lc, _ := lambdacontext.FromContext(ctx)
	callerId := lc.InvokedFunctionArn
	log.Printf("CallerId: %s \n", callerId)

	// Print the event
	log.Printf("Sync Processor event: {%+v} \n", s3Event)

	// Process each record
	for _, record := range s3Event.Records {
		s3 := record.S3
		log.Printf("[%s - %s] Bucket = %s, Key = %s \n", record.EventSource, record.EventTime, s3.Bucket.Name, s3.Object.Key)

		// Process the request
		err := h.processRequest(s3.Bucket.Name, s3.Object.Key, callerId)
		if err != nil {
			log.Printf("Failed to process request. Error: %s \n", err)
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
