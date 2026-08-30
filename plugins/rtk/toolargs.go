package rtk

import (
	"strings"

	"github.com/tidwall/gjson"
)

// extractCommandFromArguments extracts the real shell command from a tool
// call's raw arguments string. Real agents pass arguments as JSON, most
// commonly `{"command":"git status"}` or `{"cmd":"..."}`, so the raw string
// can never match a filter's commandPatterns regex. We peel the wrapping JSON
// away and return the bare command; when the string is not JSON (older
// adapters that pass the command verbatim) it is returned unchanged.
//
// Supports the `command` and `cmd` keys plus a final whole-string fallback
// (e.g. a plain `git status`), so a hint is never lost to a stricter parse.
func extractCommandFromArguments(args string) string {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return ""
	}
	if trimmed[0] != '{' {
		// Not JSON — the argument IS the command (legacy adapters).
		return trimmed
	}
	for _, key := range []string{"command", "cmd"} {
		res := gjson.Get(trimmed, key)
		if res.Exists() && res.Type == gjson.String {
			if cmd := strings.TrimSpace(res.String()); cmd != "" {
				return cmd
			}
		}
	}
	// JSON but no recognised command key — fall back to the raw string.
	return trimmed
}
