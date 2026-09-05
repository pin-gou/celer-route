package governance

import (
	"context"
	"testing"
	"time"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
	"github.com/pin-gou/celer-route/framework/configstore"
	configstoreTables "github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// extractMultimodalFlags unit tests
// ---------------------------------------------------------------------------

func TestExtractMultimodalFlags_ChatWithImage(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		multimodalChatTextBlock("what is this?"),
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "https://example.com/a.png"}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasImage)
	assert.False(t, flags.HasAudio)
	assert.False(t, flags.HasFile)
	assert.Equal(t, 1, flags.ImageCount)
	assert.Equal(t, 0, flags.AudioCount)
	assert.Equal(t, 0, flags.FileCount)
}

func TestExtractMultimodalFlags_ChatWithInputAudio(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeInputAudio, InputAudio: &schemas.ChatInputAudio{Data: "base64data", Format: ptrStr("mp3")}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasAudio)
	assert.False(t, flags.HasImage)
	assert.Equal(t, 1, flags.AudioCount)
}

func TestExtractMultimodalFlags_ChatWithFile(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeFile, File: &schemas.ChatInputFile{FileData: ptrStr("pdf-bytes"), FileType: ptrStr("application/pdf")}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasFile)
	assert.Equal(t, 1, flags.FileCount)
	assert.False(t, flags.HasImage)
	assert.False(t, flags.HasAudio)
}

func TestExtractMultimodalFlags_ChatWithContentStrOnly(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionRequest, nil)
	// Override content to a plain string message.
	text := "hello"
	req.ChatRequest.Input[0].Content = &schemas.ChatMessageContent{ContentStr: &text}

	flags := extractMultimodalFlags(req)
	assert.False(t, flags.HasImage)
	assert.False(t, flags.HasAudio)
	assert.False(t, flags.HasFile)
	assert.Equal(t, 0, flags.ImageCount+flags.AudioCount+flags.FileCount)
}

func TestExtractMultimodalFlags_ChatMixedContentCounts(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeText, Text: ptrStr("a")},
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u1"}},
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u2"}},
		{Type: schemas.ChatContentBlockTypeInputAudio, InputAudio: &schemas.ChatInputAudio{Data: "d", Format: ptrStr("wav")}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasImage)
	assert.True(t, flags.HasAudio)
	assert.Equal(t, 2, flags.ImageCount)
	assert.Equal(t, 1, flags.AudioCount)
}

func TestExtractMultimodalFlags_ChatStream(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionStreamRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u"}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasImage)
	assert.Equal(t, 1, flags.ImageCount)
}

func TestExtractMultimodalFlags_ResponsesWithInputImage(t *testing.T) {
	req := multimodalResponsesReq(schemas.ResponsesRequest, []schemas.ResponsesMessageContentBlock{
		{Type: schemas.ResponsesInputMessageContentBlockTypeImage, ResponsesInputMessageContentBlockImage: &schemas.ResponsesInputMessageContentBlockImage{ImageURL: ptrStr("https://example.com/a.png")}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasImage)
	assert.Equal(t, 1, flags.ImageCount)
	assert.False(t, flags.HasAudio)
}

func TestExtractMultimodalFlags_ResponsesWithInputAudioAndFile(t *testing.T) {
	req := multimodalResponsesReq(schemas.ResponsesRequest, []schemas.ResponsesMessageContentBlock{
		{Type: schemas.ResponsesInputMessageContentBlockTypeAudio, Audio: &schemas.ResponsesInputMessageContentBlockAudio{Data: "d", Format: "mp3"}},
		{Type: schemas.ResponsesInputMessageContentBlockTypeFile, ResponsesInputMessageContentBlockFile: &schemas.ResponsesInputMessageContentBlockFile{FileURL: ptrStr("https://example.com/x.pdf"), FileType: ptrStr("application/pdf")}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasAudio)
	assert.True(t, flags.HasFile)
	assert.Equal(t, 1, flags.AudioCount)
	assert.Equal(t, 1, flags.FileCount)
	assert.False(t, flags.HasImage)
}

func TestExtractMultimodalFlags_ResponsesWithContentStrOnly(t *testing.T) {
	req := multimodalResponsesReq(schemas.ResponsesRequest, nil)
	text := "hello"
	req.ResponsesRequest.Input[0].Content = &schemas.ResponsesMessageContent{ContentStr: &text}

	flags := extractMultimodalFlags(req)
	assert.False(t, flags.HasImage)
	assert.False(t, flags.HasAudio)
	assert.False(t, flags.HasFile)
}

func TestExtractMultimodalFlags_ResponsesStream(t *testing.T) {
	req := multimodalResponsesReq(schemas.ResponsesStreamRequest, []schemas.ResponsesMessageContentBlock{
		{Type: schemas.ResponsesInputMessageContentBlockTypeImage, ResponsesInputMessageContentBlockImage: &schemas.ResponsesInputMessageContentBlockImage{ImageURL: ptrStr("u")}},
	})

	flags := extractMultimodalFlags(req)
	assert.True(t, flags.HasImage)
	assert.Equal(t, 1, flags.ImageCount)
}

func TestExtractMultimodalFlags_NilAndUnsupported(t *testing.T) {
	assert.Equal(t, MultimodalFlags{}, extractMultimodalFlags(nil))

	req := &schemas.BifrostRequest{RequestType: schemas.EmbeddingRequest}
	assert.Equal(t, MultimodalFlags{}, extractMultimodalFlags(req))

	// Text completion carries no message blocks.
	req = &schemas.BifrostRequest{RequestType: schemas.TextCompletionRequest}
	assert.Equal(t, MultimodalFlags{}, extractMultimodalFlags(req))
}

// ---------------------------------------------------------------------------
// extractRoutingVariables injection
// ---------------------------------------------------------------------------

func TestExtractRoutingVariables_MultimodalVarsDefaultZero(t *testing.T) {
	ctx := &RoutingContext{Provider: schemas.OpenAI, Model: "gpt-4o"}

	variables, err := extractRoutingVariables(ctx, nil)
	require.NoError(t, err)

	assert.Equal(t, false, variables["has_image"])
	assert.Equal(t, false, variables["has_audio"])
	assert.Equal(t, false, variables["has_file"])
	assert.Equal(t, 0, variables["image_count"])
	assert.Equal(t, 0, variables["audio_count"])
	assert.Equal(t, 0, variables["file_count"])
}

func TestExtractRoutingVariables_MultimodalVarsFromRequest(t *testing.T) {
	req := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u"}},
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u2"}},
	})
	ctx := &RoutingContext{Provider: schemas.OpenAI, Model: "gpt-4o", Request: req}

	variables, err := extractRoutingVariables(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, true, variables["has_image"])
	assert.Equal(t, 2, variables["image_count"])
	assert.Equal(t, false, variables["has_audio"])
}

// ---------------------------------------------------------------------------
// End-to-end EvaluateRoutingRules with multimodal CEL expressions
// ---------------------------------------------------------------------------

func TestEvaluateRoutingRules_HasImageCELMatch(t *testing.T) {
	store, err := NewLocalGovernanceStore(context.Background(), NewMockLogger(), nil, &configstore.GovernanceConfig{}, nil)
	require.NoError(t, err)
	engine, err := NewRoutingEngine(store, NewMockLogger(), schemas.Ptr(10))
	require.NoError(t, err)

	rule := &configstoreTables.TableRoutingRule{
		ID:            "mm-img",
		Name:          "Multimodal Image Rule",
		CelExpression: "has_image == true",
		Targets: []configstoreTables.TableRoutingTarget{
			{Provider: bifrost.Ptr("azure"), Model: bifrost.Ptr("gpt-4-turbo"), Weight: 1.0},
		},
		Enabled:  bifrost.Ptr(true),
		Scope:    "global",
		Priority: 0,
	}
	require.NoError(t, store.UpdateRoutingRuleInMemory(context.Background(), rule))

	withImage := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u"}},
	})
	decision, err := engine.EvaluateRoutingRules(schemas.NewBifrostContext(context.Background(), time.Now()), &RoutingContext{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Request:  withImage,
	})
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "azure", decision.Provider)
	assert.Equal(t, "gpt-4-turbo", decision.Model)

	textOnly := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		multimodalChatTextBlock("plain text"),
	})
	decision, err = engine.EvaluateRoutingRules(schemas.NewBifrostContext(context.Background(), time.Now()), &RoutingContext{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Request:  textOnly,
	})
	require.NoError(t, err)
	assert.Nil(t, decision)
}

func TestEvaluateRoutingRules_ImageCountCELMatch(t *testing.T) {
	store, err := NewLocalGovernanceStore(context.Background(), NewMockLogger(), nil, &configstore.GovernanceConfig{}, nil)
	require.NoError(t, err)
	engine, err := NewRoutingEngine(store, NewMockLogger(), schemas.Ptr(10))
	require.NoError(t, err)

	rule := &configstoreTables.TableRoutingRule{
		ID:            "mm-img2",
		Name:          "Multi Image Rule",
		CelExpression: "image_count >= 2",
		Targets: []configstoreTables.TableRoutingTarget{
			{Provider: bifrost.Ptr("azure"), Model: bifrost.Ptr("gpt-4-turbo"), Weight: 1.0},
		},
		Enabled:  bifrost.Ptr(true),
		Scope:    "global",
		Priority: 0,
	}
	require.NoError(t, store.UpdateRoutingRuleInMemory(context.Background(), rule))

	multiImage := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u1"}},
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u2"}},
	})
	decision, err := engine.EvaluateRoutingRules(schemas.NewBifrostContext(context.Background(), time.Now()), &RoutingContext{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Request:  multiImage,
	})
	require.NoError(t, err)
	require.NotNil(t, decision)

	singleImage := multimodalChatReq(schemas.ChatCompletionRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u1"}},
	})
	decision, err = engine.EvaluateRoutingRules(schemas.NewBifrostContext(context.Background(), time.Now()), &RoutingContext{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Request:  singleImage,
	})
	require.NoError(t, err)
	assert.Nil(t, decision)
}

func TestEvaluateRoutingRules_StreamingMultimodalFlags(t *testing.T) {
	store, err := NewLocalGovernanceStore(context.Background(), NewMockLogger(), nil, &configstore.GovernanceConfig{}, nil)
	require.NoError(t, err)
	engine, err := NewRoutingEngine(store, NewMockLogger(), schemas.Ptr(10))
	require.NoError(t, err)

	rule := &configstoreTables.TableRoutingRule{
		ID:            "mm-stream",
		Name:          "Streaming Image Rule",
		CelExpression: "has_image == true",
		Targets: []configstoreTables.TableRoutingTarget{
			{Provider: bifrost.Ptr("azure"), Model: bifrost.Ptr("gpt-4-turbo"), Weight: 1.0},
		},
		Enabled:  bifrost.Ptr(true),
		Scope:    "global",
		Priority: 0,
	}
	require.NoError(t, store.UpdateRoutingRuleInMemory(context.Background(), rule))

	streamWithImage := multimodalChatReq(schemas.ChatCompletionStreamRequest, []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "u"}},
	})
	decision, err := engine.EvaluateRoutingRules(schemas.NewBifrostContext(context.Background(), time.Now()), &RoutingContext{
		Provider: schemas.OpenAI,
		Model:    "gpt-4o-mini",
		Request:  streamWithImage,
	})
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "azure", decision.Provider)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func multimodalChatReq(requestType schemas.RequestType, blocks []schemas.ChatContentBlock) *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		RequestType: requestType,
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o-mini",
			Input: []schemas.ChatMessage{
				{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentBlocks: blocks}},
			},
		},
	}
}

func multimodalResponsesReq(requestType schemas.RequestType, blocks []schemas.ResponsesMessageContentBlock) *schemas.BifrostRequest {
	role := schemas.ResponsesInputMessageRoleUser
	return &schemas.BifrostRequest{
		RequestType: requestType,
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o",
			Input: []schemas.ResponsesMessage{
				{Role: &role, Content: &schemas.ResponsesMessageContent{ContentBlocks: blocks}},
			},
		},
	}
}

func multimodalChatTextBlock(text string) schemas.ChatContentBlock {
	return schemas.ChatContentBlock{Type: schemas.ChatContentBlockTypeText, Text: &text}
}

func ptrStr(s string) *string {
	return &s
}
