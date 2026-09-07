package slug

import "testing"

func TestSlugify(t *testing.T) {
	const fallback = "padrao"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Acentos e diacríticos.
		{"acento simples", "Padrão", "padrao"},
		{"cedilha", "Configuração", "configuracao"},
		{"varios acentos", "Á É Í Ó Ú ã õ ç", "a-e-i-o-u-a-o-c"},
		{"til e crase", "Não àÀ", "nao-aa"},

		// Maiúsculas.
		{"tudo maiusculo", "MODELO LOCAL", "modelo-local"},
		{"misto", "Modelo Local", "modelo-local"},

		// Espaços.
		{"espacos internos", "fetch jira tickets", "fetch-jira-tickets"},
		{"espacos multiplos", "a    b", "a-b"},
		{"espacos nas bordas", "  trim me  ", "trim-me"},
		{"tab e newline", "linha\tum\ndois", "linha-um-dois"},

		// Caracteres especiais.
		{"simbolos", "Olá, Mundo!", "ola-mundo"},
		{"pontuacao colapsa", "a---b___c", "a-b-c"},
		{"barra e ponto", "src/main.go", "src-main-go"},
		{"parenteses", "Perfil (Copia)", "perfil-copia"},
		{"hifens nas bordas", "--inicio-fim--", "inicio-fim"},

		// Já em formato slug (idempotência).
		{"ja slug", "fetch-jira-tickets", "fetch-jira-tickets"},

		// Dígitos preservados.
		{"com numeros", "Job 42 v2", "job-42-v2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Slugify(tc.input, fallback); got != tc.want {
				t.Errorf("Slugify(%q, %q) = %q, want %q", tc.input, fallback, got, tc.want)
			}
		})
	}
}

func TestSlugifyFallback(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{"vazio usa fallback", "", "perfil", "perfil"},
		{"so espacos usa fallback", "   ", "skill", "skill"},
		{"so simbolos usa fallback", "!@#$%", "padrao", "padrao"},
		{"so acentos isolados usa fallback", "\u0301\u0303", "padrao", "padrao"},
		{"fallback vazio preserva vazio", "", "", ""},
		{"so simbolos com fallback vazio", "***", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Slugify(tc.input, tc.fallback); got != tc.want {
				t.Errorf("Slugify(%q, %q) = %q, want %q", tc.input, tc.fallback, got, tc.want)
			}
		})
	}
}

func TestSlugifyIdempotente(t *testing.T) {
	inputs := []string{"Padrão", "Olá, Mundo!", "MODELO LOCAL", "fetch-jira-tickets"}
	for _, in := range inputs {
		once := Slugify(in, "fallback")
		twice := Slugify(once, "fallback")
		if once != twice {
			t.Errorf("Slugify não idempotente para %q: %q -> %q", in, once, twice)
		}
	}
}
