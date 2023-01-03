package metadata

import (
	"log"

	"golang.org/x/exp/maps"
)

type DocumentLineageClient struct {
	metadataClient *MetadataClient
}

func NewDocumentLineageClient(metadataTopic string) *DocumentLineageClient {
	metadataClient := NewMetadataClient(
		"document-lineage",
		metadataTopic,
		"sns",
		nil,
		[]string{"documentId", "callerId", "targetBucketName", "targetFileName", "s3Event"},
	)
	return &DocumentLineageClient{
		metadataClient: metadataClient,
	}
}

func (d *DocumentLineageClient) RecordLineage(body map[string]interface{}) error {
	log.Printf("*DocumentLineage* Event to record lineage: %v \n", body)

	if _, ok := body["s3Event"]; !ok {
		maps.Copy(body, map[string]interface{}{"s3Event": "ObjectCreated:Put"})
		return d.metadataClient.Publish(body)
	} else {
		return d.metadataClient.Publish(body)
	}
}

func (d *DocumentLineageClient) RecordLineageOfCopy(body map[string]interface{}) error {
	log.Printf("*DocumentLineage* Event to record lineage of S3 Copy: %v \n", body)

	maps.Copy(body, map[string]interface{}{"s3Event": "ObjectCreated:Copy"})
	return d.metadataClient.Publish(body)
}
