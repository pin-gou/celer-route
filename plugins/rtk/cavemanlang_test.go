package rtk

import "testing"

// TestDetectCavemanLanguage verifies the en/zh detector.
func TestDetectCavemanLanguage(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"the quick brown fox jumps over the lazy dog", "en"},
		{"hello world", "en"},
		{"请帮我看看这个配置文件怎么改", "zh"},
		{"数据库连接失败了", "zh"},
		{"こんにちは世界", "en"}, // kana → not zh, falls back to en
		{"", "en"},
		{"Mixed English 中文混合", "zh"},
	}
	for _, tc := range cases {
		if got := detectCavemanLanguage(tc.text); got != tc.want {
			t.Errorf("detectCavemanLanguage(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestNormalizeCavemanLanguage verifies language value normalization.
func TestNormalizeCavemanLanguage(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "auto"},
		{"auto", "auto"},
		{"en", "en"},
		{"zh", "zh"},
		{"zh-CN", "zh"},
		{"zh-Hans", "zh"},
		{"chinese", "zh"},
		{"fr", "en"},
	}
	for _, tc := range cases {
		if got := normalizeCavemanLanguage(tc.in); got != tc.want {
			t.Errorf("normalizeCavemanLanguage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestCavemanLanguageSupported verifies pack support.
func TestCavemanLanguageSupported(t *testing.T) {
	if !cavemanLanguageSupported("en") || !cavemanLanguageSupported("zh") {
		t.Errorf("en/zh should be supported")
	}
	if cavemanLanguageSupported("fr") {
		t.Errorf("fr should not be supported")
	}
}

// TestIsCodeLikeLine verifies code-line detection.
func TestIsCodeLikeLine(t *testing.T) {
	codeLines := []string{"import os", "function main() {", "const x = 1", "  const y = 2", "return value", "if (x) {"}
	for _, l := range codeLines {
		if !isCodeLikeLine(l) {
			t.Errorf("isCodeLikeLine(%q) = false, want true", l)
		}
	}
	proseLines := []string{"please check this", "the quick brown fox", "can you help?"}
	for _, l := range proseLines {
		if isCodeLikeLine(l) {
			t.Errorf("isCodeLikeLine(%q) = true, want false", l)
		}
	}
}
