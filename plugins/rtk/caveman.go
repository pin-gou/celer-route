package rtk

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/pin-gou/celer-route/core/schemas"
)

// CavemanConfig holds the settings for the Caveman prose-compression engine.
// It mirrors OmniRoute's CavemanConfig and is nested under the RTK plugin
// Config so a single plugin instance can run both compression engines.
type CavemanConfig struct {
	// Enabled enables the Caveman engine. Default false: Caveman is opt-in
	// and does not change existing requests unless explicitly configured.
	Enabled bool `json:"enabled"`

	// Intensity is the compression aggressiveness: lite | full | ultra.
	// Default "lite" (safe, minimal semantic risk).
	Intensity string `json:"intensity"`

	// Language selects the rule pack: "auto" (detect per message) | "en" | "zh".
	// Default "auto".
	Language string `json:"language"`

	// MinMessageLength is the minimum text length (runes) before a message is
	// eligible for compression. Shorter messages pass through unchanged.
	// Default 50 (aligned with OmniRoute DEFAULT_CAVEMAN_CONFIG).
	MinMessageLength int `json:"min_message_length"`

	// CompressRoles lists the message roles Caveman may compress. Default
	// ["user"] — the conservative scope (system is never touched to protect
	// prompt-cache prefix stability).
	CompressRoles []string `json:"compress_roles"`

	// SkipRules blacklists rule names from execution. Empty = all rules run.
	SkipRules []string `json:"skip_rules"`

	// PreservePatterns are extra user-supplied regexes for regions to protect
	// from compression (on top of the built-in 17 preservation patterns).
	PreservePatterns []string `json:"preserve_patterns"`
}

// CavemanIntensityDefault is the intensity used when Config.Intensity is empty.
const CavemanIntensityDefault = "lite"

// defaultCavemanConfig returns a fully-defaulted CavemanConfig copy.
func defaultCavemanConfig() CavemanConfig {
	return CavemanConfig{
		Enabled:          false,
		Intensity:        CavemanIntensityDefault,
		Language:         "auto",
		MinMessageLength: 50,
		CompressRoles:    []string{"user"},
	}
}

// CavemanIntensitiesValid lists the accepted intensity values.
var CavemanIntensitiesValid = map[string]bool{"lite": true, "full": true, "ultra": true}

// Validate validates the Caveman config, returning an error for out-of-range
// or invalid values. Called from Config.Validate during Init.
func (c *CavemanConfig) Validate() error {
	if c.Intensity != "" && !CavemanIntensitiesValid[c.Intensity] {
		return newCavemanConfigError("invalid intensity %q: must be one of lite, full, ultra", c.Intensity)
	}
	if c.MinMessageLength < 0 {
		return newCavemanConfigError("min_message_length must be >= 0, got %d", c.MinMessageLength)
	}
	if c.Language != "" && c.Language != "auto" && !cavemanLanguageSupported(c.Language) {
		return newCavemanConfigError("invalid language %q: must be auto, en or zh", c.Language)
	}
	for _, role := range c.CompressRoles {
		if role != "user" && role != "assistant" && role != "system" {
			return newCavemanConfigError("invalid compress_role %q: must be user, assistant or system", role)
		}
	}
	return nil
}

// cavemanConfigError is a small typed error for Caveman config validation.
type cavemanConfigError struct{ msg string }

func (e *cavemanConfigError) Error() string { return e.msg }

func newCavemanConfigError(format string, args ...interface{}) error {
	return &cavemanConfigError{msg: fmt.Sprintf(format, args...)}
}

// cavemanLanguageSupported reports whether lang is a compiled-in pack.
func cavemanLanguageSupported(lang string) bool {
	switch lang {
	case "en", "zh":
		return true
	default:
		return false
	}
}

// normalizeCavemanConfig fills zero-value CavemanConfig fields with defaults.
// It must run before any compression path uses the config.
func normalizeCavemanConfig(c *CavemanConfig) {
	if c == nil {
		return
	}
	d := defaultCavemanConfig()
	if c.Intensity == "" {
		c.Intensity = d.Intensity
	}
	if c.Language == "" {
		c.Language = d.Language
	}
	if c.MinMessageLength == 0 {
		c.MinMessageLength = d.MinMessageLength
	}
	if len(c.CompressRoles) == 0 {
		c.CompressRoles = d.CompressRoles
	}
	if len(c.SkipRules) == 0 {
		c.SkipRules = nil
	}
	if len(c.PreservePatterns) == 0 {
		c.PreservePatterns = nil
	}
}

// cavemanCompressionOutcome is the per-message result of the Caveman engine.
type cavemanCompressionOutcome struct {
	Text            string   // final text (original when compressed == false)
	Original        string   // pre-compression text
	Compressed      bool     // text actually changed
	AppliedRules    []string // rule names that fired
	Language        string   // detected/selected language
	Preserved       int      // number of preserved blocks
	ValidationOK    bool     // fidelity checks passed
	FallbackApplied bool     // validation failed → original text returned
}

// cavemanCompressText runs the full Caveman pipeline on a single text block
// (one user message's content):
//
//	normalize text → extract preserved blocks → select language → apply rules
//	→ cleanup/recapitalize → restore blocks → validate fidelity → fallback.
//
// Mirrors OmniRoute's cavemanCompress per-message logic. It is a pure function:
// no I/O, no logging.
func cavemanCompressText(text string, cfg CavemanConfig) cavemanCompressionOutcome {
	// Defensive defaulting so the pure function behaves predictably even when
	// called with a zero-value config (e.g. a unit test). The production path
	// normalises before calling, so this is a no-op there.
	normalizeCavemanConfig(&cfg)
	out := cavemanCompressionOutcome{Original: text, Text: text, ValidationOK: true}
	if text == "" {
		return out
	}
	if len([]rune(text)) < cfg.MinMessageLength {
		return out
	}

	// Detect the language unless the config pins one.
	lang := normalizeCavemanLanguage(cfg.Language)
	if lang == "auto" {
		lang = normalizeCavemanLanguage(detectCavemanLanguage(text))
	}
	out.Language = lang

	// Skip rules blacklist.
	skip := make(map[string]bool, len(cfg.SkipRules))
	for _, name := range cfg.SkipRules {
		skip[name] = true
	}

	// Extract preserved blocks (code fences, URLs, markdown, inline code, ...).
	pres := newCavemanPreserve("")
	custom := compileCavemanCustomPatterns(cfg.PreservePatterns)
	extracted := pres.extract(text, custom)
	out.Preserved = len(pres.blocks)

	// Select and apply rules for the user role at the configured intensity.
	rules := getRulesForContext(CavemanContextUser, CavemanIntensity(cfg.Intensity), lang)
	filtered := make([]*cavemanRule, 0, len(rules))
	for _, r := range rules {
		if !skip[r.name] {
			filtered = append(filtered, r)
		}
	}
	applied, appliedRules := applyRulesToText(extracted, filtered)
	out.AppliedRules = appliedRules

	// Normalise whitespace artifacts and restore the preserved blocks.
	cleaned := applied
	if notCodeDominant(applied) {
		cleaned = cleanupCavemanArtifacts(recapitalizeCavemanSentences(cleaned))
	} else {
		cleaned = cleanupCavemanArtifacts(cleaned)
	}
	restored := pres.restore(cleaned)
	out.Text = restored

	if restored == text {
		return out
	}

	// Fidelity check — if the restored text lost any protected content, fall
	// back to the original (fail-safe: never ship a semantically degraded
	// user message).
	validation := validateCavemanCompression(text, restored)
	out.ValidationOK = validation.valid
	if !validation.valid {
		out.FallbackApplied = true
		out.Text = text
		out.AppliedRules = nil
		return out
	}
	out.Compressed = restored != text
	return out
}

// cleanupCavemanArtifacts collapses double spaces, mangles punctuation spacing,
// dedups terminal punctuation and trims blank lines. Mirrors OmniRoute's
// cleanupArtifacts.
func cleanupCavemanArtifacts(text string) string {
	if text == "" {
		return ""
	}
	s := text
	s = regexpReplace(s, `[ \t]{2,}`, " ")
	s = regexpReplace(s, `[ \t]+([,.;:!?])`, "$1")
	s = regexpReplace(s, `([.!?]){2,}`, "$1")
	s = regexpReplace(s, `[ \t]+$`, "")
	s = regexpReplace(s, `\n{3,}`, "\n\n")
	s = strings.TrimPrefix(s, "\n")
	s = strings.TrimSuffix(s, "\n")
	return s
}

// recapitalizeCavemanSentences re-capitalises the first letter after sentence
// boundaries (rules like articles may lowercase the word following them).
// Mirrors OmniRoute's recapitalizeSentences.
func recapitalizeCavemanSentences(text string) string {
	return regexpReplaceFunc(text, `(^|[.!?][ \t]|\n[ \t]*)([a-z])`, func(m string) string {
		// Find where the lowercase letter is and uppercase it.
		for i := len(m) - 1; i >= 0; i-- {
			if m[i] >= 'a' && m[i] <= 'z' {
				return m[:i] + string(m[i]-'a'+'A') + m[i+1:]
			}
		}
		return m
	})
}

// regexpReplace is a tiny helper applying a compiled-regex string replacement.
func regexpReplace(text, pattern, repl string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}
	return re.ReplaceAllString(text, repl)
}

// regexpReplaceFunc applies a function replacement over a compiled regex.
func regexpReplaceFunc(text, pattern string, fn func(string) string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}
	return re.ReplaceAllStringFunc(text, fn)
}

// cavemanEngine implements CompressionEngine for the Caveman prose engine.
// It is registered as "caveman" in the global EngineCatalog during Init and
// can be referenced from the plugin's pipeline configuration.
type cavemanEngine struct {
	plugin *Plugin
}

// Id returns the engine identifier.
func (e *cavemanEngine) Id() string {
	return "caveman"
}

// Apply runs the Caveman pipeline over a single text block (a user message
// content). The plugin's Caveman config drives the behaviour; cfg.Enabled
// overrides for preview paths (mirrors rtkEngine).
func (e *cavemanEngine) Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error) {
	if e.plugin == nil || e.plugin.config == nil {
		return EngineResult{Text: text, InputBytes: len(text), OutputBytes: len(text), Skipped: true, Reason: "plugin disabled"}, nil
	}
	cc := e.plugin.config.Caveman
	normalizeCavemanConfig(&cc)
	if !cc.Enabled && !cfg.Enabled {
		return EngineResult{Text: text, InputBytes: len(text), OutputBytes: len(text), Skipped: true, Reason: "disabled by config"}, nil
	}

	out := cavemanCompressText(text, cc)
	techniques := []string{}
	if out.Compressed {
		techniques = append(techniques, "caveman-rules")
		if out.Preserved > 0 {
			techniques = append(techniques, "caveman-preservation")
		}
		if out.FallbackApplied {
			techniques = append(techniques, "caveman-fallback")
		}
	}
	// Reuse RTK's raw-output persistence as an audit trail: when the engine
	// actually compressed the user text and the plugin's RawOutputRetention
	// policy keeps raw copies, persist the original and propagate the
	// pointer through the pipeline so PostLLMHook surfaces the raw-output id
	// in the log metadata. Unlike the RTK tool-result path, the
	// [rtk:raw_output_id=...] marker is NOT embedded into the message itself
	// here: a user message is the model's input, and stuffing a recovery
	// marker into the human's turn would both pollute the prompt and bloat
	// the compressed size past the ≥5% write-back threshold. The verbatim
	// original remains recoverable via /api/context/rtk/raw-output/{id}.
	stats := &ProcessStats{
		OriginalTokens:   estimateTokens(text),
		CompressedTokens: estimateTokens(out.Text),
		OriginalBytes:    len(text),
		Truncated:        out.Compressed,
	}
	config := e.plugin.config
	var loader *FilterLoader
	if e.plugin.loader != nil {
		loader = e.plugin.loader
	}
	rawPointers := []*RtkRawOutputPointer{}
	if out.Compressed {
		maybePersistRawOutput(stats, text, config, loader, "")
		rawPointers = stats.RawOutputPointers
	}

	inputBytes := len(text)
	outputBytes := len(out.Text)
	compressedBy := calcCompressedBy(inputBytes, outputBytes)
	if compressedBy < 0 {
		compressedBy = 0
	}
	res := EngineResult{
		Text:         out.Text,
		InputBytes:   inputBytes,
		OutputBytes:  outputBytes,
		CompressedBy: compressedBy,
		Skipped:      !out.Compressed,
		Techniques:   techniques,
	}
	if len(rawPointers) > 0 {
		res.rawOutputPointers = rawPointers
	}
	return res, nil
}

// HealthCheck returns nil (always healthy).
func (e *cavemanEngine) HealthCheck() error {
	return nil
}

// IsEnabled reports whether the Caveman engine is enabled by the plugin config.
func (e *cavemanEngine) IsEnabled() bool {
	return e.plugin != nil && e.plugin.config != nil && e.plugin.config.Caveman.Enabled
}

// Schema returns a JSON schema describing the engine's config options.
func (e *cavemanEngine) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

// isCodeLikeLine reports whether a trimmed line looks like source code rather
// than prose. Used by the code-dominance heuristic so sentence recapitalisation
// is skipped on code-heavy messages. Mirrors OmniRoute's toolResultCompressor.
func isCodeLikeLine(rawLine string) bool {
	line := strings.TrimSpace(rawLine)
	return strings.HasPrefix(line, "import ") ||
		strings.HasPrefix(line, "export ") ||
		strings.HasPrefix(line, "function ") ||
		strings.HasPrefix(line, "class ") ||
		strings.HasPrefix(line, "const ") ||
		strings.HasPrefix(line, "let ") ||
		strings.HasPrefix(line, "var ") ||
		strings.HasPrefix(line, "return ") ||
		strings.HasPrefix(line, "if(") ||
		strings.HasPrefix(line, "if (") ||
		strings.HasPrefix(line, "for(") ||
		strings.HasPrefix(line, "for (") ||
		strings.HasPrefix(line, "while(") ||
		strings.HasPrefix(line, "while (")
}

// notCodeDominant reports whether the text is NOT dominated by code-like lines,
// in which case sentence recapitalisation is safe. Mirrors OmniRoute's
// isCodeDominantText inverse.
func notCodeDominant(text string) bool {
	if text == "" {
		return true
	}
	lines := strings.Split(text, "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}
	if len(nonEmpty) == 0 {
		return true
	}
	codeLike := 0
	for _, l := range nonEmpty {
		if isCodeLikeLine(l) {
			codeLike++
		}
	}
	return float64(codeLike)/float64(len(nonEmpty)) < 0.3
}
