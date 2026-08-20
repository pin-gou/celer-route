package zhipu_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/pg-gateway/core/internal/llmtests"
	"github.com/pin-gou/pg-gateway/core/schemas"
)

func TestZhipu(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("ZHIPU_API_KEY")) == "" {
		t.Skip("Skipping Zhipu tests because ZHIPU_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.Zhipu,
		ChatModel: "glm-4-flash",
		Scenarios: llmtests.TestScenarios{
			SimpleChat:            true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ListModels:            true,
		},
	}

	t.Run("ZhipuTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
