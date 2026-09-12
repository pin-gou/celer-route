package azureai_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestAzureAI(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("AZURE_AI_API_KEY")) == "" {
		t.Skip("Skipping AzureAI tests because AZURE_AI_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.AzureAI,
		ChatModel: "gpt-4o-mini",
		Fallbacks: []schemas.Fallback{
			{Provider: schemas.AzureAI, Model: "gpt-4o-mini"},
		},
		TextModel:            "gpt-4o-mini",
		EmbeddingModel:       "text-embedding-3-small",
		ImageGenerationModel: "",
		ReasoningModel:       "gpt-4o-mini",
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
			ImageURL:                   true,
			ImageBase64:                false,
			MultipleImages:             false,
			ImageGeneration:            false,
			CompleteEnd2End:            true,
			Embedding:                  true,
			ListModels:                 true,
			Reasoning:                  false,
		},
	}

	t.Run("AzureAITests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
