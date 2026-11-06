package database

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func assertSameMedia(t *testing.T, want, got string) {
	t.Helper()
	var left, right any
	if err := json.Unmarshal([]byte(want), &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(got), &right); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("mídia alterada: esperado %s, recebido %s", want, got)
	}
}

func TestMediaAttachmentsPreserveOccurrencesMetadataEmptyAndReferences(t *testing.T) {
	database, ctx, root := audioRuntimeFixture(t)
	media := `[{"type":"image/png","name":"a.png","data":"AQID","size":3,"future":{"label":"preservar"}},{"type":"image/png","name":"b.png","data":"AQID","size":3},{"type":"text/plain","name":"vazio.txt","data":"","size":0},{"type":"image","url":"https://example.invalid/imagem","caption":"não baixar"},{"type":"image","url":"data:image/png;base64,AQID"}]`
	message, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: root.ConversationID, Role: "user", Media: media})
	if err != nil {
		t.Fatal(err)
	}
	var contentCount, linkCount int64
	if err := database.Table("chat_attachment_contents").Count(&contentCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Table("chat_message_attachments").Count(&linkCount).Error; err != nil {
		t.Fatal(err)
	}
	if contentCount != 2 || linkCount != 5 {
		t.Fatalf("binários=%d ocorrências=%d", contentCount, linkCount)
	}
	messages := []ChatMessage{{UUIDModel: UUIDModel{ID: message.ID}}}
	if err := LoadMessageMedia(ctx, database, messages); err != nil {
		t.Fatal(err)
	}
	assertSameMedia(t, media, messages[0].Media)
	for _, column := range []string{"media", "audio", "audio_mime_type"} {
		if database.Migrator().HasColumn("chat_messages", column) {
			t.Fatalf("coluna legada permanece: %s", column)
		}
	}
	foreign := WithUserID(ctx, "other-user")
	if err := LoadMessageMedia(foreign, database, messages); err != nil {
		t.Fatal(err)
	}
	if messages[0].Media != "" {
		t.Fatal("anexo vazou entre usuários")
	}
	if err := database.Delete(&ChatMessage{}, "id=?", message.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Table("chat_attachment_contents").Count(&contentCount).Error; err != nil || contentCount != 0 {
		t.Fatalf("conteúdo sem referência retido: %d %v", contentCount, err)
	}
}

func TestMediaAttachmentsDeduplicateReceivedAudioAndKeepPlayback(t *testing.T) {
	database, ctx, root := audioRuntimeFixture(t)
	message, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: root.ConversationID, Role: "user", Media: `[{"type":"audio/wav","name":"voz.wav","data":"AQID"}]`, Audio: "AQID", AudioMimeType: "audio/wav", AudioOrigin: AttachmentReceived})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Table("chat_message_attachments").Where("message_id=?", message.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("áudio recebido duplicado: %d %v", count, err)
	}
	data, mime, err := GetMessageAudioWithContext(ctx, message.ID)
	if err != nil || data != "AQID" || mime != "audio/wav" {
		t.Fatalf("playback recebido: %q %q %v", data, mime, err)
	}
	if err := SaveMessageAudioWithContext(ctx, message.ID, "BAUG", "audio/mpeg"); err != nil {
		t.Fatal(err)
	}
	data, mime, err = GetMessageAudioWithContext(ctx, message.ID)
	if err != nil || data != "BAUG" || mime != "audio/mpeg" {
		t.Fatalf("playback TTS: %q %q %v", data, mime, err)
	}
	messages := []ChatMessage{*message}
	if err := LoadMessageMedia(ctx, database, messages); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[0].Media, "AQID") {
		t.Fatal("TTS substituiu áudio recebido")
	}
}

func TestMediaAttachmentFailureRollsBackMessage(t *testing.T) {
	database, ctx, root := audioRuntimeFixture(t)
	var before, after int64
	database.Model(&ChatMessage{}).Count(&before)
	if _, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: root.ConversationID, Role: "user", Media: `[{"type":"image/png","data":"invalid!"}]`}); err == nil {
		t.Fatal("mídia inválida foi aceita")
	}
	database.Model(&ChatMessage{}).Count(&after)
	if before != after {
		t.Fatal("mensagem parcial ficou persistida")
	}
}

func TestMediaAttachmentMutationInvalidatesConfirmation(t *testing.T) {
	database, ctx, root := audioRuntimeFixture(t)
	before, err := messageStoredRevisionTx(ctx, database, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "a"} {
		if err := database.Transaction(func(tx *gorm.DB) error {
			return storeMessageMedia(tx, root.ID, `[{"type":"text/plain","name":"`+name+`","data":"AQID"}]`)
		}); err != nil {
			t.Fatal(err)
		}
		after, err := messageStoredRevisionTx(ctx, database, root.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after == before {
			t.Fatal("alteração de anexo não invalidou confirmação")
		}
		before = after
	}
}

func TestMediaPlaybackSkipsEmptyAndExternalAudio(t *testing.T) {
	db, ctx, root := audioRuntimeFixture(t)
	for _, playable := range []bool{false, true} {
		media := `[{"type":"audio/wav","data":""},{"type":"audio/wav","url":"https://example.invalid/audio"}`
		audio := ""
		if playable {
			media += `,{"type":"audio/wav","data":"AQID"}`
			audio = "AQID"
		}
		media += `]`
		message, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: root.ConversationID, Role: "user", Media: media, Audio: audio, AudioMimeType: "audio/wav", AudioOrigin: AttachmentReceived})
		if err != nil {
			t.Fatal(err)
		}
		got, mime, err := GetMessageAudioWithContext(ctx, message.ID)
		if err != nil || got != audio || (playable && mime != "audio/wav") {
			t.Fatalf("playback: %q %q %v", got, mime, err)
		}
		if HasMessageAudioWithContext(ctx, message.ID) != playable {
			t.Fatal("disponibilidade de playback incorreta")
		}
		messages := []ChatMessage{*message}
		if err := LoadMessageMedia(ctx, db, messages); err != nil {
			t.Fatal(err)
		}
		assertSameMedia(t, media, messages[0].Media)
	}
}
