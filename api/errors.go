package api

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError — серверная ошибка (ok=false).
type APIError struct {
	Status  int            // HTTP-код
	Message string         // поле "error"
	Raw     map[string]any // всё тело целиком (missing, vertex_id и т.п.)
}

func (e *APIError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("api error (%d): %s", e.Status, e.Message)
	}
	return "api error: " + e.Message
}

// IsNotFound — true для 404 (unknown vertex).
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// Field — удобный доступ к дополнительным полям ошибки (missing, vertex_id...).
func (e *APIError) Field(name string) (any, bool) {
	v, ok := e.Raw[name]
	return v, ok
}
