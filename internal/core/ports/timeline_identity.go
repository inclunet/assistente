package ports

import "strings"

// MessageItemID identifica uma mensagem independente na timeline (AEP-0116).
// IDs persistidos são opacos; a conversa/owner continuam sendo o escopo de acesso.
func MessageItemID(messageID string) string {
	if messageID == "" {
		return ""
	}
	return "message:" + messageID
}

// ResponseItemID identifica a resposta pela pergunta, nunca pela mensagem
// representante. Eventos operacionais sem turno não identificam um item.
func ResponseItemID(turnID string) string {
	if strings.TrimSpace(turnID) == "" {
		return ""
	}
	return "turn:" + turnID
}
