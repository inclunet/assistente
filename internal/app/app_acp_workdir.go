package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// GetAgentConversationWorkDir e SetAgentConversationWorkDir migraram para
// wailsapi.ACPWorkDir (AEP-0088). Helpers lowercase abaixo permanecem no App —
// usados por install/providers/runtime (ConversationDir do Manager).

// agentConversationDir é o que o serviço de agentes consulta para saber onde
// pôr o agente de uma conversa. Roda no caminho do turno, então lê o registro e
// nada mais.
//
// Erro é erro, e não "use o workspace": o manager trata caminho vazio como
// consentimento para o diretório do app, e engolir uma falha de banco aqui poria
// o agente a editar uma árvore que ninguém escolheu para esta conversa.
func (a *App) agentConversationDir(conversationID string) (string, error) {
	if a == nil {
		return "", nil
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return "", fmt.Errorf("sem sessão para ler o diretório do agente: %w", err)
	}
	return a.conversationAgentDir(ctx, conversationID)
}

func (a *App) conversationAgentDir(ctx context.Context, conversationID string) (string, error) {
	conv, err := database.GetConversationInfoWithContext(ctx, conversationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Conversa que ainda não está no banco não escolheu diretório nenhum, e
		// tratar isso como falha impediria o primeiro turno de uma conversa que
		// nasce junto com ele. Só este caso é silencioso: qualquer outro erro é
		// não saber a resposta, e não sabê-la é diferente de saber que não há.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(conv.AgentWorkDir), nil
}
