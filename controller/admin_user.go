package controller

import (
	"net/http"
	"strconv"

	"github.com/modex/modex-cloud/common"
	"github.com/modex/modex-cloud/constant"
	"github.com/modex/modex-cloud/model"

	"github.com/gin-gonic/gin"
)

// --- Admin: user management (create supplier accounts, reset, enable/disable) ---

type createUserRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Role         int    `json:"role"`
	SupplierCode string `json:"supplier_code"`
	SupplierName string `json:"supplier_name"`
}

// ListUsers returns users (optionally filtered by ?role=). Secrets never serialize.
func ListUsers(c *gin.Context) {
	role, _ := strconv.Atoi(c.Query("role"))
	offset, limit := pageParams(c)
	users, total, err := model.ListUsers(role, offset, limit)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to list users")
		return
	}
	common.ApiSuccess(c, gin.H{"items": users, "total": total})
}

// CreateUser provisions an account (typically a supplier) for delivery.
func CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || len(req.Password) < 8 {
		common.ApiError(c, http.StatusBadRequest, "username required and password must be >= 8 chars")
		return
	}
	role := orDefault(req.Role, constant.RoleSupplier)
	if !constant.IsValidRole(role) {
		common.ApiError(c, http.StatusBadRequest, "invalid role")
		return
	}
	if _, err := model.GetUserByUsername(req.Username); err == nil {
		common.ApiError(c, http.StatusConflict, "username already exists")
		return
	}

	u := &model.User{
		Username:     req.Username,
		Role:         role,
		Status:       constant.StatusEnabled,
		SupplierCode: req.SupplierCode,
		SupplierName: req.SupplierName,
	}
	if err := u.Create(req.Password); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to create user")
		return
	}
	adminAudit(c, "CREATE_USER", "user", u.Id)
	common.ApiSuccess(c, gin.H{"id": u.Id, "username": u.Username})
}

type updateUserRequest struct {
	Role         int    `json:"role"`
	Status       int    `json:"status"`
	SupplierCode string `json:"supplier_code"`
	SupplierName string `json:"supplier_name"`
}

// UpdateUser edits a user's role, status and supplier identity.
func UpdateUser(c *gin.Context) {
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	u, err := model.GetUserById(id)
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "user not found")
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role != 0 {
		if !constant.IsValidRole(req.Role) {
			common.ApiError(c, http.StatusBadRequest, "invalid role")
			return
		}
		u.Role = req.Role
	}
	if req.Status != 0 {
		u.Status = req.Status
	}
	u.SupplierCode = req.SupplierCode
	u.SupplierName = req.SupplierName
	if err := u.UpdateProfile(); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to update user")
		return
	}
	adminAudit(c, "UPDATE_USER", "user", u.Id)
	common.ApiSuccess(c, nil)
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ResetUserPassword sets a new password for any user (admin action).
func ResetUserPassword(c *gin.Context) {
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.NewPassword) < 8 {
		common.ApiError(c, http.StatusBadRequest, "new_password must be >= 8 chars")
		return
	}
	u, err := model.GetUserById(id)
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "user not found")
		return
	}
	if err := u.UpdatePassword(req.NewPassword); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to reset password")
		return
	}
	adminAudit(c, "RESET_PASSWORD", "user", u.Id)
	common.ApiSuccess(c, nil)
}

// DeleteUser removes a user and cascades grants/channels.
func DeleteUser(c *gin.Context) {
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := model.DeleteUser(id); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to delete user")
		return
	}
	adminAudit(c, "DELETE_USER", "user", id)
	common.ApiSuccess(c, nil)
}

// pageParams extracts (offset, limit) from ?page & ?page_size with sane defaults.
func pageParams(c *gin.Context) (offset, limit int) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return (page - 1) * size, size
}
