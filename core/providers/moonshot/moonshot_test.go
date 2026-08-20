package moonshot_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/pg-gateway/core/internal/llmtests"
	"github.com/pin-gou/pg-gateway/core/schemas"
)

func TestMoonshot(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("MOONSHOT_API_KEY")) == "" {
		t.Skip("Skipping Moonshot tests because MOONSHOT_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.Moonshot,
		ChatModel: "moonshot-v1-8k",
		Scenarios: llmtests.TestScenarios{
			SimpleChat:            true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ListModels:            true,
		},
	}

	t.Run("MoonshotTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
