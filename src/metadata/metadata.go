package metadata

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lambda/lambdaiface"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/dreamspider42/document-processing-pipeline/src/awshelper"
	"golang.org/x/exp/maps"
)

type MetadataClient struct {
	targetArn    string
	targetType   string
	snsclient    snsiface.SNSAPI
	lambdaclient lambdaiface.LambdaAPI
	metadataType string
	requiredKeys []string
	body         map[string]interface{}
}

func NewMetadataClient(metadataType string, targetArn string, targetType string, body map[string]interface{}, requiredKeys []string) *MetadataClient {
	var snsClient snsiface.SNSAPI = nil
	var lambdaClient lambdaiface.LambdaAPI = nil
	if targetType == "sns" {
		snsClient = sns.New(awshelper.NewAWSSession())
	} else if targetType == "lambda" {
		lambdaClient = lambda.New(awshelper.NewAWSSession())
	} else {
		panic(fmt.Sprintf("MetadataClient does not accept targets of type %s", targetType))
	}

	if body == nil {
		body = make(map[string]interface{})
	}
	return &MetadataClient{
		targetArn:    targetArn,
		targetType:   targetType,
		snsclient:    snsClient,
		lambdaclient: lambdaClient,
		metadataType: metadataType,
		requiredKeys: requiredKeys,
		body:         body,
	}
}

func (m *MetadataClient) _validatePayload(payload map[string]interface{}) bool {
	for _, key := range m.requiredKeys {
		_, ok := payload[key]
		if !ok {
			return false
		}
	}
	return true
}

func (m *MetadataClient) _publishSNS(message string, messageGroupId string, messageAttributes map[string]interface{}) error {
	log.Println("publishing to SNS")
	if m.targetType != "sns" {
		return fmt.Errorf("invalid targetType")
	}
	client := m.snsclient.(*sns.SNS)
	_, err := client.Publish(&sns.PublishInput{
		TopicArn:       &m.targetArn,
		Message:        &message,
		MessageGroupId: &messageGroupId,
		MessageAttributes: map[string]*sns.MessageAttributeValue{
			"metadataType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(m.metadataType),
			},
		},
	})
	return err
}

func (m *MetadataClient) Publish(payload map[string]interface{}) error {
	timestamp := time.Now().UTC().String()
	maps.Copy(payload, m.body)
	payload["timestamp"] = timestamp
	log.Printf("Payload to publish is: %v \n", payload)

	valid := m._validatePayload(payload)
	if !valid {
		return fmt.Errorf("incorrect client payload structure. Please double check the required keys")
	}

	documentId := payload["documentId"].(string)
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshalling payload: %v", err)
	}

	if m.targetType == "sns" {
		metaDataTypeAttribute := map[string]interface{}{
			"DataType":    "String",
			"StringValue": m.metadataType,
		}

		messageAttributes := map[string]interface{}{
			"metadataType": metaDataTypeAttribute,
		}
		err = m._publishSNS(string(payloadJson), documentId, messageAttributes)
	} else if m.targetType == "lambda" {
		return fmt.Errorf("lambda not implemented yet")
	} else {
		return fmt.Errorf("invalid targetType")
	}
	return err
}
