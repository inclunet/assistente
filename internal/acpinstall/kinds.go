package acpinstall

import "assistente/internal/acp"

// registryIDByAgentKind liga o tipo de provider do app ao `id` do agente no
// registro (AEP-0086 D11).
//
// Ele existe porque os dois conjuntos de identificadores foram escolhidos em
// momentos diferentes — o do app quando havia dois agentes escritos à mão, o do
// registro pelo catálogo —, e existe **escrito num lugar só**, e não espalhado
// por `switch`. Quem acrescentar tipo de provider acrescenta uma linha aqui.
var registryIDByAgentKind = map[acp.AgentKind]string{
	acp.AgentKindCursor:     "cursor",
	acp.AgentKindClaudeCode: "claude-acp",
}

// RegistryIDForKind devolve o `id` do registro de um tipo de provider do app.
//
// Vazio quer dizer que aquele tipo não tem correspondente no catálogo — o que
// não é erro: o formulário de comando e argumentos continua sendo o caminho de
// quem configura um agente à mão (D3).
func RegistryIDForKind(kind string) string {
	return registryIDByAgentKind[acp.AgentKind(kind)]
}
