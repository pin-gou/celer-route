package catalog

import (
	"strings"
	"testing"
)

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
