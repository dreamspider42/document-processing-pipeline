package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/datastores"
)

// Represents the resources used by the handler
type handler struct {
	lineageStore *datastores.LineageStore
	queueArn     string
	sqs          *awshelper.SQSHelper
}

// Handle the Lineage processing.
func (h *handler) postLineage(lineagePayload datastores.LineageItem, receipt string) error {
	var actualDocumentId string
	var err error

	if strings.HasPrefix(lineagePayload.S3Event, "ObjectRemoved") {
		actualDocumentId, err = h.lineageStore.QueryDocumentId(lineagePayload.TargetBucketName, lineagePayload.TargetFileName, lineagePayload.VersionId)
		if err != nil {
			return fmt.Errorf("unable to find documentId for %s/%s Version %s: %v", lineagePayload.TargetBucketName, lineagePayload.TargetFileName, lineagePayload.VersionId, err)
		} else if actualDocumentId == "" {
			log.Printf("Could not find corresponding documentId for this deletion event")
			return h.sqs.DeleteMessage(h.queueArn, receipt)
		}
		lineagePayload.DocumentId = actualDocumentId
	}

	err = h.lineageStore.CreateLineage(lineagePayload)
	if err == nil {
		// If no errors remove from the queue as processed.
		err = h.sqs.DeleteMessage(h.queueArn, receipt)
	} else {
		return fmt.Errorf("unable to update progress of document %s: %v", lineagePayload.DocumentId, err)
	}
	return err
}

// Lambda request handler
func (h *handler) handleRequest(ctx context.Context, event events.SQSEvent) error {
	// Process each record in the event
	for _, record := range event.Records {
		if record.EventSourceARN != h.queueArn {
			return fmt.Errorf("unexpected lambda event source ARN. Expected %s, got %s", h.queueArn, record.EventSourceARN)
		}
		log.Printf("Record: %v \n", record)

		// Unwrap the payload
		payload := map[string]interface{}{}
		log.Printf("Body: %v \n", record.Body)
		err := json.Unmarshal([]byte(record.Body), &payload)
		if err != nil {
			return err
		}

		// Unwrap the message
		message := map[string]interface{}{}
		log.Printf("Payload: %v", payload["Message"])
		err = json.Unmarshal([]byte(payload["Message"].(string)), &message)
		if err != nil {
			return err
		}

		receipt := record.ReceiptHandle
		lineagePayload := datastores.LineageItem{
			DocumentId:       message["documentId"].(string),
			CallerId:         message["callerId"].(map[string]interface{}),
			TargetFileName:   message["targetFileName"].(string),
			TargetBucketName: message["targetBucketName"].(string),
			Timestamp:        message["timestamp"].(string),
			S3Event:          message["s3Event"].(string),
		}

		if message["versionId"] != nil {
			lineagePayload.VersionId = message["versionId"].(string)
		}

		if message["s3Event"] == "ObjectCreated:Copy" {
			lineagePayload.SourceFileName = message["sourceFileName"].(string)
			lineagePayload.SourceBucketName = message["sourceBucketName"].(string)
		}

		err = h.postLineage(lineagePayload, receipt)

		if err != nil {
			return err
		}
	}
	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	// Check for missing arguments
	LINEAGE_TABLE := os.Getenv("LINEAGE_TABLE")
	LINEAGE_INDEX := os.Getenv("LINEAGE_INDEX")
	SQS_QUEUE_ARN := os.Getenv("LINEAGE_SQS_QUEUE_ARN")

	if LINEAGE_TABLE == "" {
		panic("Missing LINEAGE_TABLE environment variable.")
	}
	if LINEAGE_INDEX == "" {
		panic("Missing LINEAGE_INDEX environment variable.")
	}
	if SQS_QUEUE_ARN == "" {
		panic("Missing LINEAGE_SQS_QUEUE_ARN environment variable.")
	}

	// Create Document Lineage Store
	documentLineageStore := datastores.NewLineageStore(LINEAGE_TABLE, LINEAGE_INDEX)

	// Create SQS Helper
	sqshelper := awshelper.SQSHelper{SQSClient: sqs.New(awshelper.NewAWSSession())}

	h := handler{
		queueArn:     SQS_QUEUE_ARN,
		lineageStore: documentLineageStore,
		sqs:          &sqshelper,
	}

	lambda.Start(h.handleRequest)
}
