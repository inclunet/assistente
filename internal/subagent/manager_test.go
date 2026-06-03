package subagent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
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
			return "", context.DeadlineExceeded
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

func TestManagerRunRequiresUserScope(t *testing.T) {
	repo, _ := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	if _, err := mgr.Run(context.Background(), RunParams{Prompt: "faça X"}); err == nil {
		t.Fatal("esperava erro de escopo de usuário ausente")
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
