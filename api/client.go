package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client — клиент HTTP API v1 сервера карты.
type Client struct {
	BaseURL string       // база API, напр. "https://host/api/v1" (без слеша в конце)
	HTTP    *http.Client // может быть nil → DefaultClient
	Version string       // версия трекера, уходит в X-Client-Version

	mu    sync.RWMutex
	token string
}

// NewClient создаёт клиент с разумным таймаутом.
func NewClient(baseURL, version string) *Client {
	log.Printf("API работает на url %s", baseURL)
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		Version: version,
	}
}

// SetToken задаёт токен устройства для Authorization: Bearer.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

func (c *Client) bearer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// do выполняет запрос. payload == nil → без тела, out == nil → тело ответа не разбирается.
func (c *Client) do(ctx context.Context, method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.Version != "" {
		req.Header.Set("X-Client-Version", c.Version)
	}
	if token := c.bearer(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

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
		return newAPIError(resp, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response (status=%d): %w", resp.StatusCode, err)
		}
	}
	return nil
}

func newAPIError(resp *http.Response, raw []byte) *APIError {
	e := &APIError{Status: resp.StatusCode}
	var body errorBody
	if json.Unmarshal(raw, &body) == nil && (body.Detail != "" || body.Code != "") {
		e.Code, e.Detail = body.Code, body.Detail
	} else {
		e.Detail = strings.TrimSpace(string(raw))
		if len(e.Detail) > 200 {
			e.Detail = e.Detail[:200] + "…"
		}
	}
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
		e.RetryAfter = time.Duration(secs) * time.Second
	}
	return e
}

// ---- Рёбра графа ----

// DynamicEdge — переход через портал Авалона (хотя бы один конец — TNL).
func (c *Client) DynamicEdge(ctx context.Context, sourceID, targetID string, createdAt, expiresAt UTCTime) (*EdgeResponse, error) {
	if !expiresAt.After(createdAt) {
		return nil, fmt.Errorf("expires_at must be after created_at")
	}
	var out EdgeResponse
	err := c.do(ctx, http.MethodPost, "/edges/dynamic", DynamicEdgeRequest{
		SourceID: sourceID, TargetID: targetID, CreatedAt: createdAt, ExpiresAt: expiresAt,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// StaticEdge — постоянный переход между двумя открытыми зонами. Идемпотентно.
func (c *Client) StaticEdge(ctx context.Context, sourceID, targetID string, createdAt UTCTime) (*EdgeResponse, error) {
	var out EdgeResponse
	err := c.do(ctx, http.MethodPost, "/edges/static", StaticEdgeRequest{
		SourceID: sourceID, TargetID: targetID, CreatedAt: createdAt,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- Аккаунт ----

// Me проверяет токен и возвращает, кому он принадлежит.
func (c *Client) Me(ctx context.Context) (*MeResponse, error) {
	var out MeResponse
	if err := c.do(ctx, http.MethodGet, "/me", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeviceStart начинает вход через браузер.
func (c *Client) DeviceStart(ctx context.Context, deviceName string) (*DeviceStartResponse, error) {
	var out DeviceStartResponse
	if err := c.do(ctx, http.MethodPost, "/device/start", DeviceStartRequest{DeviceName: deviceName}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DevicePoll спрашивает, подтвердил ли человек вход. Пока не подтвердил — IsPending(err).
func (c *Client) DevicePoll(ctx context.Context, deviceCode string) (*DevicePollResponse, error) {
	var out DevicePollResponse
	if err := c.do(ctx, http.MethodPost, "/device/poll", DevicePollRequest{DeviceCode: deviceCode}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
