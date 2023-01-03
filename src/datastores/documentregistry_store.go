package datastores

import (
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
)

// Represents the Document Registry DynamoDB
type DocumentRegistryStore struct {
    registryTableName string
    dynamoDB          dynamodbiface.DynamoDBAPI
}

// Represents a Document Registry record
type DocumentRegistryItem struct {
    DocumentId          string `json:"documentId"`
    PrincipalIAMWriter  map[string]interface{} `json:"principalIAMWriter"`
    BucketName          string `json:"bucketName"`
    DocumentName        string `json:"documentName"`
    DocumentLink        string `json:"documentLink"`
    DocumentMetadata    map[string]interface{} `json:"documentMetadata"`
    Timestamp           string `json:"timestamp"`
    DocumentVersion     *string `json:"documentVersion"`
}

// Create a new instance of the DocumentRegistryStore
func NewDocumentRegistryStore(documentRegistryName string) *DocumentRegistryStore {
	sess := session.Must(session.NewSession(
		&aws.Config{
			Region: aws.String("us-east-1"),
			MaxRetries: aws.Int(30),
		},
	))

    return &DocumentRegistryStore{
        registryTableName: documentRegistryName,
        dynamoDB:          dynamodb.New(sess),
    }
}

// Create or update a Document Registry record
func (s *DocumentRegistryStore) RegisterDocument(item DocumentRegistryItem) error {
    av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.Println("Got error marshalling map:")
		log.Println(err.Error())
		return err
	}

    // Put item in Document Registry
    _, err = s.dynamoDB.PutItem(&dynamodb.PutItemInput{
        TableName:                 aws.String(s.registryTableName),
        Item:                      av,
        ConditionExpression:       aws.String("attribute_not_exists(documentId)"),
    })

	// Handle DynamoDB error codes
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			// Print the dynamo code and error message
			log.Println(aerr.Code(), aerr.Error())
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Println(err.Error())
		}
	}
    return err
}