package app

import (
	"testing"

	"assistente/internal/fstrust"
	"assistente/internal/questionnaire"
)

func TestFsScopeFromActionID(t *testing.T) {
	tests := []struct {
		id      string
		wantOK  bool
		scope   fstrust.Scope
		kind    fstrust.Kind
	}{
		{"once", true, fstrust.ScopeOnce, fstrust.KindFile},
		{"dir-session", true, fstrust.ScopeSession, fstrust.KindDir},
		{"dir-global", true, fstrust.ScopeGlobal, fstrust.KindDir},
		{"deny", false, "", ""},
		{"", false, "", ""},
		{"dir-nope", false, "", ""},
	}
	for _, tt := range tests {
		scope, kind, ok := fsScopeFromActionID(tt.id)
		if ok != tt.wantOK {
			t.Fatalf("%q: ok=%v want %v", tt.id, ok, tt.wantOK)
		}
		if !tt.wantOK {
			continue
		}
		if scope != tt.scope || kind != tt.kind {
			t.Fatalf("%q: got (%s,%s) want (%s,%s)", tt.id, scope, kind, tt.scope, tt.kind)
		}
	}
}

func TestPathConfirmationPayloadIsDecision(t *testing.T) {
	payload := pathConfirmationPayload(fstrust.PromptRequest{
		Path:      "C:/tmp/a.txt",
		Operation: "read",
	})
	if payload.Kind != questionnaire.KindDecision {
		t.Fatalf("kind=%q want decision", payload.Kind)
	}
	if len(payload.Actions) < 3 {
		t.Fatalf("actions=%d; esperava escopos file+dir+deny", len(payload.Actions))
	}
	last := payload.Actions[len(payload.Actions)-1]
	if last.ID != fsActionDenyPrefix {
		t.Fatalf("última ação=%q want deny", last.ID)
	}
}
