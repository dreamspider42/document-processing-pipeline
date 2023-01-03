package awshelper

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// Helper utility for S3
type S3Helper struct {
	S3Client s3iface.S3API
}

func (s *S3Helper) GetFileNameAndExtension(filePath string) (string, string) {
    basename := filepath.Base(filePath)
	extension := filepath.Ext(basename)
	fileName := strings.TrimSuffix(basename, extension)

	return fileName, extension
}

func (s *S3Helper) GetTagsS3(bucketName string, fileName string) (map[string]string, error) {
	// Get the tags
	tagOutput, err := s.S3Client.GetObjectTagging(&s3.GetObjectTaggingInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileName),
	})
	if err != nil {
		log.Println("Got error getting tags for object")
		log.Println(err.Error())
		return nil, err
	}
	log.Println("Successfully got tags for object")

	tagMap := make(map[string]string)
	for _, tag := range tagOutput.TagSet {
		log.Printf("Tag: %s, Value: %s \n", *tag.Key, *tag.Value)
		tagMap[*tag.Key] = *tag.Value
	}
	
	return tagMap, nil
}

func (s *S3Helper) CopyToS3(sourceBucketName string, sourceFilename string, targetBucketName string, targetFileName string) (*s3.CopyObjectOutput, error) {
	return s.S3Client.CopyObject(&s3.CopyObjectInput{
		Bucket:     aws.String(targetBucketName),
		Key:        aws.String(targetFileName),
		CopySource: aws.String(fmt.Sprintf("%s/%s", sourceBucketName, sourceFilename)),
	})
}

// Tags an S3 object
func (s *S3Helper) TagS3 (bucketName string, fileName string, tags map[string]string) error {
	// Create the tag set
	tagSet := make([]*s3.Tag, len(tags))
	i := 0
	for k, v := range tags {
		tagSet[i] = &s3.Tag{Key: aws.String(k), Value: aws.String(v)}
		i++
	}

	// Create the tag input
	tagInput := &s3.PutObjectTaggingInput{
		Bucket:  aws.String(bucketName),
		Key:     aws.String(fileName),
		Tagging: &s3.Tagging{TagSet: tagSet},
	}

	// Tag the object
	_, err := s.S3Client.PutObjectTagging(tagInput)
	if err != nil {
		log.Println("Got error tagging object")
		log.Println(err.Error())
		return err
	}

	log.Println("Successfully tagged object")
	return nil
}

func (s *S3Helper) WriteToS3(content string, bucketName string, s3FileName string, taggingStr *string) error {
	_, err := s.S3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3FileName),
		Body:   strings.NewReader(content),
		Tagging: taggingStr,
	})
	return err
}

func (s *S3Helper) WriteCSV(fieldNames []string, csvData [][]string, bucketName string, s3FileName string) error {
	csv_file := bytes.NewBufferString("")
	writer := csv.NewWriter(csv_file)
	writer.Write(fieldNames)
	writer.WriteAll(csvData)
	return s.WriteToS3(csv_file.String(), bucketName, s3FileName, nil)
}

func (s *S3Helper) WriteCSVRaw(csvData [][]string, bucketName string, s3FileName string) error {
	csv_file := bytes.NewBufferString("")
	writer := csv.NewWriter(csv_file)
	for _, item := range csvData {
		writer.Write(item)
	}
	writer.Flush()
	return s.WriteToS3(csv_file.String(), bucketName, s3FileName, nil)
}

func (s *S3Helper) ListObjectsInS3(bucketName string, bucketPrefix string, maxKeys int64) ([]*string, error) {
	log.Printf("First call listing objects in S3 bucket %s with prefix %s \n", bucketName, bucketPrefix)
	res, err := s.S3Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
		Prefix: aws.String(bucketPrefix),
		MaxKeys: aws.Int64(maxKeys),
	})
	if err != nil {
		log.Println("Got error listing objects in S3 first time: ", err.Error())
		return nil, err
	}
	
	s3Keys := make([]*string, len(res.Contents))
	isTruncated := *res.IsTruncated
	nextToken := res.NextContinuationToken
	for i, object := range res.Contents {
		s3Keys[i] = object.Key
	}

	i := 1
	for isTruncated {
		i++
		log.Printf("Calling to list objects for page: %d \n", i)
		res, err := s.S3Client.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
			Prefix: aws.String(bucketPrefix),
			MaxKeys: aws.Int64(maxKeys),
			ContinuationToken: nextToken,
		})
		if err != nil {
			log.Println("Got error listing objects in S3 while iterating: ", err.Error())
			return nil, err
		}
		
		isTruncated = *res.IsTruncated
		nextToken = res.NextContinuationToken
		for _, object := range res.Contents {
			s3Keys = append(s3Keys, object.Key)
		}
	}

	return s3Keys, nil
}

func (s *S3Helper) ReadFromS3(bucketName string, s3FileName string) ([]byte, error) {
	res, err := s.S3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3FileName),
	})
	if err != nil {
		log.Println("Got error reading object from S3: ", err.Error())
		return nil, err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(res.Body)
	
	return buf.Bytes(), nil
}