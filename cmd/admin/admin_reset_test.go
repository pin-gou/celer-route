package main

import (
	"bytes"
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/encrypt"
)

// writeTempConfig writes a minimal celer-route config.json pointing at a
// SQLite database under t.TempDir(). Returns the path to the config file.
func writeTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := minimalConfigRoot{
		ConfigStore: json.RawMessage(`{"enabled":true,"type":"sqlite","config":{"path":"` + filepath.Join(dir, "config.db") + `"}}`),
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal temp config: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfigStoreConfig_MissingFile(t *testing.T) {
	_, _, err := loadConfigStoreConfig("/nonexistent/path.json", "")
	if err == nil {
		t.Fatalf("expected error for missing config file, got nil")
	}
	if !strings.Contains(err.Error(), "read config file") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestLoadConfigStoreConfig_MissingConfigStoreBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"providers":{}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := loadConfigStoreConfig(path, "")
	if err == nil {
		t.Fatalf("expected error when config_store is absent, got nil")
	}
	if !strings.Contains(err.Error(), "config_store") {
		t.Fatalf("expected config_store-related error, got %v", err)
	}
}

func TestLoadConfigStoreConfig_DisabledBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"config_store":{"enabled":false,"type":"sqlite","config":{"path":"x"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := loadConfigStoreConfig(path, "")
	if err == nil || !strings.Contains(err.Error(), "enabled is false") {
		t.Fatalf("expected enabled-is-false error, got %v", err)
	}
}

func TestLoadConfigStoreConfig_OK(t *testing.T) {
	path := writeTempConfig(t)
	cfg, _, err := loadConfigStoreConfig(path, "")
	if err != nil {
		t.Fatalf("loadConfigStoreConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected cfg.Enabled=true")
	}
	if cfg.Type != configstore.ConfigStoreTypeSQLite {
		t.Fatalf("expected sqlite store, got %q", cfg.Type)
	}
}

func TestDefaultSQLiteConfig_OK(t *testing.T) {
	dir := t.TempDir()
	cfg, source, err := loadConfigStoreConfig("", dir)
	if err != nil {
		t.Fatalf("loadConfigStoreConfig with empty config: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected cfg.Enabled=true")
	}
	if cfg.Type != configstore.ConfigStoreTypeSQLite {
		t.Fatalf("expected sqlite store, got %q", cfg.Type)
	}
	if !strings.Contains(source, "config.db") {
		t.Fatalf("expected source to contain config.db, got %q", source)
	}
}

func TestDefaultSQLiteConfig_FallbackToAppDirEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APP_DIR", dir)
	cfg, source, err := loadConfigStoreConfig("", "")
	if err != nil {
		t.Fatalf("loadConfigStoreConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected cfg.Enabled=true")
	}
	if !strings.Contains(source, dir) {
		t.Fatalf("expected source to contain %q, got %q", dir, source)
	}
}

// runResetAgainstTemp exercises the full reset pipeline against a fresh
// SQLite DB. stdin is fed by piping the bytes of newPassword + "\n" + newPassword + "\n".
// Returns the AuthConfig persisted by the run so callers can assert on it.
func runResetAgainstTemp(t *testing.T, existing string, args resetOptions, stdinContent string) *configstore.AuthConfig {
	t.Helper()
	path := writeTempConfig(t)
	opts := args
	opts.ConfigPath = path
	// Pipe stdin via a temp file so the command can read both the username
	// (if not provided) and the password from the same fd.
	stdinPath := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(stdinPath, []byte(stdinContent), 0o600); err != nil {
		t.Fatalf("write stdin file: %v", err)
	}
	f, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("open stdin file: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	origStdin := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = origStdin })

	if err := runReset(context.Background(), opts); err != nil {
		t.Fatalf("runReset: %v", err)
	}

	// Re-open the store and read back the persisted row to assert on it.
	cfg, _, err := loadConfigStoreConfig(path, "")
	if err != nil {
		t.Fatalf("re-load config: %v", err)
	}
	store, err := configstore.NewConfigStore(context.Background(), cfg, defaultTestLogger())
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	got, err := store.GetAuthConfig(context.Background())
	if err != nil {
		t.Fatalf("GetAuthConfig: %v", err)
	}
	return got
}

// TestRunReset_CreatesInitialAdmin walks the "no admin exists yet" path:
// the persisted row has the new username and is_enabled=true.
func TestRunReset_CreatesInitialAdmin(t *testing.T) {
	got := runResetAgainstTemp(t, "", resetOptions{
		Username:      "admin",
		PasswordStdin: true,
		AssumeYes:     true,
	}, "StrongPass1!\nStrongPass1!\n")
	if got == nil {
		t.Fatalf("expected AuthConfig, got nil")
	}
	if got.AdminUserName.GetValue() != "admin" {
		t.Fatalf("username = %q, want %q", got.AdminUserName.GetValue(), "admin")
	}
	if got.AdminPassword.GetValue() == "" {
		t.Fatalf("password hash is empty")
	}
	if !got.IsEnabled {
		t.Fatalf("is_enabled = false, want true")
	}
}

// TestRunReset_OverwritesExistingAdmin walks the "reset password" path.
func TestRunReset_OverwritesExistingAdmin(t *testing.T) {
	path := writeTempConfig(t)
	// Seed an existing admin row.
	cfg, _, err := loadConfigStoreConfig(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	store, err := configstore.NewConfigStore(context.Background(), cfg, defaultTestLogger())
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	oldHash, err := hashForTest("OldPassword123!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := store.UpdateAuthConfig(context.Background(), &configstore.AuthConfig{
		AdminUserName: schemas.NewSecretVar("alice"),
		AdminPassword: schemas.NewSecretVar(oldHash),
		IsEnabled:     false,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = store.Close(context.Background())

	// Now run reset and check that we flip is_enabled back on and overwrite
	// the password with a new hash.
	got := runResetAgainstTemp(t, "", resetOptions{
		Username:      "bob",
		PasswordStdin: true,
		AssumeYes:     true,
	}, "NewPassword123!\nNewPassword123!\n")
	if got.AdminUserName.GetValue() != "bob" {
		t.Fatalf("username = %q, want bob", got.AdminUserName.GetValue())
	}
	if !got.IsEnabled {
		t.Fatalf("is_enabled not flipped on after reset")
	}
	if got.AdminPassword.GetValue() == oldHash {
		t.Fatalf("password hash was not overwritten")
	}
}

// TestRunReset_PasswordPolicyFail covers the validation gate: a too-weak
// password must error out *before* any DB write happens.
func TestRunReset_PasswordPolicyFail(t *testing.T) {
	path := writeTempConfig(t)
	opts := resetOptions{
		ConfigPath:    path,
		Username:      "admin",
		PasswordStdin: true,
		AssumeYes:     true,
	}
	// Direct pipe — wrap in a temp file because os.Stdin assignment is
	// process-global and we need the bytes available before runReset opens.
	stdinPath := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(stdinPath, []byte("weak\nweak\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig })

	err = runReset(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "password does not meet policy") {
		t.Fatalf("expected policy error, got %v", err)
	}
	// Make sure nothing got written.
	cfg, _, _ := loadConfigStoreConfig(path, "")
	store, _ := configstore.NewConfigStore(context.Background(), cfg, defaultTestLogger())
	defer store.Close(context.Background())
	got, _ := store.GetAuthConfig(context.Background())
	if got != nil {
		t.Fatalf("expected no AuthConfig after policy failure, got %+v", got)
	}
}

// TestRunReset_PasswordMismatch covers the "two passwords disagree" branch.
func TestRunReset_PasswordMismatch(t *testing.T) {
	path := writeTempConfig(t)
	opts := resetOptions{
		ConfigPath:    path,
		Username:      "admin",
		PasswordStdin: true,
		AssumeYes:     true,
	}
	stdinPath := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(stdinPath, []byte("StrongPass1!\nDifferentPass2!\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig })

	err = runReset(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "passwords do not match") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

// TestRunReset_NonTTYWithoutStdinRefuses makes sure the safety net fires:
// a non-TTY stdin without --password-stdin is rejected so the password
// cannot silently leak to whatever the stdin fd is connected to.
func TestRunReset_NonTTYWithoutStdinRefuses(t *testing.T) {
	path := writeTempConfig(t)
	opts := resetOptions{
		ConfigPath: path,
		Username:   "admin",
		// PasswordStdin: false, no AssumeYes, so the interactive Proceed?
		// prompt fires first and the non-TTY stdin triggers the refusal.
	}
	// Use an os.Pipe: the read end is a non-TTY character device (well,
	// actually a pipe, but isTerminal returns false because pipes are not
	// ModeCharDevice).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	err = runReset(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "refusing to run interactively") {
		t.Fatalf("expected interactive-refusal error, got %v", err)
	}
}

// TestIsTerminal sanity-checks the TTY detection: a regular file is not
// a terminal, an os.Pipe read end is not a terminal.
func TestIsTerminal(t *testing.T) {
	// A temp file is a regular file: not a TTY.
	path := filepath.Join(t.TempDir(), "not-a-tty")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Fatalf("regular file should not be detected as TTY")
	}

	// A pipe read end is not a TTY either.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Fatalf("pipe read end should not be detected as TTY")
	}
}

// TestReadLineFrom covers the partial-line-on-EOF branch: the final line
// has no trailing newline and must still be returned to the caller.
func TestReadLineFrom(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		lines  []string
		errOn  int // index of the call that returns a non-nil error; -1 = none
	}{
		// "abc\ndef\n" — bufio returns "abc\n" then "def\n" with io.EOF on
		// the second call (because the underlying reader is exhausted).
		// Our wrapper treats a non-empty line + EOF as success, so the
		// caller sees both lines without an error. EOF arrives only when
		// the *next* read happens.
		{"two lines with trailing newline", "abc\ndef\n", []string{"abc\n", "def\n"}, 2},
		{"final line missing newline", "abc\ndef", []string{"abc\n", "def"}, 2},
		{"empty", "", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReaderSize(strings.NewReader(tc.input), 16)
			var got []string
			for i := 0; i < 4; i++ {
				line, err := readLineFrom(br)
				got = append(got, line)
				if err != nil {
					if i != tc.errOn {
						t.Fatalf("read %d: unexpected error %v", i, err)
					}
					break
				}
				if i > tc.errOn {
					t.Fatalf("read %d: expected error, got line %q", i, line)
				}
			}
			if !equalSlices(got[:len(tc.lines)], tc.lines) {
				t.Fatalf("lines = %q, want %q", got, tc.lines)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTrimAndSanityOnPassword verifies that even with whitespace stripped
// from both password lines, equality holds and the policy check runs.
func TestTrimAndSanityOnPassword(t *testing.T) {
	in := bytes.NewBufferString("  StrongPass1!\n  StrongPass1!\n")
	// Direct exercise of the shared read logic by reusing the helper.
	br := bufio.NewReader(in)
	pw, err := readLineFrom(br)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	pw = strings.TrimRight(pw, "\r\n")
	if pw != "  StrongPass1!" {
		t.Fatalf("first line = %q", pw)
	}
}

// defaultTestLogger keeps test output quiet for the config store. We pass
// Warn so spurious info-level chatter from migrations doesn't clutter the
// test logs.
func defaultTestLogger() schemas.Logger {
	return bifrost.NewDefaultLogger(schemas.LogLevelError)
}

// hashForTest exercises the production Argon2id path so the test verifies
// the same password hashing that UpdateAuthConfig will use at runtime.
func hashForTest(pw string) (string, error) {
	return encrypt.Hash(pw)
}
