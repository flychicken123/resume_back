package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Add middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Add routes
	r.POST("/api/resume/generate", GenerateResume)
	r.POST("/api/experience/optimize", OptimizeExperience)
	r.POST("/api/experience/optimize-batch", OptimizeExperienceBatch)

	return r
}

// Mock the Python script execution for testing
func init() {
	// Set test environment variables
	os.Setenv("GEMINI_API_KEY", "test-key")
	os.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("AWS_REGION", "us-east-1")
	os.Setenv("AWS_S3_BUCKET", "test-bucket")
}

func TestGenerateResume_InvalidJSON(t *testing.T) {
	router := setupTestRouter()

	// Test invalid JSON
	req, err := http.NewRequest("POST", "/api/resume/generate", bytes.NewBuffer([]byte("invalid json")))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "error")
}

func TestGenerateResume_Success(t *testing.T) {
	origGenerator := htmlResumeGenerator
	t.Cleanup(func() {
		htmlResumeGenerator = origGenerator
	})

	called := false
	htmlResumeGenerator = func(templateName string, userData map[string]interface{}, outputPath string) error {
		called = true
		if templateName != defaultTemplateSlug {
			t.Fatalf("expected default template %s, got %s", defaultTemplateSlug, templateName)
		}
		return os.WriteFile(outputPath, []byte("<html>ok</html>"), 0o644)
	}

	wd, err := os.Getwd()
	assert.NoError(t, err)

	tmpDir := t.TempDir()
	assert.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/resume/generate", GenerateResume)

	body := map[string]interface{}{
		"name":       "Jane Doe",
		"email":      "jane@example.com",
		"phone":      "123-456-7890",
		"summary":    "Product-focused engineer",
		"experience": "Built things",
		"education":  "BS Computer Science",
		"skills":     []string{"Go", "React"},
	}
	payload, err := json.Marshal(body)
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "/api/resume/generate", bytes.NewBuffer(payload))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called, "expected HTML generator to be invoked")

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Resume generated successfully.", resp["message"])

	filePath, ok := resp["filePath"].(string)
	if !ok {
		t.Fatalf("expected filePath string, got %v", resp["filePath"])
	}
	assert.NotEmpty(t, filePath)

	relativePath := strings.TrimPrefix(filePath, "/")
	fullPath := filepath.FromSlash(relativePath)
	_, err = os.Stat(fullPath)
	assert.NoError(t, err, "expected generated resume file to exist")
}

func TestOptimizeExperience_InvalidJSON(t *testing.T) {
	router := setupTestRouter()

	// Test invalid JSON
	req, err := http.NewRequest("POST", "/api/experience/optimize", bytes.NewBuffer([]byte("invalid json")))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "error")
}

func TestOptimizeExperience_MissingFields(t *testing.T) {
	router := setupTestRouter()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
	}{
		{
			name: "missing userExperience",
			requestBody: map[string]interface{}{
				"jobDescription": "Looking for a senior developer",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty userExperience",
			requestBody: map[string]interface{}{
				"userExperience": "",
				"jobDescription": "Looking for a senior developer",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			assert.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/experience/optimize", bytes.NewBuffer(body))
			assert.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestOptimizeExperienceBatch_InvalidJSON(t *testing.T) {
	router := setupTestRouter()

	req, err := http.NewRequest("POST", "/api/experience/optimize-batch", bytes.NewBuffer([]byte("invalid json")))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOptimizeExperienceBatch_MissingExperiences(t *testing.T) {
	router := setupTestRouter()

	body, err := json.Marshal(map[string]interface{}{
		"jobDescription": "Looking for a senior developer",
		"experiences":    []interface{}{},
	})
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "/api/experience/optimize-batch", bytes.NewBuffer(body))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// validateResumeRequest validates the ResumeRequest struct
func validateResumeRequest(req ResumeRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}

	if req.Email == "" {
		return fmt.Errorf("email is required")
	}

	// Validate email format
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		return fmt.Errorf("invalid email format")
	}

	if req.Phone == "" {
		return fmt.Errorf("phone is required")
	}

	return nil
}

func TestResumeRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request ResumeRequest
		isValid bool
	}{
		{
			name: "valid request",
			request: ResumeRequest{
				Name:       "John Doe",
				Email:      "john@example.com",
				Phone:      "123-456-7890",
				Summary:    "Experienced software engineer",
				Experience: "Software Engineer at Google",
				Education:  "Bachelor of Science",
				Skills:     []string{"JavaScript", "React"},
				Format:     defaultTemplateSlug,
			},
			isValid: true,
		},
		{
			name: "missing name",
			request: ResumeRequest{
				Email:      "john@example.com",
				Phone:      "123-456-7890",
				Summary:    "Experienced software engineer",
				Experience: "Software Engineer at Google",
				Education:  "Bachelor of Science",
				Skills:     []string{"JavaScript", "React"},
				Format:     defaultTemplateSlug,
			},
			isValid: false,
		},
		{
			name: "missing email",
			request: ResumeRequest{
				Name:       "John Doe",
				Phone:      "123-456-7890",
				Summary:    "Experienced software engineer",
				Experience: "Software Engineer at Google",
				Education:  "Bachelor of Science",
				Skills:     []string{"JavaScript", "React"},
				Format:     defaultTemplateSlug,
			},
			isValid: false,
		},
		{
			name: "invalid email format",
			request: ResumeRequest{
				Name:       "John Doe",
				Email:      "invalid-email",
				Phone:      "123-456-7890",
				Summary:    "Experienced software engineer",
				Experience: "Software Engineer at Google",
				Education:  "Bachelor of Science",
				Skills:     []string{"JavaScript", "React"},
				Format:     defaultTemplateSlug,
			},
			isValid: false,
		},
		{
			name: "missing phone",
			request: ResumeRequest{
				Name:       "John Doe",
				Email:      "john@example.com",
				Summary:    "Experienced software engineer",
				Experience: "Software Engineer at Google",
				Education:  "Bachelor of Science",
				Skills:     []string{"JavaScript", "React"},
				Format:     defaultTemplateSlug,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResumeRequest(tt.request)
			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
