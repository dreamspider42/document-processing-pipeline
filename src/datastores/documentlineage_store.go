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

// Represents the Lineage DynamoDB
type LineageStore struct {
	LineageTableName string
	LineageIndexName string
	DynamoDBClient   dynamodbiface.DynamoDBAPI
}

// Represents the Lineage DynamoDB Index
type LineageIndex struct {
	DocumentSignature string `json:"documentSignature"`
	DocumentId        string `json:"documentId"`
	Timestamp         string `json:"timestamp"`
}

// Represents a Lineage record
type LineageItem struct {
	DocumentId        string `json:"documentId"`
	DocumentSignature string `json:"documentSignature"`
	CallerId          map[string]interface{} `json:"callerId"`
	TargetBucketName  string `json:"targetBucketName"`
	TargetFileName    string `json:"targetFileName"`
	Timestamp         string `json:"timestamp"`
	S3Event           string `json:"s3Event"`
	VersionId         string `json:"versionId"`
	SourceFileName    string `json:"sourceFileName"`
	SourceBucketName  string `json:"sourceBucketName"`
}

// Create a new instance of the LineageStore
func NewLineageStore(lineageTableName, lineageIndexName string) *LineageStore {
	sess := session.Must(session.NewSession())
	return &LineageStore{
		LineageTableName: lineageTableName,
		LineageIndexName: lineageIndexName,
		DynamoDBClient:   dynamodb.New(sess),
	}
}

// Create or update a Lineage record
func (ls *LineageStore) CreateLineage(item LineageItem) error {
	documentSignature := "BUCKET:" + item.TargetBucketName + "@FILE:" + item.TargetFileName
	if item.VersionId != "" {
		documentSignature = documentSignature + "@VERSION:" + item.VersionId
	}
	item.DocumentSignature = documentSignature

	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.Println("Got error marshalling map:")
		log.Println(err.Error())
		return err
	}

	tableName := aws.String(ls.LineageTableName)
	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: tableName,
	}

	// Put item in lineagedb
	_, err = ls.DynamoDBClient.PutItem(input)

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

// Find a Lineage record by documentId
func (ls *LineageStore) QueryDocumentId(targetBucketName string, targetFileName string, versionId string) (string, error) {
	documentSignature := "BUCKET:" + targetBucketName + "@FILE:" + targetFileName
	if versionId != "" {
		documentSignature = documentSignature + "@VERSION:" + versionId
	}

	tableName := aws.String(ls.LineageTableName)
	indexName := aws.String(ls.LineageIndexName)
	keyConditionExpression := aws.String("documentSignature = :documentSignature")
	expressionAttributeValues := map[string]*dynamodb.AttributeValue{
		":documentSignature": {
			S: aws.String(documentSignature),
		},
	}

	input := &dynamodb.QueryInput{
		TableName:                 tableName,
		IndexName:                 indexName,
		KeyConditionExpression:    keyConditionExpression,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	result, err := ls.DynamoDBClient.Query(input)
	if err != nil {
		return "", err
	}

	if len(result.Items) > 0 {
		item := LineageIndex{}
		err = dynamodbattribute.UnmarshalMap(result.Items[0], &item)
		if err != nil {
			return "", err
		}
		return item.DocumentId, nil
	}
	return "", nil
}
