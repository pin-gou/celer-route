package renderers

import (
	"encoding/json"
	"strings"

)

// structuredTableMaxRows caps the number of rows rendered. Larger arrays
// append a `… (+K more)` suffix instead of trimming silently.
const structuredTableMaxRows = 200

// structuredTableMaxColumns caps the number of columns rendered.
const structuredTableMaxColumns = 5

// structuredTablePriorityKeys lists column names that should be preferred
// when choosing which columns to display, regardless of frequency.
var structuredTablePriorityKeys = []string{"name", "id", "status", "type", "kind"}

// renderStructuredTable is the RTK semantic renderer for structured JSON
// array output (e.g. `aws` CLI, `kubectl get -o json`, generic JSON).
//
// Only renders if:
//   - Input parses as JSON (or contains a JSON array substring)
//   - Result is an array of ≥2 homogeneous objects (same dominant scalar
//     keys, threshold ≥ N/2 occurrences)
//
// Output: minimal TSV-like table (header + rows). Large arrays (>200 rows)
// are capped with a trailing `… (+K more)` line. Aligned with OmniRoute's
// renderers/structuredTable.ts.
func renderStructuredTable(text string, _ DetectionInfo) (RenderResult, bool) {
	parsed := tryParseJSON(text)
	if parsed == nil {
		return NoRender(text)
	}

	items, ok := parsed.([]interface{})
	if !ok || len(items) < 2 {
		return NoRender(text)
	}

	// Every item must be a non-null, non-array object.
	objects := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if item == nil {
			return NoRender(text)
		}
		obj, ok := item.(map[string]interface{})
		if !ok {
			// Reject arrays inside arrays.
			if _, isArr := item.([]interface{}); isArr {
				return NoRender(text)
			}
			return NoRender(text)
		}
		objects = append(objects, obj)
	}

	// Collect scalar keys across all objects and count occurrences.
	keyCount := make(map[string]int)
	for _, obj := range objects {
		for k, v := range obj {
			if v == nil {
				continue
			}
			if _, isObj := v.(map[string]interface{}); isObj {
				continue
			}
			if _, isArr := v.([]interface{}); isArr {
				continue
			}
			keyCount[k]++
		}
	}

	if len(keyCount) == 0 {
		return NoRender(text)
	}

	// Choose columns: keys appearing in at least half the rows, sorted by
	// priority (name/id/status/type/kind first) then by frequency.
	threshold := len(objects) / 2
	candidates := make([]string, 0, len(keyCount))
	for k, count := range keyCount {
		if count >= threshold {
			candidates = append(candidates, k)
		}
	}
	if len(candidates) == 0 {
		return NoRender(text)
	}

	priorityChosen := make([]string, 0)
	rest := make([]string, 0)
	for _, k := range candidates {
		if isPriorityKey(k) {
			priorityChosen = append(priorityChosen, k)
		} else {
			rest = append(rest, k)
		}
	}
	// Sort `rest` by descending count for stable output.
	sortByCount(rest, keyCount)

	columns := append(priorityChosen, rest...)
	if len(columns) > structuredTableMaxColumns {
		columns = columns[:structuredTableMaxColumns]
	}
	if len(columns) == 0 {
		return NoRender(text)
	}

	// Build the table.
	rows := objects
	extra := 0
	if len(rows) > structuredTableMaxRows {
		rows = rows[:structuredTableMaxRows]
		extra = len(objects) - structuredTableMaxRows
	}

	var sb strings.Builder
	sb.WriteString(strings.Join(columns, "\t"))
	for _, obj := range rows {
		sb.WriteByte('\n')
		cells := make([]string, len(columns))
		for i, k := range columns {
			cells[i] = stringifyCell(obj[k])
		}
		sb.WriteString(strings.Join(cells, "\t"))
	}
	if extra > 0 {
		sb.WriteString("\n… (+")
		sb.WriteString(itoa(extra))
		sb.WriteString(" more)")
	}

	return RenderResult{Text: sb.String(), Changed: true, Renderer: "structured-table"}, true
}

// tryParseJSON attempts direct JSON.parse on the input. On failure, it
// tries to extract the largest [...] substring and re-parse that. Returns
// nil when neither path succeeds. Mirrors OmniRoute's tryParse helper.
func tryParseJSON(text string) interface{} {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		return v
	}
	// Try to find the largest [...] substring.
	start := strings.Index(trimmed, "[")
	end := strings.LastIndex(trimmed, "]")
	if start != -1 && end > start {
		if err := json.Unmarshal([]byte(trimmed[start:end+1]), &v); err == nil {
			return v
		}
	}
	return nil
}

func isPriorityKey(k string) bool {
	for _, p := range structuredTablePriorityKeys {
		if p == k {
			return true
		}
	}
	return false
}

// sortByCount sorts `keys` in-place by descending value in `counts`. Stable
// sort via secondary alphabetical comparison for determinism.
func sortByCount(keys []string, counts map[string]int) {
	// Insertion sort — the slice is small (≤ 5 elements).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0; j-- {
			ci, cj := counts[keys[j]], counts[keys[j-1]]
			if ci > cj || (ci == cj && keys[j] < keys[j-1]) {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			} else {
				break
			}
		}
	}
}

func stringifyCell(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// Avoid scientific notation for typical integer-ish values.
		return ftoa(x)
	default:
		// Fallback: marshal via JSON.
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// ftoa converts a float64 to its shortest unambiguous decimal string.
func ftoa(f float64) string {
	// Reuse json.Marshal for the integer path; floats fall through.
	if f == float64(int64(f)) {
		return itoa64(int64(f))
	}
	b, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
