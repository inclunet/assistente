package portability

import (
	"testing"

	"assistente/internal/database"
)

func TestAudioRoundTripPreservesReceivedAndUnknownWithoutTTS(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	ctx := portabilityTestCtx()
	conversation := ConversationExport{ID: "audio-roundtrip", Title: "Audio", Messages: []MessageExport{
		{ID: "received", Role: "user", Content: "received", Audio: "AQID", AudioMimeType: "audio/wav", AudioOrigin: database.AttachmentReceived},
		{ID: "generated", Role: "assistant", Content: "generated", Audio: "BAUG", AudioMimeType: "audio/wav", AudioOrigin: database.AttachmentGenerated},
		{ID: "unknown", Role: "user", Content: "unknown", Audio: "AQID", AudioMimeType: "audio/wav", AudioOrigin: database.AttachmentLegacyUnknown},
	}}
	if _, err := importConversation(ctx, conversation, true); err != nil {
		t.Fatal(err)
	}
	for _, includeTTS := range []bool{true, false} {
		exported, err := buildConversationExports(ctx, []string{conversation.ID}, includeTTS)
		if err != nil {
			t.Fatal(err)
		}
		if len(exported) != 1 || len(exported[0].Messages) != 3 {
			t.Fatal("incomplete export", exported)
		}
		for _, message := range exported[0].Messages {
			if message.ID == "generated" && !includeTTS {
				if message.Audio != "" {
					t.Fatal("export retained generated TTS")
				}
				continue
			}
			found := false
			for _, original := range conversation.Messages {
				if original.ID == message.ID {
					found = true
					if original.Audio != message.Audio || original.AudioOrigin != message.AudioOrigin || original.AudioMimeType != message.AudioMimeType {
						t.Fatalf("audio changed: %+v", message)
					}
				}
			}
			if !found {
				t.Fatal("unexpected message", message.ID)
			}
		}
		if _, err := importConversation(ctx, exported[0], includeTTS); err != nil {
			t.Fatal("overwrite", err)
		}
	}
	for _, id := range []string{"received", "unknown"} {
		data, mime, err := database.GetMessageAudioWithContext(ctx, id)
		if err != nil || data != "AQID" || mime != "audio/wav" {
			t.Fatal("roundtrip lost received/unknown", id, err)
		}
	}
	if database.HasMessageAudioWithContext(ctx, "generated") {
		t.Fatal("roundtrip retained TTS")
	}
	var blobs int64
	database.DB().Table("chat_attachment_contents").Count(&blobs)
	inline := database.DB().Migrator().HasColumn("chat_messages", "audio") || database.DB().Migrator().HasColumn("chat_messages", "audio_mime_type")
	if blobs != 1 || inline {
		t.Fatal("duplicate storage", blobs, inline)
	}
	conversation.Messages[0].Audio = "invalid!"
	if _, err := importConversation(ctx, conversation, true); err == nil {
		t.Fatal("invalid import succeeded")
	}
	if data, _, err := database.GetMessageAudioWithContext(ctx, "received"); err != nil || data != "AQID" {
		t.Fatal("failed overwrite destroyed original", err)
	}
}

func TestAudioExportVersionPreservesLegacyWithoutInventingOrigin(t *testing.T) {
	for version := 1; version <= 4; version++ {
		for _, origin := range []string{"", database.AttachmentReceived, database.AttachmentGenerated, database.AttachmentLegacyUnknown, "invalid"} {
			file := ExportFile{Version: version, Resources: ExportResources{Conversations: []ConversationExport{{Messages: []MessageExport{{Audio: "AQID", AudioOrigin: origin}}}}}}
			err := normalizeExportVersion(&file)
			valid := (version < 4 && origin == "") || (version == 4 && origin != "" && origin != "invalid")
			if (err == nil) != valid {
				t.Fatalf("version %d origin %q: %v", version, origin, err)
			}
			if valid && version < 4 && file.Resources.Conversations[0].Messages[0].AudioOrigin != database.AttachmentLegacyUnknown {
				t.Fatal("invented legacy origin")
			}
		}
	}
}
