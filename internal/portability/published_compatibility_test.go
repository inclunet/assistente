package portability

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"assistente/internal/database"

	"github.com/google/uuid"
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
	if file.AppVersion != "0.1.9" {
		t.Fatalf("appVersion legado = %q, esperado 0.1.9", file.AppVersion)
	}
	for _, id := range []string{
		conversation.ID,
		conversation.Messages[0].ID,
		conversation.Messages[1].ID,
	} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.Version() != 7 {
			t.Fatalf("ID adaptado não é UUIDv7: %q (%v)", id, err)
		}
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

func TestLegacyExportConversationIDDoesNotDependOnEditableTitle(t *testing.T) {
	raw, err := os.ReadFile("testdata/published/0.1.9-conversations.json")
	if err != nil {
		t.Fatal(err)
	}
	original, _, err := parseExportFile(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	editedJSON := strings.Replace(
		string(raw),
		"Fixture sem dados pessoais",
		"Título editado antes do import",
		1,
	)
	edited, _, err := parseExportFile(editedJSON)
	if err != nil {
		t.Fatal(err)
	}
	if original.Resources.Conversations[0].ID != edited.Resources.Conversations[0].ID {
		t.Fatal("editar o título alterou o ID determinístico da conversa")
	}
}

func TestLegacyExportRejectsDanglingMessageLinks(t *testing.T) {
	raw, err := os.ReadFile("testdata/published/0.1.9-conversations.json")
	if err != nil {
		t.Fatal(err)
	}
	corrupted := strings.Replace(string(raw), `"parentId": 41`, `"parentId": 999`, 1)
	if _, _, err := parseExportFile(corrupted); err == nil ||
		!strings.Contains(err.Error(), "parentId inexistente 999") {
		t.Fatalf("referência órfã deveria falhar claramente, recebeu: %v", err)
	}
}

func TestLegacyExportRequiresArrayAndExportTimestamp(t *testing.T) {
	for name, conversations := range map[string]string{
		"null":   "null",
		"object": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"metadata":{"version":"2.0","type":"conversations","exported_at":"2026-03-18T12:00:00Z"},"conversations":` + conversations + `}`
			if _, matched, err := parseLegacyConversationsExport([]byte(raw)); !matched || err == nil {
				t.Fatalf("envelope inválido deveria ser reconhecido e rejeitado: matched=%v err=%v", matched, err)
			}
		})
	}

	raw := `{"metadata":{"version":"2.0","type":"conversations"},"conversations":[]}`
	if _, matched, err := parseLegacyConversationsExport([]byte(raw)); !matched ||
		err == nil || !strings.Contains(err.Error(), "metadata.exported_at") {
		t.Fatalf("timestamp ausente deveria falhar: matched=%v err=%v", matched, err)
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
