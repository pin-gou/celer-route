// Package renderers implements semantic compression for structured tool
// outputs (git diff, pytest summaries, terraform plans, JSON tables).
//
// Each Renderer transforms matched text into a more compact representation.
// Renderers are dispatched by detection Type via the package-level registry
// (`ApplyRenderer`). Renderers MUST be fail-open: any panic or unrecoverable
// error is recovered at the registry boundary and the original text is
// returned unchanged.
//
// Aligned with OmniRoute's renderers/ package
// (open-sse/services/compression/engines/rtk/renderers/).
//
// To avoid an import cycle (the parent rtk package imports this package),
// renderers do NOT depend on the parent's CommandDetection type. Instead,
// they accept a DetectionInfo struct with the fields they need (Type,
// Category, etc.). The rtk package constructs DetectionInfo values and
// passes them to ApplyRenderer.
package renderers

// DetectionInfo carries the subset of CommandDetection fields that
// renderers consume. The rtk parent package constructs one of these from
// each CommandDetection and passes it to ApplyRenderer.
type DetectionInfo struct {
	// Type is the detection type ("git-diff", "test-pytest",
	// "terraform-plan", "aws", "json-output", ...). Renderers key on Type.
	Type string
	// Command is the detected command string (e.g. "git status").
	Command string
	// Category is the high-level classification ("git"|"test"|"build"|...).
	Category string
}

// RenderResult is the outcome of a single renderer application.
// `Changed` distinguishes a real rewrite from a no-op pass-through.
// `Renderer` carries the renderer name for diagnostics / processStats.
type RenderResult struct {
	Text     string
	Changed  bool
	Renderer string
}

// Renderer is the function signature every renderer implements. It receives
// the (already ANSI-stripped) text and the originating DetectionInfo, and
// returns a RenderResult. The second return is `applied`: when false, the
// caller treats the result as a no-op and discards the Renderer name.
type Renderer func(text string, det DetectionInfo) (RenderResult, bool)

// NoRender is the canonical no-op renderer used when an input does not
// match any pattern that warrants rewriting (e.g. missing @@ headers for
// git-diff, or a failing test suite for test-green).
func NoRender(text string) (RenderResult, bool) {
	return RenderResult{Text: text, Changed: false, Renderer: ""}, false
}
