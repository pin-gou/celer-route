package providercooldown

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pin-gou/celer-route/core/schemas"
)

// Per-key metric surface smoke tests (B4 / task C).
//
// Decision: the per-key counters are exposed through GET
// /api/plugins/provider-cooldown/stats under `per_key` rather than through a
// Prometheus collector registered with the telemetry plugin.
//
// Why not Prometheus: core/schemas/plugin.go has no "plugin contributes a
// collector" seam. The telemetry plugin owns a private prometheus.Registry, is
// loaded (and reloaded) independently of this plugin, and registering into it
// from here would need a new cross-plugin accessor plus unregister-on-reload
// bookkeeping — a duplicate registration panics, so a reload could take the
// whole /metrics endpoint down. That is a large blast radius for a diagnostic
// counter. The /stats surface already answers the question the metric is meant
// to answer ("which key is paying for the cooldowns?") and needs no new wiring.
//
// The three documented counters are mark_count / suppressed_count /
// success_count. High cardinality stays in the JSON body (keyed by
// "<provider>::<keyID>"), never as a Prometheus label, so scraping is not
// affected either way.

// TestMetricsSurfacePerKeyCounters drives one mark, one suppression and one
// success through the plugin hooks and asserts the per-key row reports all
// three counters — the metric surface documented for B0/B4.
func TestMetricsSurfacePerKeyCounters(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(time.Minute)
	s.now = clk.Now
	p := &CooldownPlugin{State: s}

	// 1. A mark: cool the key down.
	s.markWithPolicy(nil, schemas.OpenAI, "k1", "gpt-4o", 40*time.Second, CooldownKindRateLimit, schemas.CooldownScopeModel)

	// 2. A suppression: the key pool filter vetoes it.
	if _, err := s.AsFilter(nil)(newPerKeyTrailCtx("k1"), schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: "k1"}}); err != nil {
		t.Fatalf("filter: %v", err)
	}

	// 3. A success on a different key, so success_count is attributable.
	ctx := withPolicy(newPerKeyTrailCtx("k2"), nil)
	if _, _, err := p.PostLLMHook(ctx, newPerKeyResponse(schemas.OpenAI), nil); err != nil {
		t.Fatalf("PostLLMHook: %v", err)
	}

	stats := p.Stats()
	if stats.PerKey == nil {
		t.Fatal("stats.per_key must be present once a per-key event happened")
	}

	row, ok := stats.PerKey["openai::k1"]
	if !ok {
		t.Fatalf("per_key has no openai::k1 row: %#v", stats.PerKey)
	}
	if row.MarkCount != 1 {
		t.Fatalf("openai::k1 mark_count = %d, want 1", row.MarkCount)
	}
	if row.SuppressedCount != 1 {
		t.Fatalf("openai::k1 suppressed_count = %d, want 1", row.SuppressedCount)
	}
	if row.SuccessCount != 0 {
		t.Fatalf("openai::k1 success_count = %d, want 0", row.SuccessCount)
	}

	okRow, ok := stats.PerKey["openai::k2"]
	if !ok {
		t.Fatalf("per_key has no openai::k2 row: %#v", stats.PerKey)
	}
	if okRow.SuccessCount != 1 {
		t.Fatalf("openai::k2 success_count = %d, want 1", okRow.SuccessCount)
	}

	// The wire shape keeps the documented snake_case names, so a dashboard can
	// address them directly.
	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal stats: %v", err)
	}
	var decoded struct {
		PerKey map[string]map[string]any `json:"per_key"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	rowAny, ok := decoded.PerKey["openai::k1"]
	if !ok {
		t.Fatalf("decoded per_key has no openai::k1 row: %s", raw)
	}
	for _, field := range []string{"mark_count", "suppressed_count", "success_count"} {
		if _, ok := rowAny[field]; !ok {
			t.Fatalf("per_key row is missing %q: %s", field, raw)
		}
	}
}

// TestMetricsSurfaceStatsAggregatesAgree pins that the per-key rows sum to the
// aggregate counters: the per-key surface is a breakdown, not a second,
// drifting accounting path.
func TestMetricsSurfaceStatsAggregatesAgree(t *testing.T) {
	clk := newFakeClock()
	s := NewCooldownState(time.Minute)
	s.now = clk.Now

	keys := []string{"a", "b", "c"}
	for _, k := range keys {
		s.MarkWithTTL(schemas.OpenAI, k, "", time.Minute, CooldownKindQuota, schemas.CooldownScopeKey)
	}
	filter := s.AsFilter(nil)
	for _, k := range keys {
		if _, err := filter(nil, schemas.OpenAI, "gpt-4o", []schemas.Key{{ID: k}}); err != nil {
			t.Fatalf("filter: %v", err)
		}
	}

	stats := s.Stats()
	var marks, suppressed uint64
	for _, row := range stats.PerKey {
		marks += row.MarkCount
		suppressed += row.SuppressedCount
	}
	if marks != stats.MarkCount {
		t.Fatalf("per_key mark total = %d, aggregate mark_count = %d", marks, stats.MarkCount)
	}
	if suppressed != stats.SuppressedCount {
		t.Fatalf("per_key suppressed total = %d, aggregate suppressed_count = %d", suppressed, stats.SuppressedCount)
	}
	if len(stats.PerKey) != len(keys) {
		t.Fatalf("per_key rows = %d, want %d", len(stats.PerKey), len(keys))
	}
}
