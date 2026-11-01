package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
)

func TestMessageCreationReconcilesAllControllerEntries(t *testing.T) {
	for _, mode := range []string{"create", "simple", "media", "tokens", "tokens-media", "child"} {
		t.Run(mode, func(t *testing.T) {
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "creation.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
			db := database.DB()
			if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			conv, err := database.CreateConversationWithContext(ctx, "criação", "")
			if err != nil {
				t.Fatal(err)
			}
			parent, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pai"})
			if err != nil {
				t.Fatal(err)
			}
			query := database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20}
			if mode == "child" {
				query.ParentID = &parent.ID
			}
			r := &commandTimelineReader{DBTimelineReader: chat.NewDBTimelineReader(db)}
			p := chat.NewLiveProjection(r)
			sub, _, err := p.SubscribeTimeline(ctx, commandTimelineSession{ctx}, "creation", query, r)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			c := NewConversationsController(ConversationsControllerConfig{MsgRepo: chat.NewDBMessageStore(), MutateTimeline: p.ApplyStorageMutation})
			var message *database.ChatMessage
			switch mode {
			case "create":
				message, err = c.CreateMessage(ctx, conv.ID, "assistant", "conteúdo")
			case "simple":
				message, err = c.AddMessage(ctx, conv.ID, "assistant", "conteúdo")
			case "media":
				message, err = c.AddMessageWithMedia(ctx, conv.ID, "assistant", "conteúdo", "[]")
			case "tokens":
				message, err = c.AddMessageWithTokens(ctx, conv.ID, "assistant", "conteúdo", 3, 5, 8, "modelo")
			case "tokens-media":
				message, err = c.AddMessageWithTokensAndMedia(ctx, conv.ID, "assistant", "conteúdo", "[]", 3, 5, 8, "modelo")
			case "child":
				message, err = c.AddChildMessage(ctx, conv.ID, parent.ID, "assistant", "conteúdo", "modelo")
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.readErr != nil || r.reads != 1 || len(r.selected.Items) != 1 || r.selected.Items[0].ItemID != "message:"+message.ID {
				t.Fatal("criação não reconciliada antes do retorno", r.readErr)
			}
			projected := r.selected.Items[0]
			if projected.Representative == nil || projected.Representative.Content != "conteúdo" || projected.Representative.Media != "" {
				t.Fatal("projeção não preservou conteúdo ou carregou mídia")
			}
			if mode == "tokens" || mode == "tokens-media" {
				if message.PromptTokens != 3 || message.CompletionTokens != 5 || message.TotalTokens != 8 || projected.Representative.Model != "modelo" {
					t.Fatal("metadados alterados")
				}
			}
			if mode == "media" || mode == "tokens-media" {
				if message.Media != "[]" {
					t.Fatal("mídia persistida alterada")
				}
			}
			if mode == "child" {
				if projected.ScopeParentID == nil || *projected.ScopeParentID != parent.ID {
					t.Fatal("escopo filho perdido")
				}
			}
			snapshot, err := sub.Refresh(ctx)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range snapshot.Items {
				if item.ItemID == "message:"+message.ID {
					found = true
				}
			}
			if !found {
				t.Fatal("inscrição não recuperou item criado")
			}
			// A mesma entrada permanece fechada para usuário sem ownership.
			if _, err := c.AddMessage(database.WithUserID(context.Background(), "other"), conv.ID, "assistant", "indevida"); err == nil {
				t.Fatal("criação de outro usuário aceita")
			}
			var count int64
			if err := db.Model(&database.ChatMessage{}).Where("conversation_id=? AND content=?", conv.ID, "indevida").Count(&count).Error; err != nil || count != 0 {
				t.Fatal("recusa persistiu mensagem", err)
			}
		})
	}
}

type rejectedCreationStore struct {
	*chat.DBMessageStore
	failure error
}

func (r rejectedCreationStore) CreateMessage(context.Context, database.MessageOptions) (*database.ChatMessage, error) {
	return nil, r.failure
}

func TestMessageCreationPreservesWriterFailureAndMissingRepository(t *testing.T) {
	failure := errors.New("gravação recusada")
	called := false
	c := NewConversationsController(ConversationsControllerConfig{
		MsgRepo: rejectedCreationStore{chat.NewDBMessageStore(), failure},
		MutateTimeline: func(_ context.Context, conversationID string, known []string, write func() ([]string, error)) error {
			called = true
			if conversationID != "conversation" || known == nil || len(known) != 0 {
				t.Fatal("criação pediu reconciliação global")
			}
			ids, err := write()
			if len(ids) != 0 {
				t.Fatal("falha inventou identidade")
			}
			return err
		},
	})
	if message, err := c.AddMessage(context.Background(), "conversation", "user", "texto"); message != nil || !errors.Is(err, failure) || !called {
		t.Fatal("erro original perdido", err)
	}
	c.msgRepo = nil
	called = false
	if _, err := c.CreateMessage(context.Background(), "conversation", "user", "texto"); err == nil || called {
		t.Fatal("repositório ausente alcançou coordenador")
	}
}
