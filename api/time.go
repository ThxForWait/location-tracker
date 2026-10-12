package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// UTCTime — время, которое ВСЕГДА маршалится в UTC с суффиксом 'Z',
// независимо от локали машины и того, что передал вызывающий код.
//
// Формат на проводе: RFC3339 без наносекунд, напр. "2026-10-12T14:33:07Z".
type UTCTime struct {
	t time.Time
}

// NewUTCTime нормализует любое время в UTC.
func NewUTCTime(t time.Time) UTCTime {
	return UTCTime{t: t.UTC()}
}

// NowUTC возвращает текущее UTC-время.
func NowUTC() UTCTime {
	return UTCTime{t: time.Now().UTC()}
}

// Add — арифметика без выхода из UTC.
func (u UTCTime) Add(d time.Duration) UTCTime {
	return UTCTime{t: u.t.Add(d)}
}

// Time возвращает внутренний time.Time (уже в UTC).
func (u UTCTime) Time() time.Time { return u.t }

// IsZero для omitempty и проверок.
func (u UTCTime) IsZero() bool { return u.t.IsZero() }

// Before / After — удобные сравнения.
func (u UTCTime) Before(o UTCTime) bool { return u.t.Before(o.t) }
func (u UTCTime) After(o UTCTime) bool  { return u.t.After(o.t) }

// MarshalJSON — точка гарантии UTC.
func (u UTCTime) MarshalJSON() ([]byte, error) {
	s := u.t.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z")
	return []byte(`"` + s + `"`), nil
}

// UnmarshalJSON — принимает 'Z' и '+HH:MM', приводит к UTC.
func (u *UTCTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return fmt.Errorf("invalid time %q: %w", s, err)
		}
	}
	u.t = t.UTC()
	return nil
}

// String — для логов.
func (u UTCTime) String() string { return u.t.UTC().Format(time.RFC3339) }
