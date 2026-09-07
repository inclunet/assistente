package acp

import (
	"errors"
	"strings"
	"testing"
)

func modelosOferecidos(values ...ConfigValue) []ConfigOption {
	return []ConfigOption{{ID: "model", Category: CategoryModel, Values: values}}
}

// O identificador de um modelo de agente é do protocolo; o nome é o que a
// pessoa reconhece. Quem lista precisa dos dois (AEP-0084, Fase 8).
func TestOModeloOferecidoVemComValorENome(t *testing.T) {
	choices := ModelChoices(modelosOferecidos(
		ConfigValue{Value: "grok-4.5[max]", Name: "Grok 4.5 (max)"},
		ConfigValue{Value: "  gpt-5  ", Name: ""},
		ConfigValue{Value: "   "},
	))

	if len(choices) != 2 {
		t.Fatalf("modelos = %+v, esperado dois (o valor em branco não conta)", choices)
	}
	if choices[0].Value != "grok-4.5[max]" || choices[0].Name != "Grok 4.5 (max)" {
		t.Errorf("primeiro modelo = %+v", choices[0])
	}
	// Valor aparado, como em ModelValues: é aparado que ele chega à escolha da
	// pessoa e volta ao agente.
	if choices[1].Value != "gpt-5" || choices[1].Name != "" {
		t.Errorf("segundo modelo = %+v", choices[1])
	}
}

// Nome vem do agente, que é fonte não confiável (D11): ele é exibido na tela e
// lido em voz alta.
func TestNomeDoModeloVemSaneado(t *testing.T) {
	choices := ModelChoices(modelosOferecidos(
		ConfigValue{Value: "m1", Name: "\x1b[31mGrok\x1b[0m\n4.5"},
	))

	if len(choices) != 1 {
		t.Fatalf("modelos = %+v", choices)
	}
	if strings.ContainsAny(choices[0].Name, "\x1b\n") {
		t.Fatalf("nome = %q, esperado sem escape de terminal nem quebra de linha", choices[0].Name)
	}
}

func TestAgenteSemEscolhaDeModeloNaoOfereceNada(t *testing.T) {
	if choices := ModelChoices(nil); len(choices) != 0 {
		t.Fatalf("modelos = %+v, esperado nenhum", choices)
	}
	if valores := ModelValues(nil); len(valores) != 0 {
		t.Fatalf("valores = %+v, esperado nenhum", valores)
	}
}

// Quem só queria listar modelos precisa distinguir "o agente não subiu" de "o
// agente respondeu isso": o primeiro se resolve na tela de provedores, e não
// adianta tentar de novo antes disso.
func TestAgenteQueNaoSobeSaiComErroReconhecivel(t *testing.T) {
	falha := errors.New(`iniciar agente cursor-agent: executable file not found in %PATH%`)
	m := NewManager(ManagerConfig{
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return nil, falha
		},
	})
	t.Cleanup(m.Shutdown)

	_, err := m.Client(testSpec())
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("erro = %v, esperado marcado como agente indisponível", err)
	}
	// O detalhe continua no caminho: é ele que o log guarda e a sonda exibe.
	if !errors.Is(err, falha) {
		t.Fatalf("erro = %v, esperado carregar o motivo original", err)
	}
}

// Falta de login já tem instrução própria e resolve-se fora do app, no CLI do
// agente (D12). Marcá-la como "não subiu" mandaria conferir comando e caminho
// que estão certos.
func TestFaltaDeLoginNaoViraAgenteIndisponivel(t *testing.T) {
	m := NewManager(ManagerConfig{
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return nil, ErrNotAuthenticated
		},
	})
	t.Cleanup(m.Shutdown)

	_, err := m.Client(testSpec())
	if errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("erro = %v, esperado seguir sendo falta de login", err)
	}
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("erro = %v, esperado falta de login", err)
	}
}
