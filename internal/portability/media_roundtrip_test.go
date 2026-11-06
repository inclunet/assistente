package portability

import (
	"encoding/json"
	"reflect"
	"testing"

	"assistente/internal/database"
)

func TestMediaRoundTripAndInvalidOverwritePreserveAttachments(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	ctx := portabilityTestCtx()
	media := `[{"type":"image/png","name":"primeira.png","data":"AQID","size":3},{"type":"image/png","name":"segunda.png","data":"AQID","size":3},{"type":"text/plain","name":"vazio.txt","data":"","size":0},{"type":"image","url":"https://example.invalid/original","extra":"preservar"},{"type":"audio/wav","data":"BAUG"}]`
	original := ConversationExport{ID: "media-roundtrip", Title: "Anexos", Messages: []MessageExport{{ID: "with-media", Role: "user", Content: "com anexos", Media: media, Audio: "BAUG", AudioMimeType: "audio/wav", AudioOrigin: database.AttachmentReceived}}}
	if _, err := importConversation(ctx, original, true); err != nil {
		t.Fatal(err)
	}
	for _, includeTTS := range []bool{false, true} {
		exported, err := buildConversationExports(ctx, []string{original.ID}, includeTTS)
		if err != nil {
			t.Fatal(err)
		}
		if len(exported) != 1 || len(exported[0].Messages) != 1 {
			t.Fatal("exportação incompleta")
		}
		message := exported[0].Messages[0]
		var want, got any
		if err := json.Unmarshal([]byte(media), &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(message.Media), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("perdeu ordem/metadados: %s", message.Media)
		}
		if message.Audio != "" {
			t.Fatal("backup duplicou áudio já presente em media")
		}
		if _, err := importConversation(ctx, exported[0], includeTTS); err != nil {
			t.Fatal(err)
		}
	}
	var contents, links int64
	database.DB().Table("chat_attachment_contents").Count(&contents)
	database.DB().Table("chat_message_attachments").Count(&links)
	if contents != 3 || links != 5 {
		t.Fatalf("binários=%d ocorrências=%d", contents, links)
	}
	original.Messages[0].Media = `[{"type":"image/png","data":"invalid!"}]`
	if _, err := importConversation(ctx, original, true); err == nil {
		t.Fatal("sobrescrita inválida aceita")
	}
	exported, err := buildConversationExports(ctx, []string{original.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(media), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(exported[0].Messages[0].Media), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("rollback perdeu anexos originais")
	}
}
