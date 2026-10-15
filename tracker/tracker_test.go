package tracker

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"location-tracker/api"
	"location-tracker/config"
	"location-tracker/photon"
)

func TestMain(m *testing.M) {
	if err := config.DB.Load("../data/zones.json"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

type fakeSink struct{ got []Transition }

func (f *fakeSink) Enqueue(tr Transition) { f.got = append(f.got, tr) }

func resp(op int64, key byte, zone string) *photon.OperationResponse {
	return &photon.OperationResponse{Parameters: map[byte]any{253: op, key: zone}}
}

func change(zone string) *photon.OperationResponse { return resp(opChangeCluster, 0, zone) }
func join(zone string) *photon.OperationResponse   { return resp(opJoin, 8, zone) }

func TestTransitions(t *testing.T) {
	sink := &fakeSink{}
	tr := newTracker(io.Discard, sink)

	// Старт: первая зона не отправляется.
	tr.OnResponse(join("3356"))
	// Открытая зона → дорога: Connect.
	tr.OnResponse(change("TNL-342"))
	tr.OnResponse(join("TNL-342"))
	// Дубликат ответа 2 игнорируется.
	tr.OnResponse(join("TNL-342"))
	// Расхождение 41 и 2: берём ID из 2 и всё равно отправляем.
	tr.OnResponse(change("TNL-001"))
	tr.OnResponse(join("TNL-057"))
	// Вход в хайдаут и выход из него не отправляются.
	tr.OnResponse(join("@HIDEOUT@4356@299c1d12"))
	tr.OnResponse(join("TNL-057"))
	// Город → город (две статичные зоны): StaticEdge.
	tr.OnResponse(join("1000"))
	tr.OnResponse(join("1006"))

	want := []Transition{
		{From: "3356", To: "TNL-342", Static: false},
		{From: "TNL-342", To: "TNL-057", Static: false},
		{From: "TNL-057", To: "1000", Static: false}, // дорога → город
		{From: "1000", To: "1006", Static: true},
	}

	if len(sink.got) != len(want) {
		t.Fatalf("получено %d переходов, ожидалось %d: %+v", len(sink.got), len(want), sink.got)
	}
	for i, w := range want {
		g := sink.got[i]
		if g.From != w.From || g.To != w.To || g.Static != w.Static {
			t.Errorf("переход %d: получено %+v, ожидалось %+v", i, g, w)
		}
	}
}

func TestZoneKinds(t *testing.T) {
	cases := map[string]config.ZoneKind{
		"TNL-342":               config.ZoneRoad,
		"3356":                  config.ZoneStatic,
		"1000":                  config.ZoneStatic,
		"ISLAND-PLAYER-0001a":   config.ZoneInstance,
		"@HIDEOUT@4356@abc":     config.ZoneInstance,
		"@MISTS@xyz":            config.ZoneInstance,
		"совсем-неизвестная-ID": config.ZoneInstance,
	}
	for id, want := range cases {
		if got := config.DB.GetZoneKind(id); got != want {
			t.Errorf("%s: получено %s, ожидалось %s", id, got, want)
		}
	}
}

// fakeAPI считает вызовы и возвращает заранее заданные ошибки.
type fakeAPI struct {
	mu      sync.Mutex
	errs    []error
	calls   int
	block   chan struct{}
	started chan struct{}
}

func (f *fakeAPI) next() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return err
}

func (f *fakeAPI) Connect(ctx context.Context, _, _ string, _, _ api.UTCTime) (*api.ConnectResponse, error) {
	if f.block != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := f.next(); err != nil {
		return nil, err
	}
	return &api.ConnectResponse{}, nil
}

func (f *fakeAPI) StaticEdge(context.Context, string, string, api.UTCTime) (*api.StaticEdgeResponse, error) {
	if err := f.next(); err != nil {
		return nil, err
	}
	return &api.StaticEdgeResponse{}, nil
}

func (f *fakeAPI) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestSender(a edgeAPI) *Sender {
	s := NewSender(a)
	s.retryDelay = func(int) time.Duration { return time.Millisecond }
	return s
}

func TestSenderRetriesNetworkErrors(t *testing.T) {
	a := &fakeAPI{errs: []error{errors.New("timeout"), errors.New("timeout")}}
	s := newTestSender(a)
	s.Enqueue(Transition{From: "A", To: "B", DetectedAt: time.Now()})
	// При Shutdown повторы не делаются, поэтому сначала ждём, пока попытки пройдут.
	for deadline := time.Now().Add(time.Second); a.Calls() < 3 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	s.Shutdown(time.Second)
	if a.Calls() != 3 {
		t.Fatalf("ожидалось 3 попытки, было %d", a.Calls())
	}
}

func TestSenderNoRetryOnAPIError(t *testing.T) {
	a := &fakeAPI{errs: []error{&api.APIError{Status: 400, Message: "bad"}}}
	s := newTestSender(a)
	s.Enqueue(Transition{From: "A", To: "B", Static: true, DetectedAt: time.Now()})
	s.Shutdown(time.Second)
	if a.Calls() != 1 {
		t.Fatalf("ожидалась 1 попытка, было %d", a.Calls())
	}
}

func TestSenderDropsStale(t *testing.T) {
	a := &fakeAPI{}
	s := newTestSender(a)
	s.Enqueue(Transition{From: "A", To: "B", DetectedAt: time.Now().Add(-6 * time.Minute)})
	s.Shutdown(time.Second)
	if a.Calls() != 0 {
		t.Fatalf("устаревший переход не должен отправляться, было %d вызовов", a.Calls())
	}
}

func TestEnqueueNeverBlocks(t *testing.T) {
	a := &fakeAPI{block: make(chan struct{}), started: make(chan struct{}, 1)}
	s := newTestSender(a)
	s.Enqueue(Transition{From: "A", To: "B", DetectedAt: time.Now()})
	<-a.started // отправщик завис на "медленном сервере"

	done := make(chan struct{})
	go func() {
		for i := 0; i < config.SendQueueSize*2; i++ {
			s.Enqueue(Transition{From: "A", To: "B", DetectedAt: time.Now()})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Enqueue заблокировался при медленном сервере")
	}

	start := time.Now()
	s.Shutdown(50 * time.Millisecond)
	if time.Since(start) > time.Second {
		t.Fatal("Shutdown не уложился в отведённое время")
	}
}
