package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewS3Service(t *testing.T) {
	// Test with missing AWS credentials
	service, err := NewS3Service()

	// Should fail without proper AWS credentials
	assert.Error(t, err)
	assert.Nil(t, service)
}

func TestGeneratePresignedURL(t *testing.T) {
	// Mock S3 service for testing
	service := &S3Service{
		bucket: "test-bucket",
		region: "us-east-1",
	}

	url, err := service.GeneratePresignedURL("test-file.pdf")

	// Should fail without proper AWS credentials
	assert.Error(t, err)
	assert.Empty(t, url)
}

func TestS3ServiceValidation(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		region  string
		isValid bool
	}{
		{
			name:    "valid configuration",
			bucket:  "my-bucket",
			region:  "us-east-1",
			isValid: true,
		},
		{
			name:    "empty bucket",
			bucket:  "",
			region:  "us-east-1",
			isValid: false,
		},
		{
			name:    "empty region",
			bucket:  "my-bucket",
			region:  "",
			isValid: false,
		},
		{
			name:    "both empty",
			bucket:  "",
			region:  "",
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &S3Service{
				bucket: tt.bucket,
				region: tt.region,
			}

			err := service.validate()
			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
