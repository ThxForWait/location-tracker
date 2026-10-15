package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDynamicEdgeRequest(t *testing.T) {
	var got struct {
		path, auth, version string
		body                DynamicEdgeRequest
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.version = r.Header.Get("X-Client-Version")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"edge_id": 7, "source_id": "TNL-001", "target_id": "TNL-002", "created": true, "status": "unconfirmed", "confirmations": 1}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/api/v1/", "1.2.3")
	c.SetToken("atm_secret")
	now := NowUTC()
	resp, err := c.DynamicEdge(context.Background(), "TNL-002", "TNL-001", now, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if got.path != "/api/v1/edges/dynamic" {
		t.Errorf("путь %q", got.path)
	}
	if got.auth != "Bearer atm_secret" || got.version != "1.2.3" {
		t.Errorf("заголовки: auth=%q version=%q", got.auth, got.version)
	}
	if got.body.SourceID != "TNL-002" || got.body.TargetID != "TNL-001" {
		t.Errorf("тело %+v", got.body)
	}
	if resp.EdgeID != 7 || !resp.Created || resp.Status != "unconfirmed" {
		t.Errorf("ответ %+v", resp)
	}
}

func TestErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail": "Слишком много запросов", "code": "rate_limited"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "1").Me(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась APIError, получено %v", err)
	}
	if apiErr.Status != 429 || apiErr.Code != "rate_limited" || apiErr.RetryAfter != 17*time.Second {
		t.Errorf("%+v", apiErr)
	}
	if !IsRetryable(err) || RetryAfter(err) != 17*time.Second {
		t.Error("429 должен повторяться с учётом Retry-After")
	}
}

func TestErrorKinds(t *testing.T) {
	cases := []struct {
		status                             int
		retryable, auth, outdated, pending bool
	}{
		{401, false, true, false, false},
		{403, false, true, false, false},
		{409, false, false, false, false},
		{422, false, false, false, false},
		{426, false, false, true, false},
		{428, false, false, false, true},
		{429, true, false, false, false},
		{500, true, false, false, false},
		{503, true, false, false, false},
	}
	for _, c := range cases {
		err := error(&APIError{Status: c.status})
		if IsRetryable(err) != c.retryable || IsAuthError(err) != c.auth ||
			IsOutdated(err) != c.outdated || IsPending(err) != c.pending {
			t.Errorf("статус %d классифицирован неверно", c.status)
		}
	}
	if !IsRetryable(errors.New("connection refused")) {
		t.Error("сетевая ошибка должна повторяться")
	}
	if IsRetryable(context.Canceled) {
		t.Error("отмена не должна повторяться")
	}
}

func TestNonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "1").Me(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 502 || apiErr.Detail != "Bad Gateway" {
		t.Fatalf("%v", err)
	}
}
