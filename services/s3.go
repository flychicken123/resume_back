package services

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Service struct {
	s3Client *s3.S3
	bucket   string
	region   string
}

func NewS3Service() (*S3Service, error) {
	// Get AWS credentials from environment variables
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")
	bucket := os.Getenv("AWS_S3_BUCKET")

	if accessKey == "" || secretKey == "" || region == "" || bucket == "" {
		return nil, fmt.Errorf("AWS credentials not configured")
	}

	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Create S3 client
	s3Client := s3.New(sess)

	return &S3Service{
		s3Client: s3Client,
		bucket:   bucket,
		region:   region,
	}, nil
}

// UploadFile uploads a file to S3 and returns the download URL
func (s *S3Service) UploadFile(filePath, fileName string) (string, error) {
	// Read file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Create S3 upload input
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(fileName),
		Body:        bytes.NewReader(fileContent),
		ContentType: aws.String("application/pdf"),
		// Note: ACL is removed as the bucket doesn't support ACLs
		// The bucket should be configured for public read access
	}

	// Upload to S3
	_, err = s.s3Client.PutObject(input)
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %v", err)
	}

	// Generate download URL
	downloadURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, fileName)

	log.Printf("File uploaded to S3: %s", downloadURL)
	return downloadURL, nil
}

// GeneratePresignedURL generates a presigned URL for secure downloads
func (s *S3Service) GeneratePresignedURL(fileName string) (string, error) {
	req, _ := s.s3Client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fileName),
	})

	// Generate presigned URL that expires in 1 hour
	url, err := req.Presign(1 * time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}

	return url, nil
}

// DeleteFile deletes a file from S3
func (s *S3Service) DeleteFile(fileName string) error {
	// Create S3 delete input
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fileName),
	}

	// Delete from S3
	_, err := s.s3Client.DeleteObject(input)
	if err != nil {
		return fmt.Errorf("failed to delete file from S3: %v", err)
	}

	log.Printf("File deleted from S3: %s", fileName)
	return nil
}

// validate checks if the S3Service configuration is valid
func (s *S3Service) validate() error {
	if s.bucket == "" {
		return fmt.Errorf("bucket name is required")
	}

	if s.region == "" {
		return fmt.Errorf("region is required")
	}

	return nil
}
