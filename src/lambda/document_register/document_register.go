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
	documentRegistryStore *datastores.DocumentRegistryStore
	queueArn              string
	sqs                   *awshelper.SQSHelper
}

// Handle the registration processing
func (h *handler) postRegistration(registryPayload datastores.DocumentRegistryItem, receipt string) error {
	err := h.documentRegistryStore.RegisterDocument(registryPayload)

	// If no errors remove from the queue as processed.
	if err == nil {
		h.sqs.DeleteMessage(h.queueArn, receipt)
	} else {
		log.Printf("Unable to update progress of document %s: %v \n", registryPayload.DocumentId, err)
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
		registryPayload := datastores.DocumentRegistryItem{
			DocumentId:         message["documentId"].(string),
			BucketName:         message["bucketName"].(string),
			DocumentName:       message["documentName"].(string),
			DocumentMetadata:   message["documentMetadata"].(map[string]interface{}),
			DocumentLink:       message["documentLink"].(string),
			PrincipalIAMWriter: message["principalIAMWriter"].(map[string]interface{}),
			Timestamp:          message["timestamp"].(string),
		}

		if payload["documentVersion"] != nil {
			registryPayload.DocumentVersion = aws.String(payload["documentVersion"].(string))
		}

		err = h.postRegistration(registryPayload, receipt)
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
	REGISTRY_TABLE := os.Getenv("REGISTRY_TABLE")
	SQS_QUEUE_ARN := os.Getenv("REGISTRY_SQS_QUEUE_ARN")
	if REGISTRY_TABLE == "" {
		panic("Missing REGISTRY_TABLE environment variable.")
	}
	if SQS_QUEUE_ARN == "" {
		panic("Missing REGISTRY_SQS_QUEUE_ARNenvironment variable.")
	}

	// Create Document Registry Store
	documentStore := datastores.NewDocumentRegistryStore(REGISTRY_TABLE)

	// Create SQS Helper
	sqshelper := awshelper.SQSHelper{SQSClient: sqs.New(awshelper.NewAWSSession())}

	h := handler{
		queueArn:              SQS_QUEUE_ARN,
		documentRegistryStore: documentStore,
		sqs:                   &sqshelper,
	}

	lambda.Start(h.handleRequest)
}
