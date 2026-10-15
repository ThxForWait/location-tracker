package api

// Модели запросов и ответов API v1 (сервер: albiontrackermap/api_v1.py).

// ---- Рёбра графа ----

// DynamicEdgeRequest — POST /edges/dynamic: переход через портал Авалона.
type DynamicEdgeRequest struct {
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	CreatedAt UTCTime `json:"created_at"`
	ExpiresAt UTCTime `json:"expires_at"`
}

// StaticEdgeRequest — POST /edges/static: постоянный переход.
type StaticEdgeRequest struct {
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	CreatedAt UTCTime `json:"created_at"`
}

// EdgeResponse — ответ на оба запроса рёбер.
type EdgeResponse struct {
	EdgeID   int64  `json:"edge_id"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	// true — ребро создано, false — подтверждено уже существующее
	Created bool `json:"created"`
	// Только для динамических рёбер: unconfirmed / confirmed / disputed
	Status        string `json:"status,omitempty"`
	Confirmations int    `json:"confirmations,omitempty"`
}

// ---- Вход через браузер (device flow) ----

type DeviceStartRequest struct {
	DeviceName string `json:"device_name"`
}

type DeviceStartResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"` // секунды
	Interval        int    `json:"interval"`   // секунды между опросами
}

type DevicePollRequest struct {
	DeviceCode string `json:"device_code"`
}

type DevicePollResponse struct {
	Token           string `json:"token"`
	DiscordUsername string `json:"discord_username"`
}

// MeResponse — GET /me: кому принадлежит токен.
type MeResponse struct {
	DiscordID       string `json:"discord_id"`
	DiscordUsername string `json:"discord_username"`
	DeviceName      string `json:"device_name"`
	IsTrusted       bool   `json:"is_trusted"`
}

// errorBody — тело ошибки сервера: {"detail": "...", "code": "..."}.
type errorBody struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}
