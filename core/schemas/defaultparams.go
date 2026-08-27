package schemas

import "strings"

// DefaultParamDefinition describes one request parameter a provider supports
// configuring as a per-model default. The registry is static (compiled into
// the binary) rather than runtime-discovered because the UI must render a
// stable set of options that cannot silently change across upgrades.
//
// Providers register only the parameters they actually support. A provider
// with no registered definitions exposes no "Default Parameters" surface in
// the UI, and the injection path ignores any configured default whose key is
// not registered for that provider.
type DefaultParamDefinition struct {
	Key     string   `json:"key"`     // Parameter name as stored in DefaultParameters, e.g. "reasoning_effort"
	Label   string   `json:"label"`   // Human-readable label for the UI
	Options []string `json:"options"` // Allowed values for the UI <select>; empty means free-form
	// ModelPatterns lists model name patterns that accept this parameter. A
	// pattern matches a model name by exact (case-insensitive) match OR
	// case-insensitive substring match. Empty means the parameter applies to
	// all models of the provider. Used to gate both the injection path and the
	// model-edit UI (only models matching a pattern expose the editor).
	ModelPatterns []string `json:"model_patterns,omitempty"`
}

// defaultParamsByProvider is the per-provider registry of supported default
// request parameters. Add a provider here to expose its "Default Parameters"
// module. Values are the same vocabulary the backend ApplyDefaultParameters
// setter registry understands (see core/providers/utils).
//
// Sensenova: reasoning_effort is only accepted by the reasoning-capable models
// DeepSeek V4 Flash and GLM-5.2 per platform.sensenova.cn docs — not all
// sensenova models.
var defaultParamsByProvider = map[ModelProvider][]DefaultParamDefinition{
	Sensenova: {
		{
			Key:           "reasoning_effort",
			Label:         "Reasoning Effort",
			Options:       []string{"none", "low", "medium", "high"},
			ModelPatterns: []string{"deepseek-v4-flash", "glm-5.2"},
		},
	},
}

// LookupProviderDefaultParams returns the registered default-parameter
// definitions for a provider. Unknown providers (including custom providers)
// return an empty slice — their "Default Parameters" module is hidden.
// The returned slice is a defensive copy.
func LookupProviderDefaultParams(provider ModelProvider) []DefaultParamDefinition {
	defs, ok := defaultParamsByProvider[provider]
	if !ok || len(defs) == 0 {
		return nil
	}
	out := make([]DefaultParamDefinition, len(defs))
	for i, d := range defs {
		out[i] = DefaultParamDefinition{
			Key:           d.Key,
			Label:         d.Label,
			Options:       append([]string(nil), d.Options...),
			ModelPatterns: append([]string(nil), d.ModelPatterns...),
		}
	}
	return out
}

// ProviderSupportsDefaultParam reports whether the provider has registered the
// given parameter as a configurable default (regardless of model). The UI uses
// this to decide whether the provider exposes a "Default Parameters" surface
// at all.
func ProviderSupportsDefaultParam(provider ModelProvider, key string) bool {
	for _, d := range LookupProviderDefaultParams(provider) {
		if d.Key == key {
			return true
		}
	}
	return false
}

// ModelMatchesDefaultParamPatterns reports whether a model name matches any of
// the given patterns (case-insensitive exact or substring). An empty pattern
// list matches everything.
func ModelMatchesDefaultParamPatterns(model string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	if model == "" {
		return false
	}
	lower := strings.ToLower(model)
	for _, p := range patterns {
		if p == "" {
			continue
		}
		lp := strings.ToLower(p)
		if lower == lp || strings.Contains(lower, lp) {
			return true
		}
	}
	return false
}

// ProviderModelSupportsDefaultParam reports whether the (provider, model) pair
// supports the given default parameter — i.e. the provider registered the key
// AND the model matches the definition's model patterns. The injection path and
// the model-edit UI both rely on this to avoid configuring/injecting a
// parameter a specific model does not accept.
func ProviderModelSupportsDefaultParam(provider ModelProvider, model, key string) bool {
	for _, d := range LookupProviderDefaultParams(provider) {
		if d.Key != key {
			continue
		}
		return ModelMatchesDefaultParamPatterns(model, d.ModelPatterns)
	}
	return false
}
