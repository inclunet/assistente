package subagent

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/eventctx"
	"assistente/internal/messaging"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupManagerTest(t *testing.T) (*DBRepository, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// :memory: SQLite vive enquanto a conexão existir; sob concorrência
	// (runs em background) o pool pode abrir conexões sem o schema. Uma única
	// conexão garante que o schema persista durante o teste.
	if sqlDB, sErr := db.DB(); sErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Conversation{}, &database.ChatMessage{}, &database.SubAgentRun{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	return NewDBRepository(db), database.WithUserID(context.Background(), "user-a")
}

func TestManagerRunSyncSuccess(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Send: func(_ context.Context, p SendParams) (string, error) {
			if p.Source != Source {
				t.Errorf("esperava Source=%q, veio %q", Source, p.Source)
			}
			// Simula o agentic loop concluindo (SaveAndFinish → Notify).
			go notifier.Notify(p.ConversationID, "resposta do sub-agente", "msg-1")
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(ctx, RunParams{
		ParentConversationID: "parent-conv",
		ParentTurnID:         "parent-turn",
		Prompt:               "faça X",
		ProfileSlug:          "coder",
	})
	if err != nil {
		t.Fatalf("Run erro inesperado: %v", err)
	}
	if res.Status != StatusSucceeded {
		t.Fatalf("status esperado succeeded, veio %q (err=%q)", res.Status, res.Error)
	}
	if res.ResultSummary != "resposta do sub-agente" || res.AssistantMessageID != "msg-1" {
		t.Fatalf("resultado inesperado: %#v", res)
	}
	if res.ConversationID == "" || res.RunID == "" {
		t.Fatalf("handles ausentes: %#v", res)
	}

	// Sub-conversa deve ter kind=subagent e parent vinculado.
	conv, err := database.GetConversationInfoWithContext(ctx, res.ConversationID)
	if err != nil {
		t.Fatalf("buscar sub-conversa: %v", err)
	}
	if conv.Kind != database.ConversationKindSubagent {
		t.Fatalf("kind esperado subagent, veio %q", conv.Kind)
	}
	if conv.ParentConversationID != "parent-conv" {
		t.Fatalf("parent esperado parent-conv, veio %q", conv.ParentConversationID)
	}

	// Run persistido como succeeded.
	run, err := repo.Get(ctx, res.RunID)
	if err != nil {
		t.Fatalf("buscar run: %v", err)
	}
	if run.Status != StatusSucceeded || run.AssistantMessageID != "msg-1" {
		t.Fatalf("run persistido inesperado: %#v", run)
	}
	if run.CompletedAt == nil || run.StartedAt == nil {
		t.Fatalf("timestamps do run não preenchidos: %#v", run)
	}
}

// TestWaitClassifiesCtxDone garante que o caminho ctx.Done() do wait distingue
// timed_out (deadline) de cancelled (cancelamento explícito), em vez de marcar
// sempre cancelled. cancelCh aberto, done vazio e timeout longo: só o ctx.Done()
// está pronto.
func TestWaitClassifiesCtxDone(t *testing.T) {
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	m := &Manager{notifier: notifier, now: time.Now}

	t.Run("cancel", func(t *testing.T) {
		ar := &activeRun{childConversationID: "c", cancelCh: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		o := m.wait(ctx, "c", make(chan completion, 1), ar, time.Hour, false)
		if o.status != StatusCancelled {
			t.Fatalf("esperava cancelled, veio %q", o.status)
		}
		if o.errMsg == "" {
			t.Fatalf("esperava mensagem de erro preenchida")
		}
	})

	t.Run("deadline", func(t *testing.T) {
		ar := &activeRun{childConversationID: "c", cancelCh: make(chan struct{})}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancel()
		o := m.wait(ctx, "c", make(chan completion, 1), ar, time.Hour, false)
		if o.status != StatusTimedOut {
			t.Fatalf("esperava timed_out, veio %q", o.status)
		}
		if o.errMsg == "" {
			t.Fatalf("esperava mensagem de erro preenchida")
		}
	})
}

func TestManagerRunRequiresPrompt(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	if _, err := mgr.Run(ctx, RunParams{Prompt: "   "}); err == nil {
		t.Fatal("esperava erro para prompt vazio")
	}
}

func TestManagerRunSendError(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Send: func(_ context.Context, _ SendParams) (string, error) {
			// Erro comum (não derivado do ctx) → failed.
			return "", fmt.Errorf("falha no envio")
		},
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "faça X"})
	if err != nil {
		t.Fatalf("Run não deve retornar erro de pré-condição: %v", err)
	}
	if res.Status != StatusFailed {
		t.Fatalf("status esperado failed, veio %q", res.Status)
	}
	run, err := repo.Get(ctx, res.RunID)
	if err != nil {
		t.Fatalf("buscar run: %v", err)
	}
	if run.Status != StatusFailed || run.Error == "" {
		t.Fatalf("run failed esperado com erro: %#v", run)
	}
}

// TestManagerRunSendClassifiesCtxError garante que um cancel/timeout que se
// manifeste como erro de m.send seja classificado como cancelled/timed_out
// (não failed), conforme o enum do AEP-0068. A mensagem real do erro deve ser
// preservada no run.
func TestManagerRunSendClassifiesCtxError(t *testing.T) {
	cases := []struct {
		name    string
		sendErr error
		want    string
	}{
		{"canceled", context.Canceled, StatusCancelled},
		{"deadline", context.DeadlineExceeded, StatusTimedOut},
		{"wrapped-canceled", fmt.Errorf("envio abortado: %w", context.Canceled), StatusCancelled},
		{"wrapped-deadline", fmt.Errorf("envio expirou: %w", context.DeadlineExceeded), StatusTimedOut},
	}
	// send é compartilhado antes do branch p.Background: a classificação deve
	// valer tanto para o caminho síncrono quanto para o background.
	for _, bg := range []bool{false, true} {
		for _, tc := range cases {
			name := tc.name
			if bg {
				name += "/background"
			}
			t.Run(name, func(t *testing.T) {
				repo, ctx := setupManagerTest(t)
				notifier := messaging.NewResponseNotifier()
				t.Cleanup(notifier.Stop)
				mgr := NewManager(ManagerConfig{
					Repo:     repo,
					Notifier: notifier,
					Send: func(_ context.Context, _ SendParams) (string, error) {
						return "", tc.sendErr
					},
				})

				res, err := mgr.Run(ctx, RunParams{Prompt: "faça X", Background: bg})
				if err != nil {
					t.Fatalf("Run não deve retornar erro de pré-condição: %v", err)
				}
				if res.Status != tc.want {
					t.Fatalf("status esperado %q, veio %q", tc.want, res.Status)
				}
				run, err := repo.Get(ctx, res.RunID)
				if err != nil {
					t.Fatalf("buscar run: %v", err)
				}
				if run.Status != tc.want {
					t.Fatalf("run persistido esperado %q, veio %#v", tc.want, run)
				}
				if run.Error == "" {
					t.Fatalf("mensagem de erro deveria ser preservada: %#v", run)
				}
			})
		}
	}
}

func TestManagerRunTimeout(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		// Send nunca notifica → deve estourar timeout.
		Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "faça X", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run erro inesperado: %v", err)
	}
	if res.Status != StatusTimedOut {
		t.Fatalf("status esperado timed_out, veio %q", res.Status)
	}
}

type recordingDelivery struct {
	mu      sync.Mutex
	notices []ParentNotice
	ch      chan ParentNotice
	err     error
}

func (d *recordingDelivery) Deliver(_ context.Context, n ParentNotice) error {
	d.mu.Lock()
	d.notices = append(d.notices, n)
	d.mu.Unlock()
	if d.ch != nil {
		select {
		case d.ch <- n:
		default:
		}
	}
	return d.err
}

func (d *recordingDelivery) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.notices)
}

func TestManagerRunBackgroundDeliversNotice(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	delivery := &recordingDelivery{ch: make(chan ParentNotice, 1)}

	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Delivery: delivery,
		Send: func(_ context.Context, p SendParams) (string, error) {
			go notifier.Notify(p.ConversationID, "resultado bg", "msg-bg")
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(ctx, RunParams{
		ParentConversationID: "parent-conv",
		Prompt:               "trabalho longo",
		Background:           true,
	})
	if err != nil {
		t.Fatalf("Run bg erro: %v", err)
	}
	if res.Status != StatusRunning {
		t.Fatalf("status imediato esperado running, veio %q", res.Status)
	}

	select {
	case n := <-delivery.ch:
		if n.Status != StatusSucceeded || n.Summary != "resultado bg" {
			t.Fatalf("notice inesperada: %#v", n)
		}
		if n.ParentConversationID != "parent-conv" {
			t.Fatalf("parent na notice inesperado: %q", n.ParentConversationID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("aviso de conclusão não entregue ao pai")
	}

	// Run persistido como succeeded e marcado como entregue (idempotência).
	deadline := time.Now().Add(2 * time.Second)
	for {
		run, err := repo.Get(ctx, res.RunID)
		if err != nil {
			t.Fatalf("buscar run: %v", err)
		}
		if run.Status == StatusSucceeded && run.DeliveredAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run não chegou a succeeded/delivered: %#v", run)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManagerDeliverIsIdempotent(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	delivery := &recordingDelivery{}
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Delivery: delivery, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	// Cria um run terminal (succeeded) manualmente.
	conv, err := database.CreateSubAgentConversationWithContext(ctx, "t", "parent-conv")
	if err != nil {
		t.Fatalf("criar conv: %v", err)
	}
	run := &database.SubAgentRun{UserID: "user-a", ParentConversationID: "parent-conv", ChildConversationID: conv.ID, Status: StatusSucceeded, ResultSummary: "x"}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("criar run: %v", err)
	}

	mgr.deliver(ctx, run, RunParams{ParentConversationID: "parent-conv"})
	mgr.deliver(ctx, run, RunParams{ParentConversationID: "parent-conv"})

	if delivery.count() != 1 {
		t.Fatalf("esperava 1 entrega (idempotente), veio %d", delivery.count())
	}
}

func TestManagerStatus(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) {
		go notifier.Notify(p.ConversationID, "ok", "m1")
		return p.ConversationID, nil
	}})

	res, err := mgr.Run(ctx, RunParams{Prompt: "x"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st, err := mgr.Status(ctx, res.ConversationID, "")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Status != StatusSucceeded || st.RunID != res.RunID {
		t.Fatalf("status inesperado: %#v", st)
	}
}

func TestManagerCancelNoOpWhenTerminal(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) {
		go notifier.Notify(p.ConversationID, "ok", "m1")
		return p.ConversationID, nil
	}})

	res, err := mgr.Run(ctx, RunParams{Prompt: "x"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cr, err := mgr.Cancel(ctx, res.ConversationID, "")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cr.Cancelled {
		t.Fatalf("esperava no-op (cancelled=false) para run terminal; veio %#v", cr)
	}
	if cr.Status != StatusSucceeded {
		t.Fatalf("status real esperado succeeded; veio %q", cr.Status)
	}
}

func TestManagerCancelActiveBackgroundRun(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	var cancelledConv string
	mgr := NewManager(ManagerConfig{
		Repo:         repo,
		Notifier:     notifier,
		Delivery:     &recordingDelivery{},
		CancelStream: func(conversationID string) { cancelledConv = conversationID },
		// Nunca notifica → run fica ativo até cancelar.
		Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "loop", Background: true, ParentConversationID: "parent-conv"})
	if err != nil {
		t.Fatalf("Run bg: %v", err)
	}
	cr, err := mgr.Cancel(ctx, res.ConversationID, res.RunID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !cr.Cancelled {
		t.Fatalf("esperava cancelled=true para run ativo; veio %#v", cr)
	}
	if cancelledConv != res.ConversationID {
		t.Fatalf("CancelStream não chamado com a sub-conversa; veio %q", cancelledConv)
	}

	// Eventualmente o run persiste como cancelado.
	deadline := time.Now().Add(2 * time.Second)
	for {
		run, _ := repo.Get(ctx, res.RunID)
		if run != nil && run.Status == StatusCancelled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run não chegou a cancelled")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestManagerCancelAfterCompletionIsNoOp garante que, após a conclusão de um run
// em background, um Cancel posterior seja no-op (cancelled:false + status
// terminal real) e NÃO cancele o streaming já concluído (corrida — AEP-0068:
// cancel após término é no-op). Fluxo real: o callback do notifier marca
// terminalStatus e publica em `done`; a remoção de `active` ocorre depois, em
// unregisterActive, após o finish persistir o terminal.
func TestManagerCancelAfterCompletionIsNoOp(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	var mu sync.Mutex
	cancelStreamCalls := 0
	mgr := NewManager(ManagerConfig{
		Repo:         repo,
		Notifier:     notifier,
		Delivery:     &recordingDelivery{},
		CancelStream: func(string) { mu.Lock(); cancelStreamCalls++; mu.Unlock() },
		// Não notifica automaticamente: controlamos a conclusão no teste.
		Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "bg", Background: true, ParentConversationID: "parent-conv"})
	if err != nil {
		t.Fatalf("Run bg: %v", err)
	}

	// Entrega a conclusão: o callback marca terminalStatus e publica em `done`.
	notifier.Notify(res.ConversationID, "resultado", "msg-1")

	// Aguarda o run sair de `active` — o que ocorre em unregisterActive, após o
	// finish persistir (o callback do notifier roda em goroutine).
	deadline := time.Now().Add(2 * time.Second)
	for mgr.lookupActive(res.RunID) != nil {
		if time.Now().After(deadline) {
			t.Fatal("run não saiu de active após a conclusão")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// E persistir como succeeded, para asserir o status terminal real no no-op.
	deadline = time.Now().Add(2 * time.Second)
	for {
		run, _ := repo.Get(ctx, res.RunID)
		if run != nil && run.Status == StatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run não chegou a succeeded")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cr, err := mgr.Cancel(ctx, res.ConversationID, res.RunID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cr.Cancelled {
		t.Fatalf("cancel após conclusão deveria ser no-op (cancelled=false); veio %#v", cr)
	}
	if cr.Status != StatusSucceeded {
		t.Fatalf("status terminal real esperado succeeded; veio %q", cr.Status)
	}
	mu.Lock()
	calls := cancelStreamCalls
	mu.Unlock()
	if calls != 0 {
		t.Fatalf("CancelStream não deveria ser chamado após a conclusão; chamadas=%d", calls)
	}
}

// blockingTerminalUpdateRepo bloqueia a persistência do status TERMINAL (a do
// finish, identificada por CompletedAt != nil) até ser liberado, dando
// determinismo à janela "conclusão entregue mas finish ainda não persistiu".
type blockingTerminalUpdateRepo struct {
	Repository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingTerminalUpdateRepo) Update(ctx context.Context, run *database.SubAgentRun) error {
	if run != nil && run.CompletedAt != nil {
		r.once.Do(func() { close(r.entered) })
		<-r.release
	}
	return r.Repository.Update(ctx, run)
}

// TestManagerCancelDuringCompletionWindowReturnsTerminal cobre a janela entre a
// entrega da conclusão (o callback marca terminalStatus e o run AINDA está em
// `active`) e a persistência do terminal pelo finish: Cancel deve retornar no-op
// (cancelled:false) com o status TERMINAL real (succeeded), NUNCA running.
func TestManagerCancelDuringCompletionWindowReturnsTerminal(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	blocking := &blockingTerminalUpdateRepo{
		Repository: repo,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	var mu sync.Mutex
	cancelStreamCalls := 0
	mgr := NewManager(ManagerConfig{
		Repo:         blocking,
		Notifier:     notifier,
		Delivery:     &recordingDelivery{},
		CancelStream: func(string) { mu.Lock(); cancelStreamCalls++; mu.Unlock() },
		Send:         func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "bg", Background: true, ParentConversationID: "parent-conv"})
	if err != nil {
		t.Fatalf("Run bg: %v", err)
	}

	// Entrega a conclusão: o callback marca terminalStatus=succeeded e publica done.
	notifier.Notify(res.ConversationID, "resultado", "msg-1")

	// Aguarda o finish ENTRAR no persist do terminal (bloqueado): estamos na janela.
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("finish não entrou no persist do terminal")
	}
	// Garante liberação do finish mesmo se a asserção falhar (LIFO: roda antes do Stop).
	t.Cleanup(func() { close(blocking.release) })

	// Na janela: run ainda em `active`, DB ainda NÃO terminal. Cancel deve ser
	// no-op com o status terminal real (succeeded), nunca running.
	cr, err := mgr.Cancel(ctx, res.ConversationID, res.RunID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cr.Cancelled {
		t.Fatalf("cancel na janela de conclusão deveria ser no-op (cancelled=false); veio %#v", cr)
	}
	if cr.Status != StatusSucceeded {
		t.Fatalf("status terminal real esperado succeeded (NUNCA running); veio %q", cr.Status)
	}
	mu.Lock()
	calls := cancelStreamCalls
	mu.Unlock()
	if calls != 0 {
		t.Fatalf("CancelStream não deveria ser chamado na janela de conclusão; chamadas=%d", calls)
	}
}

// TestManagerCancelDuringSendErrorWindowReturnsTerminal cobre a MESMA janela do
// teste de conclusão, mas no CAMINHO DE ERRO de send: o run marca o terminal
// (failed) em memória (markCompleting) e o finish bloqueia ao persistir. Um
// Cancel concorrente na janela deve ser no-op (cancelled:false) com o status
// terminal real (failed), NUNCA running. Garante que o caminho de erro segue a
// ordem markCompleting → finish → unregisterActive.
func TestManagerCancelDuringSendErrorWindowReturnsTerminal(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	blocking := &blockingTerminalUpdateRepo{
		Repository: repo,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	var mu sync.Mutex
	cancelStreamCalls := 0
	convCh := make(chan string, 1)
	mgr := NewManager(ManagerConfig{
		Repo:         blocking,
		Notifier:     notifier,
		Delivery:     &recordingDelivery{},
		CancelStream: func(string) { mu.Lock(); cancelStreamCalls++; mu.Unlock() },
		// Send falha com erro comum (não derivado do ctx) → desfecho failed.
		Send: func(_ context.Context, p SendParams) (string, error) {
			convCh <- p.ConversationID
			return "", fmt.Errorf("falha de envio")
		},
	})

	// O caminho de erro de send é síncrono (antes do branch background): Run
	// bloqueia no finish (persist do terminal). Rodamos em goroutine.
	runDone := make(chan RunResult, 1)
	go func() {
		res, _ := mgr.Run(ctx, RunParams{Prompt: "x", ParentConversationID: "parent-conv"})
		runDone <- res
	}()

	childConvID := <-convCh

	// Aguarda o finish ENTRAR no persist do terminal (bloqueado): estamos na janela.
	select {
	case <-blocking.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("finish não entrou no persist do terminal")
	}
	// Garante liberação do finish mesmo se a asserção falhar (LIFO: roda antes do Stop).
	t.Cleanup(func() { close(blocking.release) })

	// Na janela: run ainda em `active`, DB ainda NÃO terminal. Cancel (resolvido
	// pela sub-conversa) deve ser no-op com o status terminal real (failed).
	cr, err := mgr.Cancel(ctx, childConvID, "")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cr.Cancelled {
		t.Fatalf("cancel na janela de erro deveria ser no-op (cancelled=false); veio %#v", cr)
	}
	if cr.Status != StatusFailed {
		t.Fatalf("status terminal real esperado failed (NUNCA running); veio %q", cr.Status)
	}
	mu.Lock()
	calls := cancelStreamCalls
	mu.Unlock()
	if calls != 0 {
		t.Fatalf("CancelStream não deveria ser chamado na janela de erro; chamadas=%d", calls)
	}
}

// TestManagerRunCancelsSendOnTimeout garante que, ao concluir por timeout, o
// ctx passado ao pipeline de envio (que dispara o agentic loop da sub-conversa
// em background) é efetivamente cancelado — interrompendo trabalho/custo após o
// desfecho — SEM depender do cancelamento do ctx-pai.
func TestManagerRunCancelsSendOnTimeout(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	var sendCtx context.Context
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		// Captura o ctx do envio e retorna sem notificar → estoura timeout.
		// (O loop real rodaria em background sob um filho deste ctx.)
		Send: func(c context.Context, p SendParams) (string, error) {
			sendCtx = c
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "faça X", Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run erro inesperado: %v", err)
	}
	if res.Status != StatusTimedOut {
		t.Fatalf("status esperado timed_out, veio %q", res.Status)
	}
	if sendCtx == nil {
		t.Fatal("send não foi chamado")
	}
	if sendCtx.Err() == nil {
		t.Fatal("sendCtx deveria ter sido cancelado após o timeout (loop em background não interrompido)")
	}
	if ctx.Err() != nil {
		t.Fatalf("ctx-pai não deveria ser cancelado pelo desfecho do run: %v", ctx.Err())
	}
}

// TestManagerRunCancelsSendOnCtxCancel garante que um cancelamento do ctx (ex.:
// usuário cancela a tool) conclui cancelled E cancela o ctx do envio (escopado
// ao run), interrompendo o loop da sub-conversa.
func TestManagerRunCancelsSendOnCtxCancel(t *testing.T) {
	repo, base := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	cctx, cancel := context.WithCancel(base)
	defer cancel()
	var sendCtx context.Context
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Send: func(c context.Context, p SendParams) (string, error) {
			sendCtx = c
			cancel() // simula cancelamento depois que o envio iniciou o loop
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(cctx, RunParams{Prompt: "faça X", Timeout: time.Hour})
	if err != nil {
		t.Fatalf("Run erro inesperado: %v", err)
	}
	if res.Status != StatusCancelled {
		t.Fatalf("status esperado cancelled, veio %q", res.Status)
	}
	if sendCtx == nil || sendCtx.Err() == nil {
		t.Fatal("sendCtx deveria ter sido cancelado")
	}
}

// TestManagerRunBackgroundCancelsSendOnTimeout garante que, no modo background,
// o ctx do envio (desacoplado do pai via WithoutCancel para o run sobreviver ao
// turno-pai) é cancelado quando a espera em background estoura o timeout —
// interrompendo o loop daquele run sem cancelar o ctx-pai.
func TestManagerRunBackgroundCancelsSendOnTimeout(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	sendCtxCh := make(chan context.Context, 1)
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Delivery: &recordingDelivery{},
		Send: func(c context.Context, p SendParams) (string, error) {
			sendCtxCh <- c
			return p.ConversationID, nil // nunca notifica → timeout no background
		},
	})

	res, err := mgr.Run(ctx, RunParams{
		ParentConversationID: "parent-conv",
		Prompt:               "trabalho longo",
		Background:           true,
		Timeout:              20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run bg erro: %v", err)
	}
	if res.Status != StatusRunning {
		t.Fatalf("status imediato esperado running, veio %q", res.Status)
	}

	var sendCtx context.Context
	select {
	case sendCtx = <-sendCtxCh:
	case <-time.After(2 * time.Second):
		t.Fatal("send não foi chamado")
	}

	// Após o timeout do background, o sendCtx (detached do pai) deve ser cancelado.
	deadline := time.Now().Add(2 * time.Second)
	for sendCtx.Err() == nil {
		if time.Now().After(deadline) {
			t.Fatal("sendCtx do run em background deveria ter sido cancelado após o timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatalf("ctx-pai não deveria ser cancelado pelo desfecho do run: %v", ctx.Err())
	}
}

// TestResolveTimeoutByMode prova a separação de semântica de timeout por modo
// (determinístico, sem depender de 5min/1h reais): explícito é respeitado em
// ambos os modos; sem timeout, síncrono usa DefaultSyncTimeout e background usa
// DefaultBackgroundTimeout (nunca o default síncrono).
func TestResolveTimeoutByMode(t *testing.T) {
	if got := resolveTimeout(0, false); got != DefaultSyncTimeout {
		t.Fatalf("síncrono sem timeout: esperava DefaultSyncTimeout (%v), veio %v", DefaultSyncTimeout, got)
	}
	if got := resolveTimeout(0, true); got != DefaultBackgroundTimeout {
		t.Fatalf("background sem timeout: esperava DefaultBackgroundTimeout (%v), veio %v", DefaultBackgroundTimeout, got)
	}
	if got := resolveTimeout(0, true); got == DefaultSyncTimeout {
		t.Fatal("background sem timeout NÃO deve usar DefaultSyncTimeout")
	}
	explicit := 50 * time.Millisecond
	if got := resolveTimeout(explicit, false); got != explicit {
		t.Fatalf("síncrono com timeout explícito: esperava %v, veio %v", explicit, got)
	}
	if got := resolveTimeout(explicit, true); got != explicit {
		t.Fatalf("background com timeout explícito: esperava %v, veio %v", explicit, got)
	}
}

// TestManagerRunBackgroundNoTimeoutDoesNotUseSyncDefault garante, em nível de
// comportamento, que um run background SEM Timeout não expira por causa do
// default síncrono: com Send que nunca notifica, o run permanece ativo (não vira
// timed_out) e só termina por cancelamento explícito. Cancela ao fim para não
// vazar a goroutine de backstop longo.
func TestManagerRunBackgroundNoTimeoutDoesNotUseSyncDefault(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Delivery: &recordingDelivery{},
		// Nunca notifica: sem timeout síncrono, o run fica ativo (backstop longo).
		Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	res, err := mgr.Run(ctx, RunParams{
		ParentConversationID: "parent-conv",
		Prompt:               "trabalho de fundo",
		Background:           true, // sem Timeout: usa DefaultBackgroundTimeout, não o síncrono
	})
	if err != nil {
		t.Fatalf("Run bg erro: %v", err)
	}
	if res.Status != StatusRunning {
		t.Fatalf("status imediato esperado running, veio %q", res.Status)
	}

	// Janela de observação: o run NÃO pode virar timed_out (não usou default síncrono).
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		got, err := repo.Get(ctx, res.RunID)
		if err != nil {
			t.Fatalf("Get run: %v", err)
		}
		if got.Status == StatusTimedOut {
			t.Fatal("run background sem Timeout não deveria expirar (default síncrono indevido)")
		}
		if isTerminal(got.Status) {
			t.Fatalf("run não deveria estar terminal ainda, veio %q", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Limpeza: cancela o run para encerrar a goroutine do backstop (1h).
	if _, err := mgr.Cancel(ctx, res.ConversationID, res.RunID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

// TestManagerRunBackgroundExplicitTimeoutRespected garante que um Timeout
// explícito é respeitado no modo background (vira timed_out), sem depender do
// default de background longo.
func TestManagerRunBackgroundExplicitTimeoutRespected(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	delivery := &recordingDelivery{ch: make(chan ParentNotice, 1)}
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Delivery: delivery,
		Send:     func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	res, err := mgr.Run(ctx, RunParams{
		ParentConversationID: "parent-conv",
		Prompt:               "trabalho longo",
		Background:           true,
		Timeout:              20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run bg erro: %v", err)
	}
	if res.Status != StatusRunning {
		t.Fatalf("status imediato esperado running, veio %q", res.Status)
	}

	// Espera o aviso de conclusão do background com status terminal.
	select {
	case n := <-delivery.ch:
		if n.Status != StatusTimedOut {
			t.Fatalf("esperava timed_out pelo Timeout explícito, veio %q", n.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("aviso de conclusão do background não chegou no prazo")
	}
}

// TestDeriveProvenanceSourceDefaults garante a normalização do Source ao
// contrato do eventctx/AEP-0067 ("user"/"job"): sem carimbo ou Source vazio →
// "user"; "job" preservado; existingChainID só preenche ChainID vazio sem mexer
// no Source, e ChainID herdado não é sobrescrito.
func TestDeriveProvenanceSourceDefaults(t *testing.T) {
	if p := deriveProvenance(context.Background(), ""); p.Source != "user" {
		t.Fatalf("sem carimbo esperava Source=user, veio %q", p.Source)
	}
	if p := deriveProvenance(context.Background(), ""); p.Source == Source {
		t.Fatalf("Source nunca deve ser subagent.Source (%q)", Source)
	}
	jctx := eventctx.With(context.Background(), eventctx.Provenance{Source: "job", SourceJobID: "j1"})
	if p := deriveProvenance(jctx, ""); p.Source != "job" {
		t.Fatalf("com job esperava Source=job, veio %q", p.Source)
	}
	ectx := eventctx.With(context.Background(), eventctx.Provenance{Source: ""})
	if p := deriveProvenance(ectx, ""); p.Source != "user" {
		t.Fatalf("Source vazio carimbado esperava user, veio %q", p.Source)
	}
	if p := deriveProvenance(context.Background(), "conv-x"); p.ChainID != "conv-x" || p.Source != "user" {
		t.Fatalf("esperava ChainID=conv-x/Source=user, veio %q/%q", p.ChainID, p.Source)
	}
	cctx := eventctx.With(context.Background(), eventctx.Provenance{Source: "job", ChainID: "chain-1"})
	if p := deriveProvenance(cctx, "conv-x"); p.ChainID != "chain-1" {
		t.Fatalf("ChainID herdado deveria ser preservado (chain-1), veio %q", p.ChainID)
	}
}

// provCapturingDelivery captura a proveniência (eventctx) do ctx de entrega para
// verificar a continuidade da cadeia (_chain_id) no auto-wake.
type provCapturingDelivery struct {
	ch chan eventctx.Provenance
}

func (d provCapturingDelivery) Deliver(ctx context.Context, _ ParentNotice) error {
	prov, _ := eventctx.From(ctx)
	select {
	case d.ch <- prov:
	default:
	}
	return nil
}

// TestManagerRunUserFlowDerivesStableChainID garante que, num fluxo de usuário
// (ctx sem eventctx), o run persiste um ChainID estável (= conv.ID, não vazio) e
// que a entrega/auto-wake propaga o MESMO ChainID com Source=user — preservando
// a continuidade da cadeia/circuit breaker do AEP-0067.
func TestManagerRunUserFlowDerivesStableChainID(t *testing.T) {
	repo, ctx := setupManagerTest(t) // ctx => user-a, SEM eventctx carimbado
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	provCh := make(chan eventctx.Provenance, 1)
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Delivery: provCapturingDelivery{ch: provCh},
		Send: func(_ context.Context, p SendParams) (string, error) {
			go notifier.Notify(p.ConversationID, "ok", "msg")
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(ctx, RunParams{
		ParentConversationID: "parent-conv",
		Prompt:               "trabalho",
		Background:           true,
	})
	if err != nil {
		t.Fatalf("Run erro: %v", err)
	}

	// O run persistido tem ChainID estável = conv.ID (não vazio).
	got, err := repo.Get(ctx, res.RunID)
	if err != nil {
		t.Fatalf("Get run: %v", err)
	}
	if got.ChainID == "" {
		t.Fatal("ChainID do run não deveria ser vazio em fluxo de usuário")
	}
	if got.ChainID != res.ConversationID {
		t.Fatalf("ChainID deveria ser semeado de conv.ID (%q), veio %q", res.ConversationID, got.ChainID)
	}

	// A entrega/auto-wake propaga o MESMO ChainID, com Source=user.
	select {
	case prov := <-provCh:
		if prov.ChainID != got.ChainID {
			t.Fatalf("auto-wake deveria propagar o mesmo ChainID (%q), veio %q", got.ChainID, prov.ChainID)
		}
		if prov.Source != "user" {
			t.Fatalf("auto-wake esperava Source=user, veio %q", prov.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("entrega/auto-wake não ocorreu no prazo")
	}
}

// ---- B2: idempotência/erros do deliver ----

// getFailRepo falha o Get para exercitar a idempotência fail-closed do deliver.
type getFailRepo struct {
	Repository
	fail bool
}

func (r *getFailRepo) Get(ctx context.Context, id string) (*database.SubAgentRun, error) {
	if r.fail {
		return nil, fmt.Errorf("falha simulada no Get")
	}
	return r.Repository.Get(ctx, id)
}

// deliveredUpdateFailRepo falha o Update que grava DeliveredAt (run já entregue).
type deliveredUpdateFailRepo struct {
	Repository
}

func (r *deliveredUpdateFailRepo) Update(ctx context.Context, run *database.SubAgentRun) error {
	if run != nil && run.DeliveredAt != nil {
		return fmt.Errorf("falha simulada ao persistir DeliveredAt")
	}
	return r.Repository.Update(ctx, run)
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}

func seedDeliverableRun(t *testing.T, repo Repository, ctx context.Context) *database.SubAgentRun {
	t.Helper()
	run := &database.SubAgentRun{
		ParentConversationID: "parent-conv",
		ParentTurnID:         "parent-turn",
		ChildConversationID:  "child-conv",
		Status:               database.SubAgentRunStatusSucceeded,
		ResultSummary:        "ok",
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create run: %v", err)
	}
	return run
}

// TestDeliverIdempotencyFailClosed garante que, se não for possível confirmar o
// estado de entrega (repo.Get erro), o deliver NÃO entrega (fail-closed) e loga.
func TestDeliverIdempotencyFailClosed(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	run := seedDeliverableRun(t, repo, ctx)
	delivery := &recordingDelivery{}
	logBuf := captureLog(t)

	mgr := NewManager(ManagerConfig{
		Repo:     &getFailRepo{Repository: repo, fail: true},
		Notifier: messaging.NewResponseNotifier(),
		Delivery: delivery,
		Send:     func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})
	t.Cleanup(mgr.notifier.Stop)

	mgr.deliver(ctx, run, RunParams{ParentConversationID: run.ParentConversationID})

	if delivery.count() != 0 {
		t.Fatalf("fail-closed: não deveria entregar quando Get falha, entregou %d", delivery.count())
	}
	if !strings.Contains(logBuf.String(), "fail-closed") {
		t.Fatalf("esperava log de fail-closed, veio: %q", logBuf.String())
	}
}

// TestDeliverDeliveryErrorIsLoggedAndNotMarked garante que falha de Deliver é
// logada (best-effort) e NÃO marca DeliveredAt (permite reentrega).
func TestDeliverDeliveryErrorIsLoggedAndNotMarked(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	run := seedDeliverableRun(t, repo, ctx)
	delivery := &recordingDelivery{err: fmt.Errorf("falha de entrega")}
	logBuf := captureLog(t)

	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: messaging.NewResponseNotifier(),
		Delivery: delivery,
		Send:     func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})
	t.Cleanup(mgr.notifier.Stop)

	mgr.deliver(ctx, run, RunParams{ParentConversationID: run.ParentConversationID})

	if delivery.count() != 1 {
		t.Fatalf("Deliver deveria ter sido tentado uma vez, veio %d", delivery.count())
	}
	got, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DeliveredAt != nil {
		t.Fatal("DeliveredAt não deveria ser marcado quando Deliver falha (reentrega)")
	}
	if !strings.Contains(logBuf.String(), "falha ao entregar") {
		t.Fatalf("esperava log de falha de entrega, veio: %q", logBuf.String())
	}
}

// TestDeliverDeliveredAtPersistErrorIsLogged garante que o erro ao persistir
// DeliveredAt NÃO é engolido: a entrega ocorre, mas o erro é logado.
func TestDeliverDeliveredAtPersistErrorIsLogged(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	run := seedDeliverableRun(t, repo, ctx)
	delivery := &recordingDelivery{}
	logBuf := captureLog(t)

	mgr := NewManager(ManagerConfig{
		Repo:     &deliveredUpdateFailRepo{Repository: repo},
		Notifier: messaging.NewResponseNotifier(),
		Delivery: delivery,
		Send:     func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})
	t.Cleanup(mgr.notifier.Stop)

	mgr.deliver(ctx, run, RunParams{ParentConversationID: run.ParentConversationID})

	if delivery.count() != 1 {
		t.Fatalf("Deliver deveria ter ocorrido uma vez, veio %d", delivery.count())
	}
	if !strings.Contains(logBuf.String(), "DeliveredAt") {
		t.Fatalf("esperava log do erro de persistência de DeliveredAt, veio: %q", logBuf.String())
	}
}

// ---- B4: striped locks por conversa-pai ----

// TestParentLockStripedStable garante que o mesmo parentID mapeia sempre para o
// MESMO mutex (serialização por pai preservada) e que parentLock nunca retorna
// nil para qualquer id (sem crescimento de map: array fixo).
func TestParentLockStripedStable(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	// Mesmo parentID deve devolver sempre o MESMO mutex (serialização por pai).
	p1First := mgr.parentLock("p1")
	p1Again := mgr.parentLock("p1")
	if p1First != p1Again {
		t.Fatal("mesmo parentID deveria devolver o mesmo mutex (stripe estável)")
	}
	alphaFirst := mgr.parentLock("alpha")
	alphaAgain := mgr.parentLock("alpha")
	if alphaFirst != alphaAgain {
		t.Fatal("mapeamento por id deveria ser estável")
	}
	for i := 0; i < 5000; i++ {
		if mgr.parentLock(fmt.Sprintf("parent-%d", i)) == nil {
			t.Fatalf("parentLock retornou nil para id %d", i)
		}
	}
}

func TestManagerRunRequiresUserScope(t *testing.T) {
	repo, _ := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	if _, err := mgr.Run(context.Background(), RunParams{Prompt: "faça X"}); err == nil {
		t.Fatal("esperava erro de escopo de usuário ausente")
	}
}

// TestWaitPrefersDoneOverCancelAndTimeout garante a semântica de prioridade do
// select no limite: com a resposta já disponível em `done`, mesmo que cancelCh,
// timer e ctx também estejam prontos, o desfecho deve ser succeeded (não pode
// virar cancelled/timed_out por causa do não-determinismo do select).
func TestWaitPrefersDoneOverCancelAndTimeout(t *testing.T) {
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	m := &Manager{notifier: notifier, now: time.Now}

	for i := 0; i < 300; i++ {
		done := make(chan completion, 1)
		done <- completion{response: "ok", assistantMessageID: "msg"}
		ar := &activeRun{childConversationID: "c", cancelCh: make(chan struct{})}
		close(ar.cancelCh) // cancel também pronto ao mesmo tempo que done

		cctx, cancel := context.WithCancel(context.Background())
		cancel() // ctx.Done() também pronto

		o := m.wait(cctx, "c", done, ar, time.Nanosecond, false) // timer praticamente pronto
		if o.status != StatusSucceeded {
			t.Fatalf("iter %d: com done disponível esperava succeeded, veio %q", i, o.status)
		}
		if o.summary != "ok" || o.assistantMessageID != "msg" {
			t.Fatalf("iter %d: desfecho de sucesso incompleto: %#v", i, o)
		}
	}
}

// runningUpdateFailRepo embute um Repository real mas falha o Update que marca
// o run como running (segundo Update da vida do run), permitindo exercitar o
// tratamento de erro de persistência da transição para running.
type runningUpdateFailRepo struct {
	Repository
	failOnStatus string
}

func (r *runningUpdateFailRepo) Update(ctx context.Context, run *database.SubAgentRun) error {
	if run != nil && run.Status == r.failOnStatus {
		return fmt.Errorf("falha simulada ao persistir status %q", r.failOnStatus)
	}
	return r.Repository.Update(ctx, run)
}

// TestManagerRunningUpdateErrorAborts garante que, se a transição para running
// não puder ser persistida, o Run aborta ANTES de enviar (não deixa trabalho
// órfão) e reporta failed, em vez de descartar o erro silenciosamente.
func TestManagerRunningUpdateErrorAborts(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	sendCalled := false
	mgr := NewManager(ManagerConfig{
		Repo:     &runningUpdateFailRepo{Repository: repo, failOnStatus: database.SubAgentRunStatusRunning},
		Notifier: notifier,
		Send: func(_ context.Context, p SendParams) (string, error) {
			sendCalled = true
			return p.ConversationID, nil
		},
	})

	res, err := mgr.Run(ctx, RunParams{Prompt: "faça X"})
	if err != nil {
		t.Fatalf("Run não deve retornar erro de pré-condição: %v", err)
	}
	if res.Status != StatusFailed {
		t.Fatalf("status esperado failed, veio %q", res.Status)
	}
	if res.Error == "" {
		t.Fatal("esperava mensagem de erro no RunResult")
	}
	if sendCalled {
		t.Fatal("Send NÃO deveria ser chamado quando a transição para running falha")
	}
}
