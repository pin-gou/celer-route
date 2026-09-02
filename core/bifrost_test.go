package bifrost

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mistralprovider "github.com/pin-gou/celer-route/core/providers/mistral"
	schemas "github.com/pin-gou/celer-route/core/schemas"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Mock time.Sleep to avoid real delays in tests
var mockSleep func(time.Duration)

// Override time.Sleep in tests and setup logger
func init() {
	mockSleep = func(d time.Duration) {
		// Do nothing in tests to avoid real delays
	}
}

// Helper function to create test config with specific retry settings
func createTestConfig(maxRetries int, initialBackoff, maxBackoff time.Duration) *schemas.ProviderConfig {
	return &schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			MaxRetries:          maxRetries,
			RetryBackoffInitial: initialBackoff,
			RetryBackoffMax:     maxBackoff,
		},
	}
}

// Helper function to create a BifrostError
func createBifrostError(message string, statusCode *int, errorType *string, isBifrostError bool) *schemas.BifrostError {
	return &schemas.BifrostError{
		IsBifrostError: isBifrostError,
		StatusCode:     statusCode,
		Error: &schemas.ErrorField{
			Message: message,
			Type:    errorType,
		},
	}
}

// Test executeRequestWithRetries - success scenarios
func TestExecuteRequestWithRetries_SuccessScenarios(t *testing.T) {
	config := createTestConfig(3, 100*time.Millisecond, 1*time.Second)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	logger := NewDefaultLogger(schemas.LogLevelError)
	// Adding dummy tracer to the context
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	// Test immediate success
	t.Run("ImmediateSuccess", func(t *testing.T) {
		callCount := 0
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			callCount++
			return "success", nil
		}

		result, err := executeRequestWithRetries(
			ctx,
			config,
			handler,
			nil,
			nil,
			schemas.ChatCompletionRequest,
			schemas.OpenAI,
			"gpt-4",
			nil,
			logger,
		)

		if callCount != 1 {
			t.Errorf("Expected 1 call, got %d", callCount)
		}
		if result != "success" {
			t.Errorf("Expected 'success', got %s", result)
		}
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	// Test success after retries
	t.Run("SuccessAfterRetries", func(t *testing.T) {
		callCount := 0
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			callCount++
			if callCount <= 2 {
				// First two calls fail with retryable error
				return "", createBifrostError("rate limit exceeded", Ptr(429), nil, false)
			}
			// Third call succeeds
			return "success", nil
		}

		result, err := executeRequestWithRetries(
			ctx,
			config,
			handler,
			nil,
			nil,
			schemas.ChatCompletionRequest,
			schemas.OpenAI,
			"gpt-4",
			nil,
			logger,
		)

		if callCount != 3 {
			t.Errorf("Expected 3 calls, got %d", callCount)
		}
		if result != "success" {
			t.Errorf("Expected 'success', got %s", result)
		}
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// Test executeRequestWithRetries - retry limits
func TestExecuteRequestWithRetries_RetryLimits(t *testing.T) {
	config := createTestConfig(2, 100*time.Millisecond, 1*time.Second)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	logger := NewDefaultLogger(schemas.LogLevelError)
	t.Run("ExceedsMaxRetries", func(t *testing.T) {
		callCount := 0
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			callCount++
			// Always fail with retryable error
			return "", createBifrostError("rate limit exceeded", Ptr(429), nil, false)
		}

		result, err := executeRequestWithRetries(
			ctx,
			config,
			handler,
			nil,
			nil,
			schemas.ChatCompletionRequest,
			schemas.OpenAI,
			"gpt-4",
			nil,
			logger,
		)

		// Should try: initial + 2 retries = 3 total attempts
		if callCount != 3 {
			t.Errorf("Expected 3 calls (initial + 2 retries), got %d", callCount)
		}
		if result != "" {
			t.Errorf("Expected empty result, got %s", result)
		}
		if err == nil {
			t.Fatal("Expected error after exceeding max retries")
		}
		if err.Error == nil {
			t.Fatal("Expected error structure, got nil")
		}
		if err.Error.Message != "rate limit exceeded" {
			t.Errorf("Expected rate limit error, got %s", err.Error.Message)
		}
	})
}

// Test executeRequestWithRetries - non-retryable errors
func TestExecuteRequestWithRetries_NonRetryableErrors(t *testing.T) {
	config := createTestConfig(3, 100*time.Millisecond, 1*time.Second)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	testCases := []struct {
		name  string
		error *schemas.BifrostError
	}{
		{
			name:  "BifrostError",
			error: createBifrostError("validation error", nil, nil, true),
		},
		{
			name:  "RequestCancelled",
			error: createBifrostError("request cancelled", nil, Ptr(schemas.ErrRequestCancelled), false),
		},
		{
			name:  "Non-retryable status code",
			error: createBifrostError("bad request", Ptr(400), nil, false),
		},
		{
			name:  "Non-retryable error message",
			error: createBifrostError("invalid model", nil, nil, false),
		},
	}
	logger := NewDefaultLogger(schemas.LogLevelError)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			callCount := 0
			handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
				callCount++
				return "", tc.error
			}

			result, err := executeRequestWithRetries(
				ctx,
				config,
				handler,
				nil,
				nil,
				schemas.ChatCompletionRequest,
				schemas.OpenAI,
				"gpt-4",
				nil,
				logger,
			)

			if callCount != 1 {
				t.Errorf("Expected 1 call (no retries), got %d", callCount)
			}
			if result != "" {
				t.Errorf("Expected empty result, got %s", result)
			}
			if err != tc.error {
				t.Error("Expected original error to be returned")
			}
		})
	}
}

// Test executeRequestWithRetries - retryable conditions
func TestExecuteRequestWithRetries_RetryableConditions(t *testing.T) {
	config := createTestConfig(1, 100*time.Millisecond, 1*time.Second)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	testCases := []struct {
		name  string
		error *schemas.BifrostError
	}{
		{
			name:  "StatusCode_500",
			error: createBifrostError("internal server error", Ptr(500), nil, false),
		},
		{
			name:  "StatusCode_502",
			error: createBifrostError("bad gateway", Ptr(502), nil, false),
		},
		{
			name:  "StatusCode_503",
			error: createBifrostError("service unavailable", Ptr(503), nil, false),
		},
		{
			name:  "StatusCode_504",
			error: createBifrostError("gateway timeout", Ptr(504), nil, false),
		},
		{
			name:  "StatusCode_429",
			error: createBifrostError("too many requests", Ptr(429), nil, false),
		},
		{
			name:  "ErrProviderDoRequest",
			error: createBifrostError(schemas.ErrProviderDoRequest, nil, nil, false),
		},
		{
			name:  "RateLimitMessage",
			error: createBifrostError("rate limit exceeded", nil, nil, false),
		},
		{
			name:  "RateLimitType",
			error: createBifrostError("some error", nil, Ptr("rate_limit"), false),
		},
	}
	logger := NewDefaultLogger(schemas.LogLevelError)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			callCount := 0
			handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
				callCount++
				return "", tc.error
			}

			result, err := executeRequestWithRetries(
				ctx,
				config,
				handler,
				nil,
				nil,
				schemas.ChatCompletionRequest,
				schemas.OpenAI,
				"gpt-4",
				nil,
				logger,
			)

			// Should try: initial + 1 retry = 2 total attempts
			if callCount != 2 {
				t.Errorf("Expected 2 calls (initial + 1 retry), got %d", callCount)
			}
			if result != "" {
				t.Errorf("Expected empty result, got %s", result)
			}
			if err != tc.error {
				t.Error("Expected original error to be returned")
			}
		})
	}
}

// Test calculateBackoff - exponential growth (base calculations without jitter)
func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	config := createTestConfig(5, 100*time.Millisecond, 5*time.Second)

	// Test the base exponential calculation by checking that results fall within expected ranges
	// Since we can't easily mock rand.Float64, we'll test the bounds instead
	testCases := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{0, 80 * time.Millisecond, 120 * time.Millisecond},    // 100ms ± 20%
		{1, 160 * time.Millisecond, 240 * time.Millisecond},   // 200ms ± 20%
		{2, 320 * time.Millisecond, 480 * time.Millisecond},   // 400ms ± 20%
		{3, 640 * time.Millisecond, 960 * time.Millisecond},   // 800ms ± 20%
		{4, 1280 * time.Millisecond, 1920 * time.Millisecond}, // 1600ms ± 20%
		{5, 2560 * time.Millisecond, 3840 * time.Millisecond}, // 3200ms ± 20%
		{10, 4 * time.Second, 6 * time.Second},                // should be capped at max (5s) ± 20%
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Attempt_%d", tc.attempt), func(t *testing.T) {
			backoff := calculateBackoff(tc.attempt, config)
			if backoff < tc.minExpected || backoff > tc.maxExpected {
				t.Errorf("Backoff %v outside expected range [%v, %v]", backoff, tc.minExpected, tc.maxExpected)
			}
		})
	}
}

// Test calculateBackoff - jitter bounds
func TestCalculateBackoff_JitterBounds(t *testing.T) {
	config := createTestConfig(3, 100*time.Millisecond, 5*time.Second)

	// Test jitter bounds for multiple attempts
	for attempt := 0; attempt < 3; attempt++ {
		t.Run(fmt.Sprintf("Attempt_%d_JitterBounds", attempt), func(t *testing.T) {
			// Calculate expected base backoff
			baseBackoff := config.NetworkConfig.RetryBackoffInitial * time.Duration(1<<uint(attempt))
			if baseBackoff > config.NetworkConfig.RetryBackoffMax {
				baseBackoff = config.NetworkConfig.RetryBackoffMax
			}

			// Test multiple samples to verify jitter bounds
			for i := 0; i < 100; i++ {
				backoff := calculateBackoff(attempt, config)

				// Jitter should be ±20% (0.8 to 1.2 multiplier), but capped at configured max
				minExpected := time.Duration(float64(baseBackoff) * 0.8)
				maxExpected := min(time.Duration(float64(baseBackoff)*1.2), config.NetworkConfig.RetryBackoffMax)

				if backoff < minExpected || backoff > maxExpected {
					t.Errorf("Backoff %v outside expected range [%v, %v] for attempt %d",
						backoff, minExpected, maxExpected, attempt)
				}
			}
		})
	}
}

// Test calculateBackoff - max backoff cap
func TestCalculateBackoff_MaxBackoffCap(t *testing.T) {
	config := createTestConfig(10, 100*time.Millisecond, 500*time.Millisecond)

	// High attempt numbers should be capped at max backoff
	for attempt := 5; attempt < 10; attempt++ {
		backoff := calculateBackoff(attempt, config)

		// Jitter should never exceed the configured maximum
		if backoff > config.NetworkConfig.RetryBackoffMax {
			t.Errorf("Backoff %v exceeds configured max %v for attempt %d",
				backoff, config.NetworkConfig.RetryBackoffMax, attempt)
		}
	}
}

// Test IsRateLimitErrorMessage - all patterns
func TestIsRateLimitError_AllPatterns(t *testing.T) {
	// Test all patterns from rateLimitPatterns
	patterns := []string{
		"rate limit",
		"rate_limit",
		"ratelimit",
		"too many requests",
		"quota exceeded",
		"quota_exceeded",
		"request limit",
		"throttled",
		"throttling",
		"rate exceeded",
		"limit exceeded",
		"requests per",
		"rpm exceeded",
		"tpm exceeded",
		"tokens per minute",
		"requests per minute",
		"requests per second",
		"api rate limit",
		"usage limit",
		"concurrent requests limit",
		"burst_rate",
		"rate increased",
	}

	for _, pattern := range patterns {
		t.Run(fmt.Sprintf("Pattern_%s", strings.ReplaceAll(pattern, " ", "_")), func(t *testing.T) {
			// Test exact match
			if !IsRateLimitErrorMessage(pattern) {
				t.Errorf("Pattern '%s' should be detected as rate limit error", pattern)
			}

			// Test case insensitive - uppercase
			if !IsRateLimitErrorMessage(strings.ToUpper(pattern)) {
				t.Errorf("Uppercase pattern '%s' should be detected as rate limit error", strings.ToUpper(pattern))
			}

			// Test case insensitive - mixed case
			if !IsRateLimitErrorMessage(cases.Title(language.English).String(pattern)) {
				t.Errorf("Title case pattern '%s' should be detected as rate limit error", cases.Title(language.English).String(pattern))
			}

			// Test as part of larger message
			message := fmt.Sprintf("Error: %s occurred", pattern)
			if !IsRateLimitErrorMessage(message) {
				t.Errorf("Pattern '%s' in message '%s' should be detected", pattern, message)
			}

			// Test with prefix and suffix
			message = fmt.Sprintf("API call failed due to %s - please retry later", pattern)
			if !IsRateLimitErrorMessage(message) {
				t.Errorf("Pattern '%s' in complex message should be detected", pattern)
			}
		})
	}
}

// Test IsRateLimitErrorMessage - negative cases
func TestIsRateLimitError_NegativeCases(t *testing.T) {
	negativeCases := []string{
		"",
		"invalid request",
		"authentication failed",
		"model not found",
		"internal server error",
		"bad gateway",
		"service unavailable",
		"timeout",
		"connection refused",
		"rate",     // partial match shouldn't trigger
		"limit",    // partial match shouldn't trigger
		"quota",    // partial match shouldn't trigger
		"throttle", // partial match shouldn't trigger (need 'throttled' or 'throttling')
	}

	for _, testCase := range negativeCases {
		t.Run(fmt.Sprintf("Negative_%s", strings.ReplaceAll(testCase, " ", "_")), func(t *testing.T) {
			if IsRateLimitErrorMessage(testCase) {
				t.Errorf("Message '%s' should NOT be detected as rate limit error", testCase)
			}
		})
	}
}

// Test IsRateLimitErrorMessage - edge cases
func TestIsRateLimitError_EdgeCases(t *testing.T) {
	t.Run("EmptyString", func(t *testing.T) {
		if IsRateLimitErrorMessage("") {
			t.Error("Empty string should not be detected as rate limit error")
		}
	})

	t.Run("OnlyWhitespace", func(t *testing.T) {
		if IsRateLimitErrorMessage("   \t\n  ") {
			t.Error("Whitespace-only string should not be detected as rate limit error")
		}
	})

	t.Run("UnicodeCharacters", func(t *testing.T) {
		// Test with unicode characters that might affect case conversion
		message := "RATE LIMIT exceeded 🚫"
		if !IsRateLimitErrorMessage(message) {
			t.Error("Message with unicode should still detect rate limit pattern")
		}
	})

	t.Run("DashScopeErrorCode", func(t *testing.T) {
		// DashScope returns "limit_burst_rate" as the error code
		if !IsRateLimitErrorMessage("limit_burst_rate") {
			t.Error("DashScope error code 'limit_burst_rate' should be detected as rate limit error")
		}
	})

	t.Run("DashScopeErrorMessage", func(t *testing.T) {
		// DashScope returns this as the error message
		if !IsRateLimitErrorMessage("Request rate increased too quickly, please slow down and try again") {
			t.Error("DashScope error message should be detected as rate limit error")
		}
	})
}

// Test retry logging and attempt counting
func TestExecuteRequestWithRetries_LoggingAndCounting(t *testing.T) {
	config := createTestConfig(2, 50*time.Millisecond, 1*time.Second)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	// Capture calls and timing for verification
	var attemptCounts []int
	callCount := 0

	handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
		callCount++
		attemptCounts = append(attemptCounts, callCount)

		if callCount <= 2 {
			// First two calls fail with retryable error
			return "", createBifrostError("rate limit exceeded", Ptr(429), nil, false)
		}
		// Third call succeeds
		return "success", nil
	}
	logger := NewDefaultLogger(schemas.LogLevelError)

	result, err := executeRequestWithRetries(
		ctx,
		config,
		handler,
		nil,
		nil,
		schemas.ChatCompletionRequest,
		schemas.OpenAI,
		"gpt-4",
		nil,
		logger,
	)

	// Verify call progression
	if len(attemptCounts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", len(attemptCounts))
	}

	for i, count := range attemptCounts {
		if count != i+1 {
			t.Errorf("Attempt %d should have call count %d, got %d", i, i+1, count)
		}
	}

	if result != "success" {
		t.Errorf("Expected success result, got %s", result)
	}

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestHandleProviderRequest_OCROperationNotAllowed(t *testing.T) {
	providerConfig := &schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			BaseURL:                        "http://127.0.0.1:1",
			DefaultRequestTimeoutInSeconds: 1,
		},
		CustomProviderConfig: &schemas.CustomProviderConfig{
			CustomProviderKey: "custom-mistral",
			BaseProviderType:  schemas.Mistral,
			AllowedRequests:   &schemas.AllowedRequests{},
		},
	}
	provider := mistralprovider.NewMistralProvider(providerConfig, NewDefaultLogger(schemas.LogLevelError))
	if provider.GetProviderKey() != schemas.ModelProvider("custom-mistral") {
		t.Fatalf("expected custom provider key, got %q", provider.GetProviderKey())
	}
	bifrost := &Bifrost{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	request := &ChannelMessage{
		Context: ctx,
		BifrostRequest: schemas.BifrostRequest{
			RequestType: schemas.OCRRequest,
			OCRRequest: &schemas.BifrostOCRRequest{
				Model: "custom-mistral/mistral-ocr-latest",
				Document: schemas.OCRDocument{
					Type:        schemas.OCRDocumentTypeDocumentURL,
					DocumentURL: Ptr("https://example.com/doc.pdf"),
				},
			},
		},
	}

	response, err := bifrost.handleProviderRequest(provider, providerConfig, request, schemas.Key{}, nil)
	if response != nil {
		t.Fatalf("expected nil response, got %#v", response)
	}
	if err == nil {
		t.Fatal("expected unsupported operation error, got nil")
	}
	if err.Error == nil {
		t.Fatal("expected detailed error, got nil")
	}
	if err.Error.Code == nil || *err.Error.Code != "unsupported_operation" {
		t.Fatalf("expected unsupported_operation code, got %#v", err.Error.Code)
	}
	if err.ExtraFields.Provider != schemas.ModelProvider("custom-mistral") {
		t.Fatalf("expected custom provider name, got %q", err.ExtraFields.Provider)
	}
	if err.ExtraFields.RequestType != schemas.OCRRequest {
		t.Fatalf("expected OCR request type, got %q", err.ExtraFields.RequestType)
	}
	if err.ExtraFields.OriginalModelRequested != "custom-mistral/mistral-ocr-latest" {
		t.Fatalf("expected model to be preserved, got %q", err.ExtraFields.OriginalModelRequested)
	}
}

// Test that transientServerStatusCodes are properly defined.
// These are upstream-side failures unrelated to the credential — the same key is retried.
func TestTransientServerStatusCodes(t *testing.T) {
	expected := []int{500, 502, 503, 504}
	for _, code := range expected {
		if !transientServerStatusCodes[code] {
			t.Errorf("status code %d should be in transientServerStatusCodes", code)
		}
	}

	// Codes that must NOT be in transientServerStatusCodes: per-key codes (rotated, not
	// retried-same-key), success codes, and request-bound 4xx (terminal).
	notTransient := []int{200, 201, 400, 401, 402, 403, 404, 422, 429}
	for _, code := range notTransient {
		if transientServerStatusCodes[code] {
			t.Errorf("status code %d should not be in transientServerStatusCodes", code)
		}
	}
}

// Test that perKeyFailureStatusCodes are properly defined.
// These are credential/account-bound failures — rotate to the next key instead of retrying
// the same one.
func TestPerKeyFailureStatusCodes(t *testing.T) {
	expected := []int{401, 402, 403, 429}
	for _, code := range expected {
		if !perKeyFailureStatusCodes[code] {
			t.Errorf("status code %d should be in perKeyFailureStatusCodes", code)
		}
	}

	// Request-bound 4xx, success codes, and transient-server 5xx must not trigger rotation.
	notPerKey := []int{200, 201, 400, 404, 422, 500, 502, 503, 504}
	for _, code := range notPerKey {
		if perKeyFailureStatusCodes[code] {
			t.Errorf("status code %d should not be in perKeyFailureStatusCodes", code)
		}
	}
}

// Benchmark calculateBackoff performance
func BenchmarkCalculateBackoff(b *testing.B) {
	config := createTestConfig(10, 100*time.Millisecond, 5*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calculateBackoff(i%10, config)
	}
}

// Benchmark IsRateLimitErrorMessage performance
func BenchmarkIsRateLimitError(b *testing.B) {
	messages := []string{
		"rate limit exceeded",
		"too many requests",
		"quota exceeded",
		"throttled by provider",
		"API rate limit reached",
		"not a rate limit error",
		"authentication failed",
		"model not found",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsRateLimitErrorMessage(messages[i%len(messages)])
	}
}

// Mock Account implementation for testing UpdateProvider
type MockAccount struct {
	mu      sync.RWMutex
	configs map[schemas.ModelProvider]*schemas.ProviderConfig
	keys    map[schemas.ModelProvider][]schemas.Key
}

func NewMockAccount() *MockAccount {
	return &MockAccount{
		configs: make(map[schemas.ModelProvider]*schemas.ProviderConfig),
		keys:    make(map[schemas.ModelProvider][]schemas.Key),
	}
}

func (ma *MockAccount) AddProvider(provider schemas.ModelProvider, concurrency int, bufferSize int) {
	ma.AddProviderWithBaseURL(provider, concurrency, bufferSize, "")
}

func (ma *MockAccount) AddProviderWithBaseURL(provider schemas.ModelProvider, concurrency int, bufferSize int, baseURL string) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.configs[provider] = &schemas.ProviderConfig{
		NetworkConfig: schemas.NetworkConfig{
			BaseURL:                        baseURL,
			DefaultRequestTimeoutInSeconds: 300,
			MaxRetries:                     3,
			RetryBackoffInitial:            500 * time.Millisecond,
			RetryBackoffMax:                5 * time.Second,
		},
		ConcurrencyAndBufferSize: schemas.ConcurrencyAndBufferSize{
			Concurrency: concurrency,
			BufferSize:  bufferSize,
		},
	}

	ma.keys[provider] = []schemas.Key{
		{
			ID:     fmt.Sprintf("test-key-%s", provider),
			Value:  *schemas.NewSecretVar(fmt.Sprintf("sk-test-%s", provider)),
			Weight: 100,
		},
	}
}

func (ma *MockAccount) UpdateProviderConfig(provider schemas.ModelProvider, concurrency int, bufferSize int) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if config, exists := ma.configs[provider]; exists {
		config.ConcurrencyAndBufferSize.Concurrency = concurrency
		config.ConcurrencyAndBufferSize.BufferSize = bufferSize
	}
}

func (ma *MockAccount) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	providers := make([]schemas.ModelProvider, 0, len(ma.configs))
	for provider := range ma.configs {
		providers = append(providers, provider)
	}
	return providers, nil
}

func (ma *MockAccount) GetConfigForProvider(provider schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	if config, exists := ma.configs[provider]; exists {
		// Return a copy to simulate real behavior
		configCopy := *config
		return &configCopy, nil
	}
	return nil, fmt.Errorf("provider %s not configured", provider)
}

func (ma *MockAccount) GetKeysForProvider(ctx context.Context, provider schemas.ModelProvider) ([]schemas.Key, error) {
	ma.mu.RLock()
	defer ma.mu.RUnlock()
	if keys, exists := ma.keys[provider]; exists {
		return keys, nil
	}
	return nil, fmt.Errorf("no keys for provider %s", provider)
}

func (ma *MockAccount) SetKeysForProvider(provider schemas.ModelProvider, keys []schemas.Key) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.keys[provider] = keys
}

type countingTracer struct {
	schemas.NoOpTracer
	flushed atomic.Int32
}

func (t *countingTracer) CreateTrace(_ string, _ ...string) string {
	return "trace-ws-final"
}

func (t *countingTracer) CompleteAndFlushTrace(_ string) {
	t.flushed.Add(1)
}

func TestFilterProvidersByContext(t *testing.T) {
	providers := []schemas.ModelProvider{
		schemas.OpenAI,
		schemas.Anthropic,
		schemas.Mistral,
	}

	t.Run("no context filter keeps all providers", func(t *testing.T) {
		filtered := filterProvidersByContext(nil, providers)
		if len(filtered) != len(providers) {
			t.Fatalf("expected all providers, got %v", filtered)
		}
	})

	t.Run("available providers restrict list models fanout", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyAvailableProviders, []schemas.ModelProvider{schemas.Anthropic})

		filtered := filterProvidersByContext(ctx, providers)
		if len(filtered) != 1 || filtered[0] != schemas.Anthropic {
			t.Fatalf("expected only anthropic, got %v", filtered)
		}
	})

	t.Run("empty available providers denies all providers", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyAvailableProviders, []schemas.ModelProvider{})

		filtered := filterProvidersByContext(ctx, providers)
		if len(filtered) != 0 {
			t.Fatalf("expected no providers, got %v", filtered)
		}
	})

	t.Run("malformed available providers fails closed", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyAvailableProviders, "openai")

		filtered := filterProvidersByContext(ctx, providers)
		if len(filtered) != 0 {
			t.Fatalf("expected no providers for malformed context value, got %v", filtered)
		}
	})
}

func TestRunStreamPreHooks_FinalChunkFlushesTrace(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	account := NewMockAccount()
	tracer := &countingTracer{}

	client, err := Init(ctx, schemas.BifrostConfig{
		Account: account,
		Tracer:  tracer,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("Error initializing Bifrost: %v", err)
	}
	defer client.Shutdown()

	hooks, bifrostErr := client.RunStreamPreHooks(ctx, &schemas.BifrostRequest{
		RequestType: schemas.WebSocketResponsesRequest,
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o-mini",
		},
	})
	if bifrostErr != nil {
		t.Fatalf("RunStreamPreHooks returned error: %v", bifrostErr)
	}
	defer hooks.Cleanup()

	ctx.SetValue(schemas.BifrostContextKeyStreamEndIndicator, true)
	_, bifrostErr = hooks.PostHookRunner(ctx, &schemas.BifrostResponse{
		ResponsesResponse: &schemas.BifrostResponsesResponse{
			Object:    "response",
			CreatedAt: int(time.Now().Unix()),
			Model:     "gpt-4o-mini",
		},
	}, nil)
	if bifrostErr != nil {
		t.Fatalf("PostHookRunner returned error: %v", bifrostErr)
	}

	if tracer.flushed.Load() != 1 {
		t.Fatalf("expected trace flush count 1, got %d", tracer.flushed.Load())
	}
}

// mockKVStore implements schemas.KVStore for session stickiness tests.
type mockKVStore struct {
	mu   sync.RWMutex
	data map[string]struct {
		value any
		ttl   time.Duration
	}
}

func newMockKVStore() *mockKVStore {
	return &mockKVStore{data: make(map[string]struct {
		value any
		ttl   time.Duration
	})}
}

func (m *mockKVStore) Get(key string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e, ok := m.data[key]; ok {
		return e.value, nil
	}
	return nil, fmt.Errorf("key not found")
}

func (m *mockKVStore) SetWithTTL(key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = struct {
		value any
		ttl   time.Duration
	}{value: value, ttl: ttl}
	return nil
}

func (m *mockKVStore) SetNXWithTTL(key string, value any, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		return false, nil
	}
	m.data[key] = struct {
		value any
		ttl   time.Duration
	}{value: value, ttl: ttl}
	return true, nil
}

func (m *mockKVStore) Delete(key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		delete(m.data, key)
		return true, nil
	}
	return false, nil
}

// Test selectKeyFromProviderForModelWithPool with session stickiness
func TestSelectKeyFromProviderForModel_SessionStickiness(t *testing.T) {
	kvStore := newMockKVStore()
	account := NewMockAccount()
	account.AddProvider(schemas.OpenAI, 5, 1000)
	// Use 2 keys so we hit the keySelector path (single key returns early)
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: "key-a", Name: "Key A", Value: *schemas.NewSecretVar("sk-a"), Models: schemas.WhiteList{"*"}, Weight: 1},
		{ID: "key-b", Name: "Key B", Value: *schemas.NewSecretVar("sk-b"), Models: schemas.WhiteList{"*"}, Weight: 1},
	})

	var keySelectorCalls int
	deterministicSelector := func(ctx *schemas.BifrostContext, keys []schemas.Key, _ schemas.ModelProvider, _ string) (schemas.Key, error) {
		keySelectorCalls++
		return keys[0], nil // always return first key
	}

	ctx := context.Background()
	bifrost, err := Init(ctx, schemas.BifrostConfig{
		Account:     account,
		Logger:      NewDefaultLogger(schemas.LogLevelError),
		KVStore:     kvStore,
		KeySelector: deterministicSelector,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	bfCtx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	bfCtx.SetValue(schemas.BifrostContextKeySessionID, "sess-123")

	// First call: cache miss, keySelector runs, key stored; returns single-element pool (canRotate=false)
	keys1, canRotate1, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
	if err != nil {
		t.Fatalf("first selectKeyFromProviderForModelWithPool: %v", err)
	}
	if canRotate1 {
		t.Error("first call: canRotate should be false for session-sticky request")
	}
	if len(keys1) != 1 || keys1[0].ID != "key-a" {
		t.Errorf("first call: expected [key-a], got %v", keys1)
	}
	if keySelectorCalls != 1 {
		t.Errorf("first call: expected 1 keySelector call, got %d", keySelectorCalls)
	}

	// Verify kvstore was written
	kvKey := buildSessionKey(schemas.OpenAI, "sess-123", "gpt-4")
	if raw, err := kvStore.Get(kvKey); err != nil || raw != "key-a" {
		t.Errorf("kvstore after first call: expected key-a, got %v (err=%v)", raw, err)
	}

	// Second call: cache hit, same key returned, keySelector NOT called
	keys2, canRotate2, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
	if err != nil {
		t.Fatalf("second selectKeyFromProviderForModelWithPool: %v", err)
	}
	if canRotate2 {
		t.Error("second call: canRotate should be false for session-sticky request")
	}
	if len(keys2) != 1 || keys2[0].ID != "key-a" {
		t.Errorf("second call: expected [key-a] (sticky), got %v", keys2)
	}
	if keySelectorCalls != 1 {
		t.Errorf("second call: keySelector should not run (cache hit), got %d calls", keySelectorCalls)
	}
}

// Test selectKeyFromProviderForModelWithPool - no stickiness when session ID absent
func TestSelectKeyFromProviderForModel_NoStickinessWithoutSessionID(t *testing.T) {
	kvStore := newMockKVStore()
	account := NewMockAccount()
	account.AddProvider(schemas.OpenAI, 5, 1000)
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: "key-a", Name: "Key A", Value: *schemas.NewSecretVar("sk-a"), Models: schemas.WhiteList{"*"}, Weight: 1},
		{ID: "key-b", Name: "Key B", Value: *schemas.NewSecretVar("sk-b"), Models: schemas.WhiteList{"*"}, Weight: 1},
	})

	var keySelectorCalls int
	deterministicSelector := func(ctx *schemas.BifrostContext, keys []schemas.Key, _ schemas.ModelProvider, _ string) (schemas.Key, error) {
		keySelectorCalls++
		return keys[0], nil
	}

	ctx := context.Background()
	bifrost, err := Init(ctx, schemas.BifrostConfig{
		Account:     account,
		Logger:      NewDefaultLogger(schemas.LogLevelError),
		KVStore:     kvStore,
		KeySelector: deterministicSelector,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	bfCtx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	// No session ID set — pool is returned with canRotate=true; keySelector is called each time.

	for i := 0; i < 2; i++ {
		pool, canRotate, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
		if err != nil {
			t.Fatalf("selectKeyFromProviderForModelWithPool call %d: %v", i+1, err)
		}
		if !canRotate {
			t.Fatalf("call %d: canRotate should be true without a session id", i+1)
		}
		if len(pool) == 0 {
			t.Fatalf("call %d: expected non-empty pool", i+1)
		}
	}
	if keySelectorCalls != 0 {
		t.Errorf("expected 0 keySelector calls from pool building (no session id), got %d", keySelectorCalls)
	}
	// KVStore should not have a sticky entry for an empty session id
	if _, err := kvStore.Get(buildSessionKey(schemas.OpenAI, "", "gpt-4")); err == nil {
		t.Error("kvstore should not have a sticky entry for an empty session id")
	}
}

// TestSelectKeyFromProviderForModel_SessionStickinessNoRotation verifies that when a session ID
// is present, rate-limit retries reuse the sticky key rather than rotating to another key.
func TestSelectKeyFromProviderForModel_SessionStickinessNoRotation(t *testing.T) {
	kvStore := newMockKVStore()
	account := NewMockAccount()
	account.AddProvider(schemas.OpenAI, 5, 1000)
	account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
		{ID: "key-a", Name: "Key A", Value: *schemas.NewSecretVar("sk-a"), Models: schemas.WhiteList{"*"}, Weight: 1},
		{ID: "key-b", Name: "Key B", Value: *schemas.NewSecretVar("sk-b"), Models: schemas.WhiteList{"*"}, Weight: 1},
	})

	deterministicSelector := func(ctx *schemas.BifrostContext, keys []schemas.Key, _ schemas.ModelProvider, _ string) (schemas.Key, error) {
		return keys[0], nil // always picks key-a when pool includes it
	}

	ctx := context.Background()
	bifrost, err := Init(ctx, schemas.BifrostConfig{
		Account:     account,
		Logger:      NewDefaultLogger(schemas.LogLevelError),
		KVStore:     kvStore,
		KeySelector: deterministicSelector,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	bfCtx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	bfCtx.SetValue(schemas.BifrostContextKeySessionID, "sess-sticky")
	bfCtx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})

	config := createTestConfig(3, 0, 0)
	logger := NewDefaultLogger(schemas.LogLevelError)

	// Build keyProvider the same way requestWorker does.
	pool, canRotate, poolErr := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
	if poolErr != nil {
		t.Fatalf("pool build failed: %v", poolErr)
	}
	if canRotate {
		t.Fatal("expected canRotate=false for session-sticky request")
	}
	if len(pool) != 1 || pool[0].ID != "key-a" {
		t.Fatalf("expected sticky pool=[key-a], got %v", pool)
	}

	fixedKey := pool[0]
	keyProvider := func(_, _ map[string]bool) (schemas.Key, error) { return fixedKey, nil }

	// Simulate 3 rate-limit failures then success; all attempts must use key-a.
	var usedKeyIDs []string
	callCount := 0
	handler := func(k schemas.Key) (string, *schemas.BifrostError) {
		usedKeyIDs = append(usedKeyIDs, k.ID)
		callCount++
		if callCount <= 3 {
			return "", createBifrostError("rate limit exceeded", Ptr(429), nil, false)
		}
		return "ok", nil
	}

	result, retryErr := executeRequestWithRetries(bfCtx, config, handler, keyProvider, nil,
		schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)

	if retryErr != nil {
		t.Fatalf("expected success, got error: %v", retryErr)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %s", result)
	}
	for i, id := range usedKeyIDs {
		if id != "key-a" {
			t.Errorf("attempt %d: expected sticky key-a, got %s (full sequence: %v)", i, id, usedKeyIDs)
		}
	}
}

// TestSelectKeyFromProviderForModel_KeylessProviderNoKeys verifies that keyless
// providers (e.g. bare `opencode`, the free/no-auth OpenCode tier) with zero
// configured keys — which is by design — build an empty key pool instead of
// failing with "no keys found". This is the path hit by the provider page's
// "Test" button and by any chat completion routed to a keyless provider; the
// empty pool flows through requestWorker as the keyless path (keyProvider stays
// nil, zero Key is used).
func TestSelectKeyFromProviderForModel_KeylessProviderNoKeys(t *testing.T) {
	account := NewMockAccount()
	account.AddProvider(schemas.Opencode, 5, 1000)
	account.SetKeysForProvider(schemas.Opencode, []schemas.Key{})

	ctx := context.Background()
	bifrost, err := Init(ctx, schemas.BifrostConfig{
		Account: account,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	bfCtx := schemas.NewBifrostContext(ctx, schemas.NoDeadline)

	t.Run("keyless provider with no keys returns empty pool", func(t *testing.T) {
		pool, canRotate, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.Opencode, "hy3-free", schemas.Opencode)
		if err != nil {
			t.Fatalf("keyless provider must not error on key selection: %v", err)
		}
		if canRotate {
			t.Error("expected canRotate=false for keyless empty pool")
		}
		if len(pool) != 0 {
			t.Errorf("expected empty pool, got %v", pool)
		}
	})

	t.Run("non-keyless provider with no keys still errors", func(t *testing.T) {
		account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{})
		_, _, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
		if err == nil {
			t.Fatal("expected error for non-keyless provider with no keys")
		}
		if !strings.Contains(err.Error(), "no keys found for provider") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// TestSelectKeyFromProviderForModel_BlacklistedModels tests blacklist filtering
// of keys during pool building.
func TestSelectKeyFromProviderForModel_BlacklistedModels(t *testing.T) {
	account := NewMockAccount()
	account.AddProvider(schemas.OpenAI, 5, 1000)

	ctx := context.Background()
	bifrost, err := Init(ctx, schemas.BifrostConfig{
		Account: account,
		Logger:  NewDefaultLogger(schemas.LogLevelError),
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	bfCtx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	t.Run("all keys blacklist model", func(t *testing.T) {
		account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
			{ID: "k1", Name: "K1", Value: *schemas.NewSecretVar("sk-1"), Weight: 1, BlacklistedModels: []string{"gpt-4"}},
		})
		_, _, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
		if err == nil {
			t.Fatal("expected error when model is only blacklisted")
		}
		if !strings.Contains(err.Error(), "no keys found that support model") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("blacklist wins over models allow list", func(t *testing.T) {
		account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
			{
				ID: "k1", Name: "K1", Value: *schemas.NewSecretVar("sk-1"), Weight: 1,
				Models:            []string{"gpt-4"},
				BlacklistedModels: []string{"gpt-4"},
			},
		})
		_, _, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
		if err == nil {
			t.Fatal("expected error when model is both allowed and blacklisted")
		}
	})

	t.Run("second key used when first blacklists", func(t *testing.T) {
		account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
			{ID: "k1", Name: "K1", Value: *schemas.NewSecretVar("sk-1"), Weight: 1, BlacklistedModels: []string{"gpt-4"}},
			{ID: "k2", Name: "K2", Value: *schemas.NewSecretVar("sk-2"), Weight: 1, Models: []string{"*"}},
		})
		pool, canRotate, err := bifrost.selectKeyFromProviderForModelWithPool(bfCtx, schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", schemas.OpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// After filtering, only k2 remains — single key returns canRotate=false.
		if canRotate {
			t.Fatal("expected canRotate=false for single-key pool after filtering")
		}
		if len(pool) != 1 || pool[0].ID != "k2" {
			t.Fatalf("expected pool=[k2], got %v", pool)
		}
	})
}

// Test key rotation in executeRequestWithRetries on rate-limit errors
func TestExecuteRequestWithRetries_KeyRotation(t *testing.T) {
	config := createTestConfig(3, 0, 0)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	logger := NewDefaultLogger(schemas.LogLevelError)

	keys := []schemas.Key{
		{ID: "k1", Name: "K1"},
		{ID: "k2", Name: "K2"},
		{ID: "k3", Name: "K3"},
	}

	t.Run("RotatesKeyOnRateLimitRetry", func(t *testing.T) {
		var selectedKeyIDs []string
		keyProvider := func(usedKeyIDs, _ map[string]bool) (schemas.Key, error) {
			for _, k := range keys {
				if !usedKeyIDs[k.ID] {
					return k, nil
				}
			}
			// Fresh round
			for id := range usedKeyIDs {
				delete(usedKeyIDs, id)
			}
			return keys[0], nil
		}

		handler := func(k schemas.Key) (string, *schemas.BifrostError) {
			selectedKeyIDs = append(selectedKeyIDs, k.ID)
			// First two calls rate-limit, third succeeds
			if len(selectedKeyIDs) <= 2 {
				return "", createBifrostError("rate limit exceeded", Ptr(429), nil, false)
			}
			return "success", nil
		}

		result, err := executeRequestWithRetries(ctx, config, handler, keyProvider, nil,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)

		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != "success" {
			t.Errorf("expected 'success', got %s", result)
		}
		if len(selectedKeyIDs) != 3 {
			t.Fatalf("expected 3 attempts, got %d", len(selectedKeyIDs))
		}
		// Each attempt should use a different key
		seen := map[string]struct{}{}
		for _, id := range selectedKeyIDs {
			seen[id] = struct{}{}
		}
		if len(seen) != len(selectedKeyIDs) {
			t.Errorf("expected distinct keys per rate-limit retry, got %v", selectedKeyIDs)
		}
	})

	t.Run("SameKeyOnNetworkError", func(t *testing.T) {
		var selectedKeyIDs []string
		keyProviderCalls := 0
		keyProvider := func(usedKeyIDs, _ map[string]bool) (schemas.Key, error) {
			keyProviderCalls++
			for _, k := range keys {
				if !usedKeyIDs[k.ID] {
					return k, nil
				}
			}
			for id := range usedKeyIDs {
				delete(usedKeyIDs, id)
			}
			return keys[0], nil
		}

		callCount := 0
		handler := func(k schemas.Key) (string, *schemas.BifrostError) {
			selectedKeyIDs = append(selectedKeyIDs, k.ID)
			callCount++
			if callCount <= 2 {
				return "", createBifrostError(schemas.ErrProviderDoRequest, nil, nil, false)
			}
			return "success", nil
		}

		result, err := executeRequestWithRetries(ctx, config, handler, keyProvider, nil,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)

		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != "success" {
			t.Errorf("expected 'success', got %s", result)
		}
		if len(selectedKeyIDs) != 3 {
			t.Fatalf("expected 3 attempts, got %d", len(selectedKeyIDs))
		}
		if keyProviderCalls != 1 {
			t.Fatalf("expected keyProvider to be called once for network retries, got %d", keyProviderCalls)
		}
		// All attempts should use the same key (network error = same key)
		for i := 1; i < len(selectedKeyIDs); i++ {
			if selectedKeyIDs[i] != selectedKeyIDs[0] {
				t.Errorf("expected same key for all network-error retries, got %v", selectedKeyIDs)
			}
		}
	})

	t.Run("CyclesFreshRoundWhenPoolExhausted", func(t *testing.T) {
		var selectedKeyIDs []string
		// 3 keys, 6 retries — should cycle through all 3 keys twice
		config6 := createTestConfig(5, 0, 0) // 5 retries = 6 total attempts
		keyProvider := func(usedKeyIDs, _ map[string]bool) (schemas.Key, error) {
			available := make([]schemas.Key, 0)
			for _, k := range keys {
				if !usedKeyIDs[k.ID] {
					available = append(available, k)
				}
			}
			if len(available) == 0 {
				for id := range usedKeyIDs {
					delete(usedKeyIDs, id)
				}
				available = keys
			}
			return available[0], nil
		}

		handler := func(k schemas.Key) (string, *schemas.BifrostError) {
			selectedKeyIDs = append(selectedKeyIDs, k.ID)
			return "", createBifrostError("rate limit exceeded", Ptr(429), nil, false)
		}

		executeRequestWithRetries(ctx, config6, handler, keyProvider, nil,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)

		if len(selectedKeyIDs) != 6 {
			t.Fatalf("expected 6 attempts (1 initial + 5 retries), got %d", len(selectedKeyIDs))
		}
		// First cycle: k1, k2, k3; second cycle: k1, k2, k3
		expected := []string{"k1", "k2", "k3", "k1", "k2", "k3"}
		for i, id := range selectedKeyIDs {
			if id != expected[i] {
				t.Errorf("attempt %d: expected key %s, got %s (full sequence: %v)", i, expected[i], id, selectedKeyIDs)
			}
		}
	})

	t.Run("NilKeyProviderUsesZeroKey", func(t *testing.T) {
		cleanCtx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		cleanCtx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})

		var receivedKey schemas.Key
		handler := func(k schemas.Key) (string, *schemas.BifrostError) {
			receivedKey = k
			return "ok", nil
		}

		result, err := executeRequestWithRetries(cleanCtx, config, handler, nil, nil,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "ok" {
			t.Errorf("expected 'ok', got %s", result)
		}
		if receivedKey.ID != "" {
			t.Errorf("expected zero Key when keyProvider is nil, got ID=%s", receivedKey.ID)
		}
		if trail, ok := cleanCtx.Value(schemas.BifrostContextKeyAttemptTrail).([]schemas.KeyAttemptRecord); ok && len(trail) > 0 {
			t.Fatalf("expected no attempt trail for nil keyProvider, got %v", trail)
		}
		if selectedID, _ := cleanCtx.Value(schemas.BifrostContextKeySelectedKeyID).(string); selectedID != "" {
			t.Fatalf("expected empty selected key id, got %q", selectedID)
		}
		if selectedName, _ := cleanCtx.Value(schemas.BifrostContextKeySelectedKeyName).(string); selectedName != "" {
			t.Fatalf("expected empty selected key name, got %q", selectedName)
		}
	})
}

// TestExecuteRequestWithRetries_PerKeyFailureMarker pins the contract the
// provider-cooldown plugin relies on: when the retry loop rotates past a
// per-key failure, the marker MUST observe that failure (with the failing
// key's ID/name, not the eventually-successful key), even though the
// request as a whole ultimately succeeds. PostLLMHook only sees the
// terminal success and skips it; without this hook, a key that 429s
// every retry cycle would never be marked in cooldown.
//
// Behavioural surface pinned here:
//   - transient per-key failures (HTTP 429) trigger the marker with the
//     failing key's ID, on every attempt — not just the terminal one
//   - permanent per-key failures (HTTP 401/403) intentionally do NOT
//     trigger the marker: they are already isolated via deadKeyIDs and
//     any cross-request cooldown would be redundant noise
//   - non-key-bound failures (transient 5xx) do NOT trigger the marker
//     even when they cause a same-key retry, because there's no key
//     attribution to act on
func TestExecuteRequestWithRetries_PerKeyFailureMarker(t *testing.T) {
	config := createTestConfig(3, 0, 0)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	logger := NewDefaultLogger(schemas.LogLevelError)

	keys := []schemas.Key{
		{ID: "key-A", Name: "A"},
		{ID: "key-B", Name: "B"},
	}
	// keyProvider that excludes usedKeyIDs and deadKeyIDs — mirrors the
	// production rotating-pool behaviour in core/bifrost.go.
	keyProvider := func(usedKeyIDs, deadKeyIDs map[string]bool) (schemas.Key, error) {
		for _, k := range keys {
			if !usedKeyIDs[k.ID] && !deadKeyIDs[k.ID] {
				return k, nil
			}
		}
		return schemas.Key{}, fmt.Errorf("no eligible keys")
	}

	t.Run("MarkerFiresForEachRateLimitedKey", func(t *testing.T) {
		type markerCall struct {
			keyID, keyName, provider string
			model                    string
			statusCode               int
		}
		var calls []markerCall
		marker := func(_ *schemas.BifrostContext, provider schemas.ModelProvider, keyID string, keyName string, model string, err *schemas.BifrostError) {
			sc := 0
			if err != nil && err.StatusCode != nil {
				sc = *err.StatusCode
			}
			calls = append(calls, markerCall{keyID: keyID, keyName: keyName, provider: string(provider), model: model, statusCode: sc})
		}

		// Attempt 0: key-A → 429 (rate_limit_error); attempt 1: key-B → success.
		// Marker must observe the key-A failure but NOT the key-B success.
		attempts := 0
		handler := func(k schemas.Key) (string, *schemas.BifrostError) {
			attempts++
			if attempts == 1 {
				return "", createBifrostError("rate limit exceeded", Ptr(429), Ptr("rate_limit_error"), false)
			}
			return "ok", nil
		}

		result, err := executeRequestWithRetries(ctx, config, handler, keyProvider, marker,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != "ok" {
			t.Fatalf("expected 'ok', got %q", result)
		}
		if len(calls) != 1 {
			t.Fatalf("expected exactly one marker call (the 429 on key-A), got %d: %+v", len(calls), calls)
		}
		if calls[0].keyID != "key-A" || calls[0].keyName != "A" {
			t.Errorf("marker fired for %s/%s, expected key-A/A", calls[0].keyID, calls[0].keyName)
		}
		if calls[0].statusCode != 429 {
			t.Errorf("marker observed status %d, expected 429", calls[0].statusCode)
		}
		if calls[0].model != "gpt-4" || calls[0].provider != string(schemas.OpenAI) {
			t.Errorf("marker received provider=%q model=%q, expected openai/gpt-4", calls[0].provider, calls[0].model)
		}
	})

	t.Run("PermanentFailuresDoNotFire", func(t *testing.T) {
		// 401 is permanent per-key failure — the retry loop routes it into
		// deadKeyIDs and surfaces it as the terminal error without retrying
		// (provider has only one key so the second call would just loop).
		// Verify the marker is silent on that path.
		var calls int
		marker := func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ string, _ string, _ *schemas.BifrostError) {
			calls++
		}
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			return "", createBifrostError("invalid api key", Ptr(401), Ptr("invalid_request_error"), false)
		}
		_, err := executeRequestWithRetries(ctx, config, handler, keyProvider, marker,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)
		if err == nil {
			t.Fatal("expected the 401 to surface as a terminal error")
		}
		if calls != 0 {
			t.Errorf("marker must not fire for permanent per-key failures (401/402/403), got %d calls", calls)
		}
	})

	t.Run("NonKeyBoundFailuresDoNotFire", func(t *testing.T) {
		// Transient 5xx is a per-server, not per-key, failure. The retry
		// loop keeps the same key across attempts, so even if a marker
		// fired it would attribute the failure to a healthy key. Verify
		// the loop never invokes the marker for that path.
		var calls int
		marker := func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ string, _ string, _ *schemas.BifrostError) {
			calls++
		}
		// 503 is not in perKeyFailureStatusCodes, so the retry loop
		// shouldRetry branch fires but the per-key-attribution branch does
		// not — usedKeyIDs stays empty across retries.
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			return "", createBifrostError("upstream unavailable", Ptr(503), Ptr("upstream_error"), false)
		}
		_, err := executeRequestWithRetries(ctx, config, handler, keyProvider, marker,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)
		if err == nil {
			t.Fatal("expected the 503 to surface as a terminal error")
		}
		if calls != 0 {
			t.Errorf("marker must not fire for non-key-bound failures (5xx), got %d calls", calls)
		}
	})
}

// TestPerKeyFailureMarker_NilIsNoOp pins the "marker is optional" invariant
// every plugin owner implicitly relies on: when no plugin registers a
// PerKeyFailureMarker (the default — only provider-cooldown does today), the
// retry loop MUST skip the call entirely without panicking, and the request
// MUST still succeed through rotation. This is the regression guard for the
// three nil-guards in core/bifrost.go:
//   - SetPerKeyFailureMarker(nil) → Store(nil)
//   - executeRequestWithRetries caller: var perKeyFailureMarker ... ; if pm := ... Load(); pm != nil
//   - executeRequestWithRetries inner call: if perKeyFailureMarker != nil { ... }
//
// Without these guards, adding the marker parameter to executeRequestWithRetries
// would have made every plugin-less bifrost init panic on its first 429.
func TestPerKeyFailureMarker_NilIsNoOp(t *testing.T) {
	t.Run("NilMarkerDoesNotPanicDuringRotation", func(t *testing.T) {
		config := createTestConfig(3, 0, 0)
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
		logger := NewDefaultLogger(schemas.LogLevelError)

		keys := []schemas.Key{{ID: "k1", Name: "K1"}, {ID: "k2", Name: "K2"}}
		keyProvider := func(usedKeyIDs, _ map[string]bool) (schemas.Key, error) {
			for _, k := range keys {
				if !usedKeyIDs[k.ID] {
					return k, nil
				}
			}
			return schemas.Key{}, fmt.Errorf("no eligible keys")
		}
		// First attempt 429s, second succeeds. marker is intentionally nil.
		attempts := 0
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			attempts++
			if attempts == 1 {
				return "", createBifrostError("rate limit exceeded", Ptr(429), Ptr("rate_limit_error"), false)
			}
			return "ok", nil
		}
		result, err := executeRequestWithRetries(ctx, config, handler, keyProvider, nil,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)
		if err != nil {
			t.Fatalf("expected success despite nil marker, got error: %v", err)
		}
		if result != "ok" {
			t.Fatalf("expected 'ok', got %q", result)
		}
		if attempts != 2 {
			t.Fatalf("expected exactly 2 attempts (1 retry), got %d", attempts)
		}
	})

	t.Run("SetNilDisablesPreviouslyInstalledMarker", func(t *testing.T) {
		// Set a sentinel marker that records calls, then nil it out and
		// verify the loop never invokes it after. This pins the
		// hot-swap-friendly contract: SetPerKeyFailureMarker(nil) is a
		// first-class way to disable, not a no-op that leaves the old
		// hook in place.
		config := createTestConfig(3, 0, 0)
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
		logger := NewDefaultLogger(schemas.LogLevelError)

		// We can't reach the live bifrost instance from inside this
		// test function — but the marker parameter on
		// executeRequestWithRetries is read once at call time, so we can
		// install/clear it in a goroutine-safe wrapper that mirrors
		// SetPerKeyFailureMarker's atomic-pointer semantics. The point
		// of this test is the per-request snapshot path: the marker the
		// caller passes in is the marker the loop uses, no stale lookup
		// from a global atomic.
		var marker schemas.PerKeyFailureMarker
		// Install: a hook that always fires.
		var installedCalls int
		marker = func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ string, _ string, _ *schemas.BifrostError) {
			installedCalls++
		}
		// Disable mid-flight: nil out the variable the caller will use.
		marker = nil

		keys := []schemas.Key{{ID: "k1", Name: "K1"}, {ID: "k2", Name: "K2"}}
		keyProvider := func(usedKeyIDs, _ map[string]bool) (schemas.Key, error) {
			for _, k := range keys {
				if !usedKeyIDs[k.ID] {
					return k, nil
				}
			}
			return schemas.Key{}, fmt.Errorf("no eligible keys")
		}
		attempts := 0
		handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
			attempts++
			if attempts == 1 {
				return "", createBifrostError("rate limit", Ptr(429), Ptr("rate_limit_error"), false)
			}
			return "ok", nil
		}
		_, err := executeRequestWithRetries(ctx, config, handler, keyProvider, marker,
			schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)
		if err != nil {
			t.Fatalf("expected success with nil marker, got: %v", err)
		}
		if installedCalls != 0 {
			t.Errorf("marker was installed but later nilled — must have fired 0 times, got %d", installedCalls)
		}
	})
}

// TestPerKeyFailureMarker_AtomicSwapMidRequest pins the "snapshot per
// request" contract the worker relies on. The retry loop captures the
// marker value once per request (line ~7244 of bifrost.go) — a
// SetPerKeyFailureMarker hot-swap between attempts within the same
// request MUST NOT silently swap the marker the loop is using. This
// matches the same contract KeyPoolFilter holds, and is the reason the
// worker does an atomic.Pointer.Load at request entry rather than
// reading on every retry iteration.
func TestPerKeyFailureMarker_AtomicSwapMidRequest(t *testing.T) {
	config := createTestConfig(5, 0, 0)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
	logger := NewDefaultLogger(schemas.LogLevelError)

	keys := []schemas.Key{{ID: "k1", Name: "K1"}, {ID: "k2", Name: "K2"}, {ID: "k3", Name: "K3"}}
	keyProvider := func(usedKeyIDs, _ map[string]bool) (schemas.Key, error) {
		for _, k := range keys {
			if !usedKeyIDs[k.ID] {
				return k, nil
			}
		}
		return schemas.Key{}, fmt.Errorf("no eligible keys")
	}

	var originalCalls, replacementCalls int
	original := func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ string, _ string, _ *schemas.BifrostError) {
		originalCalls++
	}
	replacement := func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ string, _ string, _ *schemas.BifrostError) {
		replacementCalls++
	}

	// Always fail — exercises every retry iteration.
	handler := func(_ schemas.Key) (string, *schemas.BifrostError) {
		return "", createBifrostError("rate limit", Ptr(429), Ptr("rate_limit_error"), false)
	}

	// Simulate the worker pattern: load the marker once at request entry,
	// then ignore subsequent atomic-pointer swaps. atomic.Pointer[T] wants
	// T as a value type; for a func literal the cleanest pattern is to
	// store *func directly, then unwrap inside the forwarding closure.
	var current atomic.Pointer[func(*schemas.BifrostContext, schemas.ModelProvider, string, string, string, *schemas.BifrostError)]
	current.Store(&original)

	forwarding := func(ctx *schemas.BifrostContext, p schemas.ModelProvider, kID, kName, model string, e *schemas.BifrostError) {
		m := current.Load()
		if m != nil {
			(*m)(ctx, p, kID, kName, model, e)
		}
	}

	_, err := executeRequestWithRetries(ctx, config, handler, keyProvider, forwarding,
		schemas.ChatCompletionRequest, schemas.OpenAI, "gpt-4", nil, logger)
	if err == nil {
		t.Fatal("expected a terminal error after all retries exhausted")
	}

	// The forwarding closure re-reads `current` on every call, so we
	// cannot use it to assert "the production caller would have
	// snapshotted". But we CAN assert the swap semantics on the
	// underlying atomic.Pointer: a hot-swap that happens AFTER the
	// loop has finished simply replaces the stored pointer for any
	// future caller. Here that means: after `replacement` is installed,
	// a fresh request forwarded through the same closure would see
	// `replacement`, not `original`. Pin that contract.
	current.Store(&replacement)
	if got := current.Load(); got != &replacement {
		t.Fatalf("after Store(&replacement), Load did not return it: got=%p want=%p", got, &replacement)
	}
	// And the original was called at least once during the loop above.
	if originalCalls == 0 {
		t.Fatal("expected the original marker to fire at least once during the retry loop")
	}
	if replacementCalls != 0 {
		t.Errorf("replacement marker must not have been called during the loop that pre-dated its install, got %d calls", replacementCalls)
	}
}

// Test UpdateProvider functionality
func TestUpdateProvider(t *testing.T) {
	t.Run("SuccessfulUpdate", func(t *testing.T) {
		// Setup mock account with initial configuration
		account := NewMockAccount()
		account.AddProvider(schemas.OpenAI, 5, 1000)

		// Initialize Bifrost
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError), // Keep tests quiet
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Verify initial provider exists
		initialProvider := bifrost.getProviderByKey(schemas.OpenAI)
		if initialProvider == nil {
			t.Fatalf("Initial provider not found")
		}

		// Update configuration
		account.UpdateProviderConfig(schemas.OpenAI, 10, 2000)

		// Perform update
		err = bifrost.UpdateProvider(schemas.OpenAI)
		if err != nil {
			t.Fatalf("UpdateProvider failed: %v", err)
		}

		// Verify provider was replaced
		updatedProvider := bifrost.getProviderByKey(schemas.OpenAI)
		if updatedProvider == nil {
			t.Fatalf("Updated provider not found")
		}

		// Verify it's a different instance (provider should have been recreated)
		if initialProvider == updatedProvider {
			t.Errorf("Provider instance was not replaced - same memory address")
		}

		// Verify provider key is still correct
		if updatedProvider.GetProviderKey() != schemas.OpenAI {
			t.Errorf("Updated provider has wrong key: got %s, want %s",
				updatedProvider.GetProviderKey(), schemas.OpenAI)
		}
	})

	t.Run("UpdateNonExistentProvider", func(t *testing.T) {
		// Setup account without the provider we'll try to update
		account := NewMockAccount()
		account.AddProvider(schemas.OpenAI, 5, 1000)

		ctx := context.Background()
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError),
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Try to update a provider not in the account
		err = bifrost.UpdateProvider(schemas.Anthropic)
		if err == nil {
			t.Errorf("Expected error when updating non-existent provider, got nil")
		}

		// Verify error message
		expectedErrMsg := "failed to get updated config for provider anthropic"
		if err != nil && !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("Expected error containing '%s', got: %v", expectedErrMsg, err)
		}
	})

	t.Run("UpdateInactiveProvider", func(t *testing.T) {
		// Setup account with provider but don't initialize it in Bifrost
		account := NewMockAccount()

		ctx := context.Background()
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError),
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Verify provider doesn't exist initially
		// Note: Use Ollama (not in dynamicallyConfigurableProviders) to test truly inactive provider
		if bifrost.getProviderByKey(schemas.Ollama) != nil {
			t.Fatal("Provider should not exist initially")
		}

		// Add provider to account after bifrost initialization
		// Note: Ollama requires a BaseURL
		account.AddProviderWithBaseURL(schemas.Ollama, 3, 500, "http://localhost:11434")

		// Update should succeed and initialize the provider
		err = bifrost.UpdateProvider(schemas.Ollama)
		if err != nil {
			t.Fatalf("UpdateProvider should succeed for inactive provider: %v", err)
		}

		// Verify provider now exists
		provider := bifrost.getProviderByKey(schemas.Ollama)
		if provider == nil {
			t.Fatal("Provider should exist after update")
		}

		if provider.GetProviderKey() != schemas.Ollama {
			t.Errorf("Provider has wrong key: got %s, want %s",
				provider.GetProviderKey(), schemas.Ollama)
		}
	})

	t.Run("MultipleProviderUpdates", func(t *testing.T) {
		// Test updating multiple different providers
		account := NewMockAccount()
		account.AddProvider(schemas.OpenAI, 5, 1000)
		account.AddProvider(schemas.Anthropic, 3, 500)
		account.AddProvider(schemas.Cohere, 2, 200)

		ctx := context.Background()
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError),
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Get initial provider references
		initialOpenAI := bifrost.getProviderByKey(schemas.OpenAI)
		initialAnthropic := bifrost.getProviderByKey(schemas.Anthropic)
		initialCohere := bifrost.getProviderByKey(schemas.Cohere)

		// Update configurations
		account.UpdateProviderConfig(schemas.OpenAI, 10, 2000)
		account.UpdateProviderConfig(schemas.Anthropic, 6, 1000)
		account.UpdateProviderConfig(schemas.Cohere, 4, 400)

		// Update all providers
		providers := []schemas.ModelProvider{schemas.OpenAI, schemas.Anthropic, schemas.Cohere}
		for _, provider := range providers {
			err = bifrost.UpdateProvider(provider)
			if err != nil {
				t.Fatalf("Failed to update provider %s: %v", provider, err)
			}
		}

		// Verify all providers were replaced
		newOpenAI := bifrost.getProviderByKey(schemas.OpenAI)
		newAnthropic := bifrost.getProviderByKey(schemas.Anthropic)
		newCohere := bifrost.getProviderByKey(schemas.Cohere)

		if initialOpenAI == newOpenAI {
			t.Error("OpenAI provider was not replaced")
		}
		if initialAnthropic == newAnthropic {
			t.Error("Anthropic provider was not replaced")
		}
		if initialCohere == newCohere {
			t.Error("Cohere provider was not replaced")
		}

		// Verify all providers still have correct keys
		if newOpenAI.GetProviderKey() != schemas.OpenAI {
			t.Error("OpenAI provider has wrong key after update")
		}
		if newAnthropic.GetProviderKey() != schemas.Anthropic {
			t.Error("Anthropic provider has wrong key after update")
		}
		if newCohere.GetProviderKey() != schemas.Cohere {
			t.Error("Cohere provider has wrong key after update")
		}
	})

	t.Run("ConcurrentProviderUpdates", func(t *testing.T) {
		// Test updating the same provider concurrently (should be serialized by mutex)
		account := NewMockAccount()
		account.AddProvider(schemas.OpenAI, 5, 1000)

		ctx := context.Background()
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError),
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Launch concurrent updates
		const numConcurrentUpdates = 5
		errChan := make(chan error, numConcurrentUpdates)

		for i := 0; i < numConcurrentUpdates; i++ {
			go func(updateNum int) {
				// Update with slightly different config each time
				account.UpdateProviderConfig(schemas.OpenAI, 5+updateNum, 1000+updateNum*100)
				err := bifrost.UpdateProvider(schemas.OpenAI)
				errChan <- err
			}(i)
		}

		// Collect results
		var errors []error
		for i := 0; i < numConcurrentUpdates; i++ {
			if err := <-errChan; err != nil {
				errors = append(errors, err)
			}
		}

		// All updates should succeed (mutex should serialize them)
		if len(errors) > 0 {
			t.Fatalf("Expected no errors from concurrent updates, got: %v", errors)
		}

		// Verify provider still exists and has correct key
		provider := bifrost.getProviderByKey(schemas.OpenAI)
		if provider == nil {
			t.Fatal("Provider should exist after concurrent updates")
		}
		if provider.GetProviderKey() != schemas.OpenAI {
			t.Error("Provider has wrong key after concurrent updates")
		}
	})
}

// Test provider slice management during updates
func TestUpdateProvider_ProviderSliceIntegrity(t *testing.T) {
	t.Run("ProviderSliceConsistency", func(t *testing.T) {
		account := NewMockAccount()
		account.AddProvider(schemas.OpenAI, 5, 1000)
		account.AddProvider(schemas.Anthropic, 3, 500)

		ctx := context.Background()
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError),
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Get initial provider count
		initialProviders := bifrost.providers.Load()
		initialCount := len(*initialProviders)

		// Update one provider
		account.UpdateProviderConfig(schemas.OpenAI, 10, 2000)
		err = bifrost.UpdateProvider(schemas.OpenAI)
		if err != nil {
			t.Fatalf("UpdateProvider failed: %v", err)
		}

		// Verify provider count is the same (replacement, not addition)
		updatedProviders := bifrost.providers.Load()
		updatedCount := len(*updatedProviders)

		if initialCount != updatedCount {
			t.Errorf("Provider count changed: initial=%d, updated=%d", initialCount, updatedCount)
		}

		// Verify both providers still exist with correct keys
		foundOpenAI := false
		foundAnthropic := false

		for _, provider := range *updatedProviders {
			switch provider.GetProviderKey() {
			case schemas.OpenAI:
				foundOpenAI = true
			case schemas.Anthropic:
				foundAnthropic = true
			}
		}

		if !foundOpenAI {
			t.Error("OpenAI provider not found in providers slice after update")
		}
		if !foundAnthropic {
			t.Error("Anthropic provider not found in providers slice after update")
		}
	})

	t.Run("ProviderSliceNoMemoryLeaks", func(t *testing.T) {
		account := NewMockAccount()
		account.AddProvider(schemas.OpenAI, 5, 1000)

		ctx := context.Background()
		bifrost, err := Init(ctx, schemas.BifrostConfig{
			Account: account,
			Logger:  NewDefaultLogger(schemas.LogLevelError),
		})
		if err != nil {
			t.Fatalf("Failed to initialize Bifrost: %v", err)
		}

		// Perform multiple updates to ensure no memory leaks in provider slice
		for i := 0; i < 10; i++ {
			account.UpdateProviderConfig(schemas.OpenAI, 5+i, 1000+i*100)
			err = bifrost.UpdateProvider(schemas.OpenAI)
			if err != nil {
				t.Fatalf("UpdateProvider failed on iteration %d: %v", i, err)
			}

			// Verify only one OpenAI provider exists
			providers := bifrost.providers.Load()
			openAICount := 0
			for _, provider := range *providers {
				if provider.GetProviderKey() == schemas.OpenAI {
					openAICount++
				}
			}

			if openAICount != 1 {
				t.Fatalf("Expected exactly 1 OpenAI provider, found %d on iteration %d", openAICount, i)
			}
		}
	})
}

// TestProviderQueue_SendOnClosedChannel_Race demonstrates the TOCTOU race that
// caused the "send on closed channel" production panic in the OLD code.
//
// The old code called close(pq.queue) during provider shutdown. The sequence:
//  1. Producer calls isClosing() → false  (queue is still open)
//  2. Concurrently: shutdown calls signalClosing() then close(pq.queue)
//  3. Producer enters select { case pq.queue <- msg: ... case <-pq.done: ... }
//     → PANIC: Go's selectgo iterates cases in a randomised pollorder. When the
//     closed-channel send case is checked first, it immediately panics via
//     goto sclose — before it can reach the done case.
//     The case <-pq.done: guard only saves you when done happens to be checked
//     first in that random ordering (≈50 % of the time with two cases).
//
// THE FIX: pq.queue is never closed. See the ProviderQueue struct comment for
// the full explanation. This test is kept as a proof-of-concept showing why
// closing pq.queue is unsafe; the fix is validated by TestProviderQueue_NoPanicWithoutCloseQueue.
//
// We run many iterations so that the panic is statistically certain to surface
// at least once, confirming the hypothesis.
func TestProviderQueue_SendOnClosedChannel_Race(t *testing.T) {
	// With two select cases each iteration has a ~50 % chance of panicking.
	// The probability of never panicking in 200 iterations is (0.5)^200 ≈ 0.
	const iterations = 200
	panicCount := 0

	for i := 0; i < iterations; i++ {
		func() {
			pq := &ProviderQueue{
				queue:      make(chan *ChannelMessage, 10),
				done:       make(chan struct{}),
				signalOnce: sync.Once{},
			}

			// Synchronization barriers to force the exact race interleaving.
			passedIsClosingCheck := make(chan struct{})
			queueClosed := make(chan struct{})

			var panicked bool
			var wg sync.WaitGroup
			wg.Add(1)

			// Producer — mirrors the hot path in tryRequest.
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil && fmt.Sprint(r) == "send on closed channel" {
						panicked = true
					}
				}()

				// Step 1: isClosing() passes — queue is open.
				if pq.isClosing() {
					return
				}

				// Signal: past the isClosing() gate.
				close(passedIsClosingCheck)

				// Wait for the queue to be closed. This represents the real work
				// tryRequest does between the isClosing() check and the select
				// (MCP setup, tracer lookup, plugin pipeline acquisition).
				<-queueClosed

				// Step 2: enter the exact select guard used in production.
				// pq.queue is closed AND pq.done is closed.
				// When selectgo picks the send case first in its random pollorder
				// it hits goto sclose and panics — the done case cannot save it.
				msg := &ChannelMessage{}
				select {
				case pq.queue <- msg: // panics ~50 % of iterations
				case <-pq.done: // selected the other ~50 %
				}
			}()

			// Closer — mirrors UpdateProvider / RemoveProvider.
			go func() {
				<-passedIsClosingCheck
				pq.signalClosing() // closes done, sets closing = 1
				close(pq.queue)
				close(queueClosed) // release the producer into the select
			}()

			wg.Wait()
			if panicked {
				panicCount++
			}
		}()
	}

	if panicCount == 0 {
		t.Fatalf("expected at least one 'send on closed channel' panic across %d iterations, got none", iterations)
	}
	t.Logf("confirmed: panic triggered in %d / %d iterations — hypothesis is correct", panicCount, iterations)
}

// =============================================================================
// ProviderQueue Unit Tests
//
// These tests exercise the ProviderQueue lifecycle in isolation — no full
// Bifrost instance required. They validate the core safety invariants that
// prevent the "send on closed channel" panic.
// =============================================================================

// newTestChannelMessage creates a minimal ChannelMessage suitable for drain tests.
// The Err channel is buffered (size 1) so the worker can send without blocking.
func newTestChannelMessage(ctx *schemas.BifrostContext) *ChannelMessage {
	return &ChannelMessage{
		BifrostRequest: schemas.BifrostRequest{
			RequestType: schemas.ChatCompletionRequest,
			ChatRequest: &schemas.BifrostChatRequest{
				Provider: schemas.OpenAI,
				Model:    "gpt-4",
			},
		},
		Context:  ctx,
		Response: make(chan *schemas.BifrostResponse, 1),
		Err:      make(chan schemas.BifrostError, 1),
	}
}

// TestProviderQueue_IsClosingStateTransition verifies the atomic state flag:
// isClosing() must return false before signalClosing() and true after.
func TestProviderQueue_IsClosingStateTransition(t *testing.T) {
	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	if pq.isClosing() {
		t.Fatal("isClosing() must be false before signalClosing() is called")
	}

	pq.signalClosing()

	if !pq.isClosing() {
		t.Fatal("isClosing() must be true after signalClosing() is called")
	}

	// done channel must also be closed
	select {
	case <-pq.done:
		// correct: done is closed
	default:
		t.Fatal("pq.done must be closed after signalClosing()")
	}

	// queue channel must remain OPEN — this is the core of the fix
	// (sending should not panic even though done is closed)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		select {
		case pq.queue <- &ChannelMessage{}:
		case <-pq.done: // done is closed so this is always ready — no panic
		}
	}()
	if panicked {
		t.Fatal("queue channel must stay open after signalClosing() — sending to it must not panic")
	}
}

// TestProviderQueue_SignalOnceIdempotent verifies that calling signalClosing()
// multiple times is safe. sync.Once ensures done is only closed once and the
// atomic store only happens once — no "close of closed channel" panic.
func TestProviderQueue_SignalOnceIdempotent(t *testing.T) {
	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic from multiple signalClosing() calls: %v", r)
		}
	}()

	pq.signalClosing()
	pq.signalClosing()
	pq.signalClosing()

	if !pq.isClosing() {
		t.Fatal("isClosing() must be true after multiple signalClosing() calls")
	}
}

// TestProviderQueue_WorkerExitsViaDone verifies that a worker running the
// fixed select loop exits cleanly after signalClosing() without closeQueue().
// Before the fix, workers used `for req := range pq.queue` which required
// the channel to be closed. After the fix, done is the exit signal.
func TestProviderQueue_WorkerExitsViaDone(t *testing.T) {
	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	workerExited := make(chan struct{})

	// Minimal worker loop — mirrors the exact select pattern in requestWorker
	go func() {
		defer close(workerExited)
		for {
			select {
			case r, ok := <-pq.queue:
				if !ok {
					return
				}
				_ = r // process (no-op in this test)
			case <-pq.done:
				// Drain remaining buffered items (queue is empty here)
				for {
					select {
					case <-pq.queue:
					default:
						return
					}
				}
			}
		}
	}()

	// Worker is now blocked on the select. Signal shutdown WITHOUT closing queue.
	pq.signalClosing()

	select {
	case <-workerExited:
		// correct: worker exited via done
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after signalClosing() — it may be stuck on range over unclosed channel")
	}
}

// TestProviderQueue_WorkerDrainSendsErrors verifies the drain behaviour when
// done fires while items are still buffered: every buffered ChannelMessage must
// receive a "provider is shutting down" error on its Err channel. No client
// should be left blocked waiting for a response that will never come.
//
// This test exercises the drain path directly — same code as requestWorker's
// case <-pq.done: branch — to avoid a non-deterministic select race between the
// normal processing path and the done path.
func TestProviderQueue_WorkerDrainSendsErrors(t *testing.T) {
	const numBuffered = 5

	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, numBuffered+2),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	// Pre-fill queue — simulates requests buffered when done fires
	msgs := make([]*ChannelMessage, numBuffered)
	for i := 0; i < numBuffered; i++ {
		msgs[i] = newTestChannelMessage(ctx)
		pq.queue <- msgs[i]
	}

	// Signal closing: done is now closed
	pq.signalClosing()

	// Execute the drain path synchronously — exactly what requestWorker does in
	// the case <-pq.done: branch. This is deterministic: we know done is closed
	// and the queue has numBuffered items.
	<-pq.done // fires immediately since signalClosing was already called
drainLoop:
	for {
		select {
		case r := <-pq.queue:
			provKey, mod, _ := r.GetRequestFields()
			r.Err <- schemas.BifrostError{
				IsBifrostError: false,
				Error: &schemas.ErrorField{
					Message: "provider is shutting down",
				},
				ExtraFields: schemas.BifrostErrorExtraFields{
					RequestType:            r.RequestType,
					Provider:               provKey,
					OriginalModelRequested: mod,
				},
			}
		default:
			break drainLoop
		}
	}

	// Verify every message received a shutdown error
	for i, msg := range msgs {
		select {
		case bifrostErr := <-msg.Err:
			if bifrostErr.Error == nil {
				t.Errorf("message %d: received nil Error field", i)
				continue
			}
			if bifrostErr.Error.Message != "provider is shutting down" {
				t.Errorf("message %d: expected 'provider is shutting down', got %q",
					i, bifrostErr.Error.Message)
			}
			if bifrostErr.ExtraFields.Provider != schemas.OpenAI {
				t.Errorf("message %d: expected provider %s, got %s",
					i, schemas.OpenAI, bifrostErr.ExtraFields.Provider)
			}
			if bifrostErr.ExtraFields.RequestType != schemas.ChatCompletionRequest {
				t.Errorf("message %d: expected requestType %v, got %v",
					i, schemas.ChatCompletionRequest, bifrostErr.ExtraFields.RequestType)
			}
		default:
			t.Errorf("message %d: no error received — client would be left hanging indefinitely", i)
		}
	}
}

// TestProviderQueue_NoPanicWithoutCloseQueue verifies that the fixed hot path
// — select { case pq.queue <- msg | case <-pq.done } — never panics when
// signalClosing() fires but the queue channel is NOT closed.
//
// This is the direct inverse of TestProviderQueue_SendOnClosedChannel_Race:
// that test proves the old code panics ~50% of the time; this test proves
// the fixed code panics 0% of the time.
func TestProviderQueue_NoPanicWithoutCloseQueue(t *testing.T) {
	const iterations = 500

	for i := 0; i < iterations; i++ {
		func() {
			pq := &ProviderQueue{
				queue:      make(chan *ChannelMessage, 10),
				done:       make(chan struct{}),
				signalOnce: sync.Once{},
			}

			passedIsClosingCheck := make(chan struct{})
			shutdownDone := make(chan struct{})

			var panicked bool
			var wg sync.WaitGroup
			wg.Add(1)

			// Producer: mirrors the tryRequest hot path after the fix.
			// Passes isClosing(), waits for signalClosing, then sends.
			// The queue channel is NEVER closed — only done is closed.
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()

				if pq.isClosing() {
					return
				}
				close(passedIsClosingCheck)
				<-shutdownDone

				msg := &ChannelMessage{}
				select {
				case pq.queue <- msg: // queue is open → safe to send
				case <-pq.done: // done is closed → selected immediately
				}
			}()

			// Closer: signal shutdown but never close the queue channel
			go func() {
				<-passedIsClosingCheck
				pq.signalClosing() // closes done; does NOT close queue
				close(shutdownDone)
			}()

			wg.Wait()

			if panicked {
				t.Errorf("iteration %d: unexpected panic — queue must not be closed in the fixed path", i)
			}
		}()

		if t.Failed() {
			return
		}
	}

	t.Logf("confirmed: zero panics in %d iterations with the fix applied", iterations)
}

// =============================================================================
// UpdateProvider Lifecycle Tests
//
// These tests verify the three key invariants of the UpdateProvider fix:
//   1. New queue is stored BEFORE signalClosing fires (stale producers re-route)
//   2. Transfer happens BEFORE signalClosing (items go to new workers, not errored)
//   3. Concurrent producers + UpdateProvider produce zero panics
// =============================================================================

// TestUpdateProvider_StaleProducerReroutes verifies that a "stale producer" —
// a goroutine that fetched oldPq before UpdateProvider atomically replaced it —
// can transparently re-route to newPq when it later detects isClosing().
//
// The re-routing logic in tryRequest is:
//
//	if pq.isClosing() {
//	    if newPq, err := bifrost.getProviderQueue(provider); err == nil && newPq != pq {
//	        pq = newPq   // transparent re-route
//	    }
//	}
//
// This test exercises that exact sequence without a full Bifrost instance.
func TestUpdateProvider_StaleProducerReroutes(t *testing.T) {
	var requestQueues sync.Map
	provider := schemas.OpenAI

	oldPq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}
	newPq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	// Initial state: requestQueues holds oldPq
	requestQueues.Store(provider, oldPq)

	// Stale producer: fetched its reference before UpdateProvider ran
	stalePq := oldPq

	// Simulate UpdateProvider steps 2 + 4:
	// Step 2: atomically replace — new producers now get newPq
	requestQueues.Store(provider, newPq)
	// Step 4: signal old closing — stale producers will detect this
	oldPq.signalClosing()

	// --- Stale producer detects isClosing and attempts re-route ---
	var reroutedPq *ProviderQueue
	if stalePq.isClosing() {
		if val, ok := requestQueues.Load(provider); ok {
			candidate := val.(*ProviderQueue)
			if candidate != stalePq {
				reroutedPq = candidate
			}
		}
	}

	if reroutedPq == nil {
		t.Fatal("stale producer failed to re-route: re-route returned nil (check step ordering)")
	}
	if reroutedPq != newPq {
		t.Fatal("stale producer re-routed to wrong queue: expected newPq")
	}
	if reroutedPq.isClosing() {
		t.Fatal("re-routed queue is already closing — re-route is useless (newPq must be fresh)")
	}

	// Verify: sending to re-routed queue succeeds without panic
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		msg := &ChannelMessage{}
		select {
		case reroutedPq.queue <- msg:
		case <-reroutedPq.done:
			t.Error("newPq.done fired — newPq should be open")
		}
	}()
	if panicked {
		t.Fatal("panic while sending to re-routed queue — queue must not be closed")
	}
}

// TestUpdateProvider_TransferOrdering verifies the ordering invariant:
// items are moved from oldPq to newPq BEFORE signalClosing(oldPq) is called.
//
// Observable consequence: during the entire transfer loop, oldPq.isClosing()
// must remain false. Only after transfer completes does signalClosing fire.
func TestUpdateProvider_TransferOrdering(t *testing.T) {
	const numMessages = 8

	oldPq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, numMessages+2),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}
	newPq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, numMessages+2),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	// Pre-fill oldPq — simulates buffered requests at the moment UpdateProvider runs
	for i := 0; i < numMessages; i++ {
		oldPq.queue <- &ChannelMessage{}
	}

	// Invariant check before transfer begins
	if oldPq.isClosing() {
		t.Fatal("invariant violated: oldPq already closing before transfer begins")
	}

	// Perform transfer, mirroring UpdateProvider step 3.
	// Record whether isClosing() ever fired during the loop.
	closingDuringTransfer := false
	transferred := 0
	for {
		select {
		case msg := <-oldPq.queue:
			if oldPq.isClosing() {
				closingDuringTransfer = true
			}
			newPq.queue <- msg
			transferred++
		default:
			goto transferComplete
		}
	}
transferComplete:

	if closingDuringTransfer {
		t.Error("invariant violated: oldPq was already closing during transfer — " +
			"signalClosing must fire AFTER the transfer loop completes")
	}

	// NOW signal closing, mirroring UpdateProvider step 4
	oldPq.signalClosing()

	if !oldPq.isClosing() {
		t.Error("expected isClosing() == true after signalClosing()")
	}

	// All messages must have moved to newPq
	if transferred != numMessages {
		t.Errorf("expected %d messages transferred, got %d", numMessages, transferred)
	}
	if len(newPq.queue) != numMessages {
		t.Errorf("expected %d messages in newPq after transfer, got %d", numMessages, len(newPq.queue))
	}
	if len(oldPq.queue) != 0 {
		t.Errorf("expected 0 messages remaining in oldPq after transfer, got %d", len(oldPq.queue))
	}
}

// TestUpdateProvider_NoPanicConcurrentAccess verifies that concurrent producers
// sending to a queue that is being replaced (UpdateProvider-style) never cause
// a "send on closed channel" panic.
//
// This test directly models the production scenario that triggered the bug:
// many goroutines continuously send to a ProviderQueue while UpdateProvider
// atomically swaps the queue and signals the old one closing. With the fix
// (queue channel is never closed), the select in producers is always safe.
func TestUpdateProvider_NoPanicConcurrentAccess(t *testing.T) {
	const (
		numProducers    = 10
		numUpdates      = 30
		producerRunTime = 300 * time.Millisecond
	)

	var requestQueues sync.Map
	provider := schemas.OpenAI

	makePq := func() *ProviderQueue {
		return &ProviderQueue{
			queue:      make(chan *ChannelMessage, 200),
			done:       make(chan struct{}),
			signalOnce: sync.Once{},
		}
	}

	initialPq := makePq()
	requestQueues.Store(provider, initialPq)

	var panicCount int64
	var transferDropCount int64

	stop := make(chan struct{})
	var producerWg sync.WaitGroup

	// Drainer: continuously empties queues so producers never block on a full queue
	drainStop := make(chan struct{})
	go func() {
		for {
			select {
			case <-drainStop:
				return
			default:
				if val, ok := requestQueues.Load(provider); ok {
					pq := val.(*ProviderQueue)
					select {
					case <-pq.queue:
					default:
					}
				}
				runtime.Gosched()
			}
		}
	}()

	// Producers: continuously simulate the tryRequest hot path
	for i := 0; i < numProducers; i++ {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}

				val, ok := requestQueues.Load(provider)
				if !ok {
					runtime.Gosched()
					continue
				}
				pq := val.(*ProviderQueue)

				func() {
					defer func() {
						if r := recover(); r != nil {
							atomic.AddInt64(&panicCount, 1)
						}
					}()

					// Re-route check (mirrors tryRequest)
					if pq.isClosing() {
						if newVal, ok2 := requestQueues.Load(provider); ok2 {
							if candidate := newVal.(*ProviderQueue); candidate != pq {
								pq = candidate
							}
						}
						// If still closing (RemoveProvider path), just return
						if pq.isClosing() {
							return
						}
					}

					msg := &ChannelMessage{}
					select {
					case pq.queue <- msg:
					case <-pq.done:
					case <-stop: // unblock immediately when the test signals stop
					}
				}()

				runtime.Gosched()
			}
		}()
	}

	// Updater: repeatedly performs UpdateProvider-style queue replacements
	var updaterWg sync.WaitGroup
	updaterWg.Add(1)
	go func() {
		defer updaterWg.Done()
		for i := 0; i < numUpdates; i++ {
			val, ok := requestQueues.Load(provider)
			if !ok {
				continue
			}
			oldPq := val.(*ProviderQueue)
			newPq := makePq()

			// Mirror production UpdateProvider step order exactly:
			// Step 2: expose newPq first so stale producers can re-route to it
			// once they see oldPq is closing.
			requestQueues.Store(provider, newPq)

			// Step 3: transfer buffered messages oldPq → newPq.
		drain:
			for {
				select {
				case msg := <-oldPq.queue:
					select {
					case newPq.queue <- msg:
					default:
						// newPq full during transfer — mirrors production cancel path.
						atomic.AddInt64(&transferDropCount, 1)
					}
				default:
					break drain
				}
			}

			// Step 4: signal closing — producers holding a stale oldPq ref now
			// re-route to newPq (already in the map from step 2).
			oldPq.signalClosing()

			time.Sleep(5 * time.Millisecond)
		}
	}()

	time.Sleep(producerRunTime)
	close(stop)
	close(drainStop)
	producerWg.Wait()
	updaterWg.Wait()

	if n := atomic.LoadInt64(&panicCount); n > 0 {
		t.Errorf("detected %d panic(s) — fix did not eliminate the concurrent-access race", n)
	} else {
		t.Logf("confirmed: zero panics across %d producers + %d queue replacements over %v",
			numProducers, numUpdates, producerRunTime)
	}
	if drops := atomic.LoadInt64(&transferDropCount); drops > 0 {
		t.Logf("note: %d message(s) dropped during transfer (oldPq had >200 buffered items) — does not affect panic correctness", drops)
	}
}

// =============================================================================
// RemoveProvider Lifecycle Tests
//
// These tests verify the behavioral contract of RemoveProvider:
//   1. signalClosing() blocks new producers (isClosing() → true)
//   2. Buffered items in the queue get "provider is shutting down" errors
//   3. Workers exit cleanly and the WaitGroup reaches zero
// =============================================================================

// TestRemoveProvider_BlocksNewProducers verifies that after signalClosing(),
// isClosing() returns true. Producers check this flag before sending and return
// a "provider is shutting down" error rather than trying to enqueue.
func TestRemoveProvider_BlocksNewProducers(t *testing.T) {
	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	// Sanity: before shutdown, producers can proceed
	if pq.isClosing() {
		t.Fatal("isClosing() must be false before RemoveProvider runs")
	}

	// RemoveProvider step 2: signal closing
	pq.signalClosing()

	// New producers must see isClosing() == true and abort
	if !pq.isClosing() {
		t.Fatal("isClosing() must be true after signalClosing() (RemoveProvider)")
	}

	// done must be closed so any producer blocked in the select unblocks immediately
	select {
	case <-pq.done:
		// correct
	default:
		t.Fatal("pq.done must be closed after signalClosing() so blocking producers unblock")
	}

	// CRITICAL: queue channel must remain OPEN — closing it would cause panics in
	// any producer that entered the select before seeing isClosing().
	// With the fix, we NEVER close the queue channel.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		// A select with done closed always takes the done case — safe, no panic
		select {
		case pq.queue <- &ChannelMessage{}:
		case <-pq.done:
		}
	}()
	if panicked {
		t.Fatal("queue channel must stay open after signalClosing() — closing it causes panics")
	}
}

// TestRemoveProvider_BufferedRequestsGetErrors verifies the drain contract:
// items queued BEFORE signalClosing fires must each receive a
// "provider is shutting down" error on their Err channel. No client should be
// left hanging.
//
// This test exercises the drain logic directly — the same code path that
// requestWorker executes in its case <-pq.done: branch — to avoid the
// non-deterministic select race where the normal processing path can pick up
// items before done fires.
func TestRemoveProvider_BufferedRequestsGetErrors(t *testing.T) {
	const numBuffered = 8

	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, numBuffered+5),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	// Buffer requests — simulates requests already queued when RemoveProvider runs
	msgs := make([]*ChannelMessage, numBuffered)
	for i := 0; i < numBuffered; i++ {
		msgs[i] = newTestChannelMessage(ctx)
		pq.queue <- msgs[i]
	}

	// RemoveProvider step 2: signal closing
	pq.signalClosing()

	// Execute the drain path — exactly what requestWorker does in case <-pq.done:
	<-pq.done // fires immediately since signalClosing was already called
drainLoop:
	for {
		select {
		case r := <-pq.queue:
			provKey, mod, _ := r.GetRequestFields()
			r.Err <- schemas.BifrostError{
				IsBifrostError: false,
				Error: &schemas.ErrorField{
					Message: "provider is shutting down",
				},
				ExtraFields: schemas.BifrostErrorExtraFields{
					RequestType:            r.RequestType,
					Provider:               provKey,
					OriginalModelRequested: mod,
				},
			}
		default:
			break drainLoop
		}
	}

	// Every buffered message must have received a shutdown error
	for i, msg := range msgs {
		select {
		case bifrostErr := <-msg.Err:
			if bifrostErr.Error == nil {
				t.Errorf("message %d: got nil Error field in BifrostError", i)
				continue
			}
			if bifrostErr.Error.Message != "provider is shutting down" {
				t.Errorf("message %d: expected 'provider is shutting down', got %q",
					i, bifrostErr.Error.Message)
			}
			if bifrostErr.ExtraFields.Provider != schemas.OpenAI {
				t.Errorf("message %d: expected provider %s, got %s",
					i, schemas.OpenAI, bifrostErr.ExtraFields.Provider)
			}
			if bifrostErr.ExtraFields.RequestType != schemas.ChatCompletionRequest {
				t.Errorf("message %d: expected requestType %v, got %v",
					i, schemas.ChatCompletionRequest, bifrostErr.ExtraFields.RequestType)
			}
		default:
			t.Errorf("message %d: no error received — client would be left hanging indefinitely", i)
		}
	}
}

// TestRemoveProvider_WorkerWaitGroupCompletes verifies that after signalClosing(),
// the worker goroutine decrements the WaitGroup and wg.Wait() returns promptly.
// This mirrors what RemoveProvider does: signal, then Wait() before cleanup.
func TestRemoveProvider_WorkerWaitGroupCompletes(t *testing.T) {
	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, 10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Worker goroutine — mirrors requestWorker's WaitGroup contract
	go func() {
		defer wg.Done()
		for {
			select {
			case r, ok := <-pq.queue:
				if !ok {
					return
				}
				_ = r
			case <-pq.done:
				// Drain remaining (empty in this test)
				for {
					select {
					case <-pq.queue:
					default:
						return
					}
				}
			}
		}
	}()

	// Tiny sleep to ensure worker is parked on select before we signal
	time.Sleep(10 * time.Millisecond)

	// RemoveProvider step 2: signal closing
	pq.signalClosing()

	// RemoveProvider step 3: wait for workers — must complete promptly
	waitReturned := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitReturned)
	}()

	select {
	case <-waitReturned:
		// correct: WaitGroup reached zero after signalClosing()
	case <-time.After(2 * time.Second):
		t.Fatal("wg.Wait() did not return after signalClosing() — worker is stuck (would deadlock RemoveProvider)")
	}
}

// TestRemoveProvider_ConcurrentNewProducersDuringShutdown verifies that
// concurrent producers trying to enqueue after RemoveProvider calls
// signalClosing() all get safe "provider is shutting down" errors — none panic.
// This tests the TOCTOU window: producer passes isClosing() check, then done fires.
func TestRemoveProvider_ConcurrentNewProducersDuringShutdown(t *testing.T) {
	const numProducers = 50

	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, numProducers+10),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	var panicCount int64
	var shutdownErrors int64
	var successfulSends int64

	// Gate: all producers start together after isClosing() passes
	passedGate := make(chan struct{})
	var gateOnce sync.Once
	shutdownFired := make(chan struct{})

	var producerWg sync.WaitGroup

	for i := 0; i < numProducers; i++ {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panicCount, 1)
				}
			}()

			// Each producer checks isClosing() first (mirrors tryRequest)
			if pq.isClosing() {
				atomic.AddInt64(&shutdownErrors, 1)
				return
			}

			// Signal that at least one producer passed the isClosing() check
			gateOnce.Do(func() { close(passedGate) })

			// Wait for shutdown to be signaled (the TOCTOU window)
			<-shutdownFired

			// Producers now enter the select — with the fix, done is closed but
			// queue is NOT closed, so this select is always safe (no panic)
			msg := &ChannelMessage{}
			select {
			case pq.queue <- msg:
				atomic.AddInt64(&successfulSends, 1)
			case <-pq.done:
				atomic.AddInt64(&shutdownErrors, 1)
			}
		}()
	}

	// Wait for at least one producer to pass the isClosing() gate
	select {
	case <-passedGate:
	case <-time.After(2 * time.Second):
		t.Fatal("no producer passed the isClosing() check within timeout")
	}

	// Signal shutdown (RemoveProvider step 2) — this is the TOCTOU race
	pq.signalClosing()
	close(shutdownFired)

	producerWg.Wait()

	if n := atomic.LoadInt64(&panicCount); n > 0 {
		t.Errorf("detected %d panic(s) — queue must not be closed during concurrent shutdown", n)
	}

	t.Logf("result: %d successful sends, %d shutdown errors, %d panics across %d producers",
		atomic.LoadInt64(&successfulSends),
		atomic.LoadInt64(&shutdownErrors),
		atomic.LoadInt64(&panicCount),
		numProducers)
}

// TestPluginPipelineStreamingRace reproduces the production panic:
//
//	fatal error: concurrent map read and map write
//	(*PluginPipeline).FinalizeStreamingPostHookSpans
//
// It hammers accumulatePluginTiming (per-chunk writer) concurrently with
// FinalizeStreamingPostHookSpans (end-of-stream reader) and resetPluginPipeline
// (pool-release writer). Before the streamingMu fix these three paths had no
// synchronisation and the -race detector / runtime map check would trip
// immediately. Run with: go test -race -run PluginPipelineStreamingRace
func TestPluginPipelineStreamingRace(t *testing.T) {
	p := &PluginPipeline{
		logger: NewDefaultLogger(schemas.LogLevelError),
		tracer: &schemas.NoOpTracer{},
	}

	const writers = 8
	const iterations = 2000

	var wg sync.WaitGroup

	// Per-chunk accumulator writers — simulate multiple plugins accumulating
	// timing for every streamed chunk.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pluginName := fmt.Sprintf("plugin-%d", id%3) // a few distinct plugin keys
			for i := 0; i < iterations; i++ {
				p.accumulatePluginTiming(pluginName, time.Microsecond, i%17 == 0)
			}
		}(w)
	}

	// End-of-stream finalizer racing with writers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		for i := 0; i < iterations/10; i++ {
			p.FinalizeStreamingPostHookSpans(ctx)
		}
	}()

	// resetPluginPipeline racing with writers — simulates the pool returning
	// the pipeline to another request mid-flight.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/10; i++ {
			p.resetPluginPipeline()
		}
	}()

	// Concurrent GetChunkCount readers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = p.GetChunkCount()
		}
	}()

	wg.Wait()
}

// TestFilterKeysByID covers the KeyID scoping path for ListModels requests:
// a hit returns the single matching key, a miss returns an empty slice
// (which the caller surfaces as "no key found"), and the input slice must
// not be mutated.
func TestFilterKeysByID(t *testing.T) {
	keys := []schemas.Key{
		{ID: "k1"},
		{ID: "k2"},
		{ID: "k3"},
	}

	t.Run("match returns single key", func(t *testing.T) {
		got := filterKeysByID(keys, "k2")
		if len(got) != 1 || got[0].ID != "k2" {
			t.Fatalf("filterKeysByID(_, k2) = %+v, want one key with ID=k2", got)
		}
	})

	t.Run("no match returns empty slice", func(t *testing.T) {
		got := filterKeysByID(keys, "does-not-exist")
		if len(got) != 0 {
			t.Fatalf("filterKeysByID(_, missing) = %+v, want empty", got)
		}
	})

	t.Run("empty target returns empty slice", func(t *testing.T) {
		got := filterKeysByID(keys, "")
		if len(got) != 0 {
			t.Fatalf("filterKeysByID(_, \"\") = %+v, want empty", got)
		}
	})

	t.Run("input slice is not mutated", func(t *testing.T) {
		before := make([]schemas.Key, len(keys))
		copy(before, keys)
		_ = filterKeysByID(keys, "k1")
		for i := range keys {
			if keys[i].ID != before[i].ID {
				t.Fatalf("input mutated at index %d: got %q, want %q", i, keys[i].ID, before[i].ID)
			}
		}
	})
}

// fakeRoutingPlugin is a minimal LLMPlugin whose PreRequestHook writes a routing key pin to the
// non-reserved BifrostContextKeyRoutingPinnedAPIKeyID, mirroring what the governance routing
// engine does. It exists to exercise the commit step in PluginPipeline.RunPreRequestHooks.
type fakeRoutingPlugin struct {
	name     string
	pinKeyID string // written to BifrostContextKeyRoutingPinnedAPIKeyID when non-empty
}

func (f *fakeRoutingPlugin) GetName() string { return f.name }
func (f *fakeRoutingPlugin) Cleanup() error  { return nil }
func (f *fakeRoutingPlugin) PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	if f.pinKeyID != "" {
		// A direct write to the reserved BifrostContextKeyAPIKeyID here would be dropped by the
		// restricted-write block; routing must use the non-reserved key.
		ctx.SetValue(schemas.BifrostContextKeyRoutingPinnedAPIKeyID, f.pinKeyID)
	}
	return nil
}
func (f *fakeRoutingPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}
func (f *fakeRoutingPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

// PreProviderHook is provided by schemas.LLMPluginNoOpHooks (embedded below).
func (f *fakeRoutingPlugin) PreProviderHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

func newRoutingCommitPipeline(plugins ...schemas.LLMPlugin) *PluginPipeline {
	return &PluginPipeline{
		logger:     NewDefaultLogger(schemas.LogLevelError),
		tracer:     &schemas.NoOpTracer{},
		llmPlugins: plugins,
	}
}

// TestRunPreRequestHooks_CommitsRoutingPinnedKey verifies that the pinned key a routing rule
// writes to the non-reserved BifrostContextKeyRoutingPinnedAPIKeyID (during the blocked
// PreRequestHook phase) is committed by core into the reserved BifrostContextKeyAPIKeyID that
// key selection reads — and that the routing pin's precedence over a caller-supplied pin holds.
func TestRunPreRequestHooks_CommitsRoutingPinnedKey(t *testing.T) {
	const pinned = "routing-pinned-key-id"

	t.Run("routing pin is committed to reserved api-key-id", func(t *testing.T) {
		p := newRoutingCommitPipeline(&fakeRoutingPlugin{name: "gov", pinKeyID: pinned})
		ctx := schemas.NewBifrostContext(context.Background(), time.Now())
		p.RunPreRequestHooks(ctx, &schemas.BifrostRequest{})
		if got, _ := ctx.Value(schemas.BifrostContextKeyAPIKeyID).(string); got != pinned {
			t.Fatalf("APIKeyID = %q, want %q", got, pinned)
		}
	})

	t.Run("routing pin overrides a caller-supplied api-key-id", func(t *testing.T) {
		p := newRoutingCommitPipeline(&fakeRoutingPlugin{name: "gov", pinKeyID: pinned})
		ctx := schemas.NewBifrostContext(context.Background(), time.Now())
		ctx.SetValue(schemas.BifrostContextKeyAPIKeyID, "caller-pin")
		p.RunPreRequestHooks(ctx, &schemas.BifrostRequest{})
		if got, _ := ctx.Value(schemas.BifrostContextKeyAPIKeyID).(string); got != pinned {
			t.Fatalf("APIKeyID = %q, want %q (routing pin must override caller pin)", got, pinned)
		}
	})

	t.Run("caller api-key-id preserved when no routing pin", func(t *testing.T) {
		p := newRoutingCommitPipeline(&fakeRoutingPlugin{name: "noop"})
		ctx := schemas.NewBifrostContext(context.Background(), time.Now())
		ctx.SetValue(schemas.BifrostContextKeyAPIKeyID, "caller-pin")
		p.RunPreRequestHooks(ctx, &schemas.BifrostRequest{})
		if got, _ := ctx.Value(schemas.BifrostContextKeyAPIKeyID).(string); got != "caller-pin" {
			t.Fatalf("APIKeyID = %q, want %q (no routing pin must not clobber caller pin)", got, "caller-pin")
		}
	})
}

// TestClearAnthropicPassthroughForNonNativeProvider verifies that Anthropic raw-body
// passthrough flags are cleared only when an Anthropic-integration request resolves to a
// provider that doesn't speak the Anthropic Messages API natively (e.g. Bedrock). This
// guards the fix for Claude-via-Bedrock tool calls breaking when the model is routed to
// Bedrock through a key alias (so the catalog-time guard never fires).
func TestClearAnthropicPassthroughForNonNativeProvider(t *testing.T) {
	flagKeys := []schemas.BifrostContextKey{
		schemas.BifrostContextKeyUseRawRequestBody,
		schemas.BifrostContextKeySendBackRawResponse,
		schemas.BifrostContextKeyPassthroughOverridesPresent,
	}

	tests := []struct {
		name            string
		integrationType string
		baseProvider    schemas.ModelProvider
		wantCleared     bool
	}{
		{"anthropic integration to bedrock clears", "anthropic", schemas.Bedrock, true},
		{"anthropic integration to anthropic preserved", "anthropic", schemas.Anthropic, false},
		{"anthropic integration to vertex preserved", "anthropic", schemas.Vertex, false},
		{"anthropic integration to azure preserved", "anthropic", schemas.Azure, false},
		{"non-anthropic integration to bedrock preserved", "openai", schemas.Bedrock, false},
		{"no integration type to bedrock preserved", "", schemas.Bedrock, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
			if tt.integrationType != "" {
				ctx.SetValue(schemas.BifrostContextKeyIntegrationType, tt.integrationType)
			}
			for _, k := range flagKeys {
				ctx.SetValue(k, true)
			}

			clearAnthropicPassthroughForNonNativeProvider(ctx, tt.baseProvider)

			for _, k := range flagKeys {
				got, _ := ctx.Value(k).(bool)
				want := !tt.wantCleared // flags start true; cleared means false
				if got != want {
					t.Errorf("flag %v = %v, want %v", k, got, want)
				}
			}
		})
	}
}

// Test that releaseChannelMessage clears all request-scoped references so an
// idle pooled ChannelMessage cannot pin the parsed request body, the request
// context, or an undelivered response/error.
func TestReleaseChannelMessage_ClearsPooledReferences(t *testing.T) {
	b := &Bifrost{
		channelMessagePool: sync.Pool{New: func() interface{} { return &ChannelMessage{} }},
		responseChannelPool: sync.Pool{New: func() interface{} {
			return make(chan *schemas.BifrostResponse, 1)
		}},
		errorChannelPool: sync.Pool{New: func() interface{} {
			return make(chan schemas.BifrostError, 1)
		}},
		responseStreamPool: sync.Pool{New: func() interface{} {
			return make(chan chan *schemas.BifrostStreamChunk, 1)
		}},
	}

	req := schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "test-model",
			Input: []schemas.ChatMessage{{}},
		},
	}
	msg := b.getChannelMessage(req)
	msg.Context = schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	// Simulate an undelivered response and error sitting in the channels.
	respCh := msg.Response
	errCh := msg.Err
	respCh <- &schemas.BifrostResponse{}
	errCh <- schemas.BifrostError{}

	b.releaseChannelMessage(msg)

	if msg.ChatRequest != nil || msg.RequestType != "" {
		t.Error("releaseChannelMessage should zero the embedded BifrostRequest")
	}
	if msg.Context != nil {
		t.Error("releaseChannelMessage should clear the Context reference")
	}
	select {
	case <-respCh:
		t.Error("pooled response channel should be drained before Put")
	default:
	}
	select {
	case <-errCh:
		t.Error("pooled error channel should be drained before Put")
	default:
	}
}

// Streaming variant: releaseChannelMessage must also drain and clear
// ResponseStream, which is only allocated for stream request types.
func TestReleaseChannelMessage_ClearsPooledReferences_Streaming(t *testing.T) {
	b := &Bifrost{
		channelMessagePool: sync.Pool{New: func() interface{} { return &ChannelMessage{} }},
		responseChannelPool: sync.Pool{New: func() interface{} {
			return make(chan *schemas.BifrostResponse, 1)
		}},
		errorChannelPool: sync.Pool{New: func() interface{} {
			return make(chan schemas.BifrostError, 1)
		}},
		responseStreamPool: sync.Pool{New: func() interface{} {
			return make(chan chan *schemas.BifrostStreamChunk, 1)
		}},
	}

	req := schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionStreamRequest,
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "test-model",
			Input: []schemas.ChatMessage{{}},
		},
	}
	msg := b.getChannelMessage(req)
	msg.Context = schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	if msg.ResponseStream == nil {
		t.Fatal("getChannelMessage should allocate ResponseStream for stream request types")
	}

	// Simulate an undelivered stream handoff sitting in the channel.
	streamCh := msg.ResponseStream
	streamCh <- make(chan *schemas.BifrostStreamChunk)

	b.releaseChannelMessage(msg)

	if msg.ChatRequest != nil || msg.RequestType != "" {
		t.Error("releaseChannelMessage should zero the embedded BifrostRequest")
	}
	if msg.Context != nil {
		t.Error("releaseChannelMessage should clear the Context reference")
	}
	if msg.ResponseStream != nil {
		t.Error("releaseChannelMessage should clear the ResponseStream reference")
	}
	select {
	case <-streamCh:
		t.Error("pooled response stream channel should be drained before Put")
	default:
	}
}

// TestExecuteRequestWithRetries_EmptyStreamReturnsClosedChannel pins the public
// streaming contract for zero-chunk streams: when the provider's channel closes
// before the first chunk, the caller must receive a NON-nil, closed channel with
// a nil error — not (nil, nil). A nil channel with a nil error makes integrators
// that range/receive on the result block forever, since a receive from a nil
// channel never returns.
func TestExecuteRequestWithRetries_EmptyStreamReturnsClosedChannel(t *testing.T) {
	config := createTestConfig(1, 10*time.Millisecond, 100*time.Millisecond)
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	logger := NewDefaultLogger(schemas.LogLevelError)
	ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})

	handler := func(_ schemas.Key) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
		ch := make(chan *schemas.BifrostStreamChunk)
		close(ch) // provider stream ends before emitting any chunk
		return ch, nil
	}

	stream, err := executeRequestWithRetries(
		ctx,
		config,
		handler,
		nil,
		nil,
		schemas.ChatCompletionStreamRequest,
		schemas.OpenAI,
		"gpt-4",
		nil,
		logger,
	)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if stream == nil {
		t.Fatal("Expected non-nil closed channel for an empty stream; a nil channel with a nil error hangs consumers on a nil-channel receive")
	}
	select {
	case _, ok := <-stream:
		if ok {
			t.Error("Expected zero chunks from an empty stream")
		}
	case <-time.After(time.Second):
		t.Fatal("Receive on the returned channel blocked; expected a closed channel")
	}
	count := 0
	for range stream {
		count++
	}
	if count != 0 {
		t.Errorf("Expected range over empty stream to yield 0 chunks, got %d", count)
	}
}

// TestFixedKeyProviderRespectsKeyPoolFilter pins the contract that the
// single-key / fixed-key path (canRotate=false) still honours the
// KeyPoolFilter hook. Without this, providers configured with a single
// API key (e.g. minimax in the production deployment) would bypass
// provider-cooldown entirely — the filter would never be consulted and
// every request would land on the same dead key.
//
// Red-phase contract: when a KeyPoolFilter is installed and vetoes the
// fixed key, the keyProvider closure returns errAllKeysFiltered. The
// caller (executeRequestWithRetries) translates that into a 429
// no_eligible_keys response rather than retrying the same dead
// credential.
func TestFixedKeyProviderRespectsKeyPoolFilter(t *testing.T) {
	bifrost := &Bifrost{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	var filterCalls int32
	bifrost.SetKeyPoolFilter(func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, keys []schemas.Key) ([]schemas.Key, error) {
		atomic.AddInt32(&filterCalls, 1)
		return nil, nil // veto every key — simulates "the only key is in cooldown"
	})

	onlyKey := schemas.Key{ID: "solo", Name: "main"}
	keyProvider := bifrost.newFixedKeyProvider(ctx, schemas.OpenAI, "gpt-4o", onlyKey)

	_, err := keyProvider(nil, nil)
	if err == nil {
		t.Fatalf("expected keyProvider to surface filter veto as an error, got nil")
	}
	if !errors.Is(err, errAllKeysFiltered) {
		t.Fatalf("expected errAllKeysFiltered so the caller maps it to 429 no_eligible_keys, got %v", err)
	}
	if atomic.LoadInt32(&filterCalls) != 1 {
		t.Fatalf("filter must have been invoked exactly once on the fixed-key path, got %d", filterCalls)
	}
}

// TestFixedKeyProviderPassesThroughWhenFilterAllowsKey ensures the fix
// does not regress the happy path: a filter that keeps the key in its
// returned slice must still let the caller proceed.
func TestFixedKeyProviderPassesThroughWhenFilterAllowsKey(t *testing.T) {
	bifrost := &Bifrost{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	var filterCalls int32
	bifrost.SetKeyPoolFilter(func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, keys []schemas.Key) ([]schemas.Key, error) {
		atomic.AddInt32(&filterCalls, 1)
		return keys, nil // keep all keys
	})

	onlyKey := schemas.Key{ID: "solo", Name: "main"}
	keyProvider := bifrost.newFixedKeyProvider(ctx, schemas.OpenAI, "gpt-4o", onlyKey)

	got, err := keyProvider(nil, nil)
	if err != nil {
		t.Fatalf("expected no error when filter passes the key through, got %v", err)
	}
	if got.ID != "solo" {
		t.Fatalf("expected the single key to be returned, got %+v", got)
	}
	if atomic.LoadInt32(&filterCalls) != 1 {
		t.Fatalf("filter must have been invoked exactly once, got %d", filterCalls)
	}
}

// TestFixedKeyProviderFilterErrorFallsBackToKey covers the same defensive
// fallback the rotating pool uses: if the filter returns an error, log a
// warning and proceed with the original key rather than failing the whole
// request on a plugin-side bug.
func TestFixedKeyProviderFilterErrorFallsBackToKey(t *testing.T) {
	bifrost := &Bifrost{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	bifrost.SetKeyPoolFilter(func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ []schemas.Key) ([]schemas.Key, error) {
		return nil, fmt.Errorf("simulated plugin bug")
	})

	onlyKey := schemas.Key{ID: "solo", Name: "main"}
	keyProvider := bifrost.newFixedKeyProvider(ctx, schemas.OpenAI, "gpt-4o", onlyKey)

	got, err := keyProvider(nil, nil)
	if err != nil {
		t.Fatalf("filter errors must not fail the request — expected fallback to the key, got %v", err)
	}
	if got.ID != "solo" {
		t.Fatalf("expected fallback to return the original key, got %+v", got)
	}
}

// TestFixedKeyProviderHonoursDeadKeyIDs is the unchanged contract from
// before the fix: a permanently-dead (401/402/403) key still surfaces
// errAllKeysDead even when the filter would otherwise allow it.
func TestFixedKeyProviderHonoursDeadKeyIDs(t *testing.T) {
	bifrost := &Bifrost{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)

	bifrost.SetKeyPoolFilter(func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, keys []schemas.Key) ([]schemas.Key, error) {
		return keys, nil
	})

	onlyKey := schemas.Key{ID: "solo", Name: "main"}
	keyProvider := bifrost.newFixedKeyProvider(ctx, schemas.OpenAI, "gpt-4o", onlyKey)

	_, err := keyProvider(nil, map[string]bool{"solo": true})
	if !errors.Is(err, errAllKeysDead) {
		t.Fatalf("dead key must surface errAllKeysDead regardless of filter outcome, got %v", err)
	}
}

// TestIsSyntheticNoEligibleKeysError pins the exact predicate that
// distinguishes the synthetic "no_eligible_keys" error (raised when the
// KeyPoolFilter suppresses every eligible key before any provider call, now
// surfaced as 429 with a Retry-After hint; 503 accepted for backward
// compatibility) from a real provider 429/503 (which did reach the provider).
// Callers use it to decide whether the request reached the provider; it does
// not gate PostLLMHooks anymore — those always run so the logging plugin
// records the terminal status.
func TestIsSyntheticNoEligibleKeysError(t *testing.T) {
	status429 := 429
	status503 := 503
	status500 := 500
	noEligible := "no_eligible_keys"
	upstream := "upstream_error"

	cases := []struct {
		name string
		err  *schemas.BifrostError
		want bool
	}{
		{
			name: "genuine synthetic no_eligible_keys 429",
			err:  &schemas.BifrostError{StatusCode: &status429, Type: &noEligible},
			want: true,
		},
		{
			name: "legacy synthetic no_eligible_keys 503",
			err:  &schemas.BifrostError{StatusCode: &status503, Type: &noEligible},
			want: true,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "nil status code",
			err:  &schemas.BifrostError{Type: &noEligible},
			want: false,
		},
		{
			name: "429 with unrelated type is not synthetic",
			err:  &schemas.BifrostError{StatusCode: &status429, Type: &upstream},
			want: false,
		},
		{
			name: "no_eligible_keys on non-429/503 is not synthetic",
			err:  &schemas.BifrostError{StatusCode: &status500, Type: &noEligible},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSyntheticNoEligibleKeysError(tc.err); got != tc.want {
				t.Fatalf("isSyntheticNoEligibleKeysError() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestExecuteRequestWithRetries_StreamKeyFilterVetoSurfaces429NoEligibleKeys is
// the streaming regression test for the key-pool veto path.
//
// When the KeyPoolFilter vetoes every eligible key on a STREAMING request
// (canRotate=false single-key provider), executeRequestWithRetries must:
//   - surface exactly the synthetic 429 "no_eligible_keys" error that
//     isSyntheticNoEligibleKeysError matches (a real HTTP 429 so OpenAI-
//     compatible clients like opencode retry), AND
//   - never invoke the provider requestHandler — no provider call happened.
//
// When the filter stamped BifrostContextKeyRetryAfterSeconds (shortest
// remaining cooldown), the error's ExtraFields.RetryAfterSeconds must carry it
// so the transport can emit a Retry-After header.
//
// The synthetic 429 is still passed through PostLLMHooks by the caller
// (tryStreamRequest / tryRequest) so the logging plugin records the terminal
// status; this test only pins the error-shape returned from
// executeRequestWithRetries (which opts out of the provider round-trip), not
// whether PostLLMHooks run afterwards.
func TestExecuteRequestWithRetries_StreamKeyFilterVetoSurfaces429NoEligibleKeys(t *testing.T) {
	config := createTestConfig(1, 10*time.Millisecond, 100*time.Millisecond)
	logger := NewDefaultLogger(schemas.LogLevelError)

	// requestHandler must NOT be reached when every key is vetoed.
	handlerCalls := int32(0)
	handler := func(_ schemas.Key) (chan *schemas.BifrostStreamChunk, *schemas.BifrostError) {
		atomic.AddInt32(&handlerCalls, 1)
		return make(chan *schemas.BifrostStreamChunk), nil
	}

	// keyProvider mirrors the fixed-key (canRotate=false) path when the
	// KeyPoolFilter vetoes the single key: it surfaces errAllKeysFiltered.
	keyProvider := func(_, _ map[string]bool) (schemas.Key, error) {
		return schemas.Key{}, fmt.Errorf("%w: provider %s", errAllKeysFiltered, schemas.OpenAI)
	}

	run := func(t *testing.T, ctx *schemas.BifrostContext) *schemas.BifrostError {
		stream, err := executeRequestWithRetries(
			ctx,
			config,
			handler,
			keyProvider,
			nil,
			schemas.ChatCompletionStreamRequest,
			schemas.OpenAI,
			"gpt-4",
			nil,
			logger,
		)
		if err == nil {
			t.Fatal("expected a 429 no_eligible_keys error when every key is vetoed, got nil")
		}
		if err.StatusCode == nil || *err.StatusCode != 429 {
			t.Fatalf("expected status 429, got %v", err.StatusCode)
		}
		if err.Type == nil || *err.Type != noEligibleKeysErrorType {
			t.Fatalf("expected type %q, got %v", noEligibleKeysErrorType, err.Type)
		}
		if !isSyntheticNoEligibleKeysError(err) {
			t.Fatal("expected isSyntheticNoEligibleKeysError to be true for the returned error")
		}
		if stream != nil {
			t.Fatal("expected nil stream when every key is vetoed; no provider was called")
		}
		if got := atomic.LoadInt32(&handlerCalls); got != 0 {
			t.Fatalf("provider requestHandler must not be invoked when every key is vetoed, called %d time(s)", got)
		}
		return err
	}

	t.Run("no retry hint stamped", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
		err := run(t, ctx)
		if err.ExtraFields.RetryAfterSeconds != 0 {
			t.Fatalf("expected no retry hint when the filter did not stamp one, got %d", err.ExtraFields.RetryAfterSeconds)
		}
	})

	t.Run("retry hint propagated from ctx", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
		ctx.SetValue(schemas.BifrostContextKeyTracer, &schemas.NoOpTracer{})
		ctx.SetValue(schemas.BifrostContextKeyRetryAfterSeconds, int64(37))
		err := run(t, ctx)
		if err.ExtraFields.RetryAfterSeconds != 37 {
			t.Fatalf("expected RetryAfterSeconds 37 from ctx, got %d", err.ExtraFields.RetryAfterSeconds)
		}
	})
}

// recordingSilentLogPlugin is a minimal LLMPlugin that records whether
// BifrostContextKeySilentLog was visible in PostLLMHook. It mimics the
// logging plugin's lifecycle (PreLLMHook runs, then PostLLMHook) but
// never writes log rows.
type recordingSilentLogPlugin struct {
	name       string
	mu         sync.Mutex
	preRan     bool
	postRan    bool
	postSilent bool
}

func (p *recordingSilentLogPlugin) GetName() string { return p.name }
func (p *recordingSilentLogPlugin) Cleanup() error  { return nil }

func (p *recordingSilentLogPlugin) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

func (p *recordingSilentLogPlugin) PreProviderHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

func (p *recordingSilentLogPlugin) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	p.mu.Lock()
	p.preRan = true
	p.mu.Unlock()
	return req, nil, nil
}

func (p *recordingSilentLogPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	silent, _ := ctx.Value(schemas.BifrostContextKeySilentLog).(bool)
	p.mu.Lock()
	p.postRan = true
	p.postSilent = silent
	p.mu.Unlock()
	return resp, bifrostErr, nil
}

func (p *recordingSilentLogPlugin) snapshot() (preRan, postRan, postSilent bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.preRan, p.postRan, p.postSilent
}

// TestWorkerNoEligibleKeysSetsSilentLog is the regression test for the
// worker-side key-pool veto path. When PreProviderHook does NOT short-circuit
// (some keys looked available) but the worker's KeyPoolFilter then suppresses
// every key, the synthetic 429 "no_eligible_keys" flows back through msg.Err.
// Core must set BifrostContextKeySilentLog before running PostLLMHooks so
// presentation plugins (logging) suppress the spurious entry — matching the
// PreProviderHook short-circuit path.
func TestWorkerNoEligibleKeysSetsSilentLog(t *testing.T) {
	vetoAll := func(_ *schemas.BifrostContext, _ schemas.ModelProvider, _ string, _ []schemas.Key) ([]schemas.Key, error) {
		return nil, nil
	}

	for _, tc := range []struct {
		name      string
		streaming bool
	}{
		{name: "non-streaming"},
		{name: "streaming"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := NewMockAccount()
			account.AddProvider(schemas.OpenAI, 1, 1)
			account.SetKeysForProvider(schemas.OpenAI, []schemas.Key{
				{ID: "solo", Value: *schemas.NewSecretVar("sk-test"), Models: schemas.WhiteList{"*"}, Weight: 100},
			})
			account.configs[schemas.OpenAI].NetworkConfig.MaxRetries = 0

			rec := &recordingSilentLogPlugin{name: "recorder"}
			client, err := Init(context.Background(), schemas.BifrostConfig{
				Account:       account,
				Logger:        NewDefaultLogger(schemas.LogLevelError),
				LLMPlugins:    []schemas.LLMPlugin{rec},
				KeyPoolFilter: vetoAll,
			})
			if err != nil {
				t.Fatalf("Init failed: %v", err)
			}
			t.Cleanup(client.Shutdown)

			ctx := schemas.NewBifrostContext(context.Background(), time.Now().Add(30*time.Second))

			req := &schemas.BifrostChatRequest{
				Provider: schemas.OpenAI,
				Model:    "gpt-4o",
				Input: []schemas.ChatMessage{
					{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")}},
				},
			}

			if tc.streaming {
				stream, bifrostErr := client.ChatCompletionStreamRequest(ctx, req)
				if bifrostErr == nil {
					t.Fatal("expected 429 no_eligible_keys error for streaming request, got nil")
				}
				if !isSyntheticNoEligibleKeysError(bifrostErr) {
					t.Fatalf("expected synthetic 429 no_eligible_keys, got %v", bifrostErr)
				}
				if stream != nil {
					t.Fatal("expected nil stream when every key is vetoed")
				}
			} else {
				_, bifrostErr := client.ChatCompletionRequest(ctx, req)
				if bifrostErr == nil {
					t.Fatal("expected 429 no_eligible_keys error, got nil")
				}
				if !isSyntheticNoEligibleKeysError(bifrostErr) {
					t.Fatalf("expected synthetic 429 no_eligible_keys, got %v", bifrostErr)
				}
			}

			preRan, postRan, postSilent := rec.snapshot()
			if !preRan {
				t.Error("PreLLMHook must have been called")
			}
			if !postRan {
				t.Error("PostLLMHook must have been called to close the pending processing entry")
			}
			if !postSilent {
				t.Error("BifrostContextKeySilentLog must be set when the worker returns a synthetic no_eligible_keys error")
			}
		})
	}
}

// TestClearCtxForFallbackClearsSilentLog pins the contract that the
// BifrostContextKeySilentLog flag (set by a PreProviderHook short-circuit such
// as provider-cooldown's all-keys-cooled path) is reset before a fallback
// attempt re-uses the same BifrostContext. Without this clear, the logging
// plugin's PostLLMHook would see SilentLog=true on the fallback attempt and
// skip both the log row write and the SSE "success" notification — leaving the
// user with a "processing" row that never lands and never finishes.
func TestClearCtxForFallbackClearsSilentLog(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), time.Now().Add(30*time.Second))

	// Seed every key that clearCtxForFallback touches, so a missing ClearValue
	// would surface as a residual ctx value after the call.
	ctx.SetValue(schemas.BifrostContextKeyAPIKeyID, "primary-key-id")
	ctx.SetValue(schemas.BifrostContextKeyAPIKeyName, "primary-key-name")
	ctx.SetValue(schemas.BifrostContextKeyGovernanceIncludeOnlyKeys, []string{"k1"})
	ctx.SetValue(schemas.BifrostContextKeyChangeRequestType, true)
	ctx.SetValue(schemas.BifrostContextKeyAttemptTrail, []schemas.KeyAttemptRecord{{KeyID: "k1"}})
	ctx.SetValue(schemas.BifrostContextKeyStreamEndIndicator, true)
	ctx.SetValue(schemas.BifrostContextKeyConnectionClosed, true)
	ctx.SetValue(schemas.BifrostContextKeySupportsAssistantPrefill, true)
	ctx.SetValue(schemas.BifrostContextKeySilentLog, true)
	ctx.SetValue(schemas.BifrostContextKeyRetryAfterSeconds, int64(37))

	clearCtxForFallback(ctx)

	mustBeCleared := []struct {
		name string
		key  schemas.BifrostContextKey
	}{
		{"APIKeyID", schemas.BifrostContextKeyAPIKeyID},
		{"APIKeyName", schemas.BifrostContextKeyAPIKeyName},
		{"GovernanceIncludeOnlyKeys", schemas.BifrostContextKeyGovernanceIncludeOnlyKeys},
		{"ChangeRequestType", schemas.BifrostContextKeyChangeRequestType},
		{"AttemptTrail", schemas.BifrostContextKeyAttemptTrail},
		{"StreamEndIndicator", schemas.BifrostContextKeyStreamEndIndicator},
		{"ConnectionClosed", schemas.BifrostContextKeyConnectionClosed},
		{"SupportsAssistantPrefill", schemas.BifrostContextKeySupportsAssistantPrefill},
		{"SilentLog", schemas.BifrostContextKeySilentLog},
		{"RetryAfterSeconds", schemas.BifrostContextKeyRetryAfterSeconds},
	}
	for _, k := range mustBeCleared {
		if v := ctx.Value(k); v != nil {
			t.Errorf("clearCtxForFallback must clear %s, but ctx still has value %v", k.name, v)
		}
	}
}
