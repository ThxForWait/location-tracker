package api

// Команды (должны совпадать с сервером).
const (
	CmdConnect    = 1 // динамическое ребро с TNL
	CmdStaticEdge = 2 // статичное ребро между не-TNL зонами
)

// envelope — общая часть ответа сервера.
type envelope struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Command int    `json:"command,omitempty"`
}

// ---- Команда 1: connect ----

type ConnectRequest struct {
	Command   int     `json:"command"`
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	CreatedAt UTCTime `json:"created_at"`
	ExpiresAt UTCTime `json:"expires_at"`
}

type ConnectResponse struct {
	envelope
	EdgeID    int64   `json:"edge_id"`
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	CreatedAt UTCTime `json:"created_at"`
	ExpiresAt UTCTime `json:"expires_at"`
}

// ---- Команда 2: static_edge ----

type StaticEdgeRequest struct {
	Command   int     `json:"command"`
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	CreatedAt UTCTime `json:"created_at"`
}

type StaticEdgeResponse struct {
	envelope
	EdgeID    int64   `json:"edge_id"`
	Created   bool    `json:"created"`
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	CreatedAt UTCTime `json:"created_at"`
}
