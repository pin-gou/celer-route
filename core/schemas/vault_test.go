package schemas

import (
	"context"
	"testing"
)

// TestStoreVaultSecretVar_StoresPlaintext asserts that in OSS the vault store
// surface is a pure no-op: StoreVaultSecretVar never rewrites the value into a
// vault reference (the enterprise VaultStoreHook that performed the store has
// been removed, see vaultstrip_test.go).
func TestStoreVaultSecretVar_StoresPlaintext(t *testing.T) {
	e := &SecretVar{Val: "secret-key"}
	if err := StoreVaultSecretVar(context.Background(), "bifrost/tbl/id/value", e); err != nil {
		t.Fatalf("StoreVaultSecretVar: %v", err)
	}
	if e.GetValue() != "secret-key" || e.GetRawRef() != "" || e.IsFromVault() {
		t.Errorf("StoreVaultSecretVar mutated OSS value: val=%q ref=%q fromVault=%v", e.GetValue(), e.GetRawRef(), e.IsFromVault())
	}
}

func TestStoreVaultSecretVar_NoOps(t *testing.T) {
	cases := []struct {
		name string
		e    *SecretVar
	}{
		{"nil", nil},
		{"env-sourced", &SecretVar{ref: "env.MY_VAR", SecretType: SecretTypeEnv}},
		{"already-vault", &SecretVar{Val: "vault.some/path", ref: "vault.some/path", SecretType: SecretTypeVault}},
		{"empty", &SecretVar{Val: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := StoreVaultSecretVar(context.Background(), "bifrost/tbl/id/f", tc.e); err != nil {
				t.Fatalf("StoreVaultSecretVar: %v", err)
			}
		})
	}
}

func TestStoreVaultSecretVar_NoHookNoOp(t *testing.T) {
	e := &SecretVar{Val: "secret"}
	if err := StoreVaultSecretVar(context.Background(), "p", e); err != nil {
		t.Fatalf("StoreVaultSecretVar: %v", err)
	}
	if e.IsFromSecret() || e.GetRawRef() != "" || e.Val != "secret" {
		t.Errorf("expected no mutation, got val=%q ref=%q fromSecret=%v", e.Val, e.GetRawRef(), e.IsFromSecret())
	}
}

func TestRemoveOwnedVaultSecretVars_SkipsFragmentRefs(t *testing.T) {
	type model struct {
		Normal   SecretVar `gorm:"column:normal"`
		Fragment SecretVar `gorm:"column:fragment"`
	}
	m := &model{
		Normal:   SecretVar{Val: "vault.bifrost/m/1/normal", ref: "vault.bifrost/m/1/normal", SecretType: SecretTypeVault},
		Fragment: SecretVar{Val: "vault.external/db#apiKey", ref: "vault.external/db#apiKey", SecretType: SecretTypeVault},
	}

	// In OSS there is no vault to remove from, so the call is a no-op.
	if errs := RemoveOwnedVaultSecretVars(context.Background(), "bifrost/m/1", m); len(errs) > 0 {
		t.Errorf("RemoveOwnedVaultSecretVars returned errors: %v", errs)
	}
}

func TestStoreOwnedVaultSecretVars_WalksFields(t *testing.T) {
	type model struct {
		Plain    SecretVar  `gorm:"column:plain_col"`
		Ptr      *SecretVar `gorm:"column:ptr_col"`
		NilPtr   *SecretVar
		Snake    SecretVar // no gorm tag -> snake_case of field name
		Ignored  string
		EnvBased SecretVar `gorm:"column:env_col"`
	}
	m := &model{
		Plain:    SecretVar{Val: "p1"},
		Ptr:      &SecretVar{Val: "p2"},
		Snake:    SecretVar{Val: "p3"},
		EnvBased: SecretVar{ref: "env.X", SecretType: SecretTypeEnv},
	}

	// In OSS no field is ever pushed to a vault.
	if err := StoreOwnedVaultSecretVars(context.Background(), "bifrost/m/1", m); err != nil {
		t.Fatalf("StoreOwnedVaultSecretVars: %v", err)
	}
	if m.Plain.IsFromVault() || m.Ptr.IsFromVault() || m.Snake.IsFromVault() {
		t.Error("SecretVar fields must stay plaintext in OSS")
	}
	if m.Plain.GetValue() != "p1" || m.Ptr.GetValue() != "p2" || m.Snake.GetValue() != "p3" {
		t.Error("SecretVar field values must be unchanged in OSS")
	}
}

func TestStoreOwnedVaultSecretVars_WalksMap(t *testing.T) {
	type model struct {
		Headers map[string]SecretVar `gorm:"column:headers"`
	}
	m := &model{
		Headers: map[string]SecretVar{
			"Authorization": {Val: "secret-token"},
			"X-Env":         SecretVar{ref: "env.X", SecretType: SecretTypeEnv},
		},
	}

	// In OSS no map entry is ever pushed to a vault.
	if err := StoreOwnedVaultSecretVars(context.Background(), "bifrost/m/1", m); err != nil {
		t.Fatalf("StoreOwnedVaultSecretVars: %v", err)
	}
	if auth := m.Headers["Authorization"]; auth.IsFromVault() || auth.GetValue() != "secret-token" {
		t.Errorf("map entry mutated in OSS: val=%q fromVault=%v", auth.GetValue(), auth.IsFromVault())
	}
}

func TestRemoveOwnedVaultSecretVars_WalksMap(t *testing.T) {
	type model struct {
		Headers map[string]SecretVar `gorm:"column:headers"`
	}
	m := &model{
		Headers: map[string]SecretVar{
			"Owned":    SecretVar{Val: "vault.bifrost/m/1/headers/Owned", ref: "vault.bifrost/m/1/headers/Owned", SecretType: SecretTypeVault},
			"External": SecretVar{Val: "vault.external/db#key", ref: "vault.external/db#key", SecretType: SecretTypeVault},
		},
	}

	// In OSS there is no vault to remove from, so the call is a no-op.
	if errs := RemoveOwnedVaultSecretVars(context.Background(), "bifrost/m/1", m); len(errs) > 0 {
		t.Errorf("RemoveOwnedVaultSecretVars returned errors: %v", errs)
	}
}