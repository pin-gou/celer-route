package configstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"gorm.io/gorm"
)

// StandardPriceQueryParams narrows the list endpoint. Empty fields mean "no
// filter on this dimension"; provider/model/currency may each narrow
// independently so the admin UI can answer "all OpenAI rows" or "find the
// USD-denominated version" without round-tripping the table.
type StandardPriceQueryParams struct {
	Provider string
	Model    string
	Limit    int
	Offset   int
}

// ListStandardPrices returns the versioned price-book rows filtered by
// params, plus a total count for paginated UIs. The result is sorted newest
// first so the admin UI sees the active row at the top of each (provider,
// model) bucket.
func (s *RDBConfigStore) ListStandardPrices(ctx context.Context, params StandardPriceQueryParams) ([]tables.TableStandardPrice, int64, error) {
	query := s.DB().WithContext(ctx).Model(&tables.TableStandardPrice{})
	if params.Provider != "" {
		query = query.Where("provider = ?", strings.ToLower(strings.TrimSpace(params.Provider)))
	}
	if params.Model != "" {
		query = query.Where("model = ?", params.Model)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, s.parseGormError(err)
	}
	if params.Limit <= 0 {
		params.Limit = 100
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	var rows []tables.TableStandardPrice
	if err := query.Order("effective_from DESC, provider, model").Limit(params.Limit).Offset(params.Offset).Find(&rows).Error; err != nil {
		return nil, 0, s.parseGormError(err)
	}
	return rows, total, nil
}

// GetStandardPriceByID returns the row with the given id, or ErrNotFound when
// the row has been deleted or never existed. Update / preview endpoints both
// need this lookup.
func (s *RDBConfigStore) GetStandardPriceByID(ctx context.Context, id string) (*tables.TableStandardPrice, error) {
	var row tables.TableStandardPrice
	if err := s.DB().WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, s.parseGormError(err)
	}
	return &row, nil
}

// GetActiveStandardPrice returns the most-recently-effective row for
// (provider, model) whose effective_from ≤ at. The reports endpoints call
// this for every row in the window, so it is the hot path for the team
// ledger. A nil result means no price book entry yet — the handler then
// falls back to the actual cost so the ledger is never silently zero.
func (s *RDBConfigStore) GetActiveStandardPrice(ctx context.Context, provider, model string, at time.Time) (*tables.TableStandardPrice, error) {
	var row tables.TableStandardPrice
	err := s.DB().WithContext(ctx).
		Where("provider = ? AND model = ? AND effective_from <= ?", strings.ToLower(strings.TrimSpace(provider)), model, at.UTC()).
		Order("effective_from DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, s.parseGormError(err)
	}
	return &row, nil
}

// CreateStandardPrice mints a new versioned price-book row. The id is set
// server-side if missing so callers don't need to know about the uuid
// machinery; effective_from defaults to "now" via the BeforeSave hook.
func (s *RDBConfigStore) CreateStandardPrice(ctx context.Context, row *tables.TableStandardPrice) error {
	if row == nil {
		return errors.New("standard price is required")
	}
	if strings.TrimSpace(row.ID) == "" {
		row.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if row.EffectiveFrom.IsZero() {
		row.EffectiveFrom = now
	}
	row.CreatedAt = now
	row.UpdatedAt = now
	return s.DB().WithContext(ctx).Create(row).Error
}

// DeleteStandardPrice hard-removes the row. Soft deletes are not modeled:
// price-book entries are versioned (delete = "remove the historical row"),
// not lifecycle.
func (s *RDBConfigStore) DeleteStandardPrice(ctx context.Context, id string) error {
	res := s.DB().WithContext(ctx).Delete(&tables.TableStandardPrice{}, "id = ?", id)
	if res.Error != nil {
		return s.parseGormError(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkCreateStandardPrices inserts a batch of price-book rows in one go. Used
// by the sync-from-datasheet endpoint, which may emit dozens of rows per
// call. Effective_from and ID are filled in if missing. We chunk inserts at
// batchSize to stay under SQLite's 999-variable limit (each row has ~12
// bound parameters); Postgres has a similar limit at 65535 but chunking
// keeps memory bounded either way.
func (s *RDBConfigStore) BulkCreateStandardPrices(ctx context.Context, rows []tables.TableStandardPrice) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for i := range rows {
		if strings.TrimSpace(rows[i].ID) == "" {
			rows[i].ID = uuid.NewString()
		}
		if rows[i].EffectiveFrom.IsZero() {
			rows[i].EffectiveFrom = now
		}
		rows[i].CreatedAt = now
		rows[i].UpdatedAt = now
	}
	const batchSize = 80
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		if err := s.DB().WithContext(ctx).Create(rows[start:end]).Error; err != nil {
			return s.parseGormError(err)
		}
	}
	return nil
}

// ListTeamPricingProfiles returns every per-team override. The list is
// deliberately small (one row per team) so we don't paginate.
func (s *RDBConfigStore) ListTeamPricingProfiles(ctx context.Context) ([]tables.TableTeamPricingProfile, error) {
	var rows []tables.TableTeamPricingProfile
	if err := s.DB().WithContext(ctx).Order("team_id").Find(&rows).Error; err != nil {
		return nil, s.parseGormError(err)
	}
	return rows, nil
}

// GetTeamPricingProfile returns the row for a team, or nil when no override
// exists. The team ledger treats a missing profile as "use standard_prices",
// which is the default for the overwhelming majority of teams.
func (s *RDBConfigStore) GetTeamPricingProfile(ctx context.Context, teamID string) (*tables.TableTeamPricingProfile, error) {
	var row tables.TableTeamPricingProfile
	if err := s.DB().WithContext(ctx).First(&row, "team_id = ?", teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, s.parseGormError(err)
	}
	return &row, nil
}

// UpsertTeamPricingProfile inserts or replaces the row for a team. Teams
// only ever have one profile at a time; the uniqueIndex on team_id makes
// the insert-or-replace race-free under concurrent admin writes.
func (s *RDBConfigStore) UpsertTeamPricingProfile(ctx context.Context, row *tables.TableTeamPricingProfile) error {
	if row == nil {
		return errors.New("team pricing profile is required")
	}
	if strings.TrimSpace(row.ID) == "" {
		row.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	return s.DB().WithContext(ctx).Save(row).Error
}

// DeleteTeamPricingProfile removes the row for a team. Returning the row to
// "use standard_prices" semantics.
func (s *RDBConfigStore) DeleteTeamPricingProfile(ctx context.Context, teamID string) error {
	res := s.DB().WithContext(ctx).Delete(&tables.TableTeamPricingProfile{}, "team_id = ?", teamID)
	if res.Error != nil {
		return s.parseGormError(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReconciliationQueryParams narrows ListReconciliations. Provider / Status
// / Source can each be empty (== no filter); Limit/Offset drive pagination.
// PeriodStart / PeriodEnd, when both set, narrow to batches whose window
// intersects the requested range (handy for "show me Q3 calibrations").
type ReconciliationQueryParams struct {
	Provider    string
	Status      string
	Source      string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Limit       int
	Offset      int
}

// ListReconciliations returns calibration batches filtered by params. Sorted
// newest-first so the admin UI shows the most recent calibration at the top.
// A total is included for the pagination footer.
func (s *RDBConfigStore) ListReconciliations(ctx context.Context, params ReconciliationQueryParams) ([]tables.TableBillingReconciliation, int64, error) {
	query := s.DB().WithContext(ctx).Model(&tables.TableBillingReconciliation{})
	if v := strings.TrimSpace(params.Provider); v != "" {
		query = query.Where("provider = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(params.Status); v != "" {
		query = query.Where("status = ?", v)
	}
	if v := strings.TrimSpace(params.Source); v != "" {
		query = query.Where("source = ?", v)
	}
	if !params.PeriodStart.IsZero() && !params.PeriodEnd.IsZero() {
		query = query.Where("period_end >= ? AND period_start <= ?", params.PeriodStart, params.PeriodEnd)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, s.parseGormError(err)
	}
	if params.Limit > 0 {
		query = query.Limit(params.Limit)
	}
	if params.Offset > 0 {
		query = query.Offset(params.Offset)
	}
	var rows []tables.TableBillingReconciliation
	if err := query.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, 0, s.parseGormError(err)
	}
	return rows, total, nil
}

// GetReconciliationByID returns the parent batch row, or ErrNotFound. Items
// are loaded separately via ListReconciliationItems so the parent lookup
// stays allocation-free on list pages.
func (s *RDBConfigStore) GetReconciliationByID(ctx context.Context, id string) (*tables.TableBillingReconciliation, error) {
	var row tables.TableBillingReconciliation
	if err := s.DB().WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, s.parseGormError(err)
	}
	return &row, nil
}

// ListReconciliationItems returns every breakdown row for the given batch
// in stable (model asc) order. An empty slice + nil error signals the batch
// exists but produced no per-model breakdown (e.g. unsupported provider).
func (s *RDBConfigStore) ListReconciliationItems(ctx context.Context, reconciliationID string) ([]tables.TableBillingReconItem, error) {
	if strings.TrimSpace(reconciliationID) == "" {
		return nil, nil
	}
	var rows []tables.TableBillingReconItem
	if err := s.DB().WithContext(ctx).
		Where("reconciliation_id = ?", reconciliationID).
		Order("model ASC").
		Find(&rows).Error; err != nil {
		return nil, s.parseGormError(err)
	}
	return rows, nil
}

// CreateReconciliation atomically writes a parent batch and its item rows.
// The atomicity matters: the calibration job runs once per cycle and a
// half-written batch would surface in the UI as "matched but with no
// per-model numbers" — a confusing state worth a transaction for.
func (s *RDBConfigStore) CreateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation, items []tables.TableBillingReconItem) error {
	if row == nil {
		return errors.New("reconciliation row is required")
	}
	if strings.TrimSpace(row.ID) == "" {
		row.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	row.CreatedAt = now
	row.UpdatedAt = now
	if row.Status == "" {
		row.Status = tables.ReconciliationStatusPending
	}
	// Recon items have their own timestamps but they all share the batch's
	// wall-clock so a downstream query "items in the same second as the
	// batch header" returns the right set without joining on a window.
	for i := range items {
		if items[i].ReconciliationID == "" {
			items[i].ReconciliationID = row.ID
		}
		items[i].CreatedAt = now
		items[i].UpdatedAt = now
	}
	err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return s.parseGormError(err)
	}
	return nil
}

// UpdateReconciliation mutates an existing batch — only the operator-facing
// fields (status / notes / provider+gateway cost) are writable; id,
// period_start, provider are immutable after creation so audit trail stays
// stable. The update uses Exec + raw column writes (rather than GORM
// .Updates on a struct) so the BeforeSave hook is bypassed — otherwise the
// partial row passed in would fail the "provider required" check.
func (s *RDBConfigStore) UpdateReconciliation(ctx context.Context, row *tables.TableBillingReconciliation) error {
	if row == nil {
		return errors.New("reconciliation row is required")
	}
	if strings.TrimSpace(row.ID) == "" {
		return errors.New("reconciliation id is required")
	}
	row.UpdatedAt = time.Now().UTC()
	res := s.DB().WithContext(ctx).Exec(
		`UPDATE billing_reconciliations SET status = ?, notes = ?, gateway_cost = ?, provider_cost = ?, delta = ?, updated_at = ? WHERE id = ?`,
		row.Status, row.Notes, row.GatewayCost, row.ProviderCost, row.Delta, row.UpdatedAt, row.ID,
	)
	if res.Error != nil {
		return s.parseGormError(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
