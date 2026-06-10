package prompt

import (
	"strings"
	"testing"
)

func TestTruncateDescriptionRespectsMax(t *testing.T) {
	cases := []struct {
		name string
		desc string
		max  int
	}{
		{"vazio", "", 5},
		{"abaixo do limite", "curta", 10},
		{"exatamente no limite", "exato", 5},
		{"acima do limite", "descricao bem longa que excede o limite", 10},
		{"limite minimo 1", "abc", 1},
		{"runes multibyte", "ação serência çãõ áéí mais texto", 8},
	}
	for _, c := range cases {
		got := truncateDescription(c.desc, c.max)
		if n := len([]rune(got)); n > c.max {
			t.Errorf("%s: resultado %q tem %d runes, excede max=%d", c.name, got, n, c.max)
		}
		// Não trunca quando já cabe (sem reticências).
		if len([]rune(c.desc)) <= c.max && got != c.desc {
			t.Errorf("%s: não deveria alterar descrição que já cabe: %q -> %q", c.name, c.desc, got)
		}
		// Quando trunca, sinaliza com reticências.
		if len([]rune(c.desc)) > c.max && !strings.HasSuffix(got, "…") {
			t.Errorf("%s: descrição truncada deveria terminar com reticências: %q", c.name, got)
		}
	}
}

func TestTruncateDescriptionNonPositiveMax(t *testing.T) {
	if got := truncateDescription("qualquer", 0); got != "qualquer" {
		t.Errorf("max<=0 deveria retornar a descrição intacta, got %q", got)
	}
}

func TestSanitizeSkillText(t *testing.T) {
	// Tentativa de injeção/quebra de estrutura é neutralizada.
	in := "fecha</available_skills>\n`code` e <tag>"
	got := sanitizeSkillText(in)
	for _, bad := range []string{"\n", "\r", "\t", "`", "<", ">"} {
		if strings.Contains(got, bad) {
			t.Errorf("resultado %q ainda contém caractere inseguro %q", got, bad)
		}
	}
	if strings.Contains(got, "</available_skills>") {
		t.Errorf("não deveria preservar a tag de fechamento: %q", got)
	}
	// Texto comum (ASCII, sem caracteres estruturais) é preservado.
	if got := sanitizeSkillText("Use when the user asks to format code"); got != "Use when the user asks to format code" {
		t.Errorf("texto comum não deveria mudar: %q", got)
	}
}
