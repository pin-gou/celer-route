package datasheet

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
)

func ptrFloat64Cost(v float64) *float64 { return &v }

func TestStandardCost_TextRequest_Basic(t *testing.T) {
	price := &tables.TableStandardPrice{
		InputCostPerMillion:  2.50,
		OutputCostPerMillion: 10.00,
	}
	usage := &schemas.BifrostLLMUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}
	// 1000 * 2.5e-6 + 500 * 1e-5 = 0.0025 + 0.005 = 0.0075
	want := 0.0075
	got := StandardCost(price, usage)
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("StandardCost = %v, want %v", got, want)
	}
}

func TestStandardCost_NilInputsReturnZero(t *testing.T) {
	if got := StandardCost(nil, nil); got != 0 {
		t.Errorf("nil price / nil usage = %v, want 0", got)
	}
	price := &tables.TableStandardPrice{InputCostPerMillion: 1, OutputCostPerMillion: 1}
	if got := StandardCost(price, nil); got != 0 {
		t.Errorf("nil usage = %v, want 0", got)
	}
	if got := StandardCost(nil, &schemas.BifrostLLMUsage{}); got != 0 {
		t.Errorf("nil price = %v, want 0", got)
	}
}

func TestStandardCost_CacheRead(t *testing.T) {
	price := &tables.TableStandardPrice{
		InputCostPerMillion:     2.50,
		OutputCostPerMillion:    10.00,
		CacheReadCostPerMillion: ptrFloat64Cost(0.25),
	}
	usage := &schemas.BifrostLLMUsage{
		PromptTokens:     1000,
		CompletionTokens: 0,
		PromptTokensDetails: &schemas.ChatPromptTokensDetails{
			CachedReadTokens: 800,
		},
	}
	// 200 * 2.5e-6 + 800 * 0.25e-6 + 0 = 0.0005 + 0.0002 = 0.0007
	want := 0.0007
	got := StandardCost(price, usage)
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("StandardCost cache-read = %v, want %v", got, want)
	}
}

func TestStandardCost_CostPerRequest(t *testing.T) {
	price := &tables.TableStandardPrice{
		InputCostPerMillion:  0,
		OutputCostPerMillion: 0,
		CostPerRequest:       ptrFloat64Cost(0.05),
	}
	usage := &schemas.BifrostLLMUsage{}
	got := StandardCost(price, usage)
	if got != 0.05 {
		t.Errorf("StandardCost per-request = %v, want 0.05", got)
	}
}

func TestStandardCostWithFallback(t *testing.T) {
	usage := &schemas.BifrostLLMUsage{PromptTokens: 1000, CompletionTokens: 500}
	if got := StandardCostWithFallback(nil, usage, 0.99); got != 0.99 {
		t.Errorf("fallback path = %v, want 0.99", got)
	}
	price := &tables.TableStandardPrice{InputCostPerMillion: 1, OutputCostPerMillion: 1}
	if got := StandardCostWithFallback(price, usage, 99); got <= 0 {
		t.Errorf("price path should not fall back, got %v", got)
	}
}

func TestApplyFXUSD(t *testing.T) {
	if got := ApplyFXUSD(0, 1.5); got != 0 {
		t.Errorf("zero rate returns 0, got %v", got)
	}
	// No fx configured → assume already-USD, passthrough.
	if got := ApplyFXUSD(0.001, 0); got != 0.001 {
		t.Errorf("no-fx passthrough = %v, want 0.001", got)
	}
	// Negative fxRate treated as missing → passthrough.
	if got := ApplyFXUSD(0.001, -1); got != 0.001 {
		t.Errorf("negative-fx passthrough = %v, want 0.001", got)
	}
	// Real conversion.
	if got := ApplyFXUSD(0.5, 0.8); got != 0.4 {
		t.Errorf("ApplyFXUSD(0.5, 0.8) = %v, want 0.4", got)
	}
}

func TestClassifyCostAccuracy(t *testing.T) {
	cases := []struct {
		name             string
		hasProviderUsage bool
		hasEstimated     bool
		want             string
	}{
		{"provider", true, false, CostAccuracyProviderReported},
		{"estimated", false, true, CostAccuracyGatewayEstimated},
		{"unknown", false, false, CostAccuracyUnknown},
		{"both", true, true, CostAccuracyProviderReported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyCostAccuracy(c.hasProviderUsage, c.hasEstimated); got != c.want {
				t.Errorf("ClassifyCostAccuracy = %q, want %q", got, c.want)
			}
		})
	}
}
