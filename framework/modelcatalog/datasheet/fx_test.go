package datasheet

import (
	"testing"

	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
)

// TestConvertEntryToTablePricing_AppliesFX confirms D11-A's USD conversion
// hook runs on every rate. Today the upstream datasheet ships USD, so the
// conversion is a no-op, but the hook must still execute so a future
// non-USD provider only has to populate FXRate to opt in.
func TestConvertEntryToTablePricing_AppliesFX(t *testing.T) {
	t.Run("zero fx is no-op (USD passthrough)", func(t *testing.T) {
		price := 0.001
		entry := Entry{
			Provider: "openai",
			Mode:     "chat",
			Options: Options{
				InputCostPerToken: &price,
			},
		}
		got := convertEntryToTablePricing("openai/gpt-4o", entry)
		if got.InputCostPerToken == nil || *got.InputCostPerToken != 0.001 {
			t.Errorf("input rate = %v, want 0.001 (passthrough)", *got.InputCostPerToken)
		}
	})
	t.Run("non-zero fx multiplies the rate", func(t *testing.T) {
		price := 1.0
		entry := Entry{
			Provider: "future",
			Mode:     "chat",
			FXRate:   0.5,
			Options: Options{
				InputCostPerToken: &price,
			},
		}
		got := convertEntryToTablePricing("future/x", entry)
		if got.InputCostPerToken == nil || *got.InputCostPerToken != 0.5 {
			t.Errorf("input rate = %v, want 0.5 (fxRate applied)", *got.InputCostPerToken)
		}
	})
	t.Run("nil rate stays nil after fx", func(t *testing.T) {
		entry := Entry{
			Provider: "openai",
			Mode:     "chat",
			FXRate:   1.2,
		}
		got := convertEntryToTablePricing("openai/y", entry)
		if got.InputCostPerToken != nil {
			t.Errorf("nil input rate should stay nil, got %v", *got.InputCostPerToken)
		}
		if got.CacheReadInputTokenCost != nil {
			t.Errorf("nil cache rate should stay nil, got %v", *got.CacheReadInputTokenCost)
		}
	})
	t.Run("table name matches governance_model_pricing", func(t *testing.T) {
		var p configstoreTables.TableModelPricing
		if name := p.TableName(); name != "governance_model_pricing" {
			t.Errorf("TableName = %q, want governance_model_pricing", name)
		}
	})
}
