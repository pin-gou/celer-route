package rtk

import (
	"encoding/json"
	"fmt"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// CompressionEngine defines the interface for compression engines in the pipeline.
// Each engine is registered in the EngineCatalog and executed in order by the
// PipelineRunner. The Apply method takes input text and optional configuration
// and returns the compressed result with processing statistics.
type CompressionEngine interface {
	Id() string
	Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error)
	HealthCheck() error
	IsEnabled() bool
	Schema() json.RawMessage
}

// EngineConfig holds configuration for a single engine in the pipeline.
type EngineConfig struct {
	Enabled  bool            `json:"enabled"`
	Settings json.RawMessage `json:"settings,omitempty"`

	// CommandHint is an optional command string hint that the engine can use
	// to improve filter matching. It is set by the pipeline caller (e.g.
	// applyRtkCompression) from the tool call arguments and is NOT serialised
	// — it is an internal plumbing field so the engine can pass the command
	// hint to processRtkTextWithCommand without changing the engine interface.
	// When non-empty, the command hint is first run through lastCommandSegment
	// to extract the last meaningful segment from composite commands.
	CommandHint string `json:"-"`
}

// EngineResult holds the result of a single engine's compression pass.
type EngineResult struct {
	Text         string  `json:"text"`
	InputBytes   int     `json:"input_bytes"`
	OutputBytes  int     `json:"output_bytes"`
	CompressedBy float64 `json:"compressed_by"`
	Skipped      bool    `json:"skipped,omitempty"`
	Reason       string  `json:"reason,omitempty"`

	// Techniques lists the compression techniques applied by this engine
	// (e.g. "rtk-render:git-diff", "linefilter", "dedup", "smarttruncate").
	// Propagated through the pipeline runner so the caller can record
	// granular technique attribution instead of a generic "pipeline-runner"
	// label.
	Techniques []string `json:"techniques,omitempty"`

	// FilterMatched records the filter ID or Name selected for this compression
	// pass (only set when a filter actually matched). Propagated through the
	// pipeline runner so PostLLMHook can surface it in the log detail view.
	FilterMatched string `json:"filterMatched,omitempty"`

	// rawOutputPointers carries raw output persistence pointers from the
	// engine's Apply method to the caller. It is not serialised — it is
	// an internal plumbing field so the pipeline runner can propagate
	// these pointers from processRtkTextWithCommand through to the
	// CompressionState without discarding them.
	rawOutputPointers []*RtkRawOutputPointer
}

// EngineBreakdown holds per-engine stats for the pipeline result.
type EngineBreakdown struct {
	Id           string  `json:"id"`
	InputBytes   int     `json:"input_bytes"`
	OutputBytes  int     `json:"output_bytes"`
	CompressedBy float64 `json:"compressed_by"`
}

// PipelineResult holds the final result of a pipeline run.
type PipelineResult struct {
	FinalText       string            `json:"text"`
	EngineBreakdown []EngineBreakdown `json:"engine_breakdown"`
}

// Pipeline defines a sequence of engine IDs to run in order.
type Pipeline struct {
	Engines []string
}

// PipelineStep defines a single step in the config's pipeline specification.
type PipelineStep struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config,omitempty"`
}

// EngineCatalog is a registry of named compression engines.
// Engines are registered by ID and can be retrieved, listed, or executed
// by the PipelineRunner.
type EngineCatalog struct {
	engines map[string]CompressionEngine
}

// NewEngineCatalog creates a new empty EngineCatalog.
func NewEngineCatalog() *EngineCatalog {
	return &EngineCatalog{
		engines: make(map[string]CompressionEngine),
	}
}

// RegisterEngine registers a compression engine under the given ID.
// If an engine with the same ID already exists, the last-write-wins
// (the previous engine is replaced).
func (c *EngineCatalog) RegisterEngine(id string, engine CompressionEngine) {
	c.engines[id] = engine
}

// GetEngine retrieves a compression engine by ID. Returns ok=false
// when the ID is not registered.
func (c *EngineCatalog) GetEngine(id string) (CompressionEngine, bool) {
	e, ok := c.engines[id]
	return e, ok
}

// ListEngines returns all registered engine IDs. The order is not
// guaranteed to be stable.
func (c *EngineCatalog) ListEngines() []string {
	ids := make([]string, 0, len(c.engines))
	for id := range c.engines {
		ids = append(ids, id)
	}
	return ids
}

// globalCatalog is the default global engine registry. Engines registered
// here are available to all pipeline runners by default.
var globalCatalog = NewEngineCatalog()

// RegisterEngine registers a compression engine in the global catalog
// using the engine's own Id() as the key. This is the top-level registration
// function called by engines during Init. Engines registered here are
// available to all pipeline runners via the global catalog.
func RegisterEngine(engine CompressionEngine) {
	globalCatalog.RegisterEngine(engine.Id(), engine)
}

// PipelineRunner executes a sequence of compression engines in order.
// Each engine's output feeds into the next (pipeline chaining). Unknown
// engine IDs are logged as warnings and skipped (fail-soft).
type PipelineRunner struct {
	catalog *EngineCatalog
}

// NewPipelineRunner creates a PipelineRunner that uses the given catalog
// to resolve engine IDs.
func NewPipelineRunner(catalog *EngineCatalog) *PipelineRunner {
	return &PipelineRunner{catalog: catalog}
}

// Run executes the pipeline on the input text. Each engine in the pipeline
// is looked up in the catalog and executed in order. The output of each
// engine becomes the input for the next. Unknown engines are skipped with
// a warning. Returns the final text, a breakdown of per-engine stats, an
// aggregated technique list, and any raw output pointers accumulated
// during execution.
func (r *PipelineRunner) Run(ctx *schemas.BifrostContext, pipeline *Pipeline, input string, defaultCfg EngineConfig) (string, []EngineBreakdown, []string, string, error, []*RtkRawOutputPointer) {
	if pipeline == nil || len(pipeline.Engines) == 0 {
		return input, nil, nil, "", nil, nil
	}

	text := input
	var breakdown []EngineBreakdown
	var rawPointers []*RtkRawOutputPointer
	var techniques []string
	var filterMatched string

	for _, engineID := range pipeline.Engines {
		engine, ok := r.catalog.GetEngine(engineID)
		if !ok {
			fmt.Printf("WARN: rtk: unknown engine id %q, skipping\n", engineID)
			continue
		}

		cfg := defaultCfg
		result, err := engine.Apply(ctx, text, cfg)
		if err != nil {
			fmt.Printf("WARN: rtk: engine %q error: %v\n", engineID, err)
			breakdown = append(breakdown, EngineBreakdown{Id: engineID})
			continue
		}

		if result.Text != "" {
			text = result.Text
		}

		entry := EngineBreakdown{Id: engineID}
		entry.InputBytes = result.InputBytes
		entry.OutputBytes = result.OutputBytes
		breakdown = append(breakdown, entry)

		if len(result.Techniques) > 0 {
			techniques = append(techniques, result.Techniques...)
		}

		if result.FilterMatched != "" {
			filterMatched = result.FilterMatched
		}

		if len(result.rawOutputPointers) > 0 {
			rawPointers = append(rawPointers, result.rawOutputPointers...)
		}
	}

	return text, breakdown, techniques, filterMatched, nil, rawPointers
}
