package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
	"golang.org/x/exp/slices"
)

var PIPELINE_STAGE = "DOCUMENT_CLASSIFIER"
var documentTypes = map[string][]string{}

// Represents the resources used by the handler
type handler struct {
	pipelineOperationsClient *metadata.PipelineOperationsClient
}

func (h *handler) processRequest(newImage map[string]interface{}) error {
	documentMetadata := newImage["documentMetadata"].(map[string]interface{})
	documentId := newImage["documentId"].(string)
	bucketName := newImage["bucketName"].(string)
	objectName := newImage["documentName"].(string)

	if documentMetadata != nil && documentId != "" && bucketName != "" && objectName != "" {
		log.Println("Valid document item to classify!")
	} else {
		return fmt.Errorf("invalid document item! Please check the incoming dynamoDB record stream")
	}

	log.Printf("DocumentId: %s, BucketName: %s, ObjectName: %s", documentId, bucketName, objectName)
	log.Printf("DocumentMetadata: %s", documentMetadata)

	if slices.Contains(documentTypes["NLP_VALID"], documentMetadata["class"].(string)) {
		return h.startNLPProcessing(bucketName, objectName, documentId)
	} else {
		log.Printf("Document %s not eligible for NLP Processing", documentId)
		return nil
	}
}

func (h *handler) startNLPProcessing(bucketName string, objectName string, documentId string) error {
	var body = map[string]interface{}{
		"bucketName": bucketName,
		"objectName": objectName,
		"documentId": documentId,
		"stage":      PIPELINE_STAGE,
	}
	// Initialize the document
	err := h.pipelineOperationsClient.InitDoc(body)
	if err != nil {
		log.Printf("Failed to initialize document. Error: %v", err)
		err = h.pipelineOperationsClient.StageFailed(body, "Unable to kick off pipeline")
		return err
	}

	// Mark the stage as succeeded
	err = h.pipelineOperationsClient.StageSucceeded(body, "")
	if err != nil {
		log.Printf("Failed to mark stage as succeeded. Error: %v", err)
		return err
	}
	log.Printf("Started NLP for Document %s", documentId)

	return nil
}

// Lambda request handler
func (h *handler) handleRequest(ctx context.Context, dbEvent awshelper.DynamoDBEvent) error {
	// Print the event
	log.Printf("event: {%+v} \n", dbEvent)

	// Process each record
	for _, record := range dbEvent.Records {
		// Check if the record is an INSERT or MODIFY event
		if record.EventName == "INSERT" || record.EventName == "MODIFY" {
			if record.Change.NewImage != nil {
				// Print the record
				log.Printf("record: {%+v} \n", record)

				// Deserialize the record
				newImage, err := awshelper.DynamoDBHelper{}.DeserializeItem(record.Change.NewImage)
				if err != nil {
					log.Printf("Failed to deserialize DynamoDB record. Error: %v", err)
					return err
				}

				// Print the new image
				log.Printf("Deserialized NewImage: {%+v} \n", newImage)

				// Process the request
				err = h.processRequest(newImage)
				if err != nil {
					log.Printf("Failed to process request. Error: %v", err)
					return err
				}
			} else {
				log.Printf("Record does not contain a NewImage.")
			}
		} else {
			log.Printf("Record is not an INSERT or MODIFY event.")
		}
	}
	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	// Initialize document types
	documentTypes["NLP_VALID"] = []string{"internal_research_report", "external_public_report"}
	documentTypes["NLP_INVALID"] = []string{"classified_report"}

	// Check for missing arguments
	metadataTopic := os.Getenv("METADATA_SNS_TOPIC_ARN")
	if metadataTopic == "" {
		panic("Missing METADATA_SNS_TOPIC_ARN environment variable.")
	}

	//Create Pipeline Operations Client
	pipelineClient := metadata.NewPipelineOperationsClient(metadataTopic)

	h := handler{
		pipelineOperationsClient: pipelineClient,
	}

	lambda.Start(h.handleRequest)
}
