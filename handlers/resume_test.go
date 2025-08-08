package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
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

	return r
}

func TestGenerateResume(t *testing.T) {
	router := setupTestRouter()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "valid resume generation request",
			requestBody: map[string]interface{}{
				"name":        "John Doe",
				"email":       "john@example.com",
				"phone":       "123-456-7890",
				"summary":     "Experienced software engineer",
				"experience":  "Software Engineer at Google",
				"education":   "Bachelor of Science in Computer Science",
				"skills":      []string{"JavaScript", "React", "Go"},
				"format":      "temp1",
				"htmlContent": "<div>Test HTML</div>",
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)

				// Check that response contains expected fields
				assert.Contains(t, response, "message")
				assert.Contains(t, response, "filePath")
			},
		},
		{
			name: "missing required fields",
			requestBody: map[string]interface{}{
				"name": "John Doe",
				// Missing email, phone, etc.
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		{
			name:           "invalid JSON",
			requestBody:    nil,
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error

			if tt.requestBody != nil {
				body, err = json.Marshal(tt.requestBody)
				assert.NoError(t, err)
			} else {
				body = []byte("invalid json")
			}

			req, err := http.NewRequest("POST", "/api/resume/generate", bytes.NewBuffer(body))
			assert.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.checkResponse(t, w)
		})
	}
}

func TestOptimizeExperience(t *testing.T) {
	router := setupTestRouter()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "valid experience optimization request",
			requestBody: map[string]interface{}{
				"userExperience": "Software Engineer at Google",
				"jobDescription": "Looking for a senior developer with React experience",
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)

				// Check that response contains expected fields
				assert.Contains(t, response, "optimizedExperience")
			},
		},
		{
			name: "missing userExperience",
			requestBody: map[string]interface{}{
				"jobDescription": "Looking for a senior developer",
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		{
			name: "empty userExperience",
			requestBody: map[string]interface{}{
				"userExperience": "",
				"jobDescription": "Looking for a senior developer",
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
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
			tt.checkResponse(t, w)
		})
	}
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
				Format:     "temp1",
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
				Format:     "temp1",
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
				Format:     "temp1",
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
				Format:     "temp1",
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
				Format:     "temp1",
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
