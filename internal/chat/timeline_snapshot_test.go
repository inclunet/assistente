package chat

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type snapshotFixture struct {
	db                                         *gorm.DB
	path                                       string
	ctx                                        context.Context
	conversation, question, answer, invocation string
	reader                                     *DBTimelineReader
}

func newSnapshotFixture(t testing.TB) snapshotFixture {
	t.Helper()
	restore := database.SetDB(database.DB())
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := database.InitPath(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(); restore() })
	db := database.DB()
	if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "snapshot-user"}, Username: "snapshot-user"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "snapshot-user")
	conv, err := database.CreateConversationWithContext(ctx, "Snapshot", "")
	if err != nil {
		t.Fatal(err)
	}
	question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta antiga"})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, TurnID: &question.ID, Role: "assistant", Content: "resposta antiga"})
	if err != nil {
		t.Fatal(err)
	}
	repo := database.NewChatTurnRepository(db)
	attempt, err := repo.Begin(ctx, conv.ID, question.ID, answer.ID, database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, conv.ID, question.ID, attempt.ExecutionID, database.TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "snapshot-tool", DisplayName: "Snapshot tool", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	invocation := database.ToolInvocation{UserID: "snapshot-user", ToolCatalogID: catalog.ID, OriginType: "chat", ConversationID: &conv.ID, TurnID: &question.ID, ToolCallID: "call", Status: "succeeded", OutputPreview: "resultado antigo"}
	if err := db.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	return snapshotFixture{db: db, path: path, ctx: ctx, conversation: conv.ID, question: question.ID, answer: answer.ID, invocation: invocation.ID, reader: NewDBTimelineReader(db)}
}

func (f snapshotFixture) window(t testing.TB) *TimelineWindowSnapshot {
	t.Helper()
	snapshot, err := f.reader.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func responseNode(t testing.TB, nodes []MessageNode, turnID string) MessageNode {
	t.Helper()
	for _, node := range nodes {
		if node.ItemID == "turn:"+turnID {
			return node
		}
	}
	t.Fatalf("resposta ausente: %+v", nodes)
	return MessageNode{}
}

func invocationPreview(t testing.TB, node MessageNode) string {
	t.Helper()
	for _, segment := range node.Message.TurnSegments {
		for _, call := range segment.ToolCalls {
			return call.OutputPreview
		}
	}
	t.Fatalf("invocação ausente: %+v", node)
	return ""
}

func TestTimelineSnapshotReadsOneCommitAcrossWindowAndTurn(t *testing.T) {
	for _, kind := range []string{"window", "turn"} {
		t.Run(kind, func(t *testing.T) {
			f := newSnapshotFixture(t)
			before := f.window(t)
			writer, err := gorm.Open(sqlite.Open(f.path), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := writer.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := writer.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
				t.Fatal(err)
			}
			var once sync.Once
			var writeErr error
			// O commit externo ocorre depois de fixar a versão e antes de ler o conteúdo.
			hook := "test:timeline-snapshot"
			if err := f.db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
				if !strings.Contains(tx.Statement.SQL.String(), "chat_timeline_versions") {
					return
				}
				once.Do(func() {
					writeErr = writer.Transaction(func(other *gorm.DB) error {
						if err := other.Exec("UPDATE chat_messages SET content='resposta nova' WHERE id=?", f.answer).Error; err != nil {
							return err
						}
						if err := other.Exec("UPDATE tool_invocations SET output_preview='resultado novo' WHERE id=?", f.invocation).Error; err != nil {
							return err
						}
						if err := other.Exec("UPDATE chat_turns SET state='interrupted' WHERE user_message_id=?", f.question).Error; err != nil {
							return err
						}
						return other.Create(&database.ChatMessage{ConversationID: f.conversation, ParentID: &f.answer, Role: "user", Content: "filho novo"}).Error
					})
				})
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.db.Callback().Query().Remove(hook) })
			var version database.ChatTimelineVersion
			var turn *database.ChatTurn
			var node MessageNode
			var content, preview string
			if kind == "window" {
				snapshot := f.window(t)
				version, turn = snapshot.Version, snapshot.Turns[f.question]
				node = responseNode(t, snapshot.Window.Nodes, f.question)
				content, preview = node.Message.Content, invocationPreview(t, node)
			} else {
				snapshot, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
				if err != nil {
					t.Fatal(err)
				}
				version, turn = snapshot.Version, snapshot.Turn
				if snapshot.Response == nil || snapshot.Response.Representative == nil {
					t.Fatal("resposta ausente")
				}
				content = snapshot.Response.Representative.Content
				for _, segment := range snapshot.Response.Segments {
					for _, call := range segment.Invocations {
						if call.InvocationID == f.invocation {
							preview = call.OutputPreview
						}
					}
				}
			}
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			if version != before.Version || content != "resposta antiga" || preview != "resultado antigo" || node.ChildCount != 0 || turn == nil || turn.State != database.TurnCompleted {
				t.Fatalf("snapshot híbrido: version=%+v turn=%+v node=%+v", version, turn, node)
			}
			after := f.window(t)
			current := responseNode(t, after.Window.Nodes, f.question)
			if after.Version == before.Version || current.Message.Content != "resposta nova" || invocationPreview(t, current) != "resultado novo" || current.ChildCount != 1 || after.Turns[f.question].State != database.TurnInterrupted {
				t.Fatalf("snapshot seguinte não avançou junto: %+v %+v", after.Version, current)
			}
		})
	}
}

func TestTimelineSnapshotPreservesNullableAndLegacyTurnMetadata(t *testing.T) {
	f := newSnapshotFixture(t)
	// ON DELETE SET NULL não promove um intermediário a representante persistido.
	if err := f.db.Delete(&database.ChatMessage{}, "id=?", f.answer).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil || snapshot.Turn == nil || snapshot.Turn.ResponseMessageID != nil {
		t.Fatalf("nullable perdido: %+v %v", snapshot, err)
	}
	window := f.window(t)
	if turn := window.Turns[f.question]; turn == nil || turn.ResponseMessageID != nil {
		t.Fatalf("janela perdeu turno sem resposta: %+v", turn)
	}
	legacy, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "legado"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = f.reader.ReadTurn(f.ctx, f.conversation, legacy.ID)
	if err != nil || snapshot.Turn != nil || snapshot.Response != nil {
		t.Fatalf("legado confundido com metadado: %+v %v", snapshot, err)
	}
	if _, err = f.reader.ReadTurn(f.ctx, f.conversation, "inexistente"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("pergunta ausente: %v", err)
	}
}

func TestTimelineSnapshotRejectsMissingSchemaOwnershipAndCancellation(t *testing.T) {
	f := newSnapshotFixture(t)
	for _, ctx := range []context.Context{context.Background(), database.WithUserID(context.Background(), "outro-user")} {
		if _, err := f.reader.ReadTurn(ctx, f.conversation, f.question); err == nil {
			t.Fatal("leitura sem owner")
		}
	}
	other, err := database.CreateConversationWithContext(f.ctx, "Outra", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadTurn(f.ctx, other.ID, f.question); err == nil {
		t.Fatal("pergunta de outra conversa")
	}
	if _, err := f.reader.ReadTurn(f.ctx, f.conversation, f.answer); err == nil {
		t.Fatal("assistant não é pergunta")
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.reader.ReadWindow(ctx, database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento ignorado: %v", err)
	}
	if err := f.db.Exec("DELETE FROM chat_timeline_versions WHERE conversation_id=?", f.conversation).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("metadado ausente virou versão zero: %v", err)
	}
}

func seedLargeSnapshotTurn(t testing.TB, f snapshotFixture) string {
	t.Helper()
	// Uma conversa não relacionada detecta leituras que escapem do turno.
	other, err := database.CreateConversationWithContext(f.ctx, "Fora do turno", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: other.ID, Role: "assistant", Content: "não incluir"}); err != nil {
		t.Fatal(err)
	}
	messages := make([]database.ChatMessage, 1005)
	base := time.Now().Add(-time.Hour)
	for i := range messages {
		messages[i] = database.ChatMessage{UUIDModel: database.UUIDModel{ID: fmt.Sprintf("segment-%04d", i), CreatedAt: base.Add(time.Duration(i) * time.Millisecond)}, ConversationID: f.conversation, TurnID: &f.question, Role: "assistant", Content: fmt.Sprintf("segmento %04d ", i) + strings.Repeat("x", 128)}
	}
	if err := f.db.CreateInBatches(messages, 100).Error; err != nil {
		t.Fatal(err)
	}
	final, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, TurnID: &f.question, Role: "assistant", Content: "cauda depois de mil"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id=?", final.ID, f.question).Error; err != nil {
		t.Fatal(err)
	}
	return final.ID
}

func TestTimelineSnapshotTurnBeyondThousandMatchesWindow(t *testing.T) {
	f := newSnapshotFixture(t)
	finalID := seedLargeSnapshotTurn(t, f)
	snapshot, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	node := snapshot.Response.TerminalPatch()
	if node == nil {
		t.Fatal("patch terminal ausente")
	}
	if node.Message.ID != finalID || node.Message.Content != "cauda depois de mil" || len(node.Message.TurnSegments) < 1007 {
		t.Fatalf("turno truncado: id=%s segmentos=%d", node.Message.ID, len(node.Message.TurnSegments))
	}
	window := f.window(t)
	fromWindow := responseNode(t, window.Window.Nodes, f.question)
	if fromWindow.Message.ID != node.Message.ID || len(fromWindow.Message.TurnSegments) != len(node.Message.TurnSegments) || window.Version != snapshot.Version {
		t.Fatal("janela e terminal divergentes")
	}
	for _, segment := range node.Message.TurnSegments {
		if strings.Contains(segment.Content, "não incluir") {
			t.Fatal("leu outra conversa")
		}
	}
}

func BenchmarkTimelineTurnSnapshotLarge(b *testing.B) {
	f := newSnapshotFixture(b)
	seedLargeSnapshotTurn(b, f)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question); err != nil {
			b.Fatal(err)
		}
	}
}

func TestTimelineSnapshotPreservesThreadScopeAndCurrentRetry(t *testing.T) {
	f := newSnapshotFixture(t)
	question, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "pergunta na thread"})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, TurnID: &question.ID, Role: "assistant", Content: "resposta na thread"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, TurnID: &question.ID, Role: "assistant", Content: "fora da thread"}); err != nil {
		t.Fatal(err)
	}
	repo := database.NewChatTurnRepository(f.db)
	attempt, err := repo.Begin(f.ctx, f.conversation, question.ID, answer.ID, database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(f.ctx, f.conversation, question.ID, attempt.ExecutionID, database.TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	before, err := f.reader.ReadTurn(f.ctx, f.conversation, question.ID)
	if err != nil {
		t.Fatal(err)
	}
	node := before.Response.TerminalPatch()
	if node == nil {
		t.Fatal("patch de thread ausente")
	}
	if node.Message.ID != answer.ID || node.Message.ParentID == nil || *node.Message.ParentID != f.question {
		t.Fatalf("misturou escopos: %+v", node)
	}
	window, err := f.reader.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, ParentID: &f.question, Limit: 20, Anchor: "end", Direction: "before"})
	if err != nil {
		t.Fatal(err)
	}
	if windowNode := responseNode(t, window.Window.Nodes, question.ID); windowNode.Level != 1 || windowNode.Message.ID != answer.ID {
		t.Fatal("thread divergiu entre janela/turno")
	}
	if _, err := f.reader.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, ParentID: &question.ID, Limit: 20}); err == nil {
		t.Fatal("aceitou thread aninhada")
	}
	// Snapshot é do estado persistido atual; não é barreira para evento de execução
	// antiga. A precedência por executionId pertence ao futuro projetor incremental.
	retry, err := repo.Begin(f.ctx, f.conversation, question.ID, answer.ID, database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishResponse(f.ctx, f.conversation, question.ID, retry.ExecutionID, database.TurnCompleted, "completed", answer.ID, &database.MessageOptions{Content: "resposta do retry"}); err != nil {
		t.Fatal(err)
	}
	after, err := f.reader.ReadTurn(f.ctx, f.conversation, question.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version == before.Version || after.Response.TerminalPatch().Message.Content != "resposta do retry" || node.Message.Content != "resposta na thread" {
		t.Fatal("snapshot não isolou gerações de leitura")
	}
}

func TestTimelineSnapshotDoesNotHideLedgerReadFailure(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Exec("DROP TABLE tool_invocations").Error; err != nil {
		t.Fatal(err)
	}
	if result, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question); err == nil || result != nil {
		t.Fatalf("falha virou turno sem ferramentas: %+v %v", result, err)
	}
	if result, err := f.reader.ReadWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Limit: 20}); err == nil || result != nil {
		t.Fatalf("falha virou janela sem ferramentas: %+v %v", result, err)
	}
}
