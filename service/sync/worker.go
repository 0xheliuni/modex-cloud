// Package sync is the ONLY place in the codebase that decrypts channel keys.
//
// It loads a channel's sealed key, opens it in memory via crypto.SyncOpener(),
// forwards the plaintext to the AGT platform, and — on success — wipes the local
// ciphertext (destroy-by-default). The plaintext is zeroized as soon as the AGT
// call returns.
//
// INVARIANT: crypto.SyncOpener().Open must appear nowhere else. A CI grep for
// "SyncOpener" outside this package is a build-breaking violation.
package sync

import (
	"context"
	"fmt"

	"github.com/modex/agt-vault/crypto"
	"github.com/modex/agt-vault/model"
	"github.com/modex/agt-vault/service/agt"
)

// SyncChannel pushes one channel to its AGT platform.
//
//   - New channel (RemoteId == 0): POST create, then MarkSynced (wipes key).
//   - Re-keyed channel (RemoteId != 0, key pending): PUT with new key, then wipe.
//   - Metadata-only (key already destroyed): PUT without key (AGT preserves it).
//
// It records success/failure on the channel and returns the first error.
func SyncChannel(ctx context.Context, channelId int) error {
	ch, err := model.LoadChannelForSync(channelId)
	if err != nil {
		return err
	}
	platform, err := model.GetPlatformById(ch.PlatformId)
	if err != nil {
		return err
	}

	// Decrypt the platform's AGT token (long-lived secret) for this call only.
	tokenBytes, err := crypto.SyncOpener().Open([]byte(platform.AGTTokenEnc))
	if err != nil {
		_ = ch.MarkFailed("platform token could not be decrypted")
		return fmt.Errorf("decrypt platform token: %w", err)
	}
	defer crypto.Zeroize(tokenBytes)
	client := agt.NewClient(platform.BaseURL, string(tokenBytes))

	payload := agt.ChannelPayload{
		Id:      ch.RemoteId,
		Name:    ch.Name,
		Type:    ch.Type,
		BaseURL: ch.BaseURL,
		Models:  ch.Models,
		Group:   ch.Group,
	}

	// Decide whether we need to send the key.
	if ch.HasRecoverableKey() {
		return syncWithKey(ctx, client, ch, payload)
	}
	return syncMetadataOnly(ctx, client, ch, payload)
}

// syncWithKey opens the sealed key, forwards it, then destroys it on success.
func syncWithKey(ctx context.Context, client *agt.Client, ch *model.Channel, payload agt.ChannelPayload) error {
	keyBytes, err := crypto.SyncOpener().Open([]byte(ch.EncKey))
	if err != nil {
		_ = ch.MarkFailed("sealed key could not be decrypted")
		return fmt.Errorf("decrypt channel key: %w", err)
	}
	// The plaintext exists only within this function scope.
	defer crypto.Zeroize(keyBytes)
	payload.Key = string(keyBytes)

	if ch.RemoteId == 0 {
		remoteId, err := client.CreateChannel(ctx, payload)
		if err != nil {
			_ = ch.MarkFailed(err.Error())
			return err
		}
		return ch.MarkSynced(remoteId) // <-- wipes enc_key
	}

	// Re-key of an existing remote channel.
	if err := client.UpdateChannel(ctx, payload); err != nil {
		_ = ch.MarkFailed(err.Error())
		return err
	}
	return ch.MarkSynced(ch.RemoteId) // <-- wipes enc_key
}

// syncMetadataOnly updates a channel whose key was already destroyed. No key is
// sent; AGT keeps the upstream key it already holds.
func syncMetadataOnly(ctx context.Context, client *agt.Client, ch *model.Channel, payload agt.ChannelPayload) error {
	if ch.RemoteId == 0 {
		// No key locally and never synced — nothing we can publish.
		_ = ch.MarkFailed("no key available to publish; supplier must re-upload")
		return fmt.Errorf("channel %d has no key and no remote id", ch.Id)
	}
	if err := client.UpdateChannel(ctx, payload); err != nil { // payload.Key empty
		_ = ch.MarkFailed(err.Error())
		return err
	}
	return ch.MarkMetadataSynced()
}
