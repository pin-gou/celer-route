// Package handlers provides HTTP request handlers for the Bifrost HTTP transport.
// This file contains all provider management functionality including CRUD operations.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	bifrost "github.com/pin-gou/pg-gateway/core"
	"github.com/pin-gou/pg-gateway/core/schemas"
	"github.com/pin-gou/pg-gateway/framework/configstore"
	"github.com/pin-gou/pg-gateway/framework/configstore/tables"
	"github.com/pin-gou/pg-gateway/framework/modelcatalog"
	governanceplugin "github.com/pin-gou/pg-gateway/plugins/governance"
	"github.com/pin-gou/pg-gateway/transports/pg-gateway-http/lib"
	"github.com/valyala/fasthttp"
	"golang.org/x/sync/errgroup"
)

// ModelsManager defines the interface for managing provider models
type ModelsManager interface {
	ReloadProvider(ctx context.Context, provider schemas.ModelProvider) (*tables.TableProvider, error)
	RemoveProvider(ctx context.Context, provider schemas.ModelProvider) error
	GetModelsForProvider(provider schemas.ModelProvider) []string
	GetUnfilteredModelsForProvider(provider schemas.ModelProvider) []string
	UpsertModelPricingAttributes(ctx context.Context, entries []ModelPricingAttributesEntry) error
	OnKeyAdded(ctx context.Context, provider schemas.ModelProvider, key schemas.Key) error
	OnKeyUpdated(ctx context.Context, provider schemas.ModelProvider, key schemas.Key) error
	OnKeyDeleted(ctx context.Context, provider schemas.ModelProvider, keyID string) error
	// RefreshLiveModelsForKey re-fetches list-models for a single key on
	// demand. Returns ErrRefreshInProgress when the provider is already being
	// refreshed.
	RefreshLiveModelsForKey(ctx context.Context, provider schemas.ModelProvider, keyID string) error
	// RefreshLiveModelsForAllKeys re-fetches list-models across every enabled
	// key of the provider on demand. Returns ErrRefreshInProgress when the
	// provider is already being refreshed.
	RefreshLiveModelsForAllKeys(ctx context.Context, provider schemas.ModelProvider) error
}

// ErrRefreshInProgress is returned by the on-demand model refresh entrypoints
// when a refresh for the same provider is already running. Repeated presses of
// the UI refresh button collapse into the in-flight pass rather than each
// spawning their own (enabled keys x 2) burst of upstream calls.
var ErrRefreshInProgress = errors.New("model refresh already in progress for this provider")

// ModelPricingAttributesEntry is the wire shape for PUT /api/models/catalog.
// (model, provider) is the natural key on governance_model_pricing. When the
// pricing row does not exist yet, an explicit Mode seeds one (so a model not
// discovered by a provider key can still be registered); otherwise everything
// except AdditionalAttributes is ignored for an existing row.
type ModelPricingAttributesEntry struct {
	Model                string            `json:"model"`
	Provider             string            `json:"provider"`
	Mode                 string            `json:"mode,omitempty"`
	AdditionalAttributes map[string]string `json:"additional_attributes,omitempty"`
}

// ProviderHandler manages HTTP requests for provider operations
type ProviderHandler struct {
	dbStore       configstore.ConfigStore
	inMemoryStore *lib.Config
	client        *bifrost.Bifrost
	modelsManager ModelsManager
	logStats      ProviderLogStats
}

// NewProviderHandler creates a new provider handler instance
func NewProviderHandler(modelsManager ModelsManager, inMemoryStore *lib.Config, client *bifrost.Bifrost) *ProviderHandler {
	h := &ProviderHandler{
		dbStore:       inMemoryStore.ConfigStore,
		inMemoryStore: inMemoryStore,
		client:        client,
		modelsManager: modelsManager,
	}
	if inMemoryStore != nil && inMemoryStore.LogsStore != nil {
		h.logStats = newLogStatsFromLogStore(inMemoryStore.LogsStore)
	}
	return h
}

type ProviderStatus = string

const (
	ProviderStatusActive  ProviderStatus = "active"  // Provider is active and working
	ProviderStatusError   ProviderStatus = "error"   // Provider failed to initialize
	ProviderStatusDeleted ProviderStatus = "deleted" // Provider is deleted from the store
)

// ProviderResponse represents the response for provider operations
type ProviderResponse struct {
	Name                     schemas.ModelProvider            `json:"name"`
	NetworkConfig            schemas.NetworkConfig            `json:"network_config"`                   // Network-related settings
	ConcurrencyAndBufferSize schemas.ConcurrencyAndBufferSize `json:"concurrency_and_buffer_size"`      // Concurrency settings
	ProxyConfig              *schemas.ProxyConfig             `json:"proxy_config"`                     // Proxy configuration
	SendBackRawRequest       bool                             `json:"send_back_raw_request"`            // Include raw request in BifrostResponse
	SendBackRawResponse      bool                             `json:"send_back_raw_response"`           // Include raw response in BifrostResponse
	StoreRawRequestResponse  bool                             `json:"store_raw_request_response"`       // Capture raw request/response for internal logging only
	CustomProviderConfig     *schemas.CustomProviderConfig    `json:"custom_provider_config,omitempty"` // Custom provider configuration
	OpenAIConfig             *schemas.OpenAIConfig            `json:"openai_config,omitempty"`          // OpenAI-specific configuration
	ProviderStatus           ProviderStatus                   `json:"provider_status"`                  // Health/initialization status of the provider
	Status                   string                           `json:"status,omitempty"`                 // Operational status (e.g., list_models_failed)
	Description              string                           `json:"description,omitempty"`            // Error/status description
	ConfigHash               string                           `json:"config_hash,omitempty"`            // Hash of config.json version, used for change detection
	// Read-only aggregation fields (populated on list/get)
	KeysCount        int     `json:"keys_count,omitempty"`         // Number of keys for this provider
	ModelsCount      int     `json:"models_count,omitempty"`       // Number of models for this provider
	KeysHealthStatus string  `json:"keys_health_status,omitempty"` // "healthy", "degraded", or "unknown"
	KeysEnabled      bool    `json:"keys_enabled"`                 // Whether any key is enabled for this provider
	HourlyRequests   int     `json:"hourly_requests,omitempty"`    // Backward compat: 1h request count
	HourlyErrors     int     `json:"hourly_errors,omitempty"`      // Backward compat: 1h error count
	LastUsedAt       string  `json:"last_used_at,omitempty"`       // Last successful request time (RFC3339)
	LastErrorAt      string  `json:"last_error_at,omitempty"`      // Last error time (RFC3339)
	Uptime           float64 `json:"uptime,omitempty"`             // 24h health ratio (0-1)
	AvgLatencyMs     int     `json:"avg_latency_ms,omitempty"`     // 24h average latency in ms
}

// ProviderStats holds the computed aggregation values for a single provider.
type ProviderStats struct {
	KeysCount        int
	ModelsCount      int
	KeysHealthStatus string
	KeysEnabled      bool
	HourlyRequests   int
	HourlyErrors     int
	LastUsedAt       string
	LastErrorAt      string
	Uptime           float64
	AvgLatencyMs     int
}

// ProviderLogStats provides per-provider request-log aggregation.
// The production implementation queries the logs store; tests provide a mock.
type ProviderLogStats interface {
	// AggregateProviderLogStats returns the log-derived aggregates for a single
	// provider (last-hour requests/errors, last success/error timestamps, 1h avg
	// latency). Used by the single-provider detail path.
	AggregateProviderLogStats(ctx context.Context, providerName schemas.ModelProvider) (hourlyRequests int, hourlyErrors int, lastUsedAt string, lastErrorAt string, avgLatencyMs int, err error)
}

// ListProvidersResponse represents the response for listing all providers
type ListProvidersResponse struct {
	Providers []ProviderResponse `json:"providers"`
	Total     int                `json:"total"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type providerCreatePayload struct {
	Provider                 schemas.ModelProvider             `json:"provider"`
	NetworkConfig            *schemas.NetworkConfig            `json:"network_config,omitempty"`
	ConcurrencyAndBufferSize *schemas.ConcurrencyAndBufferSize `json:"concurrency_and_buffer_size,omitempty"`
	ProxyConfig              *schemas.ProxyConfig              `json:"proxy_config,omitempty"`
	SendBackRawRequest       *bool                             `json:"send_back_raw_request,omitempty"`
	SendBackRawResponse      *bool                             `json:"send_back_raw_response,omitempty"`
	StoreRawRequestResponse  *bool                             `json:"store_raw_request_response,omitempty"`
	CustomProviderConfig     *schemas.CustomProviderConfig     `json:"custom_provider_config,omitempty"`
	OpenAIConfig             *schemas.OpenAIConfig             `json:"openai_config,omitempty"` // OpenAI-specific configuration
}

type providerUpdatePayload struct {
	NetworkConfig            schemas.NetworkConfig            `json:"network_config"`
	ConcurrencyAndBufferSize schemas.ConcurrencyAndBufferSize `json:"concurrency_and_buffer_size"`
	ProxyConfig              *schemas.ProxyConfig             `json:"proxy_config,omitempty"`
	SendBackRawRequest       *bool                            `json:"send_back_raw_request,omitempty"`
	SendBackRawResponse      *bool                            `json:"send_back_raw_response,omitempty"`
	StoreRawRequestResponse  *bool                            `json:"store_raw_request_response,omitempty"`
	CustomProviderConfig     *schemas.CustomProviderConfig    `json:"custom_provider_config,omitempty"`
	OpenAIConfig             *schemas.OpenAIConfig            `json:"openai_config,omitempty"` // OpenAI-specific configuration
}

// RegisterRoutes registers all provider management routes
func (h *ProviderHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.BifrostHTTPMiddleware) {
	// Provider CRUD operations
	r.GET("/api/providers", lib.ChainMiddlewares(h.listProviders, middlewares...))
	r.GET("/api/providers/{provider}", lib.ChainMiddlewares(h.getProvider, middlewares...))
	r.GET("/api/providers/{provider}/keys", lib.ChainMiddlewares(h.listProviderKeys, middlewares...))
	r.GET("/api/providers/{provider}/keys/{key_id}", lib.ChainMiddlewares(h.getProviderKey, middlewares...))
	r.POST("/api/providers", lib.ChainMiddlewares(h.addProvider, middlewares...))
	r.POST("/api/providers/{provider}/keys", lib.ChainMiddlewares(h.createProviderKey, middlewares...))
	r.POST("/api/providers/{provider}/keys/batch", lib.ChainMiddlewares(h.batchUpdateProviderKeys, middlewares...))
	r.PUT("/api/providers/{provider}", lib.ChainMiddlewares(h.updateProvider, middlewares...))
	r.PUT("/api/providers/{provider}/keys/{key_id}", lib.ChainMiddlewares(h.updateProviderKey, middlewares...))
	r.DELETE("/api/providers/{provider}", lib.ChainMiddlewares(h.deleteProvider, middlewares...))
	r.DELETE("/api/providers/{provider}/keys/{key_id}", lib.ChainMiddlewares(h.deleteProviderKey, middlewares...))
	// On-demand model discovery. The catalog otherwise refreshes on the
	// configured live_models_sync_interval, so these let an operator pick up a
	// newly served model (or re-check a failing key) without waiting.
	r.POST("/api/providers/{provider}/refresh-models", lib.ChainMiddlewares(h.refreshProviderModels, middlewares...))
	r.POST("/api/providers/{provider}/keys/{key_id}/refresh-models", lib.ChainMiddlewares(h.refreshProviderKeyModels, middlewares...))
	r.POST("/api/providers/{provider}/test-model", lib.ChainMiddlewares(h.testProviderModel, middlewares...))
	r.POST("/api/providers/{provider}/test-models", lib.ChainMiddlewares(h.testProviderModels, middlewares...))
	r.GET("/api/keys", lib.ChainMiddlewares(h.listKeys, middlewares...))
	r.GET("/api/models", lib.ChainMiddlewares(h.listModels, middlewares...))
	r.GET("/api/models/details", lib.ChainMiddlewares(h.listModelDetails, middlewares...))
	r.GET("/api/models/parameters", lib.ChainMiddlewares(h.getModelParameters, middlewares...))
	r.GET("/api/models/base", lib.ChainMiddlewares(h.listBaseModels, middlewares...))
	r.PUT("/api/models/catalog", lib.ChainMiddlewares(h.upsertModelCatalogEntries, middlewares...))
}

// listProviders handles GET /api/providers - List all providers
func (h *ProviderHandler) listProviders(ctx *fasthttp.RequestCtx) {
	// Fetching providers from database or in-memory store
	var providers map[schemas.ModelProvider]configstore.ProviderConfig
	if h.dbStore != nil {
		var err error
		providers, err = h.dbStore.GetProvidersConfig(ctx)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get providers: %v", err))
			return
		}
	} else {
		h.inMemoryStore.Mu.RLock()
		providers = h.inMemoryStore.Providers
		h.inMemoryStore.Mu.RUnlock()
	}

	providersInClient := []schemas.ModelProvider{}
	if h.client != nil {
		var err error
		providersInClient, err = h.client.GetConfiguredProviders()
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get providers from client: %v", err))
			return
		}
	}

	providerResponses := []ProviderResponse{}

	for providerName, provider := range providers {
		config := provider.Redacted()

		providerStatus := ProviderStatusError
		if slices.Contains(providersInClient, providerName) {
			providerStatus = ProviderStatusActive
		}
		response := h.getProviderResponseFromConfig(providerName, *config, providerStatus)
		stats := h.computeInMemoryProviderStats(providerName)
		h.applyProviderStats(&response, stats)
		providerResponses = append(providerResponses, response)
	}
	// Sort providers alphabetically
	sort.Slice(providerResponses, func(i, j int) bool {
		return providerResponses[i].Name < providerResponses[j].Name
	})
	response := ListProvidersResponse{
		Providers: providerResponses,
		Total:     len(providerResponses),
	}

	SendJSON(ctx, response)
}

// getProvider handles GET /api/providers/{provider} - Get specific provider
func (h *ProviderHandler) getProvider(ctx *fasthttp.RequestCtx) {
	provider, err := getProviderFromCtx(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid provider: %v", err))
		return
	}

	providersInClient := []schemas.ModelProvider{}
	if h.client != nil {
		providersInClient, err = h.client.GetConfiguredProviders()
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get providers from client: %v", err))
			return
		}
	}

	var config *configstore.ProviderConfig
	if h.dbStore != nil {
		config, err = h.dbStore.GetProviderConfig(ctx, provider)
		if err != nil {
			if errors.Is(err, configstore.ErrNotFound) {
				SendError(ctx, fasthttp.StatusNotFound, fmt.Sprintf("Provider not found: %v", err))
				return
			}
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get provider config: %v", err))
			return
		}
	} else {
		config, err = h.inMemoryStore.GetProviderConfigRaw(provider)
		if err != nil {
			if errors.Is(err, lib.ErrNotFound) {
				SendError(ctx, fasthttp.StatusNotFound, fmt.Sprintf("Provider not found: %v", err))
				return
			}
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get provider config: %v", err))
			return
		}
	}
	redactedConfig := config.Redacted()

	providerStatus := ProviderStatusError
	if slices.Contains(providersInClient, provider) {
		providerStatus = ProviderStatusActive
	}

	response := h.getProviderResponseFromConfig(provider, *redactedConfig, providerStatus)

	stats, statsErr := h.aggregateProviderStats(ctx, provider)
	if statsErr != nil {
		logger.Warn("Failed to aggregate stats for provider %s: %v", provider, statsErr)
	} else {
		h.applyProviderStats(&response, stats)
	}

	SendJSON(ctx, response)
}

// addProvider handles POST /api/providers - Add a new provider
// NOTE: This only gets called when a new custom provider is added
func (h *ProviderHandler) addProvider(ctx *fasthttp.RequestCtx) {
	var payload providerCreatePayload
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	// Validate provider
	if payload.Provider == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing provider")
		return
	}
	if payload.CustomProviderConfig != nil {
		// custom provider key should not be same as standard provider names
		if bifrost.IsStandardProvider(payload.Provider) {
			SendError(ctx, fasthttp.StatusBadRequest, "Custom provider cannot be same as a standard provider")
			return
		}
		if payload.CustomProviderConfig.BaseProviderType == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "BaseProviderType is required when CustomProviderConfig is provided")
			return
		}
		// check if base provider is a supported base provider
		if !bifrost.IsSupportedBaseProvider(payload.CustomProviderConfig.BaseProviderType) {
			SendError(ctx, fasthttp.StatusBadRequest, "BaseProviderType must be a standard provider")
			return
		}
	}
	if payload.ConcurrencyAndBufferSize != nil {
		if payload.ConcurrencyAndBufferSize.Concurrency == 0 {
			SendError(ctx, fasthttp.StatusBadRequest, "Concurrency must be greater than 0")
			return
		}
		if payload.ConcurrencyAndBufferSize.BufferSize == 0 {
			SendError(ctx, fasthttp.StatusBadRequest, "Buffer size must be greater than 0")
			return
		}
		if payload.ConcurrencyAndBufferSize.Concurrency > payload.ConcurrencyAndBufferSize.BufferSize {
			SendError(ctx, fasthttp.StatusBadRequest, "Concurrency must be less than or equal to buffer size")
			return
		}
	}
	// Validate retry backoff values if NetworkConfig is provided
	if payload.NetworkConfig != nil {
		if err := validateRetryBackoff(payload.NetworkConfig); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid retry backoff: %v", err))
			return
		}
		if payload.NetworkConfig.BaseURL != "" {
			if err := bifrost.ValidateExternalURL(payload.NetworkConfig.BaseURL, payload.NetworkConfig.AllowPrivateNetwork); err != nil {
				SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid base URL: %v", err))
				return
			}
		}
	}
	// Check if provider already exists
	if _, err := h.inMemoryStore.GetProviderConfigRedacted(payload.Provider); err != nil {
		if !errors.Is(err, lib.ErrNotFound) {
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to check provider config: %v", err))
			return
		}
	} else {
		SendError(ctx, fasthttp.StatusConflict, fmt.Sprintf("Provider %s already exists", payload.Provider))
		return
	}

	// Construct ProviderConfig from individual fields
	config := configstore.ProviderConfig{
		NetworkConfig:            payload.NetworkConfig,
		ProxyConfig:              payload.ProxyConfig,
		ConcurrencyAndBufferSize: payload.ConcurrencyAndBufferSize,
		SendBackRawRequest:       payload.SendBackRawRequest != nil && *payload.SendBackRawRequest,
		SendBackRawResponse:      payload.SendBackRawResponse != nil && *payload.SendBackRawResponse,
		StoreRawRequestResponse:  payload.StoreRawRequestResponse != nil && *payload.StoreRawRequestResponse,
		CustomProviderConfig:     payload.CustomProviderConfig,
		OpenAIConfig:             payload.OpenAIConfig,
	}
	// Validate custom provider configuration before persisting
	if err := lib.ValidateCustomProvider(config, payload.Provider); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid custom provider config: %v", err))
		return
	}
	// Add provider to store (env vars will be processed by store)
	if err := h.inMemoryStore.AddProvider(ctx, payload.Provider, config); err != nil {
		logger.Warn("Failed to add provider %s: %v", payload.Provider, err)
		if errors.Is(err, lib.ErrAlreadyExists) {
			SendError(ctx, fasthttp.StatusConflict, err.Error())
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to add provider: %v", err))
		return
	}
	logger.Info("Provider %s added successfully", payload.Provider)

	if err := h.reloadProviderAfterCreate(ctx, payload.Provider); err != nil {
		logger.Warn("Failed to reload provider %s after add: %v", payload.Provider, err)
		if rollbackErr := h.inMemoryStore.RemoveProvider(context.Background(), payload.Provider); rollbackErr != nil {
			logger.Error("Failed to rollback provider %s after reload failure: %v", payload.Provider, rollbackErr)
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to initialize provider after add: %v (rollback failed: %v)", err, rollbackErr))
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to initialize provider after add: %v", err))
		return
	}

	// Get redacted config for response (in-memory store is now updated by updateKeyStatus)
	redactedConfig, err := h.inMemoryStore.GetProviderConfigRedacted(payload.Provider)
	if err != nil {
		logger.Warn("Failed to get redacted config for provider %s: %v", payload.Provider, err)
		// Fall back to the raw config (no keys)
		response := h.getProviderResponseFromConfig(payload.Provider, configstore.ProviderConfig{
			NetworkConfig:            config.NetworkConfig,
			ConcurrencyAndBufferSize: config.ConcurrencyAndBufferSize,
			ProxyConfig:              config.ProxyConfig,
			SendBackRawRequest:       config.SendBackRawRequest,
			SendBackRawResponse:      config.SendBackRawResponse,
			StoreRawRequestResponse:  config.StoreRawRequestResponse,
			CustomProviderConfig:     config.CustomProviderConfig,
			Status:                   config.Status,
			Description:              config.Description,
		}, ProviderStatusActive)
		SendJSON(ctx, response)
		return
	}

	response := h.getProviderResponseFromConfig(payload.Provider, *redactedConfig, ProviderStatusActive)

	SendJSON(ctx, response)
}

// updateProvider handles PUT /api/providers/{provider} - Update provider config
// NOTE: This endpoint expects ALL fields to be provided in the request body,
// including both edited and non-edited fields. Partial updates are not supported.
// The frontend should send the complete provider configuration.
// This flow upserts the config
func (h *ProviderHandler) updateProvider(ctx *fasthttp.RequestCtx) {
	provider, err := getProviderFromCtx(ctx)
	if err != nil {
		// If not found, then first we create and then update
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid provider: %v", err))
		return
	}

	var payload = struct {
		Keys []schemas.Key `json:"keys"` // API keys for the provider
		providerUpdatePayload
	}{}

	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}

	// Reject `keys` in the request body. This endpoint manages provider-level
	// configuration only; keys are managed via the dedicated /keys endpoints
	// (POST/PUT/DELETE /api/providers/{provider}/keys[/{key_id}]). Accepting
	// the field silently would discard the caller's intent — the construction
	// below ignores `payload.Keys` and reuses `oldConfigRaw.Keys`. Failing
	// fast keeps the API contract honest.
	if len(payload.Keys) > 0 {
		SendError(
			ctx,
			fasthttp.StatusBadRequest,
			"keys are not accepted on this endpoint; use POST/PUT /api/providers/{provider}/keys[/{key_id}] to manage keys",
		)
		return
	}

	// Get the raw config to access actual values for merging with redacted request values
	oldConfigRaw, err := h.inMemoryStore.GetProviderConfigRaw(provider)
	if err != nil {
		if !errors.Is(err, lib.ErrNotFound) {
			logger.Warn("Failed to get old config for provider %s: %v", provider, err)
			SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
	}

	if oldConfigRaw == nil {
		oldConfigRaw = &configstore.ProviderConfig{}
	}

	oldRedactedConfig, err := h.inMemoryStore.GetProviderConfigRedacted(provider)
	if err != nil {
		if !errors.Is(err, lib.ErrNotFound) {
			logger.Warn("Failed to get old redacted config for provider %s: %v", provider, err)
			SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
	}

	if oldRedactedConfig == nil {
		oldRedactedConfig = &configstore.ProviderConfig{}
	}

	// Construct ProviderConfig from individual fields (keys are managed separately via /keys endpoints)
	config := configstore.ProviderConfig{
		Keys:                     oldConfigRaw.Keys,
		NetworkConfig:            oldConfigRaw.NetworkConfig,
		ConcurrencyAndBufferSize: oldConfigRaw.ConcurrencyAndBufferSize,
		ProxyConfig:              oldConfigRaw.ProxyConfig,
		CustomProviderConfig:     oldConfigRaw.CustomProviderConfig,
		OpenAIConfig:             oldConfigRaw.OpenAIConfig,
		StoreRawRequestResponse:  oldConfigRaw.StoreRawRequestResponse,
		Status:                   oldConfigRaw.Status,
		Description:              oldConfigRaw.Description,
	}

	if payload.ConcurrencyAndBufferSize.Concurrency == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "Concurrency must be greater than 0")
		return
	}
	if payload.ConcurrencyAndBufferSize.BufferSize == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "Buffer size must be greater than 0")
		return
	}

	if payload.ConcurrencyAndBufferSize.Concurrency > payload.ConcurrencyAndBufferSize.BufferSize {
		SendError(ctx, fasthttp.StatusBadRequest, "Concurrency must be less than or equal to buffer size")
		return
	}

	// Build a prospective config with the requested CustomProviderConfig (including nil)
	prospective := config
	prospective.CustomProviderConfig = payload.CustomProviderConfig
	if err := lib.ValidateCustomProviderUpdate(prospective, *oldConfigRaw, provider); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid custom provider config: %v", err))
		return
	}

	nc := payload.NetworkConfig

	// Validate retry backoff values
	if err := validateRetryBackoff(&nc); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid retry backoff: %v", err))
		return
	}
	if nc.BaseURL != "" {
		if err := bifrost.ValidateExternalURL(nc.BaseURL, nc.AllowPrivateNetwork); err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid base URL: %v", err))
			return
		}
	}

	config.ConcurrencyAndBufferSize = &payload.ConcurrencyAndBufferSize
	// Merge network config - restore ca_cert_pem if the redacted placeholder was sent back
	if oldConfigRaw.NetworkConfig != nil && oldRedactedConfig.NetworkConfig != nil && nc.CACertPEM != nil {
		if nc.CACertPEM.IsRedacted() && nc.CACertPEM.Equals(oldRedactedConfig.NetworkConfig.CACertPEM) {
			nc.CACertPEM = oldConfigRaw.NetworkConfig.CACertPEM
		}
	}
	config.NetworkConfig = &nc
	// Merge proxy config - preserve secrets if redacted values were sent back
	if payload.ProxyConfig != nil && oldConfigRaw.ProxyConfig != nil && oldRedactedConfig.ProxyConfig != nil {
		if payload.ProxyConfig.URL != nil && payload.ProxyConfig.URL.IsRedacted() && payload.ProxyConfig.URL.Equals(oldRedactedConfig.ProxyConfig.URL) {
			payload.ProxyConfig.URL = oldConfigRaw.ProxyConfig.URL
		}
		if payload.ProxyConfig.Username != nil && payload.ProxyConfig.Username.IsRedacted() && payload.ProxyConfig.Username.Equals(oldRedactedConfig.ProxyConfig.Username) {
			payload.ProxyConfig.Username = oldConfigRaw.ProxyConfig.Username
		}
		if payload.ProxyConfig.Password != nil && payload.ProxyConfig.Password.IsRedacted() && payload.ProxyConfig.Password.Equals(oldRedactedConfig.ProxyConfig.Password) {
			payload.ProxyConfig.Password = oldConfigRaw.ProxyConfig.Password
		}
		if payload.ProxyConfig.CACertPEM != nil && payload.ProxyConfig.CACertPEM.IsRedacted() && payload.ProxyConfig.CACertPEM.Equals(oldRedactedConfig.ProxyConfig.CACertPEM) {
			payload.ProxyConfig.CACertPEM = oldConfigRaw.ProxyConfig.CACertPEM
		}
	}

	config.ProxyConfig = payload.ProxyConfig
	config.CustomProviderConfig = payload.CustomProviderConfig
	config.OpenAIConfig = payload.OpenAIConfig
	if payload.SendBackRawRequest != nil {
		config.SendBackRawRequest = *payload.SendBackRawRequest
	}
	if payload.SendBackRawResponse != nil {
		config.SendBackRawResponse = *payload.SendBackRawResponse
	}
	if payload.StoreRawRequestResponse != nil {
		config.StoreRawRequestResponse = *payload.StoreRawRequestResponse
	}

	// Add provider to store if it doesn't exist (upsert behavior)
	if _, err := h.inMemoryStore.GetProviderConfigRaw(provider); err != nil {
		if !errors.Is(err, lib.ErrNotFound) {
			logger.Warn("Failed to get provider %s: %v", provider, err)
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get provider: %v", err))
			return
		}
		// Adding the provider to store
		if err := h.inMemoryStore.AddProvider(ctx, provider, config); err != nil {
			// In an upsert flow, "already exists" is not fatal — the provider may have been
			// added concurrently or exist in the DB from a previous failed attempt.
			if !errors.Is(err, lib.ErrAlreadyExists) {
				logger.Warn("Failed to add provider %s: %v", provider, err)
				SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to add provider: %v", err))
				return
			}
			logger.Info("Provider %s already exists during upsert, proceeding with update", provider)
		}
	}

	// Update provider config in store (env vars will be processed by store)
	if err := h.inMemoryStore.UpdateProviderConfig(ctx, provider, config); err != nil {
		logger.Warn("Failed to update provider %s: %v", provider, err)
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to update provider: %v", err))
		return
	}
	// Attempt model discovery (also triggers cluster broadcast via ReloadProvider).
	// For keyless providers, model discovery is skipped but we still need to
	// call ReloadProvider directly so the config change is broadcast to cluster peers.
	if payload.CustomProviderConfig != nil && payload.CustomProviderConfig.IsKeyLess {
		ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, reloadErr := h.modelsManager.ReloadProvider(ctxWithTimeout, provider); reloadErr != nil {
			logger.Warn("ReloadProvider failed for keyless provider %s: %v", provider, reloadErr)
		}
	} else {
		err = h.attemptModelDiscovery(ctx, provider, payload.CustomProviderConfig)
		if err != nil {
			logger.Warn("Model discovery failed for provider %s: %v", provider, err)
		}
	}

	// Get redacted config for response (in-memory store is now updated by updateKeyStatus)
	redactedConfig, err := h.inMemoryStore.GetProviderConfigRedacted(provider)
	if err != nil {
		logger.Warn("Failed to get redacted config for provider %s: %v", provider, err)
		// Fall back to sanitized config (no keys)
		response := h.getProviderResponseFromConfig(provider, configstore.ProviderConfig{
			NetworkConfig:            config.NetworkConfig,
			ConcurrencyAndBufferSize: config.ConcurrencyAndBufferSize,
			ProxyConfig:              config.ProxyConfig,
			SendBackRawRequest:       config.SendBackRawRequest,
			SendBackRawResponse:      config.SendBackRawResponse,
			StoreRawRequestResponse:  config.StoreRawRequestResponse,
			CustomProviderConfig:     config.CustomProviderConfig,
			Status:                   config.Status,
			Description:              config.Description,
		}, ProviderStatusActive)
		SendJSON(ctx, response)
		return
	}

	response := h.getProviderResponseFromConfig(provider, *redactedConfig, ProviderStatusActive)

	SendJSON(ctx, response)
}

// deleteProvider handles DELETE /api/providers/{provider} - Remove provider
func (h *ProviderHandler) deleteProvider(ctx *fasthttp.RequestCtx) {
	provider, err := getProviderFromCtx(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid provider: %v", err))
		return
	}

	// Check if provider exists
	if _, err := h.inMemoryStore.GetProviderConfigRedacted(provider); err != nil && !errors.Is(err, lib.ErrNotFound) {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Failed to get provider: %v", err))
		return
	}

	if err := h.modelsManager.RemoveProvider(ctx, provider); err != nil {
		logger.Warn("Failed to delete models for provider %s: %v", provider, err)
	}

	response := ProviderResponse{
		Name: provider,
	}

	SendJSON(ctx, response)
}

// listKeys handles GET /api/keys - List all keys
func (h *ProviderHandler) listKeys(ctx *fasthttp.RequestCtx) {
	keys, err := h.inMemoryStore.GetAllKeys()
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get keys: %v", err))
		return
	}

	SendJSON(ctx, keys)
}

// ModelResponse represents a single model in the response
type ModelResponse struct {
	Name             string   `json:"name"`
	Provider         string   `json:"provider"`
	IsDeprecated     bool     `json:"is_deprecated,omitempty"`
	AccessibleByKeys []string `json:"accessible_by_keys,omitempty"`
}

// ListModelsResponse represents the response for listing models
type ListModelsResponse struct {
	Models []ModelResponse `json:"models"`
	Total  int             `json:"total"`
}

// ModelDetailsResponse represents a model with capability metadata.
type ModelDetailsResponse struct {
	Name                 string                `json:"name"`
	Provider             string                `json:"provider"`
	ContextLength        *int                  `json:"context_length,omitempty"`
	MaxInputTokens       *int                  `json:"max_input_tokens,omitempty"`
	MaxOutputTokens      *int                  `json:"max_output_tokens,omitempty"`
	InputCostPerToken    *float64              `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken   *float64              `json:"output_cost_per_token,omitempty"`
	CacheWriteCost       *float64              `json:"cache_creation_input_token_cost,omitempty"`
	CacheReadCost        *float64              `json:"cache_read_input_token_cost,omitempty"`
	Architecture         *schemas.Architecture `json:"architecture,omitempty"`
	IsDeprecated         bool                  `json:"is_deprecated,omitempty"`
	AdditionalAttributes map[string]string     `json:"additional_attributes,omitempty"`
	AccessibleByKeys     []string              `json:"accessible_by_keys,omitempty"`
}

// ListModelDetailsResponse represents the response for listing detailed models.
type ListModelDetailsResponse struct {
	Models []ModelDetailsResponse `json:"models"`
	Total  int                    `json:"total"`
}

type modelListQuery struct {
	Provider   schemas.ModelProvider
	Query      string
	KeyIDs     []string
	Limit      int
	Offset     int
	Unfiltered bool
	// VK-based filtering: populated when a virtual key is found in request headers.
	// HasVKFilter=true restricts providers/models to those allowed by the VK.
	HasVKFilter       bool
	VKProviderConfigs []tables.TableVirtualKeyProviderConfig
}

type listedModel struct {
	Name             string
	Provider         schemas.ModelProvider
	AccessibleByKeys []string
}

// listModels handles GET /api/models - List models with filtering
// Query parameters:
//   - query: Filter models by name (case-insensitive partial match)
//   - provider: Filter by specific provider name
//   - keys: Comma-separated list of provider key UUIDs to filter models accessible by those keys
//   - limit: Maximum number of results to return (default: 5)
//
// Request headers:
//   - x-bf-vk / Authorization: Bearer / x-api-key / x-goog-api-key: Virtual key (sk-bf-…) to scope
//     results to providers and models allowed by that virtual key.
func (h *ProviderHandler) listModels(ctx *fasthttp.RequestCtx) {
	query, ok := h.parseModelListQuery(ctx, 5)
	if !ok {
		return
	}
	allModels, total, err := h.listManagementModels(query)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get providers: %v", err))
		return
	}

	responseModels := make([]ModelResponse, 0, len(allModels))
	for _, model := range allModels {
		entry := ModelResponse{
			Name:         model.Name,
			Provider:     string(model.Provider),
			IsDeprecated: h.isModelDeprecated(model.Name, model.Provider),
		}
		if len(model.AccessibleByKeys) > 0 {
			entry.AccessibleByKeys = model.AccessibleByKeys
		}
		responseModels = append(responseModels, entry)
	}

	response := ListModelsResponse{
		Models: responseModels,
		Total:  total,
	}

	SendJSON(ctx, response)
}

// listModelDetails handles GET /api/models/details - List models with capability metadata.
// Query parameters:
//   - query: Filter models by name (case-insensitive partial match)
//   - provider: Filter by specific provider name
//   - keys: Comma-separated list of key IDs to filter models accessible by those keys
//   - unfiltered: If true, bypass provider-level model pool restrictions only
//   - limit: Maximum number of results to return (default: 20)
//   - offset: Number of results to skip (for pagination)
//
// Request headers:
//   - x-bf-vk / Authorization: Bearer / x-api-key / x-goog-api-key: Virtual key (sk-bf-…) to scope
//     results to providers and models allowed by that virtual key.
func (h *ProviderHandler) listModelDetails(ctx *fasthttp.RequestCtx) {
	query, ok := h.parseModelListQuery(ctx, 20)
	if !ok {
		return
	}

	modelCatalog := h.inMemoryStore.ModelCatalog
	if modelCatalog == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "model catalog not available")
		return
	}

	allModels, total, err := h.listManagementModels(query)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get providers: %v", err))
		return
	}

	responseModels := make([]ModelDetailsResponse, 0, len(allModels))
	for _, model := range allModels {
		details := ModelDetailsResponse{
			Name:     model.Name,
			Provider: string(model.Provider),
		}
		if len(model.AccessibleByKeys) > 0 {
			details.AccessibleByKeys = model.AccessibleByKeys
		}
		if capabilities := modelCatalog.GetModelCapabilityEntryForModel(model.Name, model.Provider); capabilities != nil {
			details.ContextLength = capabilities.ContextLength
			details.MaxInputTokens = capabilities.MaxInputTokens
			details.MaxOutputTokens = capabilities.MaxOutputTokens
			details.InputCostPerToken = capabilities.InputCostPerToken
			details.OutputCostPerToken = capabilities.OutputCostPerToken
			details.CacheWriteCost = capabilities.CacheCreationInputTokenCost
			details.CacheReadCost = capabilities.CacheReadInputTokenCost
			details.Architecture = capabilities.Architecture
			details.IsDeprecated = capabilities.IsDeprecated
			details.AdditionalAttributes = capabilities.AdditionalAttributes
		}
		responseModels = append(responseModels, details)
	}

	SendJSON(ctx, ListModelDetailsResponse{
		Models: responseModels,
		Total:  total,
	})
}

func (h *ProviderHandler) isModelDeprecated(model string, provider schemas.ModelProvider) bool {
	modelCatalog := h.inMemoryStore.ModelCatalog
	if modelCatalog == nil {
		return false
	}
	capabilities := modelCatalog.GetModelCapabilityEntryForModel(model, provider)
	return capabilities != nil && capabilities.IsDeprecated
}

// parseModelListQuery normalizes the management model-list query string and resolves
// any virtual key present in the request headers to populate provider/model filters.
func (h *ProviderHandler) parseModelListQuery(ctx *fasthttp.RequestCtx, defaultLimit int) (modelListQuery, bool) {
	queryArgs := ctx.QueryArgs()
	query := modelListQuery{
		Provider:   schemas.ModelProvider(string(queryArgs.Peek("provider"))),
		Query:      string(queryArgs.Peek("query")),
		Limit:      defaultLimit,
		Unfiltered: string(queryArgs.Peek("unfiltered")) == "true",
	}

	if keysRaw := queryArgs.Peek("keys"); len(keysRaw) > 0 {
		keyIDs := strings.Split(string(keysRaw), ",")
		query.KeyIDs = make([]string, 0, len(keyIDs))
		for _, keyID := range keyIDs {
			trimmedKeyID := strings.TrimSpace(keyID)
			if trimmedKeyID == "" {
				continue
			}
			query.KeyIDs = append(query.KeyIDs, trimmedKeyID)
		}
	}

	if len(queryArgs.Peek("limit")) > 0 {
		if limit, err := queryArgs.GetUint("limit"); err == nil {
			query.Limit = limit
		}
	}
	if len(queryArgs.Peek("offset")) > 0 {
		if offset, err := queryArgs.GetUint("offset"); err == nil {
			query.Offset = offset
		}
	}

	// Resolve virtual key from request headers and populate provider/model filters.
	if vkValue := governanceplugin.ParseVirtualKeyFromFastHTTPRequest(ctx); vkValue != nil {
		trimmedVKValue := strings.TrimSpace(*vkValue)

		if h.dbStore == nil {
			SendError(ctx, fasthttp.StatusServiceUnavailable, "database store unavailable")
			return query, false
		}

		vk, err := h.dbStore.GetVirtualKeyByValue(ctx, trimmedVKValue)
		if err != nil {
			if !errors.Is(err, configstore.ErrNotFound) {
				SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to resolve virtual key: %v", err))
				return query, false
			}
		}

		if vk != nil {
			query.HasVKFilter = true
			query.VKProviderConfigs = vk.ProviderConfigs
		}
	}

	return query, true
}

// listManagementModels lists models across one or all providers and applies the top-level limit.
func (h *ProviderHandler) listManagementModels(query modelListQuery) ([]listedModel, int, error) {
	providers := []schemas.ModelProvider{}
	if query.Provider != "" {
		providers = append(providers, query.Provider)
	} else {
		var err error
		providers, err = h.inMemoryStore.GetAllProviders()
		if err != nil {
			return nil, 0, err
		}
	}

	// When a virtual key is present, restrict the provider list to those explicitly
	// permitted by the VK. An empty ProviderConfigs means no providers are allowed
	// (deny-by-default), so we return nothing even if providers are configured.
	if query.HasVKFilter {
		providers = slices.DeleteFunc(providers, func(p schemas.ModelProvider) bool {
			return !slices.ContainsFunc(query.VKProviderConfigs, func(pc tables.TableVirtualKeyProviderConfig) bool {
				return strings.EqualFold(pc.Provider, string(p))
			})
		})
	}

	slices.Sort(providers)

	models := make([]listedModel, 0)
	for _, provider := range providers {
		models = append(models, h.listManagementModelsForProvider(provider, query)...)
	}

	total := len(models)
	if query.Offset > 0 {
		if query.Offset >= len(models) {
			models = models[:0]
		} else {
			models = models[query.Offset:]
		}
	}
	if query.Limit > 0 && query.Limit < len(models) {
		models = models[:query.Limit]
	}

	return models, total, nil
}

// listManagementModelsForProvider applies provider-level model selection and key filtering.
func (h *ProviderHandler) listManagementModelsForProvider(
	provider schemas.ModelProvider,
	query modelListQuery,
) []listedModel {
	models := h.modelsManager.GetModelsForProvider(provider)
	if query.Unfiltered {
		models = h.modelsManager.GetUnfilteredModelsForProvider(provider)
	}

	// Apply VK-level model whitelist filtering.
	// AllowedModels=["*"] passes all; empty AllowedModels denies all (deny-by-default).
	if query.HasVKFilter {
		if idx := slices.IndexFunc(query.VKProviderConfigs, func(pc tables.TableVirtualKeyProviderConfig) bool {
			return strings.EqualFold(pc.Provider, string(provider))
		}); idx >= 0 {
			allowedModels := query.VKProviderConfigs[idx].AllowedModels
			models = slices.DeleteFunc(models, func(m string) bool { return !allowedModels.IsAllowed(m) })
		}
	}

	if len(query.KeyIDs) == 0 || query.Unfiltered {
		return buildListedModels(provider, models, nil, query.Query)
	}

	config, err := h.inMemoryStore.GetProviderConfigRaw(provider)
	if err != nil {
		logger.Warn("Failed to get config for provider %s: %v", provider, err)
		return buildListedModels(provider, models, nil, query.Query)
	}
	if config == nil {
		logger.Warn("Failed to get config for provider %s: nil provider config", provider)
		return buildListedModels(provider, models, nil, query.Query)
	}

	validKeyIDs := getValidKeyIDsForProvider(config, query.KeyIDs)
	if len(validKeyIDs) == 0 {
		return buildListedModels(provider, models, nil, query.Query)
	}

	filteredModels, accessByModel := filterModelsByKeysWithAccessMap(
		config,
		provider,
		h.inMemoryStore.ModelCatalog,
		models,
		validKeyIDs,
	)

	return buildListedModels(provider, filteredModels, accessByModel, query.Query)
}

// buildListedModels filters model names by query and projects them into internal rows.
func buildListedModels(
	provider schemas.ModelProvider,
	models []string,
	accessByModel map[string][]string,
	query string,
) []listedModel {
	listedModels := make([]listedModel, 0, len(models))
	for _, model := range models {
		if !matchesModelQuery(model, query) {
			continue
		}

		entry := listedModel{
			Name:     model,
			Provider: provider,
		}
		if len(accessByModel[model]) > 0 {
			entry.AccessibleByKeys = accessByModel[model]
		}
		listedModels = append(listedModels, entry)
	}
	return listedModels
}

// getModelParameters handles GET /api/models/parameters - Get model parameters for a specific model
// Query parameters:
//   - model: The model name to get parameters for (required)
func (h *ProviderHandler) getModelParameters(ctx *fasthttp.RequestCtx) {
	modelParam := string(ctx.QueryArgs().Peek("model"))
	if modelParam == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "model query parameter is required")
		return
	}

	if h.dbStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "database store not available")
		return
	}

	// Prefer catalog-aware resolution so provider-qualified IDs from
	// /v1/models ("openai/gpt-5.5", "openrouter/openai/gpt-5.5") and bare
	// aliases resolve to the datasheet's stored key instead of 404ing on an
	// exact-match miss.
	var params *tables.TableModelParameters
	var err error
	if h.inMemoryStore != nil && h.inMemoryStore.ModelCatalog != nil {
		params, err = h.inMemoryStore.ModelCatalog.ResolveModelParameters(ctx, modelParam)
	} else {
		params, err = h.dbStore.GetModelParametersByModel(ctx, modelParam)
	}
	if err == nil && params == nil {
		err = configstore.ErrNotFound
	}
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, fmt.Sprintf("no parameters found for model %s", modelParam))
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to get model parameters: %v", err))
		return
	}

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString(params.Data)
}

// keyAllowsModelForList reports whether a provider key permits model for catalog listing.
// When a non-nil catalog is provided, it also checks whether any allowlisted
// model resolves to the same base model name as the queried model (alias matching).
func keyAllowsModelForList(key schemas.Key, model string, catalog *modelcatalog.ModelCatalog) bool {
	if key.BlacklistedModels.IsBlocked(model) {
		return false
	}
	if len(key.Models) > 0 {
		if key.Models.IsAllowed(model) {
			return true
		}
		// Catalog-aware alias matching: a key allowlisting "gpt-4o-2024-08-06"
		// should also grant access to its base model "gpt-4o" in listings.
		if catalog != nil {
			for _, allowed := range key.Models {
				if strings.EqualFold(
					catalog.GetBaseModelName(allowed),
					catalog.GetBaseModelName(model),
				) {
					return true
				}
			}
		}
		return false
	}
	return true
}

// matchesModelQuery applies the shared query match used by /api/models,
// /api/models/details, and /api/models/base.
func matchesModelQuery(model, query string) bool {
	if query == "" {
		return true
	}

	queryLower := strings.ToLower(query)
	queryNormalized := strings.ReplaceAll(strings.ReplaceAll(queryLower, "-", ""), "_", "")
	modelLower := strings.ToLower(model)
	modelNormalized := strings.ReplaceAll(strings.ReplaceAll(modelLower, "-", ""), "_", "")

	return strings.Contains(modelLower, queryLower) ||
		strings.Contains(modelNormalized, queryNormalized) ||
		fuzzyMatch(modelLower, queryLower)
}

// getValidKeyIDsForProvider keeps only enabled, known, deduplicated key IDs.
func getValidKeyIDsForProvider(config *configstore.ProviderConfig, keyIDs []string) []string {
	if config == nil || len(keyIDs) == 0 {
		return nil
	}

	existing := make(map[string]bool, len(config.Keys))
	for _, key := range config.Keys {
		if key.Enabled != nil && !*key.Enabled {
			continue
		}
		existing[key.ID] = true
	}

	valid := make([]string, 0, len(keyIDs))
	seen := make(map[string]bool, len(keyIDs))
	for _, keyID := range keyIDs {
		if keyID == "" || seen[keyID] {
			continue
		}
		seen[keyID] = true
		if existing[keyID] {
			valid = append(valid, keyID)
		}
	}
	return valid
}

// filterModelsByKeysWithAccessMap filters models based on key-level model restrictions
// and returns the exact key IDs that grant access to each returned model.
func filterModelsByKeysWithAccessMap(config *configstore.ProviderConfig, provider schemas.ModelProvider, modelCatalog *modelcatalog.ModelCatalog, models []string, keyIDs []string) ([]string, map[string][]string) {
	if config == nil {
		return []string{}, map[string][]string{}
	}

	keysByID := make(map[string]schemas.Key, len(config.Keys))
	for _, key := range config.Keys {
		if key.Enabled != nil && !*key.Enabled {
			continue
		}
		keysByID[key.ID] = key
	}

	type matchedKey struct {
		id  string
		key schemas.Key
	}

	matchedKeys := make([]matchedKey, 0, len(keyIDs))
	for _, keyID := range keyIDs {
		key, ok := keysByID[keyID]
		if !ok {
			continue
		}
		matchedKeys = append(matchedKeys, matchedKey{id: keyID, key: key})
	}
	if len(matchedKeys) == 0 {
		return []string{}, map[string][]string{}
	}

	filtered := make([]string, 0, len(models))
	accessByModel := make(map[string][]string, len(models))
	for _, model := range models {
		grantedBy := make([]string, 0, len(matchedKeys))
		for _, matched := range matchedKeys {
			if keyAllowsModelForList(matched.key, model, modelCatalog) {
				grantedBy = append(grantedBy, matched.id)
			}
		}
		if len(grantedBy) == 0 {
			continue
		}
		filtered = append(filtered, model)
		accessByModel[model] = grantedBy
	}
	return filtered, accessByModel
}

// ListBaseModelsResponse represents the response for listing base models
type ListBaseModelsResponse struct {
	Models []string `json:"models"`
	Total  int      `json:"total"`
}

// listBaseModels handles GET /api/models/base - List distinct base model names from the catalog
// Query parameters:
//   - query: Filter base models by name (case-insensitive partial match)
//   - limit: Maximum number of results to return (default: 20)
func (h *ProviderHandler) listBaseModels(ctx *fasthttp.RequestCtx) {
	queryParam := string(ctx.QueryArgs().Peek("query"))
	limitParam := string(ctx.QueryArgs().Peek("limit"))

	limit := 20
	if limitParam != "" {
		if n, err := ctx.QueryArgs().GetUint("limit"); err == nil {
			limit = n
		}
	}

	modelCatalog := h.inMemoryStore.ModelCatalog
	if modelCatalog == nil {
		SendJSON(ctx, ListBaseModelsResponse{Models: []string{}, Total: 0})
		return
	}

	baseModels := modelCatalog.GetDistinctBaseModelNames()
	sort.Strings(baseModels)

	// Apply query filter if provided
	if queryParam != "" {
		filtered := []string{}
		for _, model := range baseModels {
			if matchesModelQuery(model, queryParam) {
				filtered = append(filtered, model)
			}
		}
		baseModels = filtered
	}

	total := len(baseModels)
	if limit > 0 && limit < len(baseModels) {
		baseModels = baseModels[:limit]
	}

	SendJSON(ctx, ListBaseModelsResponse{Models: baseModels, Total: total})
}

// reloadProviderAfterCreate performs a single bounded runtime reload after provider creation.
// ReloadProvider also refreshes model discovery, so create should not invoke a second discovery pass.
func (h *ProviderHandler) reloadProviderAfterCreate(ctx *fasthttp.RequestCtx, provider schemas.ModelProvider) error {
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := h.modelsManager.ReloadProvider(ctxWithTimeout, provider)
	return err
}

// attemptModelDiscovery performs model discovery with timeout
func (h *ProviderHandler) attemptModelDiscovery(ctx *fasthttp.RequestCtx, provider schemas.ModelProvider, customProviderConfig *schemas.CustomProviderConfig) error {
	// Determine if we should attempt model discovery
	shouldDiscoverModels := customProviderConfig == nil ||
		!customProviderConfig.IsKeyLess

	if !shouldDiscoverModels {
		return nil
	}

	// Attempt model discovery with reasonable timeout
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := h.modelsManager.ReloadProvider(ctxWithTimeout, provider)
	if err != nil {
		return err
	}

	return nil
}

func (h *ProviderHandler) getProviderResponseFromConfig(provider schemas.ModelProvider, config configstore.ProviderConfig, status ProviderStatus) ProviderResponse {
	if config.NetworkConfig == nil {
		config.NetworkConfig = &schemas.DefaultNetworkConfig
	}
	if config.ConcurrencyAndBufferSize == nil {
		config.ConcurrencyAndBufferSize = &schemas.DefaultConcurrencyAndBufferSize
	}

	return ProviderResponse{
		Name:                     provider,
		NetworkConfig:            *config.NetworkConfig,
		ConcurrencyAndBufferSize: *config.ConcurrencyAndBufferSize,
		ProxyConfig:              config.ProxyConfig,
		SendBackRawRequest:       config.SendBackRawRequest,
		SendBackRawResponse:      config.SendBackRawResponse,
		StoreRawRequestResponse:  config.StoreRawRequestResponse,
		CustomProviderConfig:     config.CustomProviderConfig,
		OpenAIConfig:             config.OpenAIConfig,
		ProviderStatus:           status,
		Status:                   config.Status,
		Description:              config.Description,
		ConfigHash:               config.ConfigHash,
	}
}

// applyProviderStats copies an aggregation result onto a ProviderResponse.
func (h *ProviderHandler) applyProviderStats(response *ProviderResponse, stats ProviderStats) {
	if response == nil {
		return
	}
	response.KeysCount = stats.KeysCount
	response.ModelsCount = stats.ModelsCount
	response.KeysHealthStatus = stats.KeysHealthStatus
	response.KeysEnabled = stats.KeysEnabled
	response.HourlyRequests = stats.HourlyRequests
	response.HourlyErrors = stats.HourlyErrors
	response.LastUsedAt = stats.LastUsedAt
	response.LastErrorAt = stats.LastErrorAt
	response.Uptime = stats.Uptime
	response.AvgLatencyMs = stats.AvgLatencyMs
}

// computeInMemoryProviderStats computes the in-memory-only aggregation fields
// (keys count, models count, health, enabled) for a single provider. No log
// store queries are made — this is used by the fast listProviders path.
func (h *ProviderHandler) computeInMemoryProviderStats(providerName schemas.ModelProvider) ProviderStats {
	var stats ProviderStats
	var providerConfig *configstore.ProviderConfig
	if h.inMemoryStore != nil {
		cfg, err := h.inMemoryStore.GetProviderConfigRaw(providerName)
		if err == nil && cfg != nil {
			providerConfig = cfg
		}
	}
	if providerConfig != nil {
		stats.KeysCount = len(providerConfig.Keys)
		stats.KeysHealthStatus = computeKeysHealthStatus(providerConfig.Keys)
		stats.KeysEnabled = computeKeysEnabled(providerConfig.Keys)
	}
	if h.modelsManager != nil {
		stats.ModelsCount = len(h.modelsManager.GetModelsForProvider(providerName))
	}
	return stats
}

// aggregateProviderStats computes the full aggregation fields for a single
// provider (detail path). Keys/models/health come from in-memory; log-derived
// fields (hourly requests/errors, last timestamps, avg latency, uptime) come
// from the optional ProviderLogStats source (1-hour rolling window).
func (h *ProviderHandler) aggregateProviderStats(ctx context.Context, providerName schemas.ModelProvider) (ProviderStats, error) {
	stats, err := h.computeProviderStats(ctx, providerName)
	if err != nil {
		return ProviderStats{}, fmt.Errorf("aggregate stats for provider %s: %w", providerName, err)
	}
	return stats, nil
}

// computeProviderStats derives the aggregation fields for one provider:
//   - keys count + health from the in-memory provider config
//   - models count from the models manager
//   - 1-hour rolling-window request/error/latency aggregates from the optional logs source
func (h *ProviderHandler) computeProviderStats(ctx context.Context, providerName schemas.ModelProvider) (ProviderStats, error) {
	var stats ProviderStats
	stats = h.computeInMemoryProviderStats(providerName)

	// Log-derived aggregates (1-hour rolling window)
	if h.logStats != nil {
		hourlyRequests, hourlyErrors, lastUsedAt, lastErrorAt, avgLatencyMs, err := h.logStats.AggregateProviderLogStats(ctx, providerName)
		if err != nil {
			return ProviderStats{}, fmt.Errorf("aggregate log stats for provider %s: %w", providerName, err)
		}
		stats.HourlyRequests = hourlyRequests
		stats.HourlyErrors = hourlyErrors
		stats.LastUsedAt = lastUsedAt
		stats.LastErrorAt = lastErrorAt
		stats.AvgLatencyMs = avgLatencyMs
		stats.Uptime = computeUptime(hourlyRequests, hourlyErrors)
	} else {
		stats.Uptime = 1
	}

	return stats, nil
}

// computeKeysHealthStatus aggregates per-key list-models failure flags.
// Any key flagged list_models_failed marks the provider "degraded"; a provider
// with no keys is "unknown"; otherwise "healthy".
func computeKeysHealthStatus(keys []schemas.Key) string {
	if len(keys) == 0 {
		return "unknown"
	}
	for _, key := range keys {
		if key.Status == schemas.KeyStatusListModelsFailed {
			return "degraded"
		}
	}
	return "healthy"
}

// computeKeysEnabled reports whether the provider serves traffic: true when any
// key is enabled (nil Enabled means enabled by default). A provider with no
// keys defaults to true like computeUptime's empty-data default.
func computeKeysEnabled(keys []schemas.Key) bool {
	for _, key := range keys {
		if key.Enabled == nil || *key.Enabled {
			return true
		}
	}
	return false
}

// computeUptime returns the health ratio. Empty data defaults to 1.
func computeUptime(requests, errors int) float64 {
	if requests == 0 {
		return 1
	}
	uptime := 1 - float64(errors)/float64(requests)
	if uptime < 0 {
		return 0
	}
	return uptime
}

func getProviderFromCtx(ctx *fasthttp.RequestCtx) (schemas.ModelProvider, error) {
	providerValue := ctx.UserValue("provider")
	if providerValue == nil {
		return "", fmt.Errorf("missing provider parameter")
	}
	providerStr, ok := providerValue.(string)
	if !ok {
		return "", fmt.Errorf("invalid provider parameter type")
	}

	decoded, err := url.PathUnescape(providerStr)
	if err != nil {
		return "", fmt.Errorf("invalid provider parameter encoding: %v", err)
	}

	return schemas.ModelProvider(decoded), nil
}

func validateRetryBackoff(networkConfig *schemas.NetworkConfig) error {
	if networkConfig != nil {
		if networkConfig.RetryBackoffInitial > 0 {
			if networkConfig.RetryBackoffInitial < lib.MinRetryBackoff {
				return fmt.Errorf("retry backoff initial must be at least %v", lib.MinRetryBackoff)
			}
			if networkConfig.RetryBackoffInitial > lib.MaxRetryBackoff {
				return fmt.Errorf("retry backoff initial must be at most %v", lib.MaxRetryBackoff)
			}
		}
		if networkConfig.RetryBackoffMax > 0 {
			if networkConfig.RetryBackoffMax < lib.MinRetryBackoff {
				return fmt.Errorf("retry backoff max must be at least %v", lib.MinRetryBackoff)
			}
			if networkConfig.RetryBackoffMax > lib.MaxRetryBackoff {
				return fmt.Errorf("retry backoff max must be at most %v", lib.MaxRetryBackoff)
			}
		}
		if networkConfig.RetryBackoffInitial > 0 && networkConfig.RetryBackoffMax > 0 {
			if networkConfig.RetryBackoffInitial > networkConfig.RetryBackoffMax {
				return fmt.Errorf("retry backoff initial must be less than or equal to retry backoff max")
			}
		}
	}
	return nil
}

// upsertModelCatalogEntries handles PUT /api/models/catalog — batch-upserts
// the additional_attributes JSON on the pricing rows keyed by
// (model, provider). Every requested (model, provider) must already exist in
// governance_model_pricing; the whole batch is rejected atomically if any
// entry is missing. An entry with an empty AdditionalAttributes map clears
// the column for that (model, provider).
func (h *ProviderHandler) upsertModelCatalogEntries(ctx *fasthttp.RequestCtx) {
	var payload []ModelPricingAttributesEntry
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	for i := range payload {
		payload[i].Model = strings.TrimSpace(payload[i].Model)
		payload[i].Provider = strings.TrimSpace(payload[i].Provider)
		if payload[i].Model == "" || payload[i].Provider == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "model and provider are required for every catalog entry")
			return
		}
	}

	if err := h.modelsManager.UpsertModelPricingAttributes(ctx, payload); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to upsert catalog entries: %v", err))
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// TestModelRequest is the body of POST /api/providers/{provider}/test-model.
type TestModelRequest struct {
	Model string `json:"model"`
	KeyID string `json:"key_id,omitempty"`
}

// TestModelResponse is the body returned by the test-model endpoint.
type TestModelResponse struct {
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// testProviderModel handles POST /api/providers/{provider}/test-model.
// It sends a minimal chat completion request to validate that a model is
// reachable and responds with the latency or an error message.
func (h *ProviderHandler) testProviderModel(ctx *fasthttp.RequestCtx) {
	provider, err := getProviderFromCtx(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid provider: %v", err))
		return
	}
	if h.client == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Bifrost client is not available")
		return
	}

	var payload TestModelRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if payload.Model == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "model is required")
		return
	}

	// Build the BifrostContext. The plugin pipeline is left active so logging
	// records the test call in the LLM logs page. Budget/rate-limit checks and
	// VK usage tracking are disabled so the test never affects quotas or stats.
	bfCtx := schemas.NewBifrostContext(ctx, time.Now().Add(30*time.Second))
	bfCtx.SetValue(schemas.BifrostContextKeySkipBudgetAndRateLimits, true)
	bfCtx.SetValue(schemas.BifrostContextKeySkipVirtualKeyUsageTracking, true)

	// If a key_id is specified, resolve it and force it on the context.
	if payload.KeyID != "" {
		rawKey, err := h.inMemoryStore.GetProviderKeyRaw(provider, payload.KeyID)
		if err != nil {
			if errors.Is(err, lib.ErrNotFound) {
				SendError(ctx, fasthttp.StatusNotFound, fmt.Sprintf("Key %q not found", payload.KeyID))
				return
			}
			SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get key: %v", err))
			return
		}
		bfCtx.SetValue(schemas.BifrostContextKeyDirectKey, *rawKey)
	}

	// Build a minimal chat request.
	hi := "hi"
	content := &schemas.ChatMessageContent{ContentStr: &hi}
	msg := schemas.ChatMessage{
		Role:    schemas.ChatMessageRoleUser,
		Content: content,
	}
	maxTokens := 1
	chatReq := &schemas.BifrostChatRequest{
		Provider: provider,
		Model:    payload.Model,
		Input:    []schemas.ChatMessage{msg},
		Params: &schemas.ChatParameters{
			MaxCompletionTokens: &maxTokens,
		},
	}

	start := time.Now()
	resp, bifrostErr := h.client.ChatCompletionRequest(bfCtx, chatReq)
	latencyMs := time.Since(start).Milliseconds()
	bfCtx.Cancel()

	if bifrostErr != nil {
		SendJSON(ctx, TestModelResponse{
			Success:   false,
			LatencyMs: latencyMs,
			Error:     bifrostErr.Error.Message,
		})
		return
	}
	if resp == nil {
		SendJSON(ctx, TestModelResponse{
			Success:   false,
			LatencyMs: latencyMs,
			Error:     "empty response from provider",
		})
		return
	}

	SendJSON(ctx, TestModelResponse{
		Success:   true,
		LatencyMs: latencyMs,
	})
}

// maxTestModelsPerBatch bounds the number of models a single test-models
// request may probe. Each model runs through the full inference pipeline, so
// the cap keeps a mis-specified request from fanning out an unbounded number
// of upstream calls.
const maxTestModelsPerBatch = 50

// TestModelsRequest is the body of POST /api/providers/{provider}/test-models.
type TestModelsRequest struct {
	Models []string `json:"models"`
}

// TestModelResult is a single entry in TestModelsResponse.Results.
type TestModelResult struct {
	Model     string `json:"model"`
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TestModelsResponse is the body returned by the test-models endpoint.
type TestModelsResponse struct {
	Results []TestModelResult `json:"results"`
}

// testProviderModels handles POST /api/providers/{provider}/test-models.
// It probes up to maxTestModelsPerBatch models in one request. Each model runs
// through the same chat-completion pipeline as the single test-model endpoint
// (so every probe is recorded in the LLM logs and classified per-provider),
// but the calls execute with bounded concurrency and failures in one model do
// not abort the rest. Results are returned in request order.
func (h *ProviderHandler) testProviderModels(ctx *fasthttp.RequestCtx) {
	provider, err := getProviderFromCtx(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("Invalid provider: %v", err))
		return
	}
	if h.client == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Bifrost client is not available")
		return
	}

	var payload TestModelsRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if len(payload.Models) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "models is required")
		return
	}
	if len(payload.Models) > maxTestModelsPerBatch {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("too many models: maximum %d per request", maxTestModelsPerBatch))
		return
	}

	// Deduplicate while preserving order so repeated ids are probed once.
	seen := make(map[string]struct{}, len(payload.Models))
	models := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, dup := seen[model]; dup {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	if len(models) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "models is required")
		return
	}

	results := make([]TestModelResult, len(models))
	testConcurrency := 5
	if testConcurrency > len(models) {
		testConcurrency = len(models)
	}

	modelCh := make(chan int)
	g, bfCtx := errgroup.WithContext(context.Background())
	for w := 0; w < testConcurrency; w++ {
		g.Go(func() error {
			for idx := range modelCh {
				results[idx] = h.testOneModel(ctx, provider, models[idx])
			}
			return nil
		})
	}
	g.Go(func() error {
		defer close(modelCh)
		for idx := range models {
			select {
			case modelCh <- idx:
			case <-bfCtx.Done():
				return bfCtx.Err()
			}
		}
		return nil
	})
	_ = g.Wait()

	SendJSON(ctx, TestModelsResponse{Results: results})
}

// testOneModel runs a single minimal chat-completion probe. Mirrors the single
// model test endpoint (skip budget/rate-limit checks and VK usage tracking so
// tests never affect quotas or stats) and always returns a result, never an
// error — failures are captured in the TestModelResult itself.
func (h *ProviderHandler) testOneModel(ctx *fasthttp.RequestCtx, provider schemas.ModelProvider, model string) TestModelResult {
	// The fasthttp request context is safe to share across model workers (each
	// probe gets its own cancellable BifrostContext child), and keeps request
	// values — request ID, headers — visible to the plugin pipeline exactly as
	// the single model test does.
	bfCtx := schemas.NewBifrostContext(ctx, time.Now().Add(30*time.Second))
	defer bfCtx.Cancel()
	bfCtx.SetValue(schemas.BifrostContextKeySkipBudgetAndRateLimits, true)
	bfCtx.SetValue(schemas.BifrostContextKeySkipVirtualKeyUsageTracking, true)

	hi := "hi"
	content := &schemas.ChatMessageContent{ContentStr: &hi}
	msg := schemas.ChatMessage{
		Role:    schemas.ChatMessageRoleUser,
		Content: content,
	}
	maxTokens := 1
	chatReq := &schemas.BifrostChatRequest{
		Provider: provider,
		Model:    model,
		Input:    []schemas.ChatMessage{msg},
		Params: &schemas.ChatParameters{
			MaxCompletionTokens: &maxTokens,
		},
	}

	start := time.Now()
	resp, bifrostErr := h.client.ChatCompletionRequest(bfCtx, chatReq)
	latencyMs := time.Since(start).Milliseconds()

	if bifrostErr != nil {
		return TestModelResult{
			Model:     model,
			Success:   false,
			LatencyMs: latencyMs,
			Error:     bifrostErr.Error.Message,
		}
	}
	if resp == nil {
		return TestModelResult{
			Model:     model,
			Success:   false,
			LatencyMs: latencyMs,
			Error:     "empty response from provider",
		}
	}
	return TestModelResult{
		Model:     model,
		Success:   true,
		LatencyMs: latencyMs,
	}
}
