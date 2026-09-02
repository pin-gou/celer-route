package renderers

import ()

// registry maps detection types to their Renderer implementations.
// Detection types are produced by rtk/CommandDetection.Type. Each renderer
// is fail-open by contract (see types.go): any panic returns the original
// text untouched. Aligned with OmniRoute's renderers/index.ts.
var registry = map[string]Renderer{
	// git-diff family: renderer trims context lines / metadata and keeps
	// only file headers, hunks, and +/- change lines.
	"git-diff": renderGitDiff,

	// test-green family: collapses all-green test output to a single
	// summary line. ANY failure signal (FAIL, ✖, Error, Traceback,
	// AssertionError, numeric failed counts) forces a no-op.
	"test-pytest":  renderTestGreen,
	"test-jest":    renderTestGreen,
	"test-vitest":  renderTestGreen,
	"test-go":      renderTestGreen,
	"build-eslint": renderTestGreen,

	// terraform-plan family: collapses verbose terraform/tofu plan output
	// to `Plan: +N ~M -K` plus resource address lines.
	"terraform-plan": renderTerraformPlan,
	"tofu-plan":      renderTerraformPlan,

	// structured-table family: parses JSON arrays into minimal TSV
	// tables, preferring name/id/status/type/kind columns.
	"aws":         renderStructuredTable,
	"json-output": renderStructuredTable,
}

// RenderConfig controls optional renderer filtering. When BlockedRenderers
// is non-empty, detection types in the list are skipped; everything else
// passes through to its renderer (and ultimately through unchanged if no
// renderer is registered). This mirrors `config.disabled_renderers`.
type RenderConfig struct {
	// BlockedRenderers is a blacklist of detection types whose renderers
	// should be skipped. Empty means "all registered renderers allowed".
	BlockedRenderers []string
}

// ApplyRenderer dispatches to the registered renderer for the given
// detection type. When no renderer is registered, the type is in the
// blacklist, or the renderer returns a no-op, the original text is
// returned with Changed=false.
//
// The dispatcher wraps the renderer call in a recover() guard so a panic
// inside a renderer never propagates to the request path (fail-open). On
// panic, the original text is returned unchanged.
func ApplyRenderer(text string, det DetectionInfo, cfg RenderConfig) (result RenderResult) {
	r, ok := registry[det.Type]
	if !ok {
		return RenderResult{Text: text, Changed: false, Renderer: ""}
	}
	if len(cfg.BlockedRenderers) > 0 && containsString(cfg.BlockedRenderers, det.Type) {
		return RenderResult{Text: text, Changed: false, Renderer: ""}
	}

	// Fail-open: panic in any renderer MUST NOT bring down the request.
	// Use a named return so the deferred function can restore `text` on
	// panic (otherwise the zero-value RenderResult would propagate up).
	defer func() {
		if rec := recover(); rec != nil {
			result = RenderResult{Text: text, Changed: false, Renderer: ""}
			_ = rec
		}
	}()

	res, applied := r(text, det)
	if !applied {
		return RenderResult{Text: text, Changed: false, Renderer: ""}
	}
	return res
}

// RegisteredRenderers returns the detection types currently mapped to a
// renderer. Used by diagnostic / introspection endpoints.
func RegisteredRenderers() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
