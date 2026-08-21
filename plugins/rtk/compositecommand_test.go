package rtk

import "testing"

// TestLastCommandSegment verifies that lastCommandSegment correctly extracts
// the last meaningful segment from composite shell commands.
//
// Test cases cover:
//   - Single command (no separator) -> unchanged
//   - Composite && chains -> last segment
//   - Composite || chains -> last segment
//   - Composite ; chains -> last segment
//   - Quote protection (single, double, backtick)
//   - $(...) subshell protection
//   - Nested parentheses
//   - Trailing empty segment -> fallback to previous
//   - Empty input -> unchanged
//   - Mixed separators
func TestLastCommandSegment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// ── Single command (no separator) ────────────────────────────────
		{
			name:  "single command",
			input: "npm run build",
			want:  "npm run build",
		},
		{
			name:  "single command with args",
			input: "go test ./... -v -count=1",
			want:  "go test ./... -v -count=1",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},

		// ── Composite && chains ──────────────────────────────────────────
		{
			name:  "composite &&",
			input: "cd frontend && npm run build",
			want:  "npm run build",
		},
		{
			name:  "triple &&",
			input: "git add . && git commit -m 'fix' && git push",
			want:  "git push",
		},
		{
			name:  "&& with args containing dashes",
			input: "cd /tmp && go test ./... -race",
			want:  "go test ./... -race",
		},

		// ── Composite || chains ──────────────────────────────────────────
		{
			name:  "composite ||",
			input: "false || npm run test",
			want:  "npm run test",
		},
		{
			name:  "mixed && and ||",
			input: "cd src && npm run build || npm run dev",
			want:  "npm run dev",
		},

		// ── Composite ; chains ───────────────────────────────────────────
		{
			name:  "composite semicolon",
			input: "cd src; ls -la",
			want:  "ls -la",
		},

		// ── Single quote protection ──────────────────────────────────────
		{
			name:  "single quotes protect &&",
			input: "echo 'a && b'",
			want:  "echo 'a && b'",
		},
		{
			name:  "single quotes protect ;",
			input: "echo 'a; b'",
			want:  "echo 'a; b'",
		},
		{
			name:  "single quotes with real command",
			input: "git commit -m 'fix: resolve && issue' && git push",
			want:  "git push",
		},

		// ── Double quote protection ──────────────────────────────────────
		{
			name:  "double quotes protect &&",
			input: `echo "a && b"`,
			want:  `echo "a && b"`,
		},
		{
			name:  "double quotes protect ||",
			input: `echo "a || b"`,
			want:  `echo "a || b"`,
		},
		{
			name:  "double quotes with real command",
			input: `echo "hello world" && npm run test`,
			want:  "npm run test",
		},

		// ── Backtick protection ──────────────────────────────────────────
		{
			name:  "backtick subshell protects &&",
			input: "echo `date && echo inner` && npm run build",
			want:  "npm run build",
		},
		{
			name:  "backtick alone",
			input: "echo `date`",
			want:  "echo `date`",
		},

		// ── $(...) subshell protection ───────────────────────────────────
		{
			name:  "$() subshell protects &&",
			input: "echo $(ls || pwd) && npm run build",
			want:  "npm run build",
		},
		{
			name:  "$() subshell alone",
			input: "echo $(date)",
			want:  "echo $(date)",
		},
		{
			name:  "nested $() with separators",
			input: "echo $(echo 'a && b' || echo c) && npm run test",
			want:  "npm run test",
		},

		// ── Nested parentheses ───────────────────────────────────────────
		{
			name:  "parentheses without $",
			input: "(echo a && echo b) && npm run build",
			want:  "npm run build",
		},

		// ── Trailing / empty segment fallback ────────────────────────────
		{
			name:  "trailing && falls back to previous",
			input: "cd frontend &&",
			want:  "cd frontend",
		},
		{
			name:  "trailing || falls back to previous",
			input: "npm run build ||",
			want:  "npm run build",
		},

		// ── Edge cases ───────────────────────────────────────────────────
		{
			name:  "only whitespace between separators",
			input: "cmd1 &&   && cmd2",
			want:  "cmd2",
		},
		{
			name:  "complex real-world chain",
			input: "cd frontend && npm run build 2>&1 | tee build.log",
			want:  "npm run build 2>&1 | tee build.log",
		},
		{
			name:  "pipelines are not separators",
			input: "cat file | grep foo | sort",
			want:  "cat file | grep foo | sort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastCommandSegment(tt.input)
			if got != tt.want {
				t.Errorf("lastCommandSegment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestLastCommandSegmentEdgeCases verifies edge conditions that are not
// covered by the main table-driven test.
func TestLastCommandSegmentEdgeCases(t *testing.T) {
	// Whitespace-only input should return itself.
	if got := lastCommandSegment("   "); got != "   " {
		t.Errorf("lastCommandSegment('   ') = %q, want '   '", got)
	}
	// Single-character command.
	if got := lastCommandSegment("x"); got != "x" {
		t.Errorf("lastCommandSegment('x') = %q, want 'x'", got)
	}
	// Command with only separators.
	if got := lastCommandSegment("&&"); got != "&&" {
		t.Errorf("lastCommandSegment('&&') = %q, want '&&'", got)
	}
	// Mixed quotes.
	if got := lastCommandSegment(`echo "don't split" && npm run test`); got != "npm run test" {
		t.Errorf("lastCommandSegment mixed quotes = %q, want 'npm run test'", got)
	}
}

// TestLastCommandSegmentNoSplit verifies that commands without any top-level
// separator are returned unchanged.
func TestLastCommandSegmentNoSplit(t *testing.T) {
	inputs := []string{
		"npm run build",
		"go test ./...",
		"docker compose up -d",
		"git status",
		"python -m pytest tests/",
		"cat file | grep foo | sort",
		"echo hello",
	}
	for _, input := range inputs {
		got := lastCommandSegment(input)
		if got != input {
			t.Errorf("lastCommandSegment(%q) = %q, want %q (no separator)", input, got, input)
		}
	}
}