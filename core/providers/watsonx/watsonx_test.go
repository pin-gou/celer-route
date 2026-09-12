package watsonx_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestWatsonx(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("WATSONX_API_KEY")) == "" {
		t.Skip("Skipping Watsonx tests because WATSONX_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.Watsonx,
		ChatModel: "meta-llama/llama-3-3-70b-instruct",
		Fallbacks: []schemas.Fallback{
			{Provider: schemas.Watsonx, Model: "meta-llama/llama-3-1-8b-instruct"},
		},
		TextModel:            "meta-llama/llama-3-3-70b-instruct",
		EmbeddingModel:       "",
		ImageGenerationModel: "",
		ReasoningModel:       "deepseek-ai/deepseek-r1",
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

	t.Run("WatsonxTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
