package renderers

import (
	"strings"
	"testing"
)

// TestRenderGitDiff_KeepsHeadersHunksAndChanges verifies the git-diff
// renderer keeps file headers, hunk headers, and +/- change lines while
// dropping context lines and metadata. Mirrors OmniRoute
// rtk-render-gitdiff.test.ts.
func TestRenderGitDiff_KeepsHeadersHunksAndChanges(t *testing.T) {
	input := `diff --git a/x.ts b/x.ts
index 111..222 100644
--- a/x.ts
+++ b/x.ts
@@ -1,3 +1,3 @@
-const a = 1;
+const a = 2;
 const b = 3;`
	res, ok := renderGitDiff(input, DetectionInfo{Type: "git-diff"})
	if !ok {
		t.Fatal("renderGitDiff should have applied")
	}
	if !res.Changed {
		t.Fatal("Changed=true expected")
	}
	if res.Renderer != "git-diff" {
		t.Errorf("Renderer = %q, want %q", res.Renderer, "git-diff")
	}
	if !strings.Contains(res.Text, "diff --git a/x.ts b/x.ts") {
		t.Error("missing diff header")
	}
	if !strings.Contains(res.Text, "@@ -1,3 +1,3 @@") {
		t.Error("missing hunk header")
	}
	if !strings.Contains(res.Text, "-const a = 1;") {
		t.Error("missing -change line")
	}
	if !strings.Contains(res.Text, "+const a = 2;") {
		t.Error("missing +change line")
	}
	if strings.Contains(res.Text, "index 111..222") {
		t.Error("index line should be dropped")
	}
	if strings.Contains(res.Text, "--- a/x.ts") {
		t.Error("--- a/ header should be dropped")
	}
	if strings.Contains(res.Text, " const b = 3;") {
		t.Error("context line should be dropped")
	}
}

// TestRenderGitDiff_NoHunksNoOp verifies the renderer no-ops on input
// that has no @@ header.
func TestRenderGitDiff_NoHunksNoOp(t *testing.T) {
	res, ok := renderGitDiff("just some text\nno diff here", DetectionInfo{Type: "git-diff"})
	if ok {
		t.Fatal("renderGitDiff should have returned no-op (no @@ header)")
	}
	if res.Changed {
		t.Error("Changed=false expected for no-op")
	}
	if res.Text != "just some text\nno diff here" {
		t.Errorf("Text should equal input, got %q", res.Text)
	}
}

// TestRenderGitDiff_ExcludesTriplePlusMinus verifies that `+++` and `---`
// file markers are not confused with change lines.
func TestRenderGitDiff_ExcludesTriplePlusMinus(t *testing.T) {
	input := `diff --git a/x b/x
+++ b/x
@@ -1 +1 @@
-old
+new`
	res, ok := renderGitDiff(input, DetectionInfo{Type: "git-diff"})
	if !ok {
		t.Fatal("renderGitDiff should have applied")
	}
	if strings.Contains(res.Text, "+++ b/x") {
		t.Error("+++ b/x file marker should be dropped")
	}
	if !strings.Contains(res.Text, "+new") {
		t.Error("+new change line should be kept")
	}
}