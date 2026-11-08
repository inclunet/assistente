package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func mediaV45Fixture(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v45.db")
	database, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := db
	db = database
	t.Cleanup(func() { db = previous; sqlDB, _ := database.DB(); _ = sqlDB.Close() })
	migrations := schemaMigrations[:len(schemaMigrations)-1]
	if err := runMigrationList(database, phasePreAutoMigrate, migrations); err != nil {
		t.Fatal(err)
	}
	fullAutoMigrate(t, database)
	if err := runMigrationList(database, phasePostAutoMigrate, migrations); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&User{UUIDModel: UUIDModel{ID: "media-owner"}, Username: "media-owner"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&Conversation{UUIDModel: UUIDModel{ID: "media-conv"}, UserID: "media-owner", Title: "Mídias"}).Error; err != nil {
		t.Fatal(err)
	}
	return database, path
}

func TestMediaMigrationPreservesBytesMetadataRevisionsAndSecondBoot(t *testing.T) {
	database, path := mediaV45Fixture(t)
	if err := initializeFTS5(database); err != nil {
		t.Fatal(err)
	}
	media := `[{"type":"image/png","name":"foto.png","data":"AQID","size":9,"extra":"histórico"},{"type":"audio/wav","data":"BAUG","name":"voz.wav"}]`
	if err := database.Exec("INSERT INTO chat_messages(id,conversation_id,role,content,media,created_at,updated_at) VALUES ('media-message','media-conv','user','buscável',?,'2020-01-02',NULL)", media).Error; err != nil {
		t.Fatal(err)
	}
	if err := storeMessageAudio(database, "media-message", "BAUG", "audio/wav", AttachmentReceived); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("UPDATE chat_messages SET updated_at=NULL WHERE id='media-message'").Error; err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "media-owner")
	revision, err := messageStoredRevisionTx(ctx, database, "media-message")
	if err != nil {
		t.Fatal(err)
	}
	// An unrelated historical orphan is not a new violation of this cutover.
	if err := database.Exec("CREATE TABLE unrelated_legacy(id TEXT, missing TEXT REFERENCES users(id))").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO unrelated_legacy VALUES ('legacy','missing')").Error; err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(database, phasePostAutoMigrate); err != nil {
		t.Fatal(err)
	}
	messages := []ChatMessage{{UUIDModel: UUIDModel{ID: "media-message"}}}
	if err := LoadMessageMedia(ctx, database, messages); err != nil {
		t.Fatal(err)
	}
	assertSameMedia(t, media, messages[0].Media)
	after, err := messageStoredRevisionTx(ctx, database, "media-message")
	if err != nil || after != revision {
		t.Fatalf("migração alterou nonce histórico: %v", err)
	}
	var timestamps struct {
		CreatedAt string
		UpdatedAt *string
	}
	if err := database.Raw("SELECT CAST(created_at AS TEXT) created_at,CAST(updated_at AS TEXT) updated_at FROM chat_messages WHERE id='media-message'").Scan(&timestamps).Error; err != nil {
		t.Fatal(err)
	}
	if timestamps.CreatedAt != "2020-01-02" || timestamps.UpdatedAt != nil {
		t.Fatalf("timestamps alterados: %+v", timestamps)
	}
	var links int64
	database.Table("chat_message_attachments").Count(&links)
	if links != 2 {
		t.Fatalf("duplicou ocorrência de áudio: %d", links)
	}
	if stat, err := os.Stat(path + ".pre-chat-media-v46.bak"); err != nil || stat.Size() == 0 {
		t.Fatalf("backup ausente: %v", err)
	}
	var indexed int64
	if err := database.Raw("SELECT COUNT(*) FROM chat_messages_fts WHERE chat_messages_fts MATCH 'buscável'").Scan(&indexed).Error; err != nil || indexed != 1 {
		t.Fatalf("busca perdeu mensagem: %d %v", indexed, err)
	}
	if err := database.Exec("UPDATE chat_messages SET content='atualizado' WHERE id='media-message'").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Raw("SELECT COUNT(*) FROM chat_messages_fts WHERE chat_messages_fts MATCH 'atualizado'").Scan(&indexed).Error; err != nil || indexed != 1 {
		t.Fatalf("trigger FTS não preservado: %d %v", indexed, err)
	}
	fullAutoMigrate(t, database)
	if err := runMigrations(database, phasePostAutoMigrate); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"media", "audio", "audio_mime_type"} {
		if database.Migrator().HasColumn("chat_messages", column) {
			t.Fatalf("segundo boot recriou %s", column)
		}
	}
}

func TestMediaMigrationInvalidBytesRollBackEntireCutover(t *testing.T) {
	for name, invalid := range map[string]string{
		"base64":    `[{"type":"image/png","data":"invalid!"}]`,
		"null-data": `[{"type":"image/png","data":null}]`,
		"null-url":  `[{"type":"image/png","url":null}]`,
	} {
		t.Run(name, func(t *testing.T) {
			database, _ := mediaV45Fixture(t)
			for _, row := range []struct{ ID, Media string }{{"a-valid", `[{"type":"image/png","data":"AQID"}]`}, {"z-invalid", invalid}} {
				if err := database.Exec("INSERT INTO chat_messages(id,conversation_id,role,content,media) VALUES (?,'media-conv','user','',?)", row.ID, row.Media).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := runMigrations(database, phasePostAutoMigrate); err == nil {
				t.Fatal("migração aceitou conteúdo inválido")
			}
			versions, err := appliedMigrationVersions(database)
			if err != nil || versions[46] {
				t.Fatalf("migração falha foi registrada: %v", err)
			}
			if !database.Migrator().HasColumn("chat_messages", "media") || database.Migrator().HasColumn("chat_message_attachments", "metadata") {
				t.Fatal("schema parcial após rollback")
			}
			var media string
			database.Raw("SELECT media FROM chat_messages WHERE id='a-valid'").Scan(&media)
			assertSameMedia(t, `[{"type":"image/png","data":"AQID"}]`, media)
			var count int64
			database.Table("chat_message_attachments").Count(&count)
			if count != 0 {
				t.Fatal("rollback deixou anexos parciais")
			}
			var preserved string
			if err := database.Raw("SELECT media FROM chat_messages WHERE id='z-invalid'").Scan(&preserved).Error; err != nil || preserved != invalid {
				t.Fatal("rollback alterou conteúdo inválido", err)
			}
		})
	}
}

func TestMediaMigrationDefersOwnerlessAndBlocksMissingConversation(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "ownerless", true: "missing_conversation"}[missing], func(t *testing.T) {
			database, _ := mediaV45Fixture(t)
			if err := database.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
				t.Fatal(err)
			}
			if !missing {
				if err := database.Exec("INSERT INTO conversations(id,user_id,title) VALUES ('orphan',NULL,'órfã')").Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, trigger := range []string{"chat_audio_retired_insert", "chat_audio_retired_update"} {
				if err := database.Exec("DROP TRIGGER " + trigger).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := database.Exec("INSERT INTO chat_messages(id,conversation_id,role,content,audio,audio_mime_type) VALUES ('orphan-audio','orphan','user','','AQID','audio/wav')").Error; err != nil {
				t.Fatal(err)
			}
			err := MigrateChatMediaAttachments(database)
			if missing {
				if err == nil || !strings.Contains(err.Error(), "sem conversa") {
					t.Fatalf("diagnóstico ausente: %v", err)
				}
			} else if !errors.Is(err, errMigrationDeferred) {
				t.Fatalf("deveria adiar: %v", err)
			}
			var audio string
			database.Raw("SELECT audio FROM chat_messages WHERE id='orphan-audio'").Scan(&audio)
			if audio != "AQID" {
				t.Fatal("corte perdeu áudio órfão")
			}
			if !missing {
				if err := AdoptLegacyData("media-owner"); err != nil {
					t.Fatal(err)
				}
				data, _, err := GetMessageAudioWithContext(WithUserID(context.Background(), "media-owner"), "orphan-audio")
				if err != nil || data != "AQID" {
					t.Fatalf("adoção perdeu áudio: %s %v", data, err)
				}
				if database.Migrator().HasColumn("chat_messages", "audio") {
					t.Fatal("adoção não concluiu corte")
				}
			}
		})
	}
}

func TestMediaMigrationRejectsCorruptCanonicalAudio(t *testing.T) {
	database, _ := mediaV45Fixture(t)
	if err := database.Exec("INSERT INTO chat_messages(id,conversation_id,role,content) VALUES ('audio','media-conv','user','')").Error; err != nil {
		t.Fatal(err)
	}
	if err := storeMessageAudio(database, "audio", "AQID", "audio/wav", AttachmentReceived); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("DROP TRIGGER chat_attachment_content_immutable").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("UPDATE chat_attachment_contents SET sha256=?", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatMediaAttachments(database); err == nil {
		t.Fatal("migração aceitou hash inválido")
	}
	if !database.Migrator().HasColumn("chat_messages", "media") {
		t.Fatal("falha não reverteu corte físico")
	}
}

func TestMediaMigrationReplayPreservesNormalizedAttachments(t *testing.T) {
	tx, ctx, root := audioRuntimeFixture(t)
	media := `[{"type":"image/png","name":"keep","url":"data:image/png;base64,AQID","extra":true}]`
	message, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: root.ConversationID, Role: "user", Media: media, Audio: "BAUG", AudioMimeType: "audio/wav", AudioOrigin: AttachmentGenerated})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := MigrateChatAudioAttachments(tx); err != nil {
			t.Fatal(err)
		}
		if err := MigrateChatMediaAttachments(tx); err != nil {
			t.Fatal(err)
		}
	}
	messages := []ChatMessage{*message}
	if err := LoadMessageMedia(ctx, tx, messages); err != nil {
		t.Fatal(err)
	}
	assertSameMedia(t, media, messages[0].Media)
	audio, mime, err := GetMessageAudioWithContext(ctx, message.ID)
	if err != nil || audio != "BAUG" || mime != "audio/wav" {
		t.Fatalf("replay perdeu áudio: %q %q %v", audio, mime, err)
	}
	for _, column := range []string{"media", "audio", "audio_mime_type"} {
		if tx.Migrator().HasColumn("chat_messages", column) {
			t.Fatal("replay recriou coluna", column)
		}
	}
}

func TestMediaMigrationRejectsCrossOwnerAttachment(t *testing.T) {
	tx, _ := mediaV45Fixture(t)
	if err := tx.Create(&User{UUIDModel: UUIDModel{ID: "other-owner"}, Username: "other-owner"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(&ChatMessage{UUIDModel: UUIDModel{ID: "cross-owner"}, ConversationID: "media-conv", Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := storeMessageAudio(tx, "cross-owner", "AQID", "audio/wav", AttachmentReceived); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("DROP TRIGGER chat_attachment_content_immutable").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE chat_attachment_contents SET user_id='other-owner'").Error; err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(tx, phasePostAutoMigrate); err == nil || !strings.Contains(err.Error(), "proprietário") {
		t.Fatal("migração aceitou vínculo entre usuários", err)
	}
	if !tx.Migrator().HasColumn("chat_messages", "media") {
		t.Fatal("falha não reverteu corte")
	}
}
