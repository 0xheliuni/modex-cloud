package model

import (
	"testing"

	"github.com/modex/modex-cloud/constant"
)

// TestNextChannelSeq_CountsAcrossDeletes proves the generated sequence number
// never reuses a value after a soft delete, so auto-generated channel names
// stay unique for a supplier+platform.
func TestNextChannelSeq_CountsAcrossDeletes(t *testing.T) {
	setupTestDB(t)

	// First channel -> seq 1.
	if seq, err := NextChannelSeq(1, 1); err != nil || seq != 1 {
		t.Fatalf("first seq = %d, err = %v; want 1", seq, err)
	}
	ch := &Channel{UserId: 1, PlatformId: 1, Name: "p-alice-1", Type: constant.ChannelTypeOpenAI}
	if err := ch.Create(); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Second channel -> seq 2.
	if seq, _ := NextChannelSeq(1, 1); seq != 2 {
		t.Errorf("seq after one channel = %d, want 2", seq)
	}

	// Soft delete it; the counter must still advance (Unscoped count).
	if err := SoftDeleteForUser(ch.Id, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if seq, _ := NextChannelSeq(1, 1); seq != 2 {
		t.Errorf("seq after delete = %d, want 2 (no reuse)", seq)
	}

	// A different platform has its own independent counter.
	if seq, _ := NextChannelSeq(1, 2); seq != 1 {
		t.Errorf("seq for other platform = %d, want 1", seq)
	}
}

// TestSetUsage_PersistsWithoutTouchingKey proves recording usage updates only
// the usage columns and never disturbs key/sync state.
func TestSetUsage_PersistsWithoutTouchingKey(t *testing.T) {
	setupTestDB(t)
	ch := &Channel{
		UserId: 1, PlatformId: 1, Name: "p-alice-1", Type: constant.ChannelTypeOpenAI,
		EncKey: "SEALED", KeyState: constant.KeyStatePending,
	}
	if err := ch.Create(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := ch.SetUsage(1500000); err != nil { // 500000 = $1 -> $3
		t.Fatalf("SetUsage: %v", err)
	}
	got, _ := GetChannelById(ch.Id)
	if got.UsedQuota != 1500000 {
		t.Errorf("used_quota = %d, want 1500000", got.UsedQuota)
	}
	if got.UsageSyncTime == 0 {
		t.Error("usage_sync_time should be set")
	}
	// Key state untouched.
	reread, _ := getChannelWithKeyForTest(t, ch.Id)
	if reread.KeyState != constant.KeyStatePending || reread.EncKey == "" {
		t.Error("SetUsage must not touch key/sync state")
	}
}

// TestChannelNameExists detects an existing generated name across all rows.
func TestChannelNameExists(t *testing.T) {
	setupTestDB(t)
	ch := &Channel{UserId: 1, PlatformId: 1, Name: "p-alice-1", Type: constant.ChannelTypeOpenAI}
	_ = ch.Create()

	if ok, _ := ChannelNameExists(1, 1, "p-alice-1"); !ok {
		t.Error("expected existing name to be found")
	}
	if ok, _ := ChannelNameExists(1, 1, "p-alice-2"); ok {
		t.Error("unexpected name reported as existing")
	}
}
