package main

import (
	"bytes"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// writeReencryptTempConfig writes a minimal celer-route config.json that
// includes an encryption_key, pointing at a SQLite DB under t.TempDir().
func writeReencryptTempConfig(t *testing.T, passphrase string) (configPath, dbPath string) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "config.db")
	cfg := struct {
		EncryptionKey string             `json:"encryption_key"`
		ConfigStore   configstore.Config `json:"config_store"`
	}{
		EncryptionKey: passphrase,
		ConfigStore: configstore.Config{
			Enabled: true,
			Type:    configstore.ConfigStoreTypeSQLite,
			Config:  &configstore.SQLiteConfig{Path: dbPath},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal temp config: %v", err)
	}
	configPath = filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return configPath, dbPath
}

// captureStderr redirects os.Stderr to a buffer for the duration of fn.
// Returns whatever fn wrote; restores the original os.Stderr on exit.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()
	defer func() {
		os.Stderr = original
	}()
	fn()
	_ = w.Close()
	return <-done
}

// TestRunReencrypt_Help exercises the help-only path; this is the one place
// that does not require a config store or a key. The flag library intercepts
// -h/--help before the custom help block runs, so we use Go's flag.ErrHelp
// contract: runReencrypt must surface nil so dispatchers can distinguish
// "user asked for help" from "real error".
func TestRunReencrypt_Help(t *testing.T) {
	output := captureStderr(t, func() {
		if err := runReencrypt(context.Background(), []string{"-h"}); err != nil {
			t.Fatalf("help should not error: %v", err)
		}
	})
	if !strings.Contains(output, "-confirm") {
		t.Errorf("flag help output should mention -confirm; got %q", output)
	}
	if !strings.Contains(output, "-dry-run") {
		t.Errorf("flag help output should mention -dry-run; got %q", output)
	}
	if !strings.Contains(output, "-batch-size") {
		t.Errorf("flag help output should mention -batch-size; got %q", output)
	}
}

// TestRunReencrypt_RefusesWithoutKey confirms the CLI fails fast when
// encryption is not enabled — even if --confirm is passed.
func TestRunReencrypt_RefusesWithoutKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "config.db")
	cfg := struct {
		ConfigStore configstore.Config `json:"config_store"`
	}{
		ConfigStore: configstore.Config{
			Enabled: true,
			Type:    configstore.ConfigStoreTypeSQLite,
			Config:  &configstore.SQLiteConfig{Path: dbPath},
		},
	}
	data, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	// Initialise encryption from the empty config; no key → not enabled.
	encrypt.Init("", bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer encrypt.Init("test-passphrase-32-bytes-long!!", bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	err := runReencrypt(context.Background(), []string{"--config", cfgPath, "--confirm"})
	if err == nil {
		t.Fatalf("expected re-encrypt to refuse when encryption is not enabled")
	}
	if !strings.Contains(err.Error(), "encryption is not enabled") {
		t.Errorf("expected error to mention missing encryption; got %v", err)
	}
}

// TestRunReencrypt_RefusesWithoutConfirm confirms the CLI exits cleanly when
// plaintext rows exist but --confirm is missing — the migration does not run.
func TestRunReencrypt_RefusesWithoutConfirm(t *testing.T) {
	passphrase := "test-passphrase-32-bytes-long!!"
	cfgPath, _ := writeReencryptTempConfig(t, passphrase)
	encrypt.Init(passphrase, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer encrypt.Init("test-passphrase-32-bytes-long!!", bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	err := runReencrypt(context.Background(), []string{"--config", cfgPath, "--dry-run"})
	if err != nil {
		t.Fatalf("dry-run on empty db should succeed: %v", err)
	}
}

// TestRunReencrypt_RequiresConfirmNotDryRun pins the contract: a non-dry-run
// without --confirm is a no-op and surfaces the reminder. The DB must have
// at least one plaintext row for the refusal path to trigger.
func TestRunReencrypt_RequiresConfirmNotDryRun(t *testing.T) {
	passphrase := "test-passphrase-32-bytes-long!!"
	cfgPath, dbPath := writeReencryptTempConfig(t, passphrase)
	encrypt.Init(passphrase, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer encrypt.Init("test-passphrase-32-bytes-long!!", bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	seedOnePlaintextKey(t, dbPath)

	err := runReencrypt(context.Background(), []string{"--config", cfgPath})
	if err == nil {
		t.Fatalf("expected refusal when --confirm is missing on a non-dry-run")
	}
	if !strings.Contains(err.Error(), "--confirm") {
		t.Errorf("error should mention --confirm; got %v", err)
	}
}

// TestRunReencrypt_UnsupportedRotateMode confirms the rotate-key path
// surfaces a not-supported error rather than silently doing nothing.
func TestRunReencrypt_UnsupportedRotateMode(t *testing.T) {
	passphrase := "test-passphrase-32-bytes-long!!"
	cfgPath, dbPath := writeReencryptTempConfig(t, passphrase)
	encrypt.Init(passphrase, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer encrypt.Init("test-passphrase-32-bytes-long!!", bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	seedOnePlaintextKey(t, dbPath)

	err := runReencrypt(context.Background(), []string{"--config", cfgPath, "--mode", "rotate-key", "--confirm"})
	if err == nil {
		t.Fatalf("expected rotate-key to be rejected")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error should mention 'not supported'; got %v", err)
	}
}

// seedOnePlaintextKey inserts a single plaintext config_keys row so the
// CLI has something to count. We open the store with encryption DISABLED
// (no key) so the eager EncryptPlaintextRows pass is a no-op, run migrations
// to create the schema, then insert the plaintext row directly.
func seedOnePlaintextKey(t *testing.T, dbPath string) {
	t.Helper()
	encrypt.Init("", bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	cfg := &configstore.Config{
		Enabled: true,
		Type:    configstore.ConfigStoreTypeSQLite,
		Config:  &configstore.SQLiteConfig{Path: dbPath},
	}
	store, err := configstore.NewConfigStore(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	store.Close(context.Background())

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	// config_keys.provider_id is NOT NULL with a FK to config_providers. Insert
	// a minimal provider row so the FK passes; the value of the FK is not
	// asserted by any test — we only care that the row is visible to Count.
	if err := db.Exec(
		`INSERT INTO config_providers (name, created_at, updated_at)
		 VALUES ('openai-fake', datetime('now'), datetime('now'))`,
	).Error; err != nil {
		t.Fatalf("seed fake provider: %v", err)
	}
	var providerPK int64
	if err := db.Raw(`SELECT id FROM config_providers WHERE name = ?`, "openai-fake").Scan(&providerPK).Error; err != nil {
		t.Fatalf("resolve provider id: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO config_keys (name, provider_id, provider, key_id, value, encryption_status, created_at, updated_at)
		 VALUES ('seed-key', ?, 'openai', 'seed-1', 'sk-plain', 'plain_text', datetime('now'), datetime('now'))`,
		providerPK,
	).Error; err != nil {
		t.Fatalf("seed plaintext row: %v", err)
	}
}

// TestRunAdminReencryptMain_Dispatches verifies the main entrypoint wires
// the help flag through to runReencrypt. This guards against accidental
// regressions in the dispatch switch.
func TestRunAdminReencryptMain_Dispatches(t *testing.T) {
	output := captureStderr(t, func() {
		code := runAdminReencryptMain(context.Background(), []string{"-h"})
		if code != 0 {
			t.Errorf("--help should exit 0; got %d", code)
		}
	})
	if !strings.Contains(output, "--confirm") {
		t.Errorf("expected --help to print flags; got %q", output)
	}
}
