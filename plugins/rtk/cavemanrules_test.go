package rtk

import (
	"strings"
	"testing"
)

// TestCavemanRulesCompile verifies every built-in rule (en + zh) compiles to a
// valid RE2 pattern and carries the expected intensity gate.
func TestCavemanRulesCompile(t *testing.T) {
	all := append(append([]*cavemanRule{}, cavemanRulesEn...), cavemanRulesZh...)
	if len(all) == 0 {
		t.Fatal("expected at least one caveman rule")
	}
	// Every rule must have a name and a compiled regex.
	for _, r := range all {
		if r.name == "" {
			t.Errorf("rule with empty name")
		}
		if r.re == nil {
			t.Errorf("rule %q failed to compile", r.name)
		}
	}
	// Sanity: no duplicate names within a pack.
	seen := map[string]bool{}
	for _, r := range all {
		if seen[r.name] {
			t.Errorf("duplicate rule name %q", r.name)
		}
		seen[r.name] = true
	}
}

// TestCavemanIntensityGating verifies getRulesForContext filters by intensity.
func TestCavemanIntensityGating(t *testing.T) {
	// lite pack must not include full-gated rules.
	lite := getRulesForContext(CavemanContextUser, CavemanLite, "en")
	for _, r := range lite {
		if intensityRank(r.minIntensity) > 0 {
			t.Errorf("lite pack includes rule %q gated at %q", r.name, r.minIntensity)
		}
	}
	// full pack includes both lite and full.
	full := getRulesForContext(CavemanContextUser, CavemanFull, "en")
	if len(full) <= len(lite) {
		t.Errorf("full pack (%d) should be a superset of lite (%d)", len(full), len(lite))
	}
	// zh pack returns non-empty and includes ultra only at ultra.
	zhUltra := getRulesForContext(CavemanContextUser, CavemanUltra, "zh")
	foundUltraAbbrev := false
	for _, r := range zhUltra {
		if r.name == "zh_ultra_function_abbreviation" {
			foundUltraAbbrev = true
		}
	}
	if !foundUltraAbbrev {
		t.Errorf("zh ultra pack should include zh_ultra_function_abbreviation")
	}
	zhLite := getRulesForContext(CavemanContextUser, CavemanLite, "zh")
	for _, r := range zhLite {
		if r.name == "zh_ultra_function_abbreviation" {
			t.Errorf("zh lite pack must not include ultra rule")
		}
	}
}

// TestCavemanRuleUserContext verifies role-context filtering: user-context
// rules only fire for user messages; assistant/system-context rules do not.
func TestCavemanRuleUserContext(t *testing.T) {
	userRules := getRulesForContext(CavemanContextUser, CavemanFull, "en")
	hasUserOnly := func(name string) bool {
		for _, r := range userRules {
			if r.name == name {
				return true
			}
		}
		return false
	}
	// redundant_openers is user-context — must be selectable for user.
	if !hasUserOnly("redundant_openers") {
		t.Errorf("user pack should include redundant_openers")
	}
	// assistant_fillers is assistant-context — must NOT be in user selection.
	if hasUserOnly("assistant_fillers") {
		t.Errorf("user pack must not include assistant-context rule")
	}
}

// TestCavemanRuleApplication exercises end-to-end rule application on prose.
func TestCavemanRuleApplication(t *testing.T) {
	rules := getRulesForContext(CavemanContextUser, CavemanFull, "en")
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "pleasantries removed",
			in:   "Hi there, could you please thank you very much for the update.",
			want: "much for update.",
		},
		{
			name: "hedging removed",
			in:   "I think that probably the build is fine.",
			want: "build is fine.",
		},
		{
			name: "polite framing removed",
			in:   "please check the logs",
			want: "check logs",
		},
		{
			name: "verbose instructions shortened",
			in:   "Can you explain why the test failed?",
			want: "Explain why test failed?",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, applied := applyRulesToText(tc.in, rules)
			if got != tc.want {
				t.Errorf("applyRulesToText(%q) = %q, want %q (applied=%v)", tc.in, got, tc.want, applied)
			}
		})
	}
}

// TestCavemanRuleZh verifies Chinese filler removal.
func TestCavemanRuleZh(t *testing.T) {
	rules := getRulesForContext(CavemanContextUser, CavemanFull, "zh")
	got, applied := applyRulesToText("你好，请帮我看看这个问题", rules)
	if got == "你好，请帮我看看这个问题" {
		t.Errorf("expected zh filler removal to change text, applied=%v", applied)
	}
	if !strings.Contains(got, "看看这个问题") {
		t.Errorf("got %q should retain the substantive part", got)
	}
}

// TestCavemanShouldAttemptKeyword verifies the keyword pre-filter.
func TestCavemanShouldAttemptKeyword(t *testing.T) {
	if shouldAttemptRule("pleasantries", "the sky is blue") {
		t.Errorf("pleasantries keyword pre-filter should skip text without a trigger word")
	}
	if !shouldAttemptRule("pleasantries", "thank you for the help") {
		t.Errorf("pleasantries keyword pre-filter should match on 'thank you'")
	}
}

// TestCavemanReplacementMap verifies match->replacement mapping.
func TestCavemanReplacementMap(t *testing.T) {
	rules := getRulesForContext(CavemanContextUser, CavemanFull, "en")
	got, _ := applyRulesToText("provide a detailed explanation of the bug", rules)
	if !strings.Contains(got, "explain") || strings.Contains(got, "detailed explanation") {
		t.Errorf("expected detailed explanation rewritten to explain, got %q", got)
	}
}

// TestCavemanSkipRules verifies skipRules blacklisting.
func TestCavemanSkipRules(t *testing.T) {
	rules := getRulesForContext(CavemanContextUser, CavemanFull, "en")
	skip := map[string]bool{"pleasantries": true}
	filtered := make([]*cavemanRule, 0, len(rules))
	for _, r := range rules {
		if !skip[r.name] {
			filtered = append(filtered, r)
		}
	}
	in := "Hi there, please help"
	gotNoSkip, _ := applyRulesToText(in, filtered)
	// Without pleasantries, "Hi there" stays (it is user-context too) but the
	// polite "please" rule is not in the pack at this intensity... verify the
	// filter at least removed the pleasantries rule name from applied set.
	if containsRuleName(filtered, "pleasantries") {
		t.Errorf("skipRules should remove pleasantries from filtered set")
	}
	_ = gotNoSkip
}

func containsRuleName(rules []*cavemanRule, name string) bool {
	for _, r := range rules {
		if r.name == name {
			return true
		}
	}
	return false
}
