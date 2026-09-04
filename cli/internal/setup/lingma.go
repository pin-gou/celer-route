package setup

import (
	"fmt"
	"strings"
)

// RenderLingma produces the in-app steps for pointing 通义灵码 (Tongyi
// Lingma, Alibaba) at celer-route's OpenAI-compatible endpoint. Output is
// byte-identical to the Web UI's Lingma template.
func RenderLingma(in Input) (Output, error) {
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
		"安装通义灵码插件（VS Code / JetBrains 扩展市场搜索「通义灵码」）",
		"打开设置 → 模型服务 → 自定义端点",
		"协议选择「OpenAI 兼容（Chat Completions）」",
		fmt.Sprintf("Base URL 填 %s（含 /v1）", baseURL),
		apiLine,
		fmt.Sprintf("模型（Model）填 %s（模型目录里的完整 ID）", defaultID),
		"保存并重载窗口后生效",
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
			{Path: "Lingma in-app steps", Content: b.String() + "\n"},
		},
		Steps:        steps,
		DefaultModel: defaultID,
		Agent:        Lingma,
	}, nil
}
