package metadata

import "log"

type PipelineOperationsClient struct {
	metadataClient *MetadataClient
}

func NewPipelineOperationsClient(metadataTopic string) *PipelineOperationsClient {
	metadataClient := NewMetadataClient(
		"pipeline-operations",
		metadataTopic,
		"sns",
		nil,
		[]string{"documentId", "bucketName", "objectName", "status", "stage"},
	)
	return &PipelineOperationsClient{
		metadataClient: metadataClient,
	}
}

func (p *PipelineOperationsClient) InitDoc(body map[string]interface{}) error {
	body["status"] = "IN_PROGRESS"
	body["initDoc"] = "True"

	return p.publish(body, "")
}

func (p *PipelineOperationsClient) StageInProgress(body map[string]interface{}, message string) error {
	body["status"] = "IN_PROGRESS"
	delete(body, "initDoc")

	return p.publish(body, message)
}

func (p *PipelineOperationsClient) StageSucceeded(body map[string]interface{}, message string) error {
	body["status"] = "SUCCEEDED"
	delete(body, "initDoc")

	return p.publish(body, message)
}

func (p *PipelineOperationsClient) StageFailed(body map[string]interface{}, message string) error {
	body["status"] = "FAILED"
	delete(body, "initDoc")

	return p.publish(body, message)
}

func (p *PipelineOperationsClient) publish(body map[string]interface{}, message string) error {
	if message != "" {
		body["message"] = message
	}

	log.Printf("*PipelineOperations* Publishing event with body: %v \n", body)
	return p.metadataClient.Publish(body)
}
