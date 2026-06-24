package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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
	Type       int    `json:"type"`
	Key        string `json:"key"`    // write-only: sealed then discarded
	Models     string `json:"models"` // comma-separated, AGT-native
	// Name, BaseURL and Group are intentionally NOT accepted from suppliers:
	// the name is system-generated, the group is admin-configured per platform,
	// and base_url is no longer used.
}

// channelView is the supplier-facing shape of a channel, augmented with the
// per-platform "show amount" decision so the UI knows whether to render usage.
type channelView struct {
	model.Channel
	ShowAmount bool `json:"show_amount"`
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

	// Resolve the show-amount toggle per channel from its platform's group config.
	platformCache := map[int]*model.Platform{}
	views := make([]channelView, 0, len(channels))
	for i := range channels {
		ch := channels[i]
		p := platformCache[ch.PlatformId]
		if p == nil {
			if loaded, err := model.GetPlatformById(ch.PlatformId); err == nil {
				p = loaded
				platformCache[ch.PlatformId] = p
			}
		}
		show := false
		if p != nil {
			show = p.ShowAmountForGroup(ch.Group)
		}
		views = append(views, channelView{Channel: ch, ShowAmount: show})
	}
	common.ApiSuccess(c, gin.H{"items": views, "total": total})
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

	// Group is admin-configured per platform; the supplier no longer chooses it.
	// base_url is no longer used (left empty so AGT applies its provider default).
	group := platform.PrimaryGroupName()

	// Whitelist validation (type/models/group). base_url omitted by design.
	res, err := validate.ChannelUpload(validate.ChannelInput{
		PlatformId: req.PlatformId,
		Type:       req.Type,
		Models:     strings.Split(req.Models, ","),
		Group:      group,
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

	// System-generated channel name: "{prefix}-{username}-{seq}".
	name, err := generateChannelName(userId, req.PlatformId, middleware.CurrentUsername(c), platform.NamePrefix)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to generate channel name")
		return
	}

	ch := &model.Channel{
		UserId:         userId,
		PlatformId:     req.PlatformId,
		Name:           name,
		Type:           req.Type,
		EncKey:         string(sealed),
		KeyFingerprint: fingerprint,
		KeyLast4:       last4,
		KeyState:       constant.KeyStatePending,
		BaseURL:        "",
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
	Key    string `json:"key"`    // optional: present => rotate key; absent => keep
	Models string `json:"models"` // optional: edit the model list
	// Name, BaseURL and Group are not supplier-editable (system/admin owned).
}

// UpdateChannel edits the model list and optionally rotates the key
// (preserve-on-absent semantics, matching the AGT contract). The channel name,
// group and base_url are not supplier-editable.
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

	// Re-validate the resulting metadata. Group stays admin-controlled; keep the
	// channel's current group (or the platform's, in case config changed).
	models := ch.Models
	if req.Models != "" {
		models = req.Models
	}
	group := ch.Group
	if group == "" {
		group = platform.PrimaryGroupName()
	}
	res, err := validate.ChannelUpload(validate.ChannelInput{
		PlatformId: ch.PlatformId, Type: ch.Type,
		Models: strings.Split(models, ","), Group: group,
	}, platform, grant)
	if err != nil {
		common.ApiError(c, http.StatusBadRequest, err.Error())
		return
	}

	ch.Models, ch.Group, ch.BaseURL = res.ModelsCSV, res.Group, ""
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

// RefreshUsage pulls the channel's consumed amount from AGT and returns the
// updated value. Only shown to the supplier when the platform's group config
// enables it, but the refresh itself is always allowed for the owner.
func RefreshUsage(c *gin.Context) {
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	used, err := sync.RefreshUsage(ctx, ch.Id)
	if err != nil {
		common.ApiError(c, http.StatusBadGateway, "failed to refresh usage: "+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"id": ch.Id, "used_quota": used})
}

// --- helpers ---

// generateChannelName builds a system-generated channel name of the form
// "{prefix}-{username}-{seq}". The prefix is admin-configured per platform; when
// empty the name is just "{username}-{seq}". It probes for collisions so a
// sequence reused after deletes still yields a unique name.
func generateChannelName(userId, platformId int, username, prefix string) (string, error) {
	seq, err := model.NextChannelSeq(userId, platformId)
	if err != nil {
		return "", err
	}
	build := func(n int) string {
		base := username + "-" + strconv.Itoa(n)
		if strings.TrimSpace(prefix) != "" {
			return prefix + "-" + base
		}
		return base
	}
	// Probe forward until the name is free (bounded to avoid an infinite loop).
	for i := 0; i < 1000; i++ {
		name := build(seq + i)
		exists, err := model.ChannelNameExists(userId, platformId, name)
		if err != nil {
			return "", err
		}
		if !exists {
			return name, nil
		}
	}
	return build(seq), nil
}

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
