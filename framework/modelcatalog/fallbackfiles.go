package modelcatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Bundled copy of the upstream MCP server library catalog, embedded into the
// binary so the library page still has content when getbifrost.ai is
// unreachable at startup. The background sync refreshes it once the network is
// available again.
//
//go:embed fallback/mcp-library.json
var bundledMCPLibraryJSON []byte

// loadBundledMCPLibrary parses the embedded MCP library catalog into entries.
func loadBundledMCPLibrary() ([]MCPLibraryEntry, error) {
	var payload MCPLibraryPayload
	if err := json.Unmarshal(bundledMCPLibraryJSON, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bundled MCP library data: %w", err)
	}
	return payload.Servers, nil
}
