// Package controller holds HTTP request handlers. Handlers validate input, call
// model/service code, and write the standard response envelope.
//
// SECURITY: no handler in this package may call crypto.Vault.Open or otherwise
// return a channel key plaintext. Key decryption happens only in service/sync.
package controller

import (
	"net/http"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/middleware"
	"github.com/modex/agt-vault/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login authenticates a user and establishes a session. To resist user
// enumeration, invalid-username and wrong-password return the same message.
func Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		common.ApiError(c, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := model.GetUserByUsername(req.Username)
	if err != nil || !common.ValidatePasswordAndHash(req.Password, user.Password) {
		common.ApiError(c, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if user.Status == constant.StatusDisabled {
		common.ApiError(c, http.StatusForbidden, "account has been disabled")
		return
	}

	if err := establishSession(c, user); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to create session")
		return
	}
	_ = model.TouchLogin(user.Id, c.ClientIP())
	_ = model.WriteAudit(&model.AuditLog{
		UserId: user.Id, Username: user.Username, Action: "LOGIN",
		ResourceType: "session", Ip: c.ClientIP(), Result: "success",
	})

	common.ApiSuccess(c, gin.H{
		"id":            user.Id,
		"username":      user.Username,
		"role":          user.Role,
		"supplier_code": user.SupplierCode,
		"supplier_name": user.SupplierName,
	})
}

// Logout clears the session.
func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	_ = session.Save()
	common.ApiSuccess(c, nil)
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword updates the authenticated user's password after verifying the
// old one. Applies a minimum-length policy.
func ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		common.ApiError(c, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	userId := middleware.CurrentUserId(c)
	user, err := model.GetUserById(userId)
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "user not found")
		return
	}
	if !common.ValidatePasswordAndHash(req.OldPassword, user.Password) {
		common.ApiError(c, http.StatusUnauthorized, "old password is incorrect")
		return
	}
	if err := user.UpdatePassword(req.NewPassword); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to update password")
		return
	}
	_ = model.WriteAudit(&model.AuditLog{
		UserId: user.Id, Username: user.Username, Action: "CHANGE_PASSWORD",
		ResourceType: "user", ResourceId: user.Id, Ip: c.ClientIP(), Result: "success",
	})
	common.ApiSuccess(c, nil)
}

// Self returns the authenticated user's own profile (never any secret).
func Self(c *gin.Context) {
	user, err := model.GetUserById(middleware.CurrentUserId(c))
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "user not found")
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":             user.Id,
		"username":       user.Username,
		"role":           user.Role,
		"supplier_code":  user.SupplierCode,
		"supplier_name":  user.SupplierName,
		"two_fa_enabled": user.TwoFAEnabled,
	})
}

// establishSession writes the authenticated principal into the session cookie.
func establishSession(c *gin.Context, user *model.User) error {
	session := sessions.Default(c)
	session.Set(common.CtxUserId, user.Id)
	session.Set(common.CtxUsername, user.Username)
	session.Set(common.CtxRole, user.Role)
	session.Set(common.CtxStatus, user.Status)
	return session.Save()
}
