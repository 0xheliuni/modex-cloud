package model

import (
	"errors"

	"github.com/modex/agt-vault/common"
	"gorm.io/gorm"
)

// Platform is a downstream AGT target that an admin configures. Its AGT access
// token is the platform's only long-lived secret, stored sealed (AES-256-GCM)
// in AGTTokenEnc and never returned over the API — only AGTTokenLast4 is shown.
type Platform struct {
	Id      int    `json:"id" gorm:"primaryKey"`
	Name    string `json:"name" gorm:"type:varchar(100);not null"`
	BaseURL string `json:"base_url" gorm:"type:varchar(255);not null"` // e.g. https://open.naci-tech.com
	Status  int    `json:"status" gorm:"default:1"`

	AGTTokenEnc   string `json:"-" gorm:"type:text"` // sealed AGT bearer token; never serialized
	AGTTokenLast4 string `json:"agt_token_last4" gorm:"type:varchar(8)"`

	// NamePrefix is the admin-configured prefix used to auto-generate channel
	// names as "{NamePrefix}-{username}-{seq}". Suppliers cannot set names.
	NamePrefix string `json:"name_prefix" gorm:"type:varchar(64)"`

	// Groups is the JSON-encoded list of downstream groups this platform assigns
	// to every uploaded channel, each with its own show-amount toggle. Stored as
	// TEXT (cross-DB safe). See PlatformGroup.
	Groups string `json:"groups" gorm:"type:text"`

	// Global whitelists. Stored as JSON-encoded strings (TEXT) for cross-DB
	// compatibility — no JSONB. A supplier grant may further narrow these.
	AllowedTypes  string `json:"allowed_types" gorm:"type:text"`  // JSON []int of channel types
	AllowedModels string `json:"allowed_models" gorm:"type:text"` // JSON []string
	AllowedGroups string `json:"allowed_groups" gorm:"type:text"` // JSON []string
	BaseURLAllow  string `json:"base_url_allow" gorm:"type:text"` // JSON []string of allowed upstream base URLs

	CreatedTime int64 `json:"created_time"`
	UpdatedTime int64 `json:"updated_time"`
}

// PlatformGroup is one downstream group a platform assigns to every channel
// uploaded to it, together with whether the channel's consumed amount is shown
// to the supplier. Suppliers never choose groups; the admin configures them per
// platform.
type PlatformGroup struct {
	Name       string `json:"name"`
	ShowAmount bool   `json:"show_amount"`
}

var ErrPlatformNotFound = errors.New("platform not found")
var ErrPlatformInUse = errors.New("platform still has channels and cannot be deleted")

func (p *Platform) Create() error {
	p.CreatedTime = nowUnix()
	p.UpdatedTime = p.CreatedTime
	return DB.Create(p).Error
}

func GetPlatformById(id int) (*Platform, error) {
	if id == 0 {
		return nil, ErrPlatformNotFound
	}
	var p Platform
	err := DB.First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlatformNotFound
	}
	return &p, err
}

// ListPlatforms returns all platforms (admin view). The sealed token never
// serializes (json:"-"), so this is safe to return directly.
func ListPlatforms() ([]Platform, error) {
	var ps []Platform
	err := DB.Order("id asc").Find(&ps).Error
	return ps, err
}

// Update writes mutable platform fields (everything except the sealed token,
// which is set separately via SetAGTToken so it is never accidentally cleared).
func (p *Platform) Update() error {
	p.UpdatedTime = nowUnix()
	return DB.Model(p).Omit("agt_token_enc", "agt_token_last4", "created_time").
		Updates(map[string]any{
			"name":           p.Name,
			"base_url":       p.BaseURL,
			"status":         p.Status,
			"name_prefix":    p.NamePrefix,
			"groups":         p.Groups,
			"allowed_types":  p.AllowedTypes,
			"allowed_models": p.AllowedModels,
			"allowed_groups": p.AllowedGroups,
			"base_url_allow": p.BaseURLAllow,
			"updated_time":   p.UpdatedTime,
		}).Error
}

// SetAGTToken stores the sealed AGT bearer token and its display suffix in one
// update. The caller seals via crypto.GlobalSealer(); plaintext never reaches
// the model layer beyond computing last4.
func (p *Platform) SetAGTToken(sealedBlob, last4 string) error {
	return DB.Model(p).Updates(map[string]any{
		"agt_token_enc":   sealedBlob,
		"agt_token_last4": last4,
		"updated_time":    nowUnix(),
	}).Error
}

// ParsedGroups decodes the platform's configured groups (name + show-amount
// toggle). Returns nil when none are configured.
func (p *Platform) ParsedGroups() []PlatformGroup {
	if p.Groups == "" {
		return nil
	}
	var gs []PlatformGroup
	if err := common.Unmarshal([]byte(p.Groups), &gs); err != nil {
		return nil
	}
	return gs
}

// PrimaryGroupName returns the name of the platform's first configured group,
// or "" when none are configured. This is the group assigned to newly uploaded
// channels (the supplier no longer chooses one).
func (p *Platform) PrimaryGroupName() string {
	gs := p.ParsedGroups()
	if len(gs) == 0 {
		return ""
	}
	return gs[0].Name
}

// ShowAmountForGroup reports whether the consumed amount should be shown for a
// channel in the named group. Unknown groups default to hidden.
func (p *Platform) ShowAmountForGroup(group string) bool {
	for _, g := range p.ParsedGroups() {
		if g.Name == group {
			return g.ShowAmount
		}
	}
	return false
}

// EncodeGroups serializes a group list for storage in the Groups TEXT column.
func EncodeGroups(gs []PlatformGroup) string {
	if len(gs) == 0 {
		return ""
	}
	return common.EncodeJSON(gs)
}

// DeletePlatform removes a platform and refuses if channels still reference it.
func DeletePlatform(id int) error {
	var n int64
	if err := DB.Model(&Channel{}).Where("platform_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrPlatformInUse
	}
	// Also clear grants pointing at this platform.
	if err := DB.Where("platform_id = ?", id).Delete(&Grant{}).Error; err != nil {
		return err
	}
	return DB.Delete(&Platform{}, "id = ?", id).Error
}
