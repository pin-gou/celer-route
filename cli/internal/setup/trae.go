package setup

import (
	"fmt"
	"strings"
)

// RenderTrae produces the in-app steps for configuring a custom OpenAI-
// compatible model in Trae (ByteDance). Trae v3.3.51+ supports a full Base
// URL; the origin alone is not enough, so the steps call out the /v1 path.
// Output is byte-identical to the Web UI's Trae template.
func RenderTrae(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	apiLine := fmt.Sprintf("API Key 填 %s", in.APIKey)
	if in.APIKey == "" {
		apiLine = "API Key 留空（celer-route 未开启强制鉴权）"
	}

	steps := []string{
		"打开 Trae → 设置 → 模型 → 自定义模型",
		"添加模型，API 格式选择「OpenAI」（若你的服务只提供 Anthropic 协议再换）",
		fmt.Sprintf("Name 填 %s", ProviderKey),
		fmt.Sprintf("Base URL 填 %s（务必填完整路径，含 /v1）", baseURL),
		apiLine,
		fmt.Sprintf("Model ID 填 %s（模型目录里的完整 ID）", defaultID),
		"点击连接/校验后，即可在模型下拉中切换到该模型",
	}

	var b strings.Builder
	for i, s := range steps {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d. %s", i+1, s)
	}

	return Output{
		Files: []File{
			{Path: "Trae in-app steps", Content: b.String() + "\n"},
		},
		Steps:        steps,
		DefaultModel: defaultID,
		Agent:        Trae,
	}, nil
}
