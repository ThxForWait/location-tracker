// capture/selector.go
package capture

import (
	"log"
	"time"
)

// Selector решает, с какого интерфейса сейчас читать игровой трафик.
// Читаем только один «активный» интерфейс: так копии одного и того же пакета
// на нескольких интерфейсах (мосты Hyper-V, общий доступ, split-tunnel VPN)
// не задваиваются в парсере. Если активный интерфейс замолчал дольше
// switchAfter, а трафик пошёл по другому, переключаемся на него.
// Не потокобезопасен: вызывается из одной горутины, читающей пакеты.
type Selector struct {
	switchAfter time.Duration
	active      string
	lastSeen    time.Time
}

func NewSelector(switchAfter time.Duration) *Selector {
	return &Selector{switchAfter: switchAfter}
}

// Accept сообщает, нужно ли обработать пакет с интерфейса iface.
// switched == true означает смену активного интерфейса: состояние парсера
// (недособранные фрагменты) нужно сбросить.
func (s *Selector) Accept(iface string, now time.Time) (accept, switched bool) {
	switch {
	case s.active == "":
		log.Printf("[capture] Игровой трафик идёт через: %s", Describe(iface))
		s.active = iface
		s.lastSeen = now
		return true, false
	case iface == s.active:
		s.lastSeen = now
		return true, false
	case now.Sub(s.lastSeen) > s.switchAfter:
		log.Printf("[capture] Переключение: %s ➔ %s", Describe(s.active), Describe(iface))
		s.active = iface
		s.lastSeen = now
		return true, true
	default:
		return false, false
	}
}

// Active возвращает текущий активный интерфейс ("" — ещё не выбран).
func (s *Selector) Active() string { return s.active }
