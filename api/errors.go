package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// APIError — ответ сервера с кодом не 2xx.
type APIError struct {
	Status     int           // HTTP-код
	Code       string        // машинный код из тела, например "unknown_zone"
	Detail     string        // описание для человека
	RetryAfter time.Duration // из заголовка Retry-After (для 429)
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("api error %d %s: %s", e.Status, e.Code, e.Detail)
	}
	return fmt.Sprintf("api error %d: %s", e.Status, e.Detail)
}

func asAPIError(err error) (*APIError, bool) {
	var e *APIError
	ok := errors.As(err, &e)
	return e, ok
}

// IsRetryable — стоит ли повторить запрос позже:
// сетевые ошибки, 5xx и 429 — да; остальные 4xx — нет (повтор ничего не изменит).
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	e, ok := asAPIError(err)
	if !ok {
		return true // сеть, таймаут, DNS
	}
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// IsAuthError — токена нет, он отозван (401) или игрок заблокирован (403).
func IsAuthError(err error) bool {
	e, ok := asAPIError(err)
	return ok && (e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden)
}

// IsOutdated — сервер требует более новую версию трекера (426).
func IsOutdated(err error) bool {
	e, ok := asAPIError(err)
	return ok && e.Status == http.StatusUpgradeRequired
}

// IsPending — вход через браузер ещё не подтверждён (428).
func IsPending(err error) bool {
	e, ok := asAPIError(err)
	return ok && e.Status == http.StatusPreconditionRequired
}

// RetryAfter — сколько сервер просил подождать (0, если не просил).
func RetryAfter(err error) time.Duration {
	if e, ok := asAPIError(err); ok {
		return e.RetryAfter
	}
	return 0
}
