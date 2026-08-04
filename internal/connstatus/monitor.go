// Package connstatus implementa o monitor periódico de status de conexão com
// o provider/API LLM ativo. Ele NÃO conhece Wails: recebe uma CheckFunc (que
// reaproveita providers.Service.CheckHealth) e um Emitter abstrato, emitindo
// eventos tipados que o frontend escuta para atualizar o indicador da topbar.
package connstatus

import (
	"context"
	"sync"
	"time"
)

// EventName é o nome do evento Wails emitido a cada ciclo de verificação.
const EventName = "llm:connection-status"

// DefaultInterval é o intervalo padrão entre health checks periódicos.
const DefaultInterval = 30 * time.Second

// maxLatencySamples limita a janela usada para calcular a latência média.
const maxLatencySamples = 10

// Estados possíveis reportados ao frontend.
const (
	StateOnline   = "online"
	StateOffline  = "offline"
	StateChecking = "checking" // sondagem em andamento (usado para "reconectando")
	// StateUnauthenticated é o provedor de agente de código que está de pé mas
	// sem login. Não é offline: nada há para consertar no app, e a saída é
	// rodar o login do CLI do agente (AEP-0084 D12).
	StateUnauthenticated = "unauthenticated"
)

// Emitter abstrai o envio de eventos para o frontend (Wails runtime).
type Emitter interface {
	Emit(event string, data any)
}

// Snapshot é o resultado bruto de uma sondagem de saúde, sem metadados de
// série temporal (latência média, timestamp) que o Monitor agrega.
type Snapshot struct {
	State        string
	ProviderID   string
	ProviderName string
	Model        string
	LatencyMs    int64
	Error        string
	ErrorType    string
}

// Event é o payload tipado do evento llm:connection-status.
type Event struct {
	State        string `json:"state"`
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName"`
	Model        string `json:"model,omitempty"`
	LatencyMs    int64  `json:"latencyMs"`
	AvgLatencyMs int64  `json:"avgLatencyMs"`
	Error        string `json:"error,omitempty"`
	ErrorType    string `json:"errorType,omitempty"`
	Timestamp    int64  `json:"timestamp"`
}

// CheckFunc executa uma sondagem de saúde e devolve um Snapshot.
type CheckFunc func(ctx context.Context) Snapshot

// Monitor executa health checks periódicos e emite eventos de status.
type Monitor struct {
	interval time.Duration
	check    CheckFunc
	emitter  Emitter
	now      func() time.Time

	mu      sync.Mutex
	last    Event
	hasLast bool
	samples []int64
	// samplesProviderID identifica a qual provider a janela de latência
	// pertence. Ao trocar de provider ativo, a janela é resetada para a média
	// não misturar latências de endpoints distintos.
	samplesProviderID string
}

// New cria um Monitor. Intervalos <= 0 caem no DefaultInterval.
func New(check CheckFunc, emitter Emitter, interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Monitor{
		interval: interval,
		check:    check,
		emitter:  emitter,
		now:      time.Now,
	}
}

// Run executa o loop de verificação até o ctx ser cancelado. A primeira
// verificação roda imediatamente para que o indicador da UI pinte rápido.
func (m *Monitor) Run(ctx context.Context) {
	m.checkOnce(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkOnce(ctx)
		}
	}
}

// checkOnce executa uma sondagem e emite o evento resultante. Quando o último
// estado conhecido era offline, emite um evento "checking" antes da sondagem
// para que a UI mostre "reconectando".
func (m *Monitor) checkOnce(ctx context.Context) {
	m.mu.Lock()
	reconnecting := m.hasLast && m.last.State == StateOffline
	prevID := m.last.ProviderID
	prevName := m.last.ProviderName
	m.mu.Unlock()

	if reconnecting {
		m.emit(Event{
			State:        StateChecking,
			ProviderID:   prevID,
			ProviderName: prevName,
			Timestamp:    m.now().UnixMilli(),
		})
	}

	snap := m.check(ctx)
	if ctx.Err() != nil {
		return
	}
	m.emit(m.buildEvent(snap))
}

// buildEvent atualiza a janela de latência, calcula a média e materializa o
// Event, guardando-o como último estado conhecido.
func (m *Monitor) buildEvent(snap Snapshot) Event {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Troca de provider ativo (ex.: mudança de perfil) invalida a janela de
	// latência: zera as amostras para não misturar providers diferentes na média.
	if snap.ProviderID != "" && snap.ProviderID != m.samplesProviderID {
		m.samples = nil
		m.samplesProviderID = snap.ProviderID
	}

	if snap.State == StateOnline && snap.LatencyMs > 0 {
		m.samples = append(m.samples, snap.LatencyMs)
		if len(m.samples) > maxLatencySamples {
			m.samples = m.samples[len(m.samples)-maxLatencySamples:]
		}
	}

	var avg int64
	if len(m.samples) > 0 {
		var sum int64
		for _, v := range m.samples {
			sum += v
		}
		avg = sum / int64(len(m.samples))
	}

	ev := Event{
		State:        snap.State,
		ProviderID:   snap.ProviderID,
		ProviderName: snap.ProviderName,
		Model:        snap.Model,
		LatencyMs:    snap.LatencyMs,
		AvgLatencyMs: avg,
		Error:        snap.Error,
		ErrorType:    snap.ErrorType,
		Timestamp:    m.now().UnixMilli(),
	}
	m.last = ev
	m.hasLast = true
	return ev
}

func (m *Monitor) emit(ev Event) {
	if m.emitter != nil {
		m.emitter.Emit(EventName, ev)
	}
}

// Last retorna o último evento emitido e se já houve alguma verificação.
func (m *Monitor) Last() (Event, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last, m.hasLast
}
