package routing

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// headerKeyPattern matches header map access patterns like headers["X-Api-Key"] or headers['X-Api-Key']
var headerKeyPattern = regexp.MustCompile(`headers\[["']([^"']+)["']\]`)

// headerInPattern matches "in headers" membership test patterns like "X-Api-Key" in headers or 'X-Api-Key' in headers
var headerInPattern = regexp.MustCompile(`["']([^"']+)["']\s+in\s+headers`)

// paramKeyPattern matches param map access patterns like params["Region"] or params['Region']
var paramKeyPattern = regexp.MustCompile(`params\[["']([^"']+)["']\]`)

// paramInPattern matches "in params" membership test patterns like "Region" in params or 'Region' in params
var paramInPattern = regexp.MustCompile(`["']([^"']+)["']\s+in\s+params`)

// literalPattern matches double-quoted CEL string literals used by the
// backfill walker. CEL escapes are not supported here — model names never
// contain characters that need escaping.
var literalPattern = regexp.MustCompile(`"([^"]*)"`)

// celModelLiteralPattern matches forward model literal tests in a CEL expression:
//   - model == "x"
//   - model in ["x", "y"]
//
// Reverse predicates (model != "x", model !in [...]) and dynamic predicates
// (.startsWith(), .contains(), .matches()) are deliberately not matched here —
// backfill only exposes literal names, and matching dynamic expressions would
// over-promise what the gateway actually accepts.
var celModelLiteralPattern = regexp.MustCompile(
	`\bmodel\s*(==|in)\s*(\[[^\]]*\]|"[^"]*")`,
)

// NormalizeMapKeysInCEL lowercases header and param keys in CEL expressions
// so that headers["X-Api-Key"] becomes headers["x-api-key"], "X-Api-Key" in headers becomes "x-api-key" in headers,
// params["Region"] becomes params["region"], and "Region" in params becomes "region" in params.
// This ensures CEL expressions match against the normalized (lowercase) map keys at runtime.
func NormalizeMapKeysInCEL(expr string) string {
	toLower := func(match string) string {
		return strings.ToLower(match)
	}
	// Normalize bracket access
	expr = headerKeyPattern.ReplaceAllStringFunc(expr, toLower)
	expr = paramKeyPattern.ReplaceAllStringFunc(expr, toLower)
	// Normalize "in" membership test
	expr = headerInPattern.ReplaceAllStringFunc(expr, toLower)
	expr = paramInPattern.ReplaceAllStringFunc(expr, toLower)
	return expr
}

// validateCELExpression performs basic validation on CEL expression format
func ValidateCELExpression(expr string) error {
	normalized := strings.TrimSpace(expr)
	if normalized == "" || normalized == "true" || normalized == "false" {
		return nil // Empty, true, or false are valid
	}

	// List of allowed operators and keywords
	validPatterns := []string{
		"==", "!=", "&&", "||", ">", "<", ">=", "<=",
		"in ", "matches ", ".startsWith(", ".contains(", ".endsWith(",
		"[", "]", "(", ")", "!",
	}

	// Check if expression contains at least one valid operator
	hasPattern := false
	for _, pattern := range validPatterns {
		if strings.Contains(normalized, pattern) {
			hasPattern = true
			break
		}
	}

	if !hasPattern {
		return fmt.Errorf("expression must contain at least one operator: %s", expr)
	}

	return nil
}

// ExtractModelLiterals returns the set of model name literals that a routing
// rule's condition exposes. Only forward equality ("model == \"x\"") and
// membership ("model in [\"x\", \"y\"]") produce literals; reverse or dynamic
// predicates are ignored so backfill cannot misrepresent what a rule accepts.
//
// Two input sources are consulted, in order:
//  1. celExpression — the legacy CEL expression stored alongside the rule.
//  2. queryJSON — the react-querybuilder RuleGroupType serialized as JSON,
//     optionally nil when the rule was authored only via CEL.
//
// The returned slice is de-duplicated; ordering is unspecified.
func ExtractModelLiterals(celExpression, queryJSON string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		raw = strings.Trim(raw, `"`)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}

	if strings.TrimSpace(celExpression) != "" {
		for _, m := range celModelLiteralPattern.FindAllStringSubmatch(celExpression, -1) {
			operand := m[2]
			if strings.HasPrefix(operand, "[") {
				for _, lit := range literalPattern.FindAllString(operand, -1) {
					add(lit)
				}
				continue
			}
			add(operand)
		}
	}

	if strings.TrimSpace(queryJSON) != "" {
		walkQueryLiterals(queryJSON, add)
	}

	return out
}

// queryRule mirrors the react-querybuilder RuleGroupType shape we care about.
// Extra fields are ignored, so callers may serialize the full RuleGroupType
// without truncation.
type queryRule struct {
	Combinator string      `json:"combinator"`
	Rules      []queryRule `json:"rules"`
	Field      string      `json:"field"`
	Operator   string      `json:"operator"`
	Value      any         `json:"value"`
}

// walkQueryLiterals recursively decodes a query JSON tree and forwards each
// model-field node to the add callback when the operator is forward-only.
//
// The query argument is decoded lazily one level at a time so we don't pay
// for parsing branches that don't contain a model condition. This also keeps
// the function total even when the JSON shape doesn't match — the decoder
// error is intentionally swallowed, since callers cannot meaningfully react to
// a partial parse inside an enrichment path.
func walkQueryLiterals(raw string, add func(string)) {
	var root queryRule
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return
	}
	visitQueryRule(&root, add)
}

func visitQueryRule(r *queryRule, add func(string)) {
	if r == nil {
		return
	}
	if r.Field == "model" {
		switch r.Operator {
		case "=", "==":
			if s, ok := r.Value.(string); ok {
				add(s)
			}
		case "in":
			switch v := r.Value.(type) {
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						add(s)
					}
				}
			case string:
				// Some query-builder adapters serialize the in-list as a single
				// comma-separated string. Treat it as a fallback so a stray
				// serialization can't silently hide the literal.
				for _, item := range strings.Split(v, ",") {
					add(item)
				}
			}
		}
	}
	for i := range r.Rules {
		visitQueryRule(&r.Rules[i], add)
	}
}
