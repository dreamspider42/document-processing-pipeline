package metadata

import "log"

type DocumentRegistryClient struct {
	metadataClient *MetadataClient
}

func NewDocumentRegistryClient(metadataTopic string, body map[string]interface{}) *DocumentRegistryClient {
	metadataClient := NewMetadataClient(
		"document-registry",
		metadataTopic,
		"sns",
		body,
		[]string{"documentId", "bucketName", "documentName", "documentLink", "principalIAMWriter"},
	)
	return &DocumentRegistryClient{
		metadataClient: metadataClient,
	}
}

func (d *DocumentRegistryClient) RegisterDocument(body map[string]interface{}) error {
	log.Printf("*DocumentRegistration* Publishing event with body: %v \n", body)
	return d.metadataClient.Publish(body)
}
