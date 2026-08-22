package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	bifrost "github.com/pin-gou/pg-gateway/core"
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/auth"
	"github.com/pin-gou/pg-gateway/framework/configstore"
	"github.com/pin-gou/pg-gateway/framework/encrypt"
)

// resetOptions carries parsed CLI flags for the admin reset subcommand.
type resetOptions struct {
	ConfigPath    string // empty => use default SQLite DB (AppDir/config.db)
	AppDir        string // only used when ConfigPath is empty
	Username      string // empty => prompt
	PasswordStdin bool
	AssumeYes     bool
}

// loadConfigStoreConfig resolves the config store connection. When configPath
// is non-empty it parses config.json's config_store block; when empty it
// returns a default SQLite store at <appDir>/config.db (the same default the
// server uses when no config.json exists). When appDir is also empty, the
// APP_DIR environment variable is checked before falling back to the current
// working directory — this makes `pg-gateway-admin admin reset` work inside
// the Docker container without any flags.
func loadConfigStoreConfig(configPath, appDir string) (*configstore.Config, string, error) {
	if configPath != "" {
		cfg, err := loadConfigStoreConfigFromFile(configPath)
		return cfg, configPath, err
	}
	dir := appDir
	if dir == "" {
		dir = os.Getenv("APP_DIR")
	}
	if dir == "" {
		dir = "."
	}
	cfg, err := defaultSQLiteConfig(dir)
	return cfg, filepath.Join(dir, "config.db"), err
}

// defaultSQLiteConfig builds a config store config pointing at a SQLite DB at
// <dir>/config.db, mirroring the server's default when no config.json exists.
func defaultSQLiteConfig(dir string) (*configstore.Config, error) {
	if err := initEncryptionFromEnv(); err != nil {
		return nil, err
	}
	return &configstore.Config{
		Enabled: true,
		Type:    configstore.ConfigStoreTypeSQLite,
		Config:  &configstore.SQLiteConfig{Path: filepath.Join(dir, "config.db")},
	}, nil
}

// minimalConfigRoot is the subset of pg-gateway config.json that the admin
// CLI needs to find and connect to the config store. Parsing the whole
// transports/pg-gateway-http/lib ConfigData would drag in plugins,
// providers, and other server-startup surfaces that a single-operator CLI
// has no business loading.
type minimalConfigRoot struct {
	EncryptionKey json.RawMessage `json:"encryption_key"`
	ConfigStore   json.RawMessage `json:"config_store"`
}

// loadConfigStoreConfigFromFile parses config.json and returns the embedded
// *configstore.Config ready for configstore.NewConfigStore. It also
// initialises framework/encrypt when the config defines an encryption_key,
// so that encrypted rows in the config store can be read and rewritten.
func loadConfigStoreConfigFromFile(path string) (*configstore.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	var root minimalConfigRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	if len(root.ConfigStore) == 0 || string(root.ConfigStore) == "null" {
		return nil, errors.New("config file does not define a config_store; pg-gateway cannot persist admin accounts without one — add a config_store block to config.json or omit --config to use the default SQLite DB")
	}
	var cfg configstore.Config
	if err := json.Unmarshal(root.ConfigStore, &cfg); err != nil {
		return nil, fmt.Errorf("parse config_store block: %w", err)
	}
	if !cfg.Enabled {
		return nil, errors.New("config_store.enabled is false; enable it to use pg-gateway-admin")
	}
	if err := initEncryption(root.EncryptionKey); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// initEncryptionFromEnv mirrors transports/pg-gateway-http/lib.initEncryption
// for the no-config.json path: it honours the BIFROST_ENCRYPTION_KEY env var.
func initEncryptionFromEnv() error {
	if env := os.Getenv("BIFROST_ENCRYPTION_KEY"); env != "" {
		encrypt.Init(env, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	}
	return nil
}

// initEncryption mirrors transports/pg-gateway-http/lib.initEncryption: it
// honours an explicit encryption_key from config.json and falls back to the
// BIFROST_ENCRYPTION_KEY env var. Without this initialisation the config
// store's BeforeSave hooks will refuse to write encrypted rows (or, worse,
// persist them in plaintext) and UpdateAuthConfig would either fail or
// silently corrupt the password column.
func initEncryption(rawKey json.RawMessage) error {
	if len(rawKey) > 0 && string(rawKey) != "null" {
		var sv schemas.SecretVar
		if err := json.Unmarshal(rawKey, &sv); err != nil {
			return fmt.Errorf("parse encryption_key: %w", err)
		}
		if v := sv.GetValue(); v != "" {
			encrypt.Init(v, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
			return nil
		}
	}
	if env := os.Getenv("BIFROST_ENCRYPTION_KEY"); env != "" {
		encrypt.Init(env, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	}
	return nil
}

func runReset(ctx context.Context, opts resetOptions) error {
	cfg, source, err := loadConfigStoreConfig(opts.ConfigPath, opts.AppDir)
	if err != nil {
		return err
	}
	logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	fmt.Fprintf(os.Stderr, "Connecting to config store from %s\n", source)
	fmt.Fprintf(os.Stderr, "  config_store: enabled=%t type=%s\n", cfg.Enabled, cfg.Type)

	store, err := configstore.NewConfigStore(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("connect to config store: %w", err)
	}
	defer func() {
		if cerr := store.Close(ctx); cerr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close config store: %v\n", cerr)
		}
	}()

	current, err := store.GetAuthConfig(ctx)
	if err != nil {
		return fmt.Errorf("read auth config: %w", err)
	}

	existingUsername := ""
	authEnabled := false
	if current != nil && current.AdminUserName != nil {
		existingUsername = current.AdminUserName.GetValue()
		authEnabled = current.IsEnabled
	}

	switch {
	case existingUsername == "":
		fmt.Fprintln(os.Stderr, "Current state: no admin account exists — will CREATE the initial admin.")
	default:
		fmt.Fprintf(os.Stderr, "Current state: admin account exists (username=%q, is_enabled=%t) — will RESET the password.\n", existingUsername, authEnabled)
	}

	if !opts.AssumeYes {
		if !isTerminal(os.Stdin) {
			return errors.New("refusing to run interactively without a TTY; pass --yes when supplying input via --username and --password-stdin")
		}
		fmt.Fprint(os.Stderr, "Proceed? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read confirmation: %w", err)
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			return errors.New("aborted by user")
		}
	}

	username := opts.Username
	if username == "" {
		username, err = promptUsername(existingUsername, isTerminal(os.Stdin))
		if err != nil {
			return err
		}
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("admin username cannot be empty")
	}

	password, err := promptPassword(opts.PasswordStdin, isTerminal(os.Stdin))
	if err != nil {
		return err
	}

	if failures := auth.GetPasswordPolicyFailures(password); len(failures) > 0 {
		return fmt.Errorf("password does not meet policy: %s", strings.Join(failures, ", "))
	}

	hashed, err := encrypt.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	updated := &configstore.AuthConfig{
		AdminUserName: schemas.NewSecretVar(username),
		AdminPassword: schemas.NewSecretVar(hashed),
		IsEnabled:     true,
	}
	if err := store.UpdateAuthConfig(ctx, updated); err != nil {
		return fmt.Errorf("write auth config: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Done.")
	fmt.Fprintf(os.Stderr, "  Admin username: %s\n", username)
	fmt.Fprintln(os.Stderr, "  Auth enabled:   true")
	if existingUsername == "" {
		fmt.Fprintln(os.Stderr, "  Next step:      log in to the pg-gateway dashboard.")
	} else {
		fmt.Fprintln(os.Stderr, "  Next step:      log in with the new password.")
	}
	fmt.Fprintln(os.Stderr, "  Note:           BIFROST_SETUP_TOKEN / setup_token is no longer required for this node.")
	return nil
}

func promptUsername(existing string, tty bool) (string, error) {
	if !tty {
		return "", errors.New("--username is required when stdin is not a TTY")
	}
	if existing != "" {
		fmt.Fprintf(os.Stderr, "Admin username [%s]: ", existing)
	} else {
		fmt.Fprint(os.Stderr, "Admin username: ")
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read username: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return existing, nil
	}
	return line, nil
}

// promptPassword reads the password twice and confirms the two values match.
// With --password-stdin the input is read line-by-line from stdin (no echo
// suppression — the caller's job to use a pipe that does not echo, e.g. a
// secret manager pipe). Otherwise the password is read interactively with
// terminal echo suppressed, like sudo / passwd.
func promptPassword(fromStdin, tty bool) (string, error) {
	if fromStdin {
		// A single bufio.Reader must back both reads: each ReadString call
		// prefetches up to the buffer size (4 KiB by default) into a private
		// buffer that is not returned to the underlying reader, so a fresh
		// bufio.NewReader per call would see the second line as EOF when the
		// caller pipes both lines in via heredoc.
		reader := bufio.NewReader(os.Stdin)
		pw, err := readLineFrom(reader)
		if err != nil {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		pw = strings.TrimRight(pw, "\r\n")
		confirm, err := readLineFrom(reader)
		if err != nil {
			return "", fmt.Errorf("read password confirmation from stdin: %w", err)
		}
		confirm = strings.TrimRight(confirm, "\r\n")
		if pw != confirm {
			return "", errors.New("passwords do not match")
		}
		return pw, nil
	}
	if !tty {
		return "", errors.New("--password-stdin is required when stdin is not a TTY")
	}
	fmt.Fprintln(os.Stderr, "New password (12+ chars, mixed case, digit, special):")
	pw, err := readPassword(os.Stderr, os.Stdin)
	if err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stderr, "Confirm password:")
	confirm, err := readPassword(os.Stderr, os.Stdin)
	if err != nil {
		return "", err
	}
	if pw != confirm {
		return "", errors.New("passwords do not match")
	}
	return pw, nil
}

func readPassword(prompt io.Writer, in *os.File) (string, error) {
	fmt.Fprint(prompt, "  ")
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("password prompt requires a terminal")
	}
	bytes, err := term.ReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Fprintln(prompt)
	return string(bytes), nil
}

func readLine(r io.Reader) (string, error) {
	return readLineFrom(bufio.NewReader(r))
}

func readLineFrom(br *bufio.Reader) (string, error) {
	line, err := br.ReadString('\n')
	if errors.Is(err, io.EOF) {
		// On EOF, ReadString returns whatever it managed to read plus io.EOF.
		// A zero-length result means the reader was already drained; anything
		// else is a valid final line that simply lacks a trailing newline.
		if line == "" {
			return "", io.EOF
		}
		return line, nil
	}
	if err != nil {
		return "", err
	}
	return line, nil
}

// isTerminal reports whether f is a character device (a TTY). The CLI uses
// this to decide whether interactive prompts are allowed: a non-TTY prompt
// would either echo the password into a log file or hang waiting for input
// that never comes.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
