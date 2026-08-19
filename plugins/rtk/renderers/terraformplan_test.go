package renderers

import (
	"strings"
	"testing"
)

// TestRenderTerraformPlan_SummarizePlusResources verifies the
// Plan/+/~/– summary plus resource address lines.
func TestRenderTerraformPlan_SummarizePlusResources(t *testing.T) {
	input := `Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" { ... many lines ... }

  # aws_s3_bucket.data will be updated in-place
  ~ resource "aws_s3_bucket" "data" { ... }

Plan: 1 to add, 1 to change, 0 to destroy.`
	res, ok := renderTerraformPlan(input, DetectionInfo{Type: "terraform-plan"})
	if !ok {
		t.Fatal("renderTerraformPlan should have applied")
	}
	if !res.Changed {
		t.Error("Changed=true expected")
	}
	if !strings.Contains(res.Text, "Plan: +1 ~1 -0") {
		t.Errorf("expected 'Plan: +1 ~1 -0' in result, got %q", res.Text)
	}
	if !strings.Contains(res.Text, "aws_instance.web") {
		t.Error("expected resource address aws_instance.web")
	}
	if strings.Contains(res.Text, `resource "aws_instance" "web" {`) {
		t.Error("resource body should be dropped")
	}
	// Regression: avoid "will be will be ..." duplication.
	if strings.Contains(res.Text, "will be will be") {
		t.Error("'will be will be' duplication regression")
	}
	if !strings.Contains(res.Text, "# aws_instance.web will be created") {
		t.Error("missing 'will be created' resource line")
	}
	if !strings.Contains(res.Text, "# aws_s3_bucket.data will be updated in-place") {
		t.Error("missing 'will be updated in-place' resource line")
	}
}

// TestRenderTerraformPlan_NoChanges verifies idempotent no-op for
// already-compact "No changes." output.
func TestRenderTerraformPlan_NoChanges(t *testing.T) {
	res, ok := renderTerraformPlan(
		"No changes. Your infrastructure matches the configuration.",
		DetectionInfo{Type: "terraform-plan"},
	)
	if ok {
		t.Fatal("renderTerraformPlan should have returned no-op")
	}
	if res.Text != "No changes. Your infrastructure matches the configuration." {
		t.Errorf("Text should equal input, got %q", res.Text)
	}
}

// TestRenderTerraformPlan_NoPlanLine verifies conservative no-op when no
// canonical Plan summary is present.
func TestRenderTerraformPlan_NoPlanLine(t *testing.T) {
	res, ok := renderTerraformPlan("Random output\nNo plan here", DetectionInfo{Type: "terraform-plan"})
	if ok {
		t.Fatal("renderTerraformPlan should have no-op'd without a Plan line")
	}
	if res.Text != "Random output\nNo plan here" {
		t.Errorf("Text should equal input on no-op, got %q", res.Text)
	}
}