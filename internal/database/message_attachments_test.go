package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func audioRuntimeFixture(t *testing.T) (*gorm.DB, context.Context, *ChatMessage) {
	t.Helper()
	restore := SetDB(DB())
	if err := InitPath(filepath.Join(t.TempDir(), "audio.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(); restore() })
	if err := db.Create(&User{UUIDModel: UUIDModel{ID: "audio-owner"}, Username: "audio-owner"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "audio-owner")
	conv, err := CreateConversationWithContext(ctx, "Áudio", "")
	if err != nil {
		t.Fatal(err)
	}
	message, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "user", Content: "texto"})
	if err != nil {
		t.Fatal(err)
	}
	return db, ctx, message
}

func TestAudioAttachmentsDeduplicateIsolateAndCollect(t *testing.T) {
	tx, ctx, m := audioRuntimeFixture(t)
	other, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: m.ConversationID, Role: "assistant", Content: "outra"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{m.ID, other.ID} {
		if err := SaveMessageAudioWithContext(ctx, id, "AQID", "audio/wav"); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int64) {
		t.Helper()
		var n int64
		if err := tx.Table("chat_attachment_contents").Count(&n).Error; err != nil || n != want {
			t.Fatalf("conteúdos=%d, esperado %d: %v", n, want, err)
		}
	}
	count(1)
	foreign := WithUserID(context.Background(), "foreign")
	if err := tx.Create(&User{UUIDModel: UUIDModel{ID: "foreign"}, Username: "foreign"}).Error; err != nil {
		t.Fatal(err)
	}
	conv, err := CreateConversationWithContext(foreign, "Privada", "")
	if err != nil {
		t.Fatal(err)
	}
	fm, err := CreateMessageWithContext(foreign, MessageOptions{ConversationID: conv.ID, Role: "user", Content: "privado"})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveMessageAudioWithContext(foreign, fm.ID, "AQID", "audio/wav"); err != nil {
		t.Fatal(err)
	}
	count(2)
	if _, _, err := GetMessageAudioWithContext(foreign, m.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("leitura cruzada", err)
	}
	if err := SaveMessageAudioWithContext(foreign, m.ID, "BAUG", "audio/wav"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("escrita cruzada", err)
	}
	if HasMessageAudioWithContext(foreign, m.ID) {
		t.Fatal("vazou existência")
	}
	if err := tx.Exec("UPDATE chat_message_attachments SET content_id=(SELECT content_id FROM chat_message_attachments WHERE message_id=?) WHERE message_id=?", fm.ID, m.ID).Error; err == nil {
		t.Fatal("SQL permitiu vínculo entre usuários")
	}
	if err := SaveMessageAudioWithContext(ctx, m.ID, "BAUG", "audio/wav"); err != nil {
		t.Fatal(err)
	}
	count(3)
	if err := SaveMessageAudioWithContext(ctx, m.ID, "not-base64", "audio/wav"); err == nil {
		t.Fatal("aceitou bytes inválidos")
	}
	if audio, _, err := GetMessageAudioWithContext(ctx, m.ID); err != nil || audio != "BAUG" {
		t.Fatal("falha alterou áudio", err)
	}
	if err := tx.Delete(&ChatMessage{}, "id=?", m.ID).Error; err != nil {
		t.Fatal(err)
	}
	count(2)
	if err := tx.Delete(&ChatMessage{}, "id=?", other.ID).Error; err != nil {
		t.Fatal(err)
	}
	count(1)
	if err := tx.Delete(&ChatMessage{}, "id=?", fm.ID).Error; err != nil {
		t.Fatal(err)
	}
	count(0)
}

func TestAudioMutationInvalidatesDestructiveConfirmationIncludingABA(t *testing.T) {
	_, ctx, m := audioRuntimeFixture(t)
	if err := SaveMessageAudioWithContext(ctx, m.ID, "AQID", "audio/wav"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"replace", "remove", "ABA"} {
		t.Run(mode, func(t *testing.T) {
			before, err := ConversationContentSnapshotWithContext(ctx, m.ConversationID)
			if err != nil {
				t.Fatal(err)
			}
			data := "BAUG"
			if mode == "remove" {
				data = ""
			}
			if err := SaveMessageAudioWithContext(ctx, m.ID, data, "audio/wav"); err != nil {
				t.Fatal(err)
			}
			if mode == "ABA" {
				if err := SaveMessageAudioWithContext(ctx, m.ID, "AQID", "audio/wav"); err != nil {
					t.Fatal(err)
				}
			}
			if err := ClearConversationContentIfUnchangedWithinMaintenanceWithContext(ctx, m.ConversationID, before); !errors.Is(err, ErrConversationContentChanged) {
				t.Fatal("confirmação antiga aceita", err)
			}
			if err := SaveMessageAudioWithContext(ctx, m.ID, "AQID", "audio/wav"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAudioMigrationPreservesProvenanceAndBackup(t *testing.T) {
	tx := newChatTurnTestDB(t, true)
	if err := ensureLegacyChatMediaColumns(tx); err != nil {
		t.Fatal(err)
	}
	seedChatTurnTestMessages(t, tx, "conv", "received", "unknown")
	if err := MigrateMessageRevisions(tx); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatTurns(tx); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(tx); err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTimelineVersions(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE chat_messages SET updated_at=CASE WHEN id='received' THEN '2020-01-02 03:04:05.123456' ELSE NULL END,audio='AQID',audio_mime_type='audio/wav',media=CASE WHEN id='received' THEN '[{"type":"audio/wav","data":"AQID"}]' ELSE '' END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatAudioAttachments(tx); err != nil {
		t.Fatal(err)
	}
	var changed int64
	if err := tx.Raw("SELECT COUNT(*) FROM chat_messages WHERE (id='received' AND updated_at IS NOT '2020-01-02 03:04:05.123456') OR (id='unknown' AND updated_at IS NOT NULL)").Scan(&changed).Error; err != nil || changed != 0 {
		t.Fatal("migration changed historical timestamp", changed, err)
	}
	messages := []ChatMessage{{UUIDModel: UUIDModel{ID: "received"}}, {UUIDModel: UUIDModel{ID: "unknown"}}}
	if err := LoadMessageAudios(WithUserID(context.Background(), "user"), tx, messages, AudioPreserved); err != nil {
		t.Fatal(err)
	}
	if messages[0].Audio != "AQID" || messages[0].AudioOrigin != AttachmentReceived || messages[1].AudioOrigin != AttachmentLegacyUnknown || messages[1].Audio != "AQID" {
		t.Fatalf("origem/bytes perdidos: %+v", messages)
	}
	var count int64
	tx.Table("chat_attachment_contents").Count(&count)
	if count != 1 {
		t.Fatal("duplicou binário", count)
	}
	sqlDB, _ := tx.DB()
	path, err := sqliteMainDatabasePath(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path + ".pre-chat-audio-v45.bak"); err != nil || info.Size() == 0 {
		t.Fatal("backup ausente", err)
	}
	if err := MigrateChatAudioAttachments(tx); err != nil {
		t.Fatal("não idempotente", err)
	}
	if err := tx.Exec("UPDATE chat_messages SET audio='AQID' WHERE id='received'").Error; err == nil {
		t.Fatal("aceitou escritor aposentado")
	}
}

func TestAudioMigrationRollsBackOnInvalidBytes(t *testing.T) {
	tx := newChatTurnTestDB(t, false)
	if err := ensureLegacyChatMediaColumns(tx); err != nil {
		t.Fatal(err)
	}
	seedChatTurnTestMessages(t, tx, "conv", "a-valid", "z-invalid")
	if err := MigrateChatTurns(tx); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(tx); err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTimelineVersions(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE chat_messages SET audio=CASE WHEN id='a-valid' THEN 'AQID' ELSE 'invalid!' END,audio_mime_type='audio/wav'").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatAudioAttachments(tx); err == nil {
		t.Fatal("migração aceitou bytes inválidos")
	}
	var m struct{ Audio string }
	if err := tx.Table("chat_messages").Where("id='a-valid'").Take(&m).Error; err != nil || m.Audio != "AQID" {
		t.Fatal("rollback perdeu bytes", err)
	}
	if tx.Migrator().HasTable("chat_attachment_contents") {
		t.Fatal("rollback deixou schema parcial")
	}
}

func TestLegacyAudioIsNormalizedAtomicallyOnAdoption(t *testing.T) {
	tx, _ := mediaV45Fixture(t)
	ctx := WithUserID(context.Background(), "media-owner")
	for _, name := range []string{"chat_audio_retired_insert", "chat_audio_retired_update"} {
		if err := tx.Exec("DROP TRIGGER IF EXISTS " + name).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec("INSERT INTO conversations(id,user_id,title) VALUES('orphan',NULL,'legado')").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO chat_messages(id,conversation_id,role,content,audio,audio_mime_type) VALUES('legacy-audio','orphan','user','voz','AQID','audio/wav')").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatAudioAttachments(tx); err != nil {
		t.Fatal("migração bloqueou login", err)
	}
	if _, _, err := GetMessageAudioWithContext(ctx, "legacy-audio"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("órfão acessível antes de adoção", err)
	}
	if err := AdoptLegacyData("media-owner"); err != nil {
		t.Fatal(err)
	}
	if audio, _, err := GetMessageAudioWithContext(ctx, "legacy-audio"); err != nil || audio != "AQID" {
		t.Fatal("adoção perdeu áudio", err)
	}
	if tx.Migrator().HasColumn("chat_messages", "audio") {
		t.Fatal("adoção manteve duas fontes")
	}
}

func TestAudioHistoryLoadsOnlyReceivedAttachments(t *testing.T) {
	_, ctx, original := audioRuntimeFixture(t)
	var all []ChatMessage
	for _, origin := range []string{AttachmentReceived, AttachmentGenerated, AttachmentLegacyUnknown} {
		m, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: original.ConversationID, Role: "user", Content: origin, Audio: "AQID", AudioMimeType: "audio/wav", AudioOrigin: origin})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, *m)
	}
	for _, retry := range []bool{false, true} {
		var messages []ChatMessage
		if retry {
			window, err := NewMessageRepository(db).LoadHistoryWindowThroughMessageWithContext(ctx, original.ConversationID, all[2].ID, 50)
			if err != nil {
				t.Fatal(err)
			}
			messages = window.Messages
		} else {
			window, err := NewMessageRepository(db).LoadHistoryWindowWithContext(ctx, original.ConversationID, 50)
			if err != nil {
				t.Fatal(err)
			}
			messages = window.Messages
		}
		seen := 0
		for _, m := range messages {
			for _, target := range all {
				if m.ID == target.ID {
					seen++
					if target.AudioOrigin == AttachmentReceived {
						if m.Audio != "AQID" {
							t.Fatal("áudio recebido ausente")
						}
					} else if m.Audio != "" || m.AudioOrigin != "" {
						t.Fatal("carregou áudio fora do contexto")
					}
				}
			}
		}
		if seen != 3 {
			t.Fatalf("mensagens ausentes: %d", seen)
		}
	}
}

func TestAdoptionWithoutOrphansDoesNotScanMessageAudio(t *testing.T) {
	database, _, _ := audioRuntimeFixture(t)
	scans := 0
	const callback = "test:audio_adoption_scan"
	if err := database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "chat_messages" {
			scans++
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Callback().Query().Remove(callback); err != nil {
			t.Error(err)
		}
	}()
	for range 2 {
		if err := AdoptLegacyData("audio-owner"); err != nil {
			t.Fatal(err)
		}
	}
	if scans != 0 {
		t.Fatalf("adoção sem órfãos consultou mensagens %d vezes", scans)
	}
}
