package messaging

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memPendingStore struct {
	mu   sync.Mutex
	rows map[string]ChannelPendingRecord
}

func newMemPendingStore() *memPendingStore {
	return &memPendingStore{rows: make(map[string]ChannelPendingRecord)}
}

func (s *memPendingStore) Upsert(ctx context.Context, rec ChannelPendingRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[rec.ConversationID] = rec
	return nil
}

func (s *memPendingStore) Get(ctx context.Context, conversationID string) (ChannelPendingRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.rows[conversationID]
	return rec, ok, nil
}

func (s *memPendingStore) Delete(ctx context.Context, conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, conversationID)
	return nil
}

func (s *memPendingStore) DeleteIfTrace(ctx context.Context, conversationID, traceID string) error {
	if conversationID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.rows[conversationID]
	if !ok {
		return nil
	}
	if rec.TraceID == traceID {
		delete(s.rows, conversationID)
	}
	return nil
}

func (s *memPendingStore) MarkDelivered(ctx context.Context, conversationID, traceID, assistantMsgID string) error {
	if conversationID == "" || assistantMsgID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.rows[conversationID]
	if !ok || rec.TraceID != traceID {
		return nil
	}
	rec.DeliveredAssistantID = assistantMsgID
	s.rows[conversationID] = rec
	return nil
}

func (s *memPendingStore) List(ctx context.Context) ([]ChannelPendingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ChannelPendingRecord, 0, len(s.rows))
	for _, r := range s.rows {
		out = append(out, r)
	}
	return out, nil
}

func TestResponseNotifier_PersistsChannelCallback(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "123",
		OwnerUserID: "owner-1",
		TraceID:     "t1",
		Callback:    func(string, string) {},
	})

	rows, _ := store.List(context.Background())
	if len(rows) != 1 || rows[0].ConversationID != "conv-1" {
		t.Fatalf("esperava 1 pending, got %+v", rows)
	}

	n.Notify("conv-1", "oi", "asst-1")
	rows, _ = store.List(context.Background())
	if len(rows) != 1 {
		t.Fatalf("Notify não deve apagar pending antes do Send; got %+v", rows)
	}
	// Simula entrega bem-sucedida (caminho real: deliverChannelResponse).
	_ = store.Delete(context.Background(), "conv-1")
	rows, _ = store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("pending deveria sumir após Delete pós-Send, got %+v", rows)
	}
}

func TestResponseNotifier_NotifyContextKeepsOtherTrace(t *testing.T) {
	n := NewResponseNotifier()
	defer n.Stop()

	oldFired := make(chan struct{}, 1)
	newFired := make(chan struct{}, 1)
	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "1",
		OwnerUserID: "owner-1",
		TraceID:     "trace-old",
		Callback:    func(string, string) { oldFired <- struct{}{} },
	})
	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "1",
		OwnerUserID: "owner-1",
		TraceID:     "trace-new",
		Callback:    func(string, string) { newFired <- struct{}{} },
	})
	// Segundo Register !SkipPersist substitui o primeiro — só new resta.
	if n.PendingCount() != 1 {
		t.Fatalf("pending=%d", n.PendingCount())
	}

	// Simula Notify atrasado do turno antigo (trace-old): não deve consumir o novo.
	n.NotifyContext(WithChannelTraceID(context.Background(), "trace-old"), "conv-1", "atrasado", "asst-old")
	select {
	case <-oldFired:
		t.Fatal("callback antigo já tinha sido substituído")
	case <-newFired:
		t.Fatal("Notify do turno antigo não pode disparar o callback novo")
	case <-time.After(100 * time.Millisecond):
	}
	if n.PendingCount() != 1 {
		t.Fatalf("callback novo deveria permanecer; pending=%d", n.PendingCount())
	}

	n.NotifyContext(WithChannelTraceID(context.Background(), "trace-new"), "conv-1", "ok", "asst-new")
	select {
	case <-newFired:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify do turno novo deveria disparar")
	}
}

func TestResponseNotifier_RegisterReplacesPriorChannelCallback(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	oldFired := make(chan struct{}, 1)
	newFired := make(chan struct{}, 1)
	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "1",
		OwnerUserID: "owner-1",
		TraceID:     "trace-old",
		Callback:    func(string, string) { oldFired <- struct{}{} },
	})
	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "1",
		OwnerUserID: "owner-1",
		TraceID:     "trace-new",
		Callback:    func(string, string) { newFired <- struct{}{} },
	})
	if n.PendingCount() != 1 {
		t.Fatalf("Register de canal deveria substituir o anterior; pending=%d", n.PendingCount())
	}
	rows, _ := store.List(context.Background())
	if len(rows) != 1 || rows[0].TraceID != "trace-new" {
		t.Fatalf("store deveria ter só turno novo; got %+v", rows)
	}

	n.Notify("conv-1", "só uma", "asst")
	select {
	case <-newFired:
	case <-time.After(2 * time.Second):
		t.Fatal("callback novo deveria disparar")
	}
	select {
	case <-oldFired:
		t.Fatal("callback antigo não deveria disparar após substituição")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestResponseNotifier_CancelDeletesPending(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	n.Register("conv-1", ResponseCallback{
		Channel:     "signal",
		ChatID:      "+5511",
		OwnerUserID: "owner-1",
		Callback:    func(string, string) {},
	})
	n.Cancel("conv-1")
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("pending deveria ser removido no Cancel")
	}
}

func TestResponseNotifier_CancelTraceKeepsNewerPending(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "1",
		OwnerUserID: "owner-1",
		TraceID:     "trace-old",
		Callback:    func(string, string) {},
	})
	n.Register("conv-1", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "2",
		OwnerUserID: "owner-1",
		TraceID:     "trace-new",
		Callback:    func(string, string) {},
	})
	// Upsert sobrescreve: store fica com o turno novo.
	rows, _ := store.List(context.Background())
	if len(rows) != 1 || rows[0].TraceID != "trace-new" {
		t.Fatalf("store deveria ter só trace-new; got %+v", rows)
	}

	n.CancelTrace("conv-1", "trace-old")
	rows, _ = store.List(context.Background())
	if len(rows) != 1 || rows[0].TraceID != "trace-new" {
		t.Fatalf("CancelTrace do turno antigo não pode apagar pending novo; got %+v", rows)
	}
	if n.PendingCount() != 1 {
		t.Fatalf("callback do turno novo deveria permanecer; pending=%d", n.PendingCount())
	}

	n.CancelTrace("conv-1", "trace-new")
	rows, _ = store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("CancelTrace do turno atual deveria apagar pending; got %+v", rows)
	}
	if n.PendingCount() != 0 {
		t.Fatalf("nenhum callback deveria restar; pending=%d", n.PendingCount())
	}
}

func TestResponseNotifier_SkipsPersistingInternalCallbacks(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	n.Register("conv-sub", ResponseCallback{
		Channel: "subagent",
		ChatID:  "internal-1",
		TTL:     time.Hour,
		Callback: func(string, string) {},
	})
	n.Register("conv-tel", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "123",
		OwnerUserID: "owner-1",
		Callback:    func(string, string) {},
	})

	rows, _ := store.List(context.Background())
	if len(rows) != 1 || rows[0].ConversationID != "conv-tel" {
		t.Fatalf("só telegram deveria persistir; got %+v", rows)
	}
}

func TestResponseNotifier_DurableChannelSurvivesTTLAndStillNotifies(t *testing.T) {
	store := newMemPendingStore()
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()
	n.SetPendingStore(store)

	fired := make(chan struct{}, 1)
	n.Register("conv-mixed", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "123",
		OwnerUserID: "owner-1",
		Callback: func(string, string) {
			fired <- struct{}{}
		},
	})
	n.Register("conv-mixed", ResponseCallback{
		Channel: "ui",
		ChatID:  "",
		TTL:     time.Hour,
		Callback: func(string, string) {},
	})

	rows, _ := store.List(context.Background())
	if len(rows) != 1 {
		t.Fatalf("esperava pending telegram, got %+v", rows)
	}

	now = now.Add(callbackTTL + time.Minute)
	n.expireOldCallbacks()

	// M14: canal persistido permanece em memória E no store além do TTL padrão.
	rows, _ = store.List(context.Background())
	if len(rows) != 1 {
		t.Fatalf("pending de canal deve permanecer no store; got %+v", rows)
	}
	if n.PendingCount() != 2 {
		t.Fatalf("canal durável + ui deveriam permanecer; pending=%d", n.PendingCount())
	}

	n.Notify("conv-mixed", "tarde", "asst-1")
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify após TTL padrão deveria entregar callback de canal durável")
	}
}

func TestResponseNotifier_SkipsPersistWithoutOwnerUserID(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	n.Register("conv-no-owner", ResponseCallback{
		Channel: "telegram",
		ChatID:  "123",
		Callback: func(string, string) {},
	})
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("sem OwnerUserID não deveria persistir; got %+v", rows)
	}
}

func TestResponseNotifier_DeleteIfTraceKeepsNewerTurn(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-t",
		Channel:        "telegram",
		ChatID:         "1",
		TraceID:        "trace-old",
		OwnerUserID:    "owner-1",
	})
	// Turno novo sobrescreve.
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-t",
		Channel:        "telegram",
		ChatID:         "2",
		TraceID:        "trace-new",
		OwnerUserID:    "owner-1",
	})
	_ = store.DeleteIfTrace(context.Background(), "conv-t", "trace-old")
	rows, _ := store.List(context.Background())
	if len(rows) != 1 || rows[0].TraceID != "trace-new" {
		t.Fatalf("DeleteIfTrace do turno antigo não deveria apagar o novo; got %+v", rows)
	}
	_ = store.DeleteIfTrace(context.Background(), "conv-t", "trace-new")
	rows, _ = store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("DeleteIfTrace do turno atual deveria apagar; got %+v", rows)
	}
}

func TestResponseNotifier_SkipPersistDoesNotOverwritePendingChatID(t *testing.T) {
	store := newMemPendingStore()
	n := NewResponseNotifier()
	defer n.Stop()
	n.SetPendingStore(store)

	// Gateway: destino final (ex.: Slack channel).
	n.Register("conv-1", ResponseCallback{
		Channel:     "slack",
		ChatID:      "C999",
		OwnerUserID: "owner-1",
		Callback:    func(string, string) {},
	})
	// Bridge Wails: ChatID provisório (contact/user) — não pode sobrescrever o pending.
	n.Register("conv-1", ResponseCallback{
		Channel:     "slack",
		ChatID:      "U123",
		OwnerUserID: "owner-1",
		SkipPersist: true,
		Callback:    func(string, string) {},
	})

	rows, _ := store.List(context.Background())
	if len(rows) != 1 {
		t.Fatalf("esperava 1 pending, got %+v", rows)
	}
	if rows[0].ChatID != "C999" {
		t.Fatalf("SkipPersist não deveria sobrescrever ChatID; got %q want C999", rows[0].ChatID)
	}
}

func TestGateway_ReconcilePending_ResendsSavedAssistant(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-r",
		Channel:        "telegram",
		ChatID:         "99",
		OwnerUserID:    "owner-1",
		TraceID:        "trace-r",
		CreatedAt:      time.Now().UTC().Add(-time.Minute),
	})

	notifier := NewResponseNotifier()
	defer notifier.Stop()
	notifier.SetPendingStore(store)

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}
	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "resposta salva", "asst-9", true, nil
	})

	select {
	case msg := <-fake.sentCh:
		if msg.ChatID != "99" || msg.Text != "resposta salva" {
			t.Fatalf("outbound inesperado: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando reenvio")
	}
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("pending deveria ser apagado após reconcile, got %+v", rows)
	}
}

func TestResponseNotifier_SkipPersistChannelSurvivesTTL(t *testing.T) {
	// Reconcile re-registra com SkipPersist; ainda precisa sobreviver ao TTL
	// padrão para Notify pós-restart entregar.
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()

	fired := make(chan struct{}, 1)
	n.Register("conv-skip", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "1",
		OwnerUserID: "owner-1",
		SkipPersist: true,
		TTL:         time.Minute, // remaining curto no reconcile
		Callback:    func(string, string) { fired <- struct{}{} },
	})

	now = now.Add(2 * time.Minute)
	n.expireOldCallbacks()
	if n.PendingCount() != 1 {
		t.Fatalf("callback SkipPersist de canal deveria sobreviver ao TTL; pending=%d", n.PendingCount())
	}
	n.Notify("conv-skip", "ok", "asst")
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify deveria entregar callback SkipPersist de canal")
	}
}

func TestGateway_ReconcilePending_SkipsSupersededTrace(t *testing.T) {
	// List inicial devolve stale; pendingTraceState (2º List) vê fresh → pula reenvio.
	staleFirst := &staleThenFreshStore{
		stale: ChannelPendingRecord{
			ConversationID: "conv-s2",
			Channel:        "telegram",
			ChatID:         "2",
			OwnerUserID:    "owner-1",
			TraceID:        "trace-stale",
			CreatedAt:      time.Now().UTC().Add(-time.Minute),
		},
		fresh: ChannelPendingRecord{
			ConversationID: "conv-s2",
			Channel:        "telegram",
			ChatID:         "2",
			OwnerUserID:    "owner-1",
			TraceID:        "trace-fresh",
			CreatedAt:      time.Now().UTC(),
		},
	}
	notifier := NewResponseNotifier()
	defer notifier.Stop()
	notifier.SetPendingStore(staleFirst)
	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}
	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "não enviar", "asst-x", true, nil
	})
	select {
	case msg := <-fake.sentCh:
		t.Fatalf("não deveria reenviar snapshot supersedido: %+v", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

// staleThenFreshStore: primeiro List devolve stale; List seguintes devolvem fresh
// (simula Upsert concorrente entre List inicial e pendingTraceState).
type staleThenFreshStore struct {
	mu    sync.Mutex
	stale ChannelPendingRecord
	fresh ChannelPendingRecord
}

func (s *staleThenFreshStore) Upsert(ctx context.Context, rec ChannelPendingRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh = rec
	return nil
}
func (s *staleThenFreshStore) Get(ctx context.Context, conversationID string) (ChannelPendingRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Get reflete o store atual (já sobrescrito por Upsert concorrente).
	return s.fresh, true, nil
}
func (s *staleThenFreshStore) Delete(ctx context.Context, conversationID string) error {
	return nil
}
func (s *staleThenFreshStore) DeleteIfTrace(ctx context.Context, conversationID, traceID string) error {
	return nil
}
func (s *staleThenFreshStore) MarkDelivered(ctx context.Context, conversationID, traceID, assistantMsgID string) error {
	return nil
}
func (s *staleThenFreshStore) List(ctx context.Context) ([]ChannelPendingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// List inicial do reconcile: snapshot stale (antes do Upsert concorrente).
	return []ChannelPendingRecord{s.stale}, nil
}

func TestGateway_ReconcilePending_AlreadyDeliveredSkipsSend(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID:       "conv-d",
		Channel:              "telegram",
		ChatID:               "55",
		OwnerUserID:          "owner-1",
		TraceID:              "trace-d",
		DeliveredAssistantID: "asst-d",
		CreatedAt:            time.Now().UTC().Add(-time.Minute),
	})

	notifier := NewResponseNotifier()
	defer notifier.Stop()
	notifier.SetPendingStore(store)

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}
	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "já enviado", "asst-d", true, nil
	})

	select {
	case msg := <-fake.sentCh:
		t.Fatalf("não deveria reenviar mensagem já entregue: %+v", msg)
	case <-time.After(200 * time.Millisecond):
	}
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("pending já entregue deveria ser só limpo; got %+v", rows)
	}
}

func TestGateway_ReconcilePending_ExpiredStillResendsSavedAssistant(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-old",
		Channel:        "telegram",
		ChatID:         "77",
		OwnerUserID:    "owner-1",
		TraceID:        "trace-old",
		CreatedAt:      time.Now().UTC().Add(-(callbackTTL + time.Minute)),
	})

	notifier := NewResponseNotifier()
	defer notifier.Stop()
	notifier.SetPendingStore(store)

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}
	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "ainda válida", "asst-old", true, nil
	})

	select {
	case msg := <-fake.sentCh:
		if msg.Text != "ainda válida" {
			t.Fatalf("outbound inesperado: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: pending expirado com assistant deveria reenviar")
	}
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("pending deveria ser apagado após reenvio, got %+v", rows)
	}
}

func TestGateway_ReconcilePending_SkipsReregisterWhenLiveCallback(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-dup",
		Channel:        "telegram",
		ChatID:         "55",
		OwnerUserID:    "owner-1",
		CreatedAt:      time.Now().UTC().Add(-time.Minute),
	})

	notifier := NewResponseNotifier()
	defer notifier.Stop()
	notifier.SetPendingStore(store)

	// Turno vivo (ex.: mensagem chegou durante Connect) — reconcile não substitui.
	live := make(chan string, 1)
	notifier.Register("conv-dup", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "55",
		OwnerUserID: "owner-1",
		TraceID:     "trace-live",
		Callback:    func(resp string, _ string) { live <- resp },
	})
	if notifier.PendingCount() != 1 {
		t.Fatalf("setup: pending=%d", notifier.PendingCount())
	}

	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "", "", false, nil
	})
	if notifier.PendingCount() != 1 {
		t.Fatalf("após reconcile deveria preservar o callback vivo, got %d", notifier.PendingCount())
	}

	notifier.Notify("conv-dup", "turno vivo", "asst")
	select {
	case got := <-live:
		if got != "turno vivo" {
			t.Fatalf("callback vivo não recebeu Notify: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: callback vivo deveria receber Notify")
	}
}

func TestGateway_ReconcilePending_ReregisterWhenNoLiveCallback(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-orphan",
		Channel:        "telegram",
		ChatID:         "55",
		OwnerUserID:    "owner-1",
		TraceID:        "trace-orphan",
		CreatedAt:      time.Now().UTC().Add(-time.Minute),
	})

	notifier := NewResponseNotifier()
	defer notifier.Stop()
	notifier.SetPendingStore(store)

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 2)}
	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "", "", false, nil
	})
	if notifier.PendingCount() != 1 {
		t.Fatalf("reconcile deveria re-registrar callback órfão, got %d", notifier.PendingCount())
	}

	notifier.Notify("conv-orphan", "uma vez", "asst")
	select {
	case msg := <-fake.sentCh:
		if msg.Text != "uma vez" {
			t.Fatalf("outbound inesperado: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando envio do callback re-registrado")
	}
	select {
	case msg := <-fake.sentCh:
		t.Fatalf("envio duplicado: %+v", msg)
	case <-time.After(150 * time.Millisecond):
	}
}
