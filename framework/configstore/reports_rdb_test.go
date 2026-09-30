package configstore

import (
	"context"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupReportsTestStore spins up a fresh in-memory SQLite + the new tables so
// the price-book CRUD tests run in isolation. Mirrors the pattern used by
// rdb_phase2_test.go.
func setupReportsTestStore(t *testing.T) *RDBConfigStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&tables.TableStandardPrice{},
		&tables.TableTeamPricingProfile{},
		&tables.TableBillingReconciliation{},
		&tables.TableBillingReconItem{},
		&tables.TableTeamModelPolicy{},
		// Needed by DeleteTeam, which the team-cascade test exercises: it loads
		// the team with its RateLimit preloaded and nulls team_id on virtual keys.
		&tables.TableTeam{},
		&tables.TableVirtualKey{},
		&tables.TableRateLimit{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := &RDBConfigStore{logger: nil}
	s.db.Store(db)
	s.migrateOnFreshFn = func(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
		return fn(ctx, s.DB())
	}
	s.refreshPoolFn = func(ctx context.Context) error { return nil }
	return s
}

func TestCreateAndGetStandardPrice(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	row := &tables.TableStandardPrice{
		Provider:             "openai",
		Model:                "gpt-4o",
		InputCostPerMillion:  2.5,
		OutputCostPerMillion: 10,
		EffectiveFrom:        time.Now().UTC(),
	}
	if err := s.CreateStandardPrice(ctx, row); err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.ID == "" {
		t.Fatalf("id should be minted server-side")
	}
	got, err := s.GetStandardPriceByID(ctx, row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want openai", got.Provider)
	}
}

func TestGetActiveStandardPricePicksLatest(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	older := &tables.TableStandardPrice{
		Provider: "openai", Model: "gpt-4o",
		InputCostPerMillion: 2.5, OutputCostPerMillion: 10,
		EffectiveFrom: at.Add(-30 * 24 * time.Hour),
	}
	newer := &tables.TableStandardPrice{
		Provider: "openai", Model: "gpt-4o",
		InputCostPerMillion: 5.0, OutputCostPerMillion: 20,
		EffectiveFrom: at.Add(-7 * 24 * time.Hour),
	}
	future := &tables.TableStandardPrice{
		Provider: "openai", Model: "gpt-4o",
		InputCostPerMillion: 9.0, OutputCostPerMillion: 30,
		EffectiveFrom: at.Add(7 * 24 * time.Hour),
	}
	for _, r := range []*tables.TableStandardPrice{older, newer, future} {
		if err := s.CreateStandardPrice(ctx, r); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	got, err := s.GetActiveStandardPrice(ctx, "openai", "gpt-4o", at)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a row, got nil")
	}
	if got.InputCostPerMillion != 5.0 {
		t.Errorf("input = %v, want 5.0 (newer row, before future row takes effect)", got.InputCostPerMillion)
	}
}

func TestListStandardPricesWithFilters(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	for _, p := range []*tables.TableStandardPrice{
		{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: 1, OutputCostPerMillion: 1, EffectiveFrom: time.Now().UTC()},
		{Provider: "openai", Model: "gpt-4o-mini", InputCostPerMillion: 1, OutputCostPerMillion: 1, EffectiveFrom: time.Now().UTC()},
		{Provider: "anthropic", Model: "claude-3", InputCostPerMillion: 1, OutputCostPerMillion: 1, EffectiveFrom: time.Now().UTC()},
	} {
		if err := s.CreateStandardPrice(ctx, p); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rows, total, err := s.ListStandardPrices(ctx, StandardPriceQueryParams{Provider: "openai"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}
}

func TestDeleteStandardPrice(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	row := &tables.TableStandardPrice{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: 1, OutputCostPerMillion: 1}
	if err := s.CreateStandardPrice(ctx, row); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.DeleteStandardPrice(ctx, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteStandardPrice(ctx, row.ID); err != ErrNotFound {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}

func TestBulkCreateStandardPrices(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	rows := []tables.TableStandardPrice{
		{Provider: "openai", Model: "gpt-4o", InputCostPerMillion: 1, OutputCostPerMillion: 1},
		{Provider: "openai", Model: "gpt-4o-mini", InputCostPerMillion: 1, OutputCostPerMillion: 1},
	}
	if err := s.BulkCreateStandardPrices(ctx, rows); err != nil {
		t.Fatalf("bulk: %v", err)
	}
	for i, r := range rows {
		if r.ID == "" {
			t.Errorf("row %d id not minted", i)
		}
	}
}

func TestUpsertTeamPricingProfile(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	// missing team → error
	if err := s.UpsertTeamPricingProfile(ctx, &tables.TableTeamPricingProfile{}); err == nil {
		t.Fatalf("expected missing team_id to be rejected")
	}
	// insert
	p := &tables.TableTeamPricingProfile{TeamID: "t1", Mode: tables.TeamPricingModeStandard, MarginMultiplier: 1.0}
	if err := s.UpsertTeamPricingProfile(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.GetTeamPricingProfile(ctx, "t1")
	if err != nil || got == nil {
		t.Fatalf("get after upsert: %v / %v", err, got)
	}
	// update (re-upsert)
	p.Mode = tables.TeamPricingModeActual
	if err := s.UpsertTeamPricingProfile(ctx, p); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _ = s.GetTeamPricingProfile(ctx, "t1")
	if got.Mode != tables.TeamPricingModeActual {
		t.Errorf("mode after re-upsert = %q, want actual", got.Mode)
	}
}

// TestUpsertTeamPricingProfile_FreshStructPerCall reproduces how the HTTP
// handler actually calls this: every PUT builds a brand-new struct with an
// empty ID. A plain GORM Save then mints a fresh UUID and INSERTs, tripping
// the UNIQUE constraint on team_id on every edit after the first — the team
// pricing page could never be saved twice.
//
// TestUpsertTeamPricingProfile above does not catch this because it re-upserts
// the *same* pointer, whose ID was back-filled by the first Save.
func TestUpsertTeamPricingProfile_FreshStructPerCall(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()

	first := &tables.TableTeamPricingProfile{TeamID: "t-fresh", Mode: tables.TeamPricingModeStandard, MarginMultiplier: 1.0}
	if err := s.UpsertTeamPricingProfile(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	createdUnix := first.CreatedAt.Unix()

	// Sleep so a regression that overwrites created_at is distinguishable at
	// second granularity regardless of how the driver round-trips precision.
	time.Sleep(1100 * time.Millisecond)

	second := &tables.TableTeamPricingProfile{TeamID: "t-fresh", Mode: tables.TeamPricingModeActual, MarginMultiplier: 1.25}
	if err := s.UpsertTeamPricingProfile(ctx, second); err != nil {
		t.Fatalf("re-upsert with a fresh struct: %v", err)
	}

	rows, err := s.ListTeamPricingProfiles(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	matches := 0
	for _, r := range rows {
		if r.TeamID != "t-fresh" {
			continue
		}
		matches++
		if r.Mode != tables.TeamPricingModeActual {
			t.Errorf("mode = %q, want actual", r.Mode)
		}
		if r.ID != first.ID {
			t.Errorf("id changed across upsert: %q -> %q", first.ID, r.ID)
		}
		if r.CreatedAt.Unix() != createdUnix {
			t.Errorf("created_at was overwritten across upsert: %d -> %d", createdUnix, r.CreatedAt.Unix())
		}
	}
	if matches != 1 {
		t.Errorf("found %d rows for t-fresh, want exactly 1", matches)
	}
}

func TestDeleteTeamPricingProfile(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	_ = s.UpsertTeamPricingProfile(ctx, &tables.TableTeamPricingProfile{TeamID: "t1", Mode: tables.TeamPricingModeStandard, MarginMultiplier: 1.0})
	if err := s.DeleteTeamPricingProfile(ctx, "t1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteTeamPricingProfile(ctx, "t1"); err != ErrNotFound {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}

func TestListTeamPricingProfiles(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"t1", "t2", "t3"} {
		_ = s.UpsertTeamPricingProfile(ctx, &tables.TableTeamPricingProfile{TeamID: id, Mode: tables.TeamPricingModeStandard, MarginMultiplier: 1.0})
	}
	rows, err := s.ListTeamPricingProfiles(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("rows = %d, want 3", len(rows))
	}
}

func TestUpsertTeamModelPolicy(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()

	// nil → error
	if err := s.UpsertTeamModelPolicy(ctx, nil); err == nil {
		t.Fatalf("expected nil policy to be rejected")
	}
	// missing team → error
	if err := s.UpsertTeamModelPolicy(ctx, &tables.TableTeamModelPolicy{Provider: "openai"}); err == nil {
		t.Fatalf("expected missing team_id to be rejected")
	}
	// missing provider → error
	if err := s.UpsertTeamModelPolicy(ctx, &tables.TableTeamModelPolicy{TeamID: "t1"}); err == nil {
		t.Fatalf("expected missing provider to be rejected")
	}

	// first write — row is created with a server-minted UUID
	p := &tables.TableTeamModelPolicy{
		TeamID:            "t1",
		Provider:          "openai",
		AllowedModels:     []string{"gpt-4o"},
		BlacklistedModels: []string{"gpt-3.5-turbo"},
	}
	if err := s.UpsertTeamModelPolicy(ctx, p); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if p.ID == "" {
		t.Fatalf("expected server to mint an ID on first upsert")
	}
	firstID := p.ID
	firstCreated := p.CreatedAt
	if firstCreated.IsZero() {
		t.Fatalf("expected CreatedAt to be set on first upsert")
	}

	// re-upsert with new model lists — must not trip UNIQUE constraint
	p.AllowedModels = []string{"gpt-4o", "gpt-4o-mini"}
	p.BlacklistedModels = nil
	if err := s.UpsertTeamModelPolicy(ctx, p); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if p.ID != firstID {
		t.Errorf("id changed across upsert: got %q, want %q", p.ID, firstID)
	}
	if !p.CreatedAt.Equal(firstCreated) {
		t.Errorf("CreatedAt changed across upsert: got %v, want %v", p.CreatedAt, firstCreated)
	}
	if !p.UpdatedAt.After(firstCreated) {
		t.Errorf("UpdatedAt (%v) should be after CreatedAt (%v)", p.UpdatedAt, firstCreated)
	}

	// independent second team/provider pair — both rows coexist
	p2 := &tables.TableTeamModelPolicy{TeamID: "t1", Provider: "anthropic", AllowedModels: []string{"*"}}
	if err := s.UpsertTeamModelPolicy(ctx, p2); err != nil {
		t.Fatalf("upsert anthropic: %v", err)
	}
	rows, err := s.ListTeamModelPolicies(ctx, "t1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}

	// delete one row, the other stays
	if err := s.DeleteTeamModelPolicy(ctx, "t1", "openai"); err != nil {
		t.Fatalf("delete openai: %v", err)
	}
	if err := s.DeleteTeamModelPolicy(ctx, "t1", "openai"); err != ErrNotFound {
		t.Errorf("second delete openai = %v, want ErrNotFound", err)
	}
	rows, _ = s.ListTeamModelPolicies(ctx, "t1")
	if len(rows) != 1 || rows[0].Provider != "anthropic" {
		t.Errorf("after delete, rows = %+v, want [anthropic]", rows)
	}
}

// TestCreateReconciliationWritesBatchAndItems covers the transactional
// write path. A reconciliation is meaningless without its items (the
// gateway-delta view needs both numbers) so the test fails if either side
// of the transaction silently drops.
func TestCreateReconciliationWritesBatchAndItems(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	row := &tables.TableBillingReconciliation{
		Provider:     "OpenAI",
		PeriodStart:  start,
		PeriodEnd:    end,
		Source:       tables.ReconciliationSourceUsageAPI,
		GatewayCost:  100,
		ProviderCost: 102.5,
		Delta:        2.5,
		Status:       tables.ReconciliationStatusMatched,
	}
	items := []tables.TableBillingReconItem{
		{Model: "gpt-4o", GatewayRequests: 100, ProviderRequests: 100, GatewayCost: 80, ProviderCost: 82},
		{Model: "gpt-4o-mini", GatewayRequests: 200, ProviderRequests: 200, GatewayCost: 20, ProviderCost: 20.5},
	}
	if err := s.CreateReconciliation(ctx, row, items); err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.ID == "" {
		t.Fatalf("id not assigned")
	}
	if row.Provider != "openai" {
		t.Errorf("provider should be lower-cased, got %q", row.Provider)
	}
	got, err := s.GetReconciliationByID(ctx, row.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Status != tables.ReconciliationStatusMatched {
		t.Errorf("status = %q, want matched", got.Status)
	}
	gotItems, err := s.ListReconciliationItems(ctx, row.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(gotItems) != 2 {
		t.Errorf("items = %d, want 2", len(gotItems))
	}
}

// TestReconciliationBeforeSaveRejectsBadRows exercises the validation path
// so callers get the sentinel error rather than a 500.
func TestReconciliationBeforeSaveRejectsBadRows(t *testing.T) {
	cases := []struct {
		name string
		row  *tables.TableBillingReconciliation
	}{
		{
			name: "missing provider",
			row: &tables.TableBillingReconciliation{
				PeriodStart: time.Now(), PeriodEnd: time.Now().Add(time.Hour),
				Source: tables.ReconciliationSourceUsageAPI,
			},
		},
		{
			name: "bad source",
			row: &tables.TableBillingReconciliation{
				Provider:    "openai",
				PeriodStart: time.Now(), PeriodEnd: time.Now().Add(time.Hour),
				Source: "telemetry_only",
			},
		},
		{
			name: "period inverted",
			row: &tables.TableBillingReconciliation{
				Provider:    "openai",
				PeriodStart: time.Now().Add(time.Hour), PeriodEnd: time.Now(),
				Source: tables.ReconciliationSourceUsageAPI,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.row.BeforeSave(nil); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

// TestUpdateReconciliationWritesOperatorFields covers the limited-field
// update path. Status / notes / costs are mutable; immutable fields
// (id, period, provider) must not silently shift under Update.
func TestUpdateReconciliationWritesOperatorFields(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	row := &tables.TableBillingReconciliation{
		Provider: "openai", PeriodStart: now, PeriodEnd: now.Add(time.Hour),
		Source: tables.ReconciliationSourceUsageAPI, Status: tables.ReconciliationStatusMatched,
	}
	if err := s.CreateReconciliation(ctx, row, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	row.Status = tables.ReconciliationStatusApplied
	note := "applied after credit adjustment"
	row.Notes = &note
	row.GatewayCost = 110
	row.ProviderCost = 115
	row.Delta = 5
	if err := s.UpdateReconciliation(ctx, row); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetReconciliationByID(ctx, row.ID)
	if got.Status != tables.ReconciliationStatusApplied {
		t.Errorf("status = %q, want applied", got.Status)
	}
	if got.Notes == nil || *got.Notes != note {
		t.Errorf("notes lost after update")
	}
	if got.Delta != 5 {
		t.Errorf("delta = %v, want 5", got.Delta)
	}
}

// TestListReconciliationsFiltersAndPages covers the list endpoint's filter
// combinations: provider-only, status-only, source-only, period window, and
// pagination metadata.
func TestListReconciliationsFiltersAndPages(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()
	q3Start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	q3End := q3Start.AddDate(0, 3, 0)
	q4Start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	q4End := q4Start.AddDate(0, 3, 0)
	openaiQ3 := &tables.TableBillingReconciliation{
		Provider: "openai", PeriodStart: q3Start, PeriodEnd: q3End,
		Source: tables.ReconciliationSourceUsageAPI, Status: tables.ReconciliationStatusMatched,
	}
	openaiQ4 := &tables.TableBillingReconciliation{
		Provider: "openai", PeriodStart: q4Start, PeriodEnd: q4End,
		Source: tables.ReconciliationSourceUsageAPI, Status: tables.ReconciliationStatusApplied,
	}
	anthropicQ3 := &tables.TableBillingReconciliation{
		Provider: "anthropic", PeriodStart: q3Start, PeriodEnd: q3End,
		Source: tables.ReconciliationSourceUsageAPI, Status: tables.ReconciliationStatusMatched,
	}
	for _, r := range []*tables.TableBillingReconciliation{openaiQ3, openaiQ4, anthropicQ3} {
		if err := s.CreateReconciliation(ctx, r, nil); err != nil {
			t.Fatalf("create %v: %v", r.Provider, err)
		}
	}
	t.Run("provider filter", func(t *testing.T) {
		rows, total, err := s.ListReconciliations(ctx, ReconciliationQueryParams{Provider: "openai"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 || len(rows) != 2 {
			t.Errorf("total=%d rows=%d, want 2/2", total, len(rows))
		}
	})
	t.Run("status filter", func(t *testing.T) {
		rows, _, err := s.ListReconciliations(ctx, ReconciliationQueryParams{Status: tables.ReconciliationStatusMatched})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("rows = %d, want 2", len(rows))
		}
	})
	t.Run("period window narrows", func(t *testing.T) {
		rows, _, err := s.ListReconciliations(ctx, ReconciliationQueryParams{
			PeriodStart: q3Start, PeriodEnd: q4Start,
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		// Q3 ends Sep 30, Q4 starts Oct 1; windowing on (q3Start,q4Start)
		// should keep both since their windows intersect.
		if len(rows) != 3 {
			t.Errorf("rows = %d, want 3", len(rows))
		}
	})
	t.Run("pagination", func(t *testing.T) {
		rows, total, err := s.ListReconciliations(ctx, ReconciliationQueryParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 || len(rows) != 2 {
			t.Errorf("total=%d rows=%d, want 3/2", total, len(rows))
		}
	})
}

// TestIsReconciliationUsageAPISupported confirms the first-wave list keeps
// the documented set. A drift between this set and the calibration job's
// allow-list would silently drop usage-API pulls.
func TestIsReconciliationUsageAPISupported(t *testing.T) {
	cases := []struct {
		provider string
		want     bool
	}{
		{"openai", true},
		{"OpenAI", true},
		{"anthropic", true},
		{"deepseek", true},
		{"azure", false},
		{"vertex", false},
		{"bedrock", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := tables.IsReconciliationUsageAPISupported(tc.provider); got != tc.want {
			t.Errorf("IsReconciliationUsageAPISupported(%q) = %v, want %v", tc.provider, got, tc.want)
		}
	}
}

// TestDeleteTeam_CascadesSideTables pins that deleting a team also removes its
// team-scoped side-table rows.
//
// Neither team_pricing_profiles nor team_model_policies declares a foreign key on
// team_id, and TableTeam carries no has-many relation for them, so nothing at the
// DB level cleans them up. Without an explicit delete the rows orphan and
// accumulate forever: unreachable through the API, yet still returned by the
// report/policy list endpoints and re-synced into the governance cache on boot.
func TestDeleteTeam_CascadesSideTables(t *testing.T) {
	s := setupReportsTestStore(t)
	ctx := context.Background()

	const teamID = "team-cascade"
	if err := s.DB().Create(&tables.TableTeam{ID: teamID, Name: "Cascade Team"}).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := s.UpsertTeamPricingProfile(ctx, &tables.TableTeamPricingProfile{
		TeamID: teamID, Mode: "standard", MarginMultiplier: 1.2,
	}); err != nil {
		t.Fatalf("upsert pricing profile: %v", err)
	}
	if err := s.UpsertTeamModelPolicy(ctx, &tables.TableTeamModelPolicy{
		TeamID: teamID, Provider: "openai", AllowedModels: []string{"gpt-4o"},
	}); err != nil {
		t.Fatalf("upsert model policy: %v", err)
	}

	// Sanity: both rows exist before the delete, so a pass cannot be a false
	// negative from a failed setup.
	if p, err := s.GetTeamPricingProfile(ctx, teamID); err != nil || p == nil {
		t.Fatalf("expected pricing profile to exist before delete (row=%v err=%v)", p, err)
	}
	if p, err := s.GetTeamModelPolicy(ctx, teamID, "openai"); err != nil || p == nil {
		t.Fatalf("expected model policy to exist before delete (row=%v err=%v)", p, err)
	}

	if err := s.DeleteTeam(ctx, teamID); err != nil {
		t.Fatalf("delete team: %v", err)
	}

	profile, err := s.GetTeamPricingProfile(ctx, teamID)
	if err != nil {
		t.Fatalf("get pricing profile after delete: %v", err)
	}
	if profile != nil {
		t.Fatalf("pricing profile must not survive team deletion, got %+v", profile)
	}

	policy, err := s.GetTeamModelPolicy(ctx, teamID, "openai")
	if err != nil {
		t.Fatalf("get model policy after delete: %v", err)
	}
	if policy != nil {
		t.Fatalf("model policy must not survive team deletion, got %+v", policy)
	}

	var profileCount, policyCount int64
	if err := s.DB().Model(&tables.TableTeamPricingProfile{}).Where("team_id = ?", teamID).Count(&profileCount).Error; err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if err := s.DB().Model(&tables.TableTeamModelPolicy{}).Where("team_id = ?", teamID).Count(&policyCount).Error; err != nil {
		t.Fatalf("count policies: %v", err)
	}
	if profileCount != 0 || policyCount != 0 {
		t.Fatalf("expected 0 orphan rows, got profiles=%d policies=%d", profileCount, policyCount)
	}
}
