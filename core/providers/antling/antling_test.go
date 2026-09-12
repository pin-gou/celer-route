package antling_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestAntLingProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("ANTLING_API_KEY")) == "" {
		t.Skip("Skipping Ant Ling tests because ANTLING_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:       schemas.AntLing,
		ChatModel:      "ling-plus",
		TextModel:      "", // Ant Ling doesn't support text completion
		EmbeddingModel: "", // Ant Ling doesn't support embedding
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

	t.Run("AntLingTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
