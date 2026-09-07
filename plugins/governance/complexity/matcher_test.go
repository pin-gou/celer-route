package complexity

import "testing"

func TestCompiledKeywordMatcher_StemmedSingleWordMatchesInflection(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"debug"},
	})

	signals := matcher.analyzeText("We are debugging the handler before release.", lastTextBaseScanMask)
	if signals.codeCount != 1 {
		t.Fatalf("expected stemmed debug keyword to match debugging once, got codeCount=%d", signals.codeCount)
	}
}

func TestCompiledKeywordMatcher_CustomInflectedKeywordMatchesRootForm(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"debugging"},
	})

	signals := matcher.analyzeText("Please debug the handler before release.", lastTextBaseScanMask)
	if signals.codeCount != 1 {
		t.Fatalf("expected custom debugging keyword to match debug once, got codeCount=%d", signals.codeCount)
	}
}

func TestCompiledKeywordMatcher_ExactAndStemmedMatchDoesNotDoubleCount(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"debug"},
	})

	signals := matcher.analyzeText("Please debug the handler before release.", lastTextBaseScanMask)
	if signals.codeCount != 1 {
		t.Fatalf("expected exact debug match not to be double-counted by stemming, got codeCount=%d", signals.codeCount)
	}
}

func TestCompiledKeywordMatcher_RepeatedStemmedFormsDoNotDoubleCount(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"debug"},
	})

	signals := matcher.analyzeText("Please debugged and debugging the handler before release.", lastTextBaseScanMask)
	if signals.codeCount != 1 {
		t.Fatalf("expected repeated stemmed forms of debug to count once, got codeCount=%d", signals.codeCount)
	}
}

func TestCompiledKeywordMatcher_StemDedupePreservesDistinctSameStemKeywords(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords:      []string{"debug"},
		TechnicalKeywords: []string{"debugging"},
	})

	signals := matcher.analyzeText("Please debug the handler before release.", lastTextBaseScanMask)
	if signals.codeCount != 1 {
		t.Fatalf("expected exact debug keyword to contribute code once, got codeCount=%d", signals.codeCount)
	}
	if signals.technicalCount != 1 {
		t.Fatalf("expected distinct same-stem debugging keyword to contribute technical once, got technicalCount=%d", signals.technicalCount)
	}
}

func TestCompiledKeywordMatcher_StemmedPhraseMatchesContiguousVariant(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"analyzing this function"},
	})

	signals := matcher.analyzeText("Please analyze this function before merging.", lastTextBaseScanMask)
	if signals.codeCount != 1 {
		t.Fatalf("expected stemmed phrase to match contiguous variant once, got codeCount=%d", signals.codeCount)
	}
}

func TestCompiledKeywordMatcher_StemmedPhraseDoesNotMatchInsertedWords(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"analyzing this function"},
	})

	signals := matcher.analyzeText("Please analyze this Python function before merging.", lastTextBaseScanMask)
	if signals.codeCount != 0 {
		t.Fatalf("expected stemmed phrase not to match with inserted words, got codeCount=%d", signals.codeCount)
	}
}

func TestCompiledKeywordMatcher_PunctuationKeywordRemainsLiteral(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"ci/cd"},
	})

	literalSignals := matcher.analyzeText("The ci/cd pipeline is failing.", lastTextBaseScanMask)
	if literalSignals.codeCount != 1 {
		t.Fatalf("expected literal ci/cd keyword to match, got codeCount=%d", literalSignals.codeCount)
	}

	tokenSignals := matcher.analyzeText("The ci cd pipeline is failing.", lastTextBaseScanMask)
	if tokenSignals.codeCount != 0 {
		t.Fatalf("expected ci cd not to match literal ci/cd keyword, got codeCount=%d", tokenSignals.codeCount)
	}
}

func TestCompiledKeywordMatcher_CJKKeywordMatchesWithinContinuousText(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		SimpleKeywords: []string{"提交并推送"},
	})

	signals := matcher.analyzeText("请提交并推送代码到仓库", lastTextBaseScanMask)
	if signals.simpleCount != 1 {
		t.Fatalf("expected CJK keyword 提交并推送 to match inside continuous Chinese text, got simpleCount=%d", signals.simpleCount)
	}
}

func TestCompiledKeywordMatcher_CJKSuffixKeywordMatchesContinuousText(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		SimpleKeywords: []string{"推送代码", "提交代码", "提交并推送"},
	})

	signals := matcher.analyzeText("提交并推送代码", lastTextBaseScanMask)
	if signals.simpleCount == 0 {
		t.Fatalf("expected at least one CJK simple keyword to match 提交并推送代码, got simpleCount=%d", signals.simpleCount)
	}
}

func TestCompiledKeywordMatcher_ASCIIWholeWordRemainsBoundarySensitive(t *testing.T) {
	matcher := newCompiledKeywordMatcher(KeywordConfig{
		CodeKeywords: []string{"code"},
	})

	insideSignals := matcher.analyzeText("the codepath is long", lastTextBaseScanMask)
	if insideSignals.codeCount != 0 {
		t.Fatalf("expected ASCII whole-word keyword code not to match inside codepath, got codeCount=%d", insideSignals.codeCount)
	}

	standaloneSignals := matcher.analyzeText("the code is short", lastTextBaseScanMask)
	if standaloneSignals.codeCount != 1 {
		t.Fatalf("expected ASCII keyword code to match as a standalone word, got codeCount=%d", standaloneSignals.codeCount)
	}
}
