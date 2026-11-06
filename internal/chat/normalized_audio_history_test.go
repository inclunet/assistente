package chat

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestNormalizedAudioReachesSendAndRetryHistory(t *testing.T) {
	for _, tc := range []struct {
		name, origin, text, media string
		audioCount                int
	}{
		{"received", database.AttachmentReceived, "", "", 1},
		{"transcribed", database.AttachmentReceived, "transcricao", "", 0},
		{"mixed", database.AttachmentReceived, "", `[{"type":"image/png","data":"image-marker"}]`, 1},
		{"duplicate", database.AttachmentReceived, "", `[{"type":"audio/wav","data":"/w=="}]`, 1},
		{"equivalent_encoding", database.AttachmentReceived, "", `[{"type":"audio/wav","data":"/x=="}]`, 1},
		{"generated", database.AttachmentGenerated, "texto", "", 0},
		{"unknown", database.AttachmentLegacyUnknown, "texto", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSnapshotFixture(t)
			message, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: tc.text, Media: tc.media, Audio: "/w==", AudioMimeType: "audio/wav", AudioOrigin: tc.origin})
			if err != nil {
				t.Fatal(err)
			}
			loader := MediaHistoryLoader{Repo: NewDBMessageStore(), MaxMsgs: 50}
			for _, retry := range []bool{false, true} {
				var messages []llm.Message
				if retry {
					messages, _, err = loader.LoadThroughMessage(f.ctx, f.conversation, message.ID)
				} else {
					messages, _, err = loader.Load(f.ctx, f.conversation)
				}
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, candidate := range messages {
					if candidate.MessageID != message.ID {
						continue
					}
					found = true
					content, err := json.Marshal(candidate.Content)
					if err != nil {
						t.Fatal(err)
					}
					wire := string(content)
					if strings.Count(wire, `"type":"input_audio"`) != tc.audioCount {
						t.Fatalf("retry=%v content=%s", retry, wire)
					}
					if tc.audioCount > 0 {
						for _, part := range candidate.Content.([]interface{}) {
							object := part.(map[string]interface{})
							if object["type"] == "input_audio" {
								audio := object["input_audio"].(map[string]interface{})["data"].(string)
								decoded, err := base64.StdEncoding.DecodeString(audio)
								if err != nil || len(decoded) != 1 || decoded[0] != 255 {
									t.Fatal("audio bytes lost", err)
								}
							}
						}
					}
					if tc.name == "mixed" && !strings.Contains(wire, "image-marker") {
						t.Fatal("other media lost")
					}
					if tc.text != "" && !strings.Contains(wire, tc.text) {
						t.Fatal("transcription lost")
					}
				}
				if !found {
					t.Fatal("message omitted")
				}
			}
		})
	}
}
