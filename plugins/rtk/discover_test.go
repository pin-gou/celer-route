package rtk

import (
	"reflect"
	"testing"
)

// TestDiscoverNormalizeLine verifies the extended 3-step normalization on top
// of grouper.normalizeLine, aligned with OmniRoute discover.ts
// discoverNormalizeLine:
//
//	1-7  grouper.normalizeLine (ISO ts → bracketed dt → hex → semver → int → collapse → trim)
//	8    npm/pip package identifiers with version:  left-pad@1.2.3 → <PKG>@<N>
//	9    Error/exit codes:                           E404 / ENOENT / E2BIG → <CODE>
//	10   Numeric suffixes (time/size units):         5s / 120ms / 4kb → <N>
//	+    whitespace collapse + trim (again after substitutions)
//
// TDD red phase: DiscoverNormalizeLine does not exist yet (compile error).
func TestDiscoverNormalizeLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// ── Step 8: <PKG>@<N> (package name + version collapse) ──────────
		{
			name:  "npm_install_package_version",
			input: "npm install left-pad@1.2.3",
			want:  "npm install <PKG>@<N>",
		},
		{
			name:  "different_packages_collapse_alike",
			input: "npm install lodash@4.17.21",
			want:  "npm install <PKG>@<N>",
		},
		{
			name:  "package_after_iso_timestamp",
			input: "2024-01-15T10:30:00Z left-pad@1.2.3",
			want:  "<N> <PKG>@<N>",
		},
		{
			name:  "package_inline_middle",
			input: "installing left-pad@1.2.3 from registry",
			want:  "installing <PKG>@<N> from registry",
		},

		// ── Step 9: <CODE> (error/exit codes) ─────────────────────────────
		{
			name:  "code_enoint",
			input: "code: ENOENT",
			want:  "code: <CODE>",
		},
		{
			name:  "error_numeric_code",
			input: "Error: E404",
			want:  "Error: <CODE>",
		},
		{
			name:  "exit_code_e2big",
			input: "exit code E2BIG",
			want:  "exit code <CODE>",
		},

		// ── Step 10: <N> time/size units ──────────────────────────────────
		{
			name:  "unit_seconds",
			input: "Done in 5s",
			want:  "Done in <N>",
		},
		{
			name:  "unit_milliseconds",
			input: "Downloaded in 120ms",
			want:  "Downloaded in <N>",
		},
		{
			name:  "unit_kilobytes",
			input: "Size: 4kb",
			want:  "Size: <N>",
		},
		{
			name:  "unit_megabytes",
			input: "Uploaded 12MB",
			want:  "Uploaded <N>",
		},

		// ── Whitespace collapse after substitutions ───────────────────────
		{
			name:  "whitespace_collapsed",
			input: "  npm install   lodash@4.17.21  ",
			want:  "npm install <PKG>@<N>",
		},

		// ── grouper reuse (7-step contract) ───────────────────────────────
		{
			name:  "grouper_reuse_timestamp_hex_int",
			input: "2024-01-15T10:30:00Z task 42 ab12cd",
			want:  "<N> task <N> <N>",
		},

		// ── Composite / passthrough / edge ────────────────────────────────
		{
			name:  "package_and_unit_combo",
			input: "left-pad@1.2.3 in 5s",
			want:  "<PKG>@<N> in <N>",
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
			got := DiscoverNormalizeLine(tt.input)
			if got != tt.want {
				t.Errorf("DiscoverNormalizeLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDiscoverRepeatedNoise verifies cross-sample aggregation semantics from
// OmniRoute discover.ts discoverRepeatedNoise:
//
//   - a normalised template must recur in >1 samples to be surfaced (single-sample
//     noise is filtered out);
//   - each normalised form is counted once per sample;
//   - the resulting pattern is regex-safe (special chars escaped) with a leading
//     ^ anchor;
//   - results are sorted by hits desc, then pattern asc for deterministic output;
//   - empty / whitespace-only sample sets yield no candidates.
//
// TDD red phase: DiscoverRepeatedNoise and CommandSample/NoiseCandidate do not
// exist yet (compile error).
func TestDiscoverRepeatedNoise(t *testing.T) {
	t.Run("hits_desc_ordering", func(t *testing.T) {
		samples := []CommandSample{
			{Command: "npm install", Output: "npm warn EBADENGINE 6\nCompiling src 1\n"},
			{Command: "npm install", Output: "npm warn EBADENGINE 7\nCompiling src 2\n"},
			{Command: "npm install", Output: "npm warn EBADENGINE 8\n"},
		}
		want := []NoiseCandidate{
			{Pattern: "^npm warn [A-Z][A-Z0-9]+ [\\S]+", Hits: 3},
			{Pattern: "^Compiling src [\\S]+", Hits: 2},
		}
		got := DiscoverRepeatedNoise(samples)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("DiscoverRepeatedNoise(hits_desc) = %+v, want %+v", got, want)
		}
	})

	t.Run("single_sample_filtered", func(t *testing.T) {
		samples := []CommandSample{
			{Command: "npm install", Output: "npm warn EBADENGINE 6\nadding line 42\n"},
		}
		if got := DiscoverRepeatedNoise(samples); len(got) != 0 {
			t.Errorf("single-sample input should yield no candidates, got %+v", got)
		}
	})

	t.Run("empty_and_whitespace_samples", func(t *testing.T) {
		if got := DiscoverRepeatedNoise(nil); len(got) != 0 {
			t.Errorf("nil samples should yield no candidates, got %+v", got)
		}
		if got := DiscoverRepeatedNoise([]CommandSample{{Command: "x", Output: "   \n\t\n"}}); len(got) != 0 {
			t.Errorf("whitespace-only sample should yield no candidates, got %+v", got)
		}
	})

	t.Run("pattern_asc_same_hits", func(t *testing.T) {
		samples := []CommandSample{
			{Command: "x", Output: "alpha 1\nbeta line 2\n"},
			{Command: "x", Output: "alpha 3\nbeta line 4\n"},
		}
		want := []NoiseCandidate{
			{Pattern: "^alpha [\\S]+", Hits: 2},
			{Pattern: "^beta line [\\S]+", Hits: 2},
		}
		got := DiscoverRepeatedNoise(samples)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("DiscoverRepeatedNoise(pattern_asc) = %+v, want %+v", got, want)
		}
	})

	t.Run("pattern_special_chars_escaped", func(t *testing.T) {
		samples := []CommandSample{
			{Command: "x", Output: "Downloaded chunk (1 of 3)\n"},
			{Command: "x", Output: "Downloaded chunk (4 of 7)\n"},
		}
		want := []NoiseCandidate{
			{Pattern: "^Downloaded chunk \\([\\S]+ of [\\S]+\\)", Hits: 2},
		}
		got := DiscoverRepeatedNoise(samples)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("DiscoverRepeatedNoise(escaping) = %+v, want %+v", got, want)
		}
	})
}