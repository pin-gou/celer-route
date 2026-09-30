package tables

import (
	"testing"
	"time"
)

func ptrFloat64(v float64) *float64 { return &v }

// TestTableStandardPriceBeforeSaveDefaults asserts the BeforeSave hook fills
// missing currency / fx_rate defaults and refuses non-USD currencies under
// D11-A.
func TestTableStandardPriceBeforeSaveDefaults(t *testing.T) {
	t.Run("USD default + fx 1.0", func(t *testing.T) {
		s := &TableStandardPrice{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: 2.5, OutputCostPerMillion: 10}
		if err := s.BeforeSave(nil); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if s.Currency != "USD" {
			t.Errorf("currency = %q, want USD", s.Currency)
		}
		if s.FXRate != 1.0 {
			t.Errorf("fx_rate = %v, want 1.0", s.FXRate)
		}
	})
	t.Run("non-USD currency rejected", func(t *testing.T) {
		s := &TableStandardPrice{Provider: "openai", Model: "gpt-4o", Currency: "EUR", InputCostPerMillion: 1, OutputCostPerMillion: 1}
		if err := s.BeforeSave(nil); err == nil {
			t.Fatalf("expected non-USD currency to be rejected")
		}
	})
	t.Run("negative price rejected", func(t *testing.T) {
		s := &TableStandardPrice{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: -1, OutputCostPerMillion: 1}
		if err := s.BeforeSave(nil); err == nil {
			t.Fatalf("expected negative price to be rejected")
		}
	})
	t.Run("missing provider rejected", func(t *testing.T) {
		s := &TableStandardPrice{Model: "gpt-4o", InputCostPerMillion: 1, OutputCostPerMillion: 1}
		if err := s.BeforeSave(nil); err == nil {
			t.Fatalf("expected missing provider to be rejected")
		}
	})
	t.Run("missing model rejected", func(t *testing.T) {
		s := &TableStandardPrice{Provider: "openai", InputCostPerMillion: 1, OutputCostPerMillion: 1}
		if err := s.BeforeSave(nil); err == nil {
			t.Fatalf("expected missing model to be rejected")
		}
	})
}

// TestTableStandardPriceTokenCost verifies per-token conversion math and the
// cache-aware fallback when cache_read_cost_per_million is unset.
func TestTableStandardPriceTokenCost(t *testing.T) {
	s := &TableStandardPrice{
		InputCostPerMillion:     2.50,
		OutputCostPerMillion:    10.00,
		CacheReadCostPerMillion: ptrFloat64(0.25),
	}
	if got := s.TokenCost(false); got != 2.50/1e6 {
		t.Errorf("input per-token = %v, want %v", got, 2.50/1e6)
	}
	if got := s.TokenCost(true); got != 10.00/1e6 {
		t.Errorf("output per-token = %v, want %v", got, 10.00/1e6)
	}
	if got := s.CacheReadTokenCost(); got != 0.25/1e6 {
		t.Errorf("cache-read per-token = %v, want %v", got, 0.25/1e6)
	}

	// No cache_read cost → falls back to input rate.
	s.CacheReadCostPerMillion = nil
	if got := s.CacheReadTokenCost(); got != 2.50/1e6 {
		t.Errorf("cache-read fallback = %v, want input rate %v", got, 2.50/1e6)
	}
}

// TestTableTeamPricingProfileBeforeSave asserts mode + margin validation.
func TestTableTeamPricingProfileBeforeSave(t *testing.T) {
	t.Run("defaults filled", func(t *testing.T) {
		p := &TableTeamPricingProfile{TeamID: "t1", MarginMultiplier: 1.5}
		if err := p.BeforeSave(nil); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if p.Mode != TeamPricingModeStandard {
			t.Errorf("mode = %q, want standard", p.Mode)
		}
	})
	t.Run("actual mode accepted", func(t *testing.T) {
		p := &TableTeamPricingProfile{TeamID: "t1", Mode: TeamPricingModeActual, MarginMultiplier: 1.0}
		if err := p.BeforeSave(nil); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	t.Run("invalid mode rejected", func(t *testing.T) {
		p := &TableTeamPricingProfile{TeamID: "t1", Mode: "made_up", MarginMultiplier: 1.0}
		if err := p.BeforeSave(nil); err == nil {
			t.Fatalf("expected invalid mode to be rejected")
		}
	})
	t.Run("margin below 1 rejected", func(t *testing.T) {
		p := &TableTeamPricingProfile{TeamID: "t1", Mode: TeamPricingModeStandard, MarginMultiplier: 0.9}
		if err := p.BeforeSave(nil); err == nil {
			t.Fatalf("expected sub-1 margin to be rejected")
		}
	})
	t.Run("missing team_id rejected", func(t *testing.T) {
		p := &TableTeamPricingProfile{MarginMultiplier: 1.0}
		if err := p.BeforeSave(nil); err == nil {
			t.Fatalf("expected missing team_id to be rejected")
		}
	})
}

// TestTableTeamPricingProfileUsesActual exercises the UsesActual accessor.
func TestTableTeamPricingProfileUsesActual(t *testing.T) {
	var nilP *TableTeamPricingProfile
	if nilP.UsesActual() {
		t.Fatalf("nil profile should report UsesActual=false")
	}
	standard := &TableTeamPricingProfile{TeamID: "t1", Mode: TeamPricingModeStandard}
	if standard.UsesActual() {
		t.Fatalf("standard mode should report UsesActual=false")
	}
	actual := &TableTeamPricingProfile{TeamID: "t1", Mode: TeamPricingModeActual}
	if !actual.UsesActual() {
		t.Fatalf("actual mode should report UsesActual=true")
	}
}

// TestTableStandardPriceEffectiveFromDefaulted covers the "EffectiveFrom
// defaults to now" branch of BeforeSave.
func TestTableStandardPriceEffectiveFromDefaulted(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	s := &TableStandardPrice{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: 1, OutputCostPerMillion: 1}
	if err := s.BeforeSave(nil); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.EffectiveFrom.Before(before) {
		t.Fatalf("effective_from = %v, want ≥ %v", s.EffectiveFrom, before)
	}
}
