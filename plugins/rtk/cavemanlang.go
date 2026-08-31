package rtk

import (
	"strings"
	"unicode"
)

// SupportedCavemanLanguages lists the languages with built-in rule packs.
// Only en/zh are compiled in (per the design decision); anything else falls
// back to the English pack at rule-selection time.
var SupportedCavemanLanguages = []string{"en", "zh"}

// isCJK reports whether r is a Han (CJK Unified Ideograph) character.
func isHan(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF
}

// isKana reports whether r is a Japanese kana character (hiragana or katakana).
func isKana(r rune) bool {
	return (r >= 0x3040 && r <= 0x309F) || (r >= 0x30A0 && r <= 0x30FF)
}

// detectCavemanLanguage determines the rule-pack language for a piece of text.
// Only en/zh are supported in this build; the detector returns "zh" when the
// text contains Han characters but no kana (kana would indicate Japanese,
// which we do not ship a pack for and therefore fall back to English), and
// "en" otherwise. Mirrors OmniRoute's detectCompressionLanguage CJK
// disambiguation step; the multi-lingual scorer is omitted as out of scope.
func detectCavemanLanguage(text string) string {
	hasHan := false
	for _, r := range text {
		if isHan(r) {
			hasHan = true
		} else if isKana(r) {
			// Japanese text present — do not misclassify as zh.
			return "en"
		}
	}
	if hasHan {
		return "zh"
	}
	return "en"
}

// normalizeCavemanLanguage normalises a configured language value ("auto" or
// empty → detect per message; "zh" → zh; anything else → en).
func normalizeCavemanLanguage(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "", "auto":
		return "auto"
	case "zh", "zh-cn", "zh-hans", "chinese":
		return "zh"
	default:
		return "en"
	}
}

// isWhitespace reports whether the string is all whitespace (used by the
// code-dominance heuristic).
func isWhitespace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
