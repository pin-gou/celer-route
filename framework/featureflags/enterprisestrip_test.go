package featureflags

import (
	"os"
	"regexp"
	"testing"
)

// enterpriseOnlySymbols are the enterprise-only identifiers that dev.framework
// task 7.1-7.3 deletes from framework/featureflags/ (see
// .pg/changes/strip-enterprise-features/design.md "framework/featureflags/").
//
// After stripping, none of these identifiers should appear in the production
// source files (featureflags.go, registry.go).
var enterpriseOnlySymbols = []string{
	"EnterpriseOnly",          // FlagDef field + statusForLocked field ref
	"ErrFlagEnterpriseOnly",   // error constant
	"SyncDelegate",            // interface type
	"isEnterprise",            // Store field + branch guard
	"SetDelegate",             // Store method
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

// TestStripEnterpriseOnlyFieldRemovedFromFlagDef asserts that the
// EnterpriseOnly field has been removed from the FlagDef struct in
// registry.go (dev.framework task 7.1).
func TestStripEnterpriseOnlyFieldRemovedFromFlagDef(t *testing.T) {
	src := readSource(t, "registry.go")
	if found := identsPresentInSource(src, []string{"EnterpriseOnly"}); len(found) > 0 {
		t.Fatalf("EnterpriseOnly still present in registry.go: %v", found)
	}
}

// TestStripErrFlagEnterpriseOnlyRemoved asserts that the
// ErrFlagEnterpriseOnly error constant has been deleted from
// featureflags.go (dev.framework task 7.1).
func TestStripErrFlagEnterpriseOnlyRemoved(t *testing.T) {
	src := readSource(t, "featureflags.go")
	if found := identsPresentInSource(src, []string{"ErrFlagEnterpriseOnly"}); len(found) > 0 {
		t.Fatalf("ErrFlagEnterpriseOnly still present in featureflags.go: %v", found)
	}
}

// TestStripSyncDelegateRemoved asserts that the SyncDelegate interface,
// Store.delegate field, Store.SetDelegate method, and isEnterprise field
// have all been deleted from featureflags.go (dev.framework task 7.3).
func TestStripSyncDelegateRemoved(t *testing.T) {
	src := readSource(t, "featureflags.go")
	if found := identsPresentInSource(src, []string{"SyncDelegate", "SetDelegate", "isEnterprise"}); len(found) > 0 {
		t.Fatalf("enterprise-only SyncDelegate artifacts still present in featureflags.go: %v", found)
	}
}

// TestStripNoEnterpriseBranchInRegistry asserts that registry.go does not
// contain an enterprise-only branch (e.g. def.EnterpriseOnly check) in
// Register, LookupDef, or other functions (dev.framework task 7.2).
func TestStripNoEnterpriseBranchInRegistry(t *testing.T) {
	src := readSource(t, "registry.go")
	// The enterprise-only branch in the pre-strip code referenced
	// EnterpriseOnly inside the registry. After stripping, neither
	// EnterpriseOnly nor any enterprise mention should remain.
	if found := identsPresentInSource(src, []string{"EnterpriseOnly", "enterprise"}); len(found) > 0 {
		t.Fatalf("enterprise-only branch artifacts still present in registry.go: %v", found)
	}
}

// TestStripAllEnterpriseSymbolsAbsentFromProductionSources is a
// comprehensive check that no enterprise-only identifier from the known
// list appears in any production source file under featureflags/.
func TestStripAllEnterpriseSymbolsAbsentFromProductionSources(t *testing.T) {
	files := []string{"featureflags.go", "registry.go"}
	for _, f := range files {
		src := readSource(t, f)
		if found := identsPresentInSource(src, enterpriseOnlySymbols); len(found) > 0 {
			t.Errorf("%s still contains enterprise-only symbols: %v", f, found)
		}
	}
}