package subagent

import (
	"context"
	"fmt"
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

func TestManagerResumeReusesConversationAndIncrementsTurn(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) {
		go notifier.Notify(p.ConversationID, "ok", "m")
		return p.ConversationID, nil
	}})

	first, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", Prompt: "passo 1"})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// Resume: mesma sub-conversa, novo run com turn_index incrementado.
	second, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", ConversationID: first.ConversationID, Prompt: "passo 2"})
	if err != nil {
		t.Fatalf("Run 2 (resume): %v", err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("resume deveria reusar a sub-conversa: %q != %q", second.ConversationID, first.ConversationID)
	}
	if second.RunID == first.RunID {
		t.Fatal("resume deveria criar um novo run")
	}
	run2, err := repo.Get(ctx, second.RunID)
	if err != nil {
		t.Fatalf("buscar run 2: %v", err)
	}
	if run2.TurnIndex != 1 {
		t.Fatalf("turn_index esperado 1, veio %d", run2.TurnIndex)
	}
}

func TestManagerResumeRejectsNonSubagentConversation(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	// Conversa normal (Kind vazio) criada diretamente.
	normal := &database.Conversation{UserID: "user-a", Title: "normal"}
	if err := database.DB().WithContext(ctx).Create(normal).Error; err != nil {
		t.Fatalf("criar conversa normal: %v", err)
	}

	if _, err := mgr.Run(ctx, RunParams{ConversationID: normal.ID, Prompt: "x"}); err == nil {
		t.Fatal("esperava erro ao reusar conversa que não é de sub-agente")
	}
}

func TestManagerClearResetsHistoryBeforeSend(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) {
		go notifier.Notify(p.ConversationID, "ok", "m")
		return p.ConversationID, nil
	}})

	first, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", Prompt: "passo 1"})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// Simula histórico e resumo na sub-conversa.
	msg := &database.ChatMessage{ConversationID: first.ConversationID, Role: "user", Content: "antigo"}
	if err := database.DB().WithContext(ctx).Create(msg).Error; err != nil {
		t.Fatalf("criar mensagem: %v", err)
	}
	if err := database.UpdateConversationSummaryWithContext(ctx, first.ConversationID, "resumo antigo", msg.ID); err != nil {
		t.Fatalf("set summary: %v", err)
	}

	// clear: reset + envio.
	if _, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", ConversationID: first.ConversationID, Clear: true, Prompt: "novo problema"}); err != nil {
		t.Fatalf("Run clear: %v", err)
	}

	var count int64
	if err := database.DB().WithContext(ctx).Model(&database.ChatMessage{}).Where("conversation_id = ? AND content = ?", first.ConversationID, "antigo").Count(&count).Error; err != nil {
		t.Fatalf("contar mensagens: %v", err)
	}
	if count != 0 {
		t.Fatalf("clear deveria ter removido o histórico antigo; restaram %d", count)
	}
	summary, _, err := database.GetConversationSummaryWithContext(ctx, first.ConversationID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "" {
		t.Fatalf("clear deveria ter limpado o resumo; veio %q", summary)
	}
}

func TestManagerInheritsJobProvenance(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) {
		go notifier.Notify(p.ConversationID, "ok", "m")
		return p.ConversationID, nil
	}})

	// Simula um job chamando o sub-agente: o ctx carrega a proveniência do job.
	jobCtx := eventctx.With(ctx, eventctx.Provenance{Source: "job", SourceJobID: "job-a", ChainID: "chain-1", ChainHistory: []string{"job-a"}})

	res, err := mgr.Run(jobCtx, RunParams{Prompt: "tarefa do job"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	run, err := repo.Get(ctx, res.RunID)
	if err != nil {
		t.Fatalf("buscar run: %v", err)
	}
	if run.ChainID != "chain-1" {
		t.Fatalf("chain_id do job não herdado: %q", run.ChainID)
	}
	if run.ChainHistory == "" || !contains(run.ChainHistory, "job-a") {
		t.Fatalf("chain_history do job não preservado: %q", run.ChainHistory)
	}
}

// TestManagerAppendsRunIDToProvenanceBeforeSend garante que o run.ID é anexado
// à cadeia de proveniência ANTES do envio (fix review PR #186): assim qualquer
// sub-agente/job disparado DENTRO deste run vê a cadeia maior e o backstop de
// profundidade cresce nível a nível.
func TestManagerAppendsRunIDToProvenanceBeforeSend(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)

	var sentProv eventctx.Provenance
	var sentOK bool
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(sctx context.Context, p SendParams) (string, error) {
		sentProv, sentOK = eventctx.From(sctx)
		go notifier.Notify(p.ConversationID, "ok", "m")
		return p.ConversationID, nil
	}})

	jobCtx := eventctx.With(ctx, eventctx.Provenance{Source: "job", ChainID: "chain-1", ChainHistory: []string{"job-a"}})
	res, err := mgr.Run(jobCtx, RunParams{Prompt: "tarefa"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sentOK {
		t.Fatal("ctx de envio deveria carregar proveniência (eventctx)")
	}
	if len(sentProv.ChainHistory) != 2 {
		t.Fatalf("cadeia deveria crescer de 1 para 2 ao enviar; veio %v", sentProv.ChainHistory)
	}
	last := sentProv.ChainHistory[len(sentProv.ChainHistory)-1]
	if last != res.RunID {
		t.Fatalf("último elo da cadeia deveria ser o run.ID (%s); veio %q", res.RunID, last)
	}
}

func TestManagerChainDepthBackstop(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	sendCalled := false
	mgr := NewManager(ManagerConfig{
		Repo:          repo,
		Notifier:      notifier,
		MaxChainDepth: 2,
		Send:          func(_ context.Context, p SendParams) (string, error) { sendCalled = true; return p.ConversationID, nil },
	})

	// Cadeia já no limite (2 itens) → deve recusar antes de criar conversa/run.
	deepCtx := eventctx.With(ctx, eventctx.Provenance{Source: "job", ChainID: "c", ChainHistory: []string{"a", "b"}})
	if _, err := mgr.Run(deepCtx, RunParams{Prompt: "x"}); err == nil {
		t.Fatal("esperava erro de profundidade de cadeia")
	}
	if sendCalled {
		t.Fatal("não deveria ter enviado nada ao atingir o limite de cadeia")
	}
}

func TestManagerReconcileOrphans(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	mkRun := func(status string) string {
		conv, err := database.CreateSubAgentConversationWithContext(ctx, "t", "parent")
		if err != nil {
			t.Fatalf("criar conv: %v", err)
		}
		run := &database.SubAgentRun{UserID: "user-a", ParentConversationID: "parent", ChildConversationID: conv.ID, Status: status}
		if err := repo.Create(ctx, run); err != nil {
			t.Fatalf("criar run: %v", err)
		}
		return run.ID
	}
	runningID := mkRun(StatusRunning)
	queuedID := mkRun(StatusQueued)
	succeededID := mkRun(StatusSucceeded)

	// cutoff no futuro: inclui os runs já criados (simulando órfãos de um
	// processo anterior).
	n, err := mgr.ReconcileOrphans(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("ReconcileOrphans: %v", err)
	}
	if n != 2 {
		t.Fatalf("esperava 2 runs reconciliados, veio %d", n)
	}

	for _, id := range []string{runningID, queuedID} {
		run, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("buscar run %s: %v", id, err)
		}
		if run.Status != StatusFailed || run.Error == "" || run.CompletedAt == nil {
			t.Fatalf("run órfão %s não reconciliado: %#v", id, run)
		}
	}
	done, err := repo.Get(ctx, succeededID)
	if err != nil {
		t.Fatalf("buscar run concluído: %v", err)
	}
	if done.Status != StatusSucceeded {
		t.Fatalf("run terminal não deveria ser alterado; veio %q", done.Status)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// TestManagerReconcileOrphansRespectsCutoff garante que a reconciliação NÃO
// marca como órfão um run criado após o cutoff (instante de início do app):
// runs legítimos criados em paralelo ao startup devem ser preservados.
func TestManagerReconcileOrphansRespectsCutoff(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	conv, err := database.CreateSubAgentConversationWithContext(ctx, "t", "parent")
	if err != nil {
		t.Fatalf("criar conv: %v", err)
	}
	run := &database.SubAgentRun{UserID: "user-a", ParentConversationID: "parent", ChildConversationID: conv.ID, Status: StatusRunning}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("criar run: %v", err)
	}

	// cutoff no passado: o run (criado agora) é POSTERIOR ao cutoff → não pode
	// ser reconciliado.
	n, err := mgr.ReconcileOrphans(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ReconcileOrphans: %v", err)
	}
	if n != 0 {
		t.Fatalf("nenhum run deveria ser reconciliado (criado após cutoff), veio %d", n)
	}
	got, err := repo.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("buscar run: %v", err)
	}
	if got.Status != StatusRunning {
		t.Fatalf("run criado após cutoff não deveria mudar de status; veio %q", got.Status)
	}
}

// TestManagerReconcileOrphansUnconfiguredFails garante que, sem repo/manager, a
// reconciliação falha explicitamente (não mascara wiring quebrado no startup).
func TestManagerReconcileOrphansUnconfiguredFails(t *testing.T) {
	var nilMgr *Manager
	if _, err := nilMgr.ReconcileOrphans(context.Background(), time.Now()); err == nil {
		t.Fatal("esperava erro com Manager nil")
	}
	mgr := &Manager{} // sem repo
	if _, err := mgr.ReconcileOrphans(context.Background(), time.Now()); err == nil {
		t.Fatal("esperava erro com repo não configurado")
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

func TestManagerRunRequiresUserScope(t *testing.T) {
	repo, _ := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{Repo: repo, Notifier: notifier, Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil }})

	if _, err := mgr.Run(context.Background(), RunParams{Prompt: "faça X"}); err == nil {
		t.Fatal("esperava erro de escopo de usuário ausente")
	}
}

// TestManagerFailFastConcurrentRunSameConversation garante o fail-fast (AEP-0068):
// enquanto houver um run ATIVO em uma sub-conversa, iniciar outro run (resume) na
// MESMA sub-conversa deve falhar de imediato, em vez de dois runs disputarem o
// mesmo ResponseNotifier (que é indexado só por conversationID).
func TestManagerFailFastConcurrentRunSameConversation(t *testing.T) {
	repo, ctx := setupManagerTest(t)
	notifier := messaging.NewResponseNotifier()
	t.Cleanup(notifier.Stop)
	mgr := NewManager(ManagerConfig{
		Repo:     repo,
		Notifier: notifier,
		Delivery: &recordingDelivery{},
		// Nunca notifica → o 1º run permanece ativo.
		Send: func(_ context.Context, p SendParams) (string, error) { return p.ConversationID, nil },
	})

	// 1º run em background → fica ativo (sem notificação). Timeout curto só para
	// o goroutine encerrar após o teste.
	first, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", Prompt: "tarefa longa", Background: true, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("primeiro Run: %v", err)
	}

	// 2º run (resume) na MESMA sub-conversa enquanto o 1º está ativo → fail-fast.
	if _, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", ConversationID: first.ConversationID, Prompt: "tarefa concorrente"}); err == nil {
		t.Fatal("esperava fail-fast ao iniciar run concorrente na mesma sub-conversa")
	}

	// Cancela o 1º para liberar a reserva e, então, um novo run na mesma
	// sub-conversa deve ser aceito.
	if _, err := mgr.Cancel(ctx, first.ConversationID, first.RunID); err != nil {
		t.Fatalf("cancelar 1º run: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := mgr.Run(ctx, RunParams{ParentConversationID: "parent-conv", ConversationID: first.ConversationID, Prompt: "agora pode", Background: true, Timeout: time.Second}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("após cancelar o 1º run, a sub-conversa deveria aceitar novo run")
		}
		time.Sleep(10 * time.Millisecond)
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
