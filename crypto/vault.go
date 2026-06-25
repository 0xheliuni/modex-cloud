// Package crypto provides the authenticated-encryption vault that protects
// every secret the platform handles at rest.
//
// Security model (see PLAN.md "Core security invariants"):
//
//   - Channel API keys are sealed on upload and WIPED the moment AGT sync
//     succeeds (destroy-by-default). Their ciphertext is transient.
//   - Platform AGT access tokens are the only long-term sealed secret.
//   - The master key lives in process memory only, loaded from the
//     MASTER_KEK environment variable. It is never logged or persisted.
//
// Open (decrypt) MUST only be called from the sync worker. It must never be
// invoked from any controller — that is what makes keys "write-only" over HTTP.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Blob format version. The first byte of every sealed blob is a version tag so
// the encryption scheme can evolve without breaking already-stored secrets.
const blobVersionV1 byte = 0x01

// MasterKeyLen is the required master-key length in bytes (AES-256).
const MasterKeyLen = 32

var (
	// ErrKeyLength is returned when the supplied master key is not 32 bytes.
	ErrKeyLength = fmt.Errorf("crypto: master key must be %d bytes (AES-256)", MasterKeyLen)
	// ErrBlobTooShort is returned when a blob is too short to be valid.
	ErrBlobTooShort = errors.New("crypto: sealed blob is malformed (too short)")
	// ErrUnknownVersion is returned for an unrecognized blob version byte.
	ErrUnknownVersion = errors.New("crypto: unknown sealed blob version")
	// ErrDecrypt is returned when authentication/decryption fails. It is
	// intentionally generic so callers cannot distinguish tamper from wrong-key.
	ErrDecrypt = errors.New("crypto: decryption failed")
)

// Vault performs authenticated encryption (AES-256-GCM) and keyed fingerprinting
// (HMAC-SHA256) using a single in-memory master key. It is safe for concurrent
// use: the AEAD primitive is stateless after construction and every Seal draws a
// fresh random nonce.
type Vault struct {
	aead    cipher.AEAD
	hmacKey []byte // derived from the master key, never the raw key itself
}

// New constructs a Vault from a 32-byte master key.
//
// The HMAC fingerprint key is derived from the master key via SHA-256 with a
// domain-separation tag, so the raw AES key is never reused directly for HMAC.
func New(masterKey []byte) (*Vault, error) {
	if len(masterKey) != MasterKeyLen {
		return nil, ErrKeyLength
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: aes init: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: gcm init: %w", err)
	}
	// Domain-separated derivation: HMAC key != encryption key.
	h := sha256.Sum256(append([]byte("modex-cloud/fingerprint/v1\x00"), masterKey...))
	hmacKey := make([]byte, len(h))
	copy(hmacKey, h[:])

	return &Vault{aead: aead, hmacKey: hmacKey}, nil
}

// NewFromHex constructs a Vault from a 64-character hex master key, the form the
// MASTER_KEK environment variable takes. Whitespace is not trimmed by design —
// pass an exact value.
func NewFromHex(hexKey string) (*Vault, error) {
	if len(hexKey) != MasterKeyLen*2 {
		return nil, ErrKeyLength
	}
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: master key is not valid hex: %w", err)
	}
	return New(raw)
}

// Seal encrypts plaintext and returns a self-describing blob:
//
//	[version(1)] [nonce(12)] [ciphertext+tag]
//
// A fresh random nonce is generated per call, so sealing the same plaintext
// twice yields different blobs. The version byte is authenticated as additional
// data, binding the blob to its format.
func (v *Vault) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce generation: %w", err)
	}
	aad := []byte{blobVersionV1}
	ciphertext := v.aead.Seal(nil, nonce, plaintext, aad)

	blob := make([]byte, 0, 1+len(nonce)+len(ciphertext))
	blob = append(blob, blobVersionV1)
	blob = append(blob, nonce...)
	blob = append(blob, ciphertext...)
	return blob, nil
}

// SealString is a convenience wrapper over Seal for string secrets.
func (v *Vault) SealString(plaintext string) ([]byte, error) {
	return v.Seal([]byte(plaintext))
}

// Open authenticates and decrypts a blob produced by Seal.
//
// SECURITY: Open is the sole decryption point in the system and must only be
// called from the sync worker. Never call it from a controller — doing so would
// break the write-only-key invariant.
func (v *Vault) Open(blob []byte) ([]byte, error) {
	if len(blob) < 1 {
		return nil, ErrBlobTooShort
	}
	if blob[0] != blobVersionV1 {
		return nil, ErrUnknownVersion
	}
	nonceSize := v.aead.NonceSize()
	if len(blob) < 1+nonceSize {
		return nil, ErrBlobTooShort
	}
	nonce := blob[1 : 1+nonceSize]
	ciphertext := blob[1+nonceSize:]
	aad := []byte{blobVersionV1}

	plaintext, err := v.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		// Collapse all failure modes into one opaque error.
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// Fingerprint returns a stable, keyed HMAC-SHA256 hex digest of a secret. It is
// used to detect duplicate keys and to reference a key in audit logs without
// storing the key itself. Being keyed (not a bare hash), it resists offline
// rainbow-table attacks on short/low-entropy keys even if the DB is dumped.
func (v *Vault) Fingerprint(secret []byte) string {
	mac := hmac.New(sha256.New, v.hmacKey)
	mac.Write(secret)
	return hex.EncodeToString(mac.Sum(nil))
}

// FingerprintString fingerprints a string secret.
func (v *Vault) FingerprintString(secret string) string {
	return v.Fingerprint([]byte(secret))
}

// FingerprintEqual compares two fingerprints in constant time.
func FingerprintEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Last4 returns a display-safe suffix of a secret for UI/audit ("•••• cdef").
// For very short secrets it returns a fully masked placeholder so it never
// leaks a meaningful fraction of the key.
func Last4(secret string) string {
	if len(secret) < 8 {
		return "••••"
	}
	return "••••" + secret[len(secret)-4:]
}

// Zeroize overwrites a byte slice with zeros. Call it (via defer) on any buffer
// that held plaintext, to shrink the window in which the secret sits in memory.
//
// Note: Go's GC may still have copied the data elsewhere; this is a
// best-effort defense, not a guarantee. Avoid converting secrets to string
// (immutable, un-zeroizable) on the hot path — keep them as []byte.
func Zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
