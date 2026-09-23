// Command celer-route-admin manages the dashboard admin account directly
// against the config store, bypassing the HTTP setup_token bootstrap flow.
//
// Typical Docker usage:
//
//	docker exec -it celer-route celer-route-admin admin reset --app-dir /app/data
//
// The command does not listen on any port. It reads the config store (from
// config.json's config_store block, or a default SQLite DB at app-dir/config.db),
// opens it, prompts for a new admin username and password, and writes the
// updated AuthConfig row. This is intended for operators who want to enable
// dashboard password protection without first configuring BIFROST_SETUP_TOKEN
// and walking through the UI bootstrap.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

const usage = `celer-route-admin — direct admin account management for celer-route.

Subcommands:
  reset       Create the initial admin account or reset the password of an existing one.
  re-encrypt  Migrate plaintext rows in the config store to encrypted storage (Phase 6 / D9).

Examples:
  celer-route-admin admin reset --config /path/to/config.json
  celer-route-admin admin reset --app-dir /app/data
  celer-route-admin admin re-encrypt --config /path/to/config.json --dry-run
  celer-route-admin admin re-encrypt --config /path/to/config.json --confirm

Run 'celer-route-admin <subcommand> --help' for subcommand-specific flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "admin":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "missing subcommand: expected one of 'reset', 're-encrypt'")
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		switch os.Args[2] {
		case "reset":
			if len(os.Args) > 3 && (os.Args[3] == "-h" || os.Args[3] == "--help" || os.Args[3] == "help") {
				fmt.Fprint(os.Stderr, resetUsage)
				os.Exit(0)
			}
			os.Exit(runAdminReset(context.Background(), os.Args[3:]))
		case "re-encrypt":
			os.Exit(runAdminReencryptMain(context.Background(), os.Args[3:]))
		case "-h", "--help", "help":
			fmt.Fprint(os.Stderr, adminUsage)
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "unknown admin subcommand %q\n", os.Args[2])
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

// adminUsage is the per-subcommand help text for `celer-route-admin admin --help`.
// The full per-command help (with all flags) is shown by `admin <subcommand> --help`.
const adminUsage = `celer-route-admin admin — manage admin accounts and storage encryption.

Subcommands:
  reset       Create the initial admin account or reset the password of an existing one.
  re-encrypt  Migrate plaintext rows to encrypted storage (Phase 6 / D9).

See:
  celer-route-admin admin reset --help
  celer-route-admin admin re-encrypt --help
`

// runAdminReencryptMain parses the args and dispatches to runReencrypt.
// Splitting the parse/run split keeps runReencrypt unit-testable.
func runAdminReencryptMain(ctx context.Context, args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Fprint(os.Stderr, reencryptUsage)
		return 0
	}
	if err := runReencrypt(ctx, args); err != nil {
		fmt.Fprintf(os.Stderr, "celer-route-admin: %v\n", err)
		return 1
	}
	return 0
}

const reencryptUsage = `celer-route-admin admin re-encrypt — migrate plaintext rows in the config
store to encrypted storage (Phase 6 / D9).

Usage:
  celer-route-admin admin re-encrypt [flags]

Flags:
  --config PATH        Path to the celer-route config.json file. When omitted,
                       the CLI uses a default SQLite database at <app-dir>/config.db.
  --app-dir PATH       Application data directory (default: current directory,
                       used only when --config is omitted).
  --mode MODE          Migration mode:
                         plaintext-to-encrypted  (default) — encrypt rows whose
                                                  encryption_status is plain_text.
                         rotate-key               (reserved) — re-encrypt already-
                                                  encrypted rows under a new key.
                       Today's build supports plaintext-to-encrypted only;
                       rotate-key returns ErrReencryptModeUnsupported.
  --batch-size N       Rows per transaction (default 100). Larger batches are
                       faster but hold a transaction open longer.
  --dry-run            Only print the row counts that WOULD be migrated; do not
                       write anything. Safe to run any time.
  --confirm            Required to perform a non-dry-run migration. Without it
                       the command exits with a reminder rather than touching
                       the database. There is no other way to apply the change.

Typical D9 upgrade flow:
  1. Set allow_plaintext_storage=true in config.json (or BIFROST_ALLOW_PLAINTEXT_STORAGE=true).
  2. Export a plaintext backup of every config_key and VK value to an external vault.
  3. Remove the opt-in, set encryption_key (or BIFROST_ENCRYPTION_KEY).
  4. celer-route-admin admin re-encrypt --dry-run    # sanity-check the row counts.
  5. celer-route-admin admin re-encrypt --confirm    # apply the migration.
  6. Restart the server; the startup guard no longer complains.

Examples:
  celer-route-admin admin re-encrypt --app-dir /app/data --dry-run
  celer-route-admin admin re-encrypt --config /app/data/config.json --confirm
`

const resetUsage = `celer-route-admin admin reset — create the first admin account or reset
the password of an existing one, writing directly to the config store.

Usage:
  celer-route-admin admin reset [flags]

Flags:
  --config PATH    Path to the celer-route config.json file. When omitted, the
                   CLI uses a default SQLite database at <app-dir>/config.db.
  --app-dir PATH   Application data directory (default: current directory).
                   Used only when --config is not provided.
  --yes            Skip the interactive confirmation prompt (still prompts for
                   the username and password unless --username/--password-stdin
                   are also provided; see notes below).
  --username NAME  Skip the username prompt and use NAME.
  --password-stdin
                   Read the password (and confirmation) from stdin instead of
                   the terminal. Two lines: password then confirmation. The
                   terminal-based prompt refuses to run in non-TTY mode, so
                   --password-stdin is the way to script the command.

Examples:
  docker exec -it celer-route celer-route-admin admin reset --app-dir /app/data
  docker exec -it celer-route celer-route-admin admin reset --config /app/data/config.json

After the write, dashboard auth is enabled (auth_config.is_enabled=true). The
BIFROST_SETUP_TOKEN / setup_token path is not required — this command bypasses
it entirely.
`

func runAdminReset(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("admin reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to celer-route config.json (optional — defaults to <app-dir>/config.db)")
	appDir := fs.String("app-dir", "", "application data directory (default: current directory, used when --config omitted)")
	username := fs.String("username", "", "admin username (skips the prompt)")
	passwordStdin := fs.Bool("password-stdin", false, "read password (and confirmation) from stdin")
	assumeYes := fs.Bool("yes", false, "acknowledge the action non-interactively")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := resetOptions{
		ConfigPath:    *configPath,
		AppDir:        *appDir,
		Username:      *username,
		PasswordStdin: *passwordStdin,
		AssumeYes:     *assumeYes,
	}
	if err := runReset(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "celer-route-admin: %v\n", err)
		return 1
	}
	return 0
}
