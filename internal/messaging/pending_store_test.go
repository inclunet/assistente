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

func TestResponseNotifier_TTLExpiresChannelPendingWhileInternalRemains(t *testing.T) {
	store := newMemPendingStore()
	now := time.Now()
	clock := func() time.Time { return now }
	n := newResponseNotifierWithClock(clock)
	defer n.Stop()
	n.SetPendingStore(store)

	n.Register("conv-mixed", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "123",
		OwnerUserID: "owner-1",
		Callback: func(string, string) {
			t.Fatal("callback de canal expirado não deveria ser chamado")
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

	// M14: store dura além do TTL in-memory (LLM longo / crash antes do Notify).
	rows, _ = store.List(context.Background())
	if len(rows) != 1 {
		t.Fatalf("pending de canal deve permanecer no store após TTL in-memory; got %+v", rows)
	}
	if n.PendingCount() != 1 {
		t.Fatalf("callback interno deveria permanecer; pending=%d", n.PendingCount())
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

func TestGateway_ReconcilePending_ReregisterDoesNotDuplicateCallbacks(t *testing.T) {
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

	// Callback prévio em memória (não deve sobreviver ao reconcile).
	notifier.Register("conv-dup", ResponseCallback{
		Channel:     "telegram",
		ChatID:      "55",
		OwnerUserID: "owner-1",
		SkipPersist: true,
		Callback:    func(string, string) {},
	})
	if notifier.PendingCount() != 1 {
		t.Fatalf("setup: pending=%d", notifier.PendingCount())
	}

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 2)}
	gateway := NewGateway(notifier, nil, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.ReconcilePending(context.Background(), func(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
		return "", "", false, nil
	})
	if notifier.PendingCount() != 1 {
		t.Fatalf("após reconcile deveria haver exatamente 1 callback, got %d", notifier.PendingCount())
	}

	notifier.Notify("conv-dup", "uma vez", "asst")
	select {
	case msg := <-fake.sentCh:
		if msg.Text != "uma vez" {
			t.Fatalf("outbound inesperado: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando envio único")
	}
	select {
	case msg := <-fake.sentCh:
		t.Fatalf("envio duplicado: %+v", msg)
	case <-time.After(150 * time.Millisecond):
		// ok — sem segundo envio
	}
}
