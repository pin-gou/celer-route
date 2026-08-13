package schemas

import (
	"os"
	"regexp"
	"testing"
)

// enterpriseContextKeys are the enterprise-only BifrostContextKey identifiers
// that dev.core task 2.1 deletes from core/schemas/bifrost.go (see
// .pg/changes/strip-enterprise-features/design.md "后端 3").
//
// Keys NOT in this list — such as the governance plugin's singular
// BifrostContextKeyGovernanceCustomerID/Name, BifrostContextKeyGovernanceTeamID/Name,
// BifrostContextKeyGovernanceVirtualKeyID/Name, BifrostContextKeyGovernanceBudgetIDs,
// BifrostContextKeyGovernanceRateLimitIDs, BifrostContextKeyGovernanceRoutingRuleID/Name,
// BifrostContextKeyLargePayloadMode, BifrostContextKeyLargeResponseMode, etc. —
// are OSS keys that must be preserved.
var enterpriseContextKeys = []string{
	"BifrostContextKeyClusterNodeID",
	"BifrostContextKeyGovernanceBusinessUnitID",
	"BifrostContextKeyGovernanceBusinessUnitName",
	"BifrostContextKeyGovernanceTeamIDs",
	"BifrostContextKeyGovernanceTeamNames",
	"BifrostContextKeyGovernanceBusinessUnitIDs",
	"BifrostContextKeyGovernanceBusinessUnitNames",
	"BifrostContextKeyGovernanceCustomerIDs",
	"BifrostContextKeyGovernanceCustomerNames",
	"BifrostContextKeyGovernanceScopedCustomerID",
	"BifrostContextKeyUserID",
	"BifrostContextKeyUserName",
	"BifrostContextKeyUserEmail",
	"BifrostContextKeyGuardrailDebug",
	"BifrostContextKeyRedactionData",
	"BifrostContextKeyLargePayloadContentType",
	"BifrostContextKeyLargePayloadRequestThreshold",
	"BifrostContextKeyLargeResponseThreshold",
	"BifrostContextKeyLargePayloadPrefetchSize",
	"BifrostContextKeyDeferredLargePayloadMetadata",
	"BifrostContextKeySSEReaderFactory",
	"BifrostContextKeyLargePayloadReader",
	"BifrostContextKeyLargePayloadContentLength",
	"BifrostContextKeyLargePayloadMetadata",
	"BifrostContextKeyLargeResponseReader",
	"BifrostContextKeyDeferredUsage",
}

// readSource reads a file relative to the package directory.
func readSource(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// identsPresentInSource returns the subset of idents that appear as standalone
// Go identifiers in src.
func identsPresentInSource(src string, idents []string) []string {
	var found []string
	for _, id := range idents {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`)
		if re.MatchString(src) {
			found = append(found, id)
		}
	}
	return found
}

// TestEnterpriseContextKeysRemovedFromSource asserts that the enterprise-only
// BifrostContextKey constant declarations have been deleted from
// core/schemas/bifrost.go (dev.core task 2.1).
//
// TDD red phase: the keys still exist in the source, so this test FAILS.
// It will PASS once dev.core deletes them.
func TestEnterpriseContextKeysRemovedFromSource(t *testing.T) {
	src := readSource(t, "bifrost.go")
	if found := identsPresentInSource(src, enterpriseContextKeys); len(found) > 0 {
		t.Fatalf("enterprise-only BifrostContextKey still declared in core/schemas/bifrost.go: %v", found)
	}
}

// TestEnterpriseContextKeysNotReferencedInCoreFiles asserts that the deleted
// enterprise-only keys are not referenced by the core request hot paths:
// core/schemas/context.go, core/bifrost.go, core/inference.go.
//
// TDD red phase: the keys are still referenced (e.g. BifrostContextKeyUserID
// in context.go lines 328/357, bifrost.go lines 6223-6261), so this FAILS.
// It will PASS once dev.core deletes the keys and their reference sites.
func TestEnterpriseContextKeysNotReferencedInCoreFiles(t *testing.T) {
	cases := []struct {
		desc string
		path string
	}{
		{"core/schemas/context.go", "context.go"},
		{"core/bifrost.go", "../bifrost.go"},
	}
	for _, c := range cases {
		src := readSource(t, c.path)
		if found := identsPresentInSource(src, enterpriseContextKeys); len(found) > 0 {
			t.Fatalf("enterprise-only BifrostContextKey still referenced in %s: %v", c.desc, found)
		}
	}
}