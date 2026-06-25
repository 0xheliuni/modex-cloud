package controller

import (
	"net/http"

	"github.com/modex/modex-cloud/common"
	"github.com/modex/modex-cloud/middleware"
	"github.com/modex/modex-cloud/model"

	"github.com/gin-gonic/gin"
)

// ListMyPlatforms returns the platforms the current supplier is authorized to
// upload to, joined with each platform's display info. No secrets are returned.
func ListMyPlatforms(c *gin.Context) {
	userId := middleware.CurrentUserId(c)
	grants, err := model.ListGrantsForUser(userId)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to list platforms")
		return
	}

	type platformView struct {
		PlatformId    int      `json:"platform_id"`
		Name          string   `json:"name"`
		BaseURL       string   `json:"base_url"`
		Status        int      `json:"status"`
		AllowedTypes  []int    `json:"allowed_types"`
		AllowedModels []string `json:"allowed_models"`
		AllowedGroups []string `json:"allowed_groups"`
		MaxChannels   int      `json:"max_channels"`
	}

	views := make([]platformView, 0, len(grants))
	for _, g := range grants {
		p, err := model.GetPlatformById(g.PlatformId)
		if err != nil {
			continue
		}
		// Effective whitelist = grant narrowing if present, else platform's.
		types := common.DecodeIntList(g.AllowedTypes)
		if len(types) == 0 {
			types = common.DecodeIntList(p.AllowedTypes)
		}
		models := common.DecodeStringList(g.AllowedModels)
		if len(models) == 0 {
			models = common.DecodeStringList(p.AllowedModels)
		}
		groups := common.DecodeStringList(g.AllowedGroups)
		if len(groups) == 0 {
			groups = common.DecodeStringList(p.AllowedGroups)
		}
		views = append(views, platformView{
			PlatformId: p.Id, Name: p.Name, BaseURL: p.BaseURL, Status: p.Status,
			AllowedTypes: types, AllowedModels: models, AllowedGroups: groups,
			MaxChannels: g.MaxChannels,
		})
	}
	common.ApiSuccess(c, gin.H{"items": views})
}
