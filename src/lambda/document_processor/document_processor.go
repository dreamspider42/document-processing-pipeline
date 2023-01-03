package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
)

var PIPELINE_STAGE = "DOCUMENT_PROCESSOR"

// Represents the resources used by the handler
type handler struct {
	pipelineOperationsClient *metadata.PipelineOperationsClient
	documentLineageClient    *metadata.DocumentLineageClient
	s3                       *awshelper.S3Helper
	asyncBucketName          string
	syncBucketName           string
}

func (h *handler) processRequest(documentId string, bucketName string, objectName string, callerId string) error {
	// Print the document id, bucket name and object name
	log.Printf("DocumentId: %s, BucketName: %s, ObjectName: %s \n", documentId, bucketName, objectName)

	// Initialise the pipeline payload
	var operationsBody = map[string]interface{}{
		"documentId": documentId,
		"bucketName": bucketName,
		"objectName": objectName,
		"stage":      PIPELINE_STAGE,
	}

	// Start the pipeline stage
	err := h.pipelineOperationsClient.StageInProgress(operationsBody, "")
	if err != nil {
		log.Printf("Failed to start pipeline stage. Error: %s \n", err)
		return err
	}

	// Print the input object
	log.Printf("Input Object: %s/%s \n", bucketName, objectName)

	// Get the file extension
	ext := filepath.Ext(objectName)
	log.Printf("Extension: %s \n", ext)

	// Determine the target bucket
	var targetBucketName string
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
		targetBucketName = h.syncBucketName
	} else if ext == ".pdf" {
		targetBucketName = h.asyncBucketName
	} else {
		err = fmt.Errorf("incorrect file extension")
		log.Printf("Failed to determine target bucket. Error: %s \n", err)
		return err
	}

	// Determine the target file name
	targetFileName := fmt.Sprintf("%s/%s", documentId, objectName)

	// Copy the object to the target bucket
	if targetBucketName != "" {
		log.Printf("Doing S3 Object Copy for documentId: %s, object: %s/%s \n", documentId, targetBucketName, targetFileName)
		_, err = h.s3.CopyToS3(bucketName, objectName, targetBucketName, targetFileName)
		if err != nil {
			log.Printf("Failed to copy object to target bucket. Error: %s \n", err)
			return err
		}
	} else {
		err = fmt.Errorf("target bucket is empty")
		log.Printf("Failed to copy object to target bucket. Error: %s \n", err)
		return err
	}

	// Record the lineage of the copy
	var lineageBody = map[string]interface{}{
		"documentId":       documentId,
		"callerId":         callerId,
		"sourceBucketName": bucketName,
		"targetBucketName": targetBucketName,
		"sourceFileName":   objectName,
		"targetFileName":   targetFileName,
	}
	err = h.documentLineageClient.RecordLineageOfCopy(lineageBody)
	if err != nil {
		log.Printf("Failed to record lineage of copy. Error: %s \n", err)
		return err
	}

	// Complete the pipeline stage
	err = h.pipelineOperationsClient.StageSucceeded(operationsBody, "")
	if err != nil {
		log.Printf("Failed to complete pipeline stage. Error: %s \n", err)
		return err
	}

	// Print the output
	log.Printf("Completed S3 Object Copy for documentId: %s, object: %s/%s \n", documentId, targetBucketName, targetFileName)

	return nil
}

func (h *handler) processRecord(record awshelper.DynamoDBEventRecord, syncBucketName string, asyncBucketName string, callerId string) error {
	// Get the new image
	newImage := record.Change.NewImage

	// Get the document id
	documentId := newImage["documentId"].S

	// Get the bucket name
	bucketName := newImage["bucketName"].S

	// Get the object name
	objectName := newImage["objectName"].S

	// Process the request if all are present
	if documentId != nil && bucketName != nil && objectName != nil {
		return h.processRequest(*documentId, *bucketName, *objectName, callerId)
	}
	return fmt.Errorf("missing document id, bucket name or object name")
}

// Lambda request handler
func (h *handler) handleRequest(ctx context.Context, dbEvent awshelper.DynamoDBEvent) error {
	// Print the caller id
	lc, _ := lambdacontext.FromContext(ctx)
	callerId := lc.InvokedFunctionArn
	log.Printf("CallerId: %s \n", callerId)

	// Print the event
	log.Printf("Event: %v \n", dbEvent)

	// Process each record
	var err error
	for _, record := range dbEvent.Records {
		log.Printf("Processing record: %v \n", record)

		// Check if the record is an insert
		if record.EventName == "INSERT" && record.Change.NewImage != nil {
			// Process the record
			err = h.processRecord(record, h.syncBucketName, h.asyncBucketName, callerId)
			if err != nil {
				log.Printf("Failed to process record. Error: %s \n", err)
				return err
			}
		}
	}

	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	// Check for missing arguments
	metadataTopic := os.Getenv("METADATA_SNS_TOPIC_ARN")
	syncBucketName := os.Getenv("SYNC_TEXTRACT_BUCKET_NAME")
	asyncBucketName := os.Getenv("ASYNC_TEXTRACT_BUCKET_NAME")

	if metadataTopic == "" {
		panic("Missing METADATA_SNS_TOPIC_ARN environment variable.")
	}
	if syncBucketName == "" {
		panic("Missing SYNC_TEXTRACT_BUCKET_NAME environment variable.")
	}
	if asyncBucketName == "" {
		panic("Missing ASYNC_TEXTRACT_BUCKET_NAME environment variable.")
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
		syncBucketName:           syncBucketName,
		asyncBucketName:          asyncBucketName,
	}

	lambda.Start(h.handleRequest)
}
