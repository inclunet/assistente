package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestConversationDeletionInvalidatesCapabilitiesAndReleasesContent(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "session"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	sub, before, err := p.SubscribeTimeline(f.ctx, session, "surface", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	version, err := f.reader.ReadVersion(f.ctx, f.conversation)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := p.Admit(f.ctx, f.conversation, "volatile", "execution", func() (database.ChatTimelineVersion, error) { return version, nil })
	if err != nil {
		t.Fatal(err)
	}
	stream := execution.StartStream(0)
	stream.Append("conteúdo ainda não persistido")
	deleted, err := p.ApplyConversationDeletion(f.ctx, []string{f.conversation}, func() ([]string, error) {
		return database.DeleteConversationsWithContext(f.ctx, []string{f.conversation})
	})
	if err != nil || len(deleted) != 1 {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	if stream.Append("atrasado") {
		t.Fatal("binding antigo aceitou conteúdo")
	}
	if _, err := sub.Refresh(f.ctx); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("refresh: %v", err)
	}
	if _, _, err := sub.Replay(f.ctx, before.Epoch, 0, f.reader); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatalf("replay: %v", err)
	}
	c := execution.conversation
	c.mu.Lock()
	if len(c.turns)+len(c.retained)+len(c.windows)+len(c.publication.items)+len(c.pendingStorageIDs) != 0 || c.publicationTimer != nil || c.storagePublicationErr != nil {
		t.Error("conteúdo excluído permaneceu retido")
	}
	c.mu.Unlock()
	// Importar/recriar o mesmo ID permite outra inscrição, nunca reativa a antiga.
	if err := f.db.Create(&database.Conversation{UUIDModel: database.UUIDModel{ID: f.conversation}, UserID: "snapshot-user", Title: "restaurada"}).Error; err != nil {
		t.Fatal(err)
	}
	next, after, err := p.SubscribeTimeline(f.ctx, session, "surface", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if after.Epoch == before.Epoch || after.WindowID == before.WindowID || len(after.Items) != 0 {
		t.Fatal("recriação herdou identidade/conteúdo antigo")
	}
	if _, err := sub.Refresh(f.ctx); !errors.Is(err, ErrTimelineSubscription) {
		t.Fatal("recriação reativou inscrição antiga", err)
	}
}

func TestConversationDeletionRollbackPreservesSubscription(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	sub, before, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "surface", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	failure := errors.New("rollback")
	if _, err := p.ApplyConversationDeletion(f.ctx, []string{f.conversation}, func() ([]string, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	after, err := sub.Refresh(f.ctx)
	if err != nil || after.Epoch != before.Epoch || len(after.Items) != len(before.Items) {
		t.Fatalf("rollback invalidou inscrição: %v", err)
	}
	called := false
	if _, err := p.ApplyConversationDeletion(context.Background(), []string{f.conversation}, func() ([]string, error) { called = true; return nil, nil }); err == nil || called {
		t.Fatal("writer sem usuário")
	}
}

func TestConversationDeletionBatchSerializesOppositeOrderAndFirstSubscriber(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	started, proceed := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, proceed)
	first := make(chan error, 1)
	go func() {
		_, err := p.ApplyConversationDeletion(f.ctx, []string{"other", f.conversation, "other"}, func() ([]string, error) {
			close(started)
			<-proceed
			return database.DeleteConversationsWithContext(f.ctx, []string{f.conversation})
		})
		first <- err
	}()
	<-started
	secondStarted := make(chan struct{})
	second := make(chan error, 1)
	go func() {
		_, err := p.ApplyConversationDeletion(f.ctx, []string{f.conversation, "other"}, func() ([]string, error) { close(secondStarted); return nil, nil })
		second <- err
	}()
	subscribed := make(chan error, 1)
	go func() {
		sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "s"}, "first", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
		if sub != nil {
			sub.Close()
		}
		subscribed <- err
	}()
	select {
	case <-secondStarted:
		t.Fatal("lote atravessou writer pendente")
	case err := <-subscribed:
		t.Fatal("inscrição atravessou exclusão pendente", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(proceed)
	for _, done := range []<-chan error{first, second} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("deadlock em lote")
		}
	}
	select {
	case err := <-subscribed:
		if err == nil {
			t.Fatal("inscrição retornou conteúdo excluído")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("inscrição bloqueada")
	}
}
