package ai360_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestAI360Provider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("AI360_API_KEY")) == "" {
		t.Skip("Skipping 360 AI tests because AI360_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:       schemas.ThreeSixtyAI,
		ChatModel:      "360GPT_S2_V9",
		TextModel:      "", // 360 AI doesn't support text completion
		EmbeddingModel: "", // 360 AI doesn't support embedding
		Scenarios: llmtests.TestScenarios{
			TextCompletion:        false, // Not supported
			SimpleChat:            true,
			CompletionStream:      true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ToolCallsStreaming:    true,
			MultipleToolCalls:     false, // Not supported yet
			End2EndToolCalling:    true,
			AutomaticFunctionCall: true,
			ImageURL:              false, // Not supported yet
			ImageBase64:           false, // Not supported yet
			MultipleImages:        false, // Not supported yet
			CompleteEnd2End:       true,
			Embedding:             false, // Not supported yet
			ListModels:            true,
		},
	}

	t.Run("AI360Tests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
