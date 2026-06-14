package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// VoiceAgent wraps an LLM client to transcribe audio using Gemini.
type VoiceAgent struct {
	llm llms.Model
}

// VoiceTranscriptionResult represents the result of audio transcription.
type VoiceTranscriptionResult struct {
	Text    string `json:"text"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// NewVoiceAgent constructs a LangChain-backed agent using the GEMINI_API_KEY.
func NewVoiceAgent() (*VoiceAgent, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	// Use the default Gemini model to ensure the API supports generateContent.
	llm, err := googleai.New(context.Background(),
		googleai.WithAPIKey(apiKey),
		googleai.WithDefaultModel(DefaultGeminiGenerationModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LangChain GoogleAI client for voice agent: %w", err)
	}

	return &VoiceAgent{llm: llm}, nil
}

// TranscribeAudio transcribes audio data using Gemini's multimodal capabilities.
func (a *VoiceAgent) TranscribeAudio(ctx context.Context, audioData []byte, mimeType string) (VoiceTranscriptionResult, error) {
	if a == nil || a.llm == nil {
		return VoiceTranscriptionResult{Success: false, Error: "voice agent is not initialized"}, fmt.Errorf("voice agent is not initialized")
	}

	if len(audioData) == 0 {
		return VoiceTranscriptionResult{Success: false, Error: "no audio data provided"}, fmt.Errorf("no audio data provided")
	}

	// Default mime type for webm audio from browser MediaRecorder
	if mimeType == "" {
		mimeType = "audio/webm"
	}

	// Create multimodal content with audio
	content := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.BinaryPart(mimeType, audioData),
				llms.TextPart("Please transcribe this audio accurately. Return ONLY the transcribed text, nothing else. If you cannot understand the audio or it's silent, return an empty string."),
			},
		},
	}

	// Generate response using multimodal content
	response, err := a.llm.GenerateContent(ctx, content)
	if err != nil {
		return VoiceTranscriptionResult{Success: false, Error: err.Error()}, fmt.Errorf("failed to transcribe audio: %w", err)
	}

	// Extract text from response
	if len(response.Choices) == 0 {
		return VoiceTranscriptionResult{Success: false, Error: "no transcription returned"}, fmt.Errorf("no transcription returned")
	}

	transcribedText := strings.TrimSpace(response.Choices[0].Content)

	return VoiceTranscriptionResult{
		Text:    transcribedText,
		Success: true,
	}, nil
}
