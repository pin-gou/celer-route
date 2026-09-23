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

var encryptionKey []byte
var logger schemas.Logger

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

// Init initializes the encryption key using Argon2id KDF to derive a secure 32-byte key
// from the provided passphrase. This ensures strong entropy regardless of passphrase length.
// The function accepts any passphrase but warns if it's too short (< 16 bytes).
func Init(key string, _logger schemas.Logger) {
	logger = _logger
	if key == "" {
		encryptionKey = nil
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

	// Derive a secure 32-byte key using Argon2id KDF
	// We use a fixed salt since this is a system-wide encryption key (not per-user passwords)
	// Argon2id parameters: time=1, memory=64MB, threads=4, keyLen=32
	// This provides strong security while maintaining reasonable performance for initialization
	salt := []byte("bifrost-encryption-v1-salt-2024")
	encryptionKey = argon2.IDKey([]byte(key), salt, 1, 64*1024, 4, 32)
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

// Encrypt encrypts a plaintext string using AES-256-GCM and returns a base64-encoded ciphertext.
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
	if encryptionKey == nil {
		if !AllowPlaintextStorage() {
			return "", ErrPlaintextWriteForbidden
		}
		return plaintext, nil
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return plaintext, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Create a nonce (number used once)
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext, fmt.Errorf("failed to read nonce: %w", err)
	}

	// Encrypt the data
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// IsEnabled returns true if the encryption key has been initialized
func IsEnabled() bool {
	return encryptionKey != nil
}

// Key returns a copy of the derived 32-byte encryption key, or nil if the
// encryption key has not been initialized. The returned slice is a copy so
// callers may not mutate the underlying key. Used by subsystems that need to
// derive their own domain-separated subkeys (e.g. WebSocket ticket signing).
func Key() []byte {
	if encryptionKey == nil {
		return nil
	}
	out := make([]byte, len(encryptionKey))
	copy(out, encryptionKey)
	return out
}

// HashSHA256 returns a deterministic hex-encoded SHA-256 hash of the input.
// Used for hash-based lookups on encrypted columns (e.g., virtual key value, session token).
func HashSHA256(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

// Decrypt decrypts a base64-encoded ciphertext using AES-256-GCM and returns the plaintext.
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
	if encryptionKey == nil {
		if AllowPlaintextStorage() {
			return ciphertext, nil
		}
		return ciphertext, ErrEncryptionKeyNotInitialized
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt the data
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
