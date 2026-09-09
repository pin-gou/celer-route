package opencode

import (
	"crypto/rand"
	"encoding/hex"
	"hash/fnv"
	"sync/atomic"
	"time"
)

// sessionIDLength is the length of the bare id portion emitted by
// newSessionID, matching the opencode client's SessionID format
// (`ses_` + 26 chars = 30 chars total). Mirrors
// packages/schema/src/identifier.ts in anomalyco/opencode.
const sessionIDLength = 26

// sessionIDTimeHexLen is the hex-encoded timestamp-counter segment (12 chars
// = 6 bytes), matching the 48-bit ULID-style prefix the opencode client
// generates with `(timestamp_ms * 0x1000 + counter)` and bitwise-NOT for
// descending ordering.
const sessionIDTimeHexLen = 12

// sessionIDRandLen is the random base62 suffix length.
const sessionIDRandLen = sessionIDLength - sessionIDTimeHexLen

// sessionIDAlphabet matches the opencode client's `chars` table verbatim:
// 0-9 then A-Z then a-z (62 characters total). Indexing uses `byte % 62`,
// so any byte value maps cleanly into this range.
const sessionIDAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// sessionIDPrefix is the literal prefix the opencode client enforces via
// Schema.isStartsWith("ses") on every SessionID value.
const sessionIDPrefix = "ses_"

// sessionCounter is a process-local counter that disambiguates ids emitted
// within the same millisecond. The opencode client uses the same trick
// (counter advances under the same `lastTimestamp`); the atomic guarantees
// safe concurrent use from multiple bifrost worker goroutines.
var sessionCounter atomic.Int64

// newSessionID emits a session id with the exact format the opencode CLI /
// desktop client attaches to `x-opencode-session`:
//
//	ses_ + 12 hex chars (descending timestamp + counter) + 14 base62 chars
//
// The 12-hex prefix is the bitwise-NOT of `(timestamp_ms * 0x1000 + counter)`
// so newly minted ids sort before older ones — the client stores them as
// ULID-style descending keys for tree storage. We only need the format to be
// stable for the upstream Console handler's session lookup; the ordering is
// a free side effect.
//
// Random bytes come from crypto/rand (matches `crypto.getRandomValues` in
// the browser bundle and Node's `crypto.randomBytes` in the server bundle).
func newSessionID() string {
	nowMs := time.Now().UnixMilli()
	counter := sessionCounter.Add(1)

	// current = now_ms * 0x1000 + counter. Mirrors the JS implementation:
	//   const current = BigInt(timestamp) * 0x1000n + BigInt(counter)
	current := uint64(nowMs)<<12 | uint64(counter)
	// NOT for descending ordering.
	notCurrent := ^current

	// Emit the lower 6 bytes of notCurrent as 12 hex chars (time[0..6] in JS).
	// JS does `value >> BigInt(40 - 8 * index) & 0xffn` then `.toString(16).padStart(2, "0")`
	// for index in [0..6), which is little-endian byte extraction. We pack the
	// bytes into a 6-byte big-endian integer and let hex.EncodeToString produce
	// the same string order.
	timeBytes := make([]byte, 6)
	for i := 0; i < 6; i++ {
		timeBytes[i] = byte(notCurrent >> (8 * i))
	}
	timePart := hex.EncodeToString(timeBytes)

	// 14 base62 chars from crypto/rand — same algorithm as the opencode client:
	//   const bytes = crypto.getRandomValues(new Uint8Array(length - 12))
	//   return Array.from(bytes, (byte) => chars[byte % 62]).join("")
	randBytes := make([]byte, sessionIDRandLen)
	if _, err := rand.Read(randBytes); err != nil {
		// crypto/rand failures are unrecoverable on every supported platform.
		// Fall back to an FNV-derived digest of the current timestamp + counter
		// so the request still gets a syntactically valid id rather than
		// blowing up the whole inference path. The id will not be unique
		// across requests in this branch but is functionally indistinguishable
		// from the upstream behavior of "no id at all".
		h := fnv.New64a()
		var ctrBuf [8]byte
		for i := 0; i < 8; i++ {
			ctrBuf[i] = byte(counter >> (8 * i))
		}
		h.Write([]byte("opencode-session-fallback"))
		h.Write(ctrBuf[:])
		digest := h.Sum64()
		for i := range randBytes {
			randBytes[i] = byte(digest >> (8 * (i % 8)))
		}
	}
	suffix := make([]byte, sessionIDRandLen)
	for i, b := range randBytes {
		suffix[i] = sessionIDAlphabet[b%62]
	}

	var buf []byte
	buf = make([]byte, 0, len(sessionIDPrefix)+sessionIDLength)
	buf = append(buf, sessionIDPrefix...)
	buf = append(buf, timePart...)
	buf = append(buf, suffix...)
	return string(buf)
}