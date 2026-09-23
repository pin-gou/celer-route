package tables

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Standard pricing mode labels mirror the team_pricing_profiles.mode column.
// "standard" is the default — every team bills off standard_prices; "actual"
// is the rare self-supplied-account exception where the team carries the
// provider's bill directly.
const (
	TeamPricingModeStandard = "standard"
	TeamPricingModeActual   = "actual"
)

// TableStandardPrice is the versioned "price book" the gateway uses for
// member/team cost allocation. One row per (provider, model, effective_from);
// rate changes never overwrite history — handlers add a new effective row and
// reports pick the row whose effective_from is the latest one ≤ request time.
//
// Currency is fixed to USD (decision D11): the cost-side writes USD already,
// so the price book stays single-currency and the team ledger has no fx noise.
// fx_rate is retained only as historical-compat metadata; writes to it are a
// no-op under D11-A but the column exists so future migrations do not have to
// rebuild it from scratch.
type TableStandardPrice struct {
	ID                       string    `gorm:"primaryKey;type:varchar(255)" json:"id"`
	Provider                 string    `gorm:"type:varchar(50);index:idx_std_price_provider_model;not null" json:"provider"`
	Model                    string    `gorm:"type:varchar(255);index:idx_std_price_provider_model;not null" json:"model"`
	Currency                 string    `gorm:"type:varchar(10);default:'USD';not null" json:"currency"`
	InputCostPerMillion      float64   `gorm:"type:decimal(20,10);not null;default:0" json:"input_cost_per_million"`
	OutputCostPerMillion     float64   `gorm:"type:decimal(20,10);not null;default:0" json:"output_cost_per_million"`
	CacheReadCostPerMillion  *float64  `gorm:"type:decimal(20,10);null" json:"cache_read_cost_per_million,omitempty"`
	CostPerRequest           *float64  `gorm:"type:decimal(20,10);null" json:"cost_per_request,omitempty"`
	FXRate                   float64   `gorm:"type:decimal(20,10);default:1.0;not null" json:"fx_rate"`
	EffectiveFrom            time.Time `gorm:"index;not null" json:"effective_from"`
	CreatedAt                time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt                time.Time `gorm:"index;not null" json:"updated_at"`
}

// TableName sets the table name.
func (TableStandardPrice) TableName() string { return "standard_prices" }

// BeforeSave normalizes string fields and enforces the non-negative invariant
// on every priced component. fx_rate default 1.0 marks rows that never went
// through a conversion (the common case under D11-A).
func (s *TableStandardPrice) BeforeSave(tx *gorm.DB) error {
	if s.Provider != "" {
		s.Provider = strings.ToLower(strings.TrimSpace(s.Provider))
	}
	if s.Provider == "" {
		return errEmptyStandardPriceProvider
	}
	if s.Model != "" {
		s.Model = strings.TrimSpace(s.Model)
	}
	if s.Model == "" {
		return errEmptyStandardPriceModel
	}
	if s.Currency == "" {
		s.Currency = "USD"
	}
	if strings.ToUpper(s.Currency) != "USD" {
		// D11-A: standard_prices are USD only. Refuse anything else so the
		// team ledger stays single-currency.
		return errNonUSDStandardPrice
	}
	s.Currency = "USD"
	if s.FXRate <= 0 {
		s.FXRate = 1.0
	}
	if s.InputCostPerMillion < 0 || s.OutputCostPerMillion < 0 {
		return errNegativeStandardPrice
	}
	if s.CacheReadCostPerMillion != nil && *s.CacheReadCostPerMillion < 0 {
		return errNegativeStandardPrice
	}
	if s.CostPerRequest != nil && *s.CostPerRequest < 0 {
		return errNegativeStandardPrice
	}
	if s.EffectiveFrom.IsZero() {
		s.EffectiveFrom = time.Now().UTC()
	}
	return nil
}

// TokenCost returns the per-token USD rate for input vs output. Cache read is
// not handled here — the caller folds it in via the dedicated helper below so
// the standard "no cache awareness" path stays allocation-free.
func (s *TableStandardPrice) TokenCost(isOutput bool) float64 {
	if isOutput {
		return s.OutputCostPerMillion / 1_000_000.0
	}
	return s.InputCostPerMillion / 1_000_000.0
}

// CacheReadTokenCost returns the per-token USD rate for cache_read tokens, or
// the regular input rate when the row opts out of cache-aware pricing (the
// simplified default recommended in data-model §1). A nil row returns 0 so
// callers can detect missing entries.
func (s *TableStandardPrice) CacheReadTokenCost() float64 {
	if s.CacheReadCostPerMillion == nil {
		return s.InputCostPerMillion / 1_000_000.0
	}
	return *s.CacheReadCostPerMillion / 1_000_000.0
}
