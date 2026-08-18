package rtk

// Stage-4 raw output persistence tests (TDD red phase):
//   - TestRedactRtkRawOutput            (V-plugins-2) 5 条密钥脱敏正则
//   - TestIsLikelyFailureOutput         (V-plugins-3) 失败关键词检测
//   - TestRetentionPolicies             (V-plugins-1) never/failures/always 三策略
//   - TestSidecarMetadata               (V-plugins-4) .meta.json 五字段
//   - TestDiskErrorGracefulDegradation  (V-plugins-5) EACCES best-effort 降级
//
// 注意：PersistOptions 追加了 AppDir 字段——设计文档 D2 决策的落盘根目录
// <appDir>/rtk/raw-output/ 必须可由测试注入 t.TempDir() 才能断言文件落盘
// 行为（V-plugins-1/4/5 的 t.TempDir() 前置依赖）。dev 阶段实现 PersistOptions
// 时必须提供该字段。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestRedactRtkRawOutput (V-plugins-2) verifies the 5 secret-redaction
// patterns replace keys with placeholders and set redacted=true:
//   (a) OpenAI key   sk-...            → [REDACTED_OPENAI_KEY]
//   (b) Slack token  xox[a|b|p|r|s]-... → [REDACTED_SLACK_TOKEN]
//   (c) AWS key      AKIA...          → [REDACTED_AWS_KEY]
//   (d) credential field key=value / key: value → 保留 key 名, value → [REDACTED]
//   (e) (Proxy-)Authorization: Bearer|Basic → 保留前缀, 后 → [REDACTED]
func TestRedactRtkRawOutput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantText    string
		wantRedated bool
	}{
		{
			name:        "openai_key",
			input:       "sk-abc123DEF456ghi789JKL012mno345", // ≥16 chars
			wantText:    "[REDACTED_OPENAI_KEY]",
			wantRedated: true,
		},
		{
			name:        "slack_token",
			input:       "xoxb-12345678901234567890", // xoxb- + 20 chars
			wantText:    "[REDACTED_SLACK_TOKEN]",
			wantRedated: true,
		},
		{
			name:        "aws_key",
			input:       "AKIAIOSFODNN7EXAMPLE", // AKIA + 16 alnum
			wantText:    "[REDACTED_AWS_KEY]",
			wantRedated: true,
		},
		{
			name:        "credential_field_key_value",
			input:       "api_key=supersecret123",
			wantText:    "api_key=[REDACTED]",
			wantRedated: true,
		},
		{
			name:        "credential_field_colon_quoted",
			input:       `token: "magic-token-value"`,
			wantText:    `token: [REDACTED]`,
			wantRedated: true,
		},
		{
			name:        "authorization_bearer",
			input:       "Authorization: Bearer s4mtok3nv4lue",
			wantText:    "Authorization: Bearer [REDACTED]",
			wantRedated: true,
		},
		{
			name:        "proxy_authorization_basic",
			input:       "Proxy-Authorization: Basic dXNlcjpwYXNz",
			wantText:    "Proxy-Authorization: Basic [REDACTED]",
			wantRedated: true,
		},
		{
			name:        "mixed_content_redacts_each_secret",
			input:       "using sk-abcDEFghiJKLmnoPQRS1234uvwxyz5678 and AKIAIOSFODNN7EXAMPLE",
			wantRedated: true,
		},
		{
			name:        "clean_output_unchanged",
			input:       "all checks passed, 12 files changed",
			wantText:    "all checks passed, 12 files changed",
			wantRedated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, redacted := RedactRtkRawOutput(tt.input)
			if redacted != tt.wantRedated {
				t.Errorf("RedactRtkRawOutput(%q) redacted=%v, want %v", tt.input, redacted, tt.wantRedated)
			}
			if tt.wantText != "" && text != tt.wantText {
				t.Errorf("RedactRtkRawOutput(%q) text=%q, want %q", tt.input, text, tt.wantText)
			}
		})
	}
}

// TestIsLikelyFailureOutput (V-plugins-3) verifies the 9 failure keywords
// (error/failed/failure/exception/traceback/panic/fatal/critical/TS\d{4}/FAIL)
// are recognised as likely-failure output while normal output is not.
func TestIsLikelyFailureOutput(t *testing.T) {
	keywords := []struct {
		name string
		text string
	}{
		{"keyword_error", "build error: cannot compile"},
		{"keyword_failed", "the command failed"},
		{"keyword_failure", "connection failure detected"},
		{"keyword_exception", "unexpected exception thrown"},
		{"keyword_traceback", "traceback occurred in main"},
		{"keyword_panic", "panic: runtime error"},
		{"keyword_fatal", "fatal: not a git repository"},
		{"keyword_critical", "critical: disk full"},
		{"keyword_ts_error_code", "error TS1234: type mismatch"},
		{"keyword_fail_uppercase", "BUILD FAIL: step 1 error"},
	}

	for _, kw := range keywords {
		t.Run(kw.name, func(t *testing.T) {
			if !IsLikelyFailureOutput(kw.text) {
				t.Errorf("IsLikelyFailureOutput(%q) = false, want true", kw.text)
			}
		})
	}

	negative := []struct {
		name string
		text string
	}{
		{"normal_output", "All checks succeeded, 12 files changed"},
		{"success_output", "Build succeeded in 3.2s"},
		{"word_boundary_no_partial_match", "errorsome output here"},
	}

	for _, neg := range negative {
		t.Run(neg.name, func(t *testing.T) {
			if IsLikelyFailureOutput(neg.text) {
				t.Errorf("IsLikelyFailureOutput(%q) = true, want false", neg.text)
			}
		})
	}
}

// rawOutputDirFor returns the raw-output directory under the given appDir
// (the <appDir>/rtk/raw-output/ root from design D2).
func rawOutputDirFor(appDir string) string {
	return filepath.Join(appDir, "rtk", "raw-output")
}

// countLogFiles returns the number of *.log files in the raw-output dir
// (0 when the dir does not exist).
func countLogFiles(t *testing.T, appDir string) int {
	t.Helper()
	dir := rawOutputDirFor(appDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("failed to read raw-output dir %s: %v", dir, err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			n++
		}
	}
	return n
}

// TestRetentionPolicies (V-plugins-1) verifies the three retention strategies
// take effect per configuration and produced filenames strictly follow the
// <ts_ms>-<slug>-<id24>.log template.
func TestRetentionPolicies(t *testing.T) {
	rawFilename := regexp.MustCompile(`^\d{13}-[A-Za-z0-9_-]+-[0-9a-f]{24}\.log$`)

	t.Run("retention_never_writes_nothing", func(t *testing.T) {
		appDir := t.TempDir()
		ptr := MaybePersistRtkRawOutput("some output", PersistOptions{
			Retention: RawOutputRetentionNever,
			Command:   "test",
			MaxBytes:  1048576,
			AppDir:    appDir,
		})
		if ptr != nil {
			t.Errorf("retention=never: expected nil pointer, got non-nil %+v", ptr)
		}
		if n := countLogFiles(t, appDir); n != 0 {
			t.Errorf("retention=never: expected 0 files, got %d", n)
		}
	})

	t.Run("retention_failures_persists_failure_output", func(t *testing.T) {
		appDir := t.TempDir()
		ptr := MaybePersistRtkRawOutput("error: something failed", PersistOptions{
			Retention: RawOutputRetentionFailures,
			Command:   "test",
			MaxBytes:  1048576,
			AppDir:    appDir,
		})
		if ptr == nil {
			t.Fatal("retention=failures: expected non-nil pointer for failure output")
		}
		if n := countLogFiles(t, appDir); n != 1 {
			t.Errorf("retention=failures + failure output: expected 1 file, got %d", n)
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("pointer path not persisted: %v", err)
		}
	})

	t.Run("retention_failures_skips_ok_output", func(t *testing.T) {
		appDir := t.TempDir()
		ptr := MaybePersistRtkRawOutput("all good", PersistOptions{
			Retention: RawOutputRetentionFailures,
			Command:   "test",
			MaxBytes:  1048576,
			AppDir:    appDir,
		})
		if ptr != nil {
			t.Errorf("retention=failures + ok output: expected nil pointer, got non-nil %+v", ptr)
		}
		if n := countLogFiles(t, appDir); n != 0 {
			t.Errorf("retention=failures + ok output: expected 0 files, got %d", n)
		}
	})

	t.Run("retention_failures_force_persist_via_failure_override", func(t *testing.T) {
		appDir := t.TempDir()
		forced := true
		ptr := MaybePersistRtkRawOutput("all good", PersistOptions{
			Retention: RawOutputRetentionFailures,
			Command:   "test",
			MaxBytes:  1048576,
			Failure:   &forced,
			AppDir:    appDir,
		})
		if ptr == nil {
			t.Fatal("retention=failures + Failure override: expected non-nil pointer")
		}
		if n := countLogFiles(t, appDir); n != 1 {
			t.Errorf("retention=failures + Failure override: expected 1 file, got %d", n)
		}
	})

	t.Run("retention_always_persists_all", func(t *testing.T) {
		appDir := t.TempDir()
		ptr := MaybePersistRtkRawOutput("some output", PersistOptions{
			Retention: RawOutputRetentionAlways,
			Command:   "git status",
			MaxBytes:  1048576,
			AppDir:    appDir,
		})
		if ptr == nil {
			t.Fatal("retention=always: expected non-nil pointer")
		}
		if n := countLogFiles(t, appDir); n != 1 {
			t.Errorf("retention=always: expected 1 file, got %d", n)
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("pointer path not persisted: %v", err)
		}

		// Sidecar .meta.json must accompany the .log.
		sidecar := ptr.Path[:len(ptr.Path)-len(".log")] + ".meta.json"
		if _, err := os.Stat(sidecar); err != nil {
			t.Errorf("sidecar not written: %v", err)
		}
	})

	t.Run("retention_always_skips_blank_raw", func(t *testing.T) {
		appDir := t.TempDir()
		ptr := MaybePersistRtkRawOutput("   \n\t", PersistOptions{
			Retention: RawOutputRetentionAlways,
			Command:   "test",
			MaxBytes:  1048576,
			AppDir:    appDir,
		})
		if ptr != nil {
			t.Errorf("retention=always + blank raw: expected nil pointer, got non-nil %+v", ptr)
		}
		if n := countLogFiles(t, appDir); n != 0 {
			t.Errorf("retention=always + blank raw: expected 0 files, got %d", n)
		}
	})

	t.Run("filename_follows_template", func(t *testing.T) {
		appDir := t.TempDir()
		ptr := MaybePersistRtkRawOutput("some output", PersistOptions{
			Retention: RawOutputRetentionAlways,
			Command:   "git status",
			MaxBytes:  1048576,
			AppDir:    appDir,
		})
		if ptr == nil {
			t.Fatal("expected non-nil pointer")
		}
		entries, err := os.ReadDir(rawOutputDirFor(appDir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".log" && !rawFilename.MatchString(e.Name()) {
				t.Errorf("filename %q does not match <ts_ms>-<slug>-<id24>.log template", e.Name())
			}
		}
	})
}

// TestSidecarMetadata (V-plugins-4) verifies the .meta.json sidecar carries
// command/timestamp/failure/redacted/bytes.
func TestSidecarMetadata(t *testing.T) {
	appDir := t.TempDir()
	ptr := MaybePersistRtkRawOutput("output content", PersistOptions{
		Retention: RawOutputRetentionAlways,
		Command:   "git status",
		MaxBytes:  1048576,
		AppDir:    appDir,
	})
	if ptr == nil {
		t.Fatal("expected non-nil pointer")
	}

	sidecar := ptr.Path[:len(ptr.Path)-len(".log")] + ".meta.json"
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("failed to read sidecar %s: %v", sidecar, err)
	}

	var meta struct {
		Command   string `json:"command"`
		Timestamp int64  `json:"timestamp"`
		Failure   bool   `json:"failure"`
		Redacted  bool   `json:"redacted"`
		Bytes     int    `json:"bytes"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("sidecar is not valid JSON: %v", err)
	}

	if meta.Command != "git status" {
		t.Errorf("sidecar command = %q, want %q", meta.Command, "git status")
	}
	if meta.Timestamp <= 0 {
		t.Errorf("sidecar timestamp = %d, want > 0 (unix ms)", meta.Timestamp)
	}
	if meta.Failure {
		t.Errorf("sidecar failure = true, want false for non-failure output")
	}
	if meta.Redacted {
		t.Errorf("sidecar redacted = true, want false for output without secrets")
	}
	if want := len("output content"); meta.Bytes != want {
		t.Errorf("sidecar bytes = %d, want %d", meta.Bytes, want)
	}

	// Pointer metadata should agree with the sidecar.
	if ptr.Redacted {
		t.Errorf("pointer.Redacted = true, want false")
	}
}

// TestDiskErrorGracefulDegradation (V-plugins-5) verifies that a disk-level
// EACCES failure makes MaybePersistRtkRawOutput return nil without panicking,
// so the compression pipeline treats the miss as best-effort.
func TestDiskErrorGracefulDegradation(t *testing.T) {
	if os.Geteuid() == 0 {
		// Running as root bypasses file permissions — EACCES cannot be
		// simulated (documented degradation; verified in Linux CI non-root).
		t.Skip("running as root: chmod 000 does not produce EACCES, skipping")
	}

	appDir := t.TempDir()
	rawDir := rawOutputDirFor(appDir)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(rawDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(rawDir, 0o700) // restore so t.TempDir cleanup succeeds

	ptr := MaybePersistRtkRawOutput("some output", PersistOptions{
		Retention: RawOutputRetentionAlways,
		Command:   "test",
		MaxBytes:  1048576,
		AppDir:    appDir,
	})
	if ptr != nil {
		t.Errorf("expected nil pointer on EACCES, got non-nil %+v", ptr)
	}
	// Reaching this point without a panic is the second half of the contract.
}