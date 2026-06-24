package crypto

// Global vault wiring.
//
// The platform process holds exactly one Vault, initialized at startup from the
// MASTER_KEK environment variable. To keep channel keys write-only we expose it
// through two deliberately-different accessors:
//
//   - GlobalSealer() returns a Sealer — encrypt/fingerprint ONLY. Controllers
//     and any non-sync code use this. They literally cannot decrypt.
//   - SyncOpener() returns the full *Vault including Open. ONLY the sync worker
//     calls it; the distinctive name makes a grep audit trivial.

// Sealer is the write-only face of the vault: seal secrets and derive
// fingerprints, but never decrypt.
//
// It is implemented by sealerOnly (NOT by *Vault directly), so a holder cannot
// type-assert their way back to Open(). This makes "controllers can encrypt but
// never decrypt" a structural guarantee, not a naming convention.
type Sealer interface {
	Seal(plaintext []byte) ([]byte, error)
	SealString(plaintext string) ([]byte, error)
	Fingerprint(secret []byte) string
	FingerprintString(secret string) string
}

// sealerOnly wraps a Vault exposing only its write-only operations. It has no
// Open method and embeds nothing, so no type assertion recovers decryption.
type sealerOnly struct {
	v *Vault
}

func (s sealerOnly) Seal(pt []byte) ([]byte, error)       { return s.v.Seal(pt) }
func (s sealerOnly) SealString(pt string) ([]byte, error) { return s.v.SealString(pt) }
func (s sealerOnly) Fingerprint(b []byte) string          { return s.v.Fingerprint(b) }
func (s sealerOnly) FingerprintString(b string) string    { return s.v.FingerprintString(b) }

var global *Vault

// InitGlobal initializes the process vault from a 64-char hex master key.
func InitGlobal(hexKey string) error {
	v, err := NewFromHex(hexKey)
	if err != nil {
		return err
	}
	global = v
	return nil
}

// SetGlobal installs a pre-built vault (used by tests).
func SetGlobal(v *Vault) { global = v }

// GlobalSealer returns the process vault as a write-only Sealer that cannot be
// downcast to expose Open.
func GlobalSealer() Sealer {
	if global == nil {
		panic("crypto: global vault not initialized")
	}
	return sealerOnly{v: global}
}

// SyncOpener returns the full vault, including Open. MUST be called only from
// the sync worker. Calling it elsewhere violates the write-only-key invariant.
func SyncOpener() *Vault {
	if global == nil {
		panic("crypto: global vault not initialized")
	}
	return global
}

// IsGlobalReady reports whether the process vault has been initialized.
func IsGlobalReady() bool { return global != nil }
