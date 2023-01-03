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
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/comprehend"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/textract"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"github.com/dreamspider42/document-processing-pipeline/src/metadata"
	"github.com/dreamspider42/document-processing-pipeline/src/textractparser"
)

const (
	PIPELINE_STAGE             = "SYNC_PROCESS_COMPREHEND"
	COMPREHEND_CHARACTER_LIMIT = 4096
)

// Represents the resources used by the handler
type handler struct {
	pipelineOperationsClient *metadata.PipelineOperationsClient
	documentLineageClient    *metadata.DocumentLineageClient
	s3                       *awshelper.S3Helper
	es                       *awshelper.ESHelper
	comprehendBucketName     string
}

func (h *handler) dissectObjectName(objectName string) (string, string) {
	objectParts := strings.Split(objectName, "/ocr-analysis/")
	objectPath := strings.Split(objectParts[0], "/")
	documentId := objectPath[0]
	documentName := strings.Join(objectPath[1:], "/")
	return documentId, documentName
}

func (h *handler) chunkUpTheText(text string) []string {
	chunksOfText := []string{}
	for len(text) > COMPREHEND_CHARACTER_LIMIT {
		indexSnip := COMPREHEND_CHARACTER_LIMIT

		for text[indexSnip:indexSnip+1] != " " {
			indexSnip += 1
		}
		chunksOfText = append(chunksOfText, text[0:indexSnip])
		text = text[indexSnip:]
	}
	chunksOfText = append(chunksOfText, text)
	return chunksOfText
}

func (h *handler) batchSendToComprehend(c *comprehend.Comprehend, textList []string, language string) ([]string, map[string]string, error) {
	keyPhrases := []string{}
	entitiesDetected := map[string]string{}

	keyphrase_response, err := c.BatchDetectKeyPhrases(&comprehend.BatchDetectKeyPhrasesInput{
		TextList:     aws.StringSlice(textList),
		LanguageCode: &language,
	})
	if err != nil {
		return nil, nil, err
	}

	keyphraseResultsList := keyphrase_response.ResultList
	for _, keyphraseListResp := range keyphraseResultsList {
		keyphraseList := keyphraseListResp.KeyPhrases
		for _, s := range keyphraseList {
			s_txt := *s.Text
			log.Printf("Detected keyphrase %s", s_txt)
			keyPhrases = append(keyPhrases, s_txt)
		}
	}

	detect_entity_response, err := c.BatchDetectEntities(&comprehend.BatchDetectEntitiesInput{
		TextList:     aws.StringSlice(textList),
		LanguageCode: &language,
	})
	if err != nil {
		return nil, nil, err
	}
	entityResultsList := detect_entity_response.ResultList
	for _, entitiesListResp := range entityResultsList {
		entityList := entitiesListResp.Entities
		for _, s := range entityList {
			entitiesDetected[*s.Type] = *s.Text
		}
	}

	return keyPhrases, entitiesDetected, nil
}

func (h *handler) singularSendToComprehend(c *comprehend.Comprehend, text string, language string) ([]string, map[string]string, error) {
	keyPhrases := []string{}
	entitiesDetected := map[string]string{}

	keyphrase_response, err := c.DetectKeyPhrases(&comprehend.DetectKeyPhrasesInput{
		Text:         &text,
		LanguageCode: &language,
	})
	if err != nil {
		return nil, nil, err
	}
	keyphraseList := keyphrase_response.KeyPhrases
	for _, s := range keyphraseList {
		s_txt := *s.Text
		log.Printf("Detected keyphrase %s", s_txt)
		keyPhrases = append(keyPhrases, s_txt)
	}

	detect_entity_response, err := c.DetectEntities(&comprehend.DetectEntitiesInput{
		Text:         &text,
		LanguageCode: &language,
	})
	if err != nil {
		return nil, nil, err
	}
	entityList := detect_entity_response.Entities
	for _, s := range entityList {
		entitiesDetected[*s.Type] = *s.Text
	}

	return keyPhrases, entitiesDetected, nil
}

func (h *handler) runComprehend(bucketName string, objectName string, callerId string) error {

	c := comprehend.New(awshelper.NewAWSSession())

	documentId, documentName := h.dissectObjectName(objectName)
	tags, err := h.s3.GetTagsS3(bucketName, objectName)

	// Initialise the pipeline payload
	var operationsBody = map[string]interface{}{
		"documentId": documentId,
		"bucketName": bucketName,
		"objectName": objectName,
		"stage":      PIPELINE_STAGE,
	}

	if err != nil {
		log.Printf("Failed to get tags from S3. Error: %s \n", err)
		return err
	}

	if tags["documentId"] != documentId {
		log.Printf("File path %s does not match the expected documentId tag of the object triggered.", objectName)
		return fmt.Errorf("file path %s does not match the expected documentId tag of the object triggered", objectName)
	}

	textractOutputBytes, err := h.s3.ReadFromS3(bucketName, objectName)
	if err != nil {
		log.Printf("Failed to read from S3. Error: %s \n", err)
		failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Could not read Textract results from S3.")
		if failerr != nil {
			log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
		}
		return err
	}

	var results *textract.AnalyzeDocumentOutput
	err = json.Unmarshal(textractOutputBytes, &results)
	if err != nil {
		failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Could not convert results from Textract into processable object. Try again.")
		if failerr != nil {
			log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
		}
		return err
	}

	opg := textractparser.NewOutputGenerator(h.s3, results, documentId, bucketName, objectName, false, false)

	err = h.pipelineOperationsClient.StageInProgress(operationsBody, "")
	if err != nil {
		log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, err)
		return err
	}

	document := textractparser.NewDocument(results)
	originalFileName := fmt.Sprintf("%s/%s", documentId, documentName)
	comprehendFileName := originalFileName + "/comprehend-output.json"
	tagging := "documentId=" + documentId

	esPayload := []map[string]interface{}{}
	pageNum := 1

	for _, page := range document.Pages {
		table, err := opg.OutputTable(page, 0, true)
		if err != nil {
			log.Printf("Error getting table from page %d. Error: %s \n", pageNum, err)
		}
		forms, err := opg.OutputForm(page, 0, true)
		if err != nil {
			log.Printf("Error getting forms from page %d. Error: %s \n", pageNum, err)
		}
		text, err := opg.OutputText(page, 0, true)
		if err != nil {
			log.Printf("Error getting text from page %d. Error: %s \n", pageNum, err)
		}

		keyPhrases := make([]string, 0)
		entitiesDetected := map[string]string{}
		lenOfEncodedText := len(text)

		log.Printf("Comprehend documentId %s processing page %d \n", documentId, pageNum)
		log.Printf("Length of encoded text is %d \n", lenOfEncodedText)
		if lenOfEncodedText == 0 {
			// pass
		} else if lenOfEncodedText > COMPREHEND_CHARACTER_LIMIT {
			log.Printf("Size was too big to run singularly; breaking up the page text into chunks \n")
			chunksOfText := h.chunkUpTheText(text)
			keyPhrases, entitiesDetected, err = h.batchSendToComprehend(c, chunksOfText, "en")
			if err != nil {
				failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Could not send text to Comprehend.")
				if failerr != nil {
					log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
				}
				return err
			}
		} else {
			keyPhrases, entitiesDetected, err = h.singularSendToComprehend(c, text, "en")
			if err != nil {
				failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Could not send text to Comprehend.")
				if failerr != nil {
					log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
				}
				return err
			}
		}

		esPageLoad := map[string]interface{}{
			"documentId": documentId,
			"page":       pageNum,
			"KeyPhrases": keyPhrases,
			"Entities":   entitiesDetected,
			"text":       text,
			"table":      table,
			"forms":      forms,
		}

		esPayload = append(esPayload, esPageLoad)
		pageNum = pageNum + 1

		// Marshal ES payload
		esPageBytes, err := json.Marshal(esPageLoad)
		if err != nil {
			log.Println("Error serializing response: ", err)
			return err
		}

		// Bulk post to ES
		if h.es != nil {
			err = h.es.PostBulk(documentId, esPageBytes)
			if err != nil {
				log.Println("Error writing to ES: ", err)
				failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Failed to write comprehend payload to ES")
				if failerr != nil {
					log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
				}
				return err
			}
			log.Println("data uploaded to ES")
		} else {
			log.Println("Elasticsearch is not configured. Skipping ES upload.")
		}
	}

	// Marshal ES payload
	esBytes, err := json.Marshal(esPayload)
	if err != nil {
		log.Println("Error serializing response: ", err)
		return err
	}
	esJsonPayload := string(esBytes)
	log.Println("Serialized Es Payload:", esJsonPayload)
	
	// Write to S3
	err = h.s3.WriteToS3(esJsonPayload, h.comprehendBucketName, comprehendFileName, &tagging)
	if err != nil {
		log.Println("Error writing to S3: ", err)
		failerr := h.pipelineOperationsClient.StageFailed(operationsBody, "Failed to write comprehend payload to S3")
		if failerr != nil {
			log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, failerr)
		}
		return err
	}

	// Record the lineage
	var lineageBody = map[string]interface{}{
		"documentId":       documentId,
		"callerId":         callerId,
		"sourceBucketName": bucketName,
		"targetBucketName": h.comprehendBucketName,
		"sourceFileName":   objectName,
		"targetFileName":   comprehendFileName,
	}

	err = h.documentLineageClient.RecordLineage(lineageBody)
	if err != nil {
		log.Printf("Error recording lineage for document %s. Error: %s \n", documentId, err)
	}

	// Update the pipeline stage
	err = h.pipelineOperationsClient.StageSucceeded(operationsBody, "")
	if err != nil {
		log.Printf("Error updating pipeline stage for document %s. Error: %s \n", documentId, err)
		return err
	}

	return nil

}

func (h *handler) handleRequest(ctx context.Context, s3Event events.S3Event) error {
	// Print the event
	log.Printf("Comprehend Processor event: {%+v} \n", s3Event)

	lc, _ := lambdacontext.FromContext(ctx)
	callerId := lc.InvokedFunctionArn

	// Process each record
	for _, record := range s3Event.Records {
		s3 := record.S3
		log.Printf("[%s - %s] Bucket = %s, Key = %s \n", record.EventSource, record.EventTime, s3.Bucket.Name, s3.Object.Key)

		filename, extension := h.s3.GetFileNameAndExtension(s3.Object.Key)
		if filename == "fullresponse" && extension == ".json" {
			err := h.runComprehend(s3.Bucket.Name, s3.Object.Key, callerId)
			if err != nil {
				log.Printf("Failed to run Comprehend. Error: %s \n", err)
				return err
			}
		}

	}
	return nil
}

// main is called only once, when the Lambda is initialised (started for the first time). Code in this function should
// primarily be used to create service clients, read environments variables, read configuration from disk etc.
func main() {
	metadataTopic := os.Getenv("METADATA_SNS_TOPIC_ARN")
	comprehendBucketName := os.Getenv("TARGET_COMPREHEND_BUCKET")
	esCluster := os.Getenv("TARGET_ES_CLUSTER")
	esIndex := os.Getenv("ES_CLUSTER_INDEX")

	if metadataTopic == "" {
		panic("Missing METADATA_SNS_TOPIC_ARN environment variable.")
	}
	if comprehendBucketName == "" {
		panic("Missing TEXTRACT_RESULTS_BUCKET_NAME environment variable.")
	}
	if esCluster != "" {
		esCluster = "https://" + esCluster
	}
	if esIndex == "" {
		panic("Missing ES_CLUSTER_INDEX environment variable.")
	}

	// Create AWS helpers
	s3helper := awshelper.S3Helper{S3Client: s3.New(awshelper.NewAWSSession())}
	var eshelper *awshelper.ESHelper
	if esCluster != "" {
		eshelper = awshelper.NewESHelper(esCluster, esIndex)
	}

	//Create Metadata Clients
	pipelineClient := metadata.NewPipelineOperationsClient(metadataTopic)
	lineageClient := metadata.NewDocumentLineageClient(metadataTopic)
	h := handler{
		pipelineOperationsClient: pipelineClient,
		documentLineageClient:    lineageClient,
		s3:                       &s3helper,
		comprehendBucketName:     comprehendBucketName,
		es:                       eshelper,
	}

	lambda.Start(h.handleRequest)
}
