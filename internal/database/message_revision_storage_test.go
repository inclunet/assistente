package database

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const messageRevisionTestFixedTime = "2026-01-02T03:04:05.000000000Z"

func freezeMessageRevisionTestClock(t *testing.T) {
	t.Helper()
	if db == nil || db.Config == nil {
		t.Fatal("database de teste não inicializado")
	}
	previous := db.Config.NowFunc
	fixed, err := time.Parse(time.RFC3339Nano, messageRevisionTestFixedTime)
	if err != nil {
		t.Fatal(err)
	}
	db.Config.NowFunc = func() time.Time { return fixed }
	t.Cleanup(func() { db.Config.NowFunc = previous })
}

func readMessageRevisionTest(t *testing.T, database *gorm.DB, messageID string) (string, bool) {
	t.Helper()
	var row struct {
		Revision string
	}
	err := database.Table("chat_message_revisions").Where("message_id = ?", messageID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	if err != nil {
		t.Fatalf("ler revisão de %s: %v", messageID, err)
	}
	return row.Revision, true
}

func assertMessageRevisionTestToken(t *testing.T, revision string) {
	t.Helper()
	if len(revision) != 64 {
		t.Fatalf("nonce %q tem tamanho %d, esperado 64", revision, len(revision))
	}
	if _, err := hex.DecodeString(revision); err != nil {
		t.Fatalf("nonce %q não é hexadecimal: %v", revision, err)
	}
}

func openMessageRevisionTestFileDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	fixed, err := time.Parse(time.RFC3339Nano, messageRevisionTestFixedTime)
	if err != nil {
		t.Fatal(err)
	}
	database, err := gorm.Open(sqlite.Open(sqliteDSN(path)), &gorm.Config{
		NowFunc: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatalf("abrir banco SQLite de teste: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("obter handle SQL de teste: %v", err)
	}
	configureSQLitePool(sqlDB)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return database
}

func TestMessageRevisionStorageFrozenClockDetectsABA(t *testing.T) {
	t.Run("pin", func(t *testing.T) {
		setupTestDB(t)
		freezeMessageRevisionTestClock(t)

		conversationID := createTestConversation(t, "pin ABA")
		messageID := createTestMessage(t, conversationID, "user", "same content")
		before, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ToggleMessagePinWithContext(testCtx(), messageID); err != nil {
			t.Fatal(err)
		}
		if _, err := ToggleMessagePinWithContext(testCtx(), messageID); err != nil {
			t.Fatal(err)
		}
		after, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false)
		if err != nil {
			t.Fatal(err)
		}
		if before == after {
			t.Fatal("pin ABA não renovou o snapshot com relógio congelado")
		}

		err = WithConversationLifecycle(testCtx(), func() error {
			_, err := MutateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), conversationID, messageID, before, false)
			return err
		})
		if !errors.Is(err, ErrMessageContentChanged) {
			t.Fatalf("commit antigo de pin foi aceito: %v", err)
		}
	})

	t.Run("content-and-clear", func(t *testing.T) {
		setupTestDB(t)
		freezeMessageRevisionTestClock(t)

		conversationID := createTestConversation(t, "content ABA")
		messageID := createTestMessage(t, conversationID, "user", "A")
		messageBefore, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false)
		if err != nil {
			t.Fatal(err)
		}
		conversationBefore, err := ConversationContentSnapshotWithContext(testCtx(), conversationID)
		if err != nil {
			t.Fatal(err)
		}
		if err := UpdateMessageTextWithContext(testCtx(), messageID, "B"); err != nil {
			t.Fatal(err)
		}
		if err := UpdateMessageTextWithContext(testCtx(), messageID, "A"); err != nil {
			t.Fatal(err)
		}
		messageAfter, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false)
		if err != nil {
			t.Fatal(err)
		}
		conversationAfter, err := ConversationContentSnapshotWithContext(testCtx(), conversationID)
		if err != nil {
			t.Fatal(err)
		}
		if messageBefore == messageAfter {
			t.Fatal("conteúdo ABA não renovou snapshot da mensagem com relógio congelado")
		}
		if conversationBefore == conversationAfter {
			t.Fatal("conteúdo ABA não renovou snapshot da conversa com relógio congelado")
		}

		err = WithConversationLifecycle(testCtx(), func() error {
			_, err := UpdateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), conversationID, messageID, messageBefore, "old commit")
			return err
		})
		if !errors.Is(err, ErrMessageContentChanged) {
			t.Fatalf("commit antigo de texto foi aceito: %v", err)
		}
		err = WithConversationLifecycle(testCtx(), func() error {
			return ClearConversationContentIfUnchangedWithinLifecycleWithContext(testCtx(), conversationID, conversationBefore)
		})
		if !errors.Is(err, ErrConversationContentChanged) {
			t.Fatalf("clear com snapshot antigo foi aceito: %v", err)
		}

		var final ChatMessage
		if err := db.First(&final, "id = ?", messageID).Error; err != nil {
			t.Fatal(err)
		}
		if final.Content != "A" || final.Pinned {
			t.Fatalf("estado ABA não foi preservado após rejeitar commits antigos: %+v", final)
		}
		fixed, err := time.Parse(time.RFC3339Nano, messageRevisionTestFixedTime)
		if err != nil {
			t.Fatal(err)
		}
		if !final.CreatedAt.Equal(fixed) || !final.UpdatedAt.Equal(fixed) {
			t.Fatalf("timestamps não ficaram congelados: created=%v updated=%v", final.CreatedAt, final.UpdatedAt)
		}
	})
}

func TestMessageRevisionStorageTriggersCoverRawBulkUpdateColumnAndIDMove(t *testing.T) {
	setupTestDB(t)
	freezeMessageRevisionTestClock(t)

	conversationID := createTestConversation(t, "triggers")
	rawID := createTestMessage(t, conversationID, "user", "raw")
	columnID := createTestMessage(t, conversationID, "user", "column")
	bulkOneID := createTestMessage(t, conversationID, "user", "bulk-one")
	bulkTwoID := createTestMessage(t, conversationID, "user", "bulk-two")
	moveID := createTestMessage(t, conversationID, "user", "move")

	before := make(map[string]string)
	for _, messageID := range []string{rawID, columnID, bulkOneID, bulkTwoID, moveID} {
		revision, ok := readMessageRevisionTest(t, db, messageID)
		if !ok {
			t.Fatalf("mensagem %s nasceu sem nonce", messageID)
		}
		assertMessageRevisionTestToken(t, revision)
		before[messageID] = revision
	}

	if err := db.Exec("UPDATE chat_messages SET content = ? WHERE id = ?", "raw-updated", rawID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ChatMessage{}).Where("id = ?", columnID).UpdateColumn("content", "column-updated").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ChatMessage{}).Where("id IN ?", []string{bulkOneID, bulkTwoID}).Updates(map[string]interface{}{"source": "bulk-updated"}).Error; err != nil {
		t.Fatal(err)
	}
	newID := "message-id-moved-by-sql"
	if err := db.Exec("UPDATE chat_messages SET id = ? WHERE id = ?", newID, moveID).Error; err != nil {
		t.Fatal(err)
	}

	for _, messageID := range []string{rawID, columnID, bulkOneID, bulkTwoID} {
		revision, ok := readMessageRevisionTest(t, db, messageID)
		if !ok {
			t.Fatalf("revisão desapareceu após UPDATE de %s", messageID)
		}
		assertMessageRevisionTestToken(t, revision)
		if revision == before[messageID] {
			t.Fatalf("trigger não renovou nonce de %s", messageID)
		}
	}
	if _, ok := readMessageRevisionTest(t, db, moveID); ok {
		t.Fatalf("nonce do ID antigo %s sobreviveu ao move", moveID)
	}
	movedRevision, ok := readMessageRevisionTest(t, db, newID)
	if !ok {
		t.Fatalf("ID novo %s não recebeu nonce", newID)
	}
	assertMessageRevisionTestToken(t, movedRevision)
	if movedRevision == before[moveID] {
		t.Fatal("move de ID não renovou nonce")
	}
}

func TestMessageRevisionStorageRollbackIsAtomicWithMessageWrite(t *testing.T) {
	setupTestDB(t)
	freezeMessageRevisionTestClock(t)

	conversationID := createTestConversation(t, "rollback")
	messageID := createTestMessage(t, conversationID, "user", "before")
	beforeRevision, ok := readMessageRevisionTest(t, db, messageID)
	if !ok {
		t.Fatal("mensagem de rollback sem nonce")
	}
	sentinel := errors.New("rollback solicitado pelo teste")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE chat_messages SET content = ? WHERE id = ?", "after", messageID).Error; err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro de rollback = %v", err)
	}

	var message ChatMessage
	if err := db.First(&message, "id = ?", messageID).Error; err != nil {
		t.Fatal(err)
	}
	if message.Content != "before" {
		t.Fatalf("escrita sobreviveu ao rollback: %q", message.Content)
	}
	afterRevision, ok := readMessageRevisionTest(t, db, messageID)
	if !ok || afterRevision != beforeRevision {
		t.Fatalf("nonce não sofreu rollback junto com a escrita: antes=%q depois=%q existe=%v", beforeRevision, afterRevision, ok)
	}

	if err := db.Exec("CREATE TRIGGER reject_message_revision_test AFTER UPDATE OF content ON chat_messages BEGIN SELECT RAISE(ABORT, 'reject message update'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_messages SET content = ? WHERE id = ?", "rejected", messageID).Error; err == nil {
		t.Fatal("UPDATE rejeitado pelo trigger inesperadamente confirmou")
	}
	var afterRejected ChatMessage
	if err := db.First(&afterRejected, "id = ?", messageID).Error; err != nil {
		t.Fatal(err)
	}
	rejectedRevision, ok := readMessageRevisionTest(t, db, messageID)
	if !ok || afterRejected.Content != "before" || rejectedRevision != beforeRevision {
		t.Fatalf("falha do writer deixou mutação parcial: content=%q revision=%q", afterRejected.Content, rejectedRevision)
	}
}

func TestMessageRevisionStorageDeleteReinsertSameIDGetsFreshRevision(t *testing.T) {
	setupTestDB(t)
	freezeMessageRevisionTestClock(t)

	conversationID := createTestConversation(t, "delete-reinsert")
	messageID := "same-message-id"
	fixed, err := time.Parse(time.RFC3339Nano, messageRevisionTestFixedTime)
	if err != nil {
		t.Fatal(err)
	}
	message := &ChatMessage{
		UUIDModel:      UUIDModel{ID: messageID, CreatedAt: fixed, UpdatedAt: fixed},
		ConversationID: conversationID,
		Role:           "user",
		Content:        "same payload",
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	firstRevision, ok := readMessageRevisionTest(t, db, messageID)
	if !ok {
		t.Fatal("primeira encarnação sem nonce")
	}
	oldMessageSnapshot, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false)
	if err != nil {
		t.Fatal(err)
	}
	oldConversationSnapshot, err := ConversationContentSnapshotWithContext(testCtx(), conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM chat_messages WHERE id = ?", messageID).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := readMessageRevisionTest(t, db, messageID); ok {
		t.Fatal("DELETE não removeu o sidecar")
	}

	reinserted := &ChatMessage{
		UUIDModel:      UUIDModel{ID: messageID, CreatedAt: fixed, UpdatedAt: fixed},
		ConversationID: conversationID,
		Role:           "user",
		Content:        "same payload",
	}
	if err := db.Create(reinserted).Error; err != nil {
		t.Fatal(err)
	}
	secondRevision, ok := readMessageRevisionTest(t, db, messageID)
	if !ok {
		t.Fatal("reinserção não criou sidecar")
	}
	assertMessageRevisionTestToken(t, secondRevision)
	if firstRevision == secondRevision {
		t.Fatal("reinserção do mesmo ID reutilizou nonce")
	}
	var final ChatMessage
	if err := db.First(&final, "id = ?", messageID).Error; err != nil {
		t.Fatal(err)
	}
	if final.Content != message.Content || !final.CreatedAt.Equal(fixed) || !final.UpdatedAt.Equal(fixed) {
		t.Fatalf("reinserção não preservou o mesmo payload/datas: %+v", final)
	}
	newMessageSnapshot, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false)
	if err != nil {
		t.Fatal(err)
	}
	newConversationSnapshot, err := ConversationContentSnapshotWithContext(testCtx(), conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if oldMessageSnapshot == newMessageSnapshot || oldConversationSnapshot == newConversationSnapshot {
		t.Fatal("delete/reinsert ABA não alterou snapshots com payload e datas idênticos")
	}
	err = WithConversationLifecycle(testCtx(), func() error {
		_, err := UpdateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), conversationID, messageID, oldMessageSnapshot, "old commit")
		return err
	})
	if !errors.Is(err, ErrMessageContentChanged) {
		t.Fatalf("commit antigo de texto após delete/reinsert foi aceito: %v", err)
	}
	err = WithConversationLifecycle(testCtx(), func() error {
		_, err := MutateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), conversationID, messageID, oldConversationSnapshot, true)
		return err
	})
	if !errors.Is(err, ErrMessageContentChanged) {
		t.Fatalf("commit antigo de delete após delete/reinsert foi aceito: %v", err)
	}
}

func TestMessageRevisionStorageMigrationBackfillsPreservesAndIsIdempotent(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if err := database.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	conversation := &Conversation{UUIDModel: UUIDModel{ID: "legacy-conversation"}, UserID: testUserID, Title: "legacy"}
	if err := database.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	preserved := &ChatMessage{UUIDModel: UUIDModel{ID: "legacy-preserved"}, ConversationID: conversation.ID, Role: "user", Content: "preserve"}
	backfilled := &ChatMessage{UUIDModel: UUIDModel{ID: "legacy-backfill"}, ConversationID: conversation.ID, Role: "assistant", Content: "backfill"}
	if err := database.Create(preserved).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(backfilled).Error; err != nil {
		t.Fatal(err)
	}

	if err := database.Exec(`CREATE TABLE chat_message_revisions (message_id TEXT PRIMARY KEY, revision TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	legacyToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := database.Exec("INSERT INTO chat_message_revisions (message_id, revision) VALUES (?, ?)", preserved.ID, legacyToken).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateMessageRevisions(database); err != nil {
		t.Fatal(err)
	}
	backfilledToken, ok := readMessageRevisionTest(t, database, backfilled.ID)
	if !ok {
		t.Fatal("backfill não criou nonce para mensagem legada")
	}
	assertMessageRevisionTestToken(t, backfilledToken)
	preservedToken, ok := readMessageRevisionTest(t, database, preserved.ID)
	if !ok || preservedToken != legacyToken {
		t.Fatalf("backfill trocou nonce legado: %q", preservedToken)
	}

	firstRun := map[string]string{preserved.ID: preservedToken, backfilled.ID: backfilledToken}
	if err := MigrateMessageRevisions(database); err != nil {
		t.Fatal(err)
	}
	for messageID, expected := range firstRun {
		got, ok := readMessageRevisionTest(t, database, messageID)
		if !ok || got != expected {
			t.Fatalf("segunda migração não foi idempotente para %s: got=%q want=%q", messageID, got, expected)
		}
	}
	var count int64
	if err := database.Table("chat_message_revisions").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("quantidade de sidecars após migração = %d, esperado 2", count)
	}
}

func TestMessageRevisionStorageMissingRevisionFailsClosed(t *testing.T) {
	setupTestDB(t)
	conversationID := createTestConversation(t, "missing-revision")
	messageID := createTestMessage(t, conversationID, "user", "protected")
	if err := db.Exec("DELETE FROM chat_message_revisions WHERE message_id = ?", messageID).Error; err != nil {
		t.Fatal(err)
	}

	if revision, err := MessageCommandSnapshotWithContext(testCtx(), conversationID, messageID, false); err == nil || revision != "" {
		t.Fatalf("snapshot da mensagem não falhou fechado: revision=%q err=%v", revision, err)
	}
	if revision, err := ConversationContentSnapshotWithContext(testCtx(), conversationID); err == nil || revision != "" {
		t.Fatalf("snapshot da conversa não falhou fechado: revision=%q err=%v", revision, err)
	}
}

func TestMessageRevisionStorageCommittedFileWriterInvalidatesCapturedConversation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message-revisions.sqlite")
	primary := openMessageRevisionTestFileDB(t, path)
	writer := openMessageRevisionTestFileDB(t, path)
	fixed, err := time.Parse(time.RFC3339Nano, messageRevisionTestFixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := primary.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateMessageRevisions(primary); err != nil {
		t.Fatal(err)
	}
	conversation := &Conversation{UUIDModel: UUIDModel{ID: "file-conversation"}, UserID: testUserID, Title: "file"}
	if err := primary.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	message := &ChatMessage{UUIDModel: UUIDModel{ID: "file-message"}, ConversationID: conversation.ID, Role: "user", Content: "captured"}
	if err := primary.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	initialRevision, ok := readMessageRevisionTest(t, primary, message.ID)
	if !ok {
		t.Fatal("mensagem do writer sem nonce")
	}
	var initial ChatMessage
	if err := primary.First(&initial, "id = ?", message.ID).Error; err != nil {
		t.Fatal(err)
	}

	previousDB := db
	db = primary
	t.Cleanup(func() { db = previousDB })
	oldMessageSnapshot, err := MessageCommandSnapshotWithContext(testCtx(), conversation.ID, message.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := ConversationContentSnapshotWithContext(testCtx(), conversation.ID)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	writeErrors := make(chan error, 2)
	go func() {
		<-start
		writeErrors <- writer.Model(&ChatMessage{}).
			Where("id = ?", message.ID).
			UpdateColumn("pinned", gorm.Expr("NOT pinned")).Error
	}()
	go func() {
		<-start
		writeErrors <- writer.Exec(
			"UPDATE chat_messages SET pinned = NOT pinned, updated_at = ? WHERE id = ?",
			fixed, message.ID,
		).Error
	}()
	close(start)
	var writerErrors []error
	for i := 0; i < 2; i++ {
		writerErrors = append(writerErrors, <-writeErrors)
	}
	for _, writeErr := range writerErrors {
		if writeErr != nil {
			t.Fatalf("writer concorrente não confirmou: %v", writeErr)
		}
	}

	invalidated, err := ConversationContentSnapshotWithContext(testCtx(), conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated == captured {
		t.Fatal("escrita confirmada em outro handle não invalidou snapshot capturado")
	}
	var final ChatMessage
	if err := primary.First(&final, "id = ?", message.ID).Error; err != nil {
		t.Fatal(err)
	}
	if final.Pinned || !final.UpdatedAt.Equal(initial.UpdatedAt) {
		t.Fatalf("duas alternâncias não voltaram ao estado inicial com updated_at congelado: inicial=%+v final=%+v", initial, final)
	}
	finalRevision, ok := readMessageRevisionTest(t, primary, message.ID)
	if !ok || finalRevision == initialRevision {
		t.Fatalf("writers confirmados não deixaram nonce novo: inicial=%q final=%q", initialRevision, finalRevision)
	}
	oldCommitErr := WithConversationLifecycle(testCtx(), func() error {
		_, err := MutateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), conversation.ID, message.ID, oldMessageSnapshot, false)
		return err
	})
	if !errors.Is(oldCommitErr, ErrMessageContentChanged) {
		t.Fatalf("commit antigo após writers foi aceito: %v", oldCommitErr)
	}
}
