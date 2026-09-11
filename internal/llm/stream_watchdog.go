package llm

import (
	"context"
	"sync"
	"time"

	"assistente/internal/logging"
)

// defaultStreamIdleTimeout é quanto tempo o stream pode ficar sem entregar
// nenhum evento antes de o watchdog considerar a conexão morta. Servidores
// OpenAI-compatible (proxies, gateways, providers menores) às vezes param de
// enviar sem fechar a conexão; sem este limite, o loop de leitura fica preso
// até um timeout externo — que era o Timeout global do http.Client (removido
// em favor de timeouts granulares).
const defaultStreamIdleTimeout = 60 * time.Second

// streamIdleTimeoutForProvider devolve o idle timeout do provider, com
// override opcional por configuração (StreamIdleTimeoutSeconds).
func streamIdleTimeoutForProvider(p *ProviderConfig) time.Duration {
	if p != nil && p.StreamIdleTimeoutSeconds > 0 {
		return time.Duration(p.StreamIdleTimeoutSeconds) * time.Second
	}
	return defaultStreamIdleTimeout
}

// streamWatchdog cancela o contexto da tentativa quando o stream fica ocioso
// demais. Cada evento recebido chama Kick; se o timer estourar, o cancelamento
// derruba a leitura bloqueada e o erro vira retryable (quando nada visível foi
// emitido) ou erro explícito para quem assiste.
type streamWatchdog struct {
	cancel    context.CancelFunc
	kick      chan struct{}
	done      chan struct{}
	parent    context.Context
	idle      time.Duration
	onTimeout func()

	mu              sync.Mutex
	timedOut        bool
	timeoutNotified bool
	lastActivity    time.Time
	stopRequested   bool
}

// startStreamWatchdog deriva ctx com cancelamento por ociosidade. onTimeout é
// chamado uma única vez, após o cancelamento do contexto vigiado, para
// log/telemetria.
func startStreamWatchdog(ctx context.Context, idle time.Duration, onTimeout func()) (context.Context, *streamWatchdog) {
	watchCtx, cancel := context.WithCancel(ctx)
	w := &streamWatchdog{
		cancel:       cancel,
		kick:         make(chan struct{}, 1),
		done:         make(chan struct{}),
		parent:       ctx,
		idle:         idle,
		onTimeout:    onTimeout,
		lastActivity: time.Now(),
	}

	go func() {
		defer close(w.done)
		timer := time.NewTimer(idle)
		defer timer.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-timer.C:
				w.mu.Lock()
				if w.stopRequested || w.parent.Err() != nil {
					w.mu.Unlock()
					return
				}
				remaining := w.idle - time.Since(w.lastActivity)
				if remaining > 0 {
					w.mu.Unlock()
					timer.Reset(remaining)
					continue
				}
				w.timedOut = true
				w.mu.Unlock()
				logging.Warnf(ctx, "llm.stream-watchdog", "[stream-watchdog] stream sem eventos há %s; cancelando tentativa", idle)
				cancel()
				w.notifyTimeout()
				return
			case <-w.kick:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idle)
			}
		}
	}()

	return watchCtx, w
}

// Kick sinaliza atividade: reinicia a contagem de ociosidade. Non-blocking.
func (w *streamWatchdog) Kick() {
	w.mu.Lock()
	now := time.Now()
	if w.stopRequested || w.parent.Err() != nil || w.timedOut {
		w.mu.Unlock()
		return
	}
	if now.Sub(w.lastActivity) >= w.idle {
		w.timedOut = true
		w.mu.Unlock()
		w.cancel()
		w.notifyTimeout()
		return
	}
	w.lastActivity = now
	w.mu.Unlock()
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *streamWatchdog) notifyTimeout() {
	w.mu.Lock()
	if w.timeoutNotified || w.onTimeout == nil {
		w.mu.Unlock()
		return
	}
	w.timeoutNotified = true
	callback := w.onTimeout
	w.mu.Unlock()
	callback()
}

// Stop encerra o watchdog quando a tentativa acabou por vias próprias.
func (w *streamWatchdog) Stop() {
	// Se o deadline já venceu mas a goroutine ainda não consumiu timer.C,
	// registre a expiração antes de cancelar watchCtx. Isso remove a escolha
	// não determinística do select entre timer.C e watchCtx.Done no EOF.
	w.mu.Lock()
	notifyTimeout := false
	if !w.timedOut && w.parent.Err() == nil && time.Since(w.lastActivity) >= w.idle {
		w.timedOut = true
		notifyTimeout = true
	}
	w.stopRequested = true
	w.mu.Unlock()
	w.cancel()
	if notifyTimeout {
		// O callback pode ser lento; Stop continua limitado enquanto
		// notifyTimeout garante execução única sob o próprio mutex.
		go w.notifyTimeout()
	}
	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
	}
}

// TimedOut informa se este watchdog cancelou a tentativa.
func (w *streamWatchdog) TimedOut() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timedOut
}

// TurnNoticeStreamRetry avisa que uma tentativa de streaming falhou e será
// repetida (erro transitório, watchdog de ociosidade, backoff). Count é o
// número da tentativa que acabou de falhar.
const TurnNoticeStreamRetry TurnNoticeKind = "stream_retry"

// notifyTurnNotice entrega o aviso ao handler quando ele souber recebê-lo
// (TurnNoticeSink é opcional por contrato). Garante log mesmo sem sink
// para não deixar o usuário no silêncio do backoff.
func notifyTurnNotice(handler StreamHandler, notice TurnNotice) {
	if sink, ok := handler.(TurnNoticeSink); ok {
		sink.OnTurnNotice(notice)
		return
	}
	logging.Warnf(context.Background(), "llm.stream-watchdog", "[stream] aviso %s count=%d sem TurnNoticeSink; backoff segue", notice.Kind, notice.Count)
}

// streamIdleErrorMessage é o erro mostrado quando o watchdog interrompe um
// stream que já havia entregado conteúdo visível (não é seguro retentar sem
// duplicar a resposta).
const streamIdleErrorMessage = "streaming_idle_timeout"
