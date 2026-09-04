package logstore

import (
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/bytedance/sonic"
	"github.com/pin-gou/celer-route/core/schemas"
)

const maxMCPToolInputPreviewRunes = 200

// payloadFields lists the DB column names of large TEXT fields that are
// offloaded to object storage in hybrid mode. These fields are never needed
// for analytics queries (histograms, search, rankings) — only for individual
// log detail views (FindByID).
var payloadFields = []string{
	"input_history",
	"responses_input_history",
	"output_message",
	"responses_output",
	"embedding_output",
	"rerank_output",
	"ocr_input",
	"ocr_output",
	"params",
	"tools",
	"tool_calls",
	"speech_input",
	"transcription_input",
	"image_generation_input",
	"image_edit_input",
	"image_variation_input",
	"video_generation_input",
	"speech_output",
	"transcription_output",
	"image_generation_output",
	"list_models_output",
	"video_generation_output",
	"video_retrieve_output",
	"video_download_output",
	"video_list_output",
	"video_delete_output",
	"cache_debug",
	"guardrail_debug",
	"token_usage",
	"error_details",
	"raw_request",
	"raw_response",
	"passthrough_request_body",
	"passthrough_response_body",
	"routing_engine_logs",
}

// ExtractPayload reads the serialized TEXT payload fields from a Log into a map.
// The map keys are the DB column names.
func ExtractPayload(l *Log) map[string]string {
	m := make(map[string]string, len(payloadFields)+25)
	m["input_history"] = l.InputHistory
	m["responses_input_history"] = l.ResponsesInputHistory
	m["output_message"] = l.OutputMessage
	m["responses_output"] = l.ResponsesOutput
	m["embedding_output"] = l.EmbeddingOutput
	m["rerank_output"] = l.RerankOutput
	m["ocr_input"] = l.OCRInput
	m["ocr_output"] = l.OCROutput
	m["params"] = l.Params
	m["tools"] = l.Tools
	m["tool_calls"] = l.ToolCalls
	m["speech_input"] = l.SpeechInput
	m["transcription_input"] = l.TranscriptionInput
	m["image_generation_input"] = l.ImageGenerationInput
	m["image_edit_input"] = l.ImageEditInput
	m["image_variation_input"] = l.ImageVariationInput
	m["video_generation_input"] = l.VideoGenerationInput
	m["speech_output"] = l.SpeechOutput
	m["transcription_output"] = l.TranscriptionOutput
	m["image_generation_output"] = l.ImageGenerationOutput
	m["list_models_output"] = l.ListModelsOutput
	m["video_generation_output"] = l.VideoGenerationOutput
	m["video_retrieve_output"] = l.VideoRetrieveOutput
	m["video_download_output"] = l.VideoDownloadOutput
	m["video_list_output"] = l.VideoListOutput
	m["video_delete_output"] = l.VideoDeleteOutput
	m["cache_debug"] = l.CacheDebug
	m["guardrail_debug"] = l.GuardrailDebug
	m["token_usage"] = l.TokenUsage
	m["error_details"] = l.ErrorDetails
	m["raw_request"] = l.RawRequest
	m["raw_response"] = l.RawResponse
	m["passthrough_request_body"] = l.PassthroughRequestBody
	m["passthrough_response_body"] = l.PassthroughResponseBody
	m["routing_engine_logs"] = l.RoutingEngineLogs
	// Metadata is written to the snapshot so consumers reading objects
	// directly see custom attributes, but it is deliberately NOT part of
	// payloadFields: it must always stay DB-resident as well (filters,
	// rankings, log-list display), so ClearPayload never strips it from the
	// row. NOTE: the snapshot carries the metadata value as of upload time;
	// subsequent DB updates are NOT reflected in the object store.
	if l.Metadata != nil && *l.Metadata != "" {
		m["metadata"] = *l.Metadata
	}
	m["provider"] = l.Provider
	m["model"] = l.Model
	m["status"] = l.Status
	m["timestamp"] = l.Timestamp.Format(time.RFC3339Nano)
	m["selected_key_id"] = l.SelectedKeyID
	m["selected_key_name"] = l.SelectedKeyName
	putIfPresent(m, "virtual_key_id", l.VirtualKeyID)
	putIfPresent(m, "virtual_key_name", l.VirtualKeyName)
	putIfPresent(m, "user_id", l.UserID)
	putIfPresent(m, "user_name", l.UserName)
	putIfPresent(m, "team_id", l.TeamID)
	putIfPresent(m, "team_name", l.TeamName)
	putIfPresent(m, "team_ids", l.TeamIDs)
	putIfPresent(m, "team_names", l.TeamNames)
	putIfPresent(m, "customer_id", l.CustomerID)
	putIfPresent(m, "customer_name", l.CustomerName)
	putIfPresent(m, "customer_ids", l.CustomerIDs)
	putIfPresent(m, "customer_names", l.CustomerNames)
	putIfPresent(m, "business_unit_id", l.BusinessUnitID)
	putIfPresent(m, "business_unit_name", l.BusinessUnitName)
	putIfPresent(m, "business_unit_ids", l.BusinessUnitIDs)
	putIfPresent(m, "business_unit_names", l.BusinessUnitNames)
	if l.Cost != nil {
		m["cost"] = strconv.FormatFloat(*l.Cost, 'f', -1, 64)
	}
	if l.Latency != nil {
		m["latency"] = strconv.FormatFloat(*l.Latency, 'f', -1, 64)
	}
	return m
}

// putIfPresent sets the key only when v is non-nil and non-empty, so absent
// attribution stays absent rather than becoming an empty string.
func putIfPresent(m map[string]string, key string, v *string) {
	if v != nil && *v != "" {
		m[key] = *v
	}
}

// ClearPayload zeros out both the TEXT payload columns and the Parsed virtual
// fields on a Log struct. Clearing the Parsed fields is necessary to prevent
// GORM's BeforeCreate/SerializeFields from re-populating TEXT columns.
// After calling this, the struct only contains index-weight data suitable
// for a lightweight DB INSERT.
// BillingHydrationChunkSize is how many rows a cost recomputation may hydrate at
// once. Kept deliberately tiny: a hydrated row holds its whole offloaded payload,
// which can include full message histories and raw request/response bodies, so this
// is the knob that bounds a recompute worker's peak memory. Billing query pages are
// capped to the same size because DB-resident modality payloads are materialized by
// the query before object-store hydration begins.
const BillingHydrationChunkSize = 3

// BillingHydrationResult reports what one hydration pass actually did, so the caller
// does not have to infer it from the rows.
type BillingHydrationResult struct {
	// Hydrated lists the rows whose pricing inputs were fetched from object storage.
	// These are the only rows worth persisting via BulkBackfillBillingPayloads: every
	// other row's inputs already came from the database.
	Hydrated []string
	// Unpriceable lists rows whose inputs could not be recovered — for example, a
	// missing object or unavailable object storage. Callers must skip these rather
	// than price them from the lossy fallback.
	Unpriceable []string
}

// BillingPayloadBackfill carries pricing inputs recovered from object storage that are
// worth writing back into the row.
//
// Deliberately only the two small ones. token_usage is a few hundred bytes of counters
// and cache_debug a handful of cache-hit fields, so keeping them in the row is cheap and
// repairs legacy rows written before pricing metadata became unconditionally
// DB-resident. The modality payloads
// (image_generation_output above all, which carries base64 image bytes) are exactly what
// offloading exists to keep out of the database, so they stay in object storage and
// modality rows keep fetching.
type BillingPayloadBackfill struct {
	TokenUsage string
	CacheDebug string
}

// ReleaseBillingPayloads drops the payload fields from rows that have already been
// priced, so a batch never accumulates more than one chunk of them.
//
// Safe because pricing is the last thing that reads these: the recalc job keeps only
// the ID, timestamp and computed cost afterwards, and the rows are never written back
// (BulkUpdateCost takes an id → cost map).
func ReleaseBillingPayloads(logs []*Log) {
	for _, l := range logs {
		if l != nil {
			ClearPayload(l)
		}
	}
}

func ClearPayload(l *Log) {
	// Clear serialized TEXT columns.
	l.InputHistory = ""
	l.ResponsesInputHistory = ""
	l.OutputMessage = ""
	l.ResponsesOutput = ""
	l.EmbeddingOutput = ""
	l.RerankOutput = ""
	l.OCRInput = ""
	l.OCROutput = ""
	l.Params = ""
	l.Tools = ""
	l.ToolCalls = ""
	l.SpeechInput = ""
	l.TranscriptionInput = ""
	l.ImageGenerationInput = ""
	l.ImageEditInput = ""
	l.ImageVariationInput = ""
	l.VideoGenerationInput = ""
	l.SpeechOutput = ""
	l.TranscriptionOutput = ""
	l.ImageGenerationOutput = ""
	l.ListModelsOutput = ""
	l.VideoGenerationOutput = ""
	l.VideoRetrieveOutput = ""
	l.VideoDownloadOutput = ""
	l.VideoListOutput = ""
	l.VideoDeleteOutput = ""
	l.CacheDebug = ""
	l.GuardrailDebug = ""
	l.TokenUsage = ""
	l.ErrorDetails = ""
	l.RawRequest = ""
	l.RawResponse = ""
	l.PassthroughRequestBody = ""
	l.PassthroughResponseBody = ""
	l.RoutingEngineLogs = ""

	// Clear Parsed virtual fields so GORM's SerializeFields won't re-serialize them.
	l.InputHistoryParsed = nil
	l.ResponsesInputHistoryParsed = nil
	l.OutputMessageParsed = nil
	l.ResponsesOutputParsed = nil
	l.EmbeddingOutputParsed = nil
	l.RerankOutputParsed = nil
	l.OCRInputParsed = nil
	l.OCROutputParsed = nil
	l.ParamsParsed = nil
	l.ToolsParsed = nil
	l.ToolCallsParsed = nil
	l.SpeechInputParsed = nil
	l.TranscriptionInputParsed = nil
	l.ImageGenerationInputParsed = nil
	l.ImageEditInputParsed = nil
	l.ImageVariationInputParsed = nil
	l.VideoGenerationInputParsed = nil
	l.SpeechOutputParsed = nil
	l.TranscriptionOutputParsed = nil
	l.ImageGenerationOutputParsed = nil
	l.ListModelsOutputParsed = nil
	l.VideoGenerationOutputParsed = nil
	l.VideoRetrieveOutputParsed = nil
	l.VideoDownloadOutputParsed = nil
	l.VideoListOutputParsed = nil
	l.VideoDeleteOutputParsed = nil
	l.CacheDebugParsed = nil
	l.GuardrailDebugParsed = nil
	l.TokenUsageParsed = nil
	l.ErrorDetailsParsed = nil
}

// MergePayloadFromJSON takes a JSON payload (as marshaled by MarshalPayload)
// and merges the fields back into the Log struct's serialized TEXT columns,
// then calls DeserializeFields to populate the Parsed virtual fields.
func MergePayloadFromJSON(l *Log, data []byte) error {
	var m map[string]string
	if err := sonic.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("logstore: unmarshal payload: %w", err)
	}
	if v, ok := m["input_history"]; ok && v != "" {
		l.InputHistory = v
	}
	if v, ok := m["responses_input_history"]; ok && v != "" {
		l.ResponsesInputHistory = v
	}
	if v, ok := m["output_message"]; ok && v != "" {
		l.OutputMessage = v
	}
	if v, ok := m["responses_output"]; ok && v != "" {
		l.ResponsesOutput = v
	}
	if v, ok := m["embedding_output"]; ok && v != "" {
		l.EmbeddingOutput = v
	}
	if v, ok := m["rerank_output"]; ok && v != "" {
		l.RerankOutput = v
	}
	if v, ok := m["ocr_input"]; ok && v != "" {
		l.OCRInput = v
	}
	if v, ok := m["ocr_output"]; ok && v != "" {
		l.OCROutput = v
	}
	if v, ok := m["params"]; ok && v != "" {
		l.Params = v
	}
	if v, ok := m["tools"]; ok && v != "" {
		l.Tools = v
	}
	if v, ok := m["tool_calls"]; ok && v != "" {
		l.ToolCalls = v
	}
	if v, ok := m["speech_input"]; ok && v != "" {
		l.SpeechInput = v
	}
	if v, ok := m["transcription_input"]; ok && v != "" {
		l.TranscriptionInput = v
	}
	if v, ok := m["image_generation_input"]; ok && v != "" {
		l.ImageGenerationInput = v
	}
	if v, ok := m["image_edit_input"]; ok && v != "" {
		l.ImageEditInput = v
	}
	if v, ok := m["image_variation_input"]; ok && v != "" {
		l.ImageVariationInput = v
	}
	if v, ok := m["video_generation_input"]; ok && v != "" {
		l.VideoGenerationInput = v
	}
	if v, ok := m["speech_output"]; ok && v != "" {
		l.SpeechOutput = v
	}
	if v, ok := m["transcription_output"]; ok && v != "" {
		l.TranscriptionOutput = v
	}
	if v, ok := m["image_generation_output"]; ok && v != "" {
		l.ImageGenerationOutput = v
	}
	if v, ok := m["list_models_output"]; ok && v != "" {
		l.ListModelsOutput = v
	}
	if v, ok := m["video_generation_output"]; ok && v != "" {
		l.VideoGenerationOutput = v
	}
	if v, ok := m["video_retrieve_output"]; ok && v != "" {
		l.VideoRetrieveOutput = v
	}
	if v, ok := m["video_download_output"]; ok && v != "" {
		l.VideoDownloadOutput = v
	}
	if v, ok := m["video_list_output"]; ok && v != "" {
		l.VideoListOutput = v
	}
	if v, ok := m["video_delete_output"]; ok && v != "" {
		l.VideoDeleteOutput = v
	}
	if v, ok := m["cache_debug"]; ok && v != "" {
		l.CacheDebug = v
	}
	if v, ok := m["guardrail_debug"]; ok && v != "" {
		l.GuardrailDebug = v
	}
	if v, ok := m["token_usage"]; ok && v != "" {
		l.TokenUsage = v
	}
	if v, ok := m["error_details"]; ok && v != "" {
		l.ErrorDetails = v
	}
	if v, ok := m["raw_request"]; ok && v != "" {
		l.RawRequest = v
	}
	if v, ok := m["raw_response"]; ok && v != "" {
		l.RawResponse = v
	}
	if v, ok := m["passthrough_request_body"]; ok && v != "" {
		l.PassthroughRequestBody = v
	}
	if v, ok := m["passthrough_response_body"]; ok && v != "" {
		l.PassthroughResponseBody = v
	}
	if v, ok := m["routing_engine_logs"]; ok && v != "" {
		l.RoutingEngineLogs = v
	}
	// Metadata is intentionally NOT restored from the snapshot: the copy
	// written there (see ExtractPayload) is for external object consumers
	// only, and the DB row stays authoritative.
	if err := l.DeserializeFields(); err != nil {
		return err
	}
	// Rebuild content summary from freshly deserialized Parsed fields so it
	// reflects the correct data from object storage, not a potentially
	// corrupted DB value (e.g. from client/server encoding mismatch).
	l.ContentSummary = l.BuildContentSummary()
	return nil
}

// ExtractPayloadFiltered is like ExtractPayload but omits fields present in
// the excluded set. An empty/nil excluded map is equivalent to ExtractPayload.
func ExtractPayloadFiltered(l *Log, excluded map[string]struct{}) map[string]string {
	if len(excluded) == 0 {
		return ExtractPayload(l)
	}
	m := ExtractPayload(l)
	for f := range excluded {
		delete(m, f)
	}
	return m
}

// ClearPayloadFiltered zeros only the payload fields that are not present in
// the excluded set (i.e. the fields that will be sent to object storage).
// Fields in the excluded set stay in the DB and are left untouched.
// An empty/nil excluded map is equivalent to ClearPayload.
func ClearPayloadFiltered(l *Log, excluded map[string]struct{}) {
	if len(excluded) == 0 {
		ClearPayload(l)
		return
	}
	for _, f := range payloadFields {
		if _, skip := excluded[f]; !skip {
			clearPayloadField(l, f)
		}
	}
}

func MarshalPayload(payload map[string]string) ([]byte, error) {
	return sonic.Marshal(payload)
}

// MarshalMCPToolLogPayload serializes a full MCP tool log for object storage.
// The object-store copy is intentionally complete; the DB row is only a
// lightweight index plus a short input preview.
func MarshalMCPToolLogPayload(l *MCPToolLog) ([]byte, error) {
	payload := *l
	if err := payload.SerializeFields(); err != nil {
		return nil, err
	}
	_ = payload.DeserializeFields()
	return sonic.Marshal(&payload)
}

// MergeMCPToolLogPayloadFromJSON replaces an MCP tool log with the full object
// storage copy while preserving DB-local hydration state.
func MergeMCPToolLogPayloadFromJSON(l *MCPToolLog, data []byte) error {
	hasObject := l.HasObject
	virtualKey := l.VirtualKey
	redactionMapping := l.RedactionMapping

	var payload MCPToolLog
	if err := sonic.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("logstore: unmarshal MCP tool log payload: %w", err)
	}
	if err := payload.SerializeFields(); err != nil {
		return err
	}
	*l = payload
	l.HasObject = hasObject
	l.VirtualKey = virtualKey
	l.RedactionMapping = redactionMapping
	return nil
}

// PrepareMCPToolDBEntry converts an MCP tool log to the lightweight DB form
// used by hybrid storage. It preserves indexed/display fields and metadata,
// keeps only a 200-character JSON-string argument preview, and clears result
// and error payloads.
func PrepareMCPToolDBEntry(l *MCPToolLog) {
	preview := buildMCPToolInputPreview(l)
	l.ArgumentsParsed = preview
	l.Arguments = ""
	l.Result = ""
	l.ErrorDetails = ""
	l.ResultParsed = nil
	l.ErrorDetailsParsed = nil
	_ = l.SerializeFields()
}

func buildMCPToolInputPreview(l *MCPToolLog) string {
	if l.ArgumentsParsed != nil {
		if data, err := sonic.Marshal(l.ArgumentsParsed); err == nil {
			return truncateRunes(string(data), maxMCPToolInputPreviewRunes)
		}
	}
	if l.Arguments != "" {
		return truncateRunes(l.Arguments, maxMCPToolInputPreviewRunes)
	}
	return ""
}

// BuildInputContentSummary extracts the last user message text from input fields.
// This is used in hybrid mode for the content_summary column, which powers
// full-text search and serves as a display fallback in the log list table.
// Only the last message is kept — the full conversation history lives in
// object storage and is merged back on FindByID.
func (l *Log) BuildInputContentSummary() string {
	// Chat completions: last user message
	if idx := findLastUserMessageIndex(l.InputHistoryParsed); idx >= 0 {
		if text := extractChatMessageText(&l.InputHistoryParsed[idx]); text != "" {
			return text
		}
	}

	// Responses API: last user message
	for i := len(l.ResponsesInputHistoryParsed) - 1; i >= 0; i-- {
		if l.ResponsesInputHistoryParsed[i].Role != nil && *l.ResponsesInputHistoryParsed[i].Role == schemas.ResponsesInputMessageRoleUser {
			if text := extractResponsesMessageText(&l.ResponsesInputHistoryParsed[i]); text != "" {
				return text
			}
		}
	}

	// Speech input
	if l.SpeechInputParsed != nil && l.SpeechInputParsed.Input != "" {
		return l.SpeechInputParsed.Input
	}

	// Image generation input prompt
	if l.ImageGenerationInputParsed != nil && l.ImageGenerationInputParsed.Prompt != "" {
		return l.ImageGenerationInputParsed.Prompt
	}

	// Image edit input prompt
	if l.ImageEditInputParsed != nil && l.ImageEditInputParsed.Prompt != "" {
		return l.ImageEditInputParsed.Prompt
	}

	// Video generation input prompt
	if l.VideoGenerationInputParsed != nil && l.VideoGenerationInputParsed.Prompt != "" {
		return l.VideoGenerationInputParsed.Prompt
	}

	return ""
}

// extractChatMessageText returns the text content from a ChatMessage.
// It prefers ContentStr; falls back to the FIRST non-empty text ContentBlock.
//
// First-not-last matters because some providers (notably Anthropic on the
// celer-route wire shape) inline mid-conversation system messages as a
// trailing text block on the user turn. The last text block of the last
// user message then carries a "<system-reminder>..." envelope, and the row
// preview would surface the reminder instead of the user's actual prompt.
// The matching Go path used by the SSE /active/stream `message` field
// (chatMessageContentText in transports/celer-route-http/handlers/logging.go)
// already returns the first non-empty text block; this helper must agree.
func extractChatMessageText(msg *schemas.ChatMessage) string {
	if msg.Content == nil {
		return ""
	}
	if msg.Content.ContentStr != nil && *msg.Content.ContentStr != "" {
		return *msg.Content.ContentStr
	}
	if msg.Content.ContentBlocks != nil {
		for _, block := range msg.Content.ContentBlocks {
			if block.Text != nil && *block.Text != "" {
				return *block.Text
			}
		}
	}
	return ""
}

// extractResponsesMessageText returns the text content from a ResponsesMessage.
// It prefers ContentStr; falls back to the FIRST non-empty text ContentBlock.
// See extractChatMessageText for why first-not-last.
func extractResponsesMessageText(msg *schemas.ResponsesMessage) string {
	if msg.Content == nil {
		return ""
	}
	if msg.Content.ContentStr != nil && *msg.Content.ContentStr != "" {
		return *msg.Content.ContentStr
	}
	if msg.Content.ContentBlocks != nil {
		for _, block := range msg.Content.ContentBlocks {
			if block.Text != nil && *block.Text != "" {
				return *block.Text
			}
		}
	}
	return ""
}

// attachmentStrippedPlaceholder replaces attachment payloads (base64 data,
// image/file URLs, audio data) in the last-user-message preview kept in the
// DB row. The untouched original always lives in object storage.
const attachmentStrippedPlaceholder = "[attachment stripped]"

// stripChatMessageAttachments returns a copy of msg with attachment payloads
// replaced by attachmentStrippedPlaceholder so the DB preview stays
// lightweight. Copy-on-write: msg, its blocks slice, and nested structs are
// shared with the caller's entry and are never mutated.
func stripChatMessageAttachments(msg *schemas.ChatMessage) schemas.ChatMessage {
	out := *msg
	if msg.Content == nil || len(msg.Content.ContentBlocks) == 0 {
		return out
	}
	blocks := make([]schemas.ChatContentBlock, len(msg.Content.ContentBlocks))
	copy(blocks, msg.Content.ContentBlocks)
	for i := range blocks {
		if img := blocks[i].ImageURLStruct; img != nil && img.URL != "" {
			imgCopy := *img
			imgCopy.URL = attachmentStrippedPlaceholder
			blocks[i].ImageURLStruct = &imgCopy
		}
		if audio := blocks[i].InputAudio; audio != nil && audio.Data != "" {
			audioCopy := *audio
			audioCopy.Data = attachmentStrippedPlaceholder
			blocks[i].InputAudio = &audioCopy
		}
		if file := blocks[i].File; file != nil && (file.FileData != nil || file.FileURL != nil) {
			fileCopy := *file
			if fileCopy.FileData != nil {
				fileCopy.FileData = schemas.Ptr(attachmentStrippedPlaceholder)
			}
			if fileCopy.FileURL != nil {
				fileCopy.FileURL = schemas.Ptr(attachmentStrippedPlaceholder)
			}
			blocks[i].File = &fileCopy
		}
	}
	content := *msg.Content
	content.ContentBlocks = blocks
	out.Content = &content
	return out
}

// stripResponsesMessageAttachments mirrors stripChatMessageAttachments for
// the Responses API message shape. Copy-on-write for the same reason.
func stripResponsesMessageAttachments(msg *schemas.ResponsesMessage) schemas.ResponsesMessage {
	out := *msg
	if msg.Content == nil || len(msg.Content.ContentBlocks) == 0 {
		return out
	}
	blocks := make([]schemas.ResponsesMessageContentBlock, len(msg.Content.ContentBlocks))
	copy(blocks, msg.Content.ContentBlocks)
	for i := range blocks {
		if img := blocks[i].ResponsesInputMessageContentBlockImage; img != nil && img.ImageURL != nil && *img.ImageURL != "" {
			imgCopy := *img
			imgCopy.ImageURL = schemas.Ptr(attachmentStrippedPlaceholder)
			blocks[i].ResponsesInputMessageContentBlockImage = &imgCopy
		}
		if file := blocks[i].ResponsesInputMessageContentBlockFile; file != nil && (file.FileData != nil || file.FileURL != nil) {
			fileCopy := *file
			if fileCopy.FileData != nil {
				fileCopy.FileData = schemas.Ptr(attachmentStrippedPlaceholder)
			}
			if fileCopy.FileURL != nil {
				fileCopy.FileURL = schemas.Ptr(attachmentStrippedPlaceholder)
			}
			blocks[i].ResponsesInputMessageContentBlockFile = &fileCopy
		}
		if audio := blocks[i].Audio; audio != nil && audio.Data != "" {
			audioCopy := *audio
			audioCopy.Data = attachmentStrippedPlaceholder
			blocks[i].Audio = &audioCopy
		}
	}
	content := *msg.Content
	content.ContentBlocks = blocks
	out.Content = &content
	return out
}

// findLastUserMessageIndex returns the index of the last ChatMessage with
// role "user", or -1 if none exists. Used by both BuildInputContentSummary
// and prepareDBEntry to avoid scanning the slice twice.
func findLastUserMessageIndex(msgs []schemas.ChatMessage) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == schemas.ChatMessageRoleUser {
			return i
		}
	}
	return -1
}

// findLastUserResponsesMessageIndex returns the index of the last
// ResponsesMessage with role "user", or -1 if none exists. Mirrors
// findLastUserMessageIndex for the Responses API input history so prepareDBEntry
// can preserve a last-user-message preview when payloads are offloaded.
func findLastUserResponsesMessageIndex(msgs []schemas.ResponsesMessage) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != nil && *msgs[i].Role == schemas.ResponsesInputMessageRoleUser {
			return i
		}
	}
	return -1
}

// PrepareLastUserMessagePreview rewrites the input-history and content-summary
// fields on l so the DB row carries only the last user message (with
// attachments stripped) instead of the full conversation. Both the
// TEXT columns and the Parsed virtual fields are updated so the GORM
// BeforeCreate hook (which re-serializes Parsed into the TEXT columns just
// before INSERT) observes the lightweight form.
//
// Hybrid storage applies this projection before offloading payloads to
// object storage; the full conversation survives there. Pure RDB storage
// applies it so the input_history column is never NULL on chat-completion
// rows: the list-query projection (sqliteLastUserInputHistoryExpr on SQLite,
// the raw jsonb on Postgres) always finds a non-empty array, and the
// log-list preview agrees with the SSE /active/stream `message` field — both
// render the same last-user-message text.
//
// ContentHidden rows are left untouched so hidden logs keep no request
// content anywhere.
//
// Fields present in the excluded set are left untouched: a hybrid
// configuration that excludes input_history wants the full conversation
// kept in the DB row, so the projection must not collapse it to the last
// user message. ContentSummary is still rewritten to the last user message
// text because it powers the log-list "message" preview and full-text
// search, both of which expect a single-message preview rather than a
// concatenation of every turn.
//
// Idempotent across calls: when an outer caller (hybrid prepareDBEntry)
// has already projected the Parsed slice to a single element and the inner
// RDB write path calls this again with the same Parsed, the second call
// is a no-op for the projection (already single-element) but still
// re-derives ContentSummary, preserving any prior text the outer call
// wrote. The hybrid "exclude input_history" case is detected by the
// presence of multi-message Parsed: a single-element Parsed means an
// outer caller already projected; multi-element Parsed means the caller
// explicitly wants the full conversation in the DB row (skip projection
// even when excluded is nil, to preserve hybrid intent).
func PrepareLastUserMessagePreview(l *Log, excluded map[string]struct{}) {
	if l == nil || l.ContentHidden {
		return
	}
	_, inputHistoryExcluded := excluded["input_history"]
	_, responsesInputHistoryExcluded := excluded["responses_input_history"]

	idx := findLastUserMessageIndex(l.InputHistoryParsed)
	responsesIdx := findLastUserResponsesMessageIndex(l.ResponsesInputHistoryParsed)

	parsedEmpty := len(l.InputHistoryParsed) == 0 && len(l.ResponsesInputHistoryParsed) == 0
	if parsedEmpty && l.ContentSummary != "" {
		// Outer caller (hybrid prepareDBEntry) cleared Parsed after
		// snapshotting the projected JSON. Preserve its ContentSummary
		// rather than rebuilding from empty Parsed / TEXT.
		return
	}

	// ContentSummary: prefer the last chat user message's text so it matches
	// what the message-column preview shows. Falls back to the last
	// responses user message, then to BuildInputContentSummary for non-chat
	// inputs (speech / image / etc.).
	switch {
	case idx >= 0:
		l.ContentSummary = extractChatMessageText(&l.InputHistoryParsed[idx])
	case responsesIdx >= 0:
		l.ContentSummary = extractResponsesMessageText(&l.ResponsesInputHistoryParsed[responsesIdx])
	default:
		l.ContentSummary = extractChatMessageTextFromText(l.InputHistory)
		if l.ContentSummary == "" {
			l.ContentSummary = extractResponsesMessageTextFromText(l.ResponsesInputHistory)
		}
		if l.ContentSummary == "" {
			l.ContentSummary = l.BuildInputContentSummary()
		}
	}
	if l.ContentSummary == "" {
		l.ContentSummary = contentSummaryFromTextColumns(l)
	}
	l.ContentSummary = truncateTag(l.ContentSummary, maxContentSummaryBytes)

	// Project Parsed slices to a single last-user-message element so the
	// GORM BeforeCreate → SerializeFields chain writes the lightweight
	// form. Attachment payloads are replaced by [attachment stripped].
	//
	// Skip projection when:
	//   - The Parsed slice is already a single element (an outer caller
	//     projected; re-projecting would mutate the projected form's
	//     attachments back to the original base64 if the strip step is
	//     not idempotent, and there's nothing to gain otherwise).
	//   - The field is excluded but Parsed holds a multi-message
	//     conversation (the operator explicitly kept it DB-resident; we
	//     must not collapse to one element).
	if idx >= 0 && !inputHistoryExcluded && len(l.InputHistoryParsed) > 1 {
		stripped := stripChatMessageAttachments(&l.InputHistoryParsed[idx])
		l.InputHistoryParsed = []schemas.ChatMessage{stripped}
		if data, err := sonic.Marshal(l.InputHistoryParsed); err == nil {
			l.InputHistory = sanitizeJSONForJSONB(string(data))
		}
	} else if idx >= 0 && !inputHistoryExcluded && len(l.InputHistoryParsed) == 1 {
		// Already a single element from a prior call. Strip attachments on
		// the existing single element (idempotent if already stripped) so
		// the GORM re-serialization carries the placeholder, not base64.
		stripped := stripChatMessageAttachments(&l.InputHistoryParsed[idx])
		l.InputHistoryParsed = []schemas.ChatMessage{stripped}
		if data, err := sonic.Marshal(l.InputHistoryParsed); err == nil {
			l.InputHistory = sanitizeJSONForJSONB(string(data))
		}
	} else if inputHistoryExcluded && idx >= 0 && len(l.InputHistoryParsed) > 1 {
		// Excluded but TEXT still holds the full conversation: keep Parsed
		// intact (so GORM re-serialization stays multi-message) but resync
		// the TEXT column from Parsed in case the caller only set one.
		if data, err := sonic.Marshal(l.InputHistoryParsed); err == nil {
			l.InputHistory = sanitizeJSONForJSONB(string(data))
		}
	}
	if responsesIdx >= 0 && !responsesInputHistoryExcluded && len(l.ResponsesInputHistoryParsed) > 1 {
		stripped := stripResponsesMessageAttachments(&l.ResponsesInputHistoryParsed[responsesIdx])
		l.ResponsesInputHistoryParsed = []schemas.ResponsesMessage{stripped}
		if data, err := sonic.Marshal(l.ResponsesInputHistoryParsed); err == nil {
			l.ResponsesInputHistory = sanitizeJSONForJSONB(string(data))
		}
	} else if responsesIdx >= 0 && !responsesInputHistoryExcluded && len(l.ResponsesInputHistoryParsed) == 1 {
		stripped := stripResponsesMessageAttachments(&l.ResponsesInputHistoryParsed[responsesIdx])
		l.ResponsesInputHistoryParsed = []schemas.ResponsesMessage{stripped}
		if data, err := sonic.Marshal(l.ResponsesInputHistoryParsed); err == nil {
			l.ResponsesInputHistory = sanitizeJSONForJSONB(string(data))
		}
	} else if responsesInputHistoryExcluded && responsesIdx >= 0 && len(l.ResponsesInputHistoryParsed) > 1 {
		if data, err := sonic.Marshal(l.ResponsesInputHistoryParsed); err == nil {
			l.ResponsesInputHistory = sanitizeJSONForJSONB(string(data))
		}
	}
}

// textColumnIsProjectedChat reports whether the given TEXT column already
// encodes the projected single-element chat array (the shape
// PrepareLastUserMessagePreview would produce).
func textColumnIsProjectedChat(text string) bool {
	if text == "" {
		return false
	}
	var msgs []schemas.ChatMessage
	if err := sonic.Unmarshal([]byte(text), &msgs); err != nil {
		return false
	}
	return len(msgs) == 1
}

// textColumnIsProjectedResponses reports whether the given TEXT column
// already encodes the projected single-element Responses array.
func textColumnIsProjectedResponses(text string) bool {
	if text == "" {
		return false
	}
	var msgs []schemas.ResponsesMessage
	if err := sonic.Unmarshal([]byte(text), &msgs); err != nil {
		return false
	}
	return len(msgs) == 1
}

// extractChatMessageTextFromText reads a ChatMessage JSON array out of the
// given TEXT column and returns the last user-role message's text. Empty
// input (or non-array content) yields "".
func extractChatMessageTextFromText(text string) string {
	if text == "" {
		return ""
	}
	var msgs []schemas.ChatMessage
	if err := sonic.Unmarshal([]byte(text), &msgs); err != nil {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == schemas.ChatMessageRoleUser {
			return extractChatMessageText(&msgs[i])
		}
	}
	return ""
}

// extractResponsesMessageTextFromText reads a ResponsesMessage JSON array
// out of the given TEXT column and returns the last user-role message's
// text. Empty input (or non-array content) yields "".
func extractResponsesMessageTextFromText(text string) string {
	if text == "" {
		return ""
	}
	var msgs []schemas.ResponsesMessage
	if err := sonic.Unmarshal([]byte(text), &msgs); err != nil {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != nil && *msgs[i].Role == schemas.ResponsesInputMessageRoleUser {
			return extractResponsesMessageText(&msgs[i])
		}
	}
	return ""
}

// contentSummaryFromTextColumns reads speech / image / video TEXT columns
// directly to derive a preview when the Parsed virtual fields have already
// been cleared (hybrid prepareDBEntry zeroes them after snapshotting the
// projected JSON). Mirrors the input-side branches of BuildInputContentSummary
// that look at SpeechInputParsed / ImageGenerationInputParsed / etc., but
// operates on the TEXT column so it works post-offload.
func contentSummaryFromTextColumns(l *Log) string {
	if l.SpeechInput != "" {
		var s schemas.SpeechInput
		if err := sonic.Unmarshal([]byte(l.SpeechInput), &s); err == nil && s.Input != "" {
			return s.Input
		}
	}
	if l.ImageGenerationInput != "" {
		var v schemas.ImageGenerationInput
		if err := sonic.Unmarshal([]byte(l.ImageGenerationInput), &v); err == nil && v.Prompt != "" {
			return v.Prompt
		}
	}
	if l.ImageEditInput != "" {
		var v schemas.ImageEditInput
		if err := sonic.Unmarshal([]byte(l.ImageEditInput), &v); err == nil && v.Prompt != "" {
			return v.Prompt
		}
	}
	if l.VideoGenerationInput != "" {
		var v schemas.VideoGenerationInput
		if err := sonic.Unmarshal([]byte(l.VideoGenerationInput), &v); err == nil && v.Prompt != "" {
			return v.Prompt
		}
	}
	return ""
}

// projectChatInputHistoryToSingleElement returns a JSON-encoded
// single-element array containing the last user message from parsed (with
// attachments stripped). If parsed is empty, it deserializes fallback to
// recover the messages. The result is "" if no user message exists.
func projectChatInputHistoryToSingleElement(parsed []schemas.ChatMessage, fallback string) string {
	var msgs []schemas.ChatMessage
	if len(parsed) > 0 {
		msgs = parsed
	} else if fallback != "" {
		if err := sonic.Unmarshal([]byte(fallback), &msgs); err != nil {
			return ""
		}
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == schemas.ChatMessageRoleUser {
			stripped := stripChatMessageAttachments(&msgs[i])
			data, err := sonic.MarshalString([]schemas.ChatMessage{stripped})
			if err != nil {
				return ""
			}
			return sanitizeJSONForJSONB(data)
		}
	}
	return ""
}

// projectResponsesInputHistoryToSingleElement mirrors
// projectChatInputHistoryToSingleElement for the Responses API shape.
func projectResponsesInputHistoryToSingleElement(parsed []schemas.ResponsesMessage, fallback string) string {
	var msgs []schemas.ResponsesMessage
	if len(parsed) > 0 {
		msgs = parsed
	} else if fallback != "" {
		if err := sonic.Unmarshal([]byte(fallback), &msgs); err != nil {
			return ""
		}
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != nil && *msgs[i].Role == schemas.ResponsesInputMessageRoleUser {
			stripped := stripResponsesMessageAttachments(&msgs[i])
			data, err := sonic.MarshalString([]schemas.ResponsesMessage{stripped})
			if err != nil {
				return ""
			}
			return sanitizeJSONForJSONB(data)
		}
	}
	return ""
}

// BuildTags creates the S3 object tag map from a Log's index fields.
// S3 allows max 10 tags per object; chosen for lifecycle rules and
// S3 Metadata Tables queryability.
func BuildTags(l *Log) map[string]string {
	tags := make(map[string]string, 10)
	if l.Provider != "" {
		tags["provider"] = l.Provider
	}
	if l.Model != "" {
		tags["model"] = truncateTag(l.Model, 256)
	}
	if l.Status != "" {
		tags["status"] = l.Status
	}
	if l.Object != "" {
		tags["object_type"] = l.Object
	}
	if l.VirtualKeyID != nil && *l.VirtualKeyID != "" {
		tags["virtual_key_id"] = truncateTag(*l.VirtualKeyID, 256)
	}
	if l.SelectedKeyID != "" {
		tags["selected_key_id"] = truncateTag(l.SelectedKeyID, 256)
	}
	if l.RoutingRuleID != nil && *l.RoutingRuleID != "" {
		tags["routing_rule_id"] = truncateTag(*l.RoutingRuleID, 256)
	}
	if l.Stream {
		tags["stream"] = "true"
	} else {
		tags["stream"] = "false"
	}
	tags["has_error"] = "false"
	if l.Status == "error" {
		tags["has_error"] = "true"
	}
	tags["date"] = l.Timestamp.UTC().Format("2006-01-02")
	return tags
}

// BuildMCPToolTags creates the object tag map from an MCP tool log's index
// fields. S3 allows max 10 tags per object.
func BuildMCPToolTags(l *MCPToolLog) map[string]string {
	tags := make(map[string]string, 6)
	if l.ToolName != "" {
		tags["tool_name"] = truncateTag(l.ToolName, 256)
	}
	if l.ServerLabel != "" {
		tags["server_label"] = truncateTag(l.ServerLabel, 256)
	}
	if l.Status != "" {
		tags["status"] = l.Status
	}
	if l.VirtualKeyID != nil && *l.VirtualKeyID != "" {
		tags["virtual_key_id"] = truncateTag(*l.VirtualKeyID, 256)
	}
	tags["has_error"] = "false"
	if l.Status == "error" {
		tags["has_error"] = "true"
	}
	tags["date"] = l.Timestamp.UTC().Format("2006-01-02")
	return tags
}

// ObjectKey constructs the S3 object key for a log entry.
func ObjectKey(prefix string, timestamp time.Time, logID string) string {
	ts := timestamp.UTC()
	return fmt.Sprintf("%s/logs/%04d/%02d/%02d/%02d/%s.json.gz",
		prefix,
		ts.Year(), ts.Month(), ts.Day(), ts.Hour(),
		logID,
	)
}

// MCPToolObjectKey constructs the S3 object key for an MCP tool log entry.
func MCPToolObjectKey(prefix string, timestamp time.Time, logID string) string {
	ts := timestamp.UTC()
	return fmt.Sprintf("%s/mcp-logs/%04d/%02d/%02d/%02d/%s.json.gz",
		prefix,
		ts.Year(), ts.Month(), ts.Day(), ts.Hour(),
		logID,
	)
}

// PayloadFieldNames returns the list of DB column names that are payload fields.
func PayloadFieldNames() []string {
	cp := make([]string, len(payloadFields))
	copy(cp, payloadFields)
	return cp
}

// stripExemptPayloadFields are payload columns that the retention cleaner must NOT
// strip when aging out a row. token_usage is needed for cost recomputation (the
// denormalized integer token columns don't carry the full breakdown), and
// error_details preserves error-diagnosis context.
var stripExemptPayloadFields = map[string]struct{}{
	"token_usage":   {},
	"error_details": {},
}

// StripPayloadFieldNames returns the DB column names cleared by the retention
// cleaner when a log is stripped: all payload fields except token_usage and
// error_details. metadata and content_summary are not payload fields and are
// always retained.
func StripPayloadFieldNames() []string {
	fields := make([]string, 0, len(payloadFields)-len(stripExemptPayloadFields))
	for _, f := range payloadFields {
		if _, exempt := stripExemptPayloadFields[f]; exempt {
			continue
		}
		fields = append(fields, f)
	}
	return fields
}

// payloadFieldSet is a set for O(1) lookup of payload field names.
var payloadFieldSet = func() map[string]struct{} {
	s := make(map[string]struct{}, len(payloadFields))
	for _, f := range payloadFields {
		s[f] = struct{}{}
	}
	return s
}()

// fieldsNeedHydration returns true if any of the requested fields are
// payload fields that have been offloaded to object storage.
func fieldsNeedHydration(fields []string) bool {
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		if _, ok := payloadFieldSet[f]; ok {
			return true
		}
	}
	return false
}

// ensureHydrationFields appends id, timestamp, has_object, and content_hidden
// to the projection if not already present, so hydrateLog can function
// correctly. content_hidden must always be selected: a projection that omits
// it would zero-value the flag and hydrate a hidden log.
func ensureHydrationFields(fields []string) []string {
	required := [4]string{"id", "timestamp", "has_object", "content_hidden"}
	have := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		have[f] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; !ok {
			fields = append(fields, r)
		}
	}
	return fields
}

// pruneUnrequestedPayloadFields clears payload fields that were not in the
// caller's field projection. This ensures hydration doesn't break projection
// semantics by populating unrequested fields with large blobs.
// A nil/empty requestedFields means "no projection" — everything is kept.
func pruneUnrequestedPayloadFields(l *Log, requestedFields []string) {
	if len(requestedFields) == 0 {
		return
	}
	requested := make(map[string]struct{}, len(requestedFields))
	for _, f := range requestedFields {
		requested[f] = struct{}{}
	}
	for _, pf := range payloadFields {
		if _, ok := requested[pf]; !ok {
			clearPayloadField(l, pf)
		}
	}
}

// clearPayloadField zeros a single payload field (serialized TEXT column and
// its Parsed counterpart, if any) by column name.
func clearPayloadField(l *Log, name string) {
	switch name {
	case "input_history":
		l.InputHistory = ""
		l.InputHistoryParsed = nil
	case "responses_input_history":
		l.ResponsesInputHistory = ""
		l.ResponsesInputHistoryParsed = nil
	case "output_message":
		l.OutputMessage = ""
		l.OutputMessageParsed = nil
	case "responses_output":
		l.ResponsesOutput = ""
		l.ResponsesOutputParsed = nil
	case "embedding_output":
		l.EmbeddingOutput = ""
		l.EmbeddingOutputParsed = nil
	case "rerank_output":
		l.RerankOutput = ""
		l.RerankOutputParsed = nil
	case "ocr_input":
		l.OCRInput = ""
		l.OCRInputParsed = nil
	case "ocr_output":
		l.OCROutput = ""
		l.OCROutputParsed = nil
	case "params":
		l.Params = ""
		l.ParamsParsed = nil
	case "tools":
		l.Tools = ""
		l.ToolsParsed = nil
	case "tool_calls":
		l.ToolCalls = ""
		l.ToolCallsParsed = nil
	case "speech_input":
		l.SpeechInput = ""
		l.SpeechInputParsed = nil
	case "transcription_input":
		l.TranscriptionInput = ""
		l.TranscriptionInputParsed = nil
	case "image_generation_input":
		l.ImageGenerationInput = ""
		l.ImageGenerationInputParsed = nil
	case "image_edit_input":
		l.ImageEditInput = ""
		l.ImageEditInputParsed = nil
	case "image_variation_input":
		l.ImageVariationInput = ""
		l.ImageVariationInputParsed = nil
	case "video_generation_input":
		l.VideoGenerationInput = ""
		l.VideoGenerationInputParsed = nil
	case "speech_output":
		l.SpeechOutput = ""
		l.SpeechOutputParsed = nil
	case "transcription_output":
		l.TranscriptionOutput = ""
		l.TranscriptionOutputParsed = nil
	case "image_generation_output":
		l.ImageGenerationOutput = ""
		l.ImageGenerationOutputParsed = nil
	case "list_models_output":
		l.ListModelsOutput = ""
		l.ListModelsOutputParsed = nil
	case "video_generation_output":
		l.VideoGenerationOutput = ""
		l.VideoGenerationOutputParsed = nil
	case "video_retrieve_output":
		l.VideoRetrieveOutput = ""
		l.VideoRetrieveOutputParsed = nil
	case "video_download_output":
		l.VideoDownloadOutput = ""
		l.VideoDownloadOutputParsed = nil
	case "video_list_output":
		l.VideoListOutput = ""
		l.VideoListOutputParsed = nil
	case "video_delete_output":
		l.VideoDeleteOutput = ""
		l.VideoDeleteOutputParsed = nil
	case "cache_debug":
		l.CacheDebug = ""
		l.CacheDebugParsed = nil
	case "guardrail_debug":
		l.GuardrailDebug = ""
		l.GuardrailDebugParsed = nil
	case "token_usage":
		l.TokenUsage = ""
		l.TokenUsageParsed = nil
	case "error_details":
		l.ErrorDetails = ""
		l.ErrorDetailsParsed = nil
	case "raw_request":
		l.RawRequest = ""
	case "raw_response":
		l.RawResponse = ""
	case "passthrough_request_body":
		l.PassthroughRequestBody = ""
	case "passthrough_response_body":
		l.PassthroughResponseBody = ""
	case "routing_engine_logs":
		l.RoutingEngineLogs = ""
	}
}

// truncateTag ensures a tag value doesn't exceed the given max length.
func truncateTag(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Truncate at a rune boundary without exceeding maxLen bytes.
	byteLen := 0
	for _, r := range s {
		rl := utf8.RuneLen(r)
		if byteLen+rl > maxLen {
			break
		}
		byteLen += rl
	}
	return s[:byteLen]
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	for i := range s {
		if maxRunes == 0 {
			return s[:i]
		}
		maxRunes--
	}
	return s
}
