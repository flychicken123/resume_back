package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupParseResumeTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/resume/parse", ParseResume)
	return r
}

func newResumeUploadRequest(t *testing.T, filename, content string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("resume", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/resume/parse", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func decodeJSONBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response JSON: %v\nbody: %s", err, w.Body.String())
	}
	return body
}

func withParseResumeStubs(t *testing.T, ai func(string, float64) (string, error), pythonFallback func(string) map[string]interface{}) {
	t.Helper()

	originalAI := parseResumeAI
	originalPythonFallback := parseResumePythonFallback
	parseResumeAI = ai
	parseResumePythonFallback = pythonFallback

	t.Cleanup(func() {
		parseResumeAI = originalAI
		parseResumePythonFallback = originalPythonFallback
	})
}

func sampleResumeText() string {
	return `Jane Doe
jane@example.com
555-123-4567

Experience
Acme Inc
Software Engineer
- Built APIs for internal users

Skills
Go, React`
}

func TestParseResume_FallsBackOnGeminiQuotaError(t *testing.T) {
	withParseResumeStubs(t,
		func(prompt string, temperature float64) (string, error) {
			if temperature != 0.0 {
				t.Fatalf("expected temperature 0.0, got %v", temperature)
			}
			if !strings.Contains(prompt, "Jane Doe") {
				t.Fatalf("expected prompt to include resume text")
			}
			return "", errors.New("googleapi: Error 429: billing account has exceeded its monthly spending cap")
		},
		func(string) map[string]interface{} {
			t.Fatalf("python fallback should not be called for readable txt input")
			return nil
		},
	)

	router := setupParseResumeTestRouter()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newResumeUploadRequest(t, "resume.txt", sampleResumeText()))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	body := decodeJSONBody(t, w)
	if body["method"] != "simple_fallback" {
		t.Fatalf("expected simple_fallback method, got %v", body["method"])
	}
	if body["warning"] != resumeAIParseFallbackWarning {
		t.Fatalf("expected fallback warning, got %v", body["warning"])
	}
	if !strings.Contains(body["aiError"].(string), "429") {
		t.Fatalf("expected aiError to include quota status, got %v", body["aiError"])
	}

	structured := body["structured"].(map[string]interface{})
	if structured["name"] != "Jane Doe" {
		t.Fatalf("expected fallback name, got %v", structured["name"])
	}
	if structured["email"] != "jane@example.com" {
		t.Fatalf("expected fallback email, got %v", structured["email"])
	}
}

func TestParseResume_FallsBackOnInvalidOrTruncatedGeminiJSON(t *testing.T) {
	tests := []struct {
		name       string
		aiResponse string
	}{
		{name: "invalid_json", aiResponse: "not json"},
		{name: "truncated_json", aiResponse: `{"name":"Jane Doe"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withParseResumeStubs(t,
				func(string, float64) (string, error) {
					return tt.aiResponse, nil
				},
				func(string) map[string]interface{} {
					t.Fatalf("python fallback should not be called for readable txt input")
					return nil
				},
			)

			router := setupParseResumeTestRouter()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, newResumeUploadRequest(t, "resume.txt", sampleResumeText()))

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
			}

			body := decodeJSONBody(t, w)
			if body["method"] != "simple_fallback" {
				t.Fatalf("expected simple_fallback method, got %v", body["method"])
			}
			if body["aiError"] != "AI output was not valid JSON" {
				t.Fatalf("expected invalid JSON aiError, got %v", body["aiError"])
			}
			if body["warning"] != resumeAIParseFallbackWarning {
				t.Fatalf("expected fallback warning, got %v", body["warning"])
			}
		})
	}
}

func TestParseResume_ReturnsUnprocessableForEmptyOrUnsupportedResumeText(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
	}{
		{name: "empty_txt", filename: "resume.txt", content: " \n\t "},
		{name: "unsupported_format", filename: "resume.rtf", content: "{\\rtf1 Jane Doe}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withParseResumeStubs(t,
				func(string, float64) (string, error) {
					t.Fatalf("AI should not be called when no resume text was extracted")
					return "", nil
				},
				func(string) map[string]interface{} {
					return nil
				},
			)

			router := setupParseResumeTestRouter()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, newResumeUploadRequest(t, tt.filename, tt.content))

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d body=%s", w.Code, w.Body.String())
			}

			body := decodeJSONBody(t, w)
			if body["error"] != resumeTextExtractionError {
				t.Fatalf("expected extraction error, got %v", body["error"])
			}
		})
	}
}
