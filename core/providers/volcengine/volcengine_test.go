package volcengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/pg-gateway/core/internal/llmtests"
	"github.com/pin-gou/pg-gateway/core/schemas"
)

func TestVolcengine(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("VOLCENGINE_API_KEY")) == "" {
		t.Skip("Skipping Volcengine tests because VOLCENGINE_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.Volcengine,
		ChatModel: "doubao-pro-32k",
		Scenarios: llmtests.TestScenarios{
			SimpleChat:            true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ListModels:            true,
		},
	}

	t.Run("VolcengineTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
