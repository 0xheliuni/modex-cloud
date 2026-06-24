// Package agt is the client for the downstream AGT platform's new-api-compatible
// channel API. Request/response shapes follow AGT平台接口文档.txt exactly:
//
//	POST /api/channel/  — wrapped: {mode, multi_key_mode, channel:{...}}  -> data.id
//	PUT  /api/channel/  — flat channel object with top-level id           -> data
//
// The bearer token is the platform's AGT access token (decrypted by the caller
// in the sync worker). This package never stores or logs the token or the key.
package agt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modex/agt-vault/common"
)

// Client talks to one AGT platform.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a client for a platform base URL + bearer token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// ChannelPayload is the inner channel object common to create/update.
type ChannelPayload struct {
	Id      int    `json:"id,omitempty"`
	Name    string `json:"name"`
	Type    int    `json:"type"`
	Key     string `json:"key,omitempty"` // omitted on metadata-only updates (AGT preserves it)
	BaseURL string `json:"base_url"`
	Models  string `json:"models"`
	Group   string `json:"group"`
}

type createEnvelope struct {
	Mode         string         `json:"mode"`
	MultiKeyMode string         `json:"multi_key_mode"`
	Channel      ChannelPayload `json:"channel"`
}

// apiResponse is the AGT standard envelope.
type apiResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Id  int   `json:"id"`
		Ids []int `json:"ids"`
	} `json:"data"`
}

// CreateChannel issues POST /api/channel/ in AGT's wrapped format and returns
// the remote template id.
func (c *Client) CreateChannel(ctx context.Context, p ChannelPayload) (int, error) {
	body, err := common.Marshal(createEnvelope{
		Mode:         "single",
		MultiKeyMode: "random",
		Channel:      p,
	})
	if err != nil {
		return 0, err
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/channel/", body)
	if err != nil {
		return 0, err
	}
	if !resp.Success {
		return 0, fmt.Errorf("agt rejected create: %s", resp.Message)
	}
	if resp.Data.Id == 0 {
		return 0, fmt.Errorf("agt create returned no id")
	}
	return resp.Data.Id, nil
}

// UpdateChannel issues PUT /api/channel/ in AGT's flat format. Omit p.Key to keep
// the existing upstream key (AGT preserves it when key is absent).
func (c *Client) UpdateChannel(ctx context.Context, p ChannelPayload) error {
	if p.Id == 0 {
		return fmt.Errorf("update requires a top-level id")
	}
	body, err := common.Marshal(p) // flat object, id at top level
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodPut, "/api/channel/", body)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("agt rejected update: %s", resp.Message)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*apiResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agt request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("agt server error: HTTP %d", resp.StatusCode)
	}
	var out apiResponse
	if err := common.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("agt returned non-JSON (HTTP %d)", resp.StatusCode)
	}
	return &out, nil
}
