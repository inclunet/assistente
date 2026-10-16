package jobs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunLimiter_LimitaConcorrencia garante que o semáforo nunca deixa mais que
// `max` execuções simultâneas — o coração do fix do gap dominante (AEP-0106).
func TestRunLimiter_LimitaConcorrencia(t *testing.T) {
	const max = 4
	const workers = 40
	l := newRunLimiter(max)

	var (
		active    int32
		maxActive int32
		wg        sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !l.acquire(context.Background()) {
				t.Errorf("acquire não deveria falhar com contexto vivo")
				return
			}
			defer l.release()
			cur := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got > max {
		t.Fatalf("concorrência máxima = %d; teto era %d", got, max)
	}
	if got := atomic.LoadInt32(&maxActive); got == 0 {
		t.Fatal("nenhuma execução concorrente observada; teste inválido")
	}
}

// TestRunLimiter_AbortaComCtxCancelado garante que, com o pool cheio, um novo
// acquire retorna false quando o contexto é cancelado (não vaza slot, não trava
// o shutdown).
func TestRunLimiter_AbortaComCtxCancelado(t *testing.T) {
	l := newRunLimiter(1)
	if !l.acquire(context.Background()) {
		t.Fatal("primeiro acquire deveria suceder")
	}
	defer l.release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if l.acquire(ctx) {
		t.Fatal("acquire com pool cheio e ctx cancelado deveria retornar false")
	}
}

// TestRunLimiter_AbortaComCtxCanceladoMesmoComSlotLivre garante o guard
// determinístico: mesmo com slots livres, um ctx já cancelado nunca reserva slot
// (sem depender da escolha aleatória do select). Repete para reduzir a chance de
// um falso verde caso a corrida do select fosse reintroduzida.
func TestRunLimiter_AbortaComCtxCanceladoMesmoComSlotLivre(t *testing.T) {
	l := newRunLimiter(8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 100; i++ {
		if l.acquire(ctx) {
			t.Fatal("acquire com slot livre mas ctx cancelado deveria retornar false")
		}
	}
	if len(l.sem) != 0 {
		t.Fatalf("nenhum slot deveria ter sido reservado; ocupados=%d", len(l.sem))
	}
}

// TestRunLimiter_DefaultQuandoNaoConfigurado garante o teto padrão para valores
// <= 0.
func TestRunLimiter_DefaultQuandoNaoConfigurado(t *testing.T) {
	for _, v := range []int{0, -1} {
		l := newRunLimiter(v)
		if cap(l.sem) != defaultMaxConcurrentRuns {
			t.Fatalf("newRunLimiter(%d) cap=%d; want %d", v, cap(l.sem), defaultMaxConcurrentRuns)
		}
	}
	if l := newRunLimiter(8); cap(l.sem) != 8 {
		t.Fatalf("newRunLimiter(8) cap=%d; want 8", cap(l.sem))
	}
}

// TestRunLimiter_ReleaseSemAcquireNaoEntraEmPanico protege contra release extra
// (defensivo): não deve bloquear nem entrar em pânico.
func TestRunLimiter_ReleaseSemAcquireNaoEntraEmPanico(t *testing.T) {
	l := newRunLimiter(2)
	l.release() // sem acquire prévio
}
