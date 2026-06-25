// Package middleware holds Gin middleware: authentication, RBAC, rate limiting
// and audit. The auth design mirrors new-api (session first, access-token
// fallback) but is trimmed to this platform's three roles.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/modex/modex-cloud/common"
	"github.com/modex/modex-cloud/constant"
	"github.com/modex/modex-cloud/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// authHelper resolves the caller (session or access token), enforces the minimum
// role, and stashes the principal in the Gin context. It aborts on any failure.
func authHelper(c *gin.Context, minRole int) {
	session := sessions.Default(c)
	username := session.Get(common.CtxUsername)
	role := session.Get(common.CtxRole)
	id := session.Get(common.CtxUserId)
	status := session.Get(common.CtxStatus)
	useAccessToken := false

	if id == nil {
		// No session — try the access token (API clients).
		user, err := userFromAccessToken(c)
		if err != nil {
			common.AbortError(c, http.StatusUnauthorized, "not logged in or token invalid")
			return
		}
		username, role, id, status = user.Username, user.Role, user.Id, user.Status
		useAccessToken = true
	}

	roleInt, _ := role.(int)
	statusInt, _ := status.(int)

	if statusInt == constant.StatusDisabled {
		common.AbortError(c, http.StatusForbidden, "account has been disabled")
		return
	}
	if roleInt < minRole {
		common.AbortError(c, http.StatusForbidden, "insufficient privilege")
		return
	}

	c.Set(common.CtxUserId, id)
	c.Set(common.CtxUsername, username)
	c.Set(common.CtxRole, roleInt)
	c.Set(common.CtxStatus, statusInt)
	c.Set(common.CtxUseAccessTok, useAccessToken)
	c.Next()
}

// userFromAccessToken extracts and validates a Bearer access token.
func userFromAccessToken(c *gin.Context) (*model.User, error) {
	raw := c.GetHeader("Authorization")
	if raw == "" {
		return nil, errors.New("no authorization header")
	}
	token := raw
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return nil, errors.New("empty token")
	}
	user, err := model.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Username == "" {
		return nil, errors.New("invalid token user")
	}
	return user, nil
}

// SupplierAuth requires at least the supplier role.
func SupplierAuth() gin.HandlerFunc {
	return func(c *gin.Context) { authHelper(c, constant.RoleSupplier) }
}

// AdminAuth requires at least the admin role.
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) { authHelper(c, constant.RoleAdmin) }
}

// RootAuth requires the root role.
func RootAuth() gin.HandlerFunc {
	return func(c *gin.Context) { authHelper(c, constant.RoleRoot) }
}

// CurrentUserId returns the authenticated user's id from the context.
func CurrentUserId(c *gin.Context) int {
	v, _ := c.Get(common.CtxUserId)
	id, _ := v.(int)
	return id
}

// CurrentRole returns the authenticated user's role from the context.
func CurrentRole(c *gin.Context) int {
	v, _ := c.Get(common.CtxRole)
	role, _ := v.(int)
	return role
}

// CurrentUsername returns the authenticated user's username from the context.
func CurrentUsername(c *gin.Context) string {
	v, _ := c.Get(common.CtxUsername)
	name, _ := v.(string)
	return name
}
