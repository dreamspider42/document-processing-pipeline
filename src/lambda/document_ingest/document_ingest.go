package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
	"github.com/google/uuid"
)

// Represents the resources used by the handler
type handler struct {
	documentRegistryClient *metadata.DocumentRegistryClient
	documentLineageClient  *metadata.DocumentLineageClient
	s3                     *awshelper.S3Helper
}

// Process a create event from S3
func (h *handler) processCreateRequest(bucketName string, documentName string, documentVersion string, principalIAMWriter events.S3UserIdentity, eventName string) error {

	// New uuid for item id
	documentId := uuid.New().String()

	// Log what we're tagging and tag S3 with metadata.
	log.Printf("Input Object: %s/%s version %s \n", bucketName, documentName, documentVersion)
	log.Printf("Tagging object %s with tag %s and version %s \n", documentName, documentId, documentVersion)
	err := h.s3.TagS3(bucketName, documentName, map[string]string{"documentId": documentId})
	if err != nil {
		log.Printf("Failed to tag object %s/%s with documentId %s. Error: %v", bucketName, documentName, documentId, err)
		return err
	}

	// Create document link for s3 object
	documentLink := fmt.Sprintf("s3://%s/%s", bucketName, documentName)

	// Create metadata registry item
	registryItem := map[string]interface{}{
		"documentId":         documentId,
		"bucketName":         bucketName,
		"documentName":       documentName,
		"documentLink":       documentLink,
		"principalIAMWriter": principalIAMWriter,
	}

	// Create metadata lineage item
	lineageItem := map[string]interface{}{
		"documentId":       documentId,
		"callerId":         principalIAMWriter,
		"targetBucketName": bucketName,
		"targetFileName":   documentName,
		"s3Event":          eventName,
	}

	if documentVersion != "" {
		registryItem["documentVersion"] = documentVersion
		lineageItem["versionId"] = documentVersion
	}

	err = h.documentRegistryClient.RegisterDocument(registryItem)
	if err != nil {
		log.Printf("Failed to register document %s/%s with documentId %s. Error: %v", bucketName, documentName, documentId, err)
		return err
	}

	err = h.documentLineageClient.RecordLineage(lineageItem)
	if err != nil {
		log.Printf("Failed to record lineage for document %s/%s with documentId %s. Error: %v", bucketName, documentName, documentId, err)
		return err
	}

	log.Printf("Saved document %s for %s/%s version %s \n", documentId, bucketName, documentName, documentVersion)
	return nil
}

// Process a delete event from S3
func (h *handler) processDeleteRequest(bucketName string, documentName string, documentVersion string, principalIAMWriter events.S3UserIdentity, eventName string) error {
	log.Printf("Remove Object Processing: %s/%s version %s \n", bucketName, documentName, documentVersion)

	lineageItem := map[string]interface{}{
		"documentId":       "UNKNOWN_YET",
		"callerId":         principalIAMWriter,
		"targetBucketName": bucketName,
		"targetFileName":   documentName,
		"s3Event":          eventName,
	}
	if documentVersion != "" {
		lineageItem["versionId"] = documentVersion
	}
	err := h.documentLineageClient.RecordLineage(lineageItem)
	if err != nil {
		log.Printf("Failed to record lineage for document %s/%s version %s. Error: %v", bucketName, documentName, documentVersion, err)
		return err
	}
	log.Printf("Marked document %s/%s with version %s for deletion \n", bucketName, documentName, documentVersion)
	return nil
}

// Lambda request handler
func (h *handler) handleRequest(ctx context.Context, s3Event events.S3Event) error {
	// Print the event
	log.Printf("event: {%+v} \n", s3Event)

	// Process each record
	for _, record := range s3Event.Records {
		s3 := record.S3
		log.Printf("[%s - %s] Bucket = %s, Key = %s \n", record.EventSource, record.EventTime, s3.Bucket.Name, s3.Object.Key)

		// Process create or delete requests
		eventName := record.EventName
		var err error
		if eventName == "ObjectRemoved:Delete" {
			err = h.processDeleteRequest(s3.Bucket.Name, s3.Object.Key, s3.Object.VersionID, record.PrincipalID, eventName)
		} else if strings.HasPrefix(eventName, "ObjectCreated") {
			err = h.processCreateRequest(s3.Bucket.Name, s3.Object.Key, s3.Object.VersionID, record.PrincipalID, eventName)
		} else {
			log.Printf("Processing not implement yet for event: %s \n", eventName)
		}
		if err != nil {
			log.Printf("Failed to process event %s for %s/%s version %s. Error: %v", eventName, s3.Bucket.Name, s3.Object.Key, s3.Object.VersionID, err)
			return err
		}
	}
	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	metadataTopic := os.Getenv("METADATA_SNS_TOPIC_ARN")
	if metadataTopic == "" {
		panic("Missing METADATA_SNS_TOPIC_ARN environment variable.")
	}

	// Create S3Helper
	s3helper := awshelper.S3Helper{S3Client: s3.New(awshelper.NewAWSSession())}

	// Create metadata clients
	documentMetaData := map[string]interface{}{"owner": "CustomerName", "class": "external_public_report"}
	registryClient := metadata.NewDocumentRegistryClient(metadataTopic, map[string]interface{}{"documentMetadata": documentMetaData})
	lineageClient := metadata.NewDocumentLineageClient(metadataTopic)

	h := handler{
		documentRegistryClient: registryClient,
		documentLineageClient:  lineageClient,
		s3:                     &s3helper,
	}

	lambda.Start(h.handleRequest)
}
