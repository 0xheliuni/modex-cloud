package controller

import (
	"net/http"
	"strconv"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/crypto"
	"github.com/modex/agt-vault/middleware"
	"github.com/modex/agt-vault/model"

	"github.com/gin-gonic/gin"
)

// --- Admin: target-platform management (参数管理) ---

type platformRequest struct {
	Name          string   `json:"name"`
	BaseURL       string   `json:"base_url"`
	AGTToken      string   `json:"agt_token"` // write-only; sealed, never returned
	Status        int      `json:"status"`
	AllowedTypes  []int    `json:"allowed_types"`
	AllowedModels []string `json:"allowed_models"`
	AllowedGroups []string `json:"allowed_groups"`
	BaseURLAllow  []string `json:"base_url_allow"`
}

// ListPlatforms returns all target platforms. Sealed AGT tokens never serialize.
func ListPlatforms(c *gin.Context) {
	ps, err := model.ListPlatforms()
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to list platforms")
		return
	}
	common.ApiSuccess(c, ps)
}

// CreatePlatform registers a new target AGT platform and seals its access token.
func CreatePlatform(c *gin.Context) {
	var req platformRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.BaseURL == "" {
		common.ApiError(c, http.StatusBadRequest, "name and base_url are required")
		return
	}
	if req.AGTToken == "" {
		common.ApiError(c, http.StatusBadRequest, "agt_token is required")
		return
	}

	p := &model.Platform{
		Name:          req.Name,
		BaseURL:       req.BaseURL,
		Status:        orDefault(req.Status, constant.StatusEnabled),
		AllowedTypes:  common.EncodeJSON(req.AllowedTypes),
		AllowedModels: common.EncodeJSON(req.AllowedModels),
		AllowedGroups: common.EncodeJSON(req.AllowedGroups),
		BaseURLAllow:  common.EncodeJSON(req.BaseURLAllow),
	}
	if err := p.Create(); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to create platform")
		return
	}

	// Seal the AGT token (write-only crypto — controllers cannot Open).
	if err := sealPlatformToken(p, req.AGTToken); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to secure platform token")
		return
	}

	adminAudit(c, "CREATE_PLATFORM", "platform", p.Id)
	common.ApiSuccess(c, gin.H{"id": p.Id})
}

// UpdatePlatform edits a platform; AGT token is re-sealed only if provided.
func UpdatePlatform(c *gin.Context) {
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := model.GetPlatformById(id)
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "platform not found")
		return
	}
	var req platformRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		p.Name = req.Name
	}
	if req.BaseURL != "" {
		p.BaseURL = req.BaseURL
	}
	if req.Status != 0 {
		p.Status = req.Status
	}
	p.AllowedTypes = common.EncodeJSON(req.AllowedTypes)
	p.AllowedModels = common.EncodeJSON(req.AllowedModels)
	p.AllowedGroups = common.EncodeJSON(req.AllowedGroups)
	p.BaseURLAllow = common.EncodeJSON(req.BaseURLAllow)
	if err := p.Update(); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to update platform")
		return
	}

	// Only rotate the token when a new one is supplied (omit = keep existing).
	if req.AGTToken != "" {
		if err := sealPlatformToken(p, req.AGTToken); err != nil {
			common.ApiError(c, http.StatusInternalServerError, "failed to secure platform token")
			return
		}
	}

	adminAudit(c, "UPDATE_PLATFORM", "platform", p.Id)
	common.ApiSuccess(c, gin.H{"id": p.Id})
}

// DeletePlatform removes a platform (refused if channels still reference it).
func DeletePlatform(c *gin.Context) {
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := model.DeletePlatform(id); err != nil {
		if err == model.ErrPlatformInUse {
			common.ApiError(c, http.StatusConflict, "platform still has channels; remove them first")
			return
		}
		common.ApiError(c, http.StatusInternalServerError, "failed to delete platform")
		return
	}
	adminAudit(c, "DELETE_PLATFORM", "platform", id)
	common.ApiSuccess(c, nil)
}

// sealPlatformToken seals the AGT token via the write-only global sealer and
// stores the blob + display suffix. Plaintext is not logged or retained.
func sealPlatformToken(p *model.Platform, token string) error {
	sealer := crypto.GlobalSealer()
	blob, err := sealer.SealString(token)
	if err != nil {
		return err
	}
	return p.SetAGTToken(string(blob), crypto.Last4(token))
}

// --- small shared helpers for admin controllers ---

func pathId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func adminAudit(c *gin.Context, action, resourceType string, resourceId int) {
	_ = model.WriteAudit(&model.AuditLog{
		UserId:       middleware.CurrentUserId(c),
		Username:     middleware.CurrentUsername(c),
		Action:       action,
		ResourceType: resourceType,
		ResourceId:   resourceId,
		Ip:           c.ClientIP(),
		Result:       "success",
	})
}
