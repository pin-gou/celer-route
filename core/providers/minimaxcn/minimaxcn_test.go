package minimaxcn_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestMinimaxCN(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("MINIMAX_CN_API_KEY")) == "" {
		t.Skip("Skipping MinimaxCN tests because MINIMAX_CN_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.MinimaxCN,
		ChatModel: "MiniMax-Text-01",
		Fallbacks: []schemas.Fallback{
			{Provider: schemas.MinimaxCN, Model: "abab6.5s-chat"},
		},
		TextModel:            "MiniMax-Text-01",
		EmbeddingModel:       "",
		ImageGenerationModel: "",
		ReasoningModel:       "MiniMax-M1",
		Scenarios: llmtests.TestScenarios{
			TextCompletion:             true,
			TextCompletionStream:       true,
			SimpleChat:                 true,
			CompletionStream:           true,
			MultiTurnConversation:      true,
			ToolCalls:                  true,
			ToolCallsStreaming:         true,
			MultipleToolCalls:          true,
			MultipleToolCallsStreaming: true,
			End2EndToolCalling:         true,
			AutomaticFunctionCall:      true,
			ImageURL:                   false,
			ImageBase64:                false,
			MultipleImages:             false,
			ImageGeneration:            false,
			CompleteEnd2End:            true,
			Embedding:                  false,
			ListModels:                 true,
			Reasoning:                  true,
		},
	}

	t.Run("MinimaxCNTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
