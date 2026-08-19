// Package opencode implements the Opencode, Opencode Zen, and Opencode Go AI
// gateway providers. All three gateways expose the same API surface, differ only
// in default base URL and provider key, and share the same wire-format switch +
// request-rewrite pipeline. The format switch routes models in
// claudeFormatModels through the Anthropic-compatible /v1/messages endpoint
// (with x-api-key + anthropic-version headers) and the rest through
// /v1/chat/completions.
package opencode

import (
	"context"
	"strings"
	"time"

	"github.com/pin-gou/pg-gateway/core/providers/anthropic"
	"github.com/pin-gou/pg-gateway/core/providers/openai"
	providerUtils "github.com/pin-gou/pg-gateway/core/providers/utils"
	schemas "github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// opencodeProvider implements the Provider interface for Opencode Zen and Go gateways.
type opencodeProvider struct {
	providerKey         schemas.ModelProvider
	logger              schemas.Logger
	client              *fasthttp.Client
	streamingClient     *fasthttp.Client
	networkConfig       schemas.NetworkConfig
	sendBackRawRequest  bool
	sendBackRawResponse bool
}

// NewOpencodeProvider creates a new bare Opencode provider instance.
// The bare "opencode" identity aliases the same gateway as Zen
// (https://opencode.ai/zen/v1) but is registered as a separate provider so
// users can opt into the keyless/free-tier flow without an API key.
func NewOpencodeProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*opencodeProvider, error) {
	return newOpencodeProvider(config, schemas.Opencode, "https://opencode.ai/zen", logger)
}

// NewOpencodeZenProvider creates a new Opencode Zen provider instance.
// Zen is the pay-as-you-go gateway at https://opencode.ai/zen/v1.
func NewOpencodeZenProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*opencodeProvider, error) {
	return newOpencodeProvider(config, schemas.OpencodeZen, "https://opencode.ai/zen", logger)
}

// NewOpencodeGoProvider creates a new Opencode Go provider instance.
// Go is the subscription-based gateway at https://opencode.ai/zen/go/v1.
func NewOpencodeGoProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*opencodeProvider, error) {
	return newOpencodeProvider(config, schemas.OpencodeGo, "https://opencode.ai/zen/go", logger)
}

// newOpencodeProvider initializes the shared provider infrastructure.
func newOpencodeProvider(
	config *schemas.ProviderConfig,
	providerKey schemas.ModelProvider,
	defaultBaseURL string,
	logger schemas.Logger,
) (*opencodeProvider, error) {
	config.CheckAndSetDefaults()

	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)
	client := &fasthttp.Client{
		ReadTimeout:         requestTimeout,
		WriteTimeout:        requestTimeout,
		MaxConnsPerHost:     config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration: time.Second * time.Duration(config.NetworkConfig.KeepAliveTimeoutInSeconds),
		MaxConnWaitTimeout:  requestTimeout,
		MaxConnDuration:     time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:    fasthttp.FIFO,
	}

	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)

	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = defaultBaseURL
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &opencodeProvider{
		providerKey:         providerKey,
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier stored at construction time.
func (p *opencodeProvider) GetProviderKey() schemas.ModelProvider {
	return p.providerKey
}

// ListModels performs a list models request to the Opencode API.
// For the bare `opencode` (free/no-auth) provider, the response is filtered
// to only include models accessible without an API key (free models).
func (p *opencodeProvider) ListModels(ctx *schemas.BifrostContext, keys []schemas.Key, request *schemas.BifrostListModelsRequest) (*schemas.BifrostListModelsResponse, *schemas.BifrostError) {
	resp, bifrostErr := openai.HandleOpenAIListModelsRequest(
		ctx,
		p.client,
		request,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/models"),
		keys,
		p.networkConfig.ExtraHeaders,
		p.providerKey,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
	)
	if bifrostErr != nil {
		return resp, bifrostErr
	}
	if resp != nil && p.providerKey == schemas.Opencode {
		filtered := make([]schemas.Model, 0, len(resp.Data))
		for _, m := range resp.Data {
			if isFreeOpencodeModel(m.ID) {
				filtered = append(filtered, m)
			}
		}
		resp.Data = filtered
	}
	return resp, nil
}

// TextCompletion is not supported by Opencode.
func (p *opencodeProvider) TextCompletion(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostTextCompletionRequest) (*schemas.BifrostTextCompletionResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionRequest, p.GetProviderKey())
}

// TextCompletionStream is not supported by Opencode.
func (p *opencodeProvider) TextCompletionStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostTextCompletionRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TextCompletionStreamRequest, p.GetProviderKey())
}

// prepareOpencodeRequest applies the typed-level OpenCode request rewrites
// that mirror the OmniRoute OpencodeExecutor.transformRequest pipeline:
//
//  1. Effort-tier suffix rewrite (<model>-low|high|max → canonical model +
//     reasoning_effort) — see parseEffortLevel.
//  2. client_metadata strip — issue #1442, kimi-k2.6 / deepseek upstream 400.
//  3. tools > 128 truncation — some opencode-go backends reject larger arrays.
//  4. deepseek-v4-flash-free response_format json_schema → json_object downgrade.
//
// The returned request is always non-nil; if no rewrite is needed the original
// request is returned untouched.
func prepareOpencodeRequest(request *schemas.BifrostChatRequest) *schemas.BifrostChatRequest {
	if request == nil {
		return nil
	}

	prepared := *request

	// paramsCopied guards a lazy, one-time deep copy of ChatParameters so the
	// rewrites below never mutate the caller's request. Each mutator copies the
	// specific nested structure it touches (Reasoning, ExtraParams, Tools,
	// ResponseFormat) rather than the whole parameter tree.
	paramsCopied := false
	ensureParams := func() *schemas.ChatParameters {
		if !paramsCopied {
			if prepared.Params == nil {
				prepared.Params = &schemas.ChatParameters{}
			} else {
				paramsCopy := *prepared.Params
				prepared.Params = &paramsCopy
			}
			paramsCopied = true
		}
		return prepared.Params
	}

	// 1. Effort-tier suffix: rewrite the model and inject reasoning_effort
	//    only when the caller did not already set reasoning_effort.
	if base, effort, ok := parseEffortLevel(prepared.Model); ok {
		prepared.Model = base
		params := ensureParams()
		hasEffort := params.Reasoning != nil && params.Reasoning.Effort != nil && *params.Reasoning.Effort != ""
		if !hasEffort {
			reasoningCopy := &schemas.ChatReasoning{}
			if params.Reasoning != nil {
				rc := *params.Reasoning
				reasoningCopy = &rc
			}
			if reasoningCopy.Effort == nil {
				effortCopy := effort
				reasoningCopy.Effort = &effortCopy
			}
			params.Reasoning = reasoningCopy
		}
	}

	// 2. client_metadata strip from ExtraParams. This is the only path that
	//    survives the openai handler's marshaling of typed → wire JSON.
	if prepared.Params != nil && len(prepared.Params.ExtraParams) > 0 {
		if _, present := prepared.Params.ExtraParams["client_metadata"]; present {
			params := ensureParams()
			extraCopy := make(map[string]interface{}, len(params.ExtraParams))
			for k, v := range params.ExtraParams {
				extraCopy[k] = v
			}
			delete(extraCopy, "client_metadata")
			params.ExtraParams = extraCopy
		}
	}

	// 3. tools > 128 truncation.
	if prepared.Params != nil && len(prepared.Params.Tools) > maxToolsCount {
		params := ensureParams()
		truncated := make([]schemas.ChatTool, maxToolsCount)
		copy(truncated, params.Tools[:maxToolsCount])
		params.Tools = truncated
	}

	// 4. deepseek-v4-flash-free json_schema → json_object downgrade.
	if prepared.Model == deepSeekV4FlashFreeModel && prepared.Params != nil && prepared.Params.ResponseFormat != nil {
		if rfMap, ok := (*prepared.Params.ResponseFormat).(map[string]interface{}); ok {
			if rfType, _ := rfMap["type"].(string); rfType == "json_schema" {
				// ResponseFormat is *interface{}; wrap the new value so the
				// pointer type matches.
				params := ensureParams()
				downgraded := map[string]interface{}{"type": "json_object"}
				wrapped := interface{}(downgraded)
				params.ResponseFormat = &wrapped
			}
		}
	}

	return &prepared
}

// opencodeAnthropicHeaders builds the auth/version headers for the
// /v1/messages endpoint. Mirrors sgl/fireworks/anthropic auth conventions:
// x-api-key when a key is present plus anthropic-version.
func (p *opencodeProvider) opencodeAnthropicHeaders(key schemas.Key) map[string]string {
	headers := map[string]string{}
	if key.Value.GetValue() != "" {
		headers["x-api-key"] = key.Value.GetValue()
	}
	headers["anthropic-version"] = anthropicAPIVersion
	return headers
}

// ChatCompletion performs a chat completion request to the Opencode API.
//
// Models listed in claudeFormatModels (qwen3.7-*, minimax-m*, glm-5*, etc.)
// are routed through the Anthropic-compatible /v1/messages endpoint because
// they reject oa-compat payloads upstream. All other models use
// /v1/chat/completions.
func (p *opencodeProvider) ChatCompletion(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostChatRequest) (*schemas.BifrostChatResponse, *schemas.BifrostError) {
	prepared := prepareOpencodeRequest(request)

	if isClaudeFormatModel(prepared.Model) {
		return anthropic.HandleAnthropicChatCompletionRequest(
			ctx,
			p.client,
			p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/messages"),
			prepared,
			anthropic.AnthropicRequestBuildConfig{
				Provider:                  p.GetProviderKey(),
				ShouldSendBackRawRequest:  providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
				ShouldSendBackRawResponse: providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
			},
			p.opencodeAnthropicHeaders(key),
			p.networkConfig.ExtraHeaders,
			nil,
			p.logger,
		)
	}

	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		p.client,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		prepared,
		openai.BearerAuthHeader(key),
		p.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.GetProviderKey(),
		nil,
		parseOpencodeError,
		nil,
		p.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the Opencode API.
//
// Models listed in claudeFormatModels are routed through the Anthropic-compatible
// /v1/messages endpoint; the rest go through /v1/chat/completions.
func (p *opencodeProvider) ChatCompletionStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostChatRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	prepared := prepareOpencodeRequest(request)

	if isClaudeFormatModel(prepared.Model) {
		jsonBody, bifrostErr := anthropic.BuildAnthropicChatRequestBody(ctx, prepared, anthropic.AnthropicRequestBuildConfig{
			Provider:                  p.GetProviderKey(),
			IsStreaming:               true,
			ShouldSendBackRawRequest:  providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
			ShouldSendBackRawResponse: providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		})
		if bifrostErr != nil {
			return nil, bifrostErr
		}
		return anthropic.HandleAnthropicChatCompletionStreaming(
			ctx,
			p.streamingClient,
			p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/messages"),
			jsonBody,
			p.opencodeAnthropicHeaders(key),
			p.networkConfig.ExtraHeaders,
			p.networkConfig.StreamIdleTimeoutInSeconds,
			nil,
			providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
			providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
			p.providerKey,
			postHookRunner,
			nil,
			nil,
			p.logger,
			postHookSpanFinalizer,
		)
	}

	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		p.streamingClient,
		p.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/v1/chat/completions"),
		prepared,
		openai.BearerAuthHeader(key),
		p.networkConfig.ExtraHeaders,
		p.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, p.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, p.sendBackRawResponse),
		p.providerKey,
		postHookRunner,
		nil,
		nil,
		parseOpencodeError,
		nil,
		nil,
		nil,
		p.logger,
		postHookSpanFinalizer,
	)
}

// Responses performs a responses request to the Opencode API.
func (p *opencodeProvider) Responses(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostResponsesRequest) (*schemas.BifrostResponsesResponse, *schemas.BifrostError) {
	chatResponse, err := p.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}
	return chatResponse.ToBifrostResponsesResponse(), nil
}

// ResponsesStream performs a streaming responses request to the Opencode API.
func (p *opencodeProvider) ResponsesStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostResponsesRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	ctx.SetValue(schemas.BifrostContextKeyIsResponsesToChatCompletionFallback, true)
	return p.ChatCompletionStream(ctx, postHookRunner, postHookSpanFinalizer, key, request.ToChatRequest())
}

// Embedding is not supported by Opencode.
func (p *opencodeProvider) Embedding(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostEmbeddingRequest) (*schemas.BifrostEmbeddingResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.EmbeddingRequest, p.GetProviderKey())
}

// Rerank is not supported by Opencode.
func (p *opencodeProvider) Rerank(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostRerankRequest) (*schemas.BifrostRerankResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, p.GetProviderKey())
}

// OCR is not supported by Opencode.
func (p *opencodeProvider) OCR(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostOCRRequest) (*schemas.BifrostOCRResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, p.GetProviderKey())
}

// Speech is not supported by Opencode.
func (p *opencodeProvider) Speech(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostSpeechRequest) (*schemas.BifrostSpeechResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, p.GetProviderKey())
}

// SpeechStream is not supported by Opencode.
func (p *opencodeProvider) SpeechStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostSpeechRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, p.GetProviderKey())
}

// Transcription is not supported by Opencode.
func (p *opencodeProvider) Transcription(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostTranscriptionRequest) (*schemas.BifrostTranscriptionResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, p.GetProviderKey())
}

// TranscriptionStream is not supported by Opencode.
func (p *opencodeProvider) TranscriptionStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostTranscriptionRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, p.GetProviderKey())
}

// ImageGeneration is not supported by Opencode.
func (p *opencodeProvider) ImageGeneration(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostImageGenerationRequest) (*schemas.BifrostImageGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, p.GetProviderKey())
}

// ImageGenerationStream is not supported by Opencode.
func (p *opencodeProvider) ImageGenerationStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostImageGenerationRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, p.GetProviderKey())
}

// ImageEdit is not supported by Opencode.
func (p *opencodeProvider) ImageEdit(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostImageEditRequest) (*schemas.BifrostImageGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, p.GetProviderKey())
}

// ImageEditStream is not supported by Opencode.
func (p *opencodeProvider) ImageEditStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostImageEditRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, p.GetProviderKey())
}

// ImageVariation is not supported by Opencode.
func (p *opencodeProvider) ImageVariation(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostImageVariationRequest) (*schemas.BifrostImageGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, p.GetProviderKey())
}

// VideoGeneration is not supported by Opencode.
func (p *opencodeProvider) VideoGeneration(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoGenerationRequest) (*schemas.BifrostVideoGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, p.GetProviderKey())
}

// VideoRetrieve is not supported by Opencode.
func (p *opencodeProvider) VideoRetrieve(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoRetrieveRequest) (*schemas.BifrostVideoGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, p.GetProviderKey())
}

// VideoDownload is not supported by Opencode.
func (p *opencodeProvider) VideoDownload(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoDownloadRequest) (*schemas.BifrostVideoDownloadResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, p.GetProviderKey())
}

// VideoDelete is not supported by Opencode.
func (p *opencodeProvider) VideoDelete(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoDeleteRequest) (*schemas.BifrostVideoDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, p.GetProviderKey())
}

// VideoList is not supported by Opencode.
func (p *opencodeProvider) VideoList(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoListRequest) (*schemas.BifrostVideoListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, p.GetProviderKey())
}

// VideoRemix is not supported by Opencode.
func (p *opencodeProvider) VideoRemix(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoRemixRequest) (*schemas.BifrostVideoGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, p.GetProviderKey())
}

// BatchCreate is not supported by Opencode.
func (p *opencodeProvider) BatchCreate(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostBatchCreateRequest) (*schemas.BifrostBatchCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, p.GetProviderKey())
}

// BatchList is not supported by Opencode.
func (p *opencodeProvider) BatchList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchListRequest) (*schemas.BifrostBatchListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, p.GetProviderKey())
}

// BatchRetrieve is not supported by Opencode.
func (p *opencodeProvider) BatchRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchRetrieveRequest) (*schemas.BifrostBatchRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, p.GetProviderKey())
}

// BatchCancel is not supported by Opencode.
func (p *opencodeProvider) BatchCancel(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchCancelRequest) (*schemas.BifrostBatchCancelResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, p.GetProviderKey())
}

// BatchDelete is not supported by Opencode.
func (p *opencodeProvider) BatchDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchDeleteRequest) (*schemas.BifrostBatchDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, p.GetProviderKey())
}

// BatchResults is not supported by Opencode.
func (p *opencodeProvider) BatchResults(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchResultsRequest) (*schemas.BifrostBatchResultsResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, p.GetProviderKey())
}

// FileUpload is not supported by Opencode.
func (p *opencodeProvider) FileUpload(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostFileUploadRequest) (*schemas.BifrostFileUploadResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, p.GetProviderKey())
}

// FileList is not supported by Opencode.
func (p *opencodeProvider) FileList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileListRequest) (*schemas.BifrostFileListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, p.GetProviderKey())
}

// FileRetrieve is not supported by Opencode.
func (p *opencodeProvider) FileRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileRetrieveRequest) (*schemas.BifrostFileRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, p.GetProviderKey())
}

// FileDelete is not supported by Opencode.
func (p *opencodeProvider) FileDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileDeleteRequest) (*schemas.BifrostFileDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, p.GetProviderKey())
}

// FileContent is not supported by Opencode.
func (p *opencodeProvider) FileContent(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileContentRequest) (*schemas.BifrostFileContentResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, p.GetProviderKey())
}

// CountTokens is not supported by Opencode.
func (p *opencodeProvider) CountTokens(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostResponsesRequest) (*schemas.BifrostCountTokensResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, p.GetProviderKey())
}

// Compaction is not supported by Opencode.
func (p *opencodeProvider) Compaction(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostCompactionRequest) (*schemas.BifrostCompactionResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, p.GetProviderKey())
}

// ContainerCreate is not supported by Opencode.
func (p *opencodeProvider) ContainerCreate(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostContainerCreateRequest) (*schemas.BifrostContainerCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, p.GetProviderKey())
}

// ContainerList is not supported by Opencode.
func (p *opencodeProvider) ContainerList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerListRequest) (*schemas.BifrostContainerListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, p.GetProviderKey())
}

// ContainerRetrieve is not supported by Opencode.
func (p *opencodeProvider) ContainerRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerRetrieveRequest) (*schemas.BifrostContainerRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, p.GetProviderKey())
}

// ContainerDelete is not supported by Opencode.
func (p *opencodeProvider) ContainerDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerDeleteRequest) (*schemas.BifrostContainerDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, p.GetProviderKey())
}

// ContainerFileCreate is not supported by Opencode.
func (p *opencodeProvider) ContainerFileCreate(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostContainerFileCreateRequest) (*schemas.BifrostContainerFileCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, p.GetProviderKey())
}

// ContainerFileList is not supported by Opencode.
func (p *opencodeProvider) ContainerFileList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileListRequest) (*schemas.BifrostContainerFileListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, p.GetProviderKey())
}

// ContainerFileRetrieve is not supported by Opencode.
func (p *opencodeProvider) ContainerFileRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileRetrieveRequest) (*schemas.BifrostContainerFileRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, p.GetProviderKey())
}

// ContainerFileContent is not supported by Opencode.
func (p *opencodeProvider) ContainerFileContent(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileContentRequest) (*schemas.BifrostContainerFileContentResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, p.GetProviderKey())
}

// ContainerFileDelete is not supported by Opencode.
func (p *opencodeProvider) ContainerFileDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileDeleteRequest) (*schemas.BifrostContainerFileDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, p.GetProviderKey())
}

// Passthrough is not supported by Opencode.
func (p *opencodeProvider) Passthrough(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostPassthroughRequest) (*schemas.BifrostPassthroughResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, p.GetProviderKey())
}

// PassthroughStream is not supported by Opencode.
func (p *opencodeProvider) PassthroughStream(_ *schemas.BifrostContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.BifrostPassthroughRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, p.GetProviderKey())
}
