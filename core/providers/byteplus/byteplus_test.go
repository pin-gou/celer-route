package byteplus_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestBytePlusProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("BYTEPLUS_API_KEY")) == "" {
		t.Skip("Skipping BytePlus tests because BYTEPLUS_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:       schemas.BytePlus,
		ChatModel:      "seed-1.6",
		TextModel:      "", // BytePlus doesn't support text completion
		EmbeddingModel: "", // BytePlus doesn't support embedding
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

	t.Run("BytePlusTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}