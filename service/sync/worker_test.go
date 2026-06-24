package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/crypto"
	"github.com/modex/agt-vault/model"
)

// TestSyncChannel_EndToEnd_DestroysKey is the platform's headline guarantee in
// action: a real (mock) AGT receives the key in the correct format, and the
// local ciphertext is wiped the instant the sync succeeds.
func TestSyncChannel_EndToEnd_DestroysKey(t *testing.T) {
	cleanup, err := model.InitForTest()
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer cleanup()

	vault, _ := crypto.New(make([]byte, crypto.MasterKeyLen))
	crypto.SetGlobal(vault)
	defer crypto.SetGlobal(nil)

	const supplierKey = "sk-ant-api03-REALSECRET-DONOTLEAK"
	const agtToken = "agt-platform-bearer-token"

	// Mock AGT platform: assert auth header + wrapped create body, return id 999.
	var gotAuth, gotKey, gotMode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var env struct {
			Mode    string `json:"mode"`
			Channel struct {
				Key  string `json:"key"`
				Type int    `json:"type"`
			} `json:"channel"`
		}
		_ = json.Unmarshal(body, &env)
		gotKey, gotMode = env.Channel.Key, env.Mode
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"id":999,"ids":[999]}}`))
	}))
	defer srv.Close()

	// Platform with a sealed AGT token.
	platform := &model.Platform{Name: "AGT", BaseURL: srv.URL, Status: constant.StatusEnabled}
	_ = platform.Create()
	sealedTok, _ := crypto.GlobalSealer().SealString(agtToken)
	_ = platform.SetAGTToken(string(sealedTok), crypto.Last4(agtToken))

	// Channel with a sealed supplier key, pending sync.
	sealedKey, _ := crypto.GlobalSealer().SealString(supplierKey)
	ch := &model.Channel{
		UserId: 1, PlatformId: platform.Id, Name: "modex-anthropic",
		Type:           constant.ChannelTypeAnthropic,
		EncKey:         string(sealedKey),
		KeyFingerprint: crypto.GlobalSealer().FingerprintString(supplierKey),
		KeyLast4:       crypto.Last4(supplierKey),
		KeyState:       constant.KeyStatePending,
		Models:         "claude-3-5-sonnet",
	}
	_ = ch.Create()

	// Run the sync.
	if err := SyncChannel(context.Background(), ch.Id); err != nil {
		t.Fatalf("SyncChannel: %v", err)
	}

	// AGT received the right things.
	if gotAuth != "Bearer "+agtToken {
		t.Errorf("AGT auth header = %q, want bearer %q", gotAuth, agtToken)
	}
	if gotKey != supplierKey {
		t.Errorf("AGT received key = %q, want %q", gotKey, supplierKey)
	}
	if gotMode != "single" {
		t.Errorf("AGT mode = %q, want single", gotMode)
	}

	// THE GUARANTEE: enc_key is physically empty in the DB, fingerprint survives.
	var raw struct {
		EncKey         string
		KeyState       string
		RemoteId       int
		KeyFingerprint string
	}
	_ = model.DB.Model(&model.Channel{}).
		Select("enc_key", "key_state", "remote_id", "key_fingerprint").
		Where("id = ?", ch.Id).Scan(&raw).Error
	if raw.EncKey != "" {
		t.Errorf("SECURITY FAILURE: enc_key not wiped after sync: %q", raw.EncKey)
	}
	if raw.KeyState != constant.KeyStateSynced {
		t.Errorf("key_state = %q, want synced", raw.KeyState)
	}
	if raw.RemoteId != 999 {
		t.Errorf("remote_id = %d, want 999", raw.RemoteId)
	}
	if raw.KeyFingerprint == "" {
		t.Error("fingerprint should survive the wipe")
	}
}

// TestSyncChannel_FailureRetainsKey proves a rejecting AGT leaves the sealed key
// intact for retry.
func TestSyncChannel_FailureRetainsKey(t *testing.T) {
	cleanup, err := model.InitForTest()
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer cleanup()
	vault, _ := crypto.New(make([]byte, crypto.MasterKeyLen))
	crypto.SetGlobal(vault)
	defer crypto.SetGlobal(nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"invalid key"}`))
	}))
	defer srv.Close()

	platform := &model.Platform{Name: "AGT", BaseURL: srv.URL, Status: constant.StatusEnabled}
	_ = platform.Create()
	sealedTok, _ := crypto.GlobalSealer().SealString("tok")
	_ = platform.SetAGTToken(string(sealedTok), "••••")

	sealedKey, _ := crypto.GlobalSealer().SealString("sk-bad")
	ch := &model.Channel{
		UserId: 1, PlatformId: platform.Id, Type: constant.ChannelTypeOpenAI,
		EncKey: string(sealedKey), KeyState: constant.KeyStatePending, Models: "gpt-4o",
	}
	_ = ch.Create()

	if err := SyncChannel(context.Background(), ch.Id); err == nil {
		t.Fatal("expected sync error from rejecting AGT")
	}
	reloaded, _ := model.LoadChannelForSync(ch.Id)
	if reloaded.EncKey == "" {
		t.Error("failed sync must retain the sealed key for retry")
	}
	if reloaded.KeyState != constant.KeyStateFailed {
		t.Errorf("key_state = %q, want failed", reloaded.KeyState)
	}
}
