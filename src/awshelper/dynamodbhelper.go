package awshelper

import (
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

// Helper for DynamoDB
type DynamoDBHelper struct {
	DB dynamodbiface.DynamoDBAPI
}

// BEGIN: Hacks to convert from map[string]events.DynamoDBAttributeValue to map[string]*dynamodb.AttributeValue
type DynamoDBEvent struct {
	Records []DynamoDBEventRecord `json:"Records"`
}

type DynamoDBEventRecord struct {
	AWSRegion      string                       `json:"awsRegion"`
	Change         DynamoDBStreamRecord         `json:"dynamodb"`
	EventID        string                       `json:"eventID"`
	EventName      string                       `json:"eventName"`
	EventSource    string                       `json:"eventSource"`
	EventVersion   string                       `json:"eventVersion"`
	EventSourceArn string                       `json:"eventSourceARN"`
	UserIdentity   *events.DynamoDBUserIdentity `json:"userIdentity,omitempty"`
}

type DynamoDBStreamRecord struct {
	ApproximateCreationDateTime events.SecondsEpochTime `json:"ApproximateCreationDateTime,omitempty"`
	// changed to map[string]*dynamodb.AttributeValue
	Keys map[string]*dynamodb.AttributeValue `json:"Keys,omitempty"`
	// changed to map[string]*dynamodb.AttributeValue
	NewImage map[string]*dynamodb.AttributeValue `json:"NewImage,omitempty"`
	// changed to map[string]*dynamodb.AttributeValue
	OldImage       map[string]*dynamodb.AttributeValue `json:"OldImage,omitempty"`
	SequenceNumber string                              `json:"SequenceNumber"`
	SizeBytes      int64                               `json:"SizeBytes"`
	StreamViewType string                              `json:"StreamViewType"`
}

// END: Hacks to convert from map[string]events.DynamoDBAttributeValue to map[string]*dynamodb.AttributeValue

// // Write code to Convert record of map[string]dynamodb.AttributeValue to map[string]*dynamodb.AttributeValue
// func (h DynamoDBHelper) convertToDynamoDBPointers(record map[string]dynamodb.AttributeValue) (map[string]*dynamodb.AttributeValue, error) {
// 	item := make(map[string]*dynamodb.AttributeValue)
// 	for k, v := range record {
// 		item[k] = &v
// 	}
// 	return item, nil
// }

// // Convert the collection to pointers and deserialize a DynamoDB item
// func (h DynamoDBHelper) ConvertAndDeserializeItem(record DynamoDBStreamRecord ) (map[string]interface{}, error) {
// 	// convertedRecord := ddbconversions.AttributeValueMapFrom(record) Maybe use this when we upgrade to aws-sdk-gov2
// 	// recordPointers, err := h.convertToDynamoDBPointers(convertedRecord)
// 	// if err != nil {
// 	// 	log.Printf("Error converting to pointers: %v", err)
// 	// 	return nil, err
// 	// }
// 	return h.DeserializeItem(record.NewImage)
// }

// Deserialize a DynamoDB item
func (h DynamoDBHelper) DeserializeItem(record map[string]*dynamodb.AttributeValue) (map[string]interface{}, error) {
	item := make(map[string]interface{})
	err := dynamodbattribute.UnmarshalMap(record, &item)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Get multiple items from a DynamoDB table
func (h DynamoDBHelper) GetItems(tableName, key, value string) ([]map[string]interface{}, error) {
	items := make([]map[string]interface{}, 0)
	filt := expression.Key(key).Equal(expression.Value(value))
	proj := expression.NamesList(expression.Name(key), expression.Name("sk"))
	expr, err := expression.NewBuilder().WithKeyCondition(filt).WithProjection(proj).Build()
	if err != nil {
		return nil, err
	}

	params := &dynamodb.QueryInput{
		TableName:                 aws.String(tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.Filter(),
		ProjectionExpression:      expr.Projection(),
	}

	resp, err := h.DB.Query(params)
	if err != nil {
		return nil, err
	}

	// Deserialize the items
	for _, i := range resp.Items {
		item, err := h.DeserializeItem(i)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

// Insert an item into a DynamoDB table
func (h DynamoDBHelper) InsertItem(tableName string, itemData map[string]interface{}) error {
	av, err := dynamodbattribute.MarshalMap(itemData)
	if err != nil {
		return err
	}

	params := &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      av,
	}

	_, err = h.DB.PutItem(params)
	if err != nil {
		return err
	}
	return nil
}

// Delete multiple records from a DynamoDB table
func (h DynamoDBHelper) DeleteItems(tableName, key, value, sk string) error {
	items, err := h.GetItems(tableName, key, value)
	if err != nil {
		return err
	}

	for _, item := range items {
		log.Println("Deleting...")
		log.Printf("%s : %v", key, item[key])
		log.Printf("%s : %v", sk, item[sk])
		params := &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]*dynamodb.AttributeValue{
				key: {
					S: aws.String(value),
				},
				sk: {
					S: aws.String(item[sk].(string)),
				},
			},
		}
		_, err := h.DB.DeleteItem(params)
		if err != nil {
			return err
		}
		log.Println("Deleted...")
	}

	return nil
}
