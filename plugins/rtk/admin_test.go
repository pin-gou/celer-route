package rtk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetFilterCatalogReturnsBuiltins verifies that GetFilterCatalog always
// returns a non-nil catalog containing the builtins even before Load is
// called, so /api/context/rtk/filters is usable on a freshly initialised
// plugin.
func TestGetFilterCatalogReturnsBuiltins(t *testing.T) {
	tmp := t.TempDir()
	plugin, err := Init(t.Context(), &Config{Enabled: true, Intensity: "standard"}, nil, tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	cat := plugin.GetFilterCatalog()
	if cat.Filters == nil {
		t.Fatal("Filters should be non-nil slice")
	}
	if cat.Counters["builtin"] == 0 {
		t.Error("Counters.builtin should be > 0 after Init")
	}
	if cat.Counters["total"] != cat.Counters["builtin"] {
		t.Errorf("Counters.total = %d, want %d", cat.Counters["total"], cat.Counters["builtin"])
	}
	// Diagnostics is always a non-nil slice.
	if cat.Diagnostics == nil {
		t.Error("Diagnostics should be a non-nil slice")
	}

	// Sanity-check one entry: it should have a Source set.
	seen := false
	for _, e := range cat.Filters {
		if e.Source == "builtin" {
			seen = true
			if e.Priority == 0 {
				t.Error("Priority should default to 50 when the filter has no explicit priority")
			}
		}
	}
	if !seen {
		t.Error("expected at least one builtin-source entry")
	}
}

// TestGetFilterCatalogWithProjectFilters seeds a project source with one
// custom filter and confirms it shows up under source="project".
func TestGetFilterCatalogWithProjectFilters(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".rtk"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".rtk", "trust.json"),
		[]byte(`{"filtersSha256":"00"}`), 0o644); err != nil {
		t.Fatalf("write trust: %v", err)
	}
	custom := `[{
        "id": "custom-test",
        "label": "Custom test filter",
        "description": "Created by unit test",
        "category": "shell",
        "priority": 70,
        "match": {"commands": ["^custom-test$"]},
        "rules": [{"type": "collapse", "pattern": "^\\s*$"}],
        "head": 5, "tail": 5, "max_lines": 20
    }]`
	if err := os.WriteFile(filepath.Join(tmp, ".rtk", "filters.json"), []byte(custom), 0o644); err != nil {
		t.Fatalf("write filters: %v", err)
	}

	plugin, err := Init(t.Context(), &Config{
		Enabled:             true,
		Intensity:           "standard",
		TrustProjectFilters: true,
	}, nil, tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	cat := plugin.GetFilterCatalog()
	if cat.Counters["project"] == 0 {
		t.Error("expected at least one project-source entry, got 0")
	}
	found := false
	for _, e := range cat.Filters {
		if e.ID == "custom-test" {
			found = true
			if e.Source != "project" {
				t.Errorf("custom-test source = %q, want project", e.Source)
			}
			if e.Priority != 70 {
				t.Errorf("custom-test priority = %d, want 70", e.Priority)
			}
			if e.Label != "Custom test filter" {
				t.Errorf("custom-test label = %q, want %q", e.Label, "Custom test filter")
			}
			if !e.HasOnEmpty && e.Description == "" {
				t.Error("description should be carried through to the catalog entry")
			}
		}
	}
	if !found {
		t.Error("custom-test filter not found in catalog")
	}
}

// TestRunTestOnEmptyPayload ensures RunTest is a no-op for empty input.
func TestRunTestOnEmptyPayload(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	res := plugin.RunTest(TestPayload{Output: "", Command: "git status"})
	if res.CompressedText != "" {
		t.Errorf("CompressedText = %q, want empty", res.CompressedText)
	}
	if res.CompressedTokens != 0 {
		t.Errorf("CompressedTokens = %d, want 0", res.CompressedTokens)
	}
}

// TestRunTestAppliesGitStatusFilter verifies that the git-status filter is
// selected for a realistic git status payload. The git-status filter uses
// head=5/tail=2, so the assertion is on filter selection, not token savings:
// the input below intentionally fits within head+tail so the compressed
// output may equal the input — what we verify is that the pipeline picked
// the right filter.
func TestRunTestAppliesGitStatusFilter(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true, Intensity: "standard"}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	input := strings.Join([]string{
		"On branch main",
		"Your branch is up to date with 'origin/main'.",
		"",
		"Changes not staged for commit:",
		"  modified:   plugins/rtk/admin.go",
		"  modified:   plugins/rtk/rtk.go",
		"",
		"no changes added to commit (use \"git add\" and/or \"git commit -a\")",
	}, "\n")
	res := plugin.RunTest(TestPayload{Output: input, Command: "git status", ApplyRules: true})
	if res.FilterMatched == "" {
		t.Error("expected a filter to be matched for git status input")
	}
	if !strings.Contains(strings.ToLower(res.FilterMatched), "git") {
		t.Errorf("FilterMatched = %q, expected a git-* filter", res.FilterMatched)
	}
	if res.OriginalTokens == 0 {
		t.Error("OriginalTokens should be > 0")
	}
}

// TestRunTestCompressesLongerOutput exercises a payload that exceeds the
// git-status filter's head+tail budget, ensuring compression actually fires.
func TestRunTestCompressesLongerOutput(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true, Intensity: "aggressive"}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	lines := []string{
		"On branch main",
		"Your branch is up to date with 'origin/main'.",
		"",
		"Changes not staged for commit:",
	}
	for i := 0; i < 80; i++ {
		lines = append(lines, "  modified:   file_"+itoa(i)+".go")
	}
	lines = append(lines, "", "no changes added to commit (use \"git add\")")
	res := plugin.RunTest(TestPayload{
		Output:     strings.Join(lines, "\n"),
		Command:    "git status",
		ApplyRules: true,
	})
	if res.FilterMatched == "" {
		t.Fatal("expected a filter match")
	}
	if res.CompressedTokens >= res.OriginalTokens {
		t.Errorf("CompressedTokens (%d) should be < OriginalTokens (%d)",
			res.CompressedTokens, res.OriginalTokens)
	}
	if res.CompressionRatio <= 0 {
		t.Errorf("CompressionRatio = %f, expected > 0", res.CompressionRatio)
	}
}

// TestRunTestApplyRulesFalseSkipsFilters confirms that ApplyRules=false
// does not produce a FilterMatched value (line-filter step is skipped).
func TestRunTestApplyRulesFalseSkipsFilters(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true, Intensity: "standard"}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	input := "On branch main\nYour branch is up to date with 'origin/main'.\n"
	res := plugin.RunTest(TestPayload{Output: input, Command: "git status", ApplyRules: false})
	if res.FilterMatched != "" {
		t.Errorf("FilterMatched = %q, want empty when ApplyRules=false", res.FilterMatched)
	}
}

// TestPreviewCompressionOffIsNoop verifies that mode="off" returns the
// payload unchanged.
func TestPreviewCompressionOffIsNoop(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	const payload = "On branch main\nYour branch is up to date.\n"
	res := plugin.PreviewCompression(PreviewRequest{
		Mode:    CompressionModeOff,
		Payload: TestPayload{Output: payload, Command: "git status"},
	})
	if res.Result.CompressedText != payload {
		t.Errorf("CompressedText mismatch:\n got=%q\nwant=%q", res.Result.CompressedText, payload)
	}
	if res.Result.CompressionRatio != 0 {
		t.Errorf("CompressionRatio = %f, want 0 for off mode", res.Result.CompressionRatio)
	}
	if res.OriginalConfig == nil || res.EffectiveConfig == nil {
		t.Error("OriginalConfig and EffectiveConfig should both be set")
	}
	if res.OriginalConfig.Intensity != res.EffectiveConfig.Intensity {
		t.Error("EffectiveConfig.Intensity should match OriginalConfig.Intensity when no override is provided")
	}
}

// TestPreviewCompressionRtkReturnsRatio verifies the rtk mode produces a
// non-zero compression ratio for repetitive git-status output.
func TestPreviewCompressionRtkReturnsRatio(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true, Intensity: "standard"}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Long enough that smartTruncate head/tail + the appended raw-output
	// pointer hint still leave the result noticeably smaller than the input.
	// Earlier revisions of this test used ~10 lines, which after the
	// ~14-token hint made CompressionRatio <= 0 — the assertion is supposed
	// to verify compression actually happened, not pin a specific threshold.
	lines := []string{
		"On branch main",
		"Your branch is up to date with 'origin/main'.",
		"",
		"Changes not staged for commit:",
		"  modified:   foo.go",
		"  modified:   bar.go",
		"  modified:   baz.go",
		"  modified:   qux.go",
		"  modified:   src/api/handler.go",
		"  modified:   src/api/middleware.go",
		"  modified:   src/api/router.go",
		"  modified:   src/api/server.go",
		"  modified:   src/auth/oauth.go",
		"  modified:   src/auth/jwt.go",
		"  modified:   src/config/database.go",
		"  modified:   src/config/redis.go",
		"  modified:   src/utils/logger.go",
		"  modified:   src/utils/metrics.go",
		"  modified:   src/utils/tracing.go",
		"  modified:   tests/integration/api_test.go",
		"  modified:   tests/integration/auth_test.go",
		"  modified:   tests/integration/db_test.go",
		"  modified:   tests/unit/handler_test.go",
		"  modified:   tests/unit/middleware_test.go",
		"  modified:   tests/unit/router_test.go",
		"  modified:   go.mod",
		"  modified:   go.sum",
		"  modified:   Dockerfile",
		"  modified:   docker-compose.yml",
		"  modified:   README.md",
		"  modified:   CHANGELOG.md",
		"",
		"no changes added to commit",
	}
	res := plugin.PreviewCompression(PreviewRequest{
		Mode:    CompressionModeRTK,
		Payload: TestPayload{Output: strings.Join(lines, "\n"), Command: "git status"},
	})
	if res.Result.OriginalTokens == 0 {
		t.Error("OriginalTokens should be > 0")
	}
	if res.Result.CompressedTokens == 0 {
		t.Error("CompressedTokens should be > 0 after compression")
	}
	if res.Result.CompressionRatio <= 0 {
		t.Errorf("CompressionRatio = %f, expected > 0", res.Result.CompressionRatio)
	}
	if len(res.EnginesPlanned) == 0 {
		t.Error("EnginesPlanned should at least list the rtk engine")
	}
}

// TestPreviewCompressionIntensityOverride verifies the Intensity field in
// PreviewRequest overrides the runtime intensity.
func TestPreviewCompressionIntensityOverride(t *testing.T) {
	plugin, err := Init(t.Context(), &Config{Enabled: true, Intensity: "standard"}, nil, t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Build a payload that responds to intensity scaling (long head/tail budget).
	lines := make([]string, 0, 200)
	lines = append(lines, "On branch main", "Your branch is up to date.")
	for i := 0; i < 200; i++ {
		lines = append(lines, "  modified:   file_"+itoa(i)+".go")
	}
	payload := strings.Join(lines, "\n")

	standard := plugin.PreviewCompression(PreviewRequest{
		Mode:     CompressionModeRTK,
		Payload:  TestPayload{Output: payload, Command: "git status"},
		Intensity: "aggressive",
	})
	if standard.EffectiveConfig.Intensity != "aggressive" {
		t.Errorf("EffectiveConfig.Intensity = %q, want aggressive", standard.EffectiveConfig.Intensity)
	}
	if standard.OriginalConfig.Intensity != "standard" {
		t.Errorf("OriginalConfig.Intensity = %q, want standard (must not be mutated)", standard.OriginalConfig.Intensity)
	}
	// Verify the live plugin config is unchanged.
	if plugin.config.Intensity != "standard" {
		t.Errorf("live plugin intensity = %q, want standard", plugin.config.Intensity)
	}
}

// TestIsValidRawOutputID checks the ID validator used by the handler to
// reject malformed path parameters before any disk lookup.
func TestIsValidRawOutputID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"0123456789abcdef01234567", true},
		{"0123456789ABCDEF01234567", true}, // hex regex is case-insensitive
		{"", false},
		{"too-short", false},
		{"contains spaces          ", false},
		{"0123456789abcdef0123456g", false}, // 'g' is not hex
		{strings.Repeat("a", 25), false},
		{strings.Repeat("a", 23), false},
	}
	for _, tc := range cases {
		if got := IsValidRawOutputID(tc.in); got != tc.want {
			t.Errorf("IsValidRawOutputID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// itoaFromInt — the rtk package already declares itoa (grouper.go). Reuse it
// to avoid an unused-import lint warning. This stub exists only so the
// admin_test.go file compiles standalone if the grouper.go itoa is renamed.
var _ = itoa