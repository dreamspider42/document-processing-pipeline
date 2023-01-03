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
type PipelineOperationsStore struct {
	opsTableName string
	dynamoDB     dynamodbiface.DynamoDBAPI
}

// Represents a Timeline item in a Pipeline Operations record
type TimelineItem struct {
	Timestamp string `json:"timestamp"`
	Stage     string `json:"stage"`
	Status    string `json:"status"`
}

// Represents a Pipeline Operations record
type PipelineOperationsItem struct {
	DocumentId      string         `json:"documentId"`
	BucketName      string         `json:"bucketName"`
	ObjectName      string         `json:"objectName"`
	DocumentStatus  string         `json:"documentStatus"`
	DocumentStage   string         `json:"documentStage"`
	LastUpdate      string         `json:"lastUpdate"`
	Timeline        []TimelineItem `json:"timeline"`
	DocumentVersion *string        `json:"documentVersion"`
}

type PipelineOperationsList struct {
	Documents []PipelineOperationsItem `json:"items"`
	NextToken string                   `json:"nextToken"`
}

// Create a new instance of the PipelineOperationsStore
func NewPipelineOperationsStore(opsTableName string) *PipelineOperationsStore {
	sess := session.Must(session.NewSession(
		&aws.Config{
			Region:     aws.String("us-east-1"),
			MaxRetries: aws.Int(30),
		},
	))

	return &PipelineOperationsStore{
		opsTableName: opsTableName,
		dynamoDB:     dynamodb.New(sess),
	}
}

// Start tracking a document
func (s *PipelineOperationsStore) StartDocumentTracking(item PipelineOperationsItem, receipt string) error {

	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.Println("Got error marshalling new item:")
		log.Println(err.Error())
		return err
	}

	// Put item in Document Registry
	_, err = s.dynamoDB.PutItem(&dynamodb.PutItemInput{
		TableName:           aws.String(s.opsTableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(documentId)"),
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

func (s *PipelineOperationsStore) UpdateDocumentStatus(documentId, status, stage, timestamp string, messageNote interface{}) error {
	// Create the update expression
	updateExpression := "SET documentStatus = :documentStatus, documentStage = :documentStage, lastUpdate = :lastUpdate, timeline = list_append(timeline, :new_datapoint)"
	// Create the expression attribute values
	expressionAttributeValues := map[string]*dynamodb.AttributeValue{
		":documentStatus": {
			S: aws.String(status),
		},
		":documentStage": {
			S: aws.String(stage),
		},
		":lastUpdate": {
			S: aws.String(timestamp),
		},
		":new_datapoint": {
			L: []*dynamodb.AttributeValue{
				{
					M: map[string]*dynamodb.AttributeValue{
						"timestamp": {
							S: aws.String(timestamp),
						},
						"stage": {
							S: aws.String(stage),
						},
						"status": {
							S: aws.String(status),
						},
					},
				},
			},
		},
	}
	if messageNote != nil {
		expressionAttributeValues[":new_datapoint"].L[0].M["message"] = &dynamodb.AttributeValue{
			S: aws.String(messageNote.(string)),
		}
	}

	// Update the item
	_, err := s.dynamoDB.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(s.opsTableName),
		Key: map[string]*dynamodb.AttributeValue{
			"documentId": {
				S: aws.String(documentId),
			},
		},
		UpdateExpression:          aws.String(updateExpression),
		ConditionExpression:       aws.String("attribute_exists(documentId)"),
		ExpressionAttributeValues: expressionAttributeValues,
		ReturnValues:              aws.String("UPDATED_NEW"),
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

func (s *PipelineOperationsStore) MarkDocumentComplete(documentId string, stage string, timestamp string) error {
	return s.UpdateDocumentStatus(documentId, "SUCCEEDED", stage, timestamp, "")
}

func (s *PipelineOperationsStore) GetDocument(documentId string) (*PipelineOperationsItem, error) {
	result, err := s.dynamoDB.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(s.opsTableName),
		Key: map[string]*dynamodb.AttributeValue{
			"documentId": {
				S: aws.String(documentId),
			},
		},
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
		return nil, err
	}

	item := PipelineOperationsItem{}
	err = dynamodbattribute.UnmarshalMap(result.Item, &item)
	if err != nil {
		log.Println("Got error unmarshalling:")
		log.Println(err.Error())
		return nil, err
	}

	return &item, nil
}

func (s *PipelineOperationsStore) DeleteDocument(documentId string) error {
	_, err := s.dynamoDB.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(s.opsTableName),
		Key: map[string]*dynamodb.AttributeValue{
			"documentId": {
				S: aws.String(documentId),
			},
		},
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

func (s *PipelineOperationsStore) GetDocuments(nextToken string) (*PipelineOperationsList, error) {
	// Prepare scanning input with paging.
	input := &dynamodb.ScanInput{
		TableName: aws.String(s.opsTableName),
		Limit:     aws.Int64(25),
	}

	// If we have a nextToken, use it to get the next page of results
	if nextToken != "" {
		input.ExclusiveStartKey = map[string]*dynamodb.AttributeValue{
			"documentId": {
				S: aws.String(nextToken),
			},
		}
	}

	// Scan the table.
	result, err := s.dynamoDB.Scan(input)

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
		return nil, err
	}

	// Unmarshal the results into a list of PipelineOperationsItem
	items := []PipelineOperationsItem{}
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &items)
	if err != nil {
		log.Println("Got error unmarshalling:")
		log.Println(err.Error())
		return nil, err
	}

	list := PipelineOperationsList{
		Documents: items,
	}

	if result.LastEvaluatedKey != nil {
		list.NextToken = *result.LastEvaluatedKey["documentId"].S
	}

	return &list, nil
}
