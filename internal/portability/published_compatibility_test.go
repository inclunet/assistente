package portability

import (
	"encoding/json"
	"os"
	"testing"

	"assistente/internal/database"
)

func TestPublishedExport019ImportsDirectlyAndIdempotently(t *testing.T) {
	setupPortabilityTestDB(t)
	raw, err := os.ReadFile("testdata/published/0.1.9-conversations.json")
	if err != nil {
		t.Fatal(err)
	}

	first, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("import direto da 0.1.9: %v", err)
	}
	if !first.Success || first.Imported != 1 {
		t.Fatalf("resultado da primeira importação: %#v", first)
	}

	second, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("segunda importação da 0.1.9: %v", err)
	}
	if !second.Success {
		t.Fatalf("resultado da segunda importação: %#v", second)
	}
	var conversationCount, messageCount int64
	if err := database.DB().Model(&database.Conversation{}).Count(&conversationCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.ChatMessage{}).Count(&messageCount).Error; err != nil {
		t.Fatal(err)
	}
	if conversationCount != 1 || messageCount != 2 {
		t.Fatalf("reimportação duplicou dados: conversas=%d mensagens=%d", conversationCount, messageCount)
	}

	file, _, err := parseExportFile(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	conversation := file.Resources.Conversations[0]
	if conversation.ID == "" || len(conversation.Messages) != 2 {
		t.Fatalf("adaptação incompleta: %#v", conversation)
	}
	if conversation.Messages[1].ParentID != conversation.Messages[0].ID ||
		conversation.Messages[1].TurnID != conversation.Messages[0].ID {
		t.Fatalf("hierarquia não preservada: %#v", conversation.Messages)
	}
}

func TestPublishedPortableV2RemainsCompatibleFrom020Through050(t *testing.T) {
	raw, err := os.ReadFile("testdata/published/0.2.0-0.5.0-portable-v2.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, release := range []string{"0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			fixture["appVersion"] = release
			versioned, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			file, unsupported, err := parseExportFile(string(versioned))
			if err != nil {
				t.Fatalf("parse do release %s: %v", release, err)
			}
			if file.Version != ExportVersion || file.AppVersion != release {
				t.Fatalf("metadados inesperados: %#v", file)
			}
			if len(unsupported) != 0 || len(file.Resources.Conversations) != 1 {
				t.Fatalf("fixture não reconhecida: unsupported=%v file=%#v", unsupported, file)
			}
		})
	}
}

func TestUnrelatedMetadataDoesNotSelectLegacyAdapter(t *testing.T) {
	raw := `{
		"version": 2,
		"metadata": {"type": "third-party", "version": "1"},
		"options": {},
		"resources": {}
	}`
	file, unsupported, err := parseExportFile(raw)
	if err != nil {
		t.Fatalf("metadata não relacionada bloqueou formato canônico: %v", err)
	}
	if file.Version != ExportVersion || len(unsupported) != 0 {
		t.Fatalf("parse canônico inesperado: file=%#v unsupported=%v", file, unsupported)
	}
}
