package opencode

// maxToolsCount caps the number of tools in a single request. Some
// opencode-go backends reject requests with more than 128 tools. Mirrors the
// OpencodeExecutor.truncateTools() behavior in OmniRoute.
const maxToolsCount = 128

// deepSeekV4FlashFreeModel is the only model known to reject json_schema
// response_format with HTTP 400 while accepting json_object. The check is
// only applied to this exact id.
const deepSeekV4FlashFreeModel = "deepseek-v4-flash-free"

// effortTierModels defines which model ids support effort-tier suffixes
// (<model>-<low|medium|high|max|none>), and the allowed suffix set per model.
// Effort-tier aliases are rewritten to the canonical model id with
// reasoning_effort set on the request body, mirroring the OmniRoute
// parseEffortLevel() / opencode-go EFFORT_TIERS tables.
var effortTierModels = map[string][]string{
	"deepseek-v4-pro":   {"low", "medium", "high", "max"},
	"deepseek-v4-flash": {"high", "max"},
	"glm-5.2":           {"high", "max"},
	"mimo-v2.5":         {"high", "max"},
	"grok-4.5":          {"low", "medium", "high"},
	"hy3":               {"none", "low", "high"},
	"kimi-k3":           {"max"},
	"qwen3.6-plus":      {"high", "max"},
	"qwen3.7-max":       {"high", "max"},
	"qwen3.7-plus":      {"high", "max"},
}

// parseEffortLevel extracts an effort-tier suffix from a model id. Returns
// (baseModel, effort, true) when the suffix matches a known effort tier for
// its base model; otherwise (model, "", false).
//
// Mirrors the OmniRoute OpencodeExecutor.parseEffortLevel() function.
func parseEffortLevel(model string) (string, string, bool) {
	if model == "" {
		return model, "", false
	}
	for base, levels := range effortTierModels {
		for _, level := range levels {
			if model == base+"-"+level {
				return base, level, true
			}
		}
	}
	return model, "", false
}
