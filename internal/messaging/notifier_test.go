package messaging

import (
	"sync"
	"testing"
	"time"
)

func TestNotifier_RegisterAndNotify(t *testing.T) {
	n := NewResponseNotifier()

	var received string
	var mu sync.Mutex

	n.Register("1", ResponseCallback{
		Channel: "telegram",
		ChatID:  "123",
		Callback: func(response string, msgID string) {
			mu.Lock()
			received = response
			mu.Unlock()
		},
	})

	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}

	n.Notify("1", "Hello from assistant", "42")

	// Aguarda goroutine do callback
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if received != "Hello from assistant" {
		t.Fatalf("expected 'Hello from assistant', got %q", received)
	}
	mu.Unlock()

	// Callback deve ter sido removido
	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after notify, got %d", n.PendingCount())
	}
}

func TestNotifier_NotifyWithoutCallbacks(t *testing.T) {
	n := NewResponseNotifier()

	// Não deve causar panic nem erro
	n.Notify("999", "response without listener", "")

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", n.PendingCount())
	}
}

func TestNotifier_MultipleCallbacksSameConversation(t *testing.T) {
	n := NewResponseNotifier()

	var mu sync.Mutex
	results := make([]string, 0)

	n.Register("1", ResponseCallback{
		Channel: "telegram",
		ChatID:  "123",
		Callback: func(response string, msgID string) {
			mu.Lock()
			results = append(results, "telegram:"+response)
			mu.Unlock()
		},
	})

	n.Register("1", ResponseCallback{
		Channel: "signal",
		ChatID:  "+55119999",
		Callback: func(response string, msgID string) {
			mu.Lock()
			results = append(results, "signal:"+response)
			mu.Unlock()
		},
	})

	if n.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", n.PendingCount())
	}

	n.Notify("1", "multi-channel", "100")

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
	mu.Unlock()

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after notify, got %d", n.PendingCount())
	}
}

func TestNotifier_DifferentConversations(t *testing.T) {
	n := NewResponseNotifier()

	var mu sync.Mutex
	results := make(map[string]string)

	n.Register("1", ResponseCallback{
		Channel: "telegram",
		ChatID:  "111",
		Callback: func(response string, msgID string) {
			mu.Lock()
			results["1"] = response
			mu.Unlock()
		},
	})

	n.Register("2", ResponseCallback{
		Channel: "telegram",
		ChatID:  "222",
		Callback: func(response string, msgID string) {
			mu.Lock()
			results["2"] = response
			mu.Unlock()
		},
	})

	// Notifica apenas conversa 1
	n.Notify("1", "response for conv 1", "200")

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if results["1"] != "response for conv 1" {
		t.Fatalf("expected 'response for conv 1', got %q", results["1"])
	}
	if _, ok := results["2"]; ok {
		t.Fatalf("conv 2 should not have been notified")
	}
	mu.Unlock()

	// Conv 2 ainda deve estar pendente
	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}
}

func TestNotifier_Cancel(t *testing.T) {
	n := NewResponseNotifier()

	called := false
	n.Register("1", ResponseCallback{
		Channel: "sip",
		ChatID:  "caller@pbx",
		TraceID: "trace-cancel-test",
		Callback: func(response string, msgID string) {
			called = true
		},
	})

	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}

	// Cancel remove sem chamar callback
	n.Cancel("1")

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after cancel, got %d", n.PendingCount())
	}

	// Notify após cancel não deve chamar callback
	n.Notify("1", "late response", "42")
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Fatal("callback should not have been called after cancel")
	}
}

func TestNotifier_CancelNonExistent(t *testing.T) {
	n := NewResponseNotifier()

	// Cancel de conversa inexistente não deve causar panic
	n.Cancel("999")

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", n.PendingCount())
	}
}

// TestNotifier_RecoverFromPanicCallback cobre M11 do review da Fatia 2.
// Antes: callback que panicava derrubava o processo todo (via go func).
// Agora: defer/recover isola e logs — outros callbacks devem continuar
// disparando normalmente.
func TestNotifier_RecoverFromPanicCallback(t *testing.T) {
	n := NewResponseNotifier()
	defer n.Stop()

	var mu sync.Mutex
	survived := false

	n.Register("conv-panic", ResponseCallback{
		Channel: "telegram",
		TraceID: "trace-panic",
		Callback: func(response string, msgID string) {
			panic("adapter mal escrito")
		},
	})
	n.Register("conv-panic", ResponseCallback{
		Channel: "signal",
		TraceID: "trace-survive",
		Callback: func(response string, msgID string) {
			mu.Lock()
			survived = true
			mu.Unlock()
		},
	})

	n.Notify("conv-panic", "resposta", "msg-1")
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if !survived {
		t.Fatal("callback irmão não foi chamado — panic em um derrubou o processo")
	}
	mu.Unlock()
}

// TestNotifier_TTLExpiresOldCallbacks cobre B7 (TTL no notifier).
// Callbacks que ficam pendurados além do TTL devem ser descartados pela
// goroutine de housekeeping para evitar leak.
func TestNotifier_TTLExpiresOldCallbacks(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()

	n.Register("conv-old", ResponseCallback{
		Channel: "telegram",
		TraceID: "trace-old",
		Callback: func(string, string) {
			t.Fatal("callback expirado não deveria ser chamado")
		},
	})

	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}

	now = now.Add(callbackTTL + time.Minute)
	n.expireOldCallbacks()

	if n.PendingCount() != 0 {
		t.Fatalf("callback expirado não foi removido, pending=%d", n.PendingCount())
	}
}

// TestNotifier_TTLDoesNotExpireFreshCallbacks garante que o cleanup só
// remove callbacks vencidos — fresh callbacks ficam intactos.
func TestNotifier_TTLDoesNotExpireFreshCallbacks(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()

	n.Register("conv-fresh", ResponseCallback{
		Channel: "telegram",
		TraceID: "trace-fresh",
		Callback: func(string, string) {},
	})

	now = now.Add(time.Minute)
	n.expireOldCallbacks()

	if n.PendingCount() != 1 {
		t.Fatalf("callback fresh foi removido indevidamente, pending=%d", n.PendingCount())
	}
}

// TestNotifier_PerCallbackTTLSurvivesDefault garante que um callback registrado
// com TTL próprio (> 0) sobreviva ao TTL PADRÃO de 5min e só expire após o seu
// próprio TTL — e que, enquanto vivo, ainda seja entregue por Notify. É o caso do
// sub-agente em background, cujo run pode levar bem mais que 5min (até o timeout
// efetivo) sem perder a conclusão. Um callback default (TTL=0) registrado junto
// continua expirando aos 5min (retrocompat dos canais/UI).
func TestNotifier_PerCallbackTTLSurvivesDefault(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()

	const longTTL = 30 * time.Minute
	// Canal sincroniza a entrega: o callback roda em goroutine do Notify; ler uma
	// variável simples no main seria corrida (detectada por -race).
	firedCh := make(chan string, 1)
	n.Register("conv-long", ResponseCallback{
		Channel:  "subagent",
		TraceID:  "trace-long",
		TTL:      longTTL,
		Callback: func(resp, _ string) { firedCh <- resp },
	})
	// Callback default (sem TTL): deve expirar aos 5min como antes.
	n.Register("conv-default", ResponseCallback{
		Channel:  "telegram",
		TraceID:  "trace-default",
		Callback: func(string, string) { t.Fatal("callback default não deveria sobreviver ao TTL padrão") },
	})

	// Avança 6min: passa do TTL padrão (5min), mas não do TTL longo (30min).
	now = now.Add(callbackTTL + time.Minute)
	n.expireOldCallbacks()

	if _, ok := n.callbacks["conv-default"]; ok {
		t.Fatal("callback default deveria ter expirado aos 5min")
	}
	if _, ok := n.callbacks["conv-long"]; !ok {
		t.Fatal("callback com TTL longo NÃO deveria ter expirado aos 6min")
	}

	// Enquanto vivo (bem além dos 5min), a conclusão ainda é entregue.
	n.Notify("conv-long", "conclusão tardia", "msg-1")
	select {
	case got := <-firedCh:
		if got != "conclusão tardia" {
			t.Fatalf("resposta inesperada no callback: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback de TTL longo não foi chamado por Notify")
	}

	// Após o TTL longo, expira normalmente (sem leak).
	n.Register("conv-long-2", ResponseCallback{
		Channel:  "subagent",
		TraceID:  "trace-long-2",
		TTL:      longTTL,
		Callback: func(string, string) { t.Fatal("não deveria disparar após expirar") },
	})
	now = now.Add(longTTL + time.Minute)
	n.expireOldCallbacks()
	if _, ok := n.callbacks["conv-long-2"]; ok {
		t.Fatal("callback com TTL longo deveria expirar após o seu próprio TTL")
	}
}

// TestNotifier_PendingExpiry expõe o instante de expiração calculado a partir do
// TTL efetivo do registro (padrão vs. parametrizado).
func TestNotifier_PendingExpiry(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()

	n.Register("conv-default", ResponseCallback{Channel: "telegram", Callback: func(string, string) {}})
	n.Register("conv-ttl", ResponseCallback{Channel: "subagent", TTL: time.Hour, Callback: func(string, string) {}})

	if exp, ok := n.PendingExpiry("conv-default"); !ok || !exp.Equal(now.Add(callbackTTL)) {
		t.Fatalf("expiry default esperado %v; veio %v (ok=%v)", now.Add(callbackTTL), exp, ok)
	}
	if exp, ok := n.PendingExpiry("conv-ttl"); !ok || !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("expiry parametrizado esperado %v; veio %v (ok=%v)", now.Add(time.Hour), exp, ok)
	}
	if _, ok := n.PendingExpiry("inexistente"); ok {
		t.Fatal("PendingExpiry deveria retornar ok=false para conversa sem callback")
	}
}

// TestNotifier_CancelByChannel cobre B7. Quando um canal é desregistrado
// (Unregister/Shutdown do gateway), todos os callbacks pendentes daquele
// canal devem ser cancelados — não só os de uma conversa específica.
func TestNotifier_CancelByChannel(t *testing.T) {
	n := NewResponseNotifier()
	defer n.Stop()

	called := map[string]bool{}
	var mu sync.Mutex
	mark := func(key string) func(string, string) {
		return func(string, string) {
			mu.Lock()
			called[key] = true
			mu.Unlock()
		}
	}

	n.Register("conv-tel-1", ResponseCallback{Channel: "telegram", TraceID: "t1", Callback: mark("tel-1")})
	n.Register("conv-tel-2", ResponseCallback{Channel: "telegram", TraceID: "t2", Callback: mark("tel-2")})
	n.Register("conv-sig-1", ResponseCallback{Channel: "signal", TraceID: "s1", Callback: mark("sig-1")})

	if n.PendingCount() != 3 {
		t.Fatalf("expected 3 pending, got %d", n.PendingCount())
	}

	cancelled := n.CancelByChannel("telegram")
	if cancelled != 2 {
		t.Fatalf("expected 2 cancelled, got %d", cancelled)
	}
	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending after cancel-by-channel, got %d", n.PendingCount())
	}

	// Notify dos cancelados não dispara callback.
	n.Notify("conv-tel-1", "x", "")
	n.Notify("conv-tel-2", "x", "")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if called["tel-1"] || called["tel-2"] {
		t.Fatalf("callback de canal cancelado foi chamado: %+v", called)
	}
	mu.Unlock()

	// O canal signal continua intacto.
	n.Notify("conv-sig-1", "y", "")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if !called["sig-1"] {
		t.Fatal("callback de canal não cancelado deveria ter disparado")
	}
	mu.Unlock()
}

// TestNotifier_CancelByChannelEmpty é um sanity check: passar canal
// vazio é no-op (não cancela tudo por engano).
func TestNotifier_CancelByChannelEmpty(t *testing.T) {
	n := NewResponseNotifier()
	defer n.Stop()
	n.Register("conv-1", ResponseCallback{Channel: "telegram", Callback: func(string, string) {}})
	if n.CancelByChannel("") != 0 {
		t.Fatal("CancelByChannel(\"\") deveria ser no-op")
	}
	if n.PendingCount() != 1 {
		t.Fatalf("pending alterado por canal vazio: %d", n.PendingCount())
	}
}
