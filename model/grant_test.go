package model

import (
	"errors"
	"testing"

	"github.com/modex/modex-cloud/constant"
)

// TestGrant_AllowUploadSwitch proves the grant Status field is the "allow
// upload" switch: GetGrant (the upload-authorization path) only returns an
// enabled grant, and UpsertGrant can flip the switch off and back on for the
// same (user, platform) pair without tripping the unique index.
func TestGrant_AllowUploadSwitch(t *testing.T) {
	setupTestDB(t)

	const userId, platformId = 7, 3

	// Create enabled: GetGrant resolves it.
	if err := UpsertGrant(&Grant{UserId: userId, PlatformId: platformId, Status: constant.StatusEnabled}); err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if _, err := GetGrant(userId, platformId); err != nil {
		t.Fatalf("enabled grant should resolve: %v", err)
	}

	// Disable: GetGrant now reports it as not authorized.
	if err := UpsertGrant(&Grant{UserId: userId, PlatformId: platformId, Status: constant.StatusDisabled}); err != nil {
		t.Fatalf("disable grant: %v", err)
	}
	if _, err := GetGrant(userId, platformId); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("disabled grant must be hidden from GetGrant, got err=%v", err)
	}

	// The grant row still exists (only one, not a duplicate from the unique index).
	all, total, err := ListAllGrants(0, 100)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	if total != 1 || len(all) != 1 {
		t.Fatalf("expected exactly one grant row, got total=%d len=%d", total, len(all))
	}

	// Re-enable: GetGrant resolves it again.
	if err := UpsertGrant(&Grant{UserId: userId, PlatformId: platformId, Status: constant.StatusEnabled}); err != nil {
		t.Fatalf("re-enable grant: %v", err)
	}
	if _, err := GetGrant(userId, platformId); err != nil {
		t.Fatalf("re-enabled grant should resolve: %v", err)
	}
}

// TestGrant_DefaultsToEnabled proves an upsert with no explicit status defaults
// to enabled (the allow-upload switch is on unless turned off).
func TestGrant_DefaultsToEnabled(t *testing.T) {
	setupTestDB(t)
	if err := UpsertGrant(&Grant{UserId: 1, PlatformId: 1}); err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if _, err := GetGrant(1, 1); err != nil {
		t.Fatalf("grant with omitted status should default to enabled: %v", err)
	}
}
