package awshelper

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

// Creates a new AWS session with default settings
func NewAWSSession() *session.Session {
	return session.Must(session.NewSession(
		&aws.Config{
			MaxRetries: aws.Int(30),
		},
	))
}