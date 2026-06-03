package database

import (
	"strings"
	"testing"
)

func TestParseTaskListCustomActionsJSON_Valid(t *testing.T) {
	raw := `{"actions":[{"id":"investigate","label":"Investigar","surfaces":["card_menu","card_detail"],"event":"tasklist.card.investigate"}]}`
	ca, err := ParseTaskListCustomActionsJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ca.Actions) != 1 || ca.Actions[0].ID != "investigate" {
		t.Fatalf("unexpected parse result: %#v", ca)
	}
	if !ca.Actions[0].HasSurface("card_detail") {
		t.Fatal("expected card_detail surface")
	}
}

func TestParseTaskListCustomActionsJSON_DefaultSurfaceIsCardMenu(t *testing.T) {
	ca, err := ParseTaskListCustomActionsJSON(`{"actions":[{"id":"a","label":"A","link":"https://x"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ca.Actions[0].HasSurface(CustomActionSurfaceCardMenu) {
		t.Fatal("no surfaces declared should default to card_menu")
	}
	if ca.Actions[0].HasSurface(CustomActionSurfaceBoardMenu) {
		t.Fatal("should not be visible on board_menu by default")
	}
}

func TestParseTaskListCustomActionsJSON_Invalid(t *testing.T) {
	cases := map[string]string{
		"no id":               `{"actions":[{"label":"X","event":"e"}]}`,
		"no label":            `{"actions":[{"id":"x","event":"e"}]}`,
		"no event nor link":   `{"actions":[{"id":"x","label":"X"}]}`,
		"dup id":              `{"actions":[{"id":"x","label":"X","event":"e"},{"id":"x","label":"Y","link":"l"}]}`,
		"bad surface":         `{"actions":[{"id":"x","label":"X","event":"e","surfaces":["nope"]}]}`,
		"bad json":            `{not json`,
		"id with space":       `{"actions":[{"id":"my action","label":"X","event":"e"}]}`,
		"id trailing space":   `{"actions":[{"id":"open ","label":"X","event":"e"}]}`,
		"id leading space":    `{"actions":[{"id":" open","label":"X","event":"e"}]}`,
		"id with tab":         `{"actions":[{"id":"op\ten","label":"X","event":"e"}]}`,
		"id with slash":       `{"actions":[{"id":"a/b","label":"X","event":"e"}]}`,
		"id with backslash":   `{"actions":[{"id":"a\\b","label":"X","event":"e"}]}`,
		"event with space":    `{"actions":[{"id":"x","label":"X","event":"tasklist.card.foo "}]}`,
		"event inner space":   `{"actions":[{"id":"x","label":"X","event":"tasklist.card foo"}]}`,
		"payload sem event":   `{"actions":[{"id":"x","label":"X","link":"https://x","payload_template":"{\"a\":1}"}]}`,
		"link only spaces":    `{"actions":[{"id":"x","label":"X","event":"e","link":"   "}]}`,
		"unknown field alias": `{"actions":[{"id":"x","label":"X","emits_event":"e"}]}`,
		"unknown field when":  `{"actions":[{"id":"x","label":"X","event":"e","enabled_when":"true"}]}`,
		"payload as object":   `{"actions":[{"id":"x","label":"X","event":"e","payload_template":{"a":1}}]}`,
		"unknown top-level":   `{"actions":[],"version":2}`,
		"trailing object":     `{"actions":[]}{"actions":[]}`,
		"trailing garbage":    `{"actions":[{"id":"x","label":"X","event":"e"}]} extra`,
	}
	for name, raw := range cases {
		if _, err := ParseTaskListCustomActionsJSON(raw); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestParseTaskListCustomActionsJSON_UnknownFieldMessage(t *testing.T) {
	_, err := ParseTaskListCustomActionsJSON(`{"actions":[{"id":"x","label":"X","emits_event":"e"}]}`)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	msg := err.Error()
	if !strings.Contains(msg, "emits_event") || !strings.Contains(msg, "campo desconhecido") {
		t.Fatalf("expected friendly unknown-field message naming emits_event, got: %q", msg)
	}
}

func TestSetLoadCustomActionsRoundTrip(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")

	raw := `{"actions":[{"id":"open","label":"Abrir","surfaces":["card_menu"],"link":"{{ .task.link }}"}]}`
	if err := SetTaskListCustomActionsWithContext(ctx, tl.ID, raw); err != nil {
		t.Fatalf("set: %v", err)
	}
	ca, err := LoadTaskListCustomActionsWithContext(ctx, tl.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(ca.Actions) != 1 || ca.Actions[0].ID != "open" || ca.Actions[0].Link != "{{ .task.link }}" {
		t.Fatalf("round trip mismatch: %#v", ca)
	}

	// String vazia limpa as ações.
	if err := SetTaskListCustomActionsWithContext(ctx, tl.ID, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	ca, _ = LoadTaskListCustomActionsWithContext(ctx, tl.ID)
	if len(ca.Actions) != 0 {
		t.Fatalf("expected no actions after clear, got %#v", ca)
	}
}

func TestSetCustomActions_InvalidRejected(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err := SetTaskListCustomActionsWithContext(ctx, tl.ID, `{"actions":[{"id":"x","label":"X"}]}`); err == nil {
		t.Fatal("expected rejection (no event nor link)")
	}
}
