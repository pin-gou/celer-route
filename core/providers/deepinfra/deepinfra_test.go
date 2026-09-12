package deepinfra_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestDeepinfra(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("DEEPINFRA_API_KEY")) == "" {
		t.Skip("Skipping Deepinfra tests because DEEPINFRA_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.DeepInfra,
		ChatModel: "meta-llama/Llama-3.3-70B-Instruct",
		Fallbacks: []schemas.Fallback{
			{Provider: schemas.DeepInfra, Model: "meta-llama/Llama-3.1-8B-Instruct"},
		},
		TextModel:            "meta-llama/Llama-3.3-70B-Instruct",
		EmbeddingModel:       "BAAI/bge-m3",
		ImageGenerationModel: "black-forest-labs/FLUX.1-schnell",
		ReasoningModel:       "deepseek-ai/DeepSeek-R1",
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
			ImageGeneration:            true,
			CompleteEnd2End:            true,
			Embedding:                  true,
			ListModels:                 true,
			Reasoning:                  true,
		},
	}

	t.Run("DeepinfraTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
