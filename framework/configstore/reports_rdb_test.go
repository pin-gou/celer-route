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
