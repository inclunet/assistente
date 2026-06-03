package app

import (
	"context"
	"fmt"
	"strings"

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
