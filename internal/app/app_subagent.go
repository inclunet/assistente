package app

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/database"
	"assistente/internal/subagent"
)

// subagentParentDelivery entrega o aviso de conclusão de um sub-agente em
// background na conversa-pai e re-dispara o loop do pai (auto-wake), AEP-0068.
//
// IMPORTANTE (AEP-0040): NÃO cria fluxo alternativo de envio. O auto-wake reusa
// integralmente a MESMA SendMessageUseCase (via ChatController.SendForSubagent),
// injetando o resultado do sub-agente como um turno de origem "subagent" na
// conversa-pai. Isso (a) torna o resultado visível na conversa do pai e (b)
// re-dispara o loop do pai para reagir, num único caminho oficial. O ctx já
// carrega a proveniência (eventctx) carimbada pelo Manager.
type subagentParentDelivery struct {
	app *App
}

func (d *subagentParentDelivery) Deliver(ctx context.Context, n subagent.ParentNotice) error {
	if d == nil || d.app == nil || d.app.chatCtrl == nil {
		return fmt.Errorf("subagent delivery indisponível")
	}
	content := buildSubagentNotice(n)
	// Profile vazio → o pipeline resolve o profile do pai/ativo. Model vazio →
	// derivado do profile. Source="subagent" é definido por SendForSubagent.
	_, err := d.app.chatCtrl.SendForSubagent(ctx, n.ParentConversationID, content, "", "", "")
	return err
}

// subagentConversationLister implementa subagent.ConversationLister sobre o
// pacote database (AEP-0068 F5), convertendo o tipo local do banco para o DTO
// do pacote subagent. Mantém o pacote subagent livre de detalhes de consulta.
type subagentConversationLister struct {
	app *App
}

func (l *subagentConversationLister) ListSubAgentConversations(ctx context.Context) ([]subagent.SubConversationMeta, error) {
	rows, err := database.ListSubAgentConversationsWithContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]subagent.SubConversationMeta, 0, len(rows))
	for _, r := range rows {
		out = append(out, subagent.SubConversationMeta{
			ConversationID:       r.ConversationID,
			Title:                r.Title,
			ParentConversationID: r.ParentConversationID,
			CreatedAt:            r.CreatedAt,
			UpdatedAt:            r.UpdatedAt,
			MessageCount:         r.MessageCount,
			PromptTokens:         r.PromptTokens,
			CompletionTokens:     r.CompletionTokens,
			TotalTokens:          r.TotalTokens,
		})
	}
	return out, nil
}

// GetSubAgentConversations lista as sub-conversas de sub-agentes do usuário
// autenticado (AEP-0068 F5): identidade, vínculo com o pai, status do run mais
// recente, contagem de runs e custo (tokens). Binding Wails para a UI — NÃO é
// uma tool exposta ao LLM.
func (a *App) GetSubAgentConversations() ([]subagent.SubConversationSummary, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if a.subagentMgr == nil {
		return []subagent.SubConversationSummary{}, nil
	}
	return a.subagentMgr.ListSubConversations(ctx)
}

// buildSubagentNotice monta o conteúdo do aviso entregue ao pai.
func buildSubagentNotice(n subagent.ParentNotice) string {
	var b strings.Builder
	switch n.Status {
	case subagent.StatusSucceeded:
		b.WriteString("[Sub-agente concluído]")
	case subagent.StatusFailed:
		b.WriteString("[Sub-agente falhou]")
	case subagent.StatusTimedOut:
		b.WriteString("[Sub-agente expirou]")
	case subagent.StatusCancelled:
		b.WriteString("[Sub-agente cancelado]")
	default:
		b.WriteString("[Sub-agente]")
	}
	fmt.Fprintf(&b, " sub-conversa %s, run %s.\n", n.ChildConversationID, n.RunID)
	if strings.TrimSpace(n.Summary) != "" {
		b.WriteString("\nResultado:\n")
		b.WriteString(n.Summary)
		b.WriteString("\n")
	}
	if strings.TrimSpace(n.Error) != "" {
		b.WriteString("\nErro: ")
		b.WriteString(n.Error)
		b.WriteString("\n")
	}
	b.WriteString("\nReaja conforme necessário com base neste resultado.")
	return b.String()
}
