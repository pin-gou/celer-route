package rtk

import (
	"testing"
)

// TestNormalizeLine verifies the 7-step normalization logic from OmniRoute
// grouper.ts:39-56, applied in order:
//  1. ISO timestamps (e.g. 2024-01-15T10:30:00Z, 2024-01-15 10:30:00)
//  2. Bracketed date-time (e.g. [2024-01-01 10:00:00])
//  3. Hex strings 6-40 chars (e.g. a1b2c3d4e5f6)
//  4. Semantic version tokens (e.g. v1.2.3, 1.2.3, 1.2.3.4)
//  5. Standalone integers (e.g. 42)
//  6. Collapse repeated whitespace → single space
//  7. Trim leading/trailing whitespace
//
// TDD red phase: this function does not exist yet (compile error expected).
func TestNormalizeLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// ── Step 1: ISO timestamps ──────────────────────────────────────
		{
			name:  "iso_timestamp_basic",
			input: "2024-01-15T10:30:00Z",
			want:  "<N>",
		},
		{
			name:  "iso_timestamp_with_millis",
			input: "2024-01-15T10:30:00.123Z",
			want:  "<N>",
		},
		{
			name:  "iso_timestamp_space_separator",
			input: "2024-01-15 10:30:00",
			want:  "<N>",
		},
		{
			name:  "iso_timestamp_with_offset",
			input: "2024-01-15T10:30:00+08:00",
			want:  "<N>",
		},
		{
			name:  "iso_timestamp_within_text",
			input: "Started at 2024-01-15T10:30:00Z",
			want:  "Started at <N>",
		},

		// ── Step 2: Bracketed date-time ─────────────────────────────────
		{
			name:  "bracketed_timestamp",
			input: "[2024-01-01 10:00:00] Heartbeat received",
			want:  "[<N>] Heartbeat received",
		},
		{
			name:  "bracketed_timestamp_alone",
			input: "[2024-01-01 10:00:00]",
			want:  "[<N>]",
		},

		// ── Step 3: Hex strings 6-40 chars ─────────────────────────────
		{
			name:  "hex_40bit",
			input: "a1b2c3d4e5f6",
			want:  "<N>",
		},
		{
			name:  "hex_6chars",
			input: "ab12cd",
			want:  "<N>",
		},
		{
			name:  "hex_40chars",
			input: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4",
			want:  "<N>",
		},
		{
			name:  "hex_mixed_case",
			input: "DeadBeefCafe",
			want:  "<N>",
		},
		{
			name:  "hex_too_short",
			input: "abcde",
			want:  "abcde",
		},
		{
			name:  "hex_within_text",
			input: "Processing task a1b2c3d4",
			want:  "Processing task <N>",
		},

		// ── Step 4: Semantic version tokens ────────────────────────────
		{
			name:  "semver_v_prefix",
			input: "v1.2.3",
			want:  "<N>",
		},
		{
			name:  "semver_no_prefix",
			input: "1.2.3",
			want:  "<N>",
		},
		{
			name:  "semver_multi_segment",
			input: "v1.2.3.4",
			want:  "<N>",
		},
		{
			name:  "semver_within_text",
			input: "Checking dependency v1.0.0",
			want:  "Checking dependency <N>",
		},
		{
			name:  "semver_with_pre_release",
			input: "app-1.2.3-beta",
			want:  "app-<N>-beta",
		},

		// ── Step 5: Standalone integers ────────────────────────────────
		{
			name:  "integer_standalone",
			input: "42",
			want:  "<N>",
		},
		{
			name:  "integer_within_text",
			input: "Downloaded chunk 1",
			want:  "Downloaded chunk <N>",
		},
		{
			name:  "integer_multi_digit",
			input: "Downloaded chunk 12345",
			want:  "Downloaded chunk <N>",
		},
		{
			name:  "integer_after_semver_preserved",
			input: "v1.2.3 and 42",
			want:  "<N> and <N>",
		},

		// ── Step 6: Whitespace collapse ────────────────────────────────
		{
			name:  "whitespace_collapse_tabs",
			input: "a\t\tb",
			want:  "a b",
		},
		{
			name:  "whitespace_collapse_multi_spaces",
			input: "a    b    c",
			want:  "a b c",
		},
		{
			name:  "whitespace_collapse_mixed",
			input: "a \t  b\nc",
			want:  "a b c",
		},

		// ── Step 7: Trim ────────────────────────────────────────────────
		{
			name:  "trim_leading_spaces",
			input: "  hello",
			want:  "hello",
		},
		{
			name:  "trim_trailing_spaces",
			input: "hello  ",
			want:  "hello",
		},
		{
			name:  "trim_both",
			input: "  hello world  ",
			want:  "hello world",
		},

		// ── Mixed / composite ──────────────────────────────────────────
		{
			name:  "mixed_timestamp_hex_integer",
			input: "2024-01-15T10:30:00Z task 42 ab12cd",
			want:  "<N> task <N> <N>",
		},
		{
			name:  "everything_collapsed",
			input: "  [2024-01-01 10:00:00]  v1.2.3  a1b2c3  ",
			want:  "[<N>] <N> <N>",
		},
		{
			name:  "no_volatile_parts",
			input: "Build succeeded",
			want:  "Build succeeded",
		},
		{
			name:  "empty_line",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace_only",
			input: "   \t  ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLine(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestGroupSimilarLines verifies the groupSimilarLines function from OmniRoute
// grouper.ts:67-99. Seed cases taken from OmniRoute rtk-grouping.test.ts.
//
// TDD red phase: this function does not exist yet (compile error expected).
func TestGroupSimilarLines(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		threshold      int
		wantGrouped    int
		wantContains   []string
		wantNotContain []string
	}{
		// ── 6 lines differing only by numbers → merged ─────────────────
		{
			name: "six_numeric_lines_merged",
			input: "" +
				"Downloaded chunk 1\n" +
				"Downloaded chunk 2\n" +
				"Downloaded chunk 3\n" +
				"Downloaded chunk 4\n" +
				"Downloaded chunk 5\n" +
				"Downloaded chunk 6\n",
			threshold:      3,
			wantGrouped:    5, // 6-1 = 5 grouped
			wantContains:   []string{"Downloaded chunk 1", "[rtk:grouped ×6]"},
			wantNotContain: []string{"chunk 2", "chunk 3", "chunk 4", "chunk 5"},
		},

		// ── 2 lines below default threshold=3 → not merged ─────────────
		{
			name: "two_lines_below_default_threshold",
			input: "" +
				"Fetching step 1\n" +
				"Fetching step 2\n",
			threshold:      3,
			wantGrouped:    0,
			wantContains:   []string{"Fetching step 1", "Fetching step 2"},
			wantNotContain: []string{"[rtk:grouped"},
		},

		// ── threshold=2 merges 2 lines ─────────────────────────────────
		{
			name: "threshold_2_merges_two_lines",
			input: "" +
				"Item 1\n" +
				"Item 2\n",
			threshold:      2,
			wantGrouped:    1, // 2-1 = 1 grouped
			wantContains:   []string{"[rtk:grouped ×2]"},
			wantNotContain: nil,
		},

		// ── Hex variants 3 lines → merged ──────────────────────────────
		{
			name: "hex_variants_merged",
			input: "" +
				"Processing task a1b2c3d4\n" +
				"Processing task e5f6a7b8\n" +
				"Processing task c9d0e1f2\n",
			threshold:      3,
			wantGrouped:    2, // 3-1 = 2
			wantContains:   []string{"[rtk:grouped ×3]"},
			wantNotContain: nil,
		},

		// ── Timestamp variants 3 lines → merged ────────────────────────
		{
			name: "timestamp_variants_merged",
			input: "" +
				"[2024-01-01 10:00:00] Heartbeat received\n" +
				"[2024-01-01 10:00:05] Heartbeat received\n" +
				"[2024-01-01 10:00:10] Heartbeat received\n",
			threshold:      3,
			wantGrouped:    2, // 3-1 = 2
			wantContains:   []string{"[rtk:grouped ×3]"},
			wantNotContain: nil,
		},

		// ── Version variants 8 lines → merged ──────────────────────────
		{
			name: "version_variants_merged",
			input: "" +
				"Checking dependency v1.0.0\n" +
				"Checking dependency v2.0.0\n" +
				"Checking dependency v3.0.0\n" +
				"Checking dependency v4.0.0\n" +
				"Checking dependency v5.0.0\n" +
				"Checking dependency v6.0.0\n" +
				"Checking dependency v7.0.0\n" +
				"Checking dependency v8.0.0\n",
			threshold:      3,
			wantGrouped:    7, // 8-1 = 7
			wantContains:   []string{"[rtk:grouped ×8]"},
			wantNotContain: nil,
		},

		// ── threshold=1 clamped to 2 ───────────────────────────────────
		{
			name: "threshold_1_clamped_to_2_merges_2_lines",
			input: "" +
				"Step 1\n" +
				"Step 2\n",
			threshold:      1,
			wantGrouped:    1, // 2-1 = 1 (clamped to 2, so run 2 ≥ 2)
			wantContains:   []string{"[rtk:grouped ×2]"},
			wantNotContain: nil,
		},

		// ── Non-similar lines preserved ────────────────────────────────
		{
			name: "non_similar_lines_preserved",
			input: "" +
				"Downloading package 1\n" +
				"Downloading package 2\n" +
				"Downloading package 3\n" +
				"Build succeeded\n" +
				"Tests passed: 42\n",
			threshold:      3,
			wantGrouped:    2, // 3 lines "Downloading package N" → grouped=2
			wantContains:   []string{"Build succeeded", "Tests passed: 42", "[rtk:grouped ×3]"},
			wantNotContain: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GroupingOptions{Threshold: tt.threshold}
			result := groupSimilarLines(tt.input, opts)

			if result.Grouped != tt.wantGrouped {
				t.Errorf("groupSimilarLines(%q, threshold=%d).Grouped = %d, want %d",
					tt.input, tt.threshold, result.Grouped, tt.wantGrouped)
			}

			for _, substr := range tt.wantContains {
				if !contains(result.Text, substr) {
					t.Errorf("groupSimilarLines result should contain %q, got: %q",
						substr, result.Text)
				}
			}

			for _, substr := range tt.wantNotContain {
				if contains(result.Text, substr) {
					t.Errorf("groupSimilarLines result should NOT contain %q, got: %q",
						substr, result.Text)
				}
			}
		})
	}
}