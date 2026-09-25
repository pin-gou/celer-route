// Package encrypt provides reversible AES-256-GCM encryption and decryption utilities
// for securing sensitive data like API keys and credentials.
package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/pin-gou/celer-route/core/schemas"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// saltVersionPrefix is the leading byte of an encoded ciphertext that
// tells Decrypt which salt to use when deriving the AES-GCM key. Two
// values are supported today:
//
//   - saltVersionLegacy (0): rows written before salt was configurable.
//     Decrypt falls back to DefaultSalt when this byte is present.
//   - saltVersionCustom (1): rows written after the operator supplied
//     their own encryption_salt. Decrypt uses the operator's salt.
//
// The byte is always written — even by legacy callers — so the format is
// self-describing for any future tooling.
const (
	saltVersionLegacy byte = 0
	saltVersionCustom byte = 1
)

// DefaultSalt is the historical, hardcoded salt that was baked into Init
// before per-deployment salts existed. It is kept here as a fallback so
// deployments that never opt into a custom salt continue to read their
// existing ciphertext without an extra rotation step.
const DefaultSalt = "bifrost-encryption-v1-salt-2024"

// encryptionState holds the derived AES keys, keyed by salt_version. The
// legacy key (version 0) is always populated when Init succeeds with a
// passphrase; the custom key (version 1) is populated only when the
// operator supplies a non-nil salt. Both are 32 bytes (AES-256) and the
// Argon2id parameters match the original implementation exactly so
// existing rows decrypt bit-for-bit when salt_version=0.
type encryptionState struct {
	legacyKey []byte // derived from DefaultSalt
	customKey []byte // derived from operator-provided salt; nil when no custom salt
}

var (
	encryptionStatePtr atomic.Pointer[encryptionState]
	logger             schemas.Logger
)

// allowPlaintextStorage records whether the operator has explicitly opted in
// to storing sensitive columns in plaintext. Phase 6 / D9 changes the default
// behaviour so that unset-opt-in + unset-key is rejected at startup; the policy
// is a process-global atomic so callers can read it lock-free from hot paths.
//
// The default is false (deny plaintext). Call SetAllowPlaintextStorage(true)
// from the configuration loader after parsing config.json / BIFROST_ALLOW_PLAINTEXT_STORAGE.
var allowPlaintextStorage atomic.Bool

// ErrEncryptionKeyNotInitialized is returned by Decrypt when no encryption key
// has been derived via Init. It signals a read against a database whose rows
// were never encrypted; this is recoverable only by either providing the key
// or accepting the row as plaintext via AllowPlaintextStorage + re-encrypt.
var ErrEncryptionKeyNotInitialized = errors.New("encryption key is not initialized")

// ErrPlaintextWriteForbidden is returned by Encrypt when a write is attempted
// while the encryption key is unset AND the operator has not opted in to
// plaintext storage (the D9 default). Callers must surface this to the
// operator as a configuration error rather than swallowing it.
var ErrPlaintextWriteForbidden = errors.New("plaintext storage is not allowed: set encryption_key in config.json (or BIFROST_ENCRYPTION_KEY) or set BIFROST_ALLOW_PLAINTEXT_STORAGE=true to explicitly opt in")

// SetAllowPlaintextStorage updates the operator-driven opt-in flag. Call this
// once during configuration loading; reads from BeforeSave/AfterFind paths use
// AllowPlaintextStorage() which is lock-free.
func SetAllowPlaintextStorage(allowed bool) {
	allowPlaintextStorage.Store(allowed)
}

// AllowPlaintextStorage reports whether the operator has explicitly opted in
// to storing sensitive columns as plaintext when no encryption key is set.
// D9 makes this opt-in (default false).
func AllowPlaintextStorage() bool {
	return allowPlaintextStorage.Load()
}

// Init initializes the encryption key using Argon2id KDF to derive a secure
// 32-byte key from the provided passphrase. The function accepts any
// passphrase but warns if it's too short (< 16 bytes). The historical
// hardcoded DefaultSalt is used for derivation — see InitWithSalt for
// deployments that need a per-instance salt.
//
// The derived key is held in the package-global atomic encryptionState
// under saltVersionLegacy (0); ciphertext written by this Init carries
// that header byte so Decrypt picks the right key when both a legacy and
// a custom-salt key are loaded.
func Init(key string, _logger schemas.Logger) {
	InitWithSalt(key, nil, _logger)
}

// InitWithSalt is the salt-aware variant. Pass nil for salt to keep the
// historical DefaultSalt behaviour (this is what Init does); pass a non-nil
// salt to derive a second key under saltVersionCustom (1) so the same
// passphrase produces different ciphertext per deployment. Decrypt
// dispatches by the leading byte of the encoded ciphertext, so both
// versions coexist in one database.
//
// A passphrase shorter than 16 bytes always logs a warning regardless of
// the salt variant — Argon2id does not compensate for low-entropy input.
func InitWithSalt(key string, customSalt []byte, _logger schemas.Logger) {
	logger = _logger
	if key == "" {
		encryptionStatePtr.Store(nil)
		if !AllowPlaintextStorage() {
			// D9 fail-closed: log loudly so a misconfigured boot leaves a trace
			// in the journal; the server's startup guard converts this into a
			// hard exit once it sees an existing sensitive row.
			logger.Warn("encryption key is not set and BIFROST_ALLOW_PLAINTEXT_STORAGE=false; sensitive rows will be rejected until encryption_key is provided or plaintext storage is explicitly opted in")
		} else {
			logger.Warn("encryption key is not set and BIFROST_ALLOW_PLAINTEXT_STORAGE=true; sensitive rows will be stored in plaintext. To switch to encrypted storage: provide encryption_key (or BIFROST_ENCRYPTION_KEY), restart, and run 'celer-route-admin re-encrypt' to migrate existing rows.")
		}
		return
	}

	// Warn if passphrase is too short
	if len(key) < 16 {
		logger.Warn("encryption passphrase is shorter than 16 bytes, consider using a longer passphrase for better security")
	}

	state := &encryptionState{}

	// Always derive the legacy key with the hardcoded salt. Existing rows
	// (decoded as salt_version=0) keep decrypting without an extra
	// rotation step — see 04-security-enhancements/encryption-hardening.md.
	state.legacyKey = argon2.IDKey([]byte(key), []byte(DefaultSalt), 1, 64*1024, 4, 32)

	// When the operator supplies a salt, derive a second key under it so
	// rows written from this point on carry salt_version=1 and stay
	// cryptographically isolated across deployments.
	if len(customSalt) > 0 {
		state.customKey = argon2.IDKey([]byte(key), customSalt, 1, 64*1024, 4, 32)
	}

	encryptionStatePtr.Store(state)
}

// CompareHash compares a hash and a password
func CompareHash(hash string, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, fmt.Errorf("failed to compare hash: %w", err)
	}
	return true, nil
}

// Hash hashes a password using bcrypt
func Hash(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedPassword), nil
}

// Encrypt encrypts a plaintext string using AES-256-GCM and returns a
// base64-encoded ciphertext. The encoded payload is prefixed with a single
// salt_version byte so Decrypt can pick the right derived key when both
// a legacy and a custom-salt key are loaded (see InitWithSalt).
//
// When the encryption key is unset, behaviour is governed by D9:
//   - Plaintext opt-in (SetAllowPlaintextStorage(true)): the plaintext is returned
//     unchanged so existing dev/test setups keep working.
//   - Default (opt-out): ErrPlaintextWriteForbidden is returned so callers can
//     refuse to write sensitive rows rather than silently leaking them.
//
// Empty plaintext is always returned as the empty string regardless of policy.
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	state := encryptionStatePtr.Load()
	if state == nil {
		if !AllowPlaintextStorage() {
			return "", ErrPlaintextWriteForbidden
		}
		return plaintext, nil
	}

	// Prefer the custom-salt key when both are derived: new writes should
	// land under salt_version=1 so a future rotate-salt command has a
	// distinct target set. When no custom salt was configured, fall back
	// to the legacy key (salt_version=0).
	key := state.customKey
	version := saltVersionCustom
	if key == nil {
		key = state.legacyKey
		version = saltVersionLegacy
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return plaintext, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Create a nonce (number used once).
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext, fmt.Errorf("failed to read nonce: %w", err)
	}

	// Encrypt the data, then prefix the encoded payload with the
	// salt_version byte Decrypt reads on the way back in.
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := make([]byte, 0, 1+len(ciphertext))
	encoded = append(encoded, version)
	encoded = append(encoded, ciphertext...)

	return base64.StdEncoding.EncodeToString(encoded), nil
}

// IsEnabled returns true if the encryption key has been initialized.
func IsEnabled() bool {
	return encryptionStatePtr.Load() != nil
}

// Key returns a copy of the derived 32-byte encryption key, or nil if the
// encryption key has not been initialized. The returned slice is a copy so
// callers may not mutate the underlying key. Used by subsystems that need to
// derive their own domain-separated subkeys (e.g. WebSocket ticket signing).
//
// When both a legacy and a custom key are loaded, Key() returns the legacy
// one so existing callers (e.g. WebSocket ticket signing) keep deriving
// keys bit-for-bit against the same bytes they did before salt was
// configurable. New code that needs salt-awareness should derive from
// State() instead.
func Key() []byte {
	state := encryptionStatePtr.Load()
	if state == nil || state.legacyKey == nil {
		return nil
	}
	out := make([]byte, len(state.legacyKey))
	copy(out, state.legacyKey)
	return out
}

// State returns a snapshot of the current encryption state, or nil when
// Init has not run. The returned slice contents are copies — callers may
// not mutate the underlying key material.
func State() *EncryptionState {
	state := encryptionStatePtr.Load()
	if state == nil {
		return nil
	}
	return &EncryptionState{
		LegacyKey: append([]byte(nil), state.legacyKey...),
		CustomKey: append([]byte(nil), state.customKey...),
	}
}

// EncryptionState is the read-only snapshot of derived keys returned by
// State(). LegacyKey is always populated when Init succeeds with a non-
// empty passphrase; CustomKey is populated only when InitWithSalt was
// called with a non-nil salt.
type EncryptionState struct {
	LegacyKey []byte
	CustomKey []byte
}

// HashSHA256 returns a deterministic hex-encoded SHA-256 hash of the input.
// Used for hash-based lookups on encrypted columns (e.g., virtual key value, session token).
func HashSHA256(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

// Decrypt decrypts a base64-encoded ciphertext using AES-256-GCM and
// returns the plaintext. Two ciphertext formats are supported:
//
//  1. New format — leading byte is a salt_version tag (0 = legacy key,
//     1 = custom-salt key). Written by every Encrypt call after the salt
//     was made configurable; both versions coexist in one database when
//     a deployment transitioned through a salt change.
//  2. Legacy format — no version header; the payload is just (nonce ||
//     ciphertext) under the legacy key. Written by every build prior to
//     salt configurability. We auto-detect by trying the new format
//     first and falling back to the legacy format on a GCM auth failure
//     (the only failure mode where two attempts are not ambiguous).
//
// When the encryption key is unset, Decrypt returns the input unchanged
// (treating it as plaintext) only when the operator has explicitly opted in
// to plaintext storage. With D9's default opt-out, the key being unset while
// there is a non-empty ciphertext is a configuration error.
//
// Empty input always returns "".
func Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	state := encryptionStatePtr.Load()
	if state == nil {
		if AllowPlaintextStorage() {
			return ciphertext, nil
		}
		return ciphertext, ErrEncryptionKeyNotInitialized
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Try the new (versioned) format first. It fails loudly when the
	// leading byte is a known-but-unloadable version (e.g. version=1
	// without a configured salt) so the operator can fix the deployment
	// instead of silently reading garbled data.
	if plaintext, derr := decryptVersioned(data, state); derr == nil {
		return plaintext, nil
	} else if isUnsupportedVersionError(derr) {
		return "", derr
	}

	// Fall back to the legacy (no-header) format. Old builds wrote
	// base64(nonce||ciphertext) directly; we use the legacy key in both
	// cases (it is always populated when Init succeeded). A wrong-key
	// failure here propagates so callers can distinguish a corrupt row
	// from a missing key.
	return decryptLegacy(data, state.legacyKey)
}

// decryptVersioned handles the salt_version-headered ciphertext format.
// Returns a non-nil err on every GCM failure; the caller decides whether
// to fall back to the legacy format.
func decryptVersioned(data []byte, state *encryptionState) (string, error) {
	aesGCM, err := newGCM(state)
	if err != nil {
		return "", err
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < 1+nonceSize {
		return "", fmt.Errorf("ciphertext too short for versioned format")
	}
	version := data[0]
	var key []byte
	switch version {
	case saltVersionLegacy:
		key = state.legacyKey
	case saltVersionCustom:
		key = state.customKey
		if key == nil {
			return "", &unsupportedVersionError{v: version, reason: "no custom encryption_salt configured"}
		}
	default:
		return "", &unsupportedVersionError{v: version, reason: "unknown salt_version byte"}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM2, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce, payload := data[1:1+nonceSize], data[1+nonceSize:]
	plaintext, err := aesGCM2.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt (versioned): %w", err)
	}
	return string(plaintext), nil
}

// decryptLegacy handles the pre-salt-configurable ciphertext format:
// base64(nonce||ciphertext) with no version header.
func decryptLegacy(data []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, payload := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt (legacy): %w", err)
	}
	return string(plaintext), nil
}

// newGCM wraps aes.NewCipher + cipher.NewGCM for the legacy key. Used
// only to compute the nonce size for the versioned-format length check;
// the actual key is selected per-version inside decryptVersioned.
func newGCM(state *encryptionState) (cipher.AEAD, error) {
	block, err := aes.NewCipher(state.legacyKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// unsupportedVersionError signals "the version byte was recognised as a
// hard deployment error, do not fall back to the legacy format". A GCM
// auth failure, by contrast, is a soft "maybe it was actually legacy
// data" signal that the caller should retry as legacy.
type unsupportedVersionError struct {
	v      byte
	reason string
}

func (e *unsupportedVersionError) Error() string {
	return fmt.Sprintf("unsupported salt_version %d: %s", e.v, e.reason)
}

func isUnsupportedVersionError(err error) bool {
	var uve *unsupportedVersionError
	return errors.As(err, &uve)
}
