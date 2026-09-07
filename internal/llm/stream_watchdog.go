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
	cancel context.CancelFunc
	kick   chan struct{}
	done   chan struct{}

	mu       sync.Mutex
	timedOut bool
}

// startStreamWatchdog deriva ctx com cancelamento por ociosidade. onTimeout é
// chamado uma única vez, após o cancelamento do contexto vigiado, para
// log/telemetria.
func startStreamWatchdog(ctx context.Context, idle time.Duration, onTimeout func()) (context.Context, *streamWatchdog) {
	watchCtx, cancel := context.WithCancel(ctx)
	w := &streamWatchdog{
		cancel: cancel,
		kick:   make(chan struct{}, 1),
		done:   make(chan struct{}),
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
				w.timedOut = true
				w.mu.Unlock()
				logging.Warnf(ctx, "llm.stream-watchdog", "[stream-watchdog] stream sem eventos há %s; cancelando tentativa", idle)
				cancel()
				if onTimeout != nil {
					onTimeout()
				}
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
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Stop encerra o watchdog quando a tentativa acabou por vias próprias.
func (w *streamWatchdog) Stop() {
	w.cancel()
	<-w.done
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
// (TurnNoticeSink é opcional por contrato).
func notifyTurnNotice(handler StreamHandler, notice TurnNotice) {
	if sink, ok := handler.(TurnNoticeSink); ok {
		sink.OnTurnNotice(notice)
	}
}

// streamIdleErrorMessage é o erro mostrado quando o watchdog interrompe um
// stream que já havia entregado conteúdo visível (não é seguro retentar sem
// duplicar a resposta).
const streamIdleErrorMessage = "streaming interrompido: o provedor parou de responder no meio da geração (timeout de inatividade)"
