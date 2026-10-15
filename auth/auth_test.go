package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"location-tracker/api"
)

// fakeServer — минимальный сервер с device flow и /me.
type fakeServer struct {
	validToken string
	polls      atomic.Int32 // сколько раз ответить «ещё ждём»
	logins     atomic.Int32
}

func (f *fakeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+f.validToken {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"detail":"нет","code":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"discord_username":"alice","device_name":"ПК"}`))
	})
	mux.HandleFunc("/device/start", func(w http.ResponseWriter, r *http.Request) {
		f.logins.Add(1)
		_, _ = w.Write([]byte(`{"device_code":"dev","user_code":"ABC-DEF","verification_url":"http://site/link/?code=ABC-DEF","expires_in":60,"interval":0}`))
	})
	mux.HandleFunc("/device/poll", func(w http.ResponseWriter, r *http.Request) {
		var body api.DevicePollRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.DeviceCode != "dev" {
			w.WriteHeader(404)
			return
		}
		if f.polls.Add(-1) >= 0 {
			w.WriteHeader(428)
			_, _ = w.Write([]byte(`{"detail":"ждём","code":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"token":"` + f.validToken + `","discord_username":"alice"}`))
	})
	return mux
}

func setup(t *testing.T) (*fakeServer, *api.Client, Options, *[]string) {
	t.Helper()
	fake := &fakeServer{validToken: "atm_new"}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)

	opened := &[]string{}
	opts := Options{
		Path:        filepath.Join(t.TempDir(), "AlbionTracker", "config.json"),
		DeviceName:  "ПК",
		OpenBrowser: func(url string) error { *opened = append(*opened, url); return nil },
		Out:         io.Discard,
	}
	return fake, api.NewClient(srv.URL, "1.0.0"), opts, opened
}

func TestFirstRunLogsInAndSaves(t *testing.T) {
	fake, client, opts, opened := setup(t)
	fake.polls.Store(2) // два раза «ещё ждём»

	if err := EnsureToken(context.Background(), client, opts); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 || (*opened)[0] != "http://site/link/?code=ABC-DEF" {
		t.Errorf("браузер: %v", *opened)
	}
	file, _ := Load(opts.Path)
	if file.Tokens[client.BaseURL].Token != "atm_new" {
		t.Errorf("токен не сохранён: %+v", file)
	}
}

func TestSavedValidTokenSkipsLogin(t *testing.T) {
	fake, client, opts, _ := setup(t)
	_ = Save(opts.Path, File{Tokens: map[string]Entry{client.BaseURL: {Token: "atm_new"}}})

	if err := EnsureToken(context.Background(), client, opts); err != nil {
		t.Fatal(err)
	}
	if fake.logins.Load() != 0 {
		t.Error("при рабочем токене вход не нужен")
	}
}

func TestRevokedTokenTriggersLogin(t *testing.T) {
	fake, client, opts, _ := setup(t)
	_ = Save(opts.Path, File{Tokens: map[string]Entry{
		client.BaseURL:        {Token: "atm_revoked"},
		"http://other/api/v1": {Token: "atm_other"},
	}})

	if err := EnsureToken(context.Background(), client, opts); err != nil {
		t.Fatal(err)
	}
	if fake.logins.Load() != 1 {
		t.Error("отозванный токен должен вести к повторному входу")
	}
	file, _ := Load(opts.Path)
	if file.Tokens[client.BaseURL].Token != "atm_new" || file.Tokens["http://other/api/v1"].Token != "atm_other" {
		t.Errorf("файл после входа: %+v", file)
	}
}

func TestForceLogin(t *testing.T) {
	fake, client, opts, _ := setup(t)
	_ = Save(opts.Path, File{Tokens: map[string]Entry{client.BaseURL: {Token: "atm_new"}}})
	opts.ForceLogin = true

	if err := EnsureToken(context.Background(), client, opts); err != nil {
		t.Fatal(err)
	}
	if fake.logins.Load() != 1 {
		t.Error("-login должен входить заново")
	}
}

func TestFlagTokenIsNotSaved(t *testing.T) {
	fake, client, opts, _ := setup(t)
	opts.Token = "atm_new"

	if err := EnsureToken(context.Background(), client, opts); err != nil {
		t.Fatal(err)
	}
	file, _ := Load(opts.Path)
	if fake.logins.Load() != 0 || len(file.Tokens) != 0 {
		t.Error("токен из флага используется как есть и не сохраняется")
	}

	opts.Token = "atm_bad"
	if err := EnsureToken(context.Background(), client, opts); !api.IsAuthError(err) {
		t.Errorf("неверный токен из флага должен давать ошибку, получено %v", err)
	}
}
