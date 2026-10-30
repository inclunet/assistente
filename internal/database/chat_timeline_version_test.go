package database

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func newTimelineVersionDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, database, "conv", "turn", "answer")
	if err := migrateChatTurns(database); err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTurnLeases(database); err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTimelineVersions(database); err != nil {
		t.Fatal(err)
	}
	return database
}

func timelineVersion(t *testing.T, database *gorm.DB, id string) ChatTimelineVersion {
	t.Helper()
	user := "user"
	if id == "other" {
		user = "other-user"
	}
	version, err := ReadChatTimelineVersion(WithUserID(context.Background(), user), database, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(version.Incarnation) != 32 {
		t.Fatalf("incarnation inválida: %+v", version)
	}
	return version
}

func TestChatTimelineVersionTracksDirectWritersAndRollback(t *testing.T) {
	database := newTimelineVersionDB(t)
	baseline := timelineVersion(t, database, "conv")
	other := timelineVersion(t, database, "other")
	// SQL direto cobre speech/transcrição, pin e importadores que não passam pelo repo.
	for _, statement := range []string{
		"UPDATE chat_messages SET content='transcrição', audio='audio', media='media' WHERE id='answer'",
		"UPDATE chat_messages SET reasoning='raciocínio' WHERE id='answer'",
		"UPDATE chat_messages SET model='modelo', total_tokens=123 WHERE id='answer'",
	} {
		before := timelineVersion(t, database, "conv")
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
		after := timelineVersion(t, database, "conv")
		if after.Incarnation != before.Incarnation || after.Revision <= before.Revision {
			t.Fatalf("escrita não invalidou: %s", statement)
		}
	}
	before := timelineVersion(t, database, "conv")
	if err := database.Exec("UPDATE chat_messages SET content=content, audio=audio WHERE id='answer'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got != before {
		t.Fatalf("no-op invalidou: %+v -> %+v", before, got)
	}
	sentinel := errors.New("rollback intencional")
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE chat_messages SET content='não confirmado' WHERE conversation_id='conv'").Error; err != nil {
			return err
		}
		if got := timelineVersion(t, tx, "conv"); got.Revision <= before.Revision {
			t.Fatal("alteração invisível na própria transação")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got != before {
		t.Fatalf("rollback mudou versão: %+v", got)
	}
	if got := timelineVersion(t, database, "other"); got != other {
		t.Fatalf("invalidou outra conversa: %+v", got)
	}
	if before.Revision <= baseline.Revision {
		t.Fatal("não houve avanço")
	}
}

func TestChatTimelineVersionTracksLedgerAndMoves(t *testing.T) {
	database := newTimelineVersionDB(t)
	if err := database.Create(&ToolCatalog{UUIDModel: UUIDModel{ID: "tool"}, Name: "tool"}).Error; err != nil {
		t.Fatal(err)
	}
	conv := "conv"
	invocation := ToolInvocation{UUIDModel: UUIDModel{ID: "inv"}, UserID: "user", ToolCatalogID: "tool", OriginType: "chat", ConversationID: &conv, Status: "running"}
	before := timelineVersion(t, database, "conv")
	if err := database.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	if after := timelineVersion(t, database, "conv"); after.Revision != before.Revision+1 {
		t.Fatalf("insert ledger: %+v", after)
	}
	before = timelineVersion(t, database, "conv")
	other := timelineVersion(t, database, "other")
	if err := database.Exec("UPDATE tool_invocations SET conversation_id='other', user_id='other-user', output='resultado', status='succeeded' WHERE id='inv'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision != before.Revision+1 {
		t.Fatalf("origem: %+v", got)
	}
	if got := timelineVersion(t, database, "other"); got.Revision != other.Revision+1 {
		t.Fatalf("destino: %+v", got)
	}
	other = timelineVersion(t, database, "other")
	if err := database.Exec("UPDATE tool_invocations SET conversation_id=NULL WHERE id='inv'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "other"); got.Revision != other.Revision+1 {
		t.Fatalf("NULL não invalidou origem: %+v", got)
	}
	before = timelineVersion(t, database, "conv")
	if err := database.Exec("UPDATE tool_invocations SET conversation_id='conv', origin_type='job', user_id='user' WHERE id='inv'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got != before {
		t.Fatalf("job não pertence à timeline: %+v", got)
	}
	if err := database.Exec("UPDATE tool_invocations SET origin_type='chat' WHERE id='inv'").Error; err != nil {
		t.Fatal(err)
	}
	before = timelineVersion(t, database, "conv")
	if err := database.Delete(&ToolInvocation{}, "id='inv'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision != before.Revision+1 {
		t.Fatalf("delete ledger: %+v", got)
	}
	// Mensagem independente pode mudar de conversa sem passar pelos writers de chat.
	if err := database.Exec("INSERT INTO chat_messages(id,conversation_id,role,content) VALUES('moved','conv','assistant','texto')").Error; err != nil {
		t.Fatal(err)
	}
	before, other = timelineVersion(t, database, "conv"), timelineVersion(t, database, "other")
	if err := database.Exec("UPDATE chat_messages SET conversation_id='other' WHERE id='moved'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision != before.Revision+1 {
		t.Fatal("mensagem não invalidou origem")
	}
	if got := timelineVersion(t, database, "other"); got.Revision != other.Revision+1 {
		t.Fatal("mensagem não invalidou destino")
	}
}

func TestChatTimelineVersionTracksLifecycleAndCascade(t *testing.T) {
	database := newTimelineVersionDB(t)
	ctx := WithUserID(context.Background(), "user")
	repo := NewChatTurnRepository(database)
	before := timelineVersion(t, database, "conv")
	attempt, err := repo.Begin(ctx, "conv", "turn", "answer", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision <= before.Revision {
		t.Fatal("admissão não invalidou")
	}
	before = timelineVersion(t, database, "conv")
	// Atualização isolada de tentativa também participa, sem depender do update do turno.
	if err := database.Exec("UPDATE chat_turn_attempts SET lease_path='controle' WHERE execution_id=?", attempt.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision != before.Revision+1 {
		t.Fatal("tentativa não invalidou")
	}
	if err := repo.FinishResponse(ctx, "conv", "turn", attempt.ExecutionID, TurnCompleted, "completed", "answer", &MessageOptions{Content: "final"}); err != nil {
		t.Fatal(err)
	}
	before = timelineVersion(t, database, "conv")
	if err := database.Exec("DELETE FROM chat_messages WHERE id='answer'").Error; err != nil {
		t.Fatal(err)
	}
	turn, err := repo.Get(ctx, "conv", "turn")
	if err != nil || turn.ResponseMessageID != nil {
		t.Fatalf("SET NULL: %+v %v", turn, err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision <= before.Revision {
		t.Fatal("exclusão de resposta não invalidou")
	}
	incarnation := before.Incarnation
	// chat_messages usa FK restritiva para conversa; como no repositório, remover
	// mensagens primeiro. A pergunta cascateia turno e tentativas.
	if err := database.Exec("DELETE FROM chat_messages WHERE conversation_id='conv'").Error; err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got.Revision <= before.Revision {
		t.Fatal("cascata do turno não invalidou")
	}
	if err := database.Exec("DELETE FROM conversations WHERE id='conv'").Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Table("chat_timeline_versions").Where("conversation_id='conv'").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("cascata recriou versão: %d %v", count, err)
	}
	if _, err := ReadChatTimelineVersion(ctx, database, "conv"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("conversa excluída: %v", err)
	}
	if err := database.Create(&Conversation{UUIDModel: UUIDModel{ID: "conv"}, UserID: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	got := timelineVersion(t, database, "conv")
	if got.Incarnation == incarnation || got.Revision != 0 {
		t.Fatalf("recriação reutilizou identidade: %+v", got)
	}
}

func TestChatTimelineVersionBatchAuthorizationAndReinstall(t *testing.T) {
	database := newTimelineVersionDB(t)
	ctx := WithUserID(context.Background(), "user")
	before := timelineVersion(t, database, "conv")
	if err := database.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < 300; i++ {
			if err := tx.Exec("INSERT INTO chat_messages(id,conversation_id,role,content) VALUES(?,'conv','assistant','importado')", fmt.Sprintf("import-%d", i)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after := timelineVersion(t, database, "conv")
	if after.Revision != before.Revision+300 {
		t.Fatalf("lote: %+v -> %+v", before, after)
	}
	if err := migrateChatTimelineVersions(database); err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, database, "conv"); got != after {
		t.Fatalf("reinstalação perdeu versão: %+v", got)
	}
	if _, err := ReadChatTimelineVersion(ctx, database, "other"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("outro owner: %v", err)
	}
	if _, err := ReadChatTimelineVersion(context.Background(), database, "conv"); err == nil {
		t.Fatal("sem sessão")
	}
	if err := database.Exec("DELETE FROM chat_timeline_versions WHERE conversation_id='conv'").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadChatTimelineVersion(ctx, database, "conv"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("metadado ausente: %v", err)
	}
}

func TestChatTimelineVersionMigrationWaitsForTurns(t *testing.T) {
	database := newChatTurnTestDB(t, false)
	if err := ensureSchemaMigrationsTable(database); err != nil {
		t.Fatal(err)
	}
	var target migration
	for _, m := range schemaMigrations {
		if m.Version == 43 {
			target = m
		}
	}
	if target.Run == nil {
		t.Fatal("migração não registrada")
	}
	if err := target.Run(database); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("não aguardou v42: %v", err)
	}
	if database.Migrator().HasTable("chat_timeline_versions") {
		t.Fatal("criou schema prematuramente")
	}
	// Falha depois do CREATE deve reverter tabela, backfill e todos os triggers.
	if err := migrateChatTimelineVersions(database); err == nil {
		t.Fatal("schema de turnos ausente deveria falhar")
	}
	if database.Migrator().HasTable("chat_timeline_versions") {
		t.Fatal("migração parcial sobreviveu")
	}
}

func TestChatTimelineVersionSharesReadSnapshotWithContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	reader := openMessageRevisionTestFileDB(t, path)
	if err := reader.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := reader.AutoMigrate(&User{}, &Conversation{}, &ChatMessage{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	if err := createChatTurnSchema(reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Create(&User{UUIDModel: UUIDModel{ID: "user"}, Username: "snapshot-user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := reader.Create(&Conversation{UUIDModel: UUIDModel{ID: "conv"}, UserID: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	seedChatTurnTestMessages(t, reader, "conv", "turn", "answer")
	if err := migrateChatTimelineVersions(reader); err != nil {
		t.Fatal(err)
	}
	writer := openMessageRevisionTestFileDB(t, path)
	before := timelineVersion(t, reader, "conv")
	if err := reader.Transaction(func(tx *gorm.DB) error {
		if got := timelineVersion(t, tx, "conv"); got != before {
			t.Fatal("snapshot inicial")
		}
		if err := writer.Exec("UPDATE chat_messages SET content='outro commit' WHERE id='answer'").Error; err != nil {
			return err
		}
		var message ChatMessage
		if err := tx.First(&message, "id='answer'").Error; err != nil {
			return err
		}
		if message.Content != "antes" {
			t.Fatalf("misturou conteúdo de outro commit: %q", message.Content)
		}
		if got := timelineVersion(t, tx, "conv"); got != before {
			t.Fatal("versão não pertence ao snapshot")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := timelineVersion(t, reader, "conv"); got.Revision <= before.Revision {
		t.Fatal("próximo snapshot não detectou commit externo")
	}
}

func TestChatTimelineVersionCatalogUsesReferenceIndexes(t *testing.T) {
	database := newTimelineVersionDB(t)
	predicate := strings.ReplaceAll(timelineCatalogScope("catalog"), "catalog.id", "'tool'")
	var plan []struct{ Detail string }
	if err := database.Raw("EXPLAIN QUERY PLAN UPDATE chat_timeline_versions SET revision=revision+1 WHERE " + predicate).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, row := range plan {
		if strings.Contains(row.Detail, "idx_timeline_catalog_chat") {
			indexed = true
		}
		for _, table := range []string{"c", "i", "m"} {
			if row.Detail == "SCAN "+table || strings.HasPrefix(row.Detail, "SCAN "+table+" ") {
				t.Fatalf("varredura não limitada: %+v", plan)
			}
		}
	}
	if !indexed {
		t.Fatalf("índice de referências ausente: %+v", plan)
	}
}
