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
  reset    Create the initial admin account or reset the password of an existing one.

Examples:
  celer-route-admin admin reset --config /path/to/config.json
  celer-route-admin admin reset --app-dir /app/data

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
			fmt.Fprintln(os.Stderr, "missing subcommand: expected one of 'reset'")
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		switch os.Args[2] {
		case "reset":
			os.Exit(runAdminReset(context.Background(), os.Args[3:]))
		case "-h", "--help", "help":
			fmt.Fprint(os.Stderr, resetUsage)
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