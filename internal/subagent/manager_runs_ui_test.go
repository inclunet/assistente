package subagent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/messaging"
)

// recordingEmitter coleta os eventos emitidos ao frontend pelo Manager.
type recordingEmitter struct {
	mu     sync.Mutex
	events []recordedEvent
}

type recordedEvent struct {
	name    string
	payload RunEvent
}

func (e *recordingEmitter) emit(name string, data any) {
	payload, _ := data.(RunEvent)
	e.mu.Lock()
	e.events = append(e.events, recordedEvent{name: name, payload: payload})
	e.mu.Unlock()
}

func (e *recordingEmitter) byName(name string) []RunEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []RunEvent
	for _, ev := range e.events {
		if ev.name == name {
			out = append(out, ev.payload)
		}
	}
	return out
}

// TestManagerConcurrencyLimitGlobal garante que o teto AGREGADO barra um run
// mesmo quando o usuário ainda tem vaga individual: o limite por usuário sozinho
// deixaria N usuários somarem N × MaxConcurrentPerUser runs no processo.
func TestManagerConcurrencyLimitGlobal(t *testing.T) {
	repo, ctxA := setupManagerTest(t) // user-a
	ctxB := database.WithUserID(context.Background(), "user-b")
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	mgr := NewManager(ManagerConfig{
		Repo:                 repo,
		Notifier:             notifier,
		Delivery:             &recordingDelivery{},
		MaxConcurrentPerUser: 4,
		MaxConcurrentGlobal:  1,
		CancelStream:         func(string) {},
		// Nunca notifica → o run em background segura a vaga.
		Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	first, err := mgr.Run(ctxA, RunParams{Prompt: "ocupa a vaga global", Background: true, ParentConversationID: "p"})
	if err != nil {
		t.Fatalf("primeiro run: %v", err)
	}

	// user-b tem vaga individual sobrando (0 de 4), mas o teto global já estourou.
	_, err = mgr.Run(ctxB, RunParams{Prompt: "deve falhar", Background: true, ParentConversationID: "p"})
	if err == nil {
		t.Fatal("esperava erro de limite global no run de outro usuário")
	}
	if !strings.Contains(err.Error(), "global") {
		t.Fatalf("o erro deveria identificar o teto global, veio: %v", err)
	}

	// Cancelar o run que ocupava a vaga libera o teto global para o outro usuário.
	if _, err := mgr.Cancel(ctxA, first.ConversationID, first.RunID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		_, lastErr = mgr.Run(ctxB, RunParams{Prompt: "agora vai", Background: true, ParentConversationID: "p"})
		if lastErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("vaga global não liberada após cancelar o run: %v", lastErr)
	}
}

// TestReleaseSlotKeepsGlobalCounterConsistent garante que o contador agregado
// acompanha os acquires/releases e que um release a mais (idempotência) não
// zera o teto global ocupado por outros usuários.
func TestReleaseSlotKeepsGlobalCounterConsistent(t *testing.T) {
	mgr := NewManager(ManagerConfig{MaxConcurrentPerUser: 2, MaxConcurrentGlobal: 2})

	if err := mgr.acquireSlot("user-a"); err != nil {
		t.Fatalf("acquire user-a: %v", err)
	}
	if err := mgr.acquireSlot("user-b"); err != nil {
		t.Fatalf("acquire user-b: %v", err)
	}
	if err := mgr.acquireSlot("user-a"); err == nil {
		t.Fatal("esperava erro: teto global de 2 já ocupado por dois usuários distintos")
	}

	// Release a mais para user-b: idempotente, não pode devolver vaga alheia.
	mgr.releaseSlot("user-b")
	mgr.releaseSlot("user-b")

	if _, global := mgr.concurrencySnapshot("user-a"); global != 1 {
		t.Fatalf("esperava 1 run ativo global (só user-a), veio %d", global)
	}
	if err := mgr.acquireSlot("user-b"); err != nil {
		t.Fatalf("a vaga liberada por user-b deveria estar disponível: %v", err)
	}
	if err := mgr.acquireSlot("user-b"); err == nil {
		t.Fatal("esperava erro: teto global de 2 novamente ocupado")
	}
}

// TestManagerListRunsOrdersActiveFirst garante que a listagem da UI mostra os
// runs ativos antes dos terminais (são os únicos canceláveis e não podem sumir
// só por serem antigos) e devolve os contadores de ocupação dos tetos.
func TestManagerListRunsOrdersActiveFirst(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{
		Repo:                 repo,
		Notifier:             notifier,
		Delivery:             &recordingDelivery{},
		MaxConcurrentPerUser: 3,
		MaxConcurrentGlobal:  5,
		CancelStream:         func(string) {},
		Send:                 func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	// Run terminal, criado PRIMEIRO (o mais antigo).
	oldConv, err := database.CreateSubAgentConversationWithContext(ctx, "Run antigo", "p")
	if err != nil {
		t.Fatalf("criar sub-conversa: %v", err)
	}
	completedAt := time.Now()
	old := &database.SubAgentRun{
		ChildConversationID: oldConv.ID,
		Status:              database.SubAgentRunStatusSucceeded,
		CompletedAt:         &completedAt,
	}
	if err := repo.Create(ctx, old); err != nil {
		t.Fatalf("criar run antigo: %v", err)
	}

	// Run em background que fica ativo (o Send nunca notifica).
	active, err := mgr.Run(ctx, RunParams{Prompt: "trabalho em segundo plano", Background: true, ParentConversationID: "p"})
	if err != nil {
		t.Fatalf("run em background: %v", err)
	}

	result, err := mgr.ListRuns(ctx, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(result.Runs) != 2 {
		t.Fatalf("esperava 2 runs listados, veio %d", len(result.Runs))
	}
	if result.Runs[0].RunID != active.RunID {
		t.Fatalf("o run ativo deveria vir primeiro; veio %#v", result.Runs[0])
	}
	if !result.Runs[0].Active || !result.Runs[0].Background {
		t.Fatalf("run ativo deveria estar marcado como ativo e em background: %#v", result.Runs[0])
	}
	if result.Runs[0].Title == "" {
		t.Fatalf("o título da sub-conversa deveria vir preenchido: %#v", result.Runs[0])
	}
	if result.Runs[1].RunID != old.ID || result.Runs[1].Active {
		t.Fatalf("o run terminal deveria vir depois e inativo: %#v", result.Runs[1])
	}
	if result.ActiveForUser != 1 || result.ActiveGlobal != 1 {
		t.Fatalf("contadores de ocupação inesperados: %#v", result)
	}
	if result.MaxConcurrentPerUser != 3 || result.MaxConcurrentGlobal != 5 {
		t.Fatalf("tetos configurados não refletidos na listagem: %#v", result)
	}

	// Cancelar pela mesma porta que a UI usa deixa o run fora dos ativos.
	cancelled, err := mgr.Cancel(ctx, active.ConversationID, active.RunID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !cancelled.Cancelled || cancelled.Status != StatusCancelled {
		t.Fatalf("esperava cancelamento efetivo, veio %#v", cancelled)
	}
	waitForRunStatus(t, repo, ctx, active.RunID, StatusCancelled)

	after, err := mgr.ListRuns(ctx, 0)
	if err != nil {
		t.Fatalf("ListRuns após cancelar: %v", err)
	}
	for _, run := range after.Runs {
		if run.Active {
			t.Fatalf("nenhum run deveria continuar ativo após o cancelamento: %#v", run)
		}
	}
}

// TestManagerListRunsScopedByUser garante que a listagem não vaza runs de outro
// usuário (AEP-0052).
func TestManagerListRunsScopedByUser(t *testing.T) {
	repo, ctxA := setupManagerTest(t)
	ctxB := database.WithUserID(context.Background(), "user-b")
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier})

	runB := &database.SubAgentRun{ChildConversationID: "child-b", Status: database.SubAgentRunStatusRunning}
	if err := repo.Create(ctxB, runB); err != nil {
		t.Fatalf("criar run de user-b: %v", err)
	}

	result, err := mgr.ListRuns(ctxA, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(result.Runs) != 0 {
		t.Fatalf("user-a não deveria enxergar runs de user-b: %#v", result.Runs)
	}
}

// TestManagerEmitsRunEvents garante que todo run emite o par início/fim para o
// frontend, com conversationId e status finais — é o que permite anunciar o
// trabalho em segundo plano (AEP-0058) e manter a lista da UI viva.
func TestManagerEmitsRunEvents(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	emitter := &recordingEmitter{}
	mgr := NewManager(ManagerConfig{
		Repo:      repo,
		Notifier:  notifier,
		EmitEvent: emitter.emit,
		Send: func(_ context.Context, p SendParams) (string, error) {
			go notifier.Notify(p.ConversationID, "pronto", "msg-1")
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "faça X", Title: "Tarefa X", ParentConversationID: "p"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	started := emitter.byName(EventRunStarted)
	if len(started) != 1 {
		t.Fatalf("esperava 1 evento de início, veio %d", len(started))
	}
	if started[0].RunID != res.RunID || started[0].ConversationID != res.ConversationID {
		t.Fatalf("evento de início sem os handles do run: %#v", started[0])
	}
	if started[0].Title != "Tarefa X" {
		t.Fatalf("evento de início deveria carregar o título da sub-conversa: %#v", started[0])
	}

	finished := emitter.byName(EventRunFinished)
	if len(finished) != 1 {
		t.Fatalf("esperava 1 evento de fim, veio %d", len(finished))
	}
	if finished[0].Status != StatusSucceeded {
		t.Fatalf("evento de fim deveria carregar o status terminal, veio %q", finished[0].Status)
	}
	if finished[0].ConversationID != res.ConversationID || finished[0].Title != "Tarefa X" {
		t.Fatalf("evento de fim sem conversationId/título: %#v", finished[0])
	}
}

// TestManagerRejectedRunEmitsNoEvents garante que um run barrado pelo teto de
// concorrência não produz evento algum: sem run, não há início nem fim para a
// UI anunciar.
func TestManagerRejectedRunEmitsNoEvents(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	emitter := &recordingEmitter{}
	mgr := NewManager(ManagerConfig{
		Repo:                repo,
		Notifier:            notifier,
		Delivery:            &recordingDelivery{},
		EmitEvent:           emitter.emit,
		MaxConcurrentGlobal: 1,
		CancelStream:        func(string) {},
		Send:                func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	if _, err := mgr.Run(ctx, RunParams{Prompt: "ocupa", Background: true, ParentConversationID: "p"}); err != nil {
		t.Fatalf("primeiro run: %v", err)
	}
	if _, err := mgr.Run(ctx, RunParams{Prompt: "barrado", Background: true, ParentConversationID: "p"}); err == nil {
		t.Fatal("esperava erro de limite global")
	}

	if got := len(emitter.byName(EventRunStarted)); got != 1 {
		t.Fatalf("apenas o run aceito deveria emitir início, veio %d", got)
	}
	if got := len(emitter.byName(EventRunFinished)); got != 0 {
		t.Fatalf("nenhum run terminou ainda, mas vieram %d eventos de fim", got)
	}
}

func waitForRunStatus(t *testing.T, repo Repository, ctx context.Context, runID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := repo.Get(ctx, runID)
		if err == nil && run.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s não atingiu o status %q a tempo", runID, want)
}
