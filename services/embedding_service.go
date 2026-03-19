package services

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"resumeai/models"
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// EmbeddingService wraps the Gemini embedding API.
type EmbeddingService struct {
	client *genai.Client
}

// NewEmbeddingService creates an EmbeddingService. Returns an error if apiKey is empty
// or if the underlying gRPC client cannot be created.
func NewEmbeddingService(apiKey string) (*EmbeddingService, error) {
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY is not set")
	}
	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return &EmbeddingService{client: client}, nil
}

// Close releases the underlying gRPC connection.
func (s *EmbeddingService) Close() error {
	return s.client.Close()
}

// embedText calls the Gemini embedding API for a single piece of text.
// A fresh EmbeddingModel is created per call to avoid races on shared TaskType state.
func (s *EmbeddingService) embedText(ctx context.Context, text string, taskType genai.TaskType) ([]float32, error) {
	em := s.client.EmbeddingModel("gemini-embedding-001")
	em.TaskType = taskType
	resp, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Embedding == nil {
		return nil, errors.New("empty embedding response")
	}
	return resp.Embedding.Values, nil
}

// EmbedJobPosting generates a RETRIEVAL_DOCUMENT embedding for a job posting.
// Returns an error if the posting produces empty text after HTML stripping.
func (s *EmbeddingService) EmbedJobPosting(ctx context.Context, posting *models.JobPosting) ([]float32, error) {
	cleanDescription := htmlTagRe.ReplaceAllString(posting.Description, " ")
	text := posting.Title + " " + posting.Department + " " + cleanDescription
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("job posting has no embeddable text")
	}
	runes := []rune(text)
	if len(runes) > 6000 {
		text = string(runes[:6000])
	}
	return s.embedText(ctx, text, genai.TaskTypeRetrievalDocument)
}

// EmbedResumeContent generates a RETRIEVAL_QUERY embedding for a resume match input.
// Returns an error if the input produces empty text.
func (s *EmbeddingService) EmbedResumeContent(ctx context.Context, input ResumeJobMatchInput) ([]float32, error) {
	text := strings.Join([]string{
		input.Position,
		input.Summary,
		input.Experience,
		strings.Join(input.Skills, " "),
		input.PreferredLocation,
	}, " ")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("resume input has no embeddable text")
	}
	runes := []rune(text)
	if len(runes) > 6000 {
		text = string(runes[:6000])
	}
	return s.embedText(ctx, text, genai.TaskTypeRetrievalQuery)
}
