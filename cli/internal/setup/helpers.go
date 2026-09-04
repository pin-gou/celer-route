package setup

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// StripV1Suffix removes a trailing "/v1" (and trailing slashes) from a URL
// so callers can append sibling surfaces like "/anthropic".
func StripV1Suffix(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	u = strings.TrimSuffix(u, "/v1")
	return u
}

// OpenAISurface returns the OpenAI-compatible base (…/v1).
func OpenAISurface(baseURL string) string {
	return StripV1Suffix(baseURL) + "/v1"
}

// AnthropicSurface returns the Anthropic-compatible base (…/anthropic).
// The gateway serves /anthropic/v1/messages from this prefix.
func AnthropicSurface(baseURL string) string {
	return StripV1Suffix(baseURL) + "/anthropic"
}

// pickDefaultModel returns the requested default ID when present in models,
// otherwise models[0].ID. Returns "" when models is empty.
func pickDefaultModel(models []Model, requested string) string {
	if requested != "" {
		for _, m := range models {
			if m.ID == requested {
				return requested
			}
		}
	}
	if len(models) > 0 {
		return models[0].ID
	}
	return ""
}

// limitMap mirrors opencode.json's `limit` shape: { "context": N, "output": N }.
// Fields are omitted when unknown so the output stays compact.
func limitMap(m Model) map[string]int {
	limit := map[string]int{}
	if m.ContextLength > 0 {
		limit["context"] = m.ContextLength
	}
	if m.MaxOutput > 0 {
		limit["output"] = m.MaxOutput
	}
	if len(limit) == 0 {
		return nil
	}
	return limit
}

// JSONMarshalIndent is a thin wrapper that panics on encode failure —
// we only ever feed it map/slice values we just built, so failure is a
// programming bug rather than user input.
func JSONMarshalIndent(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("setup: json encode: %v", err))
	}
	return string(b) + "\n"
}

// sortedModelIDs returns the model IDs in deterministic order (sorted
// alphabetically) so the byte output is stable for snapshots / tests.
func sortedModelIDs(models []Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ID
	}
	sort.Strings(out)
	return out
}
