package controller

import (
	"net/http"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/model"

	"github.com/gin-gonic/gin"
)

// --- Admin: supplier↔platform authorization (grants) ---

type grantRequest struct {
	UserId        int      `json:"user_id"`
	PlatformId    int      `json:"platform_id"`
	AllowedTypes  []int    `json:"allowed_types"`  // optional: narrow platform whitelist
	AllowedModels []string `json:"allowed_models"` // optional
	AllowedGroups []string `json:"allowed_groups"` // optional
	MaxChannels   int      `json:"max_channels"`   // 0 = unlimited
}

// ListGrants returns all authorizations (admin view).
func ListGrants(c *gin.Context) {
	offset, limit := pageParams(c)
	grants, total, err := model.ListAllGrants(offset, limit)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to list grants")
		return
	}
	common.ApiSuccess(c, gin.H{"items": grants, "total": total})
}

// UpsertGrant authorizes a supplier for a platform (or updates the whitelist).
func UpsertGrant(c *gin.Context) {
	var req grantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserId <= 0 || req.PlatformId <= 0 {
		common.ApiError(c, http.StatusBadRequest, "user_id and platform_id are required")
		return
	}
	// Validate referenced entities exist and the user is actually a supplier.
	u, err := model.GetUserById(req.UserId)
	if err != nil {
		common.ApiError(c, http.StatusBadRequest, "user not found")
		return
	}
	if !u.IsSupplier() {
		common.ApiError(c, http.StatusBadRequest, "grants can only target supplier accounts")
		return
	}
	if _, err := model.GetPlatformById(req.PlatformId); err != nil {
		common.ApiError(c, http.StatusBadRequest, "platform not found")
		return
	}

	g := &model.Grant{
		UserId:        req.UserId,
		PlatformId:    req.PlatformId,
		AllowedTypes:  common.EncodeJSON(req.AllowedTypes),
		AllowedModels: common.EncodeJSON(req.AllowedModels),
		AllowedGroups: common.EncodeJSON(req.AllowedGroups),
		MaxChannels:   req.MaxChannels,
	}
	if err := model.UpsertGrant(g); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to save grant")
		return
	}
	adminAudit(c, "UPSERT_GRANT", "grant", g.Id)
	common.ApiSuccess(c, gin.H{"id": g.Id})
}

// DeleteGrant revokes a supplier's authorization for a platform.
func DeleteGrant(c *gin.Context) {
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := model.DeleteGrant(id); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to delete grant")
		return
	}
	adminAudit(c, "DELETE_GRANT", "grant", id)
	common.ApiSuccess(c, nil)
}
