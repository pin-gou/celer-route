package logging

import (
	"testing"

	"github.com/pin-gou/celer-route/core/schemas"
)

// TestRtkHintOffsets verifies that the authoritative hint-offset detection the
// logging plugin records (rtk_input_hint_offset / rtk_responses_input_hint_offset)
// matches the RTK recovery hint at the head of the persisted input arrays, and
// is 0 when the arrays start with an ordinary message.
func TestRtkHintOffsets(t *testing.T) {
	hintText := schemas.RTKRecoveryHintMarker + " celer\u2011route (LLM gateway) RTK compression can truncate tool_result blocks."

	t.Run("chat_hint_at_head", func(t *testing.T) {
		input := []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: &hintText}},
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: strPtr2("hi")}},
		}
		if got := rtkChatHintOffset(input); got != 1 {
			t.Fatalf("rtkChatHintOffset = %d, want 1", got)
		}
	})

	t.Run("chat_no_hint", func(t *testing.T) {
		sys := "You are a helpful assistant"
		input := []schemas.ChatMessage{
			{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: &sys}},
			{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: strPtr2("hi")}},
		}
		if got := rtkChatHintOffset(input); got != 0 {
			t.Fatalf("rtkChatHintOffset = %d, want 0", got)
		}
	})

	t.Run("chat_empty", func(t *testing.T) {
		if got := rtkChatHintOffset(nil); got != 0 {
			t.Fatalf("rtkChatHintOffset(nil) = %d, want 0", got)
		}
	})

	t.Run("responses_hint_at_head", func(t *testing.T) {
		msgType := schemas.ResponsesMessageTypeMessage
		role := schemas.ResponsesInputMessageRoleSystem
		input := []schemas.ResponsesMessage{
			{Type: &msgType, Role: &role, Content: &schemas.ResponsesMessageContent{ContentStr: &hintText}},
			{Type: &msgType, Content: &schemas.ResponsesMessageContent{}},
		}
		if got := rtkResponsesHintOffset(input); got != 1 {
			t.Fatalf("rtkResponsesHintOffset = %d, want 1", got)
		}
	})

	t.Run("responses_no_hint", func(t *testing.T) {
		msgType := schemas.ResponsesMessageTypeMessage
		role := schemas.ResponsesInputMessageRoleUser
		user := "hi"
		input := []schemas.ResponsesMessage{
			{Type: &msgType, Role: &role, Content: &schemas.ResponsesMessageContent{ContentStr: &user}},
		}
		if got := rtkResponsesHintOffset(input); got != 0 {
			t.Fatalf("rtkResponsesHintOffset = %d, want 0", got)
		}
	})
}

func strPtr2(s string) *string { return &s }