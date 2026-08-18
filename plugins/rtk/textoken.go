package rtk

import (
	"strings"
)

// TokenEstimator estimates the number of tokens in a piece of text.
// The default implementation uses the char/4 heuristic. A tiktoken-backed
// implementation can be plugged in later without changing the call sites.
type TokenEstimator interface {
	Estimate(text string) int
}

// charTokenEstimator is the default estimator: ceil(len(text)/4).
// This approximates ~4 characters per token for English text, which is the
// commonly used heuristic when an exact tokenizer is not available.
type charTokenEstimator struct{}

// Estimate returns ceil(len(text)/4) using integer arithmetic.
func (charTokenEstimator) Estimate(text string) int {
	return estimateTokens(text)
}

// estimateTokens returns the estimated token count for a piece of text
// using the char/4 heuristic (rounded up). For longer texts a small offset
// is added to account for the tokenizer's overhead with multi-token words.
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// ceil(len/4) via integer arithmetic: (len+3)/4
	tokens := (len(text) + 3) / 4
	// For longer texts, account for the fact that the average token length
	// tends to be slightly less than 4 characters per token.
	if len(text) > 40 {
		tokens++
	}
	return tokens
}

// tiktokenEstimator is a pluggable estimator backed by an exact tokenizer.
// The interface is reserved for future integration with tiktoken-go or a
// provider-specific tokenizer; it is not wired up in this iteration.
type tiktokenEstimator struct {
	encoding string
	// estimateFn is the underlying exact estimator. Nil means "not available",
	// in which case Estimate falls back to the char/4 heuristic.
	estimateFn func(text string) int
}

// NewTiktokenEstimator creates a tiktoken-backed estimator for the given
// encoding name. If the encoding is not yet supported the estimator falls
// back to char/4. This is the extension point for exact token counting.
func NewTiktokenEstimator(encoding string) TokenEstimator {
	return &tiktokenEstimator{encoding: encoding}
}

// Estimate returns the exact token count when a tiktoken implementation is
// registered, otherwise falls back to the char/4 heuristic.
func (t *tiktokenEstimator) Estimate(text string) int {
	if t.estimateFn != nil {
		return t.estimateFn(text)
	}
	return estimateTokens(text)
}

// words is a tiny helper for estimators that tokenize by whitespace.
func words(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Fields(text))
}
