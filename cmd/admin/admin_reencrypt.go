package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	"github.com/pin-gou/celer-route/framework/encrypt"
)

// reencryptOptions carries parsed CLI flags for the admin re-encrypt subcommand.
type reencryptOptions struct {
	ConfigPath string
	AppDir     string
	Mode       string
	BatchSize  int
	DryRun     bool
	Confirm    bool
}

// runReencrypt is the entry point for `celer-route-admin admin re-encrypt`.
// It connects to the config store, counts plaintext rows, and either reports
// (--dry-run) or migrates them. Without --confirm the command refuses to
// write — this is the single bit of friction that prevents a tired operator
// from accidentally triggering a global re-encrypt at 3am.
func runReencrypt(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("admin re-encrypt", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to celer-route config.json (optional — defaults to <app-dir>/config.db)")
	appDir := fs.String("app-dir", "", "application data directory (default: current directory, used when --config omitted)")
	mode := fs.String("mode", "plaintext-to-encrypted", "migration mode: plaintext-to-encrypted (default) or rotate-key (reserved)")
	batchSize := fs.Int("batch-size", 100, "rows per transaction (default 100)")
	dryRun := fs.Bool("dry-run", false, "only report the row counts that WOULD be migrated; do not write anything")
	confirm := fs.Bool("confirm", false, "required to perform a non-dry-run migration; without it the command exits with a reminder")
	if err := fs.Parse(args); err != nil {
		// flag already prints help; surface flag.ErrHelp as a no-op so the
		// test layer can distinguish "user asked for help" from "real error".
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	opts := reencryptOptions{
		ConfigPath: *configPath,
		AppDir:     *appDir,
		Mode:       strings.TrimSpace(*mode),
		BatchSize:  *batchSize,
		DryRun:     *dryRun,
		Confirm:    *confirm,
	}

	if opts.Mode == "" {
		opts.Mode = string(configstore.ReencryptModePlaintextToEncrypted)
	}

	logger := bifrost.NewDefaultLogger(schemas.LogLevelInfo)

	cfg, source, err := loadConfigStoreConfig(opts.ConfigPath, opts.AppDir)
	if err != nil {
		return err
	}
	// Initialise the encryption subsystem before opening the store. For the
	// --config path loadConfigStoreConfig already initialised it from
	// config.json's encryption_key; for the --app-dir path (or a config.json
	// that omits the key) we fall back to BIFROST_ENCRYPTION_KEY.
	if !encrypt.IsEnabled() {
		if err := initEncryptionFromEnv(); err != nil {
			return err
		}
	}
	if !encrypt.IsEnabled() {
		return fmt.Errorf("refusing to start: encryption is not enabled. Provide encryption_key in config.json (or BIFROST_ENCRYPTION_KEY), then re-run this command")
	}
	fmt.Fprintf(os.Stderr, "Connecting to config store from %s\n", source)
	fmt.Fprintf(os.Stderr, "  config_store: enabled=%t type=%s\n", cfg.Enabled, cfg.Type)

	// WithSkipStartupEncryptionSync is essential here: the store constructor
	// normally migrates plaintext rows as soon as a key is configured, which
	// would make --dry-run a no-op and bypass the --confirm gate entirely.
	// This command drives the migration itself.
	store, err := configstore.NewConfigStore(ctx, cfg, logger, configstore.WithSkipStartupEncryptionSync())
	if err != nil {
		return fmt.Errorf("connect to config store: %w", err)
	}
	defer func() {
		if cerr := store.Close(ctx); cerr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close config store: %v\n", cerr)
		}
	}()

	// Phase 1: always print the row counts the migration will touch, so the
	// operator can sanity-check the scope before committing to --confirm.
	before, err := store.CountPlaintextRows(ctx)
	if err != nil {
		return fmt.Errorf("count plaintext rows: %w", err)
	}
	printRowCounts(os.Stderr, "Plaintext rows (before)", before)

	totalBefore := sumInt64Values(before)
	if totalBefore == 0 {
		fmt.Fprintln(os.Stderr, "Nothing to do: no plaintext rows in any sensitive table.")
		return nil
	}

	if opts.DryRun {
		fmt.Fprintln(os.Stderr, "Dry run: no rows were modified.")
		return nil
	}

	if !opts.Confirm {
		return fmt.Errorf("refusing to migrate without --confirm; re-run with --confirm to apply the change. Tip: re-run with --dry-run first to verify the row counts above")
	}

	// Refuse unsupported modes before doing any work — gives the operator
	// a clean error instead of silently completing with no rows touched.
	if configstore.ReencryptMode(opts.Mode) != configstore.ReencryptModePlaintextToEncrypted {
		return fmt.Errorf("re-encrypt mode %q is not supported by this build", opts.Mode)
	}

	fmt.Fprintf(os.Stderr, "Migrating plaintext rows to encrypted storage (mode=%s, batch-size=%d) ...\n", opts.Mode, opts.BatchSize)
	result, err := store.ReencryptPlaintextRows(ctx, configstore.ReencryptOptions{
		Mode:      configstore.ReencryptMode(opts.Mode),
		BatchSize: opts.BatchSize,
		DryRun:    false,
	})
	if err != nil {
		return fmt.Errorf("re-encrypt failed: %w", err)
	}

	printRowCounts(os.Stderr, "Encrypted rows", result.Encrypted)
	after, err := store.CountPlaintextRows(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: post-migration count failed: %v\n", err)
	} else {
		printRowCounts(os.Stderr, "Plaintext rows (after)", after)
	}
	fmt.Fprintln(os.Stderr, "Done.")
	return nil
}

// printRowCounts renders a per-table count map with the same shape regardless
// of whether it is pre/post/delta. Empty maps print as "(none)" so the
// output is unambiguous on a quiet log.
func printRowCounts(out *os.File, label string, counts configstore.PlaintextRowCounts) {
	fmt.Fprintf(out, "  %s:\n", label)
	if len(counts) == 0 {
		fmt.Fprintln(out, "    (none)")
		return
	}
	// Stable table order so consecutive runs produce diffable output.
	tables := make([]string, 0, len(counts))
	for table := range counts {
		tables = append(tables, table)
	}
	for _, table := range sortedKeys(tables) {
		fmt.Fprintf(out, "    %-30s %d\n", table, counts[table])
	}
}

func sortedKeys(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func sumInt64Values(m configstore.PlaintextRowCounts) int64 {
	var total int64
	for _, n := range m {
		total += n
	}
	return total
}
