package agent

import "assistente/internal/logging"

// OnAgentTitle aplica o nome que o agente de código deu à sessão dele
// (AEP-0084 D8). Ele nomeia o que foi pedido — "Corrigir o teste de anexos" —
// enquanto o app, sozinho, só consegue recortar o começo da primeira mensagem.
//
// Quem decide se o nome novo pode valer é o interactor, junto da regra de
// renomear que já existe: título escolhido por alguém não é substituído por um
// gerado.
func (h *SimpleStreamHandler) OnAgentTitle(title string) {
	title = singleLine(title)
	if title == "" || h.svc == nil || h.svc.renameFromAgent == nil {
		return
	}
	if err := h.svc.renameFromAgent(h.ctx, h.ConversationID, h.userMessageID, title); err != nil {
		// Não renomear é perder uma conveniência, não o turno: a resposta do
		// agente já está salva e a conversa segue com o rótulo que tinha.
		logging.Warnf(h.ctx, "agent.agent-title",
			"[ACP] o título do agente não pôde ser aplicado à conversa %s: %v", h.ConversationID, err)
	}
}
