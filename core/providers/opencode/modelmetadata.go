package opencode

// Target format constants for the OpenCode wire-format switch.
const (
	targetFormatOpenAI = "openai"
	targetFormatClaude = "claude"
)

// claudeFormatModels is the set of model ids that must be routed through the
// Anthropic-compatible Messages endpoint instead of /v1/chat/completions.
//
// These models (qwen3.x, minimax-m2.5+, glm-5 family) reject OpenAI-compatible
// payloads with errors like "Model X is not supported for format oa-compat"
// and instead return SSE in the Claude event shape even when the request hits
// /v1/chat/completions. Routing them through /v1/messages with the
// anthropic-version header is what makes the body shape match.
//
// Aligned with the OmniRoute registry entries for opencode-zen/opencode-go:
//   - qwen3.5-plus / qwen3.6-plus / qwen3.7-max / qwen3.7-plus
//   - minimax-m2.5 / minimax-m2.7 / minimax-m3
//   - glm-5 / glm-5.1 / glm-5.2
var claudeFormatModels = map[string]struct{}{
	// Qwen family
	"qwen3.5-plus":      {},
	"qwen3.6-plus":      {},
	"qwen3.7-max":       {},
	"qwen3.7-plus":      {},
	"qwen3.6-plus-high": {},
	"qwen3.6-plus-max":  {},
	"qwen3.7-max-high":  {},
	"qwen3.7-max-max":   {},
	"qwen3.7-plus-high": {},
	"qwen3.7-plus-max":  {},
	// MiniMax family
	"minimax-m2.5": {},
	"minimax-m2.7": {},
	"minimax-m3":   {},
	// GLM family — issue #2292 in OmniRoute says these reject oa-compat too.
	"glm-5":   {},
	"glm-5.1": {},
	"glm-5.2": {},
}

// textOnlyModels is the set of model ids that do NOT support vision inputs,
// regardless of what upstream discovery metadata reports. Mirrors the
// OmniRoute visionBridgeDefaults list for opencode-go:
//   - kimi-k2.5 / kimi-k2.6 / kimi-k3
//   - deepseek-v4-flash / deepseek-v4-pro
//   - glm-5 family
//   - qwen3.x family
//
// Surfaced as a helper so future vision-bridge logic can read from it.
var textOnlyModels = map[string]struct{}{
	"kimi-k2.5":              {},
	"kimi-k2.6":              {},
	"kimi-k2.7-code":         {},
	"kimi-k3":                {},
	"deepseek-v4-flash":      {},
	"deepseek-v4-flash-high": {},
	"deepseek-v4-flash-max":  {},
	"deepseek-v4-pro":        {},
	"deepseek-v4-pro-low":    {},
	"deepseek-v4-pro-medium": {},
	"deepseek-v4-pro-high":   {},
	"deepseek-v4-pro-max":    {},
	"glm-5":                  {},
	"glm-5.1":                {},
	"glm-5.2":                {},
	"glm-5.2-high":           {},
	"glm-5.2-max":            {},
	"qwen3.5-plus":           {},
	"qwen3.6-plus":           {},
	"qwen3.6-plus-high":      {},
	"qwen3.6-plus-max":       {},
	"qwen3.7-max":            {},
	"qwen3.7-max-high":       {},
	"qwen3.7-max-max":        {},
	"qwen3.7-plus":           {},
	"qwen3.7-plus-high":      {},
	"qwen3.7-plus-max":       {},
}

// anthropicAPIVersion is the value sent in the anthropic-version header when
// routing a model through the Messages endpoint.
const anthropicAPIVersion = "2023-06-01"

// opencodeFreeModels is the set of model ids that are free on the bare
// `opencode` (no-auth) tier. Any model not in this set and not ending in
// `-free` requires an API key. Mirrors the OmniRoute OPENCODE_FREE_MODELS
// set, updated to match the live https://opencode.ai/zen/v1/models catalog.
var opencodeFreeModels = map[string]struct{}{
	"big-pickle":                  {},
	"deepseek-v4-flash-free":      {},
	"mimo-v2.5-free":              {},
	"hy3-free":                    {},
	"nemotron-3-ultra-free":       {},
	"north-mini-code-free":        {},
	"laguna-s-2.1-free":           {},
	"nemotron-3.5-lightning-free": {},
}

// isFreeOpencodeModel reports whether the model is accessible without an API
// key on the bare `opencode` (free) tier. A model is free when it is in the
// opencodeFreeModels set or its id ends with "-free". Exported for tests.
func isFreeOpencodeModel(model string) bool {
	if _, ok := opencodeFreeModels[model]; ok {
		return true
	}
	return len(model) >= 5 && model[len(model)-5:] == "-free"
}

// getModelTargetFormat returns the wire format that the given model must be
// routed through. Returns "claude" for entries in claudeFormatModels, otherwise
// "openai".
func getModelTargetFormat(model string) string {
	if _, ok := claudeFormatModels[model]; ok {
		return targetFormatClaude
	}
	return targetFormatOpenAI
}

// isClaudeFormatModel reports whether the given model id must go through the
// Anthropic Messages endpoint. Exported for tests and external callers.
func isClaudeFormatModel(model string) bool {
	_, ok := claudeFormatModels[model]
	return ok
}

// isTextOnlyModel reports whether the given model id does not support vision
// inputs. Exported for tests and external callers.
func isTextOnlyModel(model string) bool {
	_, ok := textOnlyModels[model]
	return ok
}
