package acpregistry

import (
	"slices"
	"testing"

	"assistente/internal/acp"
)

func TestDetectableKindMapeiaSoOsAgentesQueADeteccaoConhece(t *testing.T) {
	// O mapeamento é curto por decisão (D1/D11): nenhum agente novo ganha
	// detecção própria, e é isso que faz a tela dizer "o app não sabe procurar"
	// em vez de "não encontrado" para os outros.
	if kind, ok := DetectableKind("cursor"); !ok || kind != acp.AgentKindCursor {
		t.Errorf("cursor = (%q, %v), quer o tipo do Cursor", kind, ok)
	}
	if kind, ok := DetectableKind("claude-acp"); !ok || kind != acp.AgentKindClaudeCode {
		t.Errorf("claude-acp = (%q, %v), quer o tipo do Claude Code", kind, ok)
	}
	// O `id` do registro e o tipo de provider do app são vocabulários
	// diferentes: `claude-code` é tipo de provider, e não linha do índice.
	if _, ok := DetectableKind("claude-code"); ok {
		t.Error("o tipo de provider do app foi aceito como id do registro")
	}
	if _, ok := DetectableKind("codex-acp"); ok {
		t.Error("agente sem detecção escrita à mão apareceu como detectável")
	}
}

func TestDetectableKindsNaoRepeteEEstavel(t *testing.T) {
	kinds := DetectableKinds()
	if len(kinds) != 2 {
		t.Fatalf("tipos detectáveis = %v, quer dois", kinds)
	}
	if !slices.IsSorted(kinds) {
		t.Errorf("tipos detectáveis fora de ordem: %v", kinds)
	}
	if slices.Equal(kinds, DetectableKinds()) == false {
		t.Error("a lista mudou entre duas chamadas")
	}
}

func TestIDForKindTraduzOTipoDoAppParaOIdDoRegistro(t *testing.T) {
	if id := IDForKind("cursor"); id != "cursor" {
		t.Errorf("cursor virou %q, queria cursor", id)
	}
	if id := IDForKind("claude-code"); id != "claude-acp" {
		t.Errorf("claude-code virou %q, queria claude-acp", id)
	}
	// Provedor HTTP não é agente, e tipo sem correspondente no catálogo não é
	// erro: configurar comando e argumentos à mão continua sendo caminho válido.
	if id := IDForKind("openai"); id != "" {
		t.Errorf("um provedor HTTP virou o agente %q", id)
	}
	// O `id` do registro não é tipo de provider, e aceitá-lo aqui faria a
	// tradução responder a um vocabulário que não é o dela.
	if id := IDForKind("claude-acp"); id != "" {
		t.Errorf("o id do registro foi aceito como tipo de provider e virou %q", id)
	}
}

func TestATraducaoVaiEVoltaSemPerderNinguem(t *testing.T) {
	// As duas leituras saem da mesma lista, e é isso que impede que uma seja
	// atualizada sem a outra — o motivo de a tradução existir num lugar só.
	for _, kind := range DetectableKinds() {
		id := IDForKind(string(kind))
		if id == "" {
			t.Errorf("o tipo detectável %q não tem id no registro", kind)
			continue
		}
		volta, ok := DetectableKind(id)
		if !ok || volta != kind {
			t.Errorf("%q virou %q e voltou (%q, %v)", kind, id, volta, ok)
		}
	}
}
