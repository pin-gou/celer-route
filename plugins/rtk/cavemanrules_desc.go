// Package rtk — human-readable descriptions for the built-in Caveman rules.
//
// The catalog returned by GetCavemanRuleCatalog() reads these maps so the
// RTK admin UI can render a meaningful label and one-sentence summary
// next to each rule instead of an opaque identifier. Keys MUST match the
// Name: field of the corresponding entry in cavemanRulesEn / cavemanRulesZh;
// keep these tables in sync when adding or renaming a rule (the catalog
// test in cavemancatalog_test.go enforces non-empty descriptions).
package rtk

// cavemanRuleDescriptionsEn is the English one-liner for each built-in
// rule. Missing keys fall back to an empty string in the catalog (the rule
// still surfaces — operators just see its raw name).
var cavemanRuleDescriptionsEn = map[string]string{
	// Category: filler (lite)
	"redundant_phrasing":      "Collapse 'make sure to', 'be sure to', 'it is important to', 'you should', 'remember to', etc. into direct phrasing.",
	"pleasantries":            "Strip conversational openers and closers like 'thanks', 'happy to', 'certainly', 'glad to help'.",
	"polite_framing":          "Remove 'please', 'kindly', 'could you please' style politeness wrappers.",
	"hedging":                 "Cut hedging like 'I think that', 'probably', 'maybe it', 'it appears that'.",
	"verbose_instructions":    "Shorten 'provide a detailed explanation of' / 'write an in-depth' down to 'explain'.",
	"filler_adverbs":          "Strip adverbs that don't add meaning: basically, essentially, actually, literally, simply, currently.",
	"filler_phrases":          "Remove 'I want to' / 'I'd like to' style lead-ins on user messages.",
	"redundant_openers":       "Strip greetings at the start of user messages: 'Hi there', 'Hello', 'Hey'.",
	"verbose_requests":        "Cut 'I was wondering if you could' / 'Would it be possible to' hedges on user requests.",
	"self_reference":          "Remove 'I am trying to' / 'I am working on' / 'I have been' style self-reference lead-ins.",
	"excessive_gratitude":     "Strip multi-word thanks: 'Thank you so much', 'Thanks in advance', 'I really appreciate'.",
	"qualifier_removal":       "Remove soft quantifiers: a bit, a little, somewhat, kind of, sort of.",
	"softeners":               "Strip politeness softeners like 'when you get a chance', 'just wondering'.",
	"uncertainty_fillers":     "Cut 'I guess', 'I suppose', 'more or less', 'in a way'.",
	"assistant_fillers":       "Strip 'Here's' / 'Below is' / 'This is' lead-ins from assistant messages.",

	// Category: terse (full)
	"articles":         "Drop 'a', 'an', 'the' before words when it doesn't change meaning.",
	"leader_phrases":   "Compress 'I will' / 'let me' / 'let's' / 'you can' leading phrases to bare verbs.",

	// Category: context (lite / full)
	"compound_collapse":      "Drop 'and any potential' filler mid-sentence.",
	"explanatory_prefix":     "Rewrite 'The function appears to be handling' / 'The code seems to' to 'Function:' / 'Code:'.",
	"question_to_directive":  "Convert 'Can you explain why' style questions into direct imperatives.",
	"context_setup":          "Replace 'Here is my code' / 'Below is the code' with 'Code:'.",
	"intent_clarification":   "Replace 'What I'm trying to do is' / 'My objective is to' with 'Goal:'.",
	"background_removal":     "Strip 'As you may know' / 'As we discussed earlier' filler.",
	"meta_commentary":        "Drop 'Note that' / 'Keep in mind that' / 'Remember that' meta-instructions.",
	"purpose_statement":      "Shorten 'for the purpose of' / 'with the goal of' to 'for' / 'to'.",

	// Category: structural
	"list_conjunction":       "Collapse 'and also' / 'as well as' to a comma.",
	"purpose_phrases":        "Replace 'in order to' / 'so as to' with 'to'.",
	"redundant_quantifiers":  "Tighten 'each and every' / 'any and all' to 'each' / 'all'.",
	"all_quantifier":         "Tighten 'any and all' to 'all'.",
	"verbose_connectors":     "Shorten 'furthermore' / 'additionally' / 'moreover' to 'also'.",
	"transition_removal":     "Strip leading 'On the other hand' / 'In contrast' / 'However'.",
	"emphasis_removal":       "Remove emphasis adverbs: very, really, extremely, highly, quite.",
	"passive_voice":          "Rewrite common passive forms ('is being used', 'was created') to active voice.",
	"redundant_because":      "Replace 'due to the fact that' with 'because'.",
	"redundant_directive":    "Strip 'it is important to' / 'you should' / 'remember to' directives.",

	// Category: dedup (multi-turn)
	"repeated_context":      "Replace 'As we discussed earlier' style call-backs with 'See above.'",
	"repeated_question":     "Tag repeat questions with '[same question]'.",
	"reestablished_context": "Replace 'Going back to the code above' / 'Referring back to' with 'Re:'.",
	"summary_replacement":   "Replace 'To summarize what we've discussed' / 'To recap' with 'Summary:'.",

	// Category: ultra (only fires at ultra intensity)
	"ultra_abbreviations": "Abbreviate long technical words (database → DB, configuration → config, function → fn, etc.).",

	// Chinese pack (zh) — keep all entries in one map keyed by name.
	"zh_filler_please":                    "Strip Chinese politeness lead-ins: 请, 请你, 请您, 请帮我, 请帮忙.",
	"zh_filler_thanks":                    "Strip Chinese thanks: 谢谢, 多谢, 感谢, 谢谢你, 感谢大家.",
	"zh_filler_trouble":                   "Strip Chinese trouble-apology wrappers: 麻烦你, 麻烦您, 劳驾.",
	"zh_filler_greeting":                  "Strip Chinese greetings: 你好, 您好, 大家好.",
	"zh_filler_hedge_think":               "Cut Chinese hedging: 我觉得, 我认为, 我想说.",
	"zh_filler_hedge_actually":            "Cut Chinese 'actually' hedging: 其实, 说实话, 基本上.",
	"zh_repeated_context":                 "Replace Chinese 'as discussed earlier' call-backs with 见上.",
	"zh_repeated_question":                "Tag Chinese repeat questions with ［同问］.",
	"zh_reestablished_context":            "Replace Chinese 'going back to' call-backs with Re:.",
	"zh_summary_replacement":              "Replace Chinese 'to summarize' lead-ins with 总结：.",
	"zh_ultra_modal_particles":            "Drop Chinese modal particles (吗, 呢, 吧, 啊, 呀, 嘛) before sentence-final punctuation.",
	"zh_ultra_database_abbreviation":      "Abbreviate 数据库 to DB.",
	"zh_ultra_application_abbreviation":   "Abbreviate 应用程序 to app.",
	"zh_ultra_dependency_abbreviation":    "Abbreviate 依赖关系 to dep.",
	"zh_ultra_config_file_abbreviation":   "Abbreviate 配置文件 to cfg.",
	"zh_ultra_function_abbreviation":      "Abbreviate 函数 to fn.",
}

// cavemanRuleCategoryLabels is the human-readable label for each Category
// value. Used both for grouping in the UI and as the rule's `label` when
// description lookup falls through.
var cavemanRuleCategoryLabels = map[string]string{
	"filler":     "Filler removal",
	"terse":      "Terse form",
	"context":    "Context condensation",
	"structural": "Structural compression",
	"dedup":      "Multi-turn dedup",
	"ultra":      "Ultra abbreviations",
}

// cavemanRuleContextLabels maps the message-role gate to a short label.
var cavemanRuleContextLabels = map[string]string{
	string(CavemanContextAll):       "all roles",
	string(CavemanContextUser):      "user",
	string(CavemanContextAssistant): "assistant",
	string(CavemanContextSystem):    "system",
}

// cavemanRuleIntensityLabels maps the MinIntensity gate to a short label.
var cavemanRuleIntensityLabels = map[string]string{
	string(CavemanLite):  "lite",
	string(CavemanFull):  "full",
	string(CavemanUltra): "ultra",
}