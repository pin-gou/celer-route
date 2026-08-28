package rtk

import (
	"encoding/json"
	"fmt"

	"github.com/pin-gou/celer-route/core/schemas"
)

// SnapshotEntry is one tool-message snapshot captured by the RTK pipeline
// for the log detail view's diff comparison. It records the message index in
// the input slice, the role/tool name, and the text content as it stood
// before compression. The compressed counterpart is read directly from the
// (now in-place-mutated) request by the log detail view, so it does not
// need to be stored here. OriginalTokens lets the UI render per-message
// compression ratios in split mode by pairing against the token count it
// re-derives from the post-compression message body.
type SnapshotEntry struct {
	Index          int    `json:"index"`
	Role           string `json:"role"`
	Name           string `json:"name,omitempty"`
	Content        string `json:"content"`
	OriginalTokens int    `json:"originalTokens,omitempty"`
}

// SnapshotPayload is the wire shape written into log metadata as
// rtk_original_snapshot. The Mode field lets the UI choose how to render
// the diff (split → side-by-side per-message; merged → single combined
// block).
type SnapshotPayload struct {
	Mode      string          `json:"mode"`
	Truncated bool            `json:"truncated"`
	Items     []SnapshotEntry `json:"items"`
}

// appendSnapshot records the original text of a message that is about to be
// compressed (or, for entries that failed the compression threshold, the
// unchanged text). It is a no-op when the original text is empty.
func appendSnapshot(state *CompressionState, index int, role, name, content string) {
	if content == "" {
		return
	}
	if state.OriginalSnapshot == nil {
		state.OriginalSnapshot = make([]SnapshotEntry, 0, 4)
	}
	state.OriginalSnapshot = append(state.OriginalSnapshot, SnapshotEntry{
		Index:          index,
		Role:           role,
		Name:           name,
		Content:        content,
		OriginalTokens: estimateTokens(content),
	})
}

// buildSnapshot serialises the pre-compression snapshot stored on the
// CompressionState into the wire format expected by the log detail view.
// When mode is "merged", every entry is collapsed into a single content
// block so the UI can render one big diff rather than a list of
// per-message diffs. When mode is "off", no payload is returned. When the
// state carries no entries at all, nil is returned so callers can detect
// "no snapshot captured" without parsing JSON.
//
// The compressed side is intentionally omitted — the log detail view
// derives it from the (mutated) request body, so duplicating it here
// would only inflate the metadata payload.
func buildSnapshot(state *CompressionState, mode string, maxBytes int) json.RawMessage {
	if state == nil || mode == "off" {
		return nil
	}
	if len(state.OriginalSnapshot) == 0 {
		return nil
	}

	original := buildSnapshotPayload(state.OriginalSnapshot, mode)

	// Byte-budget guard. If the payload exceeds the configured cap, mark
	// truncated=true and shrink the items slice until it fits. We always
	// retain at least one item so the UI can show *something*.
	original = clampSnapshotPayload(original, maxBytes)

	return original
}

func buildSnapshotPayload(entries []SnapshotEntry, mode string) json.RawMessage {
	if len(entries) == 0 {
		return mustMarshalJSON(SnapshotPayload{Mode: mode, Items: []SnapshotEntry{}})
	}
	if mode == "merged" {
		var content string
		for i, e := range entries {
			if i > 0 {
				content += "\n\n"
			}
			content += e.Content
		}
		merged := SnapshotEntry{
			Index:   -1,
			Role:    "tool",
			Content: content,
		}
		return mustMarshalJSON(SnapshotPayload{
			Mode:  mode,
			Items: []SnapshotEntry{merged},
		})
	}
	return mustMarshalJSON(SnapshotPayload{Mode: mode, Items: entries})
}

// clampSnapshotPayload trims Items from the end until the payload fits within
// maxBytes, then re-marshals. When trimming was required, the Truncated flag
// is set so the UI can render a banner. The function never returns nil for a
// non-empty input — at minimum one item survives.
func clampSnapshotPayload(raw json.RawMessage, maxBytes int) json.RawMessage {
	if len(raw) == 0 || maxBytes <= 0 || len(raw) <= maxBytes {
		return raw
	}
	var p SnapshotPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return raw
	}
	for len(p.Items) > 1 {
		p.Items = p.Items[:len(p.Items)-1]
		re := mustMarshalJSON(p)
		if len(re) <= maxBytes {
			p.Truncated = true
			return re
		}
	}
	p.Truncated = true
	return mustMarshalJSON(p)
}

func mustMarshalJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		// Fall back to an empty object so downstream code never sees nil
		// shape ambiguity. The Truncated flag will not be set in this
		// pathological case.
		return json.RawMessage("{}")
	}
	return b
}

// extractToolName returns the tool name hint for a message, falling back to
// the role string when neither Name nor ToolCallID is set. Used by the
// snapshot builder to record which tool emitted each compressed block
// (e.g. "bash" vs "shell"). The function never panics: a nil msg yields "".
func extractToolName(msg *schemas.ChatMessage) string {
	if msg == nil {
		return ""
	}
	// Defensive: the snapshot path is best-effort. We never want a malformed
	// pointer to abort the compression pipeline, so swallow any panic and
	// fall through to the role string. Logging this would require a logger
	// dependency; the caller already records the per-request snapshot so a
	// dropped label is recoverable from the metadata.
	defer func() { _ = recover() }()
	if msg.Name != nil && *msg.Name != "" {
		return *msg.Name
	}
	if msg.ToolCallID != nil && *msg.ToolCallID != "" {
		return fmt.Sprintf("tool:%s", *msg.ToolCallID)
	}
	return string(msg.Role)
}
