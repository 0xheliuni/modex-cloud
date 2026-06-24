package crypto

import (
	"strings"
	"testing"
)

func TestGlobalSealer_WriteOnly(t *testing.T) {
	v, _ := New(testKey())
	SetGlobal(v)
	defer SetGlobal(nil)

	sealer := GlobalSealer()

	// The Sealer interface must expose Seal/Fingerprint but NOT Open. This is a
	// compile-time guarantee; we assert the runtime behavior round-trips via the
	// sync opener and that the interface type genuinely lacks Open.
	blob, err := sealer.SealString("agt-secret-token")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	// A controller holding only a Sealer cannot call Open — verify the concrete
	// interface does not satisfy an opener shape.
	type opener interface{ Open([]byte) ([]byte, error) }
	if _, ok := sealer.(opener); ok {
		t.Fatal("SECURITY FAILURE: GlobalSealer() value exposes Open()")
	}

	// The sync opener (and only it) can recover the plaintext.
	got, err := SyncOpener().Open(blob)
	if err != nil || string(got) != "agt-secret-token" {
		t.Fatalf("sync opener round-trip failed: %v / %q", err, got)
	}
}

func TestGlobalSealer_PanicsWhenUninitialized(t *testing.T) {
	SetGlobal(nil)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when global vault is nil")
		}
	}()
	_ = GlobalSealer()
}

func TestInitGlobal_RejectsBadKey(t *testing.T) {
	defer SetGlobal(nil)
	if err := InitGlobal("tooshort"); err == nil {
		t.Error("InitGlobal should reject a non-64-char key")
	}
	if err := InitGlobal(strings.Repeat("a", 64)); err != nil {
		t.Errorf("InitGlobal valid hex: %v", err)
	}
	if !IsGlobalReady() {
		t.Error("global vault should be ready after valid InitGlobal")
	}
}
