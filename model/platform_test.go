package model

import (
	"testing"

	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/crypto"
)

// TestPlatform_TokenSealedNeverPlain proves a platform's AGT token is stored
// sealed and that the JSON view never exposes the ciphertext or plaintext.
func TestPlatform_TokenSealedNeverPlain(t *testing.T) {
	setupTestDB(t)
	vault, _ := crypto.New(make([]byte, crypto.MasterKeyLen))
	crypto.SetGlobal(vault)
	defer crypto.SetGlobal(nil)

	const token = "agt-bearer-SUPER-SECRET-TOKEN"
	p := &Platform{Name: "AGT-prod", BaseURL: "https://open.naci-tech.com", Status: constant.StatusEnabled}
	if err := p.Create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	sealed, _ := crypto.GlobalSealer().SealString(token)
	if err := p.SetAGTToken(string(sealed), crypto.Last4(token)); err != nil {
		t.Fatalf("set token: %v", err)
	}

	// Reload and verify the sealed blob is NOT the plaintext.
	reloaded, err := GetPlatformById(p.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AGTTokenEnc == token {
		t.Fatal("SECURITY FAILURE: AGT token stored in plaintext")
	}
	if reloaded.AGTTokenEnc == "" {
		t.Fatal("AGT token blob missing")
	}
	if reloaded.AGTTokenLast4 != crypto.Last4(token) {
		t.Errorf("last4 = %q, want %q", reloaded.AGTTokenLast4, crypto.Last4(token))
	}

	// JSON marshal must omit the sealed token entirely (json:"-").
	js, _ := marshalJSON(reloaded)
	if containsAny(js, token, reloaded.AGTTokenEnc) {
		t.Errorf("SECURITY FAILURE: serialized platform leaked token material: %s", js)
	}

	// The sync opener can still recover it for forwarding to AGT.
	got, err := crypto.SyncOpener().Open([]byte(reloaded.AGTTokenEnc))
	if err != nil || string(got) != token {
		t.Fatalf("opener round-trip failed: %v / %q", err, got)
	}
}

func TestPlatform_DeleteRefusedWhenInUse(t *testing.T) {
	setupTestDB(t)
	p := &Platform{Name: "p", BaseURL: "http://x", Status: constant.StatusEnabled}
	_ = p.Create()
	ch := &Channel{UserId: 1, PlatformId: p.Id, Type: constant.ChannelTypeOpenAI, KeyState: constant.KeyStatePending}
	_ = ch.Create()

	if err := DeletePlatform(p.Id); err != ErrPlatformInUse {
		t.Errorf("delete in-use platform: want ErrPlatformInUse, got %v", err)
	}
}
