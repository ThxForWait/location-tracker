// auth/auth.go
// Пакет auth получает и хранит токен устройства для API.
//
// Токен выдаётся сервером после входа через браузер (device flow):
// трекер показывает код, человек входит на сайт через Discord и подтверждает
// его. Токены хранятся в %APPDATA%\AlbionTracker\config.json отдельно для
// каждого адреса API, чтобы -debug и боевой сервер не мешали друг другу.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"location-tracker/api"
	"location-tracker/config"
)

// Entry — сохранённый токен для одного адреса API.
type Entry struct {
	Token           string `json:"token"`
	DiscordUsername string `json:"discord_username,omitempty"`
}

// File — содержимое config.json.
type File struct {
	Tokens map[string]Entry `json:"tokens"` // адрес API → токен
}

// DefaultPath — %APPDATA%\AlbionTracker\config.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, config.AppDirName, "config.json"), nil
}

// Load читает файл; если его нет — возвращает пустой.
func Load(path string) (File, error) {
	f := File{Tokens: map[string]Entry{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return File{Tokens: map[string]Entry{}}, fmt.Errorf("повреждён %s: %w", path, err)
	}
	if f.Tokens == nil {
		f.Tokens = map[string]Entry{}
	}
	return f, nil
}

// Save записывает файл атомарно (через временный файл), доступ только владельцу.
func Save(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Options — как получить токен при старте.
type Options struct {
	Path        string             // файл с токенами
	ForceLogin  bool               // -login: войти заново, даже если токен есть
	Token       string             // -token: использовать этот токен и не сохранять
	DeviceName  string             // имя устройства на сайте
	OpenBrowser func(string) error // открыть страницу подтверждения
	Out         io.Writer          // куда писать инструкции человеку
}

// EnsureToken ставит клиенту рабочий токен: из флага, из файла или через вход в браузере.
func EnsureToken(ctx context.Context, client *api.Client, opts Options) error {
	if opts.Token != "" {
		client.SetToken(opts.Token)
		return checkToken(ctx, client)
	}

	file, err := Load(opts.Path)
	if err != nil {
		log.Printf("[auth] %v — потребуется войти заново", err)
	}
	if entry, ok := file.Tokens[client.BaseURL]; ok && entry.Token != "" && !opts.ForceLogin {
		client.SetToken(entry.Token)
		err := checkToken(ctx, client)
		if err == nil || !api.IsAuthError(err) || isBanned(err) {
			return err
		}
		log.Printf("[auth] Сохранённый токен больше не действует, нужен повторный вход")
	}

	resp, err := Login(ctx, client, opts)
	if err != nil {
		return err
	}
	file.Tokens[client.BaseURL] = Entry{Token: resp.Token, DiscordUsername: resp.DiscordUsername}
	if err := Save(opts.Path, file); err != nil {
		log.Printf("[auth] Не удалось сохранить токен в %s: %v (при следующем запуске придётся войти снова)", opts.Path, err)
	}
	return nil
}

// checkToken проверяет токен через /me. Если сервер недоступен, работаем дальше:
// отправка сама повторит запросы, а заставлять входить заново из-за сети незачем.
func checkToken(ctx context.Context, client *api.Client) error {
	me, err := client.Me(ctx)
	switch {
	case err == nil:
		log.Printf("[auth] Вход выполнен: %s (%s)", me.DiscordUsername, me.DeviceName)
		return nil
	case api.IsAuthError(err), api.IsOutdated(err):
		return err
	default:
		log.Printf("[auth] Не удалось проверить токен (%v), продолжаю с сохранённым", err)
		return nil
	}
}

func isBanned(err error) bool {
	var e *api.APIError
	return errors.As(err, &e) && e.Status == 403
}

// Login — вход через браузер: показать код, открыть сайт и ждать подтверждения.
func Login(ctx context.Context, client *api.Client, opts Options) (*api.DevicePollResponse, error) {
	client.SetToken("")
	start, err := client.DeviceStart(ctx, opts.DeviceName)
	if err != nil {
		return nil, fmt.Errorf("не удалось начать вход: %w", err)
	}

	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "=== Подключение трекера к аккаунту ===")
	fmt.Fprintf(out, "Откройте в браузере: %s\n", start.VerificationURL)
	fmt.Fprintf(out, "Войдите через Discord и подтвердите код: %s\n", start.UserCode)
	fmt.Fprintln(out)
	if opts.OpenBrowser != nil {
		if err := opts.OpenBrowser(start.VerificationURL); err != nil {
			log.Printf("[auth] Не удалось открыть браузер (%v), откройте ссылку вручную", err)
		}
	}

	interval := time.Duration(start.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(start.ExpiresIn) * time.Second)
	for {
		resp, err := client.DevicePoll(ctx, start.DeviceCode)
		switch {
		case err == nil:
			client.SetToken(resp.Token)
			log.Printf("[auth] Трекер подключён к аккаунту %s", resp.DiscordUsername)
			return resp, nil
		case api.IsPending(err):
			// человек ещё не подтвердил — ждём
		case api.IsRetryable(err):
			if d := api.RetryAfter(err); d > interval {
				interval = d
			}
		default:
			return nil, fmt.Errorf("вход не выполнен: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("код истёк, запустите трекер заново")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// OpenBrowser открывает ссылку в браузере по умолчанию (Windows).
func OpenBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
