package app

import (
	"context"

	"assistente/internal/acp"
	"assistente/internal/database"
	"assistente/internal/logging"
)

// initACP cria o serviço que é dono dos processos e das sessões dos agentes de
// código (AEP-0084 D3). Nada sobe aqui: o processo de um provider ACP só nasce
// no primeiro uso — um turno, uma consulta de modelos, um health check.
func (a *App) initACP() {
	a.acpMgr = acp.NewManager(acp.ManagerConfig{
		// Sem banco o serviço funciona em memória e cada reinício começa com um
		// agente que não lembra da conversa. É o caso de um app ainda sem banco
		// aberto, não o normal.
		Store: acp.NewDBSessionStore(database.DB()),
		// Handler nulo nega todo pedido do agente na hora. Quem responde
		// permissão é a camada do D9, que ainda não existe; até lá negar é o
		// comportamento seguro, e nunca deixar um turno pendurado é regra.
		Handler:       nil,
		ClientName:    "assistente",
		ClientVersion: AppVersion,
	})
}

// closeACPSession encerra a sessão que o agente mantém para esta conversa. É o
// que limpar ou excluir a conversa precisa fazer: a memória do agente deixou de
// corresponder ao que a pessoa vê na tela, e uma sessão sem dono fica aberta no
// processo dele (AEP-0084 D4).
func (a *App) closeACPSession(ctx context.Context, conversationID string) {
	if a == nil || a.acpMgr == nil {
		return
	}
	if err := a.acpMgr.CloseConversation(ctx, conversationID); err != nil {
		logging.Warnf(ctx, "app.app-acp", "[ACP] erro ao encerrar a sessão da conversa %s: %v", conversationID, err)
	}
}
