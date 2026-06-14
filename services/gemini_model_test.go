package services

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultGeminiGenerationModelPinned(t *testing.T) {
	if DefaultGeminiGenerationModel != "gemini-2.5-flash" {
		t.Fatalf("DefaultGeminiGenerationModel = %q, want gemini-2.5-flash", DefaultGeminiGenerationModel)
	}
}

func TestGeminiAnalysisModelName(t *testing.T) {
	t.Setenv("GEMINI_ANALYSIS_MODEL", "")
	if got := GeminiAnalysisModelName(); got != DefaultGeminiAnalysisModel {
		t.Fatalf("GeminiAnalysisModelName() = %q, want %q", got, DefaultGeminiAnalysisModel)
	}

	t.Setenv("GEMINI_ANALYSIS_MODEL", "gemini-custom-analysis")
	if got := GeminiAnalysisModelName(); got != "gemini-custom-analysis" {
		t.Fatalf("GeminiAnalysisModelName() = %q, want configured model", got)
	}
}

func TestDirectGoogleAIClientsPinDefaultModel(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current test file")
	}
	serviceDir := filepath.Dir(currentFile)

	files, err := filepath.Glob(filepath.Join(serviceDir, "*.go"))
	if err != nil {
		t.Fatalf("glob service files: %v", err)
	}

	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasSuffix(base, "_test.go") || base == "gemini.go" {
			continue
		}

		srcBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", base, err)
		}
		src := string(srcBytes)
		if !strings.Contains(src, "googleai.New(") {
			continue
		}
		if !strings.Contains(src, "googleai.WithDefaultModel(DefaultGeminiGenerationModel)") {
			t.Fatalf("%s calls googleai.New without pinning DefaultGeminiGenerationModel", base)
		}
	}
}
