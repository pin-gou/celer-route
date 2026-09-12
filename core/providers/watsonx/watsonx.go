// Package watsonx implements the IBM watsonx.ai LLM provider.
package watsonx

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pin-gou/celer-route/core/providers/openai"
	providerUtils "github.com/pin-gou/celer-route/core/providers/utils"
	schemas "github.com/pin-gou/celer-route/core/schemas"
	"github.com/valyala/fasthttp"
)

// IAMTokenURL is the IBM Cloud IAM identity token endpoint used to exchange an
// API key for a short-lived bearer token (default expiry: 1 hour).
const IAMTokenURL = "https://iam.cloud.ibm.com/identity/token"

// DefaultIAMTokenTTLBuffer is the grace period subtracted from the token expiry
// before a refresh is forced, avoiding boundary races with the IAM server.
const DefaultIAMTokenTTLBuffer = 60 * time.Second

// iamTokenEntry holds a cached IAM bearer token with its expiry time.
type iamTokenEntry struct {
	token     string
	expiresAt time.Time
}

// WatsonxProvider implements the Provider interface for IBM watsonx's API.
type WatsonxProvider struct {
	logger              schemas.Logger        // Logger for provider operations
	client              *fasthttp.Client      // HTTP client for unary API requests (ReadTimeout bounds overall response)
	streamingClient     *fasthttp.Client      // HTTP client for streaming API requests (no ReadTimeout; idle governed by NewIdleTimeoutReader)
	networkConfig       schemas.NetworkConfig // Network configuration including extra headers
	sendBackRawRequest  bool                  // Whether to include raw request in BifrostResponse
	sendBackRawResponse bool                  // Whether to include raw response in BifrostResponse

	iamTokenMu sync.Mutex
	iamToken   *iamTokenEntry
}

// NewWatsonxProvider creates a new IBM watsonx provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts, concurrency limits, and optional proxy settings.
func NewWatsonxProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*WatsonxProvider, error) {
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

	// Configure proxy and retry policy
	client = providerUtils.ConfigureProxy(client, config.ProxyConfig, logger)
	client = providerUtils.ConfigureDialer(client, config.NetworkConfig.AllowPrivateNetwork)
	client = providerUtils.ConfigureTLS(client, config.NetworkConfig, logger)
	streamingClient := providerUtils.BuildStreamingClient(client)
	// Set default BaseURL if not provided
	if config.NetworkConfig.BaseURL == "" {
		config.NetworkConfig.BaseURL = "https://us-south.ml.cloud.ibm.com/ml/gateway/v1"
	}
	config.NetworkConfig.BaseURL = strings.TrimRight(config.NetworkConfig.BaseURL, "/")

	return &WatsonxProvider{
		logger:              logger,
		client:              client,
		streamingClient:     streamingClient,
		networkConfig:       config.NetworkConfig,
		sendBackRawRequest:  config.SendBackRawRequest,
		sendBackRawResponse: config.SendBackRawResponse,
	}, nil
}

// getIAMToken returns a valid IAM bearer token for the given key, refreshing the
// cached token via the IBM Cloud IAM endpoint when it is missing or near expiry.
// The token is cached per provider instance and shared across keys with the same
// value (keys are per-provider, so the cache is naturally scoped).
func (provider *WatsonxProvider) getIAMToken(ctx *schemas.BifrostContext, key schemas.Key) (string, *schemas.BifrostError) {
	apikey := key.Value.GetValue()
	if apikey == "" {
		return "", providerUtils.NewConfigurationError("watsonx API key is empty")
	}

	provider.iamTokenMu.Lock()
	defer provider.iamTokenMu.Unlock()

	if provider.iamToken != nil && time.Now().Before(provider.iamToken.expiresAt.Add(-DefaultIAMTokenTTLBuffer)) {
		return provider.iamToken.token, nil
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(IAMTokenURL)
	req.Header.SetMethod("POST")
	req.Header.SetContentType("application/x-www-form-urlencoded")
	req.SetBodyString("grant_type=urn:ibm:params:oauth:grant-type:apikey&apikey=" + apikey)

	_, bifrostErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if bifrostErr != nil {
		return "", bifrostErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return "", providerUtils.NewBifrostOperationError(fmt.Sprintf("watsonx IAM token request failed with status %d", resp.StatusCode()), nil)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := sonic.Unmarshal(resp.Body(), &tokenResp); err != nil {
		return "", providerUtils.NewBifrostOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}
	if tokenResp.AccessToken == "" {
		return "", providerUtils.NewBifrostOperationError("watsonx IAM token response missing access_token", nil)
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	provider.iamToken = &iamTokenEntry{
		token:     tokenResp.AccessToken,
		expiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	return provider.iamToken.token, nil
}

// watsonxAuthHeader returns the Authorization header carrying the IAM bearer
// token for the given key, fetching/refreshing the token if needed.
func (provider *WatsonxProvider) watsonxAuthHeader(ctx *schemas.BifrostContext, key schemas.Key) (map[string]string, *schemas.BifrostError) {
	token, bifrostErr := provider.getIAMToken(ctx, key)
	if bifrostErr != nil {
		return nil, bifrostErr
	}
	return map[string]string{"Authorization": "Bearer " + token}, nil
}

// GetProviderKey returns the provider identifier for IBM watsonx.
func (provider *WatsonxProvider) GetProviderKey() schemas.ModelProvider {
	return schemas.Watsonx
}

// ListModels performs a list models request to IBM watsonx's model gateway API.
// watsonx requires an IAM bearer token (not the raw API key), so each key's
// token is resolved before the models request is issued.
func (provider *WatsonxProvider) ListModels(ctx *schemas.BifrostContext, keys []schemas.Key, request *schemas.BifrostListModelsRequest) (*schemas.BifrostListModelsResponse, *schemas.BifrostError) {
	url := provider.networkConfig.BaseURL + providerUtils.GetPathFromContext(ctx, "/models")
	listModelsByKeyWrapper := func(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostListModelsRequest) (*schemas.BifrostListModelsResponse, *schemas.BifrostError) {
		token, bifrostErr := provider.getIAMToken(ctx, key)
		if bifrostErr != nil {
			return nil, bifrostErr
		}
		return provider.watsonxListModelsByKey(ctx, url, key, request.Unfiltered, token)
	}
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		listModelsByKeyWrapper,
	)
}

// watsonxListModelsByKey issues a single models request against the watsonx
// model gateway using the given IAM bearer token.
func (provider *WatsonxProvider) watsonxListModelsByKey(ctx *schemas.BifrostContext, url string, key schemas.Key, unfiltered bool, iamToken string) (*schemas.BifrostListModelsResponse, *schemas.BifrostError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(url)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	req.Header.Set("Authorization", "Bearer "+iamToken)

	latency, bifrostErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if bifrostErr != nil {
		return nil, bifrostErr
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		bifrostErr := openai.ParseOpenAIError(resp)
		return nil, providerUtils.SetErrorLatency(bifrostErr, latency)
	}

	responseBody := append([]byte(nil), resp.Body()...)

	openaiResponse := &openai.OpenAIListModelsResponse{}

	rawRequest, rawResponse, bifrostErr := providerUtils.HandleProviderResponse(responseBody, openaiResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if bifrostErr != nil {
		return nil, providerUtils.SetErrorLatency(bifrostErr, latency)
	}

	response := openaiResponse.ToBifrostListModelsResponse(provider.GetProviderKey(), key.Models, key.BlacklistedModels, key.Aliases, unfiltered)
	response.ExtraFields.Latency = latency.Milliseconds()
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// TextCompletion performs a text completion request to IBM watsonx's API.
// It formats the request, sends it to IBM watsonx, and processes the response.
// Returns a BifrostResponse containing the completion results or an error if the request fails.
func (provider *WatsonxProvider) TextCompletion(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostTextCompletionRequest) (*schemas.BifrostTextCompletionResponse, *schemas.BifrostError) {
	authHeader, bifrostErr := provider.watsonxAuthHeader(ctx, key)
	if bifrostErr != nil {
		return nil, bifrostErr
	}
	return openai.HandleOpenAITextCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/completions"),
		request,
		authHeader,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		nil,
		provider.logger,
	)
}

// TextCompletionStream performs a streaming text completion request to IBM watsonx's API.
// It formats the request, sends it to IBM watsonx, and processes the response.
// Returns a channel of BifrostStreamChunk objects or an error if the request fails.
func (provider *WatsonxProvider) TextCompletionStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostTextCompletionRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	authHeader, bifrostErr := provider.watsonxAuthHeader(ctx, key)
	if bifrostErr != nil {
		return nil, bifrostErr
	}
	return openai.HandleOpenAITextCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/completions"),
		request,
		authHeader,
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		postHookRunner,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// ChatCompletion performs a chat completion request to the IBM watsonx API.
func (provider *WatsonxProvider) ChatCompletion(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostChatRequest) (*schemas.BifrostChatResponse, *schemas.BifrostError) {
	authHeader, bifrostErr := provider.watsonxAuthHeader(ctx, key)
	if bifrostErr != nil {
		return nil, bifrostErr
	}
	return openai.HandleOpenAIChatCompletionRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/chat/completions"),
		request,
		authHeader,
		provider.networkConfig.ExtraHeaders,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		nil,
		nil,
		nil,
		provider.logger,
	)
}

// ChatCompletionStream performs a streaming chat completion request to the IBM watsonx API.
// It supports real-time streaming of responses using Server-Sent Events (SSE).
// Uses IBM watsonx's OpenAI-compatible streaming format.
// Returns a channel containing BifrostStreamChunk objects representing the stream or an error if the request fails.
func (provider *WatsonxProvider) ChatCompletionStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostChatRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	authHeader, bifrostErr := provider.watsonxAuthHeader(ctx, key)
	if bifrostErr != nil {
		return nil, bifrostErr
	}
	return openai.HandleOpenAIChatCompletionStreaming(
		ctx,
		provider.streamingClient,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/chat/completions"),
		request,
		authHeader,
		provider.networkConfig.ExtraHeaders,
		provider.networkConfig.StreamIdleTimeoutInSeconds,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		provider.GetProviderKey(),
		postHookRunner,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		provider.logger,
		postHookSpanFinalizer,
	)
}

// Responses performs a responses request to the IBM watsonx API by converting it to a chat completion.
func (provider *WatsonxProvider) Responses(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostResponsesRequest) (*schemas.BifrostResponsesResponse, *schemas.BifrostError) {
	chatResponse, err := provider.ChatCompletion(ctx, key, request.ToChatRequest())
	if err != nil {
		return nil, err
	}

	response := chatResponse.ToBifrostResponsesResponse()

	return response, nil
}

// ResponsesStream performs a streaming responses request to the IBM watsonx API.
func (provider *WatsonxProvider) ResponsesStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostResponsesRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	ctx.SetValue(schemas.BifrostContextKeyIsResponsesToChatCompletionFallback, true)
	return provider.ChatCompletionStream(
		ctx,
		postHookRunner,
		postHookSpanFinalizer,
		key,
		request.ToChatRequest(),
	)
}

// Embedding generates embeddings for the given input text(s).
// The input can be either a single string or a slice of strings for batch embedding.
// Returns a BifrostResponse containing the embedding(s) and any error that occurred.
func (provider *WatsonxProvider) Embedding(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostEmbeddingRequest) (*schemas.BifrostEmbeddingResponse, *schemas.BifrostError) {
	authHeader, bifrostErr := provider.watsonxAuthHeader(ctx, key)
	if bifrostErr != nil {
		return nil, bifrostErr
	}
	return openai.HandleOpenAIEmbeddingRequest(
		ctx,
		provider.client,
		provider.networkConfig.BaseURL+providerUtils.GetPathFromContext(ctx, "/embeddings"),
		request,
		authHeader,
		provider.networkConfig.ExtraHeaders,
		provider.GetProviderKey(),
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
		nil,
		provider.logger)
}

// Speech is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) Speech(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostSpeechRequest) (*schemas.BifrostSpeechResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, provider.GetProviderKey())
}

// Rerank is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) Rerank(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostRerankRequest) (*schemas.BifrostRerankResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.RerankRequest, provider.GetProviderKey())
}

// OCR is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) OCR(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostOCRRequest) (*schemas.BifrostOCRResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) SpeechStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostSpeechRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, provider.GetProviderKey())
}

// Transcription is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) Transcription(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostTranscriptionRequest) (*schemas.BifrostTranscriptionResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, provider.GetProviderKey())
}

// TranscriptionStream is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) TranscriptionStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostTranscriptionRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, provider.GetProviderKey())
}

// ImageGeneration is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ImageGeneration(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostImageGenerationRequest) (*schemas.BifrostImageGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationRequest, provider.GetProviderKey())
}

// ImageGenerationStream is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ImageGenerationStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostImageGenerationRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, provider.GetProviderKey())
}

// ImageEdit is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ImageEdit(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostImageEditRequest) (*schemas.BifrostImageGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditRequest, provider.GetProviderKey())
}

// ImageEditStream is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ImageEditStream(ctx *schemas.BifrostContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.BifrostImageEditRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ImageVariation(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostImageVariationRequest) (*schemas.BifrostImageGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageVariationRequest, provider.GetProviderKey())
}

// VideoGeneration is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) VideoGeneration(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoGenerationRequest) (*schemas.BifrostVideoGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) VideoRetrieve(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoRetrieveRequest) (*schemas.BifrostVideoGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) VideoDownload(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoDownloadRequest) (*schemas.BifrostVideoDownloadResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) VideoDelete(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoDeleteRequest) (*schemas.BifrostVideoDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) VideoList(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoListRequest) (*schemas.BifrostVideoListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) VideoRemix(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostVideoRemixRequest) (*schemas.BifrostVideoGenerationResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// BatchCreate is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) BatchCreate(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostBatchCreateRequest) (*schemas.BifrostBatchCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCreateRequest, provider.GetProviderKey())
}

// BatchList is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) BatchList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchListRequest) (*schemas.BifrostBatchListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchListRequest, provider.GetProviderKey())
}

// BatchRetrieve is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) BatchRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchRetrieveRequest) (*schemas.BifrostBatchRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchRetrieveRequest, provider.GetProviderKey())
}

// BatchCancel is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) BatchCancel(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchCancelRequest) (*schemas.BifrostBatchCancelResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchCancelRequest, provider.GetProviderKey())
}

// BatchDelete is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) BatchDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchDeleteRequest) (*schemas.BifrostBatchDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) BatchResults(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostBatchResultsRequest) (*schemas.BifrostBatchResultsResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchResultsRequest, provider.GetProviderKey())
}

// FileUpload is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) FileUpload(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostFileUploadRequest) (*schemas.BifrostFileUploadResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileUploadRequest, provider.GetProviderKey())
}

// FileList is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) FileList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileListRequest) (*schemas.BifrostFileListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileListRequest, provider.GetProviderKey())
}

// FileRetrieve is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) FileRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileRetrieveRequest) (*schemas.BifrostFileRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileRetrieveRequest, provider.GetProviderKey())
}

// FileDelete is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) FileDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileDeleteRequest) (*schemas.BifrostFileDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileDeleteRequest, provider.GetProviderKey())
}

// FileContent is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) FileContent(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostFileContentRequest) (*schemas.BifrostFileContentResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.FileContentRequest, provider.GetProviderKey())
}

// CountTokens is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) CountTokens(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostResponsesRequest) (*schemas.BifrostCountTokensResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CountTokensRequest, provider.GetProviderKey())
}

// Compaction is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) Compaction(ctx *schemas.BifrostContext, key schemas.Key, request *schemas.BifrostCompactionRequest) (*schemas.BifrostCompactionResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerCreate(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostContainerCreateRequest) (*schemas.BifrostContainerCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerListRequest) (*schemas.BifrostContainerListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerRetrieveRequest) (*schemas.BifrostContainerRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerDeleteRequest) (*schemas.BifrostContainerDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerFileCreate(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostContainerFileCreateRequest) (*schemas.BifrostContainerFileCreateResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerFileList(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileListRequest) (*schemas.BifrostContainerFileListResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerFileRetrieve(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileRetrieveRequest) (*schemas.BifrostContainerFileRetrieveResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerFileContent(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileContentRequest) (*schemas.BifrostContainerFileContentResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) ContainerFileDelete(_ *schemas.BifrostContext, _ []schemas.Key, _ *schemas.BifrostContainerFileDeleteRequest) (*schemas.BifrostContainerFileDeleteResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) Passthrough(_ *schemas.BifrostContext, _ schemas.Key, _ *schemas.BifrostPassthroughRequest) (*schemas.BifrostPassthroughResponse, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

// PassthroughStream is not supported by the IBM watsonx provider.
func (provider *WatsonxProvider) PassthroughStream(_ *schemas.BifrostContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.BifrostPassthroughRequest) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
