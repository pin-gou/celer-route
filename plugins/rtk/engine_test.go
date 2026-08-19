package rtk

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pin-gou/pg-gateway/core/schemas"
)

// ============================================================================
// Task 6.1: CompressionEngine interface contract + EngineCatalog registration
//          + Pipeline runner order execution + unknown id fail-soft (red phase)
//
// TDD red phase: CompressionEngine, EngineCatalog, Pipeline, EngineBreakdown
// do not exist yet. All tests will fail with compile-time "undefined" errors.
// ============================================================================

// TestEngineCatalogRegisterAndGet verifies that EngineCatalog supports
// registering and retrieving engines by ID. After dev, the catalog must
// return the registered engine for a known ID and report false for an
// unknown ID (not panic).
func TestEngineCatalogRegisterAndGet(t *testing.T) {
	catalog := NewEngineCatalog()

	engine := &mockCompressionEngine{id: "rtk"}
	catalog.RegisterEngine("rtk", engine)

	got, ok := catalog.GetEngine("rtk")
	if !ok {
		t.Fatal("EngineCatalog.GetEngine('rtk') should return ok=true after RegisterEngine")
	}
	if got == nil {
		t.Fatal("EngineCatalog.GetEngine('rtk') returned nil engine")
	}

	// Unknown ID must return ok=false, not panic
	_, ok = catalog.GetEngine("unknown-engine")
	if ok {
		t.Error("EngineCatalog.GetEngine('unknown-engine') should return ok=false")
	}
}

// TestEngineCatalogRegisterDuplicate verifies that registering the same ID
// twice replaces the previous engine (last-write-wins is the expected
// behaviour for the catalog, allowing override/swap at init time).
func TestEngineCatalogRegisterDuplicate(t *testing.T) {
	catalog := NewEngineCatalog()
	catalog.RegisterEngine("rtk", &mockCompressionEngine{id: "rtk-v1"})
	catalog.RegisterEngine("rtk", &mockCompressionEngine{id: "rtk-v2"})

	engine, ok := catalog.GetEngine("rtk")
	if !ok {
		t.Fatal("EngineCatalog.GetEngine('rtk') should return ok=true after two RegisterEngine calls")
	}
	if engine == nil {
		t.Fatal("EngineCatalog.GetEngine('rtk') returned nil engine")
	}
	// The last registered engine should be the active one
	if engine.(*mockCompressionEngine).id != "rtk-v2" {
		t.Error("EngineCatalog should return the last-registered engine for duplicate ID (last-write-wins)")
	}
}

// TestEngineCatalogListEngines verifies that EngineCatalog supports listing
// all registered engine IDs. After dev, the catalog must return the exact
// set of registered IDs, and an empty catalog returns an empty slice.
func TestEngineCatalogListEngines(t *testing.T) {
	catalog := NewEngineCatalog()

	// Empty catalog
	ids := catalog.ListEngines()
	if len(ids) != 0 {
		t.Errorf("empty catalog should return empty list, got %v", ids)
	}

	// After registering engines
	catalog.RegisterEngine("rtk", &mockCompressionEngine{id: "rtk"})
	catalog.RegisterEngine("dedup", &mockCompressionEngine{id: "dedup"})
	catalog.RegisterEngine("grouping", &mockCompressionEngine{id: "grouping"})

	ids = catalog.ListEngines()
	if len(ids) != 3 {
		t.Fatalf("expected 3 registered engines, got %d: %v", len(ids), ids)
	}

	// Verify all IDs are present
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	for _, want := range []string{"rtk", "dedup", "grouping"} {
		if !idSet[want] {
			t.Errorf("expected engine ID %q in list, got %v", want, ids)
		}
	}
}

// TestPipelineRunnerOrderedExecution verifies that the Pipeline runner
// executes engines in the order specified by the Pipeline.Engines slice.
// After dev, each engine must be called exactly once, in order, and the
// output of each engine feeds into the next (pipeline chaining).
func TestPipelineRunnerOrderedExecution(t *testing.T) {
	catalog := NewEngineCatalog()
	executionOrder := make([]string, 0)

	// Register engines that record their execution order
	catalog.RegisterEngine("engine-a", &recordingEngine{id: "engine-a", order: &executionOrder})
	catalog.RegisterEngine("engine-b", &recordingEngine{id: "engine-b", order: &executionOrder})
	catalog.RegisterEngine("engine-c", &recordingEngine{id: "engine-c", order: &executionOrder})

	pipeline := &Pipeline{
		Engines: []string{"engine-a", "engine-b", "engine-c"},
	}

	runner := NewPipelineRunner(catalog)
	input := "some tool output text to compress"
	result, breakdown, _, _ := runner.Run(nil, pipeline, input, EngineConfig{})

	// Verify engines were called in order
	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 engine executions, got %d: %v", len(executionOrder), executionOrder)
	}
	if executionOrder[0] != "engine-a" {
		t.Errorf("expected engine-a to execute first, got %q", executionOrder[0])
	}
	if executionOrder[1] != "engine-b" {
		t.Errorf("expected engine-b to execute second, got %q", executionOrder[1])
	}
	if executionOrder[2] != "engine-c" {
		t.Errorf("expected engine-c to execute third, got %q", executionOrder[2])
	}

	// Result should not be empty (the engines should have processed the text)
	if result == "" {
		t.Error("Pipeline runner returned empty result for non-empty input")
	}

	// Breakdown should contain entries for all three engines
	if len(breakdown) != 3 {
		t.Errorf("expected 3 engine breakdown entries, got %d", len(breakdown))
	}
}

// TestPipelineRunnerEmptyPipeline verifies that the Pipeline runner handles
// an empty engine list gracefully (no engines to execute, input passes through).
func TestPipelineRunnerEmptyPipeline(t *testing.T) {
	catalog := NewEngineCatalog()
	runner := NewPipelineRunner(catalog)

	pipeline := &Pipeline{
		Engines: []string{},
	}

	input := "some tool output"
	result, breakdown, _, _ := runner.Run(nil, pipeline, input, EngineConfig{})

	if result != input {
		t.Errorf("empty pipeline should pass input through unchanged, got %q, want %q", result, input)
	}
	if len(breakdown) != 0 {
		t.Errorf("empty pipeline should produce empty breakdown, got %d entries", len(breakdown))
	}
}

// TestPipelineRunnerNilPipeline verifies nil pipeline safety.
func TestPipelineRunnerNilPipeline(t *testing.T) {
	catalog := NewEngineCatalog()
	runner := NewPipelineRunner(catalog)

	input := "some tool output"
	result, breakdown, _, _ := runner.Run(nil, nil, input, EngineConfig{})

	if result != input {
		t.Errorf("nil pipeline should pass input through unchanged, got %q, want %q", result, input)
	}
	if len(breakdown) != 0 {
		t.Errorf("nil pipeline should produce empty breakdown, got %d entries", len(breakdown))
	}
}

// TestPipelineRunnerUnknownEngineIdFailSoft verifies that when a pipeline
// references an engine ID that is not registered in the catalog, the runner
// does not panic. Instead, it should warn and skip that engine, continuing
// with the next engine in the pipeline. After dev, this is the fail-soft
// behaviour required by design: unknown id -> warn + skip, not panic.
func TestPipelineRunnerUnknownEngineIdFailSoft(t *testing.T) {
	catalog := NewEngineCatalog()
	executionOrder := make([]string, 0)

	catalog.RegisterEngine("engine-a", &recordingEngine{id: "engine-a", order: &executionOrder})
	catalog.RegisterEngine("engine-c", &recordingEngine{id: "engine-c", order: &executionOrder})

	// Pipeline references an unknown engine "engine-b" between known ones
	pipeline := &Pipeline{
		Engines: []string{"engine-a", "engine-b", "engine-c"},
	}

	runner := NewPipelineRunner(catalog)
	input := "some tool output text to compress"

	// Must not panic
	result, breakdown, _, _ := runner.Run(nil, pipeline, input, EngineConfig{})

	// engine-a and engine-c should have executed, engine-b skipped
	if len(executionOrder) != 2 {
		t.Fatalf("expected 2 engines to execute (engine-b skipped), got %d: %v", len(executionOrder), executionOrder)
	}
	if executionOrder[0] != "engine-a" {
		t.Errorf("expected engine-a first, got %q", executionOrder[0])
	}
	if executionOrder[1] != "engine-c" {
		t.Errorf("expected engine-c second, got %q", executionOrder[1])
	}

	// Result should not be empty (other engines processed)
	if result == "" {
		t.Error("Pipeline runner should still produce output when unknown engines are skipped")
	}

	// Breakdown should have entries only for known engines
	if len(breakdown) != 2 {
		t.Errorf("expected 2 breakdown entries (skipped engine-b), got %d", len(breakdown))
	}
}

// TestPipelineRunnerAllUnknownIds verifies that when all pipeline engines are
// unknown, the input passes through unchanged and no panic occurs.
func TestPipelineRunnerAllUnknownIds(t *testing.T) {
	catalog := NewEngineCatalog()
	runner := NewPipelineRunner(catalog)

	pipeline := &Pipeline{
		Engines: []string{"nonexistent-a", "nonexistent-b"},
	}

	input := "some tool output"
	result, breakdown, _, _ := runner.Run(nil, pipeline, input, EngineConfig{})

	if result != input {
		t.Errorf("when all engines are unknown, input should pass through, got %q, want %q", result, input)
	}
	if len(breakdown) != 0 {
		t.Errorf("when all engines are unknown, breakdown should be empty, got %d", len(breakdown))
	}
}

// TestPipelineRunnerEngineErrorDoesNotAbort verifies that if one engine in
// the pipeline returns an error, the runner does not abort the entire
// pipeline. Instead, it records the error in the breakdown for that engine
// and continues with the next engine in the sequence.
func TestPipelineRunnerEngineErrorDoesNotAbort(t *testing.T) {
	catalog := NewEngineCatalog()
	executionOrder := make([]string, 0)

	catalog.RegisterEngine("engine-ok", &recordingEngine{id: "engine-ok", order: &executionOrder})
	catalog.RegisterEngine("engine-error", &errorEngine{id: "engine-error", order: &executionOrder})
	catalog.RegisterEngine("engine-after", &recordingEngine{id: "engine-after", order: &executionOrder})

	pipeline := &Pipeline{
		Engines: []string{"engine-ok", "engine-error", "engine-after"},
	}

	runner := NewPipelineRunner(catalog)
	input := "some tool output text to compress"

	// Must not panic from the error engine
	result, breakdown, _, _ := runner.Run(nil, pipeline, input, EngineConfig{})
	_ = result // result is not directly asserted in this test, only execution order

	// All three engines should have been attempted
	if len(executionOrder) != 3 {
		t.Fatalf("expected all 3 engines to execute, got %d: %v", len(executionOrder), executionOrder)
	}

	// engine-after should still execute even though engine-error failed
	if executionOrder[2] != "engine-after" {
		t.Errorf("expected engine-after to execute third despite preceding error, got %q", executionOrder[2])
	}

	// Breakdown should include the error engine
	if len(breakdown) < 3 {
		t.Errorf("expected 3 breakdown entries, got %d", len(breakdown))
	}
}

// TestPipelineRunnerInputPassThrough verifies that when the pipeline is nil
// or empty, the input text passes through verbatim (no transformation).
func TestPipelineRunnerInputPassThrough(t *testing.T) {
	catalog := NewEngineCatalog()
	runner := NewPipelineRunner(catalog)

	tests := []struct {
		name     string
		pipeline *Pipeline
	}{
		{"nil pipeline", nil},
		{"empty engines", &Pipeline{Engines: []string{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "exact text to preserve"
			result, _, _, _ := runner.Run(nil, tt.pipeline, input, EngineConfig{})
			if result != input {
				t.Errorf("result = %q, want %q (input passed through)", result, input)
			}
		})
	}
}

// TestEngineBreakdownAccumulation verifies that the breakdown slice
// accumulates per-engine stats after pipeline execution, with each entry
// containing the engine ID, original tokens, compressed tokens, and any
// error. After dev, the breakdown must be ordered by execution order.
func TestEngineBreakdownAccumulation(t *testing.T) {
	catalog := NewEngineCatalog()
	catalog.RegisterEngine("engine-a", &mockCompressionEngine{id: "engine-a"})
	catalog.RegisterEngine("engine-b", &mockCompressionEngine{id: "engine-b"})

	pipeline := &Pipeline{
		Engines: []string{"engine-a", "engine-b"},
	}

	runner := NewPipelineRunner(catalog)
	input := "some tool output text to compress"
	_, breakdown, _, _ := runner.Run(nil, pipeline, input, EngineConfig{})

	if len(breakdown) != 2 {
		t.Fatalf("expected 2 breakdown entries, got %d", len(breakdown))
	}

	// First entry should be engine-a
	if breakdown[0].Id != "engine-a" {
		t.Errorf("breakdown[0].Id = %q, want %q", breakdown[0].Id, "engine-a")
	}
	// Second entry should be engine-b
	if breakdown[1].Id != "engine-b" {
		t.Errorf("breakdown[1].Id = %q, want %q", breakdown[1].Id, "engine-b")
	}
}

// ============================================================================
// Test helpers (these reference types that don't exist yet — compile error
// expected in red phase)
// ============================================================================

// mockCompressionEngine is a test helper that implements CompressionEngine
// for testing the catalog and pipeline runner.
type mockCompressionEngine struct {
	id string
}

func (m *mockCompressionEngine) Id() string {
	return m.id
}

func (m *mockCompressionEngine) Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error) {
	// Return the text unchanged with a pass-through result
	return EngineResult{
		Text:         text,
		InputBytes:   len(text),
		OutputBytes:  len(text),
		CompressedBy: 0,
	}, nil
}

func (m *mockCompressionEngine) HealthCheck() error {
	return nil
}

func (m *mockCompressionEngine) IsEnabled() bool {
	return true
}

func (m *mockCompressionEngine) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

// recordingEngine is a test helper that records its execution order
// and implements CompressionEngine.
type recordingEngine struct {
	id    string
	order *[]string
}

func (r *recordingEngine) Id() string {
	return r.id
}

func (r *recordingEngine) Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error) {
	*r.order = append(*r.order, r.id)
	return EngineResult{
		Text:         text,
		InputBytes:   len(text),
		OutputBytes:  len(text),
		CompressedBy: 0,
	}, nil
}

func (r *recordingEngine) HealthCheck() error {
	return nil
}

func (r *recordingEngine) IsEnabled() bool {
	return true
}

func (r *recordingEngine) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

// errorEngine is a test helper that returns an error on Apply.
type errorEngine struct {
	id    string
	order *[]string
}

func (e *errorEngine) Id() string {
	return e.id
}

func (e *errorEngine) Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error) {
	*e.order = append(*e.order, e.id)
	return EngineResult{}, fmt.Errorf("rtk: engine %q error", e.id)
}

func (e *errorEngine) HealthCheck() error {
	return nil
}

func (e *errorEngine) IsEnabled() bool {
	return true
}

func (e *errorEngine) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}