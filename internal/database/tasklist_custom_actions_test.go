package database

import "testing"

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
		"no id":             `{"actions":[{"label":"X","event":"e"}]}`,
		"no label":          `{"actions":[{"id":"x","event":"e"}]}`,
		"no event nor link": `{"actions":[{"id":"x","label":"X"}]}`,
		"dup id":            `{"actions":[{"id":"x","label":"X","event":"e"},{"id":"x","label":"Y","link":"l"}]}`,
		"bad surface":       `{"actions":[{"id":"x","label":"X","event":"e","surfaces":["nope"]}]}`,
		"bad json":          `{not json`,
	}
	for name, raw := range cases {
		if _, err := ParseTaskListCustomActionsJSON(raw); err == nil {
			t.Fatalf("%s: expected error", name)
		}
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
