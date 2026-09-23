package configstore

import (
	"context"
	"errors"
	"fmt"
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
//
// GORM's plain Save with a primary key set behaves as UPDATE-WHERE-id (it
// emits ON CONFLICT (id), not ON CONFLICT (team_id)), so a caller that builds
// a fresh struct per request — which the HTTP handler does on every PUT —
// gets a brand-new UUID, a genuine INSERT, and a UNIQUE constraint violation
// on team_id for every edit after the first. We do an explicit First-or-Create
// inside a transaction instead, matching UpsertTeamModelPolicy: lock the row
// for the duration of the upsert, reuse its ID and CreatedAt, then save.
// CreatedAt is preserved across updates so the profile can be audited by
// creation time.
func (s *RDBConfigStore) UpsertTeamPricingProfile(ctx context.Context, row *tables.TableTeamPricingProfile) error {
	if row == nil {
		return errors.New("team pricing profile is required")
	}
	if strings.TrimSpace(row.TeamID) == "" {
		return errors.New("team pricing profile: team_id is required")
	}
	now := time.Now().UTC()
	return s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing tables.TableTeamPricingProfile
		err := dbForUpdate(tx).
			Where("team_id = ?", row.TeamID).
			First(&existing).Error
		switch {
		case err == nil:
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
		case errors.Is(err, gorm.ErrRecordNotFound):
			if row.ID == "" {
				row.ID = uuid.NewString()
			}
			if row.CreatedAt.IsZero() {
				row.CreatedAt = now
			}
		default:
			return fmt.Errorf("failed to query existing team pricing profile: %w", err)
		}
		row.UpdatedAt = now
		return tx.Save(row).Error
	})
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

// ListTeamModelPolicies returns every ACL row for a team. The admin UI
// renders this as the "model policies" tab on the team detail page. Sorted
// by provider so the rendering order is stable across requests.
func (s *RDBConfigStore) ListTeamModelPolicies(ctx context.Context, teamID string) ([]tables.TableTeamModelPolicy, error) {
	var rows []tables.TableTeamModelPolicy
	if err := s.DB().WithContext(ctx).
		Where("team_id = ?", teamID).
		Order("provider").
		Find(&rows).Error; err != nil {
		return nil, s.parseGormError(err)
	}
	return rows, nil
}

// GetTeamModelPolicy returns the ACL row for a (team, provider) pair, or
// (nil, nil) when no row exists so the resolver can short-circuit on the
// "inherit global" case without an errors.Is check.
func (s *RDBConfigStore) GetTeamModelPolicy(ctx context.Context, teamID, provider string) (*tables.TableTeamModelPolicy, error) {
	var row tables.TableTeamModelPolicy
	if err := s.DB().WithContext(ctx).
		Where("team_id = ? AND provider = ?", teamID, provider).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, s.parseGormError(err)
	}
	return &row, nil
}

// UpsertTeamModelPolicy inserts or replaces the ACL row for a (team,
// provider) pair. The unique index on (team_id, provider) makes the write
// race-free under concurrent admin edits.
//
// GORM's plain Save with a primary key set behaves as UPDATE-WHERE-id, not
// as upsert, so a re-submit would trip the UNIQUE constraint on the second
// attempt. We do an explicit First-or-Create inside a transaction instead,
// matching the pattern used by UpsertMCPPerUserHeaderCredential: lock the
// row for the duration of the upsert, mutate, save. CreatedAt is preserved
// across updates so the table can be audited by upload time.
func (s *RDBConfigStore) UpsertTeamModelPolicy(ctx context.Context, policy *tables.TableTeamModelPolicy) error {
	if policy == nil {
		return errors.New("team model policy is required")
	}
	if strings.TrimSpace(policy.TeamID) == "" {
		return errors.New("team model policy: team_id is required")
	}
	if strings.TrimSpace(policy.Provider) == "" {
		return errors.New("team model policy: provider is required")
	}
	now := time.Now().UTC()
	return s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing tables.TableTeamModelPolicy
		err := dbForUpdate(tx).
			Where("team_id = ? AND provider = ?", policy.TeamID, policy.Provider).
			First(&existing).Error
		switch {
		case err == nil:
			policy.ID = existing.ID
			policy.CreatedAt = existing.CreatedAt
		case errors.Is(err, gorm.ErrRecordNotFound):
			if policy.ID == "" {
				policy.ID = uuid.NewString()
			}
			if policy.CreatedAt.IsZero() {
				policy.CreatedAt = now
			}
		default:
			return fmt.Errorf("failed to query existing team model policy: %w", err)
		}
		policy.UpdatedAt = now
		return tx.Save(policy).Error
	})
}

// DeleteTeamModelPolicy removes the ACL row for a (team, provider) pair,
// returning the team to "inherit global" semantics for that provider.
// Returns ErrNotFound when no row matched the pair so the handler can
// surface 404 instead of a silent no-op.
func (s *RDBConfigStore) DeleteTeamModelPolicy(ctx context.Context, teamID, provider string) error {
	res := s.DB().WithContext(ctx).
		Delete(&tables.TableTeamModelPolicy{}, "team_id = ? AND provider = ?", teamID, provider)
	if res.Error != nil {
		return s.parseGormError(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
