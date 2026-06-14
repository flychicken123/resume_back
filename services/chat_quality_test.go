package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeQualityDraftGenerator struct {
	reply string
	meta  *ToolCallMetadata
	err   error
}

func (f fakeQualityDraftGenerator) GenerateDraft(ctx context.Context, input QualityGateInput) (QualityDraftResult, error) {
	return QualityDraftResult{Reply: f.reply, ToolMeta: f.meta}, f.err
}

func resetQualityCircuitForTest() {
	chatQualityCircuit.mu.Lock()
	defer chatQualityCircuit.mu.Unlock()
	chatQualityCircuit.total = 0
	chatQualityCircuit.failures = 0
	chatQualityCircuit.openUntil = time.Time{}
}

func TestLoadQualityGateConfigDefaultsToEnabledEnforce(t *testing.T) {
	t.Setenv("AI_QUALITY_GATE_ENABLED", "")
	t.Setenv("AI_QUALITY_GATE_MODE", "")

	cfg := LoadQualityGateConfig()
	require.True(t, cfg.Enabled)
	require.Equal(t, QualityGateModeEnforce, cfg.Mode)
	require.Equal(t, defaultQualityGateTimeoutSeconds, cfg.TimeoutSeconds)
}

func TestLoadQualityGateConfigFalseDisablesGate(t *testing.T) {
	t.Setenv("AI_QUALITY_GATE_ENABLED", "false")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")

	cfg := LoadQualityGateConfig()
	require.False(t, cfg.Enabled)
	require.Equal(t, QualityGateModeOff, cfg.Mode)
}

func TestDecideChatQualityGateRoutesHighValueAndBypassesLowRisk(t *testing.T) {
	resetQualityCircuitForTest()
	t.Setenv("AI_QUALITY_GATE_ENABLED", "true")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")

	high := DecideChatQualityGate(QualityRoutingInput{UserMessage: "rewrite my resume bullet for this backend job"})
	require.True(t, high.Apply)
	require.Equal(t, QualityGateModeEnforce, high.Mode)
	require.Equal(t, "resume_rewrite", high.Intent)

	low := DecideChatQualityGate(QualityRoutingInput{UserMessage: "hello"})
	require.False(t, low.Apply)
	require.Equal(t, "low_risk", low.BypassReason)
}

func TestDecideChatQualityGateRoutesUserDataAndProductFacts(t *testing.T) {
	resetQualityCircuitForTest()
	t.Setenv("AI_QUALITY_GATE_ENABLED", "true")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")

	tests := []struct {
		name    string
		message string
		intent  string
	}{
		{
			name:    "application status",
			message: "How many jobs did I apply to and which ones are rejected?",
			intent:  "application_status",
		},
		{
			name:    "user profile",
			message: "What did I enter as my skills and background?",
			intent:  "user_profile",
		},
		{
			name:    "product facts",
			message: "What does HiHired Premium include for PDF export?",
			intent:  "product_fact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := DecideChatQualityGate(QualityRoutingInput{UserMessage: tt.message})
			require.True(t, decision.Apply)
			require.Equal(t, tt.intent, decision.Intent)
		})
	}
}

func TestDecideChatQualityGateBypassesSideEffectRequest(t *testing.T) {
	resetQualityCircuitForTest()
	t.Setenv("AI_QUALITY_GATE_ENABLED", "true")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")

	decision := DecideChatQualityGate(QualityRoutingInput{UserMessage: "move my Google application to rejected"})
	require.False(t, decision.Apply)
	require.Equal(t, "side_effect_tool_action", decision.BypassReason)
}

func TestRunChatQualityDeterministicChecksFlagsUnsupportedYears(t *testing.T) {
	result := RunChatQualityDeterministicChecks(DeterministicCheckInput{
		UserMessage:   "improve my resume",
		Intent:        "resume_rewrite",
		SourceContext: "Backend work in Go and React.",
		Answer:        "You have 10 years of Kubernetes leadership experience.",
	})

	require.False(t, result.Passed)
	require.Contains(t, result.Issues, "mentions years of experience not present in source context")
}

func TestEvalPassedForIntentUsesStricterThresholds(t *testing.T) {
	cfg := QualityGateConfig{DefaultMinScore: defaultQualityGateMinScore}
	passing := QualityEvalResult{
		Passed: true,
		Score:  0.86,
		Risk:   "low",
		Dimensions: QualityDimensionScores{
			Groundedness:    0.90,
			Specificity:     0.80,
			Completeness:    0.80,
			Usefulness:      0.80,
			FormatFollowing: 0.80,
		},
	}

	require.True(t, evalPassedForIntent(cfg, "job_match", passing))

	lowScore := passing
	lowScore.Score = 0.85
	require.False(t, evalPassedForIntent(cfg, "job_match", lowScore))

	lowGroundedness := passing
	lowGroundedness.Dimensions.Groundedness = 0.89
	require.False(t, evalPassedForIntent(cfg, "job_match", lowGroundedness))

	applicationAnswer := passing
	applicationAnswer.Score = 0.84
	applicationAnswer.Dimensions.Groundedness = 0.86
	require.True(t, evalPassedForIntent(cfg, "application_answer", applicationAnswer))
}

func TestBuildQualityEvalPromptRequiresEvidenceForPersonalAndToolClaims(t *testing.T) {
	prompt := buildQualityEvalPrompt(QualityEvalInput{
		UserMessage:   "Do I match this job?",
		DraftReply:    "Your profile shows strong AWS experience.",
		SourceContext: "--- BEGIN TOOL RESULT SUMMARY ---\nTool: job_search\nResult: {}\n--- END TOOL RESULT SUMMARY ---",
		Intent:        "job_match",
	})

	require.Contains(t, prompt, "require direct support in SOURCE MATERIAL or tool results")
	require.Contains(t, prompt, "compare the DRAFT against those tool results")
	require.Contains(t, prompt, "must not imply access to the user's resume")
}

func TestRunChatQualityGateRevisesUnsupportedDraft(t *testing.T) {
	resetQualityCircuitForTest()
	t.Setenv("AI_QUALITY_GATE_ENABLED", "true")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")
	t.Setenv("AI_QUALITY_GATE_MAX_EXTRA_MODEL_CALLS", "2")

	previousEval := qualityEvaluateLLM
	previousRevise := qualityReviseLLM
	qualityEvaluateLLM = func(ctx context.Context, prompt string) (string, error) {
		return `{
			"passed": false,
			"score": 0.52,
			"requires_revision": true,
			"risk": "high",
			"dimensions": {
				"groundedness": 0.4,
				"specificity": 0.7,
				"completeness": 0.8,
				"usefulness": 0.8,
				"format_following": 0.9
			},
			"issues": ["Invents Kubernetes leadership"],
			"unsupported_claims": [{"claim": "10 years Kubernetes leadership", "reason": "not in source"}],
			"evidence_used": []
		}`, nil
	}
	qualityReviseLLM = func(ctx context.Context, prompt string) (string, error) {
		return "Your backend Go experience is relevant. Emphasize the services you built, the tools you used, and any measurable impact already present in your resume.", nil
	}
	t.Cleanup(func() {
		qualityEvaluateLLM = previousEval
		qualityReviseLLM = previousRevise
	})

	result, err := RunChatQualityGate(context.Background(), QualityGateInput{
		UserMessage:   "rewrite my resume bullet",
		SourceContext: "Backend work in Go.",
		Intent:        "resume_rewrite",
	}, fakeQualityDraftGenerator{
		reply: "You have 10 years of Kubernetes leadership experience.",
		meta:  &ToolCallMetadata{},
	}, QualityGateCallbacks{})

	require.NoError(t, err)
	require.True(t, result.Revised)
	require.Equal(t, "Your backend Go experience is relevant. Emphasize the services you built, the tools you used, and any measurable impact already present in your resume.", result.FinalAnswer)
	require.Equal(t, 2, result.ExtraModelCalls)
	require.InDelta(t, 0.4, result.Groundedness, 0.001)
}

func TestRunChatQualityGateReturnsDraftWhenEvaluatorFailsAndDraftPasses(t *testing.T) {
	resetQualityCircuitForTest()
	t.Setenv("AI_QUALITY_GATE_ENABLED", "true")
	t.Setenv("AI_QUALITY_GATE_MODE", "enforce")

	previousEval := qualityEvaluateLLM
	qualityEvaluateLLM = func(ctx context.Context, prompt string) (string, error) {
		return "", errors.New("provider unavailable")
	}
	t.Cleanup(func() {
		qualityEvaluateLLM = previousEval
	})

	result, err := RunChatQualityGate(context.Background(), QualityGateInput{
		UserMessage:   "rewrite my resume bullet",
		SourceContext: "Backend work in Go.",
		Intent:        "resume_rewrite",
	}, fakeQualityDraftGenerator{
		reply: "Built backend services in Go to support reliable job matching workflows for users.",
		meta:  &ToolCallMetadata{},
	}, QualityGateCallbacks{})

	require.NoError(t, err)
	require.Equal(t, "Built backend services in Go to support reliable job matching workflows for users.", result.FinalAnswer)
	require.Equal(t, "evaluator_failed_returned_draft", result.FallbackReason)
}
