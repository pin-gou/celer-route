package sambanova_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestSambanova(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("SAMBANOVA_API_KEY")) == "" {
		t.Skip("Skipping Sambanova tests because SAMBANOVA_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.Sambanova,
		ChatModel: "Meta-Llama-3.3-70B-Instruct",
		Fallbacks: []schemas.Fallback{
			{Provider: schemas.Sambanova, Model: "Meta-Llama-3.1-8B-Instruct"},
		},
		TextModel:            "Meta-Llama-3.3-70B-Instruct",
		EmbeddingModel:       "",
		ImageGenerationModel: "",
		ReasoningModel:       "DeepSeek-R1",
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

	t.Run("SambanovaTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
