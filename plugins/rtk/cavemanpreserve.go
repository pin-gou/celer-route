package rtk

import (
	"fmt"
	"regexp"
	"strings"
)

// cavemanPreservedBlock is a textual region extracted before rule application
// and restored verbatim afterwards. Rules never touch the inside of these
// blocks, which protects code, URLs, markdown structure and other technical
// tokens from prose-only transformations.
type cavemanPreservedBlock struct {
	placeholder string
	content     string
}

// cavemanPreserve extracts protected regions from a message. It mirrors
// OmniRoute's preservation.ts: a seeded sentinel prefix namespaces the
// placeholders so they cannot collide with anything the model produces, and
// each block is stored by placeholder so restoration is an exact substitution.
type cavemanPreserve struct {
	prefix  string
	blocks  []cavemanPreservedBlock
	counter int
}

// newCavemanPreserve creates a preservation session with a random 8-byte seed
// in the sentinel (hex-encoded).
func newCavemanPreserve(seed string) *cavemanPreserve {
	if seed == "" {
		seed = "0000000000000000"
	}
	return &cavemanPreserve{prefix: fmt.Sprintf("\x00CAVEMAN_r%s_", seed)}
}

// preservePatterns are the built-in protected-region patterns, applied in
// order. Ported from OmniRoute preservation.ts. Later patterns operate on the
// already-placeholder-substituted text, so nested/overlapping extraction is
// safe (a sentinel can never match another pattern).
var preservePatterns = []struct {
	name    string
	pattern string
}{
	{name: "frontmatter", pattern: `(?ms)^---.*?---\s*$`},
	{name: "fenced-code", pattern: "(?ms)```[^`]*```|~~~[^~]*~~~"},
	{name: "math-block", pattern: `(?s)\$\$[\s\S]*?\$\$|\\\[[\s\S]*?\\\]`},
	{name: "latex-block", pattern: `(?s)\\begin\{[A-Za-z*]+\}[\s\S]*?\\end\{[A-Za-z*]+\}`},
	{name: "markdown-heading", pattern: `(?m)^#{1,6}\s+.+$`},
	{name: "markdown-table", pattern: `(?m)^\s*\|.*\|\s*$|^\s*\|?[:\-]{3,}[:\-]?\|?`},
	{name: "typst-directive", pattern: `(?m)^\s*#(?:set|show|let|import|include)\b.+$`},
	{name: "inline-code", pattern: "`[^`\\n]+`"},
	{name: "markdown-link", pattern: `\[[^\]\n]+\]\([^) \n]+(?:\s+"[^"]*")?\)`},
	{name: "url", pattern: `\bhttps?://[^\s)"'>]+`},
	{name: "const-case", pattern: `\b[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+\b`},
	{name: "env-var", pattern: `\bprocess\.env\.[A-Za-z_][A-Za-z0-9_]*\b|\$[A-Z_][A-Z0-9_]*\b`},
	{name: "version", pattern: `\b\d+(?:\.\d+){1,3}(?:[-+][A-Za-z0-9.-]+)?\b`},
	{name: "dotted-identifier", pattern: `\b[a-zA-Z_$][\w$]*(?:\.[a-zA-Z_$][\w$]*)+\(\)?`},
	{name: "function-call", pattern: `\b[A-Za-z_$][\w$]*\s*\([^()\n]*\)`},
	{name: "file-path", pattern: `(?:^\s|\s)(?:\.{0,2}/[A-Za-z0-9_@./-]+|[A-Za-z]:\\[A-Za-z0-9_.\\/-]+)`},
	{name: "error-message", pattern: `\b(?:TypeError|ReferenceError|SyntaxError|RangeError|URIError|EvalError|Error|Exception):[^\n]+`},
}

// compileCavemanCustomPatterns compiles user-supplied preserve patterns.
// Invalid patterns are skipped (the compile result is a no-op), mirroring the
// fail-open behaviour of OmniRoute's compileUserPreservePatterns.
func compileCavemanCustomPatterns(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

// extract runs the extraction pass and returns the text with every protected
// region replaced by a placeholder.
func (p *cavemanPreserve) extract(text string, custom []*regexp.Regexp) string {
	result := text
	for _, pp := range preservePatterns {
		re, err := regexp.Compile(pp.pattern)
		if err != nil {
			continue
		}
		result = p.substitute(result, re)
	}
	for _, re := range custom {
		result = p.substitute(result, re)
	}
	return result
}

// substitute replaces every non-overlapping match of re with a placeholder and
// records the original content.
func (p *cavemanPreserve) substitute(text string, re *regexp.Regexp) string {
	return re.ReplaceAllStringFunc(text, func(m string) string {
		if m == "" {
			return m
		}
		ph := fmt.Sprintf("%s%x\x00", p.prefix, p.counter)
		p.counter++
		p.blocks = append(p.blocks, cavemanPreservedBlock{placeholder: ph, content: m})
		return ph
	})
}

// restore puts every preserved block back in place.
func (p *cavemanPreserve) restore(text string) string {
	result := text
	for _, b := range p.blocks {
		result = strings.ReplaceAll(result, b.placeholder, b.content)
	}
	return result
}

// separator joins text blocks on a single newline, matching OmniRoute's
// messageContent.textPart joining.
const cavemanBlockSeparator = "\n"
