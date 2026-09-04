package catalog

import (
	"strings"
	"testing"
)

func TestFilterNoPatterns(t *testing.T) {
	in := []Model{
		{ID: "a/b"},
		{ID: "c/d"},
	}
	got := Filter(in, nil)
	if len(got) != 2 {
		t.Errorf("no-patterns filter should return input unchanged: got %d", len(got))
	}
}

func TestFilterMatches(t *testing.T) {
	in := []Model{
		{ID: "minimax/MiniMax-M2.1"},
		{ID: "sensenova/glm-5.2"},
		{ID: "ali/deepseek-v4-flash"},
		{ID: "opencode/big-pickle"},
	}
	got := Filter(in, []string{"minimax", "ali"})
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	gotIDs := []string{got[0].ID, got[1].ID}
	if !contains(gotIDs, "minimax/MiniMax-M2.1") || !contains(gotIDs, "ali/deepseek-v4-flash") {
		t.Errorf("unexpected matches: %v", gotIDs)
	}
}

func TestFilterEmptyPatternsIgnored(t *testing.T) {
	got := Filter([]Model{{ID: "x/y"}}, []string{"", "  ", "x"})
	if len(got) != 1 {
		t.Errorf("empty pattern entries should be ignored: got %d", len(got))
	}
}

func TestSummarize(t *testing.T) {
	if !strings.HasSuffix(summarize([]byte("hi")), "hi") {
		t.Errorf("summarize short: %q", summarize([]byte("hi")))
	}
	long := strings.Repeat("a", 500)
	got := summarize([]byte(long))
	if len(got) >= len(long) {
		t.Errorf("summarize should truncate: got %d >= %d", len(got), len(long))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("summarize long should end with ellipsis: %q", got)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
