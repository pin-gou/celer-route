package encrypt

import (
	"errors"
	"testing"

	bifrost "github.com/pin-gou/celer-route/core"
	"github.com/pin-gou/celer-route/core/schemas"
)

func TestEncryptDecrypt(t *testing.T) {
	// Set a test encryption key
	testKey := "test-encryption-key-for-testing-32bytes"
	Init(testKey, bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	testCases := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "Simple text",
			plaintext: "hello world",
		},
		{
			name:      "AWS Access Key",
			plaintext: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:      "AWS Secret Key",
			plaintext: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name:      "Empty string",
			plaintext: "",
		},
		{
			name:      "Special characters",
			plaintext: "!@#$%^&*()_+-=[]{}|;':\",./<>?`~",
		},
		{
			name:      "Long text",
			plaintext: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Failed to encrypt: %v", err)
			}

			// For empty strings, encryption should return empty
			if tc.plaintext == "" {
				if encrypted != "" {
					t.Errorf("Expected empty string for empty input, got: %s", encrypted)
				}
				return
			}

			// Encrypted text should be different from plaintext
			if encrypted == tc.plaintext {
				t.Errorf("Encrypted text should be different from plaintext")
			}

			// Decrypt
			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Failed to decrypt: %v", err)
			}

			// Decrypted text should match original plaintext
			if decrypted != tc.plaintext {
				t.Errorf("Decrypted text does not match original.\nExpected: %s\nGot: %s", tc.plaintext, decrypted)
			}
		})
	}
}

func TestEncryptDeterminism(t *testing.T) {
	// Set a test encryption key
	testKey := "test-encryption-key-for-testing-32bytes"
	Init(testKey, bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	plaintext := "test-plaintext"

	// Encrypt the same text twice
	encrypted1, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}
	encrypted2, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// They should be different (due to random nonce)
	if encrypted1 == encrypted2 {
		t.Errorf("Two encryptions of the same plaintext should produce different ciphertexts (due to random nonce)")
	}

	// But both should decrypt to the same plaintext
	decrypted1, err := Decrypt(encrypted1)
	if err != nil {
		t.Fatalf("Failed to decrypt first: %v", err)
	}
	decrypted2, err := Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Failed to decrypt second: %v", err)
	}

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Errorf("Both decryptions should match original plaintext")
	}
}

func TestDecryptInvalidData(t *testing.T) {
	// Set a test encryption key
	testKey := "test-encryption-key-for-testing-32bytes"
	Init(testKey, bifrost.NewDefaultLogger(schemas.LogLevelInfo))

	testCases := []struct {
		name       string
		ciphertext string
	}{
		{
			name:       "Invalid base64",
			ciphertext: "not-valid-base64!@#$",
		},
		{
			name:       "Valid base64 but invalid ciphertext",
			ciphertext: "YWJjZGVmZ2hpamtsbW5vcA==",
		},
		{
			name:       "Too short ciphertext",
			ciphertext: "YWJj",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decrypt(tc.ciphertext)
			if err == nil {
				t.Errorf("Expected error when decrypting invalid data, got nil")
			}
		})
	}
}

func TestKDFWithVariousKeyLengths(t *testing.T) {
	// Test that keys of various lengths work correctly with KDF
	testCases := []struct {
		name string
		key  string
	}{
		{
			name: "Short key (8 bytes)",
			key:  "shortkey",
		},
		{
			name: "Medium key (16 bytes)",
			key:  "medium-key-16byt",
		},
		{
			name: "Long key (32 bytes)",
			key:  "this-is-a-32-byte-long-key!!",
		},
		{
			name: "Very long key (64 bytes)",
			key:  "this-is-a-very-long-key-that-is-definitely-more-than-64-bytes",
		},
	}

	plaintext := "test-data-for-encryption"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Initialize with this key
			Init(tc.key, bifrost.NewDefaultLogger(schemas.LogLevelInfo))

			// Encrypt
			encrypted, err := Encrypt(plaintext)
			if err != nil {
				t.Fatalf("Failed to encrypt: %v", err)
			}

			// Should produce valid ciphertext
			if encrypted == plaintext {
				t.Errorf("Encrypted text should be different from plaintext")
			}

			// Decrypt should work
			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Failed to decrypt with %s: %v", tc.name, err)
			}

			if decrypted != plaintext {
				t.Errorf("Decrypted text does not match original.\nExpected: %s\nGot: %s", plaintext, decrypted)
			}
		})
	}
}

func TestKDFDeterministic(t *testing.T) {
	// Test that the same passphrase always produces the same derived key
	passphrase := "test-passphrase"
	plaintext := "test-data"

	// Initialize with passphrase and encrypt
	Init(passphrase, bifrost.NewDefaultLogger(schemas.LogLevelInfo))
	encrypted1, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Re-initialize with same passphrase (simulating restart)
	Init(passphrase, bifrost.NewDefaultLogger(schemas.LogLevelInfo))
	
	// Should be able to decrypt the previously encrypted data
	decrypted, err := Decrypt(encrypted1)
	if err != nil {
		t.Fatalf("Failed to decrypt after re-initialization: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted text does not match original after re-initialization.\nExpected: %s\nGot: %s", plaintext, decrypted)
	}

	// Encrypt again with same passphrase
	encrypted2, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Should be able to decrypt both (even though they're different due to nonce)
	decrypted2, err := Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Failed to decrypt second encryption: %v", err)
	}

	if decrypted2 != plaintext {
		t.Errorf("Second decryption does not match original.\nExpected: %s\nGot: %s", plaintext, decrypted2)
	}
}

// TestPlaintextPolicy_D9 verifies the D9 fail-closed behaviour: when no
// encryption key is initialised and the operator has not opted in to plaintext
// storage, Encrypt must refuse to write the secret rather than silently
// returning the plaintext. Decrypt must return ErrEncryptionKeyNotInitialized
// so callers can detect a misconfigured boot.
func TestPlaintextPolicy_D9(t *testing.T) {
	// Reset to "no key, no opt-in" — this mirrors a fresh process with an
	// empty config.json. The package-level state persists across tests in
	// this binary, so we have to defend against leakage from earlier tests.
	SetAllowPlaintextStorage(false)
	Init("", bifrost.NewDefaultLogger(schemas.LogLevelWarn))

	out, err := Encrypt("sk-live-secret")
	if !errors.Is(err, ErrPlaintextWriteForbidden) {
		t.Fatalf("expected ErrPlaintextWriteForbidden when key unset + opt-out, got err=%v out=%q", err, out)
	}
	if out != "" {
		t.Errorf("expected empty output on refused write, got %q", out)
	}

	// Empty plaintext never raises the policy error.
	if out, err := Encrypt(""); err != nil || out != "" {
		t.Errorf("Encrypt(\"\") = (%q, %v), want (\"\", nil)", out, err)
	}

	// Decrypt with no key + no opt-in must surface the missing-key error.
	if _, err := Decrypt("anything"); !errors.Is(err, ErrEncryptionKeyNotInitialized) {
		t.Fatalf("expected ErrEncryptionKeyNotInitialized, got %v", err)
	}

	// Decrypt of empty input is always "".
	if out, err := Decrypt(""); err != nil || out != "" {
		t.Errorf("Decrypt(\"\") = (%q, %v), want (\"\", nil)", out, err)
	}
}

// TestPlaintextPolicy_OptIn flips the policy explicitly and confirms
// Encrypt/Decrypt now silently pass through the plaintext. This is the dev
// / opt-in mode the docs document for fast local setup.
func TestPlaintextPolicy_OptIn(t *testing.T) {
	SetAllowPlaintextStorage(true)
	Init("", bifrost.NewDefaultLogger(schemas.LogLevelWarn))

	out, err := Encrypt("sk-dev-secret")
	if err != nil {
		t.Fatalf("Encrypt with opt-in: %v", err)
	}
	if out != "sk-dev-secret" {
		t.Errorf("Encrypt with opt-in should pass through unchanged, got %q", out)
	}

	back, err := Decrypt("sk-dev-secret")
	if err != nil {
		t.Fatalf("Decrypt with opt-in: %v", err)
	}
	if back != "sk-dev-secret" {
		t.Errorf("Decrypt with opt-in should pass through unchanged, got %q", back)
	}

	// Reset for the next test in the suite.
	SetAllowPlaintextStorage(false)
}

// TestPlaintextPolicy_Restore confirms that once an encryption key is set,
// the policy opt-in is irrelevant: Encrypt always produces ciphertext,
// Decrypt always recovers plaintext.
func TestPlaintextPolicy_Restore(t *testing.T) {
	SetAllowPlaintextStorage(false)
	Init("a-test-passphrase-32-bytes-long!!", bifrost.NewDefaultLogger(schemas.LogLevelWarn))

	cipher, err := Encrypt("sk-live-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if cipher == "sk-live-secret" {
		t.Errorf("expected ciphertext, got plaintext through")
	}

	back, err := Decrypt(cipher)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if back != "sk-live-secret" {
		t.Errorf("round-trip mismatch: got %q want %q", back, "sk-live-secret")
	}
}
