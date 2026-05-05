package handlers

import (
	"context"
	"strings"

	"resumeai/services"
)

type chatQualityDraftGenerator struct{}

func (chatQualityDraftGenerator) GenerateDraft(ctx context.Context, input services.QualityGateInput) (services.QualityDraftResult, error) {
	reply, meta, err := chatCallWithRetry(func() (string, *services.ToolCallMetadata, error) {
		return callGeminiToolsBlocking(ctx, input.SystemPrompt, input.UserPrompt, input.Tools, input.UserID)
	})
	return services.QualityDraftResult{Reply: reply, ToolMeta: meta}, err
}

func buildChatQualitySourceContext(parts ...string) string {
	var b strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		b.WriteString("--- BEGIN UNTRUSTED SOURCE BLOCK ---\n")
		b.WriteString(part)
		b.WriteString("\n--- END UNTRUSTED SOURCE BLOCK ---\n")
	}
	return strings.TrimSpace(b.String())
}

func streamFinalAnswerTokens(ctx context.Context, sse *sseWriter, answer string) error {
	if sse == nil {
		return nil
	}
	for _, chunk := range splitAnswerForTokenEvents(answer) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sse.WriteToken(chunk); err != nil {
			return err
		}
	}
	return nil
}

func splitAnswerForTokenEvents(answer string) []string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil
	}
	words := strings.Fields(answer)
	if len(words) == 0 {
		return []string{answer}
	}
	chunks := make([]string, 0, len(words))
	for i, word := range words {
		if i < len(words)-1 {
			word += " "
		}
		chunks = append(chunks, word)
	}
	return chunks
}
