// Command celer-route-setup exposes a small `models` helper that prints the
// live model catalog of a running celer-route instance. The per-client
// config generation that used to live here (`setup-*`) has been removed —
// the Web UI's "接入 AI 客户端" page now emits a self-contained bash /
// PowerShell one-command apply, so there is no compiled binary to
// distribute per OS/arch.
//
// Examples:
//
//	celer-route-setup models
//	celer-route-setup models --remote http://192.168.0.15:8080
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pin-gou/celer-route/cli/internal/catalog"
)

const usage = `celer-route-setup — list models from a running celer-route instance.

Subcommands:
  models                            Print the live /v1/models catalog as JSON.

Flags (after the subcommand):
  --remote <url>     Target celer-route base URL. Default http://localhost:8080.
  --port <n>         Override port when --remote is omitted (default 8080).
  --api-key <vk>     Virtual key (sk-bf-…). Falls back to $CELER_ROUTE_API_KEY.
  --help             Show this message.

Examples:
  celer-route-setup models
  celer-route-setup models --remote http://192.168.0.15:8080 --api-key sk-bf-…
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var err error
	switch sub {
	case "models":
		err = runModels(ctx, args)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "celer-route-setup: unknown subcommand %q\n\n%s", sub, usage)
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "celer-route-setup: %v\n", err)
		os.Exit(1)
	}
}

type modelsFlags struct {
	Remote string
	Port   int
	APIKey string
	Help   bool
}

func registerModelsFlags(fs *flag.FlagSet) *modelsFlags {
	f := &modelsFlags{}
	fs.StringVar(&f.Remote, "remote", "", "celer-route base URL (overrides --port)")
	fs.IntVar(&f.Port, "port", 8080, "Local celer-route port when --remote is empty")
	fs.StringVar(&f.APIKey, "api-key", "", "Virtual key (sk-bf-…); falls back to $CELER_ROUTE_API_KEY")
	fs.BoolVar(&f.Help, "help", false, "Show flag help")
	return f
}

func (f *modelsFlags) baseURL() string {
	if f.Remote != "" {
		return f.Remote
	}
	return fmt.Sprintf("http://localhost:%d", f.Port)
}

func (f *modelsFlags) resolveAPIKey() string {
	if f.APIKey != "" {
		return f.APIKey
	}
	return os.Getenv("CELER_ROUTE_API_KEY")
}

// runModels prints the gateway's live model catalog as JSON to stdout.
func runModels(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := registerModelsFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if f.Help {
		fs.Usage()
		return nil
	}
	models, err := catalog.List(ctx, f.baseURL(), f.resolveAPIKey())
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
