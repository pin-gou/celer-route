package routing

import (
	"sort"
	"testing"
)

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestExtractModelLiterals_CEL(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []string
	}{
		{
			name: "empty",
			expr: "",
			want: nil,
		},
		{
			name: "single equality",
			expr: `model == "pg-expert"`,
			want: []string{"pg-expert"},
		},
		{
			name: "in list",
			expr: `model in ["pg-expert", "pg-associate", "pg-master"]`,
			want: []string{"pg-expert", "pg-associate", "pg-master"},
		},
		{
			name: "not-equal is ignored",
			expr: `model != "pg-expert"`,
			want: nil,
		},
		{
			name: "startsWith is ignored",
			expr: `model.startsWith("pg-")`,
			want: nil,
		},
		{
			name: "contains is ignored",
			expr: `model.contains("expert")`,
			want: nil,
		},
		{
			name: "matches is ignored",
			expr: `model.matches("^pg-.*$")`,
			want: nil,
		},
		{
			name: "compound keeps only forward literals",
			expr: `(model == "pg-expert" || model.startsWith("foo-")) && headers["x-team"] == "blue"`,
			want: []string{"pg-expert"},
		},
		{
			name: "de-duplicate",
			expr: `model == "pg-expert" || model == "pg-expert" || model in ["pg-expert", "pg-master"]`,
			want: []string{"pg-expert", "pg-master"},
		},
		{
			name: "non-model fields ignored",
			expr: `request_type == "chat_completion" && model == "pg-expert"`,
			want: []string{"pg-expert"},
		},
		{
			name: "model as substring of another identifier is ignored",
			expr: `modelname == "x"`,
			want: nil,
		},
		{
			name: "model in headers is not a model literal",
			expr: `"model" in headers`,
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractModelLiterals(c.expr, "")
			gotSorted := sortedCopy(got)
			wantSorted := sortedCopy(c.want)
			if !equalSlices(gotSorted, wantSorted) {
				t.Fatalf("expr %q: got %v want %v", c.expr, gotSorted, wantSorted)
			}
		})
	}
}

func TestExtractModelLiterals_Query(t *testing.T) {
	cases := []struct {
		name string
		json string
		want []string
	}{
		{
			name: "empty",
			json: "",
			want: nil,
		},
		{
			name: "single rule == literal",
			json: `{"combinator":"and","rules":[{"field":"model","operator":"==","value":"pg-expert"}]}`,
			want: []string{"pg-expert"},
		},
		{
			name: "in list literal",
			json: `{"combinator":"and","rules":[{"field":"model","operator":"in","value":["a","b","c"]}]}`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "startsWith ignored",
			json: `{"combinator":"and","rules":[{"field":"model","operator":"startsWith","value":"pg-"}]}`,
			want: nil,
		},
		{
			name: "non-model field ignored",
			json: `{"combinator":"and","rules":[{"field":"request_type","operator":"==","value":"chat_completion"},{"field":"model","operator":"==","value":"pg-expert"}]}`,
			want: []string{"pg-expert"},
		},
		{
			name: "nested groups",
			json: `{"combinator":"and","rules":[{"combinator":"or","rules":[{"field":"model","operator":"==","value":"a"},{"field":"model","operator":"==","value":"b"}]},{"field":"model","operator":"==","value":"c"}]}`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "malformed JSON is ignored",
			json: `{`,
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractModelLiterals("", c.json)
			gotSorted := sortedCopy(got)
			wantSorted := sortedCopy(c.want)
			if !equalSlices(gotSorted, wantSorted) {
				t.Fatalf("json %s: got %v want %v", c.json, gotSorted, wantSorted)
			}
		})
	}
}

func TestExtractModelLiterals_CombinedSources(t *testing.T) {
	cel := `model == "from-cel"`
	q := `{"combinator":"and","rules":[{"field":"model","operator":"==","value":"from-query"}]}`
	got := sortedCopy(ExtractModelLiterals(cel, q))
	want := []string{"from-cel", "from-query"}
	if !equalSlices(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
