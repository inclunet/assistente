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
	if len(rows) != 0 {
		t.Fatalf("pending deveria ser removido no Notify, got %+v", rows)
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

	rows, _ = store.List(context.Background())
	if len(rows) != 0 {
		t.Fatalf("pending de canal deveria ser removido no TTL mesmo com callback interno vivo; got %+v", rows)
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

func TestGateway_ReconcilePending_ResendsSavedAssistant(t *testing.T) {
	store := newMemPendingStore()
	_ = store.Upsert(context.Background(), ChannelPendingRecord{
		ConversationID: "conv-r",
		Channel:        "telegram",
		ChatID:         "99",
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
