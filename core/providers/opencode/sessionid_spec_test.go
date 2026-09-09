package opencode

import (
	"regexp"
	"testing"
)

// TestSessionIDFormat_MatchesOpencodeClientSpec locks the wire format to the
// exact shape used by the opencode CLI / desktop client (anomalyco/opencode):
// packages/schema/src/identifier.ts + session-id.ts. If the client changes
// the spec, this test must be updated together with sessionid.go.
func TestSessionIDFormat_MatchesOpencodeClientSpec(t *testing.T) {
	// spec from packages/schema/src/session-id.ts:
	//   export const SessionID = Schema.String.check(Schema.isStartsWith("ses"))
	// spec from packages/schema/src/identifier.ts:
	//   length = 26
	//   time segment = 12 hex chars (NOT of (timestamp_ms * 0x1000 + counter))
	//   random segment = 14 chars from `chars = "0123456789A-Z-a-z"`
	const spec = `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`

	for i := 0; i < 64; i++ {
		id := newSessionID()
		if matched, err := regexp.MatchString(spec, id); err != nil || !matched {
			t.Fatalf("id %q fails opencode spec %s (err=%v)", id, spec, err)
		}
	}
}