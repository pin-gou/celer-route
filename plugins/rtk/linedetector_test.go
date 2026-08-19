package rtk

import "testing"

// TestDetector_OmniRouteAlignedTypes verifies that the detector emits the
// granular detection types aligned with OmniRoute's DETECTORS array. This
// is the surface area that semantic renderers key on, so any drift here
// would silently disable renderers.
func TestDetector_OmniRouteAlignedTypes(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantType string
		wantCat  string
	}{
		{
			name:     "git_diff",
			input:    "diff --git a/x.ts b/x.ts\n@@ -1,3 +1,3 @@\n-old\n+new",
			wantType: "git-diff",
			wantCat:  "git",
		},
		{
			name:     "git_log",
			input:    "commit abc1234abc1234abc1234abc1234abc1234abc\nAuthor: foo",
			wantType: "git-log",
			wantCat:  "git",
		},
		{
			name:     "test_pytest",
			input:    "===== 142 passed in 3.21s =====",
			wantType: "test-pytest",
			wantCat:  "test",
		},
		{
			name:     "test_jest",
			input:    "Tests: 5 passed, 5 total",
			wantType: "test-jest",
			wantCat:  "test",
		},
		{
			name:     "test_go_uses_em_dashes_not_jest_fail",
			input:    "=== RUN   TestFoo\n--- FAIL: TestBar (0.01s)\nFAIL\n",
			wantType: "test-go",
			wantCat:  "test",
		},
		{
			name:     "terraform_plan",
			input:    "Plan: 1 to add, 0 to change, 0 to destroy.",
			wantType: "terraform-plan",
			wantCat:  "infra",
		},
		{
			name:     "aws",
			input:    "An error occurred (AccessDenied) when calling the DescribeInstances operation",
			wantType: "aws",
			wantCat:  "cloud",
		},
		{
			name:     "json_output",
			input:    "[{\"name\":\"foo\"},{\"name\":\"bar\"}]",
			wantType: "json-output",
			wantCat:  "generic",
		},
		{
			name:     "build_typescript",
			input:    "src/foo.ts:10:5 - error TS2322: Type 'string' is not assignable to type 'number'.",
			wantType: "build-typescript",
			wantCat:  "build",
		},
		{
			name:     "build_vite",
			input:    "vite v5.0.0 building for production...\n✓ built in 1.23s",
			wantType: "build-vite",
			wantCat:  "build",
		},
		{
			name:     "uv_sync",
			input:    "Resolved 42 packages in 50ms\nInstalled 38 packages in 120ms",
			wantType: "uv-sync",
			wantCat:  "package",
		},
		{
			name:     "ruff",
			input:    "src/foo.py:10:5: E501 Line too long (120 > 88)",
			wantType: "ruff",
			wantCat:  "build",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := defaultDetector.detect(tc.input, "")
			if d.Type != tc.wantType {
				t.Errorf("type = %q, want %q", d.Type, tc.wantType)
			}
			if d.Category != tc.wantCat {
				t.Errorf("category = %q, want %q", d.Category, tc.wantCat)
			}
		})
	}
}

// TestDetector_CommandHintBoost verifies that providing a command hint
// raises the confidence for the matching detector.
func TestDetector_CommandHintBoost(t *testing.T) {
	input := "diff --git a/x.ts b/x.ts\n@@ -1,3 +1,3 @@\n-old\n+new"
	without := defaultDetector.detect(input, "")
	with := defaultDetector.detect(input, "git diff")
	if with.Confidence <= without.Confidence {
		t.Errorf("command hint should boost confidence: with=%v without=%v", with.Confidence, without.Confidence)
	}
	if with.Type != "git-diff" {
		t.Errorf("type = %q, want git-diff", with.Type)
	}
}

// TestDetector_FallbackShell verifies that unmatched inputs fall back to
// the generic shell type.
func TestDetector_FallbackShell(t *testing.T) {
	d := defaultDetector.detect("some unrecognized output text\nwith no shell markers", "")
	if d.Type != "shell" {
		t.Errorf("fallback type = %q, want shell", d.Type)
	}
}

// TestDetector_EmptyInput verifies that an empty input yields an empty
// detection.
func TestDetector_EmptyInput(t *testing.T) {
	d := defaultDetector.detect("", "")
	if d.Type != "" {
		t.Errorf("empty input type = %q, want empty string", d.Type)
	}
}