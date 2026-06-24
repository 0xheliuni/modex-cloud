package model

import (
	"errors"

	"github.com/modex/agt-vault/constant"
	"gorm.io/gorm"
)

// Channel is a single uploaded key bound to one supplier (UserId) and one target
// platform (PlatformId) — the "one-channel-one-platform" decision.
//
// DESTROY-BY-DEFAULT (the core secrecy property):
//
//   - On upload the key is sealed into EncKey and KeyState=pending.
//   - The sync worker decrypts EncKey in memory exactly once, forwards it to AGT,
//     and on success MarkSynced() WIPES EncKey in the same UPDATE, leaving only
//     KeyFingerprint (HMAC) and KeyLast4. The DB then holds no recoverable key.
//   - On failure EncKey is retained (still sealed) for bounded retry.
//
// EncKey is json:"-" and every list/detail DAO Omit("enc_key"), so the sealed
// blob never crosses the API boundary either. There is deliberately no field or
// method that returns the plaintext.
type Channel struct {
	Id         int `json:"id" gorm:"primaryKey"`
	UserId     int `json:"user_id" gorm:"index;not null"`     // owning supplier
	PlatformId int `json:"platform_id" gorm:"index;not null"` // target AGT platform

	Name string `json:"name" gorm:"type:varchar(100)"`
	Type int    `json:"type" gorm:"index"` // one of constant.ChannelType*

	// --- secret material ---
	EncKey         string `json:"-" gorm:"column:enc_key;type:text"`             // sealed; WIPED after sync
	KeyFingerprint string `json:"key_fingerprint" gorm:"type:varchar(64);index"` // HMAC-SHA256, survives wipe
	KeyLast4       string `json:"key_last4" gorm:"type:varchar(8)"`              // display only
	KeyState       string `json:"key_state" gorm:"type:varchar(16);index;default:pending"`

	// --- channel metadata (forwarded to AGT verbatim) ---
	BaseURL string `json:"base_url" gorm:"type:varchar(255)"`
	Models  string `json:"models" gorm:"type:text"`                   // comma-separated, AGT-native
	Group   string `json:"group" gorm:"column:grp;type:varchar(100)"` // 'group' is reserved; store as grp

	// --- usage (pulled back from AGT; new-api quota units, 500000 = $1) ---
	UsedQuota     int64 `json:"used_quota" gorm:"default:0"` // consumed quota reported by AGT
	UsageSyncTime int64 `json:"usage_sync_time"`             // last time UsedQuota was refreshed

	// --- AGT sync bookkeeping ---
	RemoteId     int    `json:"remote_id" gorm:"index"` // template id returned by AGT
	SyncStatus   int    `json:"sync_status" gorm:"default:0;index"`
	SyncError    string `json:"sync_error" gorm:"type:text"`
	SyncAttempts int    `json:"sync_attempts" gorm:"default:0"`
	LastSyncTime int64  `json:"last_sync_time"`

	Status      int            `json:"status" gorm:"default:1;index"`
	CreatedTime int64          `json:"created_time"`
	UpdatedTime int64          `json:"updated_time"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

var (
	ErrChannelNotFound = errors.New("channel not found")
	// ErrKeyDestroyed is returned when something tries to read a key that has
	// already been wiped post-sync — by design, it is unrecoverable.
	ErrKeyDestroyed = errors.New("channel key has been destroyed after sync (write-only by design)")
)

// HasRecoverableKey reports whether the sealed key still exists locally (i.e.
// the channel has not yet been synced-and-wiped).
func (c *Channel) HasRecoverableKey() bool {
	return c.EncKey != "" && c.KeyState != constant.KeyStateSynced
}

// Create persists a freshly-uploaded channel. Callers must have already sealed
// the key into EncKey and populated KeyFingerprint/KeyLast4 via the crypto vault.
func (c *Channel) Create() error {
	now := nowUnix()
	c.CreatedTime = now
	c.UpdatedTime = now
	if c.KeyState == "" {
		c.KeyState = constant.KeyStatePending
	}
	if c.Status == 0 {
		c.Status = constant.StatusEnabled
	}
	return DB.Create(c).Error
}

// MarkSynced records a successful AGT sync and WIPES the sealed key in the same
// statement — the moment that makes this platform "destroy-by-default".
func (c *Channel) MarkSynced(remoteId int) error {
	now := nowUnix()
	updates := map[string]any{
		"enc_key":        "", // <-- the wipe
		"key_state":      constant.KeyStateSynced,
		"remote_id":      remoteId,
		"sync_status":    1,
		"sync_error":     "",
		"last_sync_time": now,
		"updated_time":   now,
	}
	if err := DB.Model(c).Updates(updates).Error; err != nil {
		return err
	}
	c.EncKey = ""
	c.KeyState = constant.KeyStateSynced
	c.RemoteId = remoteId
	c.SyncStatus = 1
	return nil
}

// MarkFailed records a failed sync attempt, retaining the sealed key for retry.
func (c *Channel) MarkFailed(reason string) error {
	now := nowUnix()
	updates := map[string]any{
		"key_state":      constant.KeyStateFailed,
		"sync_status":    2,
		"sync_error":     reason,
		"sync_attempts":  gorm.Expr("sync_attempts + 1"),
		"last_sync_time": now,
		"updated_time":   now,
	}
	return DB.Model(c).Updates(updates).Error
}

// GetChannelById loads a channel WITHOUT its sealed key — the default safe read
// used by every API path. Use getChannelWithKey (sync worker only) when the
// sealed blob is actually needed.
func GetChannelById(id int) (*Channel, error) {
	var c Channel
	err := DB.Omit("enc_key").First(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	return &c, err
}

// GetChannelForUser loads a channel scoped to its owning supplier, without the key.
func GetChannelForUser(id, userId int) (*Channel, error) {
	var c Channel
	err := DB.Omit("enc_key").First(&c, "id = ? AND user_id = ?", id, userId).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	return &c, err
}

// ListChannelsForUser returns a supplier's channels (optionally for one platform),
// never including the sealed key.
func ListChannelsForUser(userId, platformId, offset, limit int) ([]Channel, int64, error) {
	q := DB.Model(&Channel{}).Where("user_id = ?", userId)
	if platformId > 0 {
		q = q.Where("platform_id = ?", platformId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var cs []Channel
	err := q.Omit("enc_key").Order("id desc").Offset(offset).Limit(limit).Find(&cs).Error
	return cs, total, err
}

// FingerprintExistsForUser reports whether the supplier already uploaded a key
// with this fingerprint to this platform (duplicate detection without plaintext).
func FingerprintExistsForUser(userId, platformId int, fingerprint string) (bool, error) {
	var n int64
	err := DB.Model(&Channel{}).
		Where("user_id = ? AND platform_id = ? AND key_fingerprint = ?", userId, platformId, fingerprint).
		Count(&n).Error
	return n > 0, err
}

// CountChannelsForUserPlatform counts a supplier's channels on one platform,
// used to enforce a grant's MaxChannels limit.
func CountChannelsForUserPlatform(userId, platformId int) (int64, error) {
	var n int64
	err := DB.Model(&Channel{}).
		Where("user_id = ? AND platform_id = ?", userId, platformId).Count(&n).Error
	return n, err
}

// LoadChannelForSync loads a channel INCLUDING its sealed enc_key.
//
// SECURITY: this is the only DAO that returns enc_key. It exists solely for the
// sync worker (service/sync). Do not call it from controllers — every API-facing
// read uses the Omit("enc_key") loaders above.
func LoadChannelForSync(id int) (*Channel, error) {
	var c Channel
	err := DB.First(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	return &c, err
}

// MarkMetadataSynced records a successful metadata-only PUT (no key involved,
// because the key was already destroyed on first sync). It never resurrects key
// state.
func (c *Channel) MarkMetadataSynced() error {
	now := nowUnix()
	updates := map[string]any{
		"sync_status":    1,
		"sync_error":     "",
		"last_sync_time": now,
		"updated_time":   now,
	}
	if err := DB.Model(c).Updates(updates).Error; err != nil {
		return err
	}
	c.SyncStatus = 1
	return nil
}

// UpdateMetadata writes supplier-editable, non-secret channel fields. It never
// touches enc_key / key_state, so it is safe for already-synced channels.
func (c *Channel) UpdateMetadata() error {
	return DB.Model(c).Updates(map[string]any{
		"name":         c.Name,
		"base_url":     c.BaseURL,
		"models":       c.Models,
		"grp":          c.Group,
		"sync_status":  0, // needs re-sync to AGT
		"updated_time": nowUnix(),
	}).Error
}

// ReplaceKey re-arms a channel with a freshly-sealed key (key rotation). The
// channel returns to a syncable state while keeping its RemoteId so the worker
// issues a PUT rather than a POST.
func (c *Channel) ReplaceKey(sealedBlob, fingerprint, last4 string) error {
	now := nowUnix()
	updates := map[string]any{
		"enc_key":         sealedBlob,
		"key_fingerprint": fingerprint,
		"key_last4":       last4,
		"key_state":       constant.KeyStatePending,
		"sync_status":     0,
		"sync_attempts":   0,
		"updated_time":    now,
	}
	if err := DB.Model(c).Updates(updates).Error; err != nil {
		return err
	}
	c.EncKey = sealedBlob
	c.KeyState = constant.KeyStatePending
	return nil
}

// SoftDeleteForUser soft-deletes a channel owned by the supplier.
func SoftDeleteForUser(id, userId int) error {
	res := DB.Where("id = ? AND user_id = ?", id, userId).Delete(&Channel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// SetUsage records the consumed quota pulled back from AGT for this channel. It
// never touches key/sync state, so it is safe at any point in the lifecycle.
func (c *Channel) SetUsage(usedQuota int64) error {
	now := nowUnix()
	if err := DB.Model(c).Updates(map[string]any{
		"used_quota":      usedQuota,
		"usage_sync_time": now,
	}).Error; err != nil {
		return err
	}
	c.UsedQuota = usedQuota
	c.UsageSyncTime = now
	return nil
}

// NextChannelSeq returns the next 1-based sequence number for a supplier's
// channels on one platform, counting across ALL rows (including soft-deleted)
// so a generated name is never reused after a delete. Used to build the
// system-generated channel name "{prefix}-{username}-{seq}".
func NextChannelSeq(userId, platformId int) (int, error) {
	var n int64
	err := DB.Unscoped().Model(&Channel{}).
		Where("user_id = ? AND platform_id = ?", userId, platformId).Count(&n).Error
	if err != nil {
		return 0, err
	}
	return int(n) + 1, nil
}

// ChannelNameExists reports whether a channel with this exact name already
// exists for the supplier+platform (across all rows), so the generator can skip
// collisions when sequence numbers and deletes get out of step.
func ChannelNameExists(userId, platformId int, name string) (bool, error) {
	var n int64
	err := DB.Unscoped().Model(&Channel{}).
		Where("user_id = ? AND platform_id = ? AND name = ?", userId, platformId, name).
		Count(&n).Error
	return n > 0, err
}
