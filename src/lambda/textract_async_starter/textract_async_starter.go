package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/textract"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
)

var PIPELINE_STAGE = "ASYNC_START_TEXTRACT"

// Represents the resources used by the handler
type handler struct {
	pipelineOperationsClient *metadata.PipelineOperationsClient
	s3                       *awshelper.S3Helper
	textractBucketName       string
	metadataTopic            string
	snsRole                  string
	snsTopic                 string
}

func (h *handler) startJob(bucketName string, objectName string, documentId string) (string, error) {
	log.Printf("Starting job with documentId: %s, bucketName: %s, objectName: %s \n", documentId, bucketName, objectName)

	t := textract.New(awshelper.NewAWSSession())
	response, err := t.StartDocumentAnalysis(&textract.StartDocumentAnalysisInput{
		ClientRequestToken: aws.String(documentId),
		DocumentLocation: &textract.DocumentLocation{
			S3Object: &textract.S3Object{
				Bucket: aws.String(bucketName),
				Name:   aws.String(objectName),
			},
		},
		FeatureTypes: []*string{
			aws.String("TABLES"),
			aws.String("FORMS"),
		},
		NotificationChannel: &textract.NotificationChannel{
			RoleArn:     aws.String(h.snsRole),
			SNSTopicArn: aws.String(h.snsTopic),
		},
		OutputConfig: &textract.OutputConfig{
			S3Bucket: aws.String(h.textractBucketName),
			S3Prefix: aws.String(objectName + "/textract-output"),
		},
		JobTag: aws.String(documentId),
	})
	if err != nil {
		log.Printf("Failed to call textract. Error: %s \n", err)
		return "", err
	}

	return *response.JobId, nil
}

func (h *handler) processItem(bucketName string, objectName string, snsTopic string, snsRole string) error {
	log.Printf("Bucket Name: %s \n", bucketName)
	log.Printf("Object Name: %s \n", objectName)

	tags, err := h.s3.GetTagsS3(bucketName, objectName)
	documentId := tags["documentId"]
	if err != nil {
		log.Printf("Error getting tags for object %s/%s. Error: %s \n", bucketName, objectName, err)
		return err
	}

	log.Printf("Task ID: %s \n", documentId)

	// Initialise the pipeline payload
	var operationsBody = map[string]interface{}{
		"documentId": documentId,
		"bucketName": bucketName,
		"objectName": objectName,
		"stage":      PIPELINE_STAGE,
	}

	err = h.pipelineOperationsClient.StageInProgress(operationsBody, "")
	if err != nil {
		log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, err)
		return err
	}

	jobId, err := h.startJob(bucketName, objectName, documentId)
	if err != nil {
		log.Printf("Error starting job for document %s. Error: %s \n", documentId, err)
		failerr := h.pipelineOperationsClient.StageFailed(operationsBody, fmt.Sprintf("Not able to start document analysis for document Id %s; bucket %s with name %s", documentId, bucketName, objectName))
		if failerr != nil {
			log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
		}
		return err
	}
	log.Println("Started job with id: ", jobId)

	err = h.pipelineOperationsClient.StageSucceeded(operationsBody, "")
	if err != nil {
		log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, err)
		return err
	}

	return nil
}

func (h *handler) handleRequest(ctx context.Context, s3Event events.S3Event) error {
	// Print the event
	log.Printf("Async Textract Started event: {%+v} \n", s3Event)

	// Process each record
	for _, record := range s3Event.Records {
		s3 := record.S3
		log.Printf("[%s - %s] Bucket = %s, Key = %s \n", record.EventSource, record.EventTime, s3.Bucket.Name, s3.Object.Key)

		err := h.processItem(s3.Bucket.Name, s3.Object.Key, h.snsTopic, h.snsRole)
		if err != nil {
			log.Printf("Error processing item %s/%s. Error: %s \n", s3.Bucket.Name, s3.Object.Key, err)
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
	snsRole := os.Getenv("TEXTRACT_SNS_ROLE_ARN")
	snsTopic := os.Getenv("TEXTRACT_SNS_TOPIC_ARN")

	if metadataTopic == "" {
		panic("Missing METADATA_SNS_TOPIC_ARN environment variable.")
	}
	if textractBucketName == "" {
		panic("Missing TEXTRACT_RESULTS_BUCKET_NAME environment variable.")
	}
	if snsRole == "" {
		panic("Missing TEXTRACT_SNS_ROLE_ARN environment variable.")
	}
	if snsTopic == "" {
		panic("Missing TEXTRACT_SNS_TOPIC_ARN environment variable.")
	}

	//Create Metadata Clients
	pipelineClient := metadata.NewPipelineOperationsClient(metadataTopic)

	// Create S3Helper
	s3helper := awshelper.S3Helper{S3Client: s3.New(awshelper.NewAWSSession())}

	h := handler{
		pipelineOperationsClient: pipelineClient,
		s3:                       &s3helper,
		textractBucketName:       textractBucketName,
		metadataTopic:            metadataTopic,
		snsRole:                  snsRole,
		snsTopic:                 snsTopic,
	}

	lambda.Start(h.handleRequest)
}
