// Package rtk — admin endpoints support: renderer catalog.
//
// The renderer catalog lists every detection Type currently mapped to a
// renderer in plugins/rtk/renderers plus a static category label so the
// RTK admin UI can render disabled_renderers as a multi-select with
// search and grouping (mirrors the caveman skip_rules UX).
//
// Endpoints served through this surface:
//   - GET /api/context/rtk/renderers
//     Returns RendererCatalog (renderers + categories) for the active
//     plugin instance. No request body required. The catalog is
//     package-static — renderers are registered via package init() in
//     plugins/rtk/renderers/index.go and never change at runtime — so the
//     endpoint is safe to call even before the plugin is fully initialised;
//     when the plugin is missing we still return a well-shaped empty
//     catalog so the UI degrades gracefully.
package rtk

import (
	"sort"

	"github.com/pin-gou/celer-route/plugins/rtk/renderers"
)

// rendererCategoryMap is the static category assignment for every detection
// type registered with the renderer package. The keys MUST stay in sync with
// plugins/rtk/renderers/index.go (the registry map); when adding a new
// detection type to the registry, add a category here too. The UI uses the
// category to group options inside the multi-select.
//
// Categories:
//
//	git      — git-diff family (file/commit diffs, hunks)
//	test     — test framework output (pytest, jest, vitest, go test, eslint)
//	terraform — terraform/tofu plan output
//	structured — JSON arrays / cloud CLI tabular output (aws, json-output)
var rendererCategoryMap = map[string]string{
	"git-diff":       "git",
	"test-pytest":    "test",
	"test-jest":      "test",
	"test-vitest":    "test",
	"test-go":        "test",
	"build-eslint":   "test",
	"terraform-plan": "terraform",
	"tofu-plan":      "terraform",
	"aws":            "structured",
	"json-output":    "structured",
}

// RendererCatalogEntry is one row of the renderer catalog returned by
// GetRendererCatalog. The shape is consumed by the RTK admin UI to
// populate the disabled_renderers multi-select.
type RendererCatalogEntry struct {
	// Name is the canonical detection Type — the value the operator puts
	// in config.disabled_renderers to skip this renderer.
	Name string `json:"name"`
	// Category groups the renderer for the UI ("git", "test",
	// "terraform", "structured"). Empty if a future renderer ships
	// without a registered category.
	Category string `json:"category,omitempty"`
}

// RendererCatalog is the response shape for GET /api/context/rtk/renderers.
type RendererCatalog struct {
	// Renderers is the list of detection Types currently mapped to a
	// renderer, tagged with a static category. Sorted by Name for
	// stable rendering across calls.
	Renderers []RendererCatalogEntry `json:"renderers"`
}

// GetRendererCatalog returns the static catalog of registered renderers,
// tagged with categories so the UI can group options inside the
// disabled_renderers multi-select. The plugin instance is not consulted —
// the catalog is a package-level constant — but we keep the method on
// *Plugin (mirroring GetCavemanRuleCatalog) so the server-side accessor
// surface stays uniform and the UI never has to special-case the missing
// plugin path.
func (p *Plugin) GetRendererCatalog() RendererCatalog {
	names := renderers.RegisteredRenderers()
	out := make([]RendererCatalogEntry, 0, len(names))
	for _, n := range names {
		out = append(out, RendererCatalogEntry{
			Name:     n,
			Category: rendererCategoryMap[n],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return RendererCatalog{Renderers: out}
}
