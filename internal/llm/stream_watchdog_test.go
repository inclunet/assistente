package llm

import (
	"context"
	"net/http"
	"testing"
	"time"

	"assistente/internal/credentials"
)

func TestStartStreamWatchdogCancelaAposOciosidade(t *testing.T) {
	ctx := context.Background()
	fired := make(chan struct{})

	watchCtx, wd := startStreamWatchdog(ctx, 30*time.Millisecond, func() { close(fired) })
	defer wd.Stop()

	select {
	case <-watchCtx.Done():
		t.Fatal("watchdog cancelou antes da ociosidade")
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog não estourou dentro do esperado")
	}
	if watchCtx.Err() == nil {
		t.Fatal("esperava contexto cancelado após ociosidade")
	}
	if !wd.TimedOut() {
		t.Fatal("esperava TimedOut() == true")
	}
}

func TestStartStreamWatchdogKickReiniciaContagem(t *testing.T) {
	ctx := context.Background()

	watchCtx, wd := startStreamWatchdog(ctx, 50*time.Millisecond, nil)
	defer wd.Stop()

	// Kick a cada 20ms mantém o watchdog vivo por 150ms (> 3 períodos de idle).
	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case <-deadline:
			if wd.TimedOut() {
				t.Fatal("watchdog estourou apesar dos kicks")
			}
			return
		case <-watchCtx.Done():
			t.Fatal("watchdog cancelou apesar dos kicks")
		case <-time.After(20 * time.Millisecond):
			wd.Kick()
		}
	}
}

func TestStartStreamWatchdogNaoEstouraQuandoPaiCancelar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	watchCtx, wd := startStreamWatchdog(ctx, time.Hour, nil)
	wd.mu.Lock()
	wd.lastActivity = time.Now().Add(-2 * time.Hour)
	wd.mu.Unlock()
	cancel()
	wd.Stop()

	if !wd.TimedOut() && watchCtx.Err() == nil {
		t.Fatal("cancelamento do pai deve propagar ao contexto vigiado via Stop")
	}
	if wd.TimedOut() {
		t.Fatal("Stop não é estouro: TimedOut deve permanecer false")
	}
}

func TestStreamWatchdogKickNaoRessuscitaDeadlineExpirado(t *testing.T) {
	callbacks := 0
	watchCtx, wd := startStreamWatchdog(context.Background(), time.Hour, func() {
		callbacks++
	})
	wd.mu.Lock()
	expiredActivity := time.Now().Add(-2 * time.Hour)
	wd.lastActivity = expiredActivity
	wd.mu.Unlock()

	wd.Kick()
	wd.Kick()
	wd.Stop()

	if !wd.TimedOut() {
		t.Fatal("kick posterior ao deadline não pode ressuscitar a tentativa")
	}
	if watchCtx.Err() == nil {
		t.Fatal("kick posterior ao deadline deve cancelar a tentativa imediatamente")
	}
	if callbacks != 1 {
		t.Fatalf("onTimeout chamado %d vezes, esperado 1", callbacks)
	}
}

func TestStreamWatchdogStopAntesDoDeadlineNaoViraTimeoutDepois(t *testing.T) {
	_, wd := startStreamWatchdog(context.Background(), 30*time.Millisecond, nil)
	time.Sleep(10 * time.Millisecond)
	wd.Stop()
	time.Sleep(30 * time.Millisecond)

	if wd.TimedOut() {
		t.Fatal("EOF anterior ao deadline não pode virar timeout após Stop")
	}
}

func TestStreamWatchdogStopReconheceDeadlineJaExpirado(t *testing.T) {
	notified := make(chan struct{}, 1)
	_, wd := startStreamWatchdog(context.Background(), time.Hour, func() {
		notified <- struct{}{}
	})
	wd.mu.Lock()
	wd.lastActivity = time.Now().Add(-2 * time.Hour)
	wd.mu.Unlock()

	wd.Stop()

	if !wd.TimedOut() {
		t.Fatal("Stop deve preservar timeout cujo deadline venceu antes do EOF")
	}
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("Stop reconheceu timeout, mas não chamou onTimeout")
	}
}

func TestStreamWatchdogStopLimitaEsperaDeCallbackLento(t *testing.T) {
	release := make(chan struct{})
	watchCtx, wd := startStreamWatchdog(context.Background(), 10*time.Millisecond, func() {
		<-release
	})

	select {
	case <-watchCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("watchdog não cancelou o contexto")
	}

	start := time.Now()
	wd.Stop()
	elapsed := time.Since(start)
	close(release)

	if elapsed < 1500*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("Stop esperou %v; deveria respeitar o teto de 2s", elapsed)
	}
}

func TestStreamIdleTimeoutForProvider(t *testing.T) {
	if got := streamIdleTimeoutForProvider(nil); got != defaultStreamIdleTimeout {
		t.Fatalf("nil provider: esperava %v, veio %v", defaultStreamIdleTimeout, got)
	}
	if got := streamIdleTimeoutForProvider(&ProviderConfig{}); got != defaultStreamIdleTimeout {
		t.Fatalf("sem override: esperava %v, veio %v", defaultStreamIdleTimeout, got)
	}
	if got := streamIdleTimeoutForProvider(&ProviderConfig{StreamIdleTimeoutSeconds: 15}); got != 15*time.Second {
		t.Fatalf("override: esperava 15s, veio %v", got)
	}
}

func TestNewStreamingHTTPClientSemTimeoutGlobal(t *testing.T) {
	client := credentials.NewStreamingHTTPClientWithAuthMode(nil, "", credentials.AuthNone)
	if client.Timeout != 0 {
		t.Fatalf("streaming client não pode ter Timeout global; veio %v", client.Timeout)
	}
	ct, ok := client.Transport.(*credentials.CredentialTransport)
	if !ok || ct.Base == nil {
		t.Fatal("esperava CredentialTransport com Base configurado")
	}
	tr, ok := ct.Base.(*http.Transport)
	if !ok {
		t.Fatal("Base deveria ser *http.Transport")
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Fatal("esperava ResponseHeaderTimeout granular no transport de streaming")
	}
}
