package app

import (
	"context"
	"fmt"

	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

// Re-exporta tipos do pacote database para manter compatibilidade (CLI).
type Conversation = database.Conversation
type ConversationListResult = database.ConversationListResult
type ChatMessage = database.ChatMessage
type MessageSearchResult = database.MessageSearchResult

// Re-exporta funções que não dependem de App
var (
	InitDatabase  = database.Init
	GenerateTitle = database.GenerateTitle
)

func (a *App) resetLoadedToolsForConversation(conversationID string) {
	if a == nil || a.chatCtrl == nil {
		return
	}
	a.chatCtrl.ResetLoadedToolsForConversation(conversationID)
}

// resetConversationScopedState limpa o estado efêmero amarrado a uma conversa
// (tools carregadas, allowlist de rede do escopo de sessão e a sessão do agente
// ACP) quando ela é criada/reciclada, limpa ou excluída — evitando que um novo
// chat que reutilize o mesmo ConversationID herde estado da sessão anterior sem
// novo consentimento.
func (a *App) resetConversationScopedState(ctx context.Context, conversationID string) {
	a.resetLoadedToolsForConversation(conversationID)
	if a != nil && a.netTrustMgr != nil {
		a.netTrustMgr.ClearSession(conversationID)
	}
	// A sessão do agente é onde vive o histórico dessa conversa do lado dele
	// (AEP-0084 D4): mantê-la faria o agente responder com base em mensagens
	// que a pessoa já não vê na tela.
	a.closeACPSession(ctx, conversationID)
}

func (a *App) confirmDeleteMessageQuestionnaire() error {
	if a == nil || a.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}
	resp, err := a.questionnaireMgr.RequestQuestionnaire(a.ctx, questionnaire.RequestPayload{
		Title:       questionnaire.Plain("Excluir mensagem"),
		Description: questionnaire.Plain("Tem certeza que deseja excluir esta mensagem e todas as suas respostas? Esta ação não pode ser desfeita."),
		AllowCancel: true,
		SubmitLabel: questionnaire.Plain("Excluir"),
		CancelLabel: questionnaire.Plain("Cancelar"),
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   questionnaire.Plain("Confirmar exclusão?"),
				Required: true,
			},
		},
	})
	if err != nil {
		return err
	}
	if resp.Cancelled {
		return fmt.Errorf("exclusão cancelada pelo usuário")
	}
	confirmed, ok := resp.Answers["confirm"].(bool)
	if !ok || !confirmed {
		return fmt.Errorf("exclusão cancelada pelo usuário")
	}
	return nil
}

func (a *App) effectiveModelFromActiveProfile() (string, error) {
	if a == nil || a.profileManager == nil {
		return "", nil
	}
	activeProfile, err := a.profileManager.GetActive()
	if err != nil {
		return "", err
	}
	if activeProfile == nil {
		return "", nil
	}
	return activeProfile.Chat.Model, nil
}
