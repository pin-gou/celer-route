package schemas

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
}

// defaultParamsByProvider is the per-provider registry of supported default
// request parameters. Add a provider here to expose its "Default Parameters"
// module. Values are the same vocabulary the backend ApplyDefaultParameters
// setter registry understands (see core/providers/utils).
//
// Sensenova: reasoning_effort is only accepted by reasoning-capable models
// (e.g. DeepSeek V4 Flash, GLM-5.2) per platform.sensenova.cn docs.
var defaultParamsByProvider = map[ModelProvider][]DefaultParamDefinition{
	Sensenova: {
		{
			Key:     "reasoning_effort",
			Label:   "Reasoning Effort",
			Options: []string{"none", "low", "medium", "high"},
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
			Key:     d.Key,
			Label:   d.Label,
			Options: append([]string(nil), d.Options...),
		}
	}
	return out
}

// ProviderSupportsDefaultParam reports whether the provider has registered the
// given parameter as a configurable default. The injection path uses this to
// ignore defaults for parameters the provider did not opt into.
func ProviderSupportsDefaultParam(provider ModelProvider, key string) bool {
	for _, d := range LookupProviderDefaultParams(provider) {
		if d.Key == key {
			return true
		}
	}
	return false
}
