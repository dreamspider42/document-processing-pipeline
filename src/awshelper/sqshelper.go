package awshelper

import (
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

// Helper utility for SQS
type SQSHelper struct {
	SQSClient sqsiface.SQSAPI
}

// Deletes a message from the queue
func (s *SQSHelper)  DeleteMessage(queueArn, receipt string) error{
	log.Println("Deleting from the queue")
	
	queueParts := strings.Split(queueArn, ":")
	queueName := queueParts[len(queueParts)-1]
	queueAccount := queueParts[len(queueParts)-2]
	log.Println(queueName)
	log.Println(queueAccount)

	params := &sqs.GetQueueUrlInput{
		QueueName:              aws.String(queueName),
		QueueOwnerAWSAccountId: aws.String(queueAccount),
	}

	queueUrl, err := s.SQSClient.GetQueueUrl(params)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			log.Println(aerr.Code(), aerr.Error())
		} else {
			log.Println(err.Error())
		}
		return err
	}

	// Delete the message.
	_, err = s.SQSClient.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      queueUrl.QueueUrl,
		ReceiptHandle: aws.String(receipt),
	})
	if err != nil {
		log.Println("Error deleting message: ", err)
		return err
	}

	log.Println("Deleted message")
	return nil
}