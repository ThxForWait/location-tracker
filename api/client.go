package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"log"
)

// Client — тонкая обёртка над HTTP API.
type Client struct {
	BaseURL string       // полный URL endpoint'а, напр. "https://host/graph/api/packet/"
	HTTP    *http.Client // может быть nil → DefaultClient
}

// New создаёт клиент с разумным таймаутом.
func NewClient(baseURL string) *Client {
	log.Printf("Api работает на url %s", baseURL)
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// sendPacket — низкоуровневый вызов. payload должен содержать "command".
func (c *Client) sendPacket(ctx context.Context, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	cli := c.HTTP
	if cli == nil {
		cli = http.DefaultClient
	}

	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bad status %d, body=%q", resp.StatusCode, string(raw))
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode response (status=%d): %w, body=%s", resp.StatusCode, err, raw)
	}
	if !env.OK {
		var rawMap map[string]any
		_ = json.Unmarshal(raw, &rawMap)
		return &APIError{Status: resp.StatusCode, Message: env.Error, Raw: rawMap}
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status %d with ok=true", resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode ok response: %w", err)
		}
	}
	return nil
}

// Connect — команда 1. Хотя бы один из концов должен быть TNL-зоной.
func (c *Client) Connect(
	ctx context.Context,
	sourceID, targetID string,
	createdAt, expiresAt UTCTime,
) (*ConnectResponse, error) {
	if !expiresAt.After(createdAt) {
		return nil, fmt.Errorf("expires_at must be after created_at")
	}
	req := ConnectRequest{
		Command:   CmdConnect,
		SourceID:  sourceID,
		TargetID:  targetID,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	}
	var out ConnectResponse
	if err := c.sendPacket(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StaticEdge — команда 2. Идемпотентно.
func (c *Client) StaticEdge(
	ctx context.Context,
	sourceID, targetID string,
	createdAt UTCTime,
) (*StaticEdgeResponse, error) {
	req := StaticEdgeRequest{
		Command:   CmdStaticEdge,
		SourceID:  sourceID,
		TargetID:  targetID,
		CreatedAt: createdAt,
	}
	var out StaticEdgeResponse
	if err := c.sendPacket(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
