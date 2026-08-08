package acpregistry

import (
	"slices"
	"testing"

	"assistente/internal/acp"
)

func TestDetectableKindMapeiaSoOsAgentesQueADeteccaoConhece(t *testing.T) {
	// A lista é curta por decisão (D1/D11): nenhum agente novo ganha detecção
	// própria, e é isso que faz a tela dizer "o app não sabe procurar" em vez de
	// "não encontrado" para os outros.
	if kind, ok := DetectableKind("cursor"); !ok || kind != acp.AgentKindCursor {
		t.Errorf("cursor = (%q, %v), quer o tipo do Cursor", kind, ok)
	}
	if kind, ok := DetectableKind("claude-acp"); !ok || kind != acp.AgentKindClaudeCode {
		t.Errorf("claude-acp = (%q, %v), quer o tipo do Claude Code", kind, ok)
	}
	if _, ok := DetectableKind("codex-acp"); ok {
		t.Error("agente sem detecção escrita à mão apareceu como detectável")
	}
	// `claude-code` foi tipo de provider enquanto cada agente tinha o seu, e
	// deixou de ser nome de coisa nenhuma (D11, emenda). Aceitá-lo aqui
	// ressuscitaria o vocabulário que a migração v12 apagou do banco.
	if _, ok := DetectableKind("claude-code"); ok {
		t.Error("o tipo de provider antigo foi aceito como agente")
	}
}

func TestDetectableKindsNaoRepeteEEstavel(t *testing.T) {
	kinds := DetectableKinds()
	if len(kinds) != 2 {
		t.Fatalf("agentes detectáveis = %v, quer dois", kinds)
	}
	if !slices.IsSorted(kinds) {
		t.Errorf("agentes detectáveis fora de ordem: %v", kinds)
	}
	if !slices.Equal(kinds, DetectableKinds()) {
		t.Error("a lista mudou entre duas chamadas")
	}
}

func TestADeteccaoUsaOsNomesDoRegistroENaoUmVocabularioProprio(t *testing.T) {
	// Enquanto os dois conjuntos de nomes existiam, uma tradução vivia entre
	// eles e precisava ser atualizada dos dois lados. Este teste é o que impede
	// que ela volte por descuido: se alguém renomear um AgentKind para algo que
	// não é `id` de registro, a detecção do catálogo para de casar em silêncio.
	for _, kind := range DetectableKinds() {
		if _, ok := DetectableKind(string(kind)); !ok {
			t.Errorf("o agente detectável %q não é reconhecido pelo próprio nome", kind)
		}
	}
}

// DetectableKinds devolve uma cópia: quem monta o catálogo itera sobre ela, e
// uma fatia compartilhada deixaria um chamador reordenar a lista de todos.
func TestDetectableKindsNaoEntregaAListaDeDentro(t *testing.T) {
	kinds := DetectableKinds()
	if len(kinds) == 0 {
		t.Fatal("sem agentes detectáveis")
	}
	kinds[0] = "outro"
	if slices.Contains(DetectableKinds(), acp.AgentKind("outro")) {
		t.Error("mexer no resultado mudou a lista de dentro")
	}
}
