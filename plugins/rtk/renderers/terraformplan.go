package renderers

import (
	"regexp"
	"strings"

)

// terraformPlanSummaryRe extracts `Plan: N to add, M to change, K to destroy`
// from the canonical terraform/tofu plan summary line.
var terraformPlanSummaryRe = regexp.MustCompile(`Plan:\s+(\d+)\s+to add,\s+(\d+)\s+to change,\s+(\d+)\s+to destroy`)

// terraformPlanResourceRe matches resource address lines like
// `  # aws_instance.web will be created` (or "will be updated in-place",
// "will be destroyed", etc.).
var terraformPlanResourceRe = regexp.MustCompile(`^\s+#\s+(\S+)\s+will\s+be\s+(\S+(?:\s+\S+)*?)\s*$`)

// renderTerraformPlan is the RTK semantic renderer for `terraform plan`
// and `tofu plan` output. It extracts:
//
//   - The canonical summary line `Plan: N to add, M to change, K to destroy`
//     reformatted as `Plan: +N ~M -K`
//   - Resource address lines `# <addr> will be <verb>`
//
// "No changes." is a no-op (already compact). If no `Plan:` line is found,
// the renderer no-ops (conservative).
//
// Aligned with OmniRoute's renderers/terraformPlan.ts.
func renderTerraformPlan(text string, _ DetectionInfo) (RenderResult, bool) {
	// "No changes" is already compact — idempotent no-op.
	if matched, _ := regexp.MatchString(`^No changes\.`, text); matched {
		return NoRender(text)
	}

	match := terraformPlanSummaryRe.FindStringSubmatch(text)
	if match == nil {
		return NoRender(text)
	}
	add, change, destroy := match[1], match[2], match[3]
	summary := "Plan: +" + add + " ~" + change + " -" + destroy

	// Collect resource address lines.
	var resources []string
	for _, line := range strings.Split(text, "\n") {
		m := terraformPlanResourceRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		addr := m[1]
		verb := m[2]
		// Avoid the duplicate "will be will be ..." regression by checking
		// the verb does not already start with "will be".
		if strings.HasPrefix(verb, "will be ") {
			verb = strings.TrimPrefix(verb, "will be ")
		}
		resources = append(resources, "  # "+addr+" will be "+verb)
	}

	out := summary
	if len(resources) > 0 {
		out += "\n" + strings.Join(resources, "\n")
	}
	return RenderResult{Text: out, Changed: true, Renderer: "terraform-plan"}, true
}
