package governance

import (
	"os"
	"regexp"
	"testing"
)

// deletedEnterpriseContextKeys are the enterprise-only BifrostContextKey
// identifiers that dev.core task 2.1 deleted from core/schemas/bifrost.go
// (see .pg/changes/strip-enterprise-features/design.md "后端 3").
//
// dev.plugins:test task 16.1 must confirm that the CheckUserBudget /
// CheckUserRateLimit call sites in this module (resolver.go EvaluateUserRequest
// -> LocalGovernanceStore.CheckUserBudget/CheckUserRateLimit) are not polluted
// by any of these deleted keys (V-plugins-1).
var deletedEnterpriseContextKeys = []string{
	"BifrostContextKeyClusterNodeID",
	"BifrostContextKeyGovernanceBusinessUnitID",
	"BifrostContextKeyGovernanceBusinessUnitIDs",
	"BifrostContextKeyGovernanceBusinessUnitName",
	"BifrostContextKeyGovernanceBusinessUnitNames",
	"BifrostContextKeyGovernanceCustomerIDs",
	"BifrostContextKeyGovernanceCustomerNames",
	"BifrostContextKeyGovernanceScopedCustomerID",
	"BifrostContextKeyGovernanceTeamIDs",
	"BifrostContextKeyGovernanceTeamNames",
	"BifrostContextKeyUserID",
	"BifrostContextKeyUserName",
	"BifrostContextKeyUserEmail",
}

// userGovernanceCallSiteFiles are the files that implement or call
// CheckUserBudget / CheckUserRateLimit (task 16.1 scope).
var userGovernanceCallSiteFiles = []string{
	"store.go",
	"resolver.go",
	"main.go",
	"utils.go",
	"routing.go",
	"tracker.go",
}

func readPackageSource(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func identRegex(s string) string {
	return `\b` + regexp.QuoteMeta(s) + `\b`
}

// TestStrip_UserGovernanceCallSitesNotPollutedByDeletedEnterpriseKeys pins
// task 16.1: none of the files implementing/calling CheckUserBudget /
// CheckUserRateLimit may reference an enterprise-only context key that
// dev.core deleted. This is a regression guard on the state V-plugins-1
// verifies (go build passes only while this holds).
func TestStrip_UserGovernanceCallSitesNotPollutedByDeletedEnterpriseKeys(t *testing.T) {
	for _, file := range userGovernanceCallSiteFiles {
		src := readPackageSource(t, file)
		for _, key := range deletedEnterpriseContextKeys {
			re := regexp.MustCompile(identRegex(key))
			if re.MatchString(src) {
				t.Errorf("%s references deleted enterprise context key %q (task 16.1 / V-plugins-1)", file, key)
			}
		}
	}
}

// TestStrip_MainGoConfigIsEnterpriseRemoved pins task 16.2: after dev.plugins
// (task 17.x) the `is_enterprise` plugin config surface must be gone from
// main.go. Red while the field/reference still exists, green once cleaned.
func TestStrip_MainGoConfigIsEnterpriseRemoved(t *testing.T) {
	src := readPackageSource(t, "main.go")
	for _, ident := range []string{
		"IsEnterprise",  // Config.IsEnterprise field + config.IsEnterprise reads
		"isEnterprise",  // BaseGovernancePlugin internal field + p.isEnterprise writes
		"is_enterprise", // JSON struct tag (cfg keys / config.schema.json sync)
	} {
		re := regexp.MustCompile(identRegex(ident))
		if re.MatchString(src) {
			t.Errorf("main.go still references enterprise config ident %q (task 16.2)", ident)
		}
	}
}

