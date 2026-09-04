package setup

import (
	"fmt"
	"strings"
)

// RenderZCode produces the in-app steps for connecting a custom OpenAI-
// compatible endpoint in ZCode (Zhipu). Output is byte-identical to the
// Web UI's ZCode template.
func RenderZCode(in Input) (Output, error) {
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
		"打开 ZCode → 设置 → 模型接入",
		"选择「自定义 OpenAI 兼容接口」",
		fmt.Sprintf("Base URL 填 %s（含 /v1）", baseURL),
		apiLine,
		fmt.Sprintf("Model ID 填 %s（模型目录里的完整 ID）", defaultID),
		"保存后即可在模型列表中切换到该模型",
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
			{Path: "ZCode in-app steps", Content: b.String() + "\n"},
		},
		Steps:        steps,
		DefaultModel: defaultID,
		Agent:        ZCode,
	}, nil
}
