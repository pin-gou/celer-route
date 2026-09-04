// Package setup writes ready-to-paste config files (or in-app step lists)
// for the supported AI clients: coding agents, domestic desktop agents &
// IDEs, and generic OpenAI-compatible clients. Each template function mirrors
// the corresponding `generate*` function in `ui/lib/utils/agentConfigs.ts` so
// the Web UI and the CLI emit byte-identical output for the same inputs.
package setup

// Agent identifies which AI client a template targets.
type Agent string

// Platform is the OS a template targets; it drives config paths, env-var
// syntax and in-app shortcuts, mirroring the Web UI's OS selector.
type Platform string

const (
	PlatformMacOS   Platform = "macos"
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
)

// PlatformFromGOOS maps runtime.GOOS to a Platform, defaulting to linux.
func PlatformFromGOOS(goos string) Platform {
	switch goos {
	case "windows":
		return PlatformWindows
	case "darwin":
		return PlatformMacOS
	default:
		return PlatformLinux
	}
}

// platformOrDefault returns in.Platform, falling back to linux when unset so
// callers that omit the field (tests, older callers) stay POSIX by default.
func platformOrDefault(in Input) Platform {
	if in.Platform == "" {
		return PlatformLinux
	}
	return in.Platform
}

const (
	Opencode         Agent = "opencode"
	ClaudeCode       Agent = "claude-code"
	Codex            Agent = "codex"
	OpenAICompatible Agent = "openai-compatible"
	Cursor           Agent = "cursor"
	WorkBuddy        Agent = "workbuddy"
	CodeBuddy        Agent = "codebuddy"
	Trae             Agent = "trae"
	ZCode            Agent = "zcode"
	MarsCode         Agent = "marscode"
	Lingma           Agent = "lingma"
)

// IsValid reports whether a is one of the supported clients.
func (a Agent) IsValid() bool {
	switch a {
	case Opencode, ClaudeCode, Codex, OpenAICompatible, Cursor,
		WorkBuddy, CodeBuddy, Trae, ZCode, MarsCode, Lingma:
		return true
	}
	return false
}

// File is a file that should be written to disk for an agent. For agents
// that don't read a config file (Cursor / Windsurf), the File is still
// returned — its content is the in-app step instructions to display.
type File struct {
	Path    string
	Content string
}

// Input bundles everything a template needs.
//
//   - BaseURL is the celer-route origin, e.g. "http://localhost:8080" or
//     "https://gateway.example.com/v1". The template strips a trailing /v1
//     before appending its own surface.
//   - APIKey is the virtual key value (`sk-bf-…`) or empty if inference
//     auth is disabled.
//   - Models is the full set of catalog models the user picked.
//   - DefaultModelID selects which model becomes the agent's default.
//     Falls back to the first Model.ID when unset.
//   - Protocol applies only to opencode: "chat" picks
//     @ai-sdk/openai-compatible, "responses" picks @ai-sdk/openai.
//   - Platform is the target OS (macos / windows / linux); defaults to linux.
type Input struct {
	BaseURL        string
	APIKey         string
	Models         []Model
	DefaultModelID string
	Protocol       string // "" / "chat" / "responses"
	Platform       Platform
}

// Model is the catalog row the template needs.
type Model struct {
	ID            string
	Name          string
	ContextLength int
	MaxOutput     int
}

// Env bundles one env recipe in the three shell dialects. Arrays carry no
// comment headers; the CLI/view layer layers those on top for display.
type Env struct {
	Posix      []string // export KEY=value
	PowerShell []string // $env:KEY = "value"
	Cmd        []string // set KEY=value
}

// Output is the rendered set of files plus optional env-var lines.
// For Cursor/Windsurf, Steps is the same content as Files[0].Content but
// returned separately so callers can format it differently if they want.
type Output struct {
	Files        []File
	Env          *Env
	Steps        []string
	DefaultModel string // e.g. "celer-route/minimax/MiniMax-M2.1"
	Agent        Agent
}

// ProviderKey is the provider key used inside agent configs (opencode/codex).
const ProviderKey = "celer-route"
