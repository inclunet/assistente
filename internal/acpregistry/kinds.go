package acpregistry

import (
	"slices"

	"assistente/internal/acp"
)

// knownKinds liga o tipo de provider do app ao `id` do agente no registro
// (AEP-0086 D11).
//
// Os dois conjuntos de identificadores foram escolhidos em momentos diferentes
// — o do app quando havia dois agentes escritos à mão, o do registro pelo
// catálogo —, e é por isso que `claude-code` e `claude-acp` são a mesma coisa
// com dois nomes. A tradução existe escrita num lugar só, e este é o lugar:
// espalhá-la por `switch` faria cada consumidor ter a própria versão dela, e
// duas versões da mesma tradução divergem no dia em que uma delas é atualizada.
//
// A lista é curta por decisão, e não por falta de trabalho. Ela nomeia os
// agentes que este app conhece por nome, e nenhum agente novo entra aqui a
// partir do registro (D1): para os outros o app não tem detecção escrita à mão,
// e é isso que a tela diz — em vez de alegar que procurou e não achou.
var knownKinds = []struct {
	Kind acp.AgentKind
	ID   string
}{
	{acp.AgentKindCursor, "cursor"},
	{acp.AgentKindClaudeCode, "claude-acp"},
}

// IDForKind devolve o `id` do registro de um tipo de provider do app.
//
// Vazio quer dizer que aquele tipo não tem correspondente no catálogo — o que
// não é erro: o formulário de comando e argumentos continua sendo o caminho de
// quem configura um agente à mão (D3).
func IDForKind(kind string) string {
	for _, known := range knownKinds {
		if string(known.Kind) == kind {
			return known.ID
		}
	}
	return ""
}

// DetectableKind devolve o tipo de agente que a detecção sabe procurar para
// aquele `id` do registro. O `false` quer dizer que este app não tem detecção
// para o agente — o que é diferente de ter procurado e não encontrado.
func DetectableKind(id string) (acp.AgentKind, bool) {
	for _, known := range knownKinds {
		if known.ID == id {
			return known.Kind, true
		}
	}
	return "", false
}

// DetectableKinds são os tipos que a detecção conhece, sem repetição. Quem monta
// o catálogo procura uma vez por tipo, e não uma vez por linha: a procura vai ao
// sistema de arquivos, e repeti-la por agente custaria 38 varreduras para
// responder sobre 2.
func DetectableKinds() []acp.AgentKind {
	kinds := make([]acp.AgentKind, 0, len(knownKinds))
	for _, known := range knownKinds {
		if !slices.Contains(kinds, known.Kind) {
			kinds = append(kinds, known.Kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}
