package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/portability"
	"gorm.io/gorm"
)

func TestImportCoordinatesReplacementAndPartialRollbackBeforeRestore(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	sub, before, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "import"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	now := time.Now().UTC()
	file := portability.ExportFile{Version: portability.ExportVersion, ExportedAt: now, Resources: portability.ExportResources{Conversations: []portability.ConversationExport{
		{ID: f.conversation, Title: "Substituída", CreatedAt: now, Messages: []portability.MessageExport{{ID: f.question, Role: "user", Content: "pergunta importada", CreatedAt: now}}},
		// A segunda transação remove a pergunta antes de encontrar o pai inválido.
		// Seu rollback precisa preservar o commit e a projeção da primeira.
		{ID: f.conversation, Title: "Inválida", CreatedAt: now, Messages: []portability.MessageExport{{ID: "invalid-answer", ParentID: "missing", Role: "assistant", Content: "não confirmar", CreatedAt: now}}},
	}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	coordinated, restored := 0, false
	result, err := portability.ImportConversationsWithHooks(f.ctx, string(raw), nil, "", nil, portability.ConversationImportHooks{
		ImportTimeline: func(ctx context.Context, id string, write func() error) error {
			coordinated++
			if id != f.conversation {
				t.Fatal("substituição não seleciona todos os itens antigos")
			}
			return p.ApplyConversationImport(ctx, id, write)
		},
		Restore: func(ids []string) {
			restored = true
			if !reflect.DeepEqual(ids, []string{f.conversation}) {
				t.Fatal("restore incluiu transação revertida", ids)
			}
			// Ler a base já publicada antes de Refresh: Refresh não deve mascarar
			// ausência de reconciliação no caminho de escrita.
			c := sub.conversation
			c.mu.Lock()
			defer c.mu.Unlock()
			item, ok := c.publication.items["message:"+f.question]
			if !ok || canonicalText(item) != "pergunta importada" {
				t.Fatal("restore precedeu publicação", item)
			}
			// A pergunta importada conserva seu slot de resposta canônico, mesmo
			// sem fontes. Nenhum texto, ferramenta ou ID físico anterior sobrevive.
			response, exists := c.publication.items["turn:"+f.question]
			if !exists || len(response.Segments) != 0 || len(response.MessageIDs) != 0 || response.RepresentativeMessageID != nil || response.State != "historical_unknown" || response.Metadata.Origin != "imported" {
				t.Fatalf("slot importado reteve conteúdo ou estado antigo: %+v", response)
			}
		},
	})
	if err != nil || result == nil || result.Imported != 1 || result.Failed != 1 || coordinated != 2 || !restored {
		t.Fatalf("import: %+v, %v, coordenadas=%d restore=%v", result, err, coordinated, restored)
	}
	after, err := sub.Refresh(f.ctx)
	if err != nil || len(after.Items) != 2 || canonicalText(after.Items[0]) != "pergunta importada" || canonicalText(after.Items[1]) != "" || len(after.Items[1].Segments) != 0 {
		t.Fatal("snapshot não corresponde ao commit", after, err)
	}
	if after.WindowID != before.WindowID {
		t.Fatal("substituição trocou identidade da janela")
	}
}

func TestImportSameExecutionSeparatesVolatileBuffersOnlyAfterCommit(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[rollback], func(t *testing.T) {
			f, p, e, _ := automaticFixture(t)
			repo := database.NewChatTurnRepository(f.db)
			responseID, err := e.AssignResponse(func() (string, error) {
				return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := database.UpdateMessageTextWithContext(f.ctx, responseID, "conteúdo do backup"); err != nil {
				t.Fatal(err)
			}
			stream := e.StartStream(0)
			stream.Append("parcial volátil anterior")
			if err := e.CommitTerminal(database.TurnInterrupted, "cancelled", func() (database.ChatTimelineVersion, error) {
				return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnInterrupted, "cancelled", "", nil)
			}); err != nil {
				t.Fatal(err)
			}
			before, _ := publicationItem(t, e)
			if canonicalText(before) != "parcial volátil anterior" {
				t.Fatal("fixture sem conteúdo volátil", before)
			}
			raw, err := portability.ExportConversationsWithContext(f.ctx, []string{f.conversation}, nil, portability.ExportRequest{}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if rollback {
				if err := f.db.Callback().Delete().Before("gorm:delete").Register("import_rollback", func(tx *gorm.DB) {
					if tx.Statement.Table == "chat_messages" {
						_ = tx.AddError(errors.New("rollback"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			result, err := portability.ImportConversationsWithHooks(f.ctx, raw, nil, "", nil, portability.ConversationImportHooks{ImportTimeline: p.ApplyConversationImport})
			if err != nil || result == nil || result.Failed != map[bool]int{false: 0, true: 1}[rollback] {
				t.Fatal(result, err)
			}
			after, _ := publicationItem(t, e)
			want := "conteúdo do backup"
			if rollback {
				want = "parcial volátil anterior"
			}
			if canonicalText(after) != want {
				t.Fatalf("conteúdo após import=%q, esperado=%q", canonicalText(after), want)
			}
			recovery, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
			if err != nil || len(recovery.Retained) != 1 || recovery.Retained[0].ExecutionID != e.executionID || recovery.Retained[0].Detached == rollback || testLiveSegment(recovery.Retained[0].Streams[0], "text", 0).Text != "parcial volátil anterior" {
				t.Fatal("bytes/proveniência perdidos", recovery, err)
			}
			if stream.Append("tardio") {
				t.Fatal("capability terminal aceitou delta após import")
			}
		})
	}
}

func TestImportCoordinatorRefusalPreservesHistory(t *testing.T) {
	f := newSnapshotFixture(t)
	file := portability.ExportFile{Version: portability.ExportVersion, Resources: portability.ExportResources{Conversations: []portability.ConversationExport{{ID: f.conversation, Title: "Recusada", Messages: []portability.MessageExport{{ID: f.question, Role: "user", Content: "não escrever"}}}}}}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	result, err := portability.ImportConversationsWithHooks(f.ctx, string(raw), nil, "", nil, portability.ConversationImportHooks{
		ImportTimeline: func(context.Context, string, func() error) error { return errors.New("recusada") },
		Restore:        func([]string) { t.Fatal("restore sem commit") },
	})
	if err != nil || result == nil || result.Imported != 0 || result.Failed != 1 {
		t.Fatal(result, err)
	}
	var message database.ChatMessage
	if err := f.db.First(&message, "id=?", f.question).Error; err != nil || message.Content != "pergunta antiga" {
		t.Fatal(message, err)
	}
}
