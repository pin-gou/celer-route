// Package rtk — Stage 4: Raw output persistence (design D2).
//
// Provides secret redaction, failure detection, and best-effort disk persistence
// of raw tool outputs for debugging. Retention policy (never/failures/always) is
// configurable per the plugin Config. All disk errors are best-effort — the
// compression pipeline never blocks on I/O failures.
package rtk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// RtkRawOutputRetention controls when raw tool outputs are persisted to disk.
type RtkRawOutputRetention string

const (
	// RawOutputRetentionNever disables raw output persistence entirely.
	RawOutputRetentionNever RtkRawOutputRetention = "never"
	// RawOutputRetentionFailures persists only output that IsLikelyFailureOutput.
	RawOutputRetentionFailures RtkRawOutputRetention = "failures"
	// RawOutputRetentionAlways persists every output that is actually compressed.
	RawOutputRetentionAlways RtkRawOutputRetention = "always"
)

// RtkRawOutputPointer carries the result of a single raw-output persist operation.
type RtkRawOutputPointer struct {
	ID       string // sha256(<prefix>)[:24]
	Path     string // absolute path to the persisted .log file
	Bytes    int    // UTF-8 byte count of the written (redacted) text
	SHA256   string // full hex(sha256(redactedText)) 64 chars
	Redacted bool   // any of the 5 redaction patterns matched
}

// PersistOptions configures a single MaybePersistRtkRawOutput call.
type PersistOptions struct {
	Retention RtkRawOutputRetention // never | failures | always
	Command   string                // used for command slug in filename
	MaxBytes  int                   // default 1048576, minimum 1024
	Failure   *bool                 // explicit override of IsLikelyFailureOutput
	AppDir    string                // root directory for <appDir>/rtk/raw-output/
}

// rawOutputPaths is a package-level registry mapping pointer ID → absolute path,
// so ReadRtkRawOutput can locate a file when only the ID is known.
var rawOutputPaths sync.Map

// 5 pre-compiled secret redaction regexes (ReDoS-safe, all with \b markers).
var (
	// reSecretOpenAI redacts OpenAI-style API keys: sk- + ≥16 alnum.
	reSecretOpenAI = regexp.MustCompile(`\bsk-[A-Za-z0-9]{16,}`)
	// reSecretSlack redacts Slack tokens: xox<type>- + token chars.
	reSecretSlack = regexp.MustCompile(`\bxox[a-zA-Z]-[A-Za-z0-9-]{8,}`)
	// reSecretAWS redacts AWS access key IDs: AKIA + 16 alnum.
	reSecretAWS = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	// reSecretAuthHeader redacts (Proxy-)Authorization: Bearer|Basic|Token values.
	reSecretAuthHeader = regexp.MustCompile(`(?i)(^|\n)([ \t]*(?:Proxy-)?Authorization\s*:\s*(?:Bearer|Basic|Token)\s+)\S+`)
	// reSecretCredField redacts credential field values in key=value / key: "value" form.
	// Group 1 captures the field name + separator (e.g. "api_key="), which is preserved.
	reSecretCredField = regexp.MustCompile(`(?i)(\b[a-z0-9_.-]*(?:key|token|secret|password|passwd|credential)[a-z0-9_-]*\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
)

// reFailureOutput matches the 9 failure keywords (V-plugins-3).
var reFailureOutput = regexp.MustCompile(`(?i)\b(error|failed|failure|exception|traceback|panic|fatal|critical|ts\d{4}|fail)\b`)

// reSlugInvalid matches characters that are not valid in a filename slug.
var reSlugInvalid = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// RedactRtkRawOutput applies the 5 secret-redaction patterns to the input text
// and returns the redacted text and whether any redaction occurred.
//
// Replacement order: explicit key patterns (sk-, xox, AKIA) → auth header →
// credential field patterns. This order ensures that a secret embedded in a
// credential value (e.g. "api_key=sk-abc...") is fully redacted.
func RedactRtkRawOutput(value string) (text string, redacted bool) {
	text = value
	orig := text

	// 1. OpenAI keys
	text = reSecretOpenAI.ReplaceAllString(text, "[REDACTED_OPENAI_KEY]")
	// 2. Slack tokens
	text = reSecretSlack.ReplaceAllString(text, "[REDACTED_SLACK_TOKEN]")
	// 3. AWS keys
	text = reSecretAWS.ReplaceAllString(text, "[REDACTED_AWS_KEY]")
	// 4. Authorization headers
	text = reSecretAuthHeader.ReplaceAllString(text, "${1}${2}[REDACTED]")
	// 5. Credential field values
	text = reSecretCredField.ReplaceAllString(text, "${1}[REDACTED]")

	return text, text != orig
}

// IsLikelyFailureOutput reports whether the text contains any of the 9 failure
// keywords (V-plugins-3). Word boundaries are used to prevent false positives
// on partial-word matches (e.g. "errorsome" does not match "error").
func IsLikelyFailureOutput(value string) bool {
	if value == "" {
		return false
	}
	return reFailureOutput.MatchString(value)
}

// MaybePersistRtkRawOutput writes the raw output to disk under
// <appDir>/rtk/raw-output/ when the retention policy allows it, and returns
// a pointer to the persisted file. Disk errors (EACCES, ENOSPC, etc.) are
// handled best-effort: nil is returned and the caller should proceed without
// the pointer. The pointer's ID is embedded in the filename as the last 24 hex
// characters: <ts_ms>-<slug>-<id24>.log.
func MaybePersistRtkRawOutput(raw string, opts PersistOptions) *RtkRawOutputPointer {
	// Retention "never" or empty → skip.
	if opts.Retention == RawOutputRetentionNever || opts.Retention == "" {
		return nil
	}
	// Blank/whitespace-only output → skip.
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	// Determine failure state.
	failure := IsLikelyFailureOutput(raw)
	if opts.Failure != nil {
		failure = *opts.Failure
	}
	// Retention "failures" and this is not a failure → skip.
	if opts.Retention == RawOutputRetentionFailures && !failure {
		return nil
	}

	// Apply redaction.
	redactedText, redacted := RedactRtkRawOutput(raw)

	// Compute MaxBytes default/clamp.
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = 1048576
	} else if maxBytes < 1024 {
		maxBytes = 1024
	}

	// UTF-8 byte-level truncation.
	if len(redactedText) > maxBytes {
		redactedText = safeUtf8Slice(redactedText, maxBytes)
	}

	// Compute SHA-256 of the redacted text (full 64-char hex).
	shaSum := sha256.Sum256([]byte(redactedText))
	shaHex := hex.EncodeToString(shaSum[:])

	// Build ID: first 24 hex chars of the sha256 of the content.
	id := shaHex[:24]

	// Build command slug.
	slug := commandSlug(opts.Command)

	// Timestamp in milliseconds.
	ts := time.Now().UnixMilli()

	// Ensure the output directory exists.
	dir := filepath.Join(opts.AppDir, "rtk", "raw-output")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// Best-effort: return nil, no panic.
		return nil
	}

	base := fmt.Sprintf("%d-%s-%s", ts, slug, id)
	logPath := filepath.Join(dir, base+".log")
	metaPath := filepath.Join(dir, base+".meta.json")

	// Write the main .log file.
	if err := os.WriteFile(logPath, []byte(redactedText), 0o644); err != nil {
		// Best-effort: return nil, no panic.
		return nil
	}

	pointer := &RtkRawOutputPointer{
		ID:       id,
		Path:     logPath,
		Bytes:    len(redactedText),
		SHA256:   shaHex,
		Redacted: redacted,
	}

	// Register the path for ReadRtkRawOutput.
	rawOutputPaths.Store(id, logPath)

	// Write the sidecar .meta.json (best-effort: failure does not block).
	meta := struct {
		Command   string `json:"command"`
		Timestamp int64  `json:"timestamp"`
		Failure   bool   `json:"failure"`
		Redacted  bool   `json:"redacted"`
		Bytes     int    `json:"bytes"`
	}{
		Command:   opts.Command,
		Timestamp: ts,
		Failure:   failure,
		Redacted:  redacted,
		Bytes:     len(redactedText),
	}
	if metaData, err := json.Marshal(meta); err == nil {
		_ = os.WriteFile(metaPath, metaData, 0o644) // best-effort
	}

	return pointer
}

// ReadRtkRawOutput reads the persisted raw output file for the given pointer ID.
// It first checks the package-level registry, then falls back to a glob search
// under the current working directory. Returns the file content as a string, or
// empty string if the file cannot be found or read.
func ReadRtkRawOutput(pointerID string) string {
	if pointerID == "" {
		return ""
	}
	// Check the registry first.
	if v, ok := rawOutputPaths.Load(pointerID); ok {
		path, _ := v.(string)
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}
	// Fallback: glob search.
	// Try <cwd>/rtk/raw-output/*-<id>.log
	cwd, err := os.Getwd()
	if err == nil {
		pattern := filepath.Join(cwd, "rtk", "raw-output", "*"+pointerID+".log")
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			if data, err := os.ReadFile(matches[0]); err == nil {
				return string(data)
			}
		}
	}
	return ""
}

// reRawOutputID matches the 24-hex pointer ID produced by MaybePersistRtkRawOutput.
// Compile at package init for hot-path reuse. Case-insensitive so an
// operator pasting a SHA-256 prefix from a shell that uppercased it still
// resolves to the right file.
var reRawOutputID = regexp.MustCompile(`(?i)^[0-9a-f]{24}$`)

// IsValidRawOutputID reports whether the string matches the raw-output ID
// format (24 lowercase hex characters). Handlers should use this to validate
// path parameters before doing any disk lookup.
func IsValidRawOutputID(id string) bool {
	if id == "" {
		return false
	}
	return reRawOutputID.MatchString(id)
}

// ReadRtkRawOutputByID reads the persisted raw output by pointer ID, preferring
// the explicit appDir for path resolution when set. Falls back to the in-memory
// registry and finally a glob search under the current working directory, the
// same as ReadRtkRawOutput. Returns (data, found): when found is false the
// data is empty and callers should surface a 404 to the client.
func ReadRtkRawOutputByID(pointerID, appDir string) (string, bool) {
	if !IsValidRawOutputID(pointerID) {
		return "", false
	}

	// 1. Try the in-memory registry (most recent persist call sites it directly).
	if v, ok := rawOutputPaths.Load(pointerID); ok {
		path, _ := v.(string)
		if data, err := os.ReadFile(path); err == nil {
			return string(data), true
		}
	}

	// 2. Try under the provided appDir (production path — handler passes p.AppDir()).
	if appDir != "" {
		dir := filepath.Join(appDir, "rtk", "raw-output")
		if data, ok := readFirstMatch(filepath.Join(dir, "*-"+pointerID+".log")); ok {
			return data, true
		}
	}

	// 3. Fallback: glob under the current working directory.
	if cwd, err := os.Getwd(); err == nil {
		pattern := filepath.Join(cwd, "rtk", "raw-output", "*-"+pointerID+".log")
		if data, ok := readFirstMatch(pattern); ok {
			return data, true
		}
	}

	return "", false
}

// readFirstMatch performs a glob search and returns the content of the first
// matching file. Used by ReadRtkRawOutputByID to fan out over the available
// raw-output directories without exposing glob internals to callers.
func readFirstMatch(pattern string) (string, bool) {
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", false
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return "", false
	}
	return string(data), true
}

// safeUtf8Slice truncates the string to the given byte limit, ensuring the
// result is valid UTF-8 (backing off the last incomplete rune).
func safeUtf8Slice(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	// Fast path: if the byte at maxBytes is not a continuation byte, it's a
	// rune boundary. UTF-8 lead bytes never have the form 10xxxxxx.
	b := []byte(value)
	if maxBytes < len(b) && b[maxBytes]&0xC0 != 0x80 {
		return string(b[:maxBytes])
	}
	// Slow path: back off through the last rune until valid.
	slice := value[:maxBytes]
	for len(slice) > 0 && !utf8.ValidString(slice) {
		_, size := utf8.DecodeLastRuneInString(slice)
		if size <= 0 {
			slice = slice[:len(slice)-1]
		} else {
			slice = slice[:len(slice)-size]
		}
	}
	return slice
}

// commandSlug converts a shell command string into a filename-safe slug.
func commandSlug(command string) string {
	slug := strings.ToLower(strings.TrimSpace(command))
	slug = reSlugInvalid.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "unknown-command"
	}
	return slug
}