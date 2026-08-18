package rtk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// 1.5 Diagnostics interface test
// TDD red phase: FilterLoadDiagnostic, Loader.Diagnostics(), and the new
// canonical Filter fields do not exist yet (compile error).
// ============================================================================

// TestLoaderDiagnostics verifies that validation failures during Load produce
// structured FilterLoadDiagnostic records accessible via Diagnostics().
func TestLoaderDiagnostics(t *testing.T) {
	t.Run("redos_project_filter_recorded", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
			t.Fatal(err)
		}
		// Write a filter with a ReDoS-prone pattern → validation fails.
		mustWriteFile(t, filepath.Join(root, ".rtk", "filters.json"), []byte(`[
			{"id": "evil-redos", "command": "evil", "priority": 80, "rules": [{"type": "strip", "pattern": "(a+)+b"}]}
		]`))

		cfg := &Config{Enabled: true, TrustProjectFilters: true}
		loader := NewFilterLoader(cfg)
		_ = loader.Load(root)

		if len(loader.Diagnostics()) == 0 {
			t.Fatal("expected at least one diagnostic after loading a ReDoS-prone filter")
		}

		var hit *FilterLoadDiagnostic
		for i := range loader.Diagnostics() {
			d := &loader.Diagnostics()[i]
			if d.Source == "project" && strings.Contains(d.Message, "ReDoS") {
				hit = d
				break
			}
		}
		if hit == nil {
			t.Fatalf("expected a warning diagnostic containing 'ReDoS' for the project source, got %+v", loader.Diagnostics())
		}
		if hit.Level != "warning" {
			t.Errorf("diagnostic.Level = %q, want %q", hit.Level, "warning")
		}
		if hit.Format != "omniroute-json" {
			t.Errorf("diagnostic.Format = %q, want %q", hit.Format, "omniroute-json")
		}
		if !strings.Contains(hit.Path, ".rtk") || !strings.Contains(hit.Path, "filters.json") {
			t.Errorf("diagnostic.Path = %q, should contain .rtk/filters.json", hit.Path)
		}
	})

	t.Run("untrusted_project_skip_recorded", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".rtk"), 0o755); err != nil {
			t.Fatal(err)
		}
		// Valid filter, but no trust.json → project source skipped.
		mustWriteFile(t, filepath.Join(root, ".rtk", "filters.json"), []byte(`[
			{"id": "harmless", "command": "echo hello", "priority": 50, "head": 1, "tail": 1}
		]`))

		loader := NewFilterLoader(&Config{Enabled: true})
		_ = loader.Load(root)

		var hit *FilterLoadDiagnostic
		for i := range loader.Diagnostics() {
			d := &loader.Diagnostics()[i]
			if d.Source == "project" && strings.Contains(d.Message, "untrusted") {
				hit = d
				break
			}
		}
		if hit == nil {
			t.Fatalf("expected a warning diagnostic containing 'untrusted' for the project source, got %+v", loader.Diagnostics())
		}
		if hit.Level != "warning" {
			t.Errorf("diagnostic.Level = %q, want %q", hit.Level, "warning")
		}
		if hit.Path == "" {
			t.Error("diagnostic.Path should not be empty")
		}
	})
}