package setup

import (
	"fmt"
	"strings"
)

// RenderCursor produces the in-app step list for Cursor / Windsurf.
// Cursor stores its config in an opaque SQLite DB; we can't write a
// file directly. The Output.Steps is what the CLI prints to stdout.
func RenderCursor(in Input) (Output, error) {
	if len(in.Models) == 0 {
		return Output{}, ErrNoModels
	}
	baseURL := OpenAISurface(in.BaseURL)
	defaultID := pickDefaultModel(in.Models, in.DefaultModelID)

	settingsShortcut := "Ctrl+,"
	if platformOrDefault(in) == PlatformMacOS {
		settingsShortcut = "⌘,"
	}

	apiLine := fmt.Sprintf("API Key 填 %s", in.APIKey)
	if in.APIKey == "" {
		apiLine = "API Key 留空（celer-route 未开启强制鉴权）"
	}

	steps := []string{
		fmt.Sprintf("打开 Cursor → Settings（%s）→ Models", settingsShortcut),
		"在 \"Model Provider\" 下点击 \"+ Add\" → 选择 \"OpenAI\"",
		fmt.Sprintf("Name 填 %s", ProviderKey),
		fmt.Sprintf("Base URL 填 %s", baseURL),
		apiLine,
		fmt.Sprintf("点击 \"Verify\"，选择默认模型：%s", defaultID),
		"确认后可在模型下拉中切换到任意已配置模型",
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
			{Path: "Cursor/Windsurf in-app steps", Content: b.String() + "\n"},
		},
		Steps:        steps,
		DefaultModel: defaultID,
		Agent:        Cursor,
	}, nil
}
