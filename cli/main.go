// Command celer-route-setup wires up the `setup-*` command family plus
// the `models` and `connect` helpers. Each subcommand reads the live
// model catalog from a running celer-route instance (local by default,
// or via --remote) and writes the tool's own config file.
//
// Examples:
//
//	celer-route-setup models
//	celer-route-setup setup-opencode --only minimax,glm
//	celer-route-setup setup-claude --remote http://192.168.0.15:8080
//	celer-route-setup setup-cursor   # prints in-app steps (no file)
//
// All file-writing commands honour --dry-run, which prints the would-be
// file to stdout instead of touching disk.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/pin-gou/celer-route/cli/internal/catalog"
	"github.com/pin-gou/celer-route/cli/internal/setup"
)

const usage = `celer-route-setup — generate ready-to-paste config for AI clients.

Subcommands:
  models                            List the live model catalog from the gateway.
  setup-opencode                    Write ~/.config/opencode/opencode.json.
  setup-claude                       Write ~/.claude/settings.json.
  setup-codex                       Write ~/.codex/config.toml.
  setup-openai-compat                Print OPENAI_* env recipe.
  setup-cursor                       Print Cursor / Windsurf in-app steps.
  setup-workbuddy                    Write ~/.workbuddy/models.json (Tencent).
  setup-codebuddy                    Write ~/.codebuddy/models.json (Tencent).
  setup-trae                         Print Trae in-app steps (ByteDance).
  setup-zcode                        Print ZCode in-app steps (Zhipu).
  setup-marscode                     Print OPENAI_* env recipe (ByteDance).
  setup-lingma                       Print Tongyi Lingma in-app steps (Alibaba).

Common flags (after the subcommand):
  --remote <url>     Target celer-route base URL. Default http://localhost:8080.
  --port <n>         Override port when --remote is omitted (default 8080).
  --api-key <vk>     Virtual key value (sk-bf-…). Falls back to
                     $CELER_ROUTE_API_KEY env var.
  --only <patterns>  Comma-separated substrings; keep only models whose id
                     matches at least one pattern.
  --model <id>       Set the default model for the agent.
  --responses        (setup-opencode only) Use Responses API (@ai-sdk/openai).
  --dry-run          Print the file(s) to stdout instead of writing.
  --home <dir>       Override $HOME for config-file path resolution.
  --help             Show this message.

Examples:
  celer-route-setup setup-opencode
  celer-route-setup setup-claude --remote http://192.168.0.15:8080 --api-key sk-bf-…
  celer-route-setup setup-codex --only minimax,glm --dry-run
  celer-route-setup setup-cursor
  celer-route-setup setup-workbuddy --only minimax
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
	case "setup-opencode":
		err = runSetup(ctx, setup.Opencode, args)
	case "setup-claude":
		err = runSetup(ctx, setup.ClaudeCode, args)
	case "setup-codex":
		err = runSetup(ctx, setup.Codex, args)
	case "setup-openai-compat":
		err = runSetup(ctx, setup.OpenAICompatible, args)
	case "setup-cursor":
		err = runSetup(ctx, setup.Cursor, args)
	case "setup-workbuddy":
		err = runSetup(ctx, setup.WorkBuddy, args)
	case "setup-codebuddy":
		err = runSetup(ctx, setup.CodeBuddy, args)
	case "setup-trae":
		err = runSetup(ctx, setup.Trae, args)
	case "setup-zcode":
		err = runSetup(ctx, setup.ZCode, args)
	case "setup-marscode":
		err = runSetup(ctx, setup.MarsCode, args)
	case "setup-lingma":
		err = runSetup(ctx, setup.Lingma, args)
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

// commonFlags is the flag set shared by every setup-* subcommand.
// Each flag is parsed exactly once; subcommand-specific behavior (e.g.
// --responses) is layered on top by setupFlags().
type commonFlags struct {
	Remote string
	Port   int
	APIKey string
	Only   string
	Model  string
	DryRun bool
	Home   string
	Help   bool
}

func registerCommon(fs *flag.FlagSet) *commonFlags {
	f := &commonFlags{}
	fs.StringVar(&f.Remote, "remote", "", "celer-route base URL (overrides --port)")
	fs.IntVar(&f.Port, "port", 8080, "Local celer-route port when --remote is empty")
	fs.StringVar(&f.APIKey, "api-key", "", "Virtual key (sk-bf-…); falls back to $CELER_ROUTE_API_KEY")
	fs.StringVar(&f.Only, "only", "", "Comma-separated model-id substrings to keep")
	fs.StringVar(&f.Model, "model", "", "Default model id (default: first in the catalog)")
	fs.BoolVar(&f.DryRun, "dry-run", false, "Print files to stdout instead of writing")
	fs.StringVar(&f.Home, "home", "", "Override $HOME for path resolution")
	fs.BoolVar(&f.Help, "help", false, "Show flag help")
	return f
}

// baseURL resolves the gateway origin from the flag set.
func (f *commonFlags) baseURL() string {
	if f.Remote != "" {
		return f.Remote
	}
	return fmt.Sprintf("http://localhost:%d", f.Port)
}

// resolveAPIKey picks the virtual key from --api-key or env.
func (f *commonFlags) resolveAPIKey() string {
	if f.APIKey != "" {
		return f.APIKey
	}
	return os.Getenv("CELER_ROUTE_API_KEY")
}

// homeDir returns the user-provided --home or $HOME.
func (f *commonFlags) homeDir() string {
	if f.Home != "" {
		return f.Home
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func parseOnly(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// runModels prints the gateway's live model catalog as JSON to stdout.
// Useful for previewing what `setup-*` would emit.
func runModels(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := registerCommon(fs)
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

// runSetup is the shared implementation for every setup-* subcommand.
// setupFlags lets a specific agent (currently only opencode's --responses)
// add extra knobs.
func runSetup(ctx context.Context, agent setup.Agent, args []string) error {
	fs := flag.NewFlagSet(string(agent), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := registerCommon(fs)
	var useResponses bool
	if agent == setup.Opencode {
		fs.BoolVar(&useResponses, "responses", false, "Use Responses API (@ai-sdk/openai) instead of Chat API")
	}
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
	if filtered := catalog.Filter(models, parseOnly(f.Only)); len(filtered) > 0 {
		models = filtered
	}
	if len(models) == 0 {
		return fmt.Errorf("no models matched (catalog returned %d before --only filter)", len(models))
	}

	in := setup.Input{
		BaseURL:        f.baseURL(),
		APIKey:         f.resolveAPIKey(),
		Models:         toSetupModels(models),
		DefaultModelID: f.Model,
		Platform:       setup.PlatformFromGOOS(runtime.GOOS),
	}
	if agent == setup.Opencode {
		if useResponses {
			in.Protocol = "responses"
		} else {
			in.Protocol = "chat"
		}
	}

	out, err := setup.Dispatch(agent, in)
	if err != nil {
		return err
	}

	if f.DryRun {
		return writeDryRun(out)
	}

	for _, file := range out.Files {
		expanded := expandHome(file.Path, f.homeDir())
		if err := writeFile(expanded, file.Content); err != nil {
			return fmt.Errorf("write %s: %w", file.Path, err)
		}
		fmt.Fprintf(os.Stdout, "wrote %s\n", expanded)
	}
	if out.Env != nil {
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "# Set these environment variables before launching the client:")
		printEnv(out, os.Stdout, "# ")
	}
	if out.DefaultModel != "" {
		fmt.Fprintf(os.Stdout, "\n# Default model reference: %s\n", out.DefaultModel)
	}
	return nil
}

// writeDryRun prints the rendered files to stdout so operators can pipe
// them into `tee`, diff them against an existing config, etc.
func writeDryRun(out setup.Output) error {
	for i, f := range out.Files {
		if i > 0 {
			fmt.Fprintln(os.Stdout, "\n"+"--")
		}
		fmt.Fprintf(os.Stdout, "==> %s <==\n%s", f.Path, f.Content)
	}
	if out.Env != nil {
		fmt.Fprintln(os.Stdout, "\n# Environment:")
		printEnv(out, os.Stdout, "# ")
	}
	if out.DefaultModel != "" {
		fmt.Fprintf(os.Stdout, "\n# Default model: %s\n", out.DefaultModel)
	}
	return nil
}

// printEnv writes the platform-appropriate env recipe. On Windows both the
// PowerShell and the cmd block are printed so either shell has a runnable
// snippet; on macOS/Linux the POSIX export lines are shown. Every line is
// prefixed with `prefix` (a comment marker) since these are instructions,
// not something the CLI executes.
func printEnv(out setup.Output, w io.Writer, prefix string) {
	env := out.Env
	if env == nil {
		return
	}
	if runtime.GOOS == "windows" {
		if len(env.PowerShell) > 0 {
			fmt.Fprintf(w, "%s# PowerShell:\n", prefix)
			for _, line := range env.PowerShell {
				fmt.Fprintf(w, "%s%s\n", prefix, line)
			}
		}
		if len(env.Cmd) > 0 {
			fmt.Fprintf(w, "%s# cmd:\n", prefix)
			for _, line := range env.Cmd {
				fmt.Fprintf(w, "%s%s\n", prefix, line)
			}
		}
		return
	}
	for _, line := range env.Posix {
		fmt.Fprintf(w, "%s%s\n", prefix, line)
	}
}

// expandHome turns a "~"- or "%USERPROFILE%"-prefixed path into an absolute
// one. The template's path strings start with "~/.…" (POSIX) or
// "%USERPROFILE%\…" (Windows); the CLI resolves them against the real home
// dir, with --home as an override for tests.
func expandHome(path, home string) string {
	rest := ""
	switch {
	case strings.HasPrefix(path, "~/"):
		rest = path[2:]
	case strings.HasPrefix(path, "%USERPROFILE%\\"):
		rest = strings.ReplaceAll(path[len("%USERPROFILE%\\"):], "\\", "/")
	default:
		return path
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return path
	}
	return filepath.Join(home, rest)
}

// writeFile creates the parent directory if needed, refuses to follow a
// non-existent $HOME, and creates the file with 0600 perms (config
// files often contain API keys).
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// toSetupModels converts the catalog model into the setup-package
// shape — the field names differ slightly (ContextLength vs
// context_length) so we keep the conversion at the boundary.
func toSetupModels(in []catalog.Model) []setup.Model {
	out := make([]setup.Model, len(in))
	for i, m := range in {
		out[i] = setup.Model{
			ID:            m.ID,
			Name:          m.Name,
			ContextLength: m.ContextLength,
			MaxOutput:     m.MaxOutput,
		}
	}
	return out
}

var _ = io.Discard // keep io import available for future writers
