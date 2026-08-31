package rtk

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// CavemanIntensity is the compression intensity level. Rules carry a
// MinIntensity gate; a rule only fires when the configured intensity is at
// least that level. Rank order: lite(0) < full(1) < ultra(2).
type CavemanIntensity string

const (
	CavemanLite  CavemanIntensity = "lite"
	CavemanFull  CavemanIntensity = "full"
	CavemanUltra CavemanIntensity = "ultra"
)

// CavemanRuleContext restricts which message role a rule applies to. "all"
// applies to every role; the others gate on the exact message role.
type CavemanRuleContext string

const (
	CavemanContextAll       CavemanRuleContext = "all"
	CavemanContextUser      CavemanRuleContext = "user"
	CavemanContextSystem    CavemanRuleContext = "system"
	CavemanContextAssistant CavemanRuleContext = "assistant"
)

// CavemanRule describes a single prose-compression rule. It mirrors the
// OmniRoute CavemanRule shape: a regex pattern (RE2, compiled at load), a
// static replacement or a replacement map keyed on the normalized matched
// phrase, a role context, a category and an intensity gate.
type CavemanRule struct {
	Name           string
	Pattern        string            // RE2 regex source
	Replacement    string            // static replacement (may reference $1..$9)
	ReplacementMap map[string]string // match -> replacement (keys normalized)
	After          string            // optional positive-lookahead emulation: only fire when the rune following the match (or end-of-string) matches this RE2 regex
	Context        CavemanRuleContext
	Category       string
	Description    string
	MinIntensity   CavemanIntensity
}

// cavemanRule is the compiled form of a CavemanRule. replaceFn returns the
// replacement for a full match; guardSuffix emulates the negative lookbehind
// used in OmniRoute's pleasantries rule (RE2 has no lookbehind, so we skip a
// match when the text immediately before it ends with any guard suffix);
// afterRe emulates a positive lookahead (the rule only fires when the rune
// following the match — or end-of-string — matches afterRe).
type cavemanRule struct {
	name         string
	re           *regexp.Regexp
	replaceFn    func(string) string
	staticRepl   string // used when staticRepl != "" (no map, no guard)
	guardSuffix  []string
	afterRe      *regexp.Regexp
	context      CavemanRuleContext
	category     string
	minIntensity CavemanIntensity
}

// intensityRank maps an intensity to its rank. Unknown values default to lite.
func intensityRank(i CavemanIntensity) int {
	switch i {
	case CavemanLite:
		return 0
	case CavemanFull:
		return 1
	case CavemanUltra:
		return 2
	default:
		return 0
	}
}

// normalizeMatchKey normalizes a matched phrase for ReplacementMap lookups:
// trimmed, whitespace-collapsed, lowercased. Matches OmniRoute's
// normalizeReplacementKey.
func normalizeMatchKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// compileCavemanRule compiles a CavemanRule into its executable form.
// The ReplacementMap path builds a lookup closure; the static Replacement path
// is used with ReplaceAllString (so $n group references work). Guard suffixes
// only apply to static-replacement rules (OmniRoute uses them only there).
func compileCavemanRule(r CavemanRule) *cavemanRule {
	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return nil
	}
	c := &cavemanRule{
		name:         r.Name,
		re:           re,
		staticRepl:   r.Replacement,
		context:      r.Context,
		category:     r.Category,
		minIntensity: r.MinIntensity,
	}
	if c.minIntensity == "" {
		c.minIntensity = CavemanLite
	}
	if r.After != "" {
		if afterRe, err := regexp.Compile(r.After); err == nil {
			c.afterRe = afterRe
		}
	}
	if len(r.ReplacementMap) > 0 {
		m := make(map[string]string, len(r.ReplacementMap))
		for k, v := range r.ReplacementMap {
			m[normalizeMatchKey(k)] = v
		}
		c.staticRepl = ""
		c.replaceFn = func(match string) string {
			if v, ok := m[normalizeMatchKey(match)]; ok {
				return v
			}
			return match
		}
	}
	return c
}

// appliesTo reports whether a compiled rule may run for the given role context
// and intensity (both gates must pass).
func (r *cavemanRule) appliesTo(ctx CavemanRuleContext, intensity CavemanIntensity) bool {
	if r == nil {
		return false
	}
	if r.context != CavemanContextAll && r.context != ctx {
		return false
	}
	return intensityRank(r.minIntensity) <= intensityRank(intensity)
}

// shouldAttemptRule is a cheap keyword pre-filter: it returns false when the
// rule is known to need a trigger phrase that is absent from the text, letting
// the hot path skip regex scanning entirely. Mirrors OmniRoute's RULE_KEYWORDS.
func shouldAttemptRule(ruleName, lowerText string) bool {
	keywords, ok := cavemanRuleKeywords[ruleName]
	if !ok {
		return true
	}
	for _, kw := range keywords {
		if strings.Contains(lowerText, kw) {
			return true
		}
	}
	return false
}

// applyRuleToText applies a single compiled rule to the text. For unguarded
// rules it delegates to ReplaceAllString / ReplaceAllStringFunc. Guarded rules
// (guardSuffix or afterRe) run a manual pass that skips non-matching contexts.
func applyRuleToText(text string, r *cavemanRule) string {
	if r == nil || r.re == nil {
		return text
	}
	if len(r.guardSuffix) == 0 && r.afterRe == nil {
		if r.staticRepl != "" {
			return r.re.ReplaceAllString(text, r.staticRepl)
		}
		if r.replaceFn != nil {
			return r.re.ReplaceAllStringFunc(text, r.replaceFn)
		}
		// Static empty replacement.
		return r.re.ReplaceAllString(text, "")
	}
	return applyScan(text, r)
}

// applyScan runs a manual replacement pass for rules with a guard suffix
// (negative lookbehind emulation: a match is skipped when text[:start] ends
// with any guard suffix) and/or an afterRe (positive lookahead emulation: a
// match is skipped unless the following rune — or end-of-string — matches).
func applyScan(text string, r *cavemanRule) string {
	idxs := r.re.FindAllStringIndex(text, -1)
	if len(idxs) == 0 {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for _, loc := range idxs {
		start, end := loc[0], loc[1]
		skip := false
		for _, s := range r.guardSuffix {
			if start >= len(s) && strings.HasSuffix(text[:start], s) {
				skip = true
				break
			}
		}
		if !skip && r.afterRe != nil {
			// Positive lookahead: require the rune after the match (or
			// end-of-string) to match afterRe. FindAllStringIndex returns
			// rune-aligned boundaries, so the next rune starts exactly at
			// `end` when end < len(text); end == len(text) means
			// end-of-string, which always satisfies the lookahead.
			if end < len(text) {
				_, w := utf8.DecodeRuneInString(text[end:])
				if !r.afterRe.MatchString(text[end : end+w]) {
					skip = true
				}
			}
		}
		b.WriteString(text[last:start])
		if !skip {
			match := text[start:end]
			if r.replaceFn != nil {
				b.WriteString(r.replaceFn(match))
			} else {
				b.WriteString(r.staticRepl)
			}
		} else {
			b.WriteString(text[start:end])
		}
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

// isUTF8Boundary reports whether b is a valid UTF-8 rune start byte (i.e. not
// a continuation byte 0b10xxxxxx).
func isUTF8Boundary(b byte) bool {
	return b&0xC0 != 0x80
}

// applyRulesToText applies a slice of compiled rules to the text in order and
// returns the result plus the names of the rules that actually changed it.
func applyRulesToText(text string, rules []*cavemanRule) (string, []string) {
	result := text
	lower := strings.ToLower(text)
	var applied []string
	for _, rule := range rules {
		if rule == nil || !shouldAttemptRule(rule.name, lower) {
			continue
		}
		before := result
		result = applyRuleToText(result, rule)
		if result != before {
			applied = append(applied, rule.name)
		}
	}
	return result, applied
}

// getRulesForContext returns the compiled rules that apply to a message role
// at a given intensity for a language ("en" or "zh"; anything else falls back
// to the English pack). Mirrors OmniRoute's getRulesForContext.
func getRulesForContext(ctx CavemanRuleContext, intensity CavemanIntensity, language string) []*cavemanRule {
	var src []*cavemanRule
	if language == "zh" {
		src = cavemanRulesZh
	} else {
		src = cavemanRulesEn
	}
	out := make([]*cavemanRule, 0, len(src))
	for _, r := range src {
		if r.appliesTo(ctx, intensity) {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		// Fallback cascade: re-filter the English pack (mirrors OmniRoute's
		// fallback for missing language packs).
		for _, r := range cavemanRulesEn {
			if r.appliesTo(ctx, intensity) {
				out = append(out, r)
			}
		}
	}
	return out
}

// cavemanRuleKeywords mirrors OmniRoute's RULE_KEYWORDS fast-path map.
var cavemanRuleKeywords = map[string][]string{
	"redundant_phrasing":    {"make sure", "be sure", "due to the fact", "the reason is because", "it is important", "you should", "remember to"},
	"pleasantries":          {"sure", "certainly", "of course", "happy to", "thanks", "thank you", "glad to", "no problem", "you're welcome", "absolutely"},
	"polite_framing":        {"please", "kindly", "could you please", "would you please", "can you please", "i would like you", "i want you", "i need you"},
	"hedging":               {"it seems like", "it appears that", "i think that", "i believe that", "probably", "possibly", "maybe it"},
	"verbose_instructions":  {"provide a detailed", "give me a comprehensive", "write an in-depth", "create a thorough", "explain in detail"},
	"filler_adverbs":        {"basically", "essentially", "actually", "literally", "simply", "currently"},
	"articles":              {" a ", " an ", " the ", "a ", "an ", "the "},
	"filler_phrases":        {"i want to", "i need to", "i'd like to", "i'm looking for"},
	"redundant_openers":     {"hi there", "hello", "good morning", "hey"},
	"verbose_requests":      {"i was wondering", "would it be possible"},
	"leader_phrases":        {"i'll", "i will", "i can", "i'd", "let me", "you can", "we will", "we can", "let's"},
	"self_reference":        {"i am trying to", "i am working on", "i have been"},
	"excessive_gratitude":   {"thank you so much", "thanks in advance", "i really appreciate"},
	"qualifier_removal":     {"a bit", "a little", "somewhat", "kind of", "sort of"},
	"softeners":             {"if possible", "when you get a chance", "at your convenience", "just wondering"},
	"uncertainty_fillers":   {"i guess", "i suppose", "more or less", "in a way"},
	"assistant_fillers":     {"here's", "below is", "this is"},
	"compound_collapse":     {"and any potential"},
	"explanatory_prefix":    {"the function appears to be handling", "the code seems to", "the class is", "this module is"},
	"question_to_directive": {"can you explain why", "could you show me how", "would you tell me", "can you tell me"},
	"context_setup":         {"i have the following code", "here is my code", "below is the code"},
	"intent_clarification":  {"what i'm trying to do", "my objective is to", "what i need is", "i'm aiming to"},
	"background_removal":    {"as you may know", "as we discussed earlier"},
	"meta_commentary":       {"note that", "keep in mind", "remember that"},
	"purpose_statement":     {"for the purpose of", "with the goal of", "in an effort to", "for every"},
	"list_conjunction":      {"and also", "as well as"},
	"purpose_phrases":       {"in order to", "so as to"},
	"redundant_quantifiers": {"each and every", "any and all"},
	"all_quantifier":        {"any and all"},
	"verbose_connectors":    {"furthermore", "additionally", "moreover", "in addition"},
	"transition_removal":    {"on the other hand", "in contrast", "however"},
	"emphasis_removal":      {"very", "really", "extremely", "highly", "quite"},
	"passive_voice":         {"is being used", "is being called", "is being generated", "was created", "was generated", "was implemented"},
	"redundant_because":     {"due to the fact", "the reason is because"},
	"redundant_directive":   {"it is important", "you should", "remember to"},
	"repeated_context":      {"as we discussed earlier", "as mentioned before", "as previously stated", "as i said before"},
	"repeated_question":     {"same question as before", "i asked this earlier", "this is the same question"},
	"reestablished_context": {"going back to the code above", "referring back to", "returning to"},
	"summary_replacement":   {"to summarize what we've discussed", "in summary of our conversation", "to recap"},
	"ultra_abbreviations":   {"database", "configuration", "function", "request", "response", "implementation", "authentication", "authorization", "application", "dependency", "dependencies"},
}

// cavemanRulesEn are the built-in English rules: the hardcoded CAVEMAN_RULES
// set plus the en language-pack extras (softeners, uncertainty_fillers,
// assistant_fillers, redundant_because, redundant_directive, all_quantifier,
// per-word ultra abbreviations). Patterns are RE2-safe (lookarounds rewritten).
var cavemanRulesEn = func() []*cavemanRule {
	raw := []CavemanRule{
		// ── Category 1: Filler Removal ──────────────────────────────
		{Name: "redundant_phrasing", Pattern: `(?i)\b(?:make sure to|be sure to|due to the fact that|the reason is because|it is important to|you should|remember to)\b\s*`, ReplacementMap: map[string]string{"make sure to": "ensure ", "be sure to": "ensure ", "due to the fact that": "because ", "the reason is because": "because ", "it is important to": "", "you should": "", "remember to": ""}, Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},
		{Name: "pleasantries", Pattern: `(?i)\b(?:i'?d be happy to|i would be happy to|i'?d be glad to|i would be glad to|glad to help|happy to|thank you|thanks|no problem|you'?re welcome|absolutely|certainly|of course|sure)\b[,.!?\s]*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "polite_framing", Pattern: `(?i)\b(?:please|kindly|could you please|would you please|can you please|I would like you to|I want you to|I need you to)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "hedging", Pattern: `(?i)\b(?:it seems like|it appears that|I think that|I believe that|probably|possibly|maybe it)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "verbose_instructions", Pattern: `(?i)\b(?:provide a detailed explanation of|give me a comprehensive explanation of|write an in-depth explanation of|create a thorough explanation of|provide a detailed|give me a comprehensive|write an in-depth|create a thorough|explain in detail)\b`, ReplacementMap: map[string]string{"provide a detailed explanation of": "explain ", "give me a comprehensive explanation of": "explain ", "write an in-depth explanation of": "explain ", "create a thorough explanation of": "explain ", "provide a detailed": "provide ", "give me a comprehensive": "give ", "write an in-depth": "write ", "create a thorough": "create ", "explain in detail": "explain "}, Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "filler_adverbs", Pattern: `(?i)\b(?:basically|essentially|actually|literally|simply|currently)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "articles", Pattern: `\b(?:[Aa]n|[Aa]|[Tt]he)\s+([a-z])`, Replacement: "$1", Context: CavemanContextAll, Category: "terse", MinIntensity: CavemanFull},
		{Name: "filler_phrases", Pattern: `(?i)^(?:I want to|I need to|I'd like to|I'm looking for)\b\s*`, Replacement: "", Context: CavemanContextUser, Category: "filler", MinIntensity: CavemanLite},
		{Name: "redundant_openers", Pattern: `(?i)^(?:Hi there|Hello|Good morning|Hey)\s*[,.!?\s]?\s*`, Replacement: "", Context: CavemanContextUser, Category: "filler", MinIntensity: CavemanLite},
		{Name: "verbose_requests", Pattern: `(?i)\b(?:I was wondering if you could|Would it be possible to)\b\s*`, Replacement: "", Context: CavemanContextUser, Category: "filler", MinIntensity: CavemanLite},
		{Name: "leader_phrases", Pattern: `(?i)^(?:i'?ll|i will|i can|i'?d|let me|you can|we will|we can|let'?s)\s+([a-z])`, Replacement: "$1", Context: CavemanContextAll, Category: "terse", MinIntensity: CavemanFull},
		{Name: "self_reference", Pattern: `(?i)^(?:I am trying to|I am working on|I have been)\b\s*`, Replacement: "", Context: CavemanContextUser, Category: "filler", MinIntensity: CavemanLite},
		{Name: "excessive_gratitude", Pattern: `(?i)\b(?:Thank you so much|Thanks in advance|I really appreciate)\b[,.!?\s]*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "qualifier_removal", Pattern: `(?i)\b(?:a bit|a little|somewhat|kind of|sort of)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "softeners", Pattern: `(?i)\b(?:if possible|when you get a chance|at your convenience|just wondering)\b[,.!?\s]*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "uncertainty_fillers", Pattern: `(?i)\b(?:I guess|I suppose|more or less|in a way)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "assistant_fillers", Pattern: `(?i)^(?:Here'?s|Below is|This is)\s+(?:a|an|the)?\s*`, Replacement: "", Context: CavemanContextAssistant, Category: "filler", MinIntensity: CavemanLite},

		// ── Category 2: Context Condensation ────────────────────────
		{Name: "compound_collapse", Pattern: `(?i)\band any potential\b`, Replacement: "", Context: CavemanContextAll, Category: "context", MinIntensity: CavemanFull},
		{Name: "explanatory_prefix", Pattern: `(?i)\b(?:The function appears to be handling|The code seems to|The class is|This module is)\b`, ReplacementMap: map[string]string{"the function appears to be handling": "Function:", "the code seems to": "Code:", "the class is": "Class:", "this module is": "Module:"}, Context: CavemanContextAll, Category: "context", MinIntensity: CavemanLite},
		{Name: "question_to_directive", Pattern: `(?i)\b(?:Can you explain why|Could you show me how|Would you tell me|Can you tell me)\b\s*`, ReplacementMap: map[string]string{"can you explain why": "Explain why ", "could you show me how": "Show how ", "would you tell me": "Tell me ", "can you tell me": "Tell me "}, Context: CavemanContextUser, Category: "context", MinIntensity: CavemanLite},
		{Name: "context_setup", Pattern: `(?i)\b(?:I have the following code|Here is my code|Below is the code)\b\s*[:.]?\s*`, Replacement: "Code:", Context: CavemanContextUser, Category: "context", MinIntensity: CavemanLite},
		{Name: "intent_clarification", Pattern: `(?i)\b(?:What I'm trying to do is|My objective is to|What I need is|I'm aiming to)\b\s*`, Replacement: "Goal:", Context: CavemanContextUser, Category: "context", MinIntensity: CavemanLite},
		{Name: "background_removal", Pattern: `(?i)\b(?:As you may know,?\s*|As we discussed earlier,?\s*)`, Replacement: "", Context: CavemanContextAll, Category: "context", MinIntensity: CavemanLite},
		{Name: "meta_commentary", Pattern: `(?i)^(?:Note that|Keep in mind that|Remember that)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "context", MinIntensity: CavemanLite},
		{Name: "purpose_statement", Pattern: `(?i)\b(?:for the purpose of|with the goal of|in an effort to|for every)\b`, ReplacementMap: map[string]string{"for the purpose of": "for", "with the goal of": "to", "in an effort to": "to", "for every": "per"}, Context: CavemanContextAll, Category: "context", MinIntensity: CavemanLite},

		// ── Category 3: Structural Compression ──────────────────────
		{Name: "list_conjunction", Pattern: `(?i),\s*and also\s+|,\s*as well as\s+`, Replacement: ", ", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},
		{Name: "purpose_phrases", Pattern: `(?i)\b(?:in order to|so as to)\b\s*`, Replacement: "to ", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanLite},
		{Name: "redundant_quantifiers", Pattern: `(?i)\b(?:each and every single|each and every|any and all)\b`, ReplacementMap: map[string]string{"each and every single": "each", "each and every": "each", "any and all": "all"}, Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},
		{Name: "all_quantifier", Pattern: `(?i)\bany and all\b`, Replacement: "all", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},
		{Name: "verbose_connectors", Pattern: `(?i)\b(?:furthermore|additionally|moreover|in addition)\b\s*`, Replacement: "also ", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanLite},
		{Name: "transition_removal", Pattern: `(?i)^(?:On the other hand,?\s*|In contrast,?\s*|However,?\s*)`, Replacement: "", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanLite},
		{Name: "emphasis_removal", Pattern: `(?i)\b(?:very|really|extremely|highly|quite)\s+([a-z])`, Replacement: "$1", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanLite},
		{Name: "passive_voice", Pattern: `(?i)\b(?:is being used|is being called|is being generated|was created|was generated|was implemented)\b`, ReplacementMap: map[string]string{"is being used": "uses", "is being called": "calls", "is being generated": "generated", "was created": "created", "was generated": "generated", "was implemented": "implemented"}, Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},
		{Name: "redundant_because", Pattern: `(?i)\b(?:due to the fact that|the reason is because)\b\s*`, Replacement: "because ", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},
		{Name: "redundant_directive", Pattern: `(?i)\b(?:it is important to|you should|remember to)\b\s*`, Replacement: "", Context: CavemanContextAll, Category: "structural", MinIntensity: CavemanFull},

		// ── Category 4: Multi-Turn Dedup ────────────────────────────
		{Name: "repeated_context", Pattern: `(?i)\b(?:As we discussed earlier|As mentioned before|As previously stated|As I said before)\b[,.]?\s*`, Replacement: "See above. ", Context: CavemanContextAll, Category: "dedup", MinIntensity: CavemanLite},
		{Name: "repeated_question", Pattern: `(?i)\b(?:Same question as before|I asked this earlier|This is the same question)\b[,.]?\s*`, Replacement: "[same question] ", Context: CavemanContextUser, Category: "dedup", MinIntensity: CavemanLite},
		{Name: "reestablished_context", Pattern: `(?i)\b(?:Going back to the code above|Referring back to|Returning to)\b\s*`, Replacement: "Re: ", Context: CavemanContextAll, Category: "dedup", MinIntensity: CavemanLite},
		{Name: "summary_replacement", Pattern: `(?i)\b(?:To summarize what we've discussed|In summary of our conversation|To recap)\b[,.]?\s*`, Replacement: "Summary: ", Context: CavemanContextAssistant, Category: "dedup", MinIntensity: CavemanLite},

		// ── Category 5: Ultra Abbreviations ─────────────────────────
		{Name: "ultra_abbreviations", Pattern: `(?i)\b(?:database|configuration|function|request|response|implementation|authentication|authorization|application|dependency|dependencies)\b`, ReplacementMap: map[string]string{"database": "DB", "configuration": "config", "function": "fn", "request": "req", "response": "res", "implementation": "impl", "authentication": "auth", "authorization": "authz", "application": "app", "dependency": "dep", "dependencies": "deps"}, Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanUltra},
	}

	out := make([]*cavemanRule, 0, len(raw))
	for _, r := range raw {
		if c := compileCavemanRule(r); c != nil {
			out = append(out, c)
		}
	}
	return out
}()

// cavemanRulesZh are the built-in Simplified-Chinese rules, ported from
// OmniRoute's rules/zh/{filler,dedup,ultra}.json. Chinese prose compresses via
// filler removal, dedup markers and ultra abbreviations; the articles and
// structural rules are English-specific and intentionally absent.
var cavemanRulesZh = func() []*cavemanRule {
	raw := []CavemanRule{
		// filler
		{Name: "zh_filler_please", Pattern: `请(?:你|您)?(?:帮我|帮忙)?`, Replacement: "", Context: CavemanContextUser, Category: "filler", MinIntensity: CavemanLite},
		{Name: "zh_filler_thanks", Pattern: `(?:谢谢|多谢|感谢)(?:你|您|大家)?[。，！]?`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "zh_filler_trouble", Pattern: `(?:麻烦你|麻烦您|劳驾)[，]?`, Replacement: "", Context: CavemanContextUser, Category: "filler", MinIntensity: CavemanLite},
		{Name: "zh_filler_greeting", Pattern: `(?:你好|您好|大家好)[，,]?`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanLite},
		{Name: "zh_filler_hedge_think", Pattern: `(?:我觉得|我认为|我想说)`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanFull},
		{Name: "zh_filler_hedge_actually", Pattern: `(?:其实|说实话|基本上)[，,]?`, Replacement: "", Context: CavemanContextAll, Category: "filler", MinIntensity: CavemanFull},
		// dedup
		{Name: "zh_repeated_context", Pattern: `(?:如前所述|如之前所说|如先前提到|正如我之前所说)`, Replacement: "见上。", Context: CavemanContextAll, Category: "dedup", MinIntensity: CavemanLite},
		{Name: "zh_repeated_question", Pattern: `(?:和之前一样的问题|我之前问过这个问题|这是同一个问题)`, Replacement: "［同问］", Context: CavemanContextUser, Category: "dedup", MinIntensity: CavemanLite},
		{Name: "zh_reestablished_context", Pattern: `(?:回到上面的代码|关于之前提到的内容|回到之前的话题)`, Replacement: "Re: ", Context: CavemanContextAll, Category: "dedup", MinIntensity: CavemanLite},
		{Name: "zh_summary_replacement", Pattern: `(?:总结一下我们讨论的内容|总结我们的对话|概括来说)`, Replacement: "总结：", Context: CavemanContextAssistant, Category: "dedup", MinIntensity: CavemanLite},
		// ultra
		{Name: "zh_ultra_modal_particles", Pattern: `(?:吗|呢|吧|啊|呀|嘛)`, After: `[。，！？、\s]`, Replacement: "", Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanFull},
		{Name: "zh_ultra_database_abbreviation", Pattern: `数据库`, Replacement: "DB", Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanUltra},
		{Name: "zh_ultra_application_abbreviation", Pattern: `应用程序`, Replacement: "app", Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanUltra},
		{Name: "zh_ultra_dependency_abbreviation", Pattern: `依赖关系`, Replacement: "dep", Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanUltra},
		{Name: "zh_ultra_config_file_abbreviation", Pattern: `配置文件`, Replacement: "cfg", Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanUltra},
		{Name: "zh_ultra_function_abbreviation", Pattern: `函数`, Replacement: "fn", Context: CavemanContextAll, Category: "ultra", MinIntensity: CavemanUltra},
	}

	out := make([]*cavemanRule, 0, len(raw))
	for _, r := range raw {
		if c := compileCavemanRule(r); c != nil {
			out = append(out, c)
		}
	}
	return out
}()
