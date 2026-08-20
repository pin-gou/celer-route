package siliconflow_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/pg-gateway/core/internal/llmtests"
	"github.com/pin-gou/pg-gateway/core/schemas"
)

func TestSiliconflow(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("SILICONFLOW_API_KEY")) == "" {
		t.Skip("Skipping Siliconflow tests because SILICONFLOW_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.Siliconflow,
		ChatModel: "deepseek-ai/DeepSeek-V3",
		Scenarios: llmtests.TestScenarios{
			SimpleChat:            true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ListModels:            true,
		},
	}

	t.Run("SiliconflowTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}
