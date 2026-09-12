package qiniu_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"

	"github.com/pin-gou/celer-route/core/schemas"
)

func TestQiniuProvider(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("QINIU_API_KEY")) == "" {
		t.Skip("Skipping Qiniu tests because QINIU_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:       schemas.Qiniu,
		ChatModel:      "deepseek-v3.1",
		TextModel:      "", // Qiniu doesn't support text completion
		EmbeddingModel: "", // Qiniu doesn't support embedding
		Scenarios: llmtests.TestScenarios{
			TextCompletion:        false, // Not supported
			SimpleChat:            true,
			CompletionStream:      true,
			MultiTurnConversation: true,
			ToolCalls:             false, // Gateway model variance; not pinned
			ToolCallsStreaming:    false, // Gateway model variance; not pinned
			MultipleToolCalls:     false, // Not supported yet
			End2EndToolCalling:    false, // Gateway model variance; not pinned
			AutomaticFunctionCall: false, // Gateway model variance; not pinned
			ImageURL:              false, // Not supported yet
			ImageBase64:           false, // Not supported yet
			MultipleImages:        false, // Not supported yet
			CompleteEnd2End:       true,
			Embedding:             false, // Not supported yet
			ListModels:            true,
		},
	}

	t.Run("QiniuTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}