package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/datastores"
)

// Represents the resources used by the handler
type handler struct {
	pipelineOpsStore *datastores.PipelineOperationsStore
	queueArn         string
	sqs              *awshelper.SQSHelper
}

// Start tracking the document progress in the pipeline.
func (h *handler) startDocumentTracking(documentId, bucketName, objectName, status, stage, timestamp, receipt string, versionId interface{}) error {
	// Create the document payload
	documentPayload := datastores.PipelineOperationsItem{
		DocumentId:     documentId,
		BucketName:     bucketName,
		ObjectName:     objectName,
		DocumentStatus: status,
		DocumentStage:  stage,
		Timeline: []datastores.TimelineItem{
			{
				Timestamp: timestamp,
				Stage:     stage,
				Status:    status,
			},
		},
	}
	if versionId != nil {
		documentPayload.DocumentVersion = aws.String(versionId.(string))
	}

	// Start tracking the document
	err := h.pipelineOpsStore.StartDocumentTracking(documentPayload, receipt)
	if err != nil {
		return err
	}

	// Remove the message from the queue if processed successfully
	err = h.sqs.DeleteMessage(h.queueArn, receipt)

	return err
}

// Update the document status in the pipeline.
func (h *handler) updateDocumentTracking(documentId, status, stage, timestamp, receipt string, messageNote interface{}) error {
	// Update the document tracking record
	err := h.pipelineOpsStore.UpdateDocumentStatus(documentId, status, stage, timestamp, messageNote)
	if err != nil {
		return err
	}

	// Remove the message from the queue if processed successfully
	err = h.sqs.DeleteMessage(h.queueArn, receipt)

	return err
}

func (h *handler) handleRequest(ctx context.Context, event events.SQSEvent) error {
	// Loop through each message in the SQS event
	for _, message := range event.Records {
		// Check the event source ARN
		if message.EventSourceARN != h.queueArn {
			return fmt.Errorf("unexpected Lambda event source ARN. Expected %s, got %s", h.queueArn, message.EventSourceARN)
		}

		// Parse the message body
		var payload map[string]interface{}
		err := json.Unmarshal([]byte(message.Body), &payload)
		if err != nil {
			return fmt.Errorf("failed to parse message body: %s", err)
		}

		// Parse the message payload
		var messagePayload map[string]interface{}
		err = json.Unmarshal([]byte(payload["Message"].(string)), &messagePayload)
		if err != nil {
			return fmt.Errorf("failed to parse message payload: %s", err)
		}

		// Print the message payload
		log.Printf("Message payload: %v", messagePayload)

		// Check if this is the first message for this document
		if messagePayload["initDoc"] == "True" {
			// Start tracking the document
			err = h.startDocumentTracking(messagePayload["documentId"].(string), messagePayload["bucketName"].(string), messagePayload["objectName"].(string), messagePayload["status"].(string), messagePayload["stage"].(string), messagePayload["timestamp"].(string), message.ReceiptHandle, messagePayload["versionId"])
			if err != nil {
				return fmt.Errorf("failed to start tracking document: %s", err)
			}
		} else {
			// Update the document status
			err = h.updateDocumentTracking(messagePayload["documentId"].(string), messagePayload["status"].(string), messagePayload["stage"].(string), messagePayload["timestamp"].(string), message.ReceiptHandle, messagePayload["message"])
			if err != nil {
				return fmt.Errorf("failed to update document status: %s", err)
			}
		}
	}

	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	// Check for missing arguments
	PIPELINEOPS_TABLE := os.Getenv("PIPELINE_OPS_TABLE")
	SQS_QUEUE_ARN := os.Getenv("OPS_SQS_QUEUE_ARN")
	if PIPELINEOPS_TABLE == "" {
		panic("Missing PIPELINE_OPS_TABLE environment variable.")
	}
	if SQS_QUEUE_ARN == "" {
		panic("Missing OPS_SQS_QUEUE_ARN environment variable.")
	}

	// Create a Pipeline Operations Store
	pipelineOpsStore := datastores.NewPipelineOperationsStore(PIPELINEOPS_TABLE)

	// Create SQS Helper
	sqshelper := awshelper.SQSHelper{SQSClient: sqs.New(awshelper.NewAWSSession())}

	h := handler{
		queueArn:         SQS_QUEUE_ARN,
		pipelineOpsStore: pipelineOpsStore,
		sqs:              &sqshelper,
	}

	lambda.Start(h.handleRequest)
}
