package model

import (
	"errors"

	"github.com/modex/modex-cloud/constant"
	"gorm.io/gorm"
)

// Grant authorizes one supplier (UserId) to upload keys to one Platform
// (PlatformId) — the "admin-grants-per-supplier" decision. The unique composite
// index prevents duplicate grants.
//
// The Allowed* fields, when non-empty, NARROW the platform's global whitelist
// for this supplier (JSON-encoded TEXT, cross-DB safe). Empty means "inherit the
// platform whitelist unchanged".
type Grant struct {
	Id         int `json:"id" gorm:"primaryKey"`
	UserId     int `json:"user_id" gorm:"uniqueIndex:idx_grant_user_platform;not null"`
	PlatformId int `json:"platform_id" gorm:"uniqueIndex:idx_grant_user_platform;not null"`

	AllowedTypes  string `json:"allowed_types" gorm:"type:text"`  // JSON []int (subset of platform)
	AllowedModels string `json:"allowed_models" gorm:"type:text"` // JSON []string
	AllowedGroups string `json:"allowed_groups" gorm:"type:text"` // JSON []string
	MaxChannels   int    `json:"max_channels" gorm:"default:0"`   // 0 = unlimited

	Status      int   `json:"status" gorm:"default:1"`
	CreatedTime int64 `json:"created_time"`
}

var ErrGrantNotFound = errors.New("grant not found")

func (g *Grant) Create() error {
	g.CreatedTime = nowUnix()
	if g.Status == 0 {
		g.Status = constant.StatusEnabled
	}
	return DB.Create(g).Error
}

// GetGrant returns the (enabled) grant linking a supplier to a platform, or
// ErrGrantNotFound if the supplier is not authorized for that platform.
func GetGrant(userId, platformId int) (*Grant, error) {
	var g Grant
	err := DB.First(&g, "user_id = ? AND platform_id = ? AND status = ?",
		userId, platformId, constant.StatusEnabled).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGrantNotFound
	}
	return &g, err
}

// ListGrantsForUser returns all platforms a supplier is authorized to use.
func ListGrantsForUser(userId int) ([]Grant, error) {
	var gs []Grant
	err := DB.Where("user_id = ? AND status = ?", userId, constant.StatusEnabled).
		Find(&gs).Error
	return gs, err
}

// IsAuthorized reports whether the supplier may target the platform at all.
func IsAuthorized(userId, platformId int) (bool, error) {
	_, err := GetGrant(userId, platformId)
	if errors.Is(err, ErrGrantNotFound) {
		return false, nil
	}
	return err == nil, err
}

// ListAllGrants returns every grant (admin view), newest first.
func ListAllGrants(offset, limit int) ([]Grant, int64, error) {
	var total int64
	if err := DB.Model(&Grant{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var gs []Grant
	err := DB.Order("id desc").Offset(offset).Limit(limit).Find(&gs).Error
	return gs, total, err
}

// UpsertGrant creates or updates the (user, platform) grant in place, so an
// admin re-granting an existing pair adjusts the whitelist instead of erroring
// on the unique index. The grant's Status (the "allow upload" switch) is honored
// on both create and update.
func UpsertGrant(g *Grant) error {
	if g.Status == 0 {
		g.Status = constant.StatusEnabled
	}
	// Look up any existing grant for this pair regardless of status, so a disabled
	// grant can be re-enabled (GetGrant only returns enabled ones).
	var existing Grant
	err := DB.First(&existing, "user_id = ? AND platform_id = ?", g.UserId, g.PlatformId).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return g.Create()
	}
	if err != nil {
		return err
	}
	g.Id = existing.Id
	return DB.Model(&Grant{}).Where("id = ?", existing.Id).Updates(map[string]any{
		"allowed_types":  g.AllowedTypes,
		"allowed_models": g.AllowedModels,
		"allowed_groups": g.AllowedGroups,
		"max_channels":   g.MaxChannels,
		"status":         g.Status,
	}).Error
}

// DeleteGrant revokes a supplier's access to a platform.
func DeleteGrant(id int) error {
	return DB.Delete(&Grant{}, "id = ?", id).Error
}
