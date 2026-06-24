package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/crypto"
	"github.com/modex/agt-vault/middleware"
	"github.com/modex/agt-vault/model"
	"github.com/modex/agt-vault/service/sync"
	"github.com/modex/agt-vault/service/validate"

	"github.com/gin-gonic/gin"
)

// --- Supplier: channel (key) management ---
//
// SECURITY: this controller seals keys via the write-only GlobalSealer and never
// decrypts. Plaintext is read from the request, sealed immediately, and dropped.

type createChannelRequest struct {
	PlatformId int    `json:"platform_id"`
	Name       string `json:"name"`
	Type       int    `json:"type"`
	Key        string `json:"key"` // write-only: sealed then discarded
	BaseURL    string `json:"base_url"`
	Models     string `json:"models"` // comma-separated, AGT-native
	Group      string `json:"group"`
}

// ListChannels returns the supplier's own channels, never including the key.
func ListChannels(c *gin.Context) {
	userId := middleware.CurrentUserId(c)
	platformId := atoiQuery(c, "platform_id")
	offset, limit := pageParams(c)

	channels, total, err := model.ListChannelsForUser(userId, platformId, offset, limit)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to list channels")
		return
	}
	common.ApiSuccess(c, gin.H{"items": channels, "total": total})
}

// CreateChannel accepts an uploaded key, validates it against the supplier's
// grant + platform whitelist, seals it, and triggers AGT sync.
func CreateChannel(c *gin.Context) {
	userId := middleware.CurrentUserId(c)
	var req createChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		common.ApiError(c, http.StatusBadRequest, "key is required")
		return
	}

	// Authorization: supplier must hold a grant for this platform.
	platform, grant, err := authorizePlatform(userId, req.PlatformId)
	if err != nil {
		common.ApiError(c, http.StatusForbidden, err.Error())
		return
	}

	// Whitelist validation (type/models/group/base_url).
	res, err := validate.ChannelUpload(validate.ChannelInput{
		PlatformId: req.PlatformId,
		Type:       req.Type,
		Models:     strings.Split(req.Models, ","),
		Group:      req.Group,
		BaseURL:    req.BaseURL,
	}, platform, grant)
	if err != nil {
		common.ApiError(c, http.StatusBadRequest, err.Error())
		return
	}

	// MaxChannels limit from the grant.
	if grant.MaxChannels > 0 {
		count, _ := model.CountChannelsForUserPlatform(userId, req.PlatformId)
		if count >= int64(grant.MaxChannels) {
			common.ApiError(c, http.StatusForbidden, "channel limit reached for this platform")
			return
		}
	}

	// Seal the key (write-only crypto). Plaintext is dropped after this point.
	sealer := crypto.GlobalSealer()
	sealed, err := sealer.SealString(req.Key)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to secure key")
		return
	}
	fingerprint := sealer.FingerprintString(req.Key)
	last4 := crypto.Last4(req.Key)

	// Duplicate detection without comparing plaintext.
	if dup, _ := model.FingerprintExistsForUser(userId, req.PlatformId, fingerprint); dup {
		common.ApiError(c, http.StatusConflict, "this key has already been uploaded to this platform")
		return
	}

	ch := &model.Channel{
		UserId:         userId,
		PlatformId:     req.PlatformId,
		Name:           req.Name,
		Type:           req.Type,
		EncKey:         string(sealed),
		KeyFingerprint: fingerprint,
		KeyLast4:       last4,
		KeyState:       constant.KeyStatePending,
		BaseURL:        res.BaseURL,
		Models:         res.ModelsCSV,
		Group:          res.Group,
	}
	if err := ch.Create(); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to save channel")
		return
	}

	// Audit BEFORE sync; detail carries only non-secret context (fingerprint).
	_ = model.WriteAudit(&model.AuditLog{
		UserId: userId, Username: middleware.CurrentUsername(c),
		Action: "CREATE_CHANNEL", ResourceType: "channel", ResourceId: ch.Id,
		Detail: "fp=" + fingerprint[:12], Ip: c.ClientIP(), Result: "success",
	})

	dispatchSync(ch.Id)
	common.ApiSuccess(c, gin.H{"id": ch.Id, "key_state": ch.KeyState, "sync_status": ch.SyncStatus})
}

type updateChannelRequest struct {
	Name    string `json:"name"`
	Key     string `json:"key"` // optional: present => rotate key; absent => keep
	BaseURL string `json:"base_url"`
	Models  string `json:"models"`
	Group   string `json:"group"`
}

// UpdateChannel edits metadata and optionally rotates the key (preserve-on-absent
// semantics, matching the AGT contract).
func UpdateChannel(c *gin.Context) {
	userId := middleware.CurrentUserId(c)
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	ch, err := model.GetChannelForUser(id, userId)
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "channel not found")
		return
	}
	var req updateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	platform, grant, err := authorizePlatform(userId, ch.PlatformId)
	if err != nil {
		common.ApiError(c, http.StatusForbidden, err.Error())
		return
	}

	// Re-validate the resulting metadata.
	models := ch.Models
	if req.Models != "" {
		models = req.Models
	}
	group := ch.Group
	if req.Group != "" {
		group = req.Group
	}
	baseURL := ch.BaseURL
	if req.BaseURL != "" {
		baseURL = req.BaseURL
	}
	res, err := validate.ChannelUpload(validate.ChannelInput{
		PlatformId: ch.PlatformId, Type: ch.Type,
		Models: strings.Split(models, ","), Group: group, BaseURL: baseURL,
	}, platform, grant)
	if err != nil {
		common.ApiError(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name != "" {
		ch.Name = req.Name
	}
	ch.Models, ch.Group, ch.BaseURL = res.ModelsCSV, res.Group, res.BaseURL
	if err := ch.UpdateMetadata(); err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to update channel")
		return
	}

	// Optional key rotation.
	if strings.TrimSpace(req.Key) != "" {
		sealer := crypto.GlobalSealer()
		sealed, err := sealer.SealString(req.Key)
		if err != nil {
			common.ApiError(c, http.StatusInternalServerError, "failed to secure key")
			return
		}
		if err := ch.ReplaceKey(string(sealed), sealer.FingerprintString(req.Key), crypto.Last4(req.Key)); err != nil {
			common.ApiError(c, http.StatusInternalServerError, "failed to rotate key")
			return
		}
	}

	adminAuditAs(c, userId, "UPDATE_CHANNEL", "channel", ch.Id)
	dispatchSync(ch.Id)
	common.ApiSuccess(c, gin.H{"id": ch.Id})
}

// DeleteChannel soft-deletes a supplier's channel.
func DeleteChannel(c *gin.Context) {
	userId := middleware.CurrentUserId(c)
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := model.SoftDeleteForUser(id, userId); err != nil {
		if errors.Is(err, model.ErrChannelNotFound) {
			common.ApiError(c, http.StatusNotFound, "channel not found")
			return
		}
		common.ApiError(c, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	adminAuditAs(c, userId, "DELETE_CHANNEL", "channel", id)
	common.ApiSuccess(c, nil)
}

// ResyncChannel re-attempts a failed/pending sync.
func ResyncChannel(c *gin.Context) {
	userId := middleware.CurrentUserId(c)
	id, ok := pathId(c)
	if !ok {
		common.ApiError(c, http.StatusBadRequest, "invalid id")
		return
	}
	ch, err := model.GetChannelForUser(id, userId)
	if err != nil {
		common.ApiError(c, http.StatusNotFound, "channel not found")
		return
	}
	if ch.SyncAttempts >= constant.MaxSyncRetries {
		common.ApiError(c, http.StatusTooManyRequests, "max retries reached; please re-upload the key")
		return
	}
	dispatchSync(ch.Id)
	common.ApiSuccess(c, gin.H{"id": ch.Id})
}

// --- helpers ---

// authorizePlatform confirms the platform exists, is enabled, and the supplier
// holds an enabled grant for it. Returns both for downstream validation.
func authorizePlatform(userId, platformId int) (*model.Platform, *model.Grant, error) {
	platform, err := model.GetPlatformById(platformId)
	if err != nil {
		return nil, nil, errors.New("platform not found")
	}
	if platform.Status == constant.StatusDisabled {
		return nil, nil, errors.New("platform is disabled")
	}
	grant, err := model.GetGrant(userId, platformId)
	if err != nil {
		return nil, nil, errors.New("you are not authorized to use this platform")
	}
	return platform, grant, nil
}

// dispatchSync runs the sync worker in the background with a bounded timeout, so
// the HTTP response returns immediately while the key is forwarded to AGT.
func dispatchSync(channelId int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		_ = sync.SyncChannel(ctx, channelId)
	}()
}

func atoiQuery(c *gin.Context, key string) int {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// adminAuditAs writes an audit row attributed to a specific user id (used by
// supplier self-service actions).
func adminAuditAs(c *gin.Context, userId int, action, resourceType string, resourceId int) {
	_ = model.WriteAudit(&model.AuditLog{
		UserId: userId, Username: middleware.CurrentUsername(c),
		Action: action, ResourceType: resourceType, ResourceId: resourceId,
		Ip: c.ClientIP(), Result: "success",
	})
}
