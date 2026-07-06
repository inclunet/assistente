package app

import (
	"testing"

	"assistente/internal/nettrust"
)

// scopeFromOption deve parsear pelo valor ESTÁVEL do escopo (prefixo antes de
// scopeOptionSep), independente do rótulo humano — assim o copy pode mudar ou
// ganhar i18n sem quebrar o consentimento.
func TestScopeFromOption(t *testing.T) {
	cases := []struct {
		name   string
		option string
		want   nettrust.Scope
		ok     bool
	}{
		{"opção completa gerada", scopeOptionText(networkScopeOptions[1]), nettrust.ScopeSession, true},
		{"rótulo humano alterado", "session — Qualquer copy novo aqui", nettrust.ScopeSession, true},
		{"só o valor do escopo", "global", nettrust.ScopeGlobal, true},
		{"valor com espaços", "  workspace  ", nettrust.ScopeWorkspace, true},
		{"escopo desconhecido", "banana — algo", "", false},
		{"rótulo antigo sem prefixo", "Durante esta conversa", "", false},
		{"vazio", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scopeFromOption(tc.option)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("scopeFromOption(%q) = (%q, %v), want (%q, %v)", tc.option, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Toda option apresentada ao usuário deve ser parseável de volta ao seu escopo.
func TestScopeOptionsRoundTrip(t *testing.T) {
	for _, o := range networkScopeOptions {
		text := scopeOptionText(o)
		got, ok := scopeFromOption(text)
		if !ok || got != o.scope {
			t.Errorf("round-trip falhou para %q: (%q, %v), want %q", text, got, ok, o.scope)
		}
	}
}
