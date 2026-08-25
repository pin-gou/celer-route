package schemas

// ProviderErrorCatalog is the static list of well-known type/code values the
// CooldownPolicy UI exposes in its <select> pickers. The catalog is intentionally
// static (compiled into the binary) rather than runtime-discovered because:
//
//   - The UI renders a stable dropdown that must not silently lose options
//     across upgrades when a provider retires an error variant.
//   - Operators need a predictable reference for "what does OpenAI call a
//     rate-limit error" without trawling through response bodies.
//
// Source attribution per provider is recorded inline next to each entry so
// future maintainers know whether an entry comes from public documentation or
// was observed in production traffic.
type ProviderErrorCatalog struct {
	Types []string `json:"types"`
	Codes []string `json:"codes"`
}

// IsEmpty reports whether the catalog has neither types nor codes. Used by
// the UI to decide whether to render the "no known values" fallback.
func (c ProviderErrorCatalog) IsEmpty() bool {
	return len(c.Types) == 0 && len(c.Codes) == 0
}

// providerErrorCatalog is the per-provider reference data. New providers
// added to StandardProviders should also be added here; the unknown-provider
// fallback in LookupProviderErrorCatalog keeps the UI alive in the meantime.
//
// Sources:
//   - "docs": public provider documentation as of 2026-Q1.
//   - "observed": captured in bifrost production traffic 2026-Q2.
//
// When a provider changes its error vocabulary, prefer adding new entries
// (don't reorder existing ones — ordering is part of the UI's stability
// contract) and never delete an entry unless the provider explicitly retires
// the underlying error.
var providerErrorCatalog = map[ModelProvider]ProviderErrorCatalog{
	OpenAI: {
		Types: []string{
			"rate_limit_error",       // docs
			"insufficient_quota",     // docs
			"invalid_request_error", // docs
			"authentication_error",  // docs
			"permission_error",      // docs
			"not_found_error",       // docs
			"server_error",          // docs
			"timeout",               // observed
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_quota",      // docs
			"invalid_api_key",         // docs
			"model_not_found",         // docs
			"context_length_exceeded", // docs
		},
	},
	Anthropic: {
		Types: []string{
			"rate_limit_error",       // observed
			"invalid_request_error",  // docs
			"authentication_error",   // docs
			"permission_error",       // docs
			"not_found_error",        // docs
			"overloaded_error",       // docs
			"api_error",              // docs
		},
		Codes: []string{
			"rate_limit_error",  // observed
			"invalid_api_key",   // docs
			"model_not_found",   // docs
			"billing_error",     // docs
		},
	},
	Gemini: {
		Types: []string{
			"resource_exhausted",   // docs (RESOURCE_EXHAUSTED)
			"invalid_argument",     // docs
			"permission_denied",    // docs
			"not_found",            // docs
			"unauthenticated",      // docs
			"internal",             // docs
			"unavailable",          // docs
			"deadline_exceeded",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",   // docs
			"quota_exceeded",        // docs
			"resource_exhausted",    // docs
			"billing_not_active",    // docs
		},
	},
	Vertex: {
		Types: []string{
			"resource_exhausted",   // docs (same vocabulary as Gemini)
			"invalid_argument",     // docs
			"permission_denied",    // docs
			"not_found",            // docs
			"unauthenticated",      // docs
			"internal",             // docs
		},
		Codes: []string{
			"rate_limit_exceeded",   // docs
			"quota_exceeded",        // docs
			"resource_exhausted",    // docs
		},
	},
	Bedrock: {
		Types: []string{
			"throttlingexception",   // observed
			"validationexception",   // docs
			"accessdeniedexception", // docs
			"resourcenotfoundexception", // docs
			"internalserverexception",   // docs
			"modeltimeout",              // docs
		},
		Codes: []string{
			"throttling",     // docs
			"model_stalled",  // docs
			"provisioned_throughput_exceeded", // docs
		},
	},
	BedrockMantle: {
		Types: []string{
			"throttlingexception",   // observed
			"validationexception",   // docs
			"accessdeniedexception", // docs
		},
		Codes: []string{
			"throttling", // docs
		},
	},
	Azure: {
		Types: []string{
			"rate_limit_error",       // docs (Azure OpenAI inherits OpenAI shape)
			"insufficient_quota",     // docs
			"invalid_request_error",  // docs
			"authentication_error",   // docs
			"server_error",           // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_quota",      // docs
			"invalid_api_key",         // docs
			"deployment_not_found",    // docs
		},
	},
	Cohere: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"unauthorized",            // docs
			"not_found",               // docs
			"internal_server_error",   // docs
		},
		Codes: []string{
			"too_many_requests",       // docs
			"invalid_api_key",         // docs
			"quota_exceeded",          // docs
		},
	},
	Mistral: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"permission_error",        // docs
			"not_found",               // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_quota",      // docs
			"invalid_api_key",         // docs
		},
	},
	Groq: {
		Types: []string{
			"rate_limit_error",        // observed (Groq mirrors OpenAI)
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_quota",      // docs
		},
	},
	DeepSeek: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
			"invalid_api_key",         // docs
		},
	},
	OpenRouter: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"provider_error",          // docs (upstream provider failure)
			"no_provider",             // docs (no upstream provider available)
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_credits",    // docs
		},
	},
	Perplexity: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"invalid_api_key",         // docs
		},
	},
	Fireworks: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"context_length_exceeded", // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_credits",    // docs
		},
	},
	Cerebras: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"invalid_api_key",         // docs
		},
	},
	XAI: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_quota",      // docs
		},
	},
	Nebius: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_credits",    // docs
		},
	},
	Parasail: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
	SGL: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
	Ollama: {
		Types: []string{
			"rate_limit_error",        // observed (uncommon; Ollama is local)
			"invalid_request_error",   // docs
			"server_error",            // docs
		},
		Codes: []string{
			"model_not_found",         // docs
		},
	},
	VLLM: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"server_error",            // docs
		},
		Codes: []string{
			"model_not_found",         // docs
		},
	},
	HuggingFace: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"invalid_api_key",         // docs
		},
	},
	Replicate: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_credit",     // docs
		},
	},
	Elevenlabs: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"quota_exceeded",          // docs
		},
	},
	Runway: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_credits",    // docs
			"invalid_request_error",   // docs
		},
		Codes: []string{
			"insufficient_credits",    // docs
		},
	},
	Runware: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_credits",    // docs
			"invalid_request_error",   // docs
		},
		Codes: []string{
			"insufficient_credits",    // docs
		},
	},
	Sarvam: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
	Wafer: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
	Alibaba: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_quota",      // docs
			"invalid_api_key",         // docs
		},
	},
	AlibabaTokenplan: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
		},
		Codes: []string{
			"token_plan_exhausted",    // observed
			"invalid_api_key",         // docs
		},
	},
	Minimax: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_balance",    // docs
			"invalid_api_key",         // docs
		},
	},
	Moonshot: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"insufficient_balance",    // docs
		},
	},
	Siliconflow: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Volcengine: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Zhipu: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Tencent: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Baidu: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Sensenova: {
		Types: []string{
			"invalid_request_error",  // observed (sensenova tags quota as invalid_request_error)
			"rate_limit_error",       // observed
			"authentication_error",   // docs
			"permission_error",       // docs
		},
		Codes: []string{
			"insufficient_quota",      // observed
			"invalid_api_key",         // docs
			"model_not_found",         // docs
		},
	},
	Baichuan: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Iflytek: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Stepfun: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	XiaomiMimo: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Modelscope: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_balance",    // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"insufficient_balance",    // docs
		},
	},
	Coze: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"token_quota_exceeded",    // docs
		},
	},
	CozeCn: {
		Types: []string{
			"rate_limit_error",        // observed
			"insufficient_quota",      // docs
			"invalid_request_error",   // docs
			"authentication_error",    // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
			"token_quota_exceeded",    // docs
		},
	},
	Opencode: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
			"authentication_error",    // docs
			"server_error",            // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
	OpencodeGo: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
	OpencodeZen: {
		Types: []string{
			"rate_limit_error",        // observed
			"invalid_request_error",   // docs
		},
		Codes: []string{
			"rate_limit_exceeded",     // docs
		},
	},
}

// genericErrorCatalog is the fallback for providers that aren't in the
// per-provider catalog above. It lists generic types/codes that are common
// across LLM providers; the UI shows them so the dropdown is never empty.
var genericErrorCatalog = ProviderErrorCatalog{
	Types: []string{
		"rate_limit_error",
		"insufficient_quota",
		"invalid_request_error",
		"authentication_error",
		"permission_error",
		"not_found_error",
		"server_error",
	},
	Codes: []string{
		"rate_limit_exceeded",
		"insufficient_quota",
		"invalid_api_key",
		"model_not_found",
	},
}

// LookupProviderErrorCatalog returns the catalog for the given provider,
// falling back to genericErrorCatalog for unknown providers (including
// custom provider names). The returned slice is a defensive copy so
// callers can mutate freely without affecting the global catalog.
func LookupProviderErrorCatalog(provider ModelProvider) ProviderErrorCatalog {
	if c, ok := providerErrorCatalog[provider]; ok {
		return ProviderErrorCatalog{
			Types: append([]string(nil), c.Types...),
			Codes: append([]string(nil), c.Codes...),
		}
	}
	return ProviderErrorCatalog{
		Types: append([]string(nil), genericErrorCatalog.Types...),
		Codes: append([]string(nil), genericErrorCatalog.Codes...),
	}
}

// IsKnownProviderErrorType reports whether the (provider, type) pair is in
// the static catalog. The UI uses this to flag "Custom..." entries that
// don't match the catalog — useful signal that the operator may be using
// a non-standard vocabulary or a value the catalog hasn't caught up to yet.
func IsKnownProviderErrorType(provider ModelProvider, typ string) bool {
	c := LookupProviderErrorCatalog(provider)
	for _, t := range c.Types {
		if t == typ {
			return true
		}
	}
	return false
}

// IsKnownProviderErrorCode is the code-side counterpart of
// IsKnownProviderErrorType.
func IsKnownProviderErrorCode(provider ModelProvider, code string) bool {
	c := LookupProviderErrorCatalog(provider)
	for _, cd := range c.Codes {
		if cd == code {
			return true
		}
	}
	return false
}