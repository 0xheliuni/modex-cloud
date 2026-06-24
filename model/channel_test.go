package model

import (
	"testing"

	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/crypto"
)

// setupTestDB opens an in-memory SQLite database and migrates the schema.
func setupTestDB(t *testing.T) {
	t.Helper()
	// Force SQLite in-memory; each test gets a clean DB via a unique DSN.
	UsingSQLite, UsingMySQL, UsingPostgreSQL = true, false, false
	initCol()
	openInMemory(t)
}

// TestChannel_DestroyByDefault is the platform's defining guarantee: after a
// successful sync, the sealed key is gone from the database, but the fingerprint
// and last4 survive for audit/dedup.
func TestChannel_DestroyByDefault(t *testing.T) {
	setupTestDB(t)

	vault, err := crypto.New(make([]byte, crypto.MasterKeyLen))
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	const plainKey = "sk-ant-api03-SECRETSECRETSECRET"

	// Upload: seal the key, store pending.
	sealed, _ := vault.SealString(plainKey)
	ch := &Channel{
		UserId:         1,
		PlatformId:     1,
		Name:           "modex-anthropic",
		Type:           constant.ChannelTypeAnthropic,
		EncKey:         string(sealed),
		KeyFingerprint: vault.FingerprintString(plainKey),
		KeyLast4:       crypto.Last4(plainKey),
		KeyState:       constant.KeyStatePending,
	}
	if err := ch.Create(); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Pre-sync: the sealed key is recoverable.
	if !ch.HasRecoverableKey() {
		t.Fatal("expected recoverable key before sync")
	}

	// Simulate a successful AGT sync.
	if err := ch.MarkSynced(12345); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}

	// Re-read straight from the DB, explicitly SELECTing enc_key, to prove the
	// column was physically wiped (not just hidden by Omit).
	var raw struct {
		EncKey         string
		KeyState       string
		KeyFingerprint string
		KeyLast4       string
		RemoteId       int
	}
	if err := DB.Model(&Channel{}).
		Select("enc_key", "key_state", "key_fingerprint", "key_last4", "remote_id").
		Where("id = ?", ch.Id).Scan(&raw).Error; err != nil {
		t.Fatalf("reread: %v", err)
	}

	if raw.EncKey != "" {
		t.Errorf("SECURITY FAILURE: enc_key not wiped after sync, got %q", raw.EncKey)
	}
	if raw.KeyState != constant.KeyStateSynced {
		t.Errorf("key_state = %q, want synced", raw.KeyState)
	}
	if raw.KeyFingerprint != vault.FingerprintString(plainKey) {
		t.Error("fingerprint should survive the wipe")
	}
	if raw.KeyLast4 != crypto.Last4(plainKey) {
		t.Errorf("last4 should survive the wipe, got %q", raw.KeyLast4)
	}
	if raw.RemoteId != 12345 {
		t.Errorf("remote_id = %d, want 12345", raw.RemoteId)
	}
}

// TestChannel_OmitKeyOnRead proves the standard read paths never load enc_key,
// even before a sync wipe.
func TestChannel_OmitKeyOnRead(t *testing.T) {
	setupTestDB(t)
	ch := &Channel{
		UserId: 7, PlatformId: 2, Name: "x", Type: constant.ChannelTypeOpenAI,
		EncKey: "SEALED-BLOB-SHOULD-NOT-LOAD", KeyState: constant.KeyStatePending,
	}
	if err := ch.Create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := GetChannelForUser(ch.Id, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.EncKey != "" {
		t.Errorf("GetChannelForUser leaked enc_key: %q", got.EncKey)
	}
	list, _, err := ListChannelsForUser(7, 0, 0, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].EncKey != "" {
		t.Errorf("ListChannelsForUser leaked enc_key")
	}
}

// TestChannel_FailedSyncRetainsKey proves a failed sync keeps the sealed key for
// retry and bumps the attempt counter.
func TestChannel_FailedSyncRetainsKey(t *testing.T) {
	setupTestDB(t)
	ch := &Channel{
		UserId: 1, PlatformId: 1, Type: constant.ChannelTypeGemini,
		EncKey: "SEALED", KeyState: constant.KeyStatePending,
	}
	if err := ch.Create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := ch.MarkFailed("agt 500"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	reread, _ := getChannelWithKeyForTest(t, ch.Id)
	if reread.EncKey == "" {
		t.Error("failed sync must retain sealed key for retry")
	}
	if reread.KeyState != constant.KeyStateFailed {
		t.Errorf("key_state = %q, want failed", reread.KeyState)
	}
	if reread.SyncAttempts != 1 {
		t.Errorf("sync_attempts = %d, want 1", reread.SyncAttempts)
	}
}

// getChannelWithKeyForTest reads a channel INCLUDING enc_key (test-only helper;
// production code uses the sync worker's internal loader).
func getChannelWithKeyForTest(t *testing.T, id int) (*Channel, error) {
	t.Helper()
	var c Channel
	err := DB.First(&c, "id = ?", id).Error
	return &c, err
}
