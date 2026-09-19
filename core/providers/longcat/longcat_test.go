package longcat_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"
	"github.com/pin-gou/celer-route/core/schemas"
)

func TestLongCat(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("LONGCAT_API_KEY")) == "" {
		t.Skip("Skipping LongCat AI tests because LONGCAT_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:       schemas.LongCat,
		ChatModel:      "LongCat-2.0",
		TextModel:      "",
		EmbeddingModel: "",
		Scenarios: llmtests.TestScenarios{
			TextCompletion:        false,
			SimpleChat:            true,
			CompletionStream:      true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ToolCallsStreaming:    true,
			MultipleToolCalls:     false,
			End2EndToolCalling:    true,
			AutomaticFunctionCall: true,
			ImageURL:              false,
			ImageBase64:           false,
			MultipleImages:        false,
			CompleteEnd2End:       true,
			Embedding:             false,
			ListModels:            true,
		},
	}

	t.Run("LongCatTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
