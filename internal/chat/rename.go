package chat

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/core/ports"
	"assistente/internal/logging"
)

const renameComponent = "chat.rename"

// DefaultConversationTitle é o nome que uma conversa recebe ao nascer, antes de
// qualquer mensagem.
const DefaultConversationTitle = "Nova Conversa"

// maxAutomaticTitle é o recorte da mensagem que vira rótulo provisório.
const maxAutomaticTitle = 50

// shortenTitle corta o título do agente no mesmo teto do rótulo provisório: ele
// vive em aba e em lista, e uma frase inteira estouraria as duas. O corte é por
// runa, e não por byte, porque um título em português tem acento e partir um
// caractere ao meio deixaria lixo no fim do nome.
func shortenTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxAutomaticTitle {
		return title
	}
	return strings.TrimRight(string(runes[:maxAutomaticTitle-1]), " ") + "…"
}

// automaticTitle é o rótulo provisório que o app dá à conversa na primeira
// mensagem: o começo do que a pessoa escreveu. Não é um nome escolhido — é o que
// existe até alguém (ou algo) dar um melhor.
func automaticTitle(content string) string {
	if len(content) > maxAutomaticTitle {
		return content[:maxAutomaticTitle]
	}
	return content
}

// RenameFromAgent troca o título da conversa pelo que o agente de código gerou
// para a sessão dele (AEP-0084 D8, session_info_update).
//
// A regra é não apagar decisão de ninguém: o título do agente substitui o padrão
// e o rótulo provisório que o app tirou da primeira mensagem, e mais nada. Título
// diferente desses dois foi escolhido — pela pessoa, na tela — e sobrescrevê-lo
// seria trocar um nome deliberado por um gerado.
//
// turnMessageID é a mensagem do usuário deste turno, usada para reconhecer o
// rótulo provisório. Ele só pode ser o desta conversa; ausente ou de outra
// conversa, sobra reconhecer o título padrão.
func (i *Interactor) RenameFromAgent(ctx context.Context, conversationID, turnMessageID, title string) error {
	conversationID = strings.TrimSpace(conversationID)
	title = strings.TrimSpace(title)
	if conversationID == "" || title == "" {
		return nil
	}
	if i.convRepo == nil {
		return errors.New("sem repositório de conversas para renomear")
	}
	title = shortenTitle(title)

	conv, err := i.convRepo.GetConversationInfo(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return nil
	}
	if conv.Title == title {
		return nil
	}
	if !i.titleIsAutomatic(ctx, conversationID, turnMessageID, conv.Title) {
		logging.Debugf(ctx, renameComponent,
			"[ACP] título do agente ignorado na conversa %s: o nome atual foi escolhido", conversationID)
		return nil
	}

	if err := i.convRepo.UpdateConversation(ctx, conversationID, title, ""); err != nil {
		return err
	}
	i.emitter.Emit("conversation:renamed", ports.ConversationRenamedEvent{
		ConversationID: conversationID,
		NewTitle:       title,
	})
	return nil
}

// titleIsAutomatic diz se o título atual é um dos que o app escreveu sozinho: o
// padrão de conversa nova ou o recorte da mensagem deste turno.
func (i *Interactor) titleIsAutomatic(ctx context.Context, conversationID, turnMessageID, current string) bool {
	if current == DefaultConversationTitle || strings.TrimSpace(current) == "" {
		return true
	}
	turnMessageID = strings.TrimSpace(turnMessageID)
	if turnMessageID == "" || i.repo == nil {
		return false
	}
	message, err := i.repo.GetMessage(ctx, turnMessageID)
	if err != nil || message == nil {
		return false
	}
	if message.ConversationID != conversationID {
		return false
	}
	return current == automaticTitle(message.Content)
}
