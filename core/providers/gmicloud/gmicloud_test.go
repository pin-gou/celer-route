package gmicloud_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"
	"github.com/pin-gou/celer-route/core/schemas"
)

func TestGMICloud(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("GMI_API_KEY")) == "" {
		t.Skip("Skipping GMI Cloud tests because GMI_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.GMICloud,
		ChatModel: "deepseek-ai/DeepSeek-V4-Flash-0731",
		Fallbacks: []schemas.Fallback{
			{Provider: schemas.GMICloud, Model: "deepseek-ai/DeepSeek-V4-Flash-0731"},
		},
		Scenarios: llmtests.TestScenarios{
			TextCompletion:        false,
			TextCompletionStream:  false,
			SimpleChat:            true,
			CompletionStream:      true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ToolCallsStreaming:    true,
			MultipleToolCalls:     true,
			End2EndToolCalling:    true,
			ImageURL:              false,
			ImageBase64:           false,
			MultipleImages:        false,
			FileBase64:            false,
			FileURL:               false,
			CompleteEnd2End:       true,
			Embedding:             false,
			ListModels:            true,
			Reasoning:             false,
			Transcription:         false,
			SpeechSynthesis:       false,
		},
	}
	t.Run("GMICloudTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
