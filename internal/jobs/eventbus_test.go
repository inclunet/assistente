package jobs

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_SubscribeAndPublish(t *testing.T) {
	eb := NewEventBus()
	var received atomic.Value
	done := make(chan struct{})

	eb.Subscribe("test.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		received.Store(payload["key"])
		close(done)
	})

	eb.Publish(context.Background(), "test.event", map[string]any{"key": "value"})

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	if received.Load() != "value" {
		t.Errorf("expected 'value', got %v", received.Load())
	}
}

// TestEventBus_PublishNilPayload garante que publicar com payload nil não causa
// panic no guard de clip de _chain_history. Regressão do review #170.
func TestEventBus_PublishNilPayload(t *testing.T) {
	eb := NewEventBus()
	done := make(chan map[string]any, 1)
	eb.Subscribe("nil.event", "sub-1", func(_ context.Context, _ string, payload map[string]any) {
		done <- payload
	})

	eb.Publish(context.Background(), "nil.event", nil)

	select {
	case payload := <-done:
		if payload != nil {
			t.Fatalf("payload = %v, want nil (preservado)", payload)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		id := i
		eb.Subscribe("multi.event", "sub-"+string(rune('a'+id)), func(ctx context.Context, name string, payload map[string]any) {
			count.Add(1)
			wg.Done()
		})
	}

	eb.Publish(context.Background(), "multi.event", map[string]any{})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for all subscribers")
	}

	if got := int(count.Load()); got != 3 {
		t.Errorf("expected 3 calls, got %d", got)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32

	eb.Subscribe("unsub.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
	})

	eb.Unsubscribe("unsub.event", "sub-1")
	eb.Publish(context.Background(), "unsub.event", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("expected 0 calls after unsubscribe, got %d", got)
	}
}

func TestEventBus_UnsubscribeAll(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32

	handler := func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
	}

	eb.Subscribe("event.a", "job-1", handler)
	eb.Subscribe("event.b", "job-1", handler)

	eb.UnsubscribeAll("job-1")

	eb.Publish(context.Background(), "event.a", map[string]any{})
	eb.Publish(context.Background(), "event.b", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("expected 0 calls after UnsubscribeAll, got %d", got)
	}
}

func TestEventBus_PublishNoListeners(t *testing.T) {
	eb := NewEventBus()
	if delivered := eb.Publish(context.Background(), "no.listeners", map[string]any{}); delivered {
		t.Fatal("evento sem consumidor foi marcado como entregue")
	}
	if got := eb.Stats().EventsDropped; got != 1 {
		t.Fatalf("events_dropped = %d, want 1", got)
	}
}

// levelCaptureHandler captura o nível de cada record emitido, para asserir em
// que severidade um evento descartado é logado.
type levelCaptureHandler struct {
	mu      sync.Mutex
	byEvent map[string]slog.Level
}

func (h *levelCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *levelCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	var name string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "event_name" {
			name = a.Value.String()
			return false
		}
		return true
	})
	h.mu.Lock()
	h.byEvent[name] = r.Level
	h.mu.Unlock()
	return nil
}

func (h *levelCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *levelCaptureHandler) WithGroup(string) slog.Handler            { return h }

// TestEventBus_DroppedSuccessLogsDebugOthersWarn garante que o descarte de um
// evento `.success` sem consumidor (semântica normal de pub/sub em job terminal)
// é logado em DEBUG, enquanto qualquer outro evento sem listener permanece em
// WARN (possível cadeia quebrada). Em ambos, o contador segue incrementando.
func TestEventBus_DroppedSuccessLogsDebugOthersWarn(t *testing.T) {
	capture := &levelCaptureHandler{byEvent: make(map[string]slog.Level)}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	defer slog.SetDefault(previous)

	eb := NewEventBus()
	eb.Publish(context.Background(), "job.terminal.success", map[string]any{})
	eb.Publish(context.Background(), "job.terminal.failure", map[string]any{})

	capture.mu.Lock()
	defer capture.mu.Unlock()
	if got, ok := capture.byEvent["job.terminal.success"]; !ok || got != slog.LevelDebug {
		t.Fatalf("drop de `.success` = nível %v (presente=%v), want DEBUG", got, ok)
	}
	if got, ok := capture.byEvent["job.terminal.failure"]; !ok || got != slog.LevelWarn {
		t.Fatalf("drop de não-`.success` = nível %v (presente=%v), want WARN", got, ok)
	}
	if got := eb.Stats().EventsDropped; got != 2 {
		t.Fatalf("events_dropped = %d, want 2 (contador preservado independe do nível)", got)
	}
}

func TestEventBus_DroppedEventWarningIsThrottledPerEvent(t *testing.T) {
	eb := NewEventBus()
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	eb.now = func() time.Time { return now }

	// perEvent conta por nome de evento; total é o contador global. O WARN é
	// throttled por nome de evento.
	if perEvent, total, warn := eb.recordDropped("pipeline.card"); perEvent != 1 || total != 1 || !warn {
		t.Fatalf("1º descarte = (perEvent=%d, total=%d, warn=%v), want (1, 1, true)", perEvent, total, warn)
	}
	if perEvent, total, warn := eb.recordDropped("pipeline.card"); perEvent != 2 || total != 2 || warn {
		t.Fatalf("descarte dentro do throttle = (perEvent=%d, total=%d, warn=%v), want (2, 2, false)", perEvent, total, warn)
	}
	// Outro evento: perEvent reinicia em 1 (honesto), mas o total global segue.
	if perEvent, total, warn := eb.recordDropped("outro.evento"); perEvent != 1 || total != 3 || !warn {
		t.Fatalf("1º descarte de outro evento = (perEvent=%d, total=%d, warn=%v), want (1, 3, true)", perEvent, total, warn)
	}

	now = now.Add(droppedEventWarningInterval)
	if perEvent, total, warn := eb.recordDropped("pipeline.card"); perEvent != 3 || total != 4 || !warn {
		t.Fatalf("descarte após throttle = (perEvent=%d, total=%d, warn=%v), want (3, 4, true)", perEvent, total, warn)
	}
}

func TestEventBus_HandlerPanicDoesNotCrash(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	eb.Subscribe("panic.event", "panic-sub", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
		wg.Done()
		panic("boom")
	})

	eb.Subscribe("panic.event", "safe-sub", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
		wg.Done()
	})

	eb.Publish(context.Background(), "panic.event", map[string]any{})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: panic in one handler should not affect others")
	}

	if got := int(count.Load()); got != 2 {
		t.Errorf("expected 2 handlers called, got %d", got)
	}
}

func TestEventBus_CloseStopsPublish(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32

	eb.Subscribe("close.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
	})

	eb.Close()
	eb.Publish(context.Background(), "close.event", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("expected 0 after Close, got %d", got)
	}
}

func TestEventBus_CloseStopsSubscribe(t *testing.T) {
	eb := NewEventBus()
	eb.Close()

	eb.Subscribe("post.close", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		t.Error("should not be called")
	})

	if count := eb.SubscriberCount("post.close"); count != 0 {
		t.Errorf("expected 0 subscribers after Close, got %d", count)
	}
}

func TestEventBus_SubscriberCount(t *testing.T) {
	eb := NewEventBus()

	eb.Subscribe("count.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {})
	eb.Subscribe("count.event", "sub-2", func(ctx context.Context, name string, payload map[string]any) {})

	if got := eb.SubscriberCount("count.event"); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}

	if got := eb.SubscriberCount("other.event"); got != 0 {
		t.Errorf("expected 0 for unknown event, got %d", got)
	}
}

// TestEventBus_CloseDrainsInFlightHandlers garante que Close() bloqueia até as
// goroutines de fan-out em voo terminarem (shutdown gracioso). É a barreira que
// impede um handler (ex.: execução de job) de sobreviver ao Stop e escrever no
// estado global depois — em testes, no DB de outro teste após o swap do
// singleton database.DB(), causando o flake de isolamento entre pacotes.
func TestEventBus_CloseDrainsInFlightHandlers(t *testing.T) {
	eb := NewEventBus()
	started := make(chan struct{})
	var finished atomic.Bool

	eb.Subscribe("evt", "sub", func(_ context.Context, _ string, _ map[string]any) {
		close(started)
		time.Sleep(80 * time.Millisecond)
		finished.Store(true)
	})

	eb.Publish(context.Background(), "evt", nil)
	<-started // garante que o handler começou antes de fechar

	eb.Close() // deve bloquear até o handler em voo terminar

	if !finished.Load() {
		t.Fatal("Close retornou antes do handler em voo terminar (não drenou o fan-out)")
	}
}

func TestEventBus_Events(t *testing.T) {
	eb := NewEventBus()

	eb.Subscribe("alpha", "sub", func(ctx context.Context, name string, payload map[string]any) {})
	eb.Subscribe("beta", "sub", func(ctx context.Context, name string, payload map[string]any) {})

	events := eb.Events()
	found := make(map[string]bool)
	for _, e := range events {
		found[e] = true
	}

	if !found["alpha"] || !found["beta"] {
		t.Errorf("expected alpha and beta, got %v", events)
	}
}
