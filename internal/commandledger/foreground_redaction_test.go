package commandledger

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestReserveEnvelopePersistsOnlyForegroundSummary(t *testing.T) {
	req, now := envelopeRequest(t)
	store, db := testStore(t, &now)
	req.Envelope.ForegroundSnapshot = rawPtr(`{"executable":"editor.exe","window_class":"EditorWindow","provider_version":"windows-foreground.v1"}`)
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var audit invocationRow
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	var summary map[string]string
	if audit.ForegroundSummary == nil || json.Unmarshal([]byte(*audit.ForegroundSummary), &summary) != nil ||
		len(summary) != 3 || summary["executable"] != "editor.exe" || summary["window_class"] != "EditorWindow" || summary["provider_version"] != "windows-foreground.v1" {
		t.Fatalf("resumo seguro não persistiu: %+v", summary)
	}
}

func TestForegroundAuditRejectsUnknownOrSensitiveDocuments(t *testing.T) {
	if redactedForegroundIfPresent(nil) != nil {
		t.Fatal("ausência virou documento")
	}
	valid := `{"executable":"editor.exe","window_class":"EditorWindow","provider_version":"windows-foreground.v1"}`
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"title":"secret"}`, valid + `{}`,
		strings.Replace(valid, `"editor.exe"`, `"C:\\Users\\private\\editor.exe"`, 1),
		strings.Replace(valid, `"editor.exe"`, `"/private/editor.exe"`, 1),
		strings.Replace(valid, `"editor.exe"`, `"EDITOR.EXE"`, 1),
		strings.Replace(valid, `"editor.exe"`, `null`, 1),
		strings.Replace(valid, `"EditorWindow"`, `"secret\ntext"`, 1),
		strings.Replace(valid, `"EditorWindow"`, `"C:\\Users\\private\\secret"`, 1),
		strings.Replace(valid, `"EditorWindow"`, `"/private/secret"`, 1),
		strings.Replace(valid, `"EditorWindow"`, `"C:private.txt"`, 1),
		strings.Replace(valid, `"EditorWindow"`, `"Editor\u202eWindow"`, 1),
		strings.Replace(valid, `"editor.exe"`, `"editor\u202e.exe"`, 1),
		strings.Replace(valid, `"EditorWindow"`, `"`+strings.Repeat("x", 257)+`"`, 1),
		strings.Replace(valid, `windows-foreground.v1`, `unknown-provider`, 1),
		strings.TrimSuffix(valid, "}") + `,"title":"secret","path":"private"}`,
		strings.TrimSuffix(valid, "}") + `,"executable":"other.exe"}`,
	} {
		value := json.RawMessage(raw)
		got := redactedForegroundIfPresent(&value)
		if got == nil || *got != redactedDocument {
			t.Fatalf("documento inesperado não redigido: %s => %v", raw, got)
		}
	}
}

func TestForegroundAuditAllowsClassColonWithoutDrivePath(t *testing.T) {
	value := json.RawMessage(`{"executable":"editor.exe","window_class":"ATL:00012345","provider_version":"windows-foreground.v1"}`)
	got := redactedForegroundIfPresent(&value)
	if got == nil || *got == redactedDocument {
		t.Fatal("classe com dois-pontos sem caminho redigida")
	}
}
