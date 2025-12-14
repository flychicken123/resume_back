package services

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
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

	// Upload to S3 with simple retries to handle transient network issues
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, lastErr = s.s3Client.PutObject(input)
		if lastErr == nil {
			break
		}
		log.Printf("S3 upload attempt %d failed: %v", attempt, lastErr)
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	if lastErr != nil {
		return "", fmt.Errorf("failed to upload to S3 after retries: %v", lastErr)
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

// DownloadBytes downloads an object from S3 and returns its raw bytes.
func (s *S3Service) DownloadBytes(key string) ([]byte, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("s3 key is required")
	}
	if err := s.validate(); err != nil {
		return nil, err
	}

	resp, err := s.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %v", err)
	}
	defer resp.Body.Close()

	const maxSize = 25 << 20 // 25 MiB
	limited := &io.LimitedReader{R: resp.Body, N: maxSize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed reading S3 object: %v", err)
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("s3 object exceeds max size (%d bytes)", maxSize)
	}
	return data, nil
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
