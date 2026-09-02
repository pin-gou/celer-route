package rtk

import (
	"testing"
)

func TestGetCavemanRuleCatalog_NilPlugin(t *testing.T) {
	var p *Plugin
	cat := p.GetCavemanRuleCatalog()
	if cat.Rules == nil {
		t.Fatal("expected non-nil Rules slice for nil receiver")
	}
	if len(cat.Rules) != 0 {
		t.Fatalf("expected empty rules for nil receiver, got %d", len(cat.Rules))
	}
	if cat.BuiltInPreservePatterns == nil {
		t.Fatal("expected non-nil BuiltInPreservePatterns slice for nil receiver")
	}
}

func TestGetCavemanRuleCatalog_HasExpectedShape(t *testing.T) {
	cat := (&Plugin{}).GetCavemanRuleCatalog()
	if len(cat.Rules) == 0 {
		t.Fatal("catalog must contain at least one rule")
	}

	// First call and second call must return identical content (cache hit).
	cat2 := (&Plugin{}).GetCavemanRuleCatalog()
	if len(cat.Rules) != len(cat2.Rules) {
		t.Fatalf("catalog length changed across calls: %d vs %d", len(cat.Rules), len(cat2.Rules))
	}
	for i := range cat.Rules {
		if cat.Rules[i] != cat2.Rules[i] {
			t.Fatalf("catalog entry %d drifted across calls", i)
		}
	}
}

func TestGetCavemanRuleCatalog_NoDuplicateNames(t *testing.T) {
	cat := (&Plugin{}).GetCavemanRuleCatalog()
	seen := make(map[string]int)
	for _, r := range cat.Rules {
		seen[r.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Fatalf("rule %q appears %d times in catalog", name, count)
		}
	}
}

func TestGetCavemanRuleCatalog_StableSort(t *testing.T) {
	cat := (&Plugin{}).GetCavemanRuleCatalog()
	for i := 1; i < len(cat.Rules); i++ {
		prev := cat.Rules[i-1]
		cur := cat.Rules[i]
		// Language-first ordering: every en entry precedes every zh entry.
		if prev.Language == "zh" && cur.Language == "en" {
			t.Fatalf("catalog not sorted by language: zh at %d preceded en at %d", i-1, i)
		}
		if prev.Language == cur.Language && prev.Category > cur.Category {
			t.Fatalf("catalog not sorted by category within language=%q", prev.Language)
		}
		if prev.Language == cur.Language && prev.Category == cur.Category && prev.Name > cur.Name {
			t.Fatalf("catalog not sorted by name within category=%q", cur.Category)
		}
	}
}

func TestGetCavemanRuleCatalog_FieldsPopulated(t *testing.T) {
	cat := (&Plugin{}).GetCavemanRuleCatalog()
	if len(cat.Rules) == 0 {
		t.Fatal("expected non-empty rules")
	}
	for i, r := range cat.Rules {
		if r.Name == "" {
			t.Fatalf("rule at %d missing Name", i)
		}
		if r.Label == "" {
			t.Fatalf("rule %q missing Label", r.Name)
		}
		if r.Context == "" {
			t.Fatalf("rule %q missing Context", r.Name)
		}
		if r.Language != "en" && r.Language != "zh" {
			t.Fatalf("rule %q has unexpected language %q", r.Name, r.Language)
		}
		if r.MinIntensity != "lite" && r.MinIntensity != "full" && r.MinIntensity != "ultra" {
			t.Fatalf("rule %q has unexpected MinIntensity %q", r.Name, r.MinIntensity)
		}
		if r.Category == "" {
			t.Fatalf("rule %q missing Category", r.Name)
		}
	}
}

func TestGetCavemanRuleCatalog_KnownEntriesPresent(t *testing.T) {
	cat := (&Plugin{}).GetCavemanRuleCatalog()
	want := []string{"pleasantries", "articles", "ultra_abbreviations", "zh_filler_please", "zh_ultra_modal_particles"}
	have := make(map[string]bool, len(cat.Rules))
	for _, r := range cat.Rules {
		have[r.Name] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("expected rule %q in catalog", name)
		}
	}
}

func TestGetCavemanRuleCatalog_BuiltInPreserve(t *testing.T) {
	cat := (&Plugin{}).GetCavemanRuleCatalog()
	if len(cat.BuiltInPreservePatterns) != len(preservePatterns) {
		t.Fatalf("BuiltInPreservePatterns length = %d, want %d", len(cat.BuiltInPreservePatterns), len(preservePatterns))
	}
	// Sanity: frontmatter and fenced-code are well-known entries.
	want := map[string]bool{"frontmatter": false, "fenced-code": false, "url": false, "const-case": false}
	for _, name := range cat.BuiltInPreservePatterns {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected built-in preserve pattern %q", name)
		}
	}
}

func TestGetCavemanRuleCatalog_ResetCache(t *testing.T) {
	// Mutate the cached rules slice in place; after Reset, the next call
	// must produce a fresh, independent slice that doesn't share the
	// mutation.
	cat1 := (&Plugin{}).GetCavemanRuleCatalog()
	if len(cat1.Rules) == 0 {
		t.Fatal("expected non-empty catalog for reset test")
	}
	originalName := cat1.Rules[0].Name
	cat1.Rules[0].Name = "__mutated__"

	ResetCavemanRuleCatalogCache()
	cat2 := (&Plugin{}).GetCavemanRuleCatalog()
	if cat2.Rules[0].Name != originalName {
		t.Fatalf("cache reset did not yield fresh data: got %q, want %q", cat2.Rules[0].Name, originalName)
	}
}