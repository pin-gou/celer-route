package tables

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Reconciliation source / status constants. Source values are the "ground
// truth" channels a calibration batch can be sourced from; status drives the
// gateway-delta view in the admin UI (data-model §6).
const (
	ReconciliationSourceUsageAPI    = "usage_api"
	ReconciliationSourceInvoice     = "invoice_upload"
	ReconciliationSourceManual      = "manual"
	ReconciliationStatusPending     = "pending"
	ReconciliationStatusMatched     = "matched"
	ReconciliationStatusApplied     = "applied"
	ReconciliationStatusError       = "error"
	ReconciliationStatusUnsupported = "unsupported"

	// First-wave providers whose usage API we pull from in Phase 5. Anything
	// outside this list must go through invoice_upload (Azure / Vertex /
	// Bedrock / etc.) per data-model §6.1.
	ReconciliationProviderOpenAI    = "openai"
	ReconciliationProviderAnthropic = "anthropic"
	ReconciliationProviderDeepSeek  = "deepseek"
)

// IsReconciliationUsageAPISupported reports whether the given provider is in
// the first-wave usage-API list. Handlers return "please upload invoice" when
// this is false (data-model §6.1).
func IsReconciliationUsageAPISupported(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ReconciliationProviderOpenAI, ReconciliationProviderAnthropic, ReconciliationProviderDeepSeek:
		return true
	}
	return false
}

// TableBillingReconciliation is one calibration batch. The row pairs the
// gateway's Σactual (cost computed from datasheet prices × log tokens) with
// the provider's Σactual (from usage API or invoice). Delta is the per-batch
// correction factor; downstream datasheet updates consume it.
//
// Notes are optional free-form text surfaced in the UI — they capture the
// "why" behind manual adjustments (e.g. "credit redemption skew") so an
// auditor can trace unusual numbers back to context.
type TableBillingReconciliation struct {
	ID           string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Provider     string    `gorm:"type:varchar(50);index:idx_recon_provider_period;not null" json:"provider"`
	PeriodStart  time.Time `gorm:"index:idx_recon_provider_period;not null" json:"period_start"`
	PeriodEnd    time.Time `gorm:"index:idx_recon_provider_period;not null" json:"period_end"`
	Source       string    `gorm:"type:varchar(20);not null" json:"source"`
	GatewayCost  float64   `gorm:"type:decimal(20,10);not null;default:0" json:"gateway_cost"`
	ProviderCost float64   `gorm:"type:decimal(20,10);not null;default:0" json:"provider_cost"`
	Delta        float64   `gorm:"type:decimal(20,10);not null;default:0" json:"delta"`
	Status       string    `gorm:"type:varchar(20);index;not null;default:'pending'" json:"status"`
	Notes        *string   `gorm:"type:text;null" json:"notes,omitempty"`
	CreatedAt    time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"index;not null" json:"updated_at"`
}

// TableName returns the table name used by auto-migration.
func (TableBillingReconciliation) TableName() string { return "billing_reconciliations" }

// BeforeSave normalizes the row before insert / update. Lower-casing the
// provider keeps the (provider, period_start, period_end) unique semantics
// working even if the admin types "OpenAI". Source / status are validated
// against the closed enums; unknown values would render silently in the UI.
func (r *TableBillingReconciliation) BeforeSave(tx *gorm.DB) error {
	if r.Provider != "" {
		r.Provider = strings.ToLower(strings.TrimSpace(r.Provider))
	}
	if r.Provider == "" {
		return errEmptyReconciliationProvider
	}
	switch r.Source {
	case ReconciliationSourceUsageAPI, ReconciliationSourceInvoice, ReconciliationSourceManual:
	default:
		return errInvalidReconciliationSource
	}
	switch r.Status {
	case "", ReconciliationStatusPending:
		r.Status = ReconciliationStatusPending
	case ReconciliationStatusMatched, ReconciliationStatusApplied, ReconciliationStatusError, ReconciliationStatusUnsupported:
	default:
		return errInvalidReconciliationStatus
	}
	if r.PeriodStart.IsZero() || r.PeriodEnd.IsZero() {
		return errEmptyReconciliationPeriod
	}
	if !r.PeriodStart.Before(r.PeriodEnd) {
		return errInvalidReconciliationPeriod
	}
	return nil
}

// TableBillingReconItem is one (provider, model) breakdown inside a
// reconciliation batch. Splitting token_delta_pct and price_delta_pct lets
// the operator tell "tokenizer mismatch" from "datasheet lag" without
// mixing the two error sources (data-model §6 table).
type TableBillingReconItem struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ReconciliationID string    `gorm:"type:varchar(64);index:idx_recon_item_recon_model;not null" json:"reconciliation_id"`
	Model            string    `gorm:"type:varchar(255);index:idx_recon_item_recon_model;not null" json:"model"`
	GatewayRequests  int64     `gorm:"type:bigint;not null;default:0" json:"gateway_requests"`
	GatewayTokens    int64     `gorm:"type:bigint;not null;default:0" json:"gateway_tokens"`
	GatewayCost      float64   `gorm:"type:decimal(20,10);not null;default:0" json:"gateway_cost"`
	ProviderRequests int64     `gorm:"type:bigint;not null;default:0" json:"provider_requests"`
	ProviderTokens   int64     `gorm:"type:bigint;not null;default:0" json:"provider_tokens"`
	ProviderCost     float64   `gorm:"type:decimal(20,10);not null;default:0" json:"provider_cost"`
	TokenDeltaPct    *float64  `gorm:"type:decimal(10,4);null" json:"token_delta_pct,omitempty"`
	PriceDeltaPct    *float64  `gorm:"type:decimal(10,4);null" json:"price_delta_pct,omitempty"`
	CreatedAt        time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"index;not null" json:"updated_at"`
}

// TableName returns the table name used by auto-migration.
func (TableBillingReconItem) TableName() string { return "billing_recon_items" }

// BeforeSave enforces the bare-minimum invariants on a recon item row. The
// ReconciliationID is checked because items orphaned from their parent
// batch would silently disappear from the admin UI.
func (i *TableBillingReconItem) BeforeSave(tx *gorm.DB) error {
	if strings.TrimSpace(i.ReconciliationID) == "" {
		return errEmptyReconciliationItemParent
	}
	if strings.TrimSpace(i.Model) == "" {
		return errEmptyReconciliationItemModel
	}
	return nil
}
