package configstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/pin-gou/celer-route/framework/configstore/tables"
	"github.com/pin-gou/celer-route/framework/encrypt"
	"gorm.io/gorm"
)

const (
	encryptionStatusPlainText  = "plain_text"
	encryptionStatusEncrypted  = "encrypted"
	defaultEncryptionBatchSize = 100
)

// Per-table predicates identifying rows that actually carry a plaintext secret.
// A row whose sensitive column is empty has nothing to migrate, so it must be
// neither counted by CountPlaintextRows nor selected by the encryptPlaintext*
// helpers — otherwise `re-encrypt` reports work it cannot do and its "after"
// count never reaches zero, which is exactly the misleading output an operator
// has to be able to trust. The counter and the migration both read these
// constants, so the two can never drift apart.
const (
	sensitiveFilterVirtualKeyValue = "value != ''"
	sensitiveFilterSessionToken    = "token != ''"
	sensitiveFilterTempToken       = "token != ''"
	sensitiveFilterOAuthSecret     = "client_secret != ''"
	sensitiveFilterProviderProxy   = "proxy_config_json != '' AND proxy_config_json IS NOT NULL"
	sensitiveFilterVectorStoreCfg  = "config IS NOT NULL AND config != ''"
	sensitiveFilterPluginCfg       = "config_json != '' AND config_json != '{}'"
)

// sensitiveTableSpec pairs a table with the predicate that identifies its
// plaintext payload. An empty payload predicate means every plaintext row on
// the table carries data worth migrating.
type sensitiveTableSpec struct {
	name    string
	model   any
	payload string
}

// sensitiveTables is the single source of truth for which tables participate in
// the plaintext→encrypted migration and how their payload is detected. Counts
// are keyed by name (matching TableName() on each model) so error messages can
// be cross-referenced against the source of truth.
var sensitiveTables = []sensitiveTableSpec{
	{"config_keys", &tables.TableKey{}, ""},
	{"governance_virtual_keys", &tables.TableVirtualKey{}, sensitiveFilterVirtualKeyValue},
	{"sessions", &tables.SessionsTable{}, sensitiveFilterSessionToken},
	{"temp_tokens", &tables.TempToken{}, sensitiveFilterTempToken},
	{"mcp_oauth_tokens", &tables.TableMCPOauthToken{}, ""},
	{"oauth_configs", &tables.TableOauthConfig{}, sensitiveFilterOAuthSecret},
	{"config_mcp_clients", &tables.TableMCPClient{}, ""},
	{"config_providers", &tables.TableProvider{}, sensitiveFilterProviderProxy},
	{"config_vector_store", &tables.TableVectorStoreConfig{}, sensitiveFilterVectorStoreCfg},
	{"config_plugins", &tables.TablePlugin{}, sensitiveFilterPluginCfg},
}

// plaintextWhere builds the WHERE clause selecting rows that still need
// encryption: the status predicate, AND-ed with the table's payload predicate
// when one is defined. Always pass encryptionStatusPlainText as the sole
// placeholder argument.
func plaintextWhere(payloadPredicate string) string {
	base := "(encryption_status = ? OR encryption_status IS NULL OR encryption_status = '')"
	if payloadPredicate == "" {
		return base
	}
	return base + " AND " + payloadPredicate
}

// encryptionBatchSize is the per-transaction cap for the plaintext→encrypted
// migration. It is a var (not a const) so ReencryptPlaintextRows can swap in
// a caller-supplied batch size for a single run and restore on exit; the
// startup path keeps using the default.
var encryptionBatchSize = defaultEncryptionBatchSize

// EncryptPlaintextRows encrypts all rows with encryption_status='plain_text'
// across all sensitive tables. Called during startup when encryption is enabled.
// Each table's GORM BeforeSave hook handles the actual encryption.
func (s *RDBConfigStore) EncryptPlaintextRows(ctx context.Context) error {
	if !encrypt.IsEnabled() {
		return nil
	}

	var totalEncrypted int

	// config_keys
	count, err := s.encryptPlaintextKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt config_keys: %w", err)
	}
	totalEncrypted += count

	// governance_virtual_keys
	count, err = s.encryptPlaintextVirtualKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt virtual_keys: %w", err)
	}
	totalEncrypted += count

	// sessions
	count, err = s.encryptPlaintextSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt sessions: %w", err)
	}
	totalEncrypted += count

	// temp_tokens
	count, err = s.encryptPlaintextTempTokens(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt temp_tokens: %w", err)
	}
	totalEncrypted += count

	// mcp_oauth_tokens
	count, err = s.encryptPlaintextOAuthTokens(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt mcp_oauth_tokens: %w", err)
	}
	totalEncrypted += count

	// oauth_configs
	count, err = s.encryptPlaintextOAuthConfigs(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt oauth_configs: %w", err)
	}
	totalEncrypted += count

	// config_mcp_clients
	count, err = s.encryptPlaintextMCPClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt mcp_clients: %w", err)
	}
	totalEncrypted += count

	// config_providers (proxy config)
	count, err = s.encryptPlaintextProviderProxies(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt provider proxy configs: %w", err)
	}
	totalEncrypted += count

	// config_vector_store
	count, err = s.encryptPlaintextVectorStoreConfigs(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt vector_store configs: %w", err)
	}
	totalEncrypted += count

	// config_plugins
	count, err = s.encryptPlaintextPlugins(ctx)
	if err != nil {
		return fmt.Errorf("failed to encrypt plugin configs: %w", err)
	}
	totalEncrypted += count

	if totalEncrypted > 0 && s.logger != nil {
		s.logger.Info(fmt.Sprintf("encrypted %d plaintext rows across all tables", totalEncrypted))
	}

	return nil
}

// CountPlaintextRows walks every sensitive table and returns the per-table
// count of rows whose encryption_status is plain_text (or NULL/empty — these
// are pre-policy rows) AND whose sensitive payload is non-empty. A row with an
// empty payload is not counted: there is nothing for `re-encrypt` to migrate,
// so including it would make the dry-run overstate the work and leave a
// residual "after" count. Used by the re-encrypt --dry-run path and the
// startup guard so operators see the migration size before they pull the
// trigger.
func (s *RDBConfigStore) CountPlaintextRows(ctx context.Context) (PlaintextRowCounts, error) {
	counts := PlaintextRowCounts{}
	for _, t := range sensitiveTables {
		var n int64
		if err := s.DB().WithContext(ctx).
			Model(t.model).
			Where(plaintextWhere(t.payload), encryptionStatusPlainText).
			Count(&n).Error; err != nil {
			return nil, fmt.Errorf("count plaintext rows in %s: %w", t.name, err)
		}
		if n > 0 {
			counts[t.name] = n
		}
	}
	return counts, nil
}

// ReencryptPlaintextRows runs a one-shot plaintext→encrypted migration with
// caller-controlled batch size and an optional dry-run. It is the engine of
// the `celer-route-admin re-encrypt` command. The function is a thin wrapper
// over EncryptPlaintextRows + CountPlaintextRows, returning both before/after
// counts so the CLI can render a meaningful summary.
//
// Future ReencryptModeRotateKey support plugs in here without changing the
// public signature; today the rotate-key mode returns ErrReencryptModeUnsupported
// so callers fail fast instead of silently doing nothing.
func (s *RDBConfigStore) ReencryptPlaintextRows(ctx context.Context, opts ReencryptOptions) (ReencryptResult, error) {
	mode := opts.Mode
	if mode == "" {
		mode = ReencryptModePlaintextToEncrypted
	}
	if mode != ReencryptModePlaintextToEncrypted {
		return ReencryptResult{}, ErrReencryptModeUnsupported
	}
	if !encrypt.IsEnabled() {
		return ReencryptResult{}, errors.New("re-encrypt refused: encryption key is not set; provide encryption_key in config.json (or BIFROST_ENCRYPTION_KEY) before running re-encrypt")
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = encryptionBatchSize
	}

	before, err := s.CountPlaintextRows(ctx)
	if err != nil {
		return ReencryptResult{}, fmt.Errorf("count plaintext rows before re-encrypt: %w", err)
	}

	if opts.DryRun {
		return ReencryptResult{
			DryRun:          true,
			Mode:            mode,
			BatchSize:       batchSize,
			PlaintextBefore: before,
			Encrypted:       PlaintextRowCounts{}, // no rows were touched
		}, nil
	}

	// Swap in the caller's batch size for this run only. Restore on exit so
	// the next EncryptPlaintextRows call from the startup path keeps the
	// default cadence.
	previous := encryptionBatchSize
	encryptionBatchSize = batchSize
	defer func() { encryptionBatchSize = previous }()

	if err := s.EncryptPlaintextRows(ctx); err != nil {
		return ReencryptResult{}, err
	}
	after, err := s.CountPlaintextRows(ctx)
	if err != nil {
		return ReencryptResult{Mode: mode, BatchSize: batchSize, PlaintextBefore: before}, fmt.Errorf("count plaintext rows after re-encrypt: %w", err)
	}
	encrypted := PlaintextRowCounts{}
	for table, n := range before {
		if delta := n - after[table]; delta > 0 {
			encrypted[table] = delta
		}
	}
	return ReencryptResult{
		Mode:            mode,
		BatchSize:       batchSize,
		PlaintextBefore: before,
		Encrypted:       encrypted,
	}, nil
}

// encryptPlaintextKeys finds all config_keys rows with plaintext encryption status and
// re-saves them in batches. The TableKey.BeforeSave hook handles the actual encryption.
func (s *RDBConfigStore) encryptPlaintextKeys(ctx context.Context) (int, error) {
	var count int
	for {
		var keys []tables.TableKey
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(""), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&keys).Error; err != nil {
			return count, err
		}
		if len(keys) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range keys {
				if err := tx.Save(&keys[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(keys)
	}
	return count, nil
}

// encryptPlaintextVirtualKeys finds all governance_virtual_keys rows with plaintext encryption
// status and re-saves them in batches. The TableVirtualKey.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextVirtualKeys(ctx context.Context) (int, error) {
	var count int
	for {
		var vks []tables.TableVirtualKey
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterVirtualKeyValue), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&vks).Error; err != nil {
			return count, err
		}
		if len(vks) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range vks {
				if err := tx.Save(&vks[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(vks)
	}
	return count, nil
}

// encryptPlaintextSessions finds all sessions rows with plaintext encryption status and
// re-saves them in batches. The SessionsTable.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextSessions(ctx context.Context) (int, error) {
	var count int
	for {
		var sessions []tables.SessionsTable
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterSessionToken), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&sessions).Error; err != nil {
			return count, err
		}
		if len(sessions) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range sessions {
				if err := tx.Save(&sessions[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(sessions)
	}
	return count, nil
}

// encryptPlaintextTempTokens finds all temp_tokens rows with plaintext encryption status
// and re-saves them in batches. The TempToken.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextTempTokens(ctx context.Context) (int, error) {
	var count int
	for {
		var tokens []tables.TempToken
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterTempToken), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&tokens).Error; err != nil {
			return count, err
		}
		if len(tokens) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range tokens {
				if err := tx.Save(&tokens[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(tokens)
	}
	return count, nil
}

// encryptPlaintextOAuthTokens finds all mcp_oauth_tokens rows with plaintext encryption status
// and re-saves them in batches. The TableMCPOauthToken.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextOAuthTokens(ctx context.Context) (int, error) {
	var count int
	for {
		var tokens []tables.TableMCPOauthToken
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(""), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&tokens).Error; err != nil {
			return count, err
		}
		if len(tokens) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range tokens {
				if err := tx.Save(&tokens[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(tokens)
	}
	return count, nil
}

// encryptPlaintextOAuthConfigs finds all oauth_configs rows with plaintext encryption status
// and re-saves them in batches. The TableOauthConfig.BeforeSave hook handles encryption.
// client_secret is the only sensitive column left on this table — state/
// code_verifier/code_challenge/expires_at moved to mcp_oauth_flows (see that
// migration) and code_verifier was the only one of those that was ever
// encrypted, so the WHERE clause below no longer needs an OR branch for it.
func (s *RDBConfigStore) encryptPlaintextOAuthConfigs(ctx context.Context) (int, error) {
	var count int
	for {
		var configs []tables.TableOauthConfig
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterOAuthSecret), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&configs).Error; err != nil {
			return count, err
		}
		if len(configs) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range configs {
				if err := tx.Save(&configs[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(configs)
	}
	return count, nil
}

// encryptPlaintextMCPClients finds all config_mcp_clients rows with plaintext encryption
// status and re-saves them in batches. The TableMCPClient.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextMCPClients(ctx context.Context) (int, error) {
	var count int
	for {
		var clients []tables.TableMCPClient
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(""), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&clients).Error; err != nil {
			return count, err
		}
		if len(clients) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range clients {
				if err := tx.Save(&clients[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(clients)
	}
	return count, nil
}

// encryptPlaintextProviderProxies finds all config_providers rows that have a non-empty
// proxy config with plaintext encryption status and re-saves them in batches. The
// TableProvider.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextProviderProxies(ctx context.Context) (int, error) {
	var count int
	for {
		var providers []tables.TableProvider
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterProviderProxy), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&providers).Error; err != nil {
			return count, err
		}
		if len(providers) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range providers {
				if err := tx.Save(&providers[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(providers)
	}
	return count, nil
}

// encryptPlaintextVectorStoreConfigs finds all config_vector_store rows that have a non-empty
// config with plaintext encryption status and re-saves them in batches. The
// TableVectorStoreConfig.BeforeSave hook handles encryption.
func (s *RDBConfigStore) encryptPlaintextVectorStoreConfigs(ctx context.Context) (int, error) {
	var count int
	for {
		var configs []tables.TableVectorStoreConfig
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterVectorStoreCfg), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&configs).Error; err != nil {
			return count, err
		}
		if len(configs) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range configs {
				if err := tx.Save(&configs[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(configs)
	}
	return count, nil
}

// encryptPlaintextPlugins finds all config_plugins rows that have a non-empty config with
// plaintext encryption status and re-saves them in batches. The TablePlugin.BeforeSave hook
// handles encryption.
func (s *RDBConfigStore) encryptPlaintextPlugins(ctx context.Context) (int, error) {
	var count int
	for {
		var plugins []tables.TablePlugin
		if err := s.DB().WithContext(ctx).
			Where(plaintextWhere(sensitiveFilterPluginCfg), encryptionStatusPlainText).
			Limit(encryptionBatchSize).
			Find(&plugins).Error; err != nil {
			return count, err
		}
		if len(plugins) == 0 {
			break
		}
		if err := s.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range plugins {
				if err := tx.Save(&plugins[i]).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return count, err
		}
		count += len(plugins)
	}
	return count, nil
}
