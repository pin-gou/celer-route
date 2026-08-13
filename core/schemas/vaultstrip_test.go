package schemas

import (
	"context"
	"os"
	"regexp"
	"testing"
)

// vaultEnterpriseHookIdents are the four vault integration hook globals and
// VaultStoreWriteEnabled that dev.core task 2.2 deletes from vault.go (see
// design.md "后端 6"). After deletion, the OSS vault surface becomes a pure
// no-op: all store/remove/resolve helpers return safely without panic.
var vaultEnterpriseHookIdents = []string{
	"VaultResolveHook",
	"VaultRemoveHook",
	"VaultStoreHook",
	"VaultPrefixHook",
	"VaultStoreWriteEnabled",
}

// vaultHookIdentsPresent returns the subset of hook identifiers whose
// declarations are still present in the vault.go source.
func vaultHookIdentsPresent(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("vault.go")
	if err != nil {
		t.Fatalf("read core/schemas/vault.go: %v", err)
	}
	var found []string
	for _, id := range vaultEnterpriseHookIdents {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`)
		if re.MatchString(string(data)) {
			found = append(found, id)
		}
	}
	return found
}

// TestVaultHookGlobalsRemoved asserts the four enterprise vault hook globals
// and VaultStoreWriteEnabled have been deleted from core/schemas/vault.go
// (dev.core task 2.2).
//
// TDD red phase: the hooks still exist in vault.go, so this FAILS. It will
// PASS once dev.core deletes them.
func TestVaultHookGlobalsRemoved(t *testing.T) {
	if found := vaultHookIdentsPresent(t); len(found) > 0 {
		t.Fatalf("enterprise vault hooks still declared in core/schemas/vault.go: %v", found)
	}
}

// TestVaultConversionsNoOpInOSS asserts that once the enterprise hooks are
// deleted, the OSS vault surface stays a pure no-op: VaultPrefix falls back
// to "bifrost", StoreVaultSecretVar mutates nothing, LookupVault returns
// not-found, and SecretVar.GetValue returns the plaintext — all without panic.
//
// TDD red phase: the first assertion (hooks absent) FAILS now. The behavioral
// assertions below it only run once the absence gate passes (green phase).
func TestVaultConversionsNoOpInOSS(t *testing.T) {
	// Gate: hooks must be removed first.
	if found := vaultHookIdentsPresent(t); len(found) > 0 {
		t.Fatalf("enterprise vault hooks still present in core/schemas/vault.go: %v", found)
	}

	// VaultPrefix falls back to the default "bifrost".
	if got := VaultPrefix(); got != "bifrost" {
		t.Errorf("VaultPrefix() = %q, want default %q", got, "bifrost")
	}

	// StoreVaultSecretVar is a no-op: returns nil, field unchanged.
	e := &SecretVar{Val: "plaintext"}
	if err := StoreVaultSecretVar(context.Background(), "bifrost/tbl/id/value", e); err != nil {
		t.Fatalf("StoreVaultSecretVar no-op returned error: %v", err)
	}
	if e.GetValue() != "plaintext" || e.IsFromSecret() {
		t.Errorf("StoreVaultSecretVar mutated OSS value: val=%q fromSecret=%v", e.GetValue(), e.IsFromSecret())
	}

	// LookupVault returns not-found when no resolver is registered.
	if val, ok := LookupVault("vault.some/path"); ok || val != "" {
		t.Errorf("LookupVault = (%q, %v), want (\"\", false) in OSS", val, ok)
	}

	// SecretVar.GetValue is a plain field accessor, never panics.
	sv := &SecretVar{Val: "hello"}
	if got := sv.GetValue(); got != "hello" {
		t.Errorf("SecretVar.GetValue = %q, want %q", got, "hello")
	}

	// StoreOwnedVaultSecretVars is a no-op when no hook is wired.
	type model struct {
		F SecretVar `gorm:"column:f"`
	}
	m := &model{F: SecretVar{Val: "x"}}
	if err := StoreOwnedVaultSecretVars(context.Background(), "bifrost/tbl/1", m); err != nil {
		t.Fatalf("StoreOwnedVaultSecretVars no-op returned error: %v", err)
	}
	if m.F.GetValue() != "x" || m.F.IsFromSecret() {
		t.Errorf("StoreOwnedVaultSecretVars mutated field: val=%q fromSecret=%v", m.F.GetValue(), m.F.IsFromSecret())
	}

	// RemoveOwnedVaultSecretVars is a no-op when no hook is wired.
	if errs := RemoveOwnedVaultSecretVars(context.Background(), "bifrost/tbl/1", m); len(errs) > 0 {
		t.Errorf("RemoveOwnedVaultSecretVars returned errors: %v", errs)
	}
}