package alibabatokenplan_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pin-gou/celer-route/core/internal/llmtests"
	"github.com/pin-gou/celer-route/core/schemas"
)

func TestAlibabaTokenplan(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("ALIBABA_TOKENPLAN_API_KEY")) == "" {
		t.Skip("Skipping Alibaba Token Plan tests because ALIBABA_TOKENPLAN_API_KEY is not set")
	}

	client, ctx, cancel, err := llmtests.SetupTest()
	if err != nil {
		t.Fatalf("Error initializing test setup: %v", err)
	}
	defer cancel()
	defer client.Shutdown()

	testConfig := llmtests.ComprehensiveTestConfig{
		Provider:  schemas.AlibabaTokenplan,
		ChatModel: "qwen3.8-max",
		Scenarios: llmtests.TestScenarios{
			SimpleChat:            true,
			MultiTurnConversation: true,
			ToolCalls:             true,
			ListModels:            true,
		},
	}

	t.Run("AlibabaTokenplanTests", func(t *testing.T) {
		llmtests.RunAllComprehensiveTests(t, client, ctx, testConfig)
	})
}