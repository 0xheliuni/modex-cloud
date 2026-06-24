package constant

// User roles. Ordered so a numeric >= comparison expresses "at least this role".
const (
	RoleGuest    = 0
	RoleSupplier = 1   // can upload keys to granted platforms
	RoleAdmin    = 10  // manages platforms, users, grants; can NEVER read a key
	RoleRoot     = 100 // full control
)

// IsValidRole reports whether r is a recognized role.
func IsValidRole(r int) bool {
	return r == RoleSupplier || r == RoleAdmin || r == RoleRoot
}

// User / platform / grant status values.
const (
	StatusEnabled  = 1
	StatusDisabled = 2
)

// KeyState tracks the lifecycle of a channel's secret under destroy-by-default.
//
//	pending  — uploaded & sealed, not yet synced to AGT; EncKey is present.
//	synced   — AGT accepted it; EncKey has been WIPED. Only fingerprint+last4 remain.
//	failed   — sync failed; EncKey retained (still sealed) for bounded retry.
//
// The whole point of the platform: once a channel reaches `synced`, the database
// contains no recoverable copy of the key.
const (
	KeyStatePending = "pending"
	KeyStateSynced  = "synced"
	KeyStateFailed  = "failed"
)

// Sync retry policy.
const (
	MaxSyncRetries = 5
)
