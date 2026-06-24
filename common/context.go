package common

// Gin context keys for the authenticated principal. Centralized so handlers and
// middleware never disagree on the string literals.
const (
	CtxUserId       = "user_id"
	CtxUsername     = "username"
	CtxRole         = "role"
	CtxStatus       = "status"
	CtxUseAccessTok = "use_access_token"
)
