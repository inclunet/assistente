package connstatus

import (
	"context"
	"sync"
	"testing"
	"time"
)

// captureEmitter coleta todos os eventos emitidos para inspeção nos testes.
type captureEmitter struct {
	mu     sync.Mutex
	events []Event
}

func (e *captureEmitter) Emit(_ string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ev, ok := data.(Event); ok {
		e.events = append(e.events, ev)
	}
}

func (e *captureEmitter) snapshot() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Event, len(e.events))
	copy(out, e.events)
	return out
}

// TestMonitor_EmitsEventName valida que o evento emitido usa o nome de evento
// contratual (consumido pelo frontend).
func TestMonitor_EmitsEventName(t *testing.T) {
	var gotName string
	emitter := emitterFunc(func(name string, _ any) { gotName = name })

	m := New(func(context.Context) Snapshot {
		return Snapshot{State: StateOnline, LatencyMs: 10}
	}, emitter, time.Hour)

	m.checkOnce(context.Background())
	if gotName != EventName {
		t.Fatalf("nome do evento: got %q, want %q", gotName, EventName)
	}
}

// TestMonitor_StateChangeReflectedInEvent garante que o estado da sondagem é
// propagado para o evento.
func TestMonitor_StateChangeReflectedInEvent(t *testing.T) {
	emitter := &captureEmitter{}
	state := StateOnline
	m := New(func(context.Context) Snapshot {
		return Snapshot{State: state, ProviderID: "p1", ProviderName: "Prov", LatencyMs: 42}
	}, emitter, time.Hour)

	m.checkOnce(context.Background())
	state = StateOffline
	m.checkOnce(context.Background())

	events := emitter.snapshot()
	// 1º online, depois (offline anterior? não) → 2ª checkOnce: estado anterior
	// era online, então NÃO emite "checking"; emite offline direto.
	if len(events) != 2 {
		t.Fatalf("esperava 2 eventos, got %d (%+v)", len(events), events)
	}
	if events[0].State != StateOnline || events[1].State != StateOffline {
		t.Fatalf("sequência de estados inesperada: %q, %q", events[0].State, events[1].State)
	}
}

// TestMonitor_ReconnectingEmittedWhenWasOffline: quando o último estado era
// offline, a próxima verificação emite primeiro um evento "checking"
// (reconectando) e depois o resultado.
func TestMonitor_ReconnectingEmittedWhenWasOffline(t *testing.T) {
	emitter := &captureEmitter{}
	state := StateOffline
	m := New(func(context.Context) Snapshot {
		return Snapshot{State: state, ProviderID: "p1"}
	}, emitter, time.Hour)

	m.checkOnce(context.Background()) // offline
	state = StateOnline
	m.checkOnce(context.Background()) // checking + online

	events := emitter.snapshot()
	if len(events) != 3 {
		t.Fatalf("esperava 3 eventos (offline, checking, online), got %d (%+v)", len(events), events)
	}
	if events[1].State != StateChecking {
		t.Fatalf("segundo evento deveria ser checking, got %q", events[1].State)
	}
	if events[2].State != StateOnline {
		t.Fatalf("terceiro evento deveria ser online, got %q", events[2].State)
	}
}

// TestMonitor_AverageLatency calcula a média apenas sobre amostras online.
func TestMonitor_AverageLatency(t *testing.T) {
	emitter := &captureEmitter{}
	latencies := []int64{100, 200, 300}
	idx := 0
	m := New(func(context.Context) Snapshot {
		l := latencies[idx]
		idx++
		return Snapshot{State: StateOnline, LatencyMs: l}
	}, emitter, time.Hour)

	for range latencies {
		m.checkOnce(context.Background())
	}

	events := emitter.snapshot()
	last := events[len(events)-1]
	if last.AvgLatencyMs != 200 {
		t.Fatalf("média esperada 200, got %d", last.AvgLatencyMs)
	}
}

// TestMonitor_OfflineLatencyNotAveraged: latências de sondagens offline não
// poluem a média.
func TestMonitor_OfflineLatencyNotAveraged(t *testing.T) {
	emitter := &captureEmitter{}
	snaps := []Snapshot{
		{State: StateOnline, LatencyMs: 100},
		{State: StateOffline, LatencyMs: 15000},
	}
	idx := 0
	m := New(func(context.Context) Snapshot {
		s := snaps[idx]
		idx++
		return s
	}, emitter, time.Hour)

	m.checkOnce(context.Background())
	m.checkOnce(context.Background())

	events := emitter.snapshot()
	last := events[len(events)-1]
	if last.AvgLatencyMs != 100 {
		t.Fatalf("média deveria ignorar a latência offline; esperava 100, got %d", last.AvgLatencyMs)
	}
}

// TestMonitor_RunStopsOnContextCancel garante que Run respeita o cancelamento.
func TestMonitor_RunStopsOnContextCancel(t *testing.T) {
	emitter := &captureEmitter{}
	m := New(func(context.Context) Snapshot {
		return Snapshot{State: StateOnline, LatencyMs: 1}
	}, emitter, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()

	time.Sleep(35 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run não retornou após cancelamento do contexto")
	}

	if _, ok := m.Last(); !ok {
		t.Fatal("esperava ao menos uma verificação registrada")
	}
}

// emitterFunc adapta uma função simples para a interface Emitter.
type emitterFunc func(event string, data any)

func (f emitterFunc) Emit(event string, data any) { f(event, data) }
