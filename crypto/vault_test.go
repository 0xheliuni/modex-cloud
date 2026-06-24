package crypto

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// testKey is a deterministic 32-byte key for tests (NOT for production).
func testKey() []byte {
	k := make([]byte, MasterKeyLen)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v, err := New(testKey())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

func TestNew_KeyLengthValidation(t *testing.T) {
	for _, n := range []int{0, 1, 16, 31, 33, 64} {
		if _, err := New(make([]byte, n)); err != ErrKeyLength {
			t.Errorf("New(len=%d): want ErrKeyLength, got %v", n, err)
		}
	}
	if _, err := New(make([]byte, MasterKeyLen)); err != nil {
		t.Errorf("New(len=32): unexpected error %v", err)
	}
}

func TestNewFromHex(t *testing.T) {
	good := hex.EncodeToString(testKey()) // 64 chars
	if _, err := NewFromHex(good); err != nil {
		t.Errorf("NewFromHex(valid): %v", err)
	}
	// Wrong length.
	if _, err := NewFromHex("abcd"); err != ErrKeyLength {
		t.Errorf("NewFromHex(short): want ErrKeyLength, got %v", err)
	}
	// Right length, invalid hex (64 non-hex chars).
	if _, err := NewFromHex(strings.Repeat("zz", MasterKeyLen)); err == nil {
		t.Error("NewFromHex(non-hex): want error, got nil")
	}
}

func TestSealOpen_RoundTrip(t *testing.T) {
	v := newTestVault(t)
	cases := [][]byte{
		[]byte(""),
		[]byte("sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxx"),
		[]byte("AKIAIOSFODNN7EXAMPLE\nwJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"), // multi-line AWS-style
		bytes.Repeat([]byte("A"), 4096),
	}
	for i, pt := range cases {
		blob, err := v.Seal(pt)
		if err != nil {
			t.Fatalf("case %d Seal: %v", i, err)
		}
		got, err := v.Open(blob)
		if err != nil {
			t.Fatalf("case %d Open: %v", i, err)
		}
		if !bytes.Equal(got, pt) {
			t.Errorf("case %d round-trip mismatch", i)
		}
	}
}

func TestSeal_NonceUniqueness(t *testing.T) {
	v := newTestVault(t)
	pt := []byte("same plaintext every time")
	blob1, _ := v.Seal(pt)
	blob2, _ := v.Seal(pt)
	if bytes.Equal(blob1, blob2) {
		t.Fatal("two Seals of identical plaintext produced identical blobs (nonce reuse!)")
	}
	// Both must still decrypt back to the same plaintext.
	for _, b := range [][]byte{blob1, blob2} {
		got, err := v.Open(b)
		if err != nil || !bytes.Equal(got, pt) {
			t.Fatalf("Open after nonce check failed: %v", err)
		}
	}
}

func TestOpen_TamperDetection(t *testing.T) {
	v := newTestVault(t)
	blob, _ := v.SealString("tamper me")

	// Flip one bit in every byte position; all must fail authentication.
	for i := 0; i < len(blob); i++ {
		bad := make([]byte, len(blob))
		copy(bad, blob)
		bad[i] ^= 0x01
		if _, err := v.Open(bad); err == nil {
			t.Errorf("tampered blob at byte %d decrypted without error", i)
		}
	}
}

func TestOpen_WrongKeyFails(t *testing.T) {
	v1 := newTestVault(t)
	otherKey := testKey()
	otherKey[0] ^= 0xFF
	v2, _ := New(otherKey)

	blob, _ := v1.SealString("secret")
	if _, err := v2.Open(blob); err != ErrDecrypt {
		t.Errorf("Open with wrong key: want ErrDecrypt, got %v", err)
	}
}

func TestOpen_MalformedBlobs(t *testing.T) {
	v := newTestVault(t)
	tests := []struct {
		name string
		blob []byte
		want error
	}{
		{"nil", nil, ErrBlobTooShort},
		{"empty", []byte{}, ErrBlobTooShort},
		{"version-only", []byte{blobVersionV1}, ErrBlobTooShort},
		{"unknown-version", append([]byte{0xFE}, make([]byte, 32)...), ErrUnknownVersion},
		{"truncated-nonce", append([]byte{blobVersionV1}, make([]byte, 5)...), ErrBlobTooShort},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := v.Open(tc.blob); err != tc.want {
				t.Errorf("Open(%s): want %v, got %v", tc.name, tc.want, err)
			}
		})
	}
}

func TestSeal_VersionByteAndStructure(t *testing.T) {
	v := newTestVault(t)
	blob, _ := v.SealString("x")
	if blob[0] != blobVersionV1 {
		t.Errorf("first byte = %#x, want version %#x", blob[0], blobVersionV1)
	}
	// version(1) + nonce(12) + ciphertext(>=tag 16) for 1-byte plaintext.
	minLen := 1 + v.aead.NonceSize() + v.aead.Overhead()
	if len(blob) < minLen {
		t.Errorf("blob len %d < expected min %d", len(blob), minLen)
	}
}

func TestFingerprint_StableAndKeyed(t *testing.T) {
	v := newTestVault(t)
	secret := "sk-deterministic-key"

	// Stable across calls.
	if v.FingerprintString(secret) != v.FingerprintString(secret) {
		t.Error("fingerprint not stable across calls")
	}
	// Different secret -> different fingerprint.
	if v.FingerprintString(secret) == v.FingerprintString(secret+"!") {
		t.Error("distinct secrets share a fingerprint")
	}
	// Keyed: a different master key yields a different fingerprint for the same secret.
	otherKey := testKey()
	otherKey[5] ^= 0xFF
	v2, _ := New(otherKey)
	if v.FingerprintString(secret) == v2.FingerprintString(secret) {
		t.Error("fingerprint is not keyed (same digest under different master keys)")
	}
	// Hex-encoded SHA-256 => 64 chars.
	if got := len(v.FingerprintString(secret)); got != 64 {
		t.Errorf("fingerprint length = %d, want 64", got)
	}
}

func TestFingerprintEqual_ConstantTime(t *testing.T) {
	v := newTestVault(t)
	a := v.FingerprintString("k")
	if !FingerprintEqual(a, a) {
		t.Error("identical fingerprints reported unequal")
	}
	if FingerprintEqual(a, v.FingerprintString("other")) {
		t.Error("different fingerprints reported equal")
	}
}

func TestLast4(t *testing.T) {
	tests := map[string]string{
		"":                         "••••",
		"short":                    "••••",
		"sk-1234567890abcdef":      "••••cdef",
		"AKIAIOSFODNN7EXAMPLEKEYZ": "••••KEYZ",
	}
	for in, want := range tests {
		if got := Last4(in); got != want {
			t.Errorf("Last4(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestZeroize(t *testing.T) {
	b := []byte("topsecret")
	Zeroize(b)
	for i, c := range b {
		if c != 0 {
			t.Errorf("byte %d not zeroized: %#x", i, c)
		}
	}
}

// TestOpen_NoPlaintextInError guards the invariant that decryption failures
// never echo back any secret-derived material in the error string.
func TestOpen_NoPlaintextInError(t *testing.T) {
	v := newTestVault(t)
	blob, _ := v.SealString("this-must-never-appear")
	blob[len(blob)-1] ^= 0x01 // corrupt the tag
	_, err := v.Open(blob)
	if err == nil {
		t.Fatal("expected decryption error")
	}
	if strings.Contains(err.Error(), "this-must-never-appear") {
		t.Error("error message leaked plaintext")
	}
}
