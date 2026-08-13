package schemas

import (
	"context"
	"fmt"
)

const defaultVaultPrefix = "bifrost"

// VaultPrefix returns the configured vault path prefix, defaulting to "bifrost".
func VaultPrefix() string {
	return defaultVaultPrefix
}

// VaultBasePath returns the standard vault path prefix for a table row.
func VaultBasePath(tableName, primaryKey string) string {
	return fmt.Sprintf("%s/%s/%s", VaultPrefix(), tableName, primaryKey)
}

// LookupVault resolves a vault reference string (e.g. "vault.path/to/secret").
// OSS deployments never wire a resolver, so lookups always return not-found.
func LookupVault(ref string) (string, bool) {
	return "", false
}

// VaultPathKeyer is implemented by GORM models that own vault secrets. The
// global vault callback uses VaultPathKey() (together with the table name) to
// build the base path for auto-store and auto-remove, so individual models do
// not need to wire StoreOwnedVaultSecretVars / RemoveOwnedVaultSecretVars manually.
type VaultPathKeyer interface {
	VaultPathKey() string
}

// RemoveOwnedVaultSecretVars is a no-op in OSS deployments: no vault
// remove hook is registered, so there is nothing to delete. It exists to
// keep the API surface stable for models that call it in BeforeSave hooks.
func RemoveOwnedVaultSecretVars(ctx context.Context, ownedPrefix string, model interface{}) []error {
	return nil
}

// StoreVaultSecretVar is a no-op in OSS deployments: no vault store hook is
// registered, so plaintext values are never pushed to a vault and never
// converted to vault references. The secret field is left unchanged.
func StoreVaultSecretVar(ctx context.Context, path string, e *SecretVar) error {
	return nil
}

// StoreOwnedVaultSecretVars is a no-op in OSS deployments: no vault store
// hook is registered, so model fields are never pushed to a vault. It exists
// to keep the API surface stable for models that call it in BeforeSave hooks.
func StoreOwnedVaultSecretVars(ctx context.Context, basePath string, model interface{}) error {
	return nil
}
