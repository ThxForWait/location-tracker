// tracker/sender.go
package tracker

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"location-tracker/api"
	"location-tracker/config"
)

// Transition — переход между двумя зонами, который нужно отправить на сервер.
type Transition struct {
	From       string
	To         string
	Static     bool      // true → StaticEdge, false → DynamicEdge (есть TNL-конец)
	DetectedAt time.Time // момент детекта, уходит на сервер как created_at
}

// edgeAPI — то, что sender использует из api.Client (нужно для тестов).
type edgeAPI interface {
	DynamicEdge(ctx context.Context, sourceID, targetID string, createdAt, expiresAt api.UTCTime) (*api.EdgeResponse, error)
	StaticEdge(ctx context.Context, sourceID, targetID string, createdAt api.UTCTime) (*api.EdgeResponse, error)
}

// Sender отправляет переходы на сервер в отдельной горутине,
// чтобы HTTP-запросы не блокировали разбор пакетов.
// Очередь живёт только в памяти: при выходе неотправленное теряется.
type Sender struct {
	client edgeAPI
	queue  chan Transition
	maxAge time.Duration
	// Отправка остановлена: токен отозван, игрок забанен или трекер устарел.
	// Повторять бессмысленно до перезапуска трекера.
	disabled atomic.Bool

	ctx      context.Context
	cancel   context.CancelFunc
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}

	now        func() time.Time
	retryDelay func(attempt int) time.Duration
}

// NewSender создаёт отправщика и сразу запускает его горутину.
func NewSender(client edgeAPI) *Sender {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Sender{
		client:     client,
		queue:      make(chan Transition, config.SendQueueSize),
		maxAge:     config.SendMaxAge,
		ctx:        ctx,
		cancel:     cancel,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		now:        time.Now,
		retryDelay: backoff,
	}
	go s.run()
	return s
}

// Enqueue ставит переход в очередь. Никогда не блокирует:
// если очередь переполнена, переход отбрасывается.
func (s *Sender) Enqueue(tr Transition) {
	if s.disabled.Load() {
		return
	}
	select {
	case s.queue <- tr:
	default:
		log.Printf("[API] Очередь переполнена, переход %s ➔ %s отброшен", tr.From, tr.To)
	}
}

// Shutdown даёт отправщику до grace на досылку очереди, затем прерывает запросы.
func (s *Sender) Shutdown(grace time.Duration) {
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-time.After(grace):
		s.cancel()
		<-s.done
	}
	s.cancel()
}

func (s *Sender) run() {
	defer close(s.done)
	for {
		select {
		case tr := <-s.queue:
			s.deliver(tr)
		case <-s.stop:
		}
		// select выбирает случайно, поэтому остановку проверяем явно.
		select {
		case <-s.stop:
			s.drain()
			return
		default:
		}
	}
}

// drain отправляет то, что осталось в очереди на момент выхода (по одной попытке).
func (s *Sender) drain() {
	for {
		select {
		case tr := <-s.queue:
			if s.ctx.Err() != nil {
				return
			}
			s.deliver(tr)
		default:
			return
		}
	}
}

func (s *Sender) deliver(tr Transition) {
	for attempt := 1; ; attempt++ {
		if s.disabled.Load() {
			return
		}
		if age := s.now().Sub(tr.DetectedAt); age > s.maxAge {
			log.Printf("[API] Переход %s ➔ %s устарел (%s), не отправлен", tr.From, tr.To, age.Round(time.Second))
			return
		}

		err := s.send(tr)
		if err == nil {
			return
		}

		switch {
		case api.IsAuthError(err):
			s.disable("Токен недействителен или аккаунт заблокирован (%v). "+
				"Отправка на сервер остановлена, перезапустите трекер с флагом -login", err)
			return
		case api.IsOutdated(err):
			s.disable("Трекер устарел (%v). Отправка на сервер остановлена, скачайте новую версию", err)
			return
		case !api.IsRetryable(err):
			// 4xx: данные не прошли проверку сервера — повтор ничего не изменит.
			log.Printf("[API] Сервер отклонил переход %s ➔ %s: %v", tr.From, tr.To, err)
			return
		}

		log.Printf("[API] Ошибка отправки %s ➔ %s (попытка %d): %v", tr.From, tr.To, attempt, err)

		delay := s.retryDelay(attempt)
		if ra := api.RetryAfter(err); ra > delay {
			delay = ra
		}
		select {
		case <-s.stop:
			// Выходим: повторов больше не делаем.
			return
		case <-s.ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (s *Sender) send(tr Transition) error {
	createdAt := api.NewUTCTime(tr.DetectedAt)
	if tr.Static {
		resp, err := s.client.StaticEdge(s.ctx, tr.From, tr.To, createdAt)
		if err != nil {
			return err
		}
		log.Printf("[API] StaticEdge %s ➔ %s: edge_id=%d created=%v", tr.From, tr.To, resp.EdgeID, resp.Created)
		return nil
	}
	resp, err := s.client.DynamicEdge(s.ctx, tr.From, tr.To, createdAt, createdAt.Add(config.PortalLifetime))
	if err != nil {
		return err
	}
	log.Printf("[API] DynamicEdge %s ➔ %s: edge_id=%d created=%v status=%s",
		tr.From, tr.To, resp.EdgeID, resp.Created, resp.Status)
	return nil
}

// disable останавливает отправку и один раз пишет причину в лог.
func (s *Sender) disable(format string, err error) {
	if s.disabled.CompareAndSwap(false, true) {
		log.Printf("[API] "+format, err)
	}
}

// backoff: 2с, 4с, 8с, 16с, дальше 30с.
func backoff(attempt int) time.Duration {
	d := time.Second << attempt
	if attempt >= 5 || d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}
