// tracker/tracker.go
package tracker

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"location-tracker/config"
	"location-tracker/photon"
)

const unknownZone = "unknown"

// Коды операций Albion, по которым определяется переход.
const (
	opChangeCluster = 41 // начало перехода (до экрана загрузки), param 0 — ID зоны
	opJoin          = 2  // окончательный вход в зону после загрузки, param 8 — ID зоны
)

// transitionSink — куда трекер отдаёт переходы для отправки (Sender).
type transitionSink interface {
	Enqueue(Transition)
}

// Tracker обрабатывает события от парсера и записывает переходы в файл.
type Tracker struct {
	logOut  io.Writer
	logFile *os.File
	mu      sync.Mutex
	// currentZoneID — зона, в которой игрок сейчас (подтверждена ответом 2).
	currentZoneID string
	// pendingZoneID — зона, ожидаемая после ответа 41; нужна только для сверки.
	pendingZoneID string
	sink          transitionSink
	now           func() time.Time
}

// NewTracker создает новый трекер и открывает файл для записи.
func NewTracker(sink transitionSink) (*Tracker, error) {
	file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл лога: %w", err)
	}

	log.Printf("Логирование переходов будет вестись в файл: %s", config.LogFilePath)

	t := newTracker(file, sink)
	t.logFile = file
	return t, nil
}

func newTracker(logOut io.Writer, sink transitionSink) *Tracker {
	return &Tracker{
		logOut:        logOut,
		currentZoneID: unknownZone,
		sink:          sink,
		now:           time.Now,
	}
}

func (t *Tracker) OnResponse(resp *photon.OperationResponse) {
	albionOp := photon.GetInt(resp.Parameters, 253)
	if albionOp == 0 {
		albionOp = int64(resp.OperationCode)
	}
	switch albionOp {
	case opChangeCluster:
		clusterId := photon.GetStr(resp.Parameters, 0)
		if clusterId == "" {
			log.Printf("Ошибка в считывании ID локации, OpCode 41")
			return
		}
		t.mu.Lock()
		t.pendingZoneID = clusterId
		t.mu.Unlock()
	case opJoin:
		clusterId := photon.GetStr(resp.Parameters, 8)
		if clusterId == "" {
			log.Printf("Ошибка в считывании ID локации, OpCode 2")
			return
		}
		t.enterZone(clusterId)
	}
}

// enterZone фиксирует вход в зону по ответу 2 — он источник истины.
func (t *Tracker) enterZone(newZoneID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.pendingZoneID != "" && t.pendingZoneID != newZoneID {
		log.Printf("Предупреждение: ID в OpCode 41 (%s) и 2 (%s) не совпали, используется ID из 2",
			t.pendingZoneID, newZoneID)
	}
	t.pendingZoneID = ""

	// Игнорируем дубликаты: сервер может прислать тот же ID повторно.
	if newZoneID == t.currentZoneID {
		return
	}

	oldZoneID := t.currentZoneID
	t.currentZoneID = newZoneID
	now := t.now()

	tr, send, reason := classifyTransition(oldZoneID, newZoneID, now)

	oldZoneName := config.DB.GetZoneName(oldZoneID)
	newZoneName := config.DB.GetZoneName(newZoneID)
	log.Printf("[ПЕРЕХОД] %s ➔ %s (%s)", oldZoneName, newZoneName, reason)

	logLine := fmt.Sprintf("%s | %s [%s] ➔ %s [%s] | %s\n",
		now.Format("2006-01-02 15:04:05"), oldZoneName, oldZoneID, newZoneName, newZoneID, reason)
	if _, err := io.WriteString(t.logOut, logLine); err != nil {
		log.Printf("Ошибка записи в файл лога: %v", err)
	}

	// Enqueue не блокирует, так что сеть никогда не держит mutex и разбор пакетов.
	if send {
		t.sink.Enqueue(tr)
	}
}

// classifyTransition решает, отправлять ли переход и каким типом ребра.
func classifyTransition(fromID, toID string, at time.Time) (Transition, bool, string) {
	if fromID == unknownZone {
		return Transition{}, false, "старт, не отправляется"
	}
	from := config.DB.GetZoneKind(fromID)
	to := config.DB.GetZoneKind(toID)
	if from == config.ZoneCity || to == config.ZoneCity {
		return Transition{}, false, "город, не отправляется"
	}
	if from == config.ZoneInstance || to == config.ZoneInstance {
		return Transition{}, false, "инстанс, не отправляется"
	}
	static := from == config.ZoneStatic && to == config.ZoneStatic
	tr := Transition{From: fromID, To: toID, Static: static, DetectedAt: at}
	if static {
		return tr, true, "static"
	}
	return tr, true, "connect"
}

// Close закрывает файл лога.
func (t *Tracker) Close() error {
	if t.logFile == nil {
		return nil
	}
	return t.logFile.Close()
}
