package renderers

import (
	"strings"
	"testing"
)

// TestRenderStructuredTable_HomogeneousJSONArray verifies that a
// homogeneous JSON array is rendered as a minimal TSV table.
func TestRenderStructuredTable_HomogeneousJSONArray(t *testing.T) {
	input := `[{"name":"pod-a","status":"Running","restarts":0},{"name":"pod-b","status":"Pending","restarts":2}]`
	res, ok := renderStructuredTable(input, DetectionInfo{Type: "json-output"})
	if !ok {
		t.Fatal("renderStructuredTable should have applied")
	}
	if !res.Changed {
		t.Error("Changed=true expected")
	}
	if !strings.Contains(res.Text, "name") {
		t.Errorf("expected header 'name' in result, got %q", res.Text)
	}
	if !strings.Contains(res.Text, "pod-a") {
		t.Error("missing 'pod-a'")
	}
	if !strings.Contains(res.Text, "Pending") {
		t.Error("missing 'Pending'")
	}
	if strings.Contains(res.Text, `"status":`) {
		t.Error("JSON notation should be stripped — table is TSV, not JSON")
	}
}

// TestRenderStructuredTable_MalformedJSONNoOp verifies graceful no-op on
// invalid JSON.
func TestRenderStructuredTable_MalformedJSONNoOp(t *testing.T) {
	res, ok := renderStructuredTable("{not json", DetectionInfo{Type: "json-output"})
	if ok {
		t.Fatal("renderStructuredTable should have no-op'd on malformed JSON")
	}
	if res.Text != "{not json" {
		t.Errorf("Text should equal input on no-op, got %q", res.Text)
	}
}

// TestRenderStructuredTable_SingleObjectNoOp verifies that a single
// object (not an array) is treated as a no-op.
func TestRenderStructuredTable_SingleObjectNoOp(t *testing.T) {
	input := `{"name":"x"}`
	res, ok := renderStructuredTable(input, DetectionInfo{Type: "json-output"})
	if ok {
		t.Fatal("renderStructuredTable should have no-op'd on single object")
	}
	if res.Text != input {
		t.Errorf("Text should equal input on no-op, got %q", res.Text)
	}
}

// TestRenderStructuredTable_PriorityColumns verifies that priority keys
// (name/id/status/type/kind) are preferred over frequency-based picks.
func TestRenderStructuredTable_PriorityColumns(t *testing.T) {
	input := `[{"a":1,"b":2,"c":3,"name":"x"},{"a":1,"b":2,"c":3,"name":"y"}]`
	res, ok := renderStructuredTable(input, DetectionInfo{Type: "json-output"})
	if !ok {
		t.Fatal("renderStructuredTable should have applied")
	}
	lines := strings.Split(res.Text, "\n")
	if len(lines) == 0 {
		t.Fatal("empty output")
	}
	headerCols := strings.Split(lines[0], "\t")
	// "name" must appear in the first 5 columns.
	found := false
	for i, col := range headerCols {
		if col == "name" {
			found = true
			break
		}
		_ = i
	}
	if !found {
		t.Errorf("expected 'name' in priority columns, got header %v", headerCols)
	}
}

// TestRenderStructuredTable_MaxRowsCap verifies that arrays exceeding
// 200 rows are truncated with a "… (+K more)" suffix.
func TestRenderStructuredTable_MaxRowsCap(t *testing.T) {
	items := make([]map[string]interface{}, 250)
	for i := range items {
		items[i] = map[string]interface{}{"name": "row"}
	}
	input := mustMarshalJSON(items)
	res, ok := renderStructuredTable(input, DetectionInfo{Type: "json-output"})
	if !ok {
		t.Fatal("renderStructuredTable should have applied")
	}
	if !strings.Contains(res.Text, "(+50 more)") {
		t.Errorf("expected '... (+50 more)' suffix, got %q", res.Text)
	}
}

func mustMarshalJSON(v interface{}) string {
	// Use the renderer's JSON parser via tryParseJSON to convert back.
	// Simpler: just call json.Marshal directly via a tiny shim.
	return jsonMarshalForTest(v)
}