package app

import (
	"context"
	"os"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/questionnaire"
)

// initACP cria o serviço que é dono dos processos e das sessões dos agentes de
// código (AEP-0084 D3). Nada sobe aqui: o processo de um provider ACP só nasce
// no primeiro uso — um turno, uma consulta de modelos, um health check.
func (a *App) initACP() {
	// O handler pergunta ao serviço quem espera o turno, e o serviço precisa
	// do handler para nascer. Ele é preenchido logo abaixo: nenhum pedido do
	// agente chega antes disso, porque o primeiro processo só sobe no primeiro
	// turno.
	handler := &acpRequestHandler{
		questions: func() *questionnaire.Manager { return a.questionnaireMgr },
		notices:   func() ports.Emitter { return a.emitter },
	}
	a.acpMgr = acp.NewManager(acp.ManagerConfig{
		// O banco é buscado a cada uso, não guardado: resetá-lo fecha a conexão
		// e abre outra, e uma conexão guardada aqui ficaria apontando para a
		// fechada até o app reiniciar.
		Store:         acp.NewDBSessionStore(database.DB),
		Handler:       handler,
		WorkDir:       a.acpWorkDir,
		ClientName:    "assistente",
		ClientVersion: AppVersion,
	})
	handler.owner = a.acpMgr.TurnOwnerOf
}

// acpWorkDir é o diretório sobre o qual o agente age (AEP-0084 D5): o workspace
// ativo, o mesmo que o terminal e a allowlist de rede seguem. Ele muda em
// runtime sem mexer no cwd do processo, e usar o cwd cru faria o agente editar
// arquivos de uma árvore enquanto o terminal roda comandos em outra. Sem
// workspace ativo sobra o cwd, que é de onde o app foi iniciado.
func (a *App) acpWorkDir() (string, error) {
	if a != nil && a.workspaceMgr != nil {
		if base := strings.TrimSpace(a.workspaceMgr.ActivePath()); base != "" {
			return base, nil
		}
	}
	return os.Getwd()
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

// closeAllACPSessions é o mesmo para o "limpar tudo": nenhuma das conversas que
// as sessões descrevem existe mais. Sem isso o agente segue respondendo com base
// em mensagens apagadas e os vínculos ficam no banco sem conversa que os
// reencontre.
func (a *App) closeAllACPSessions(ctx context.Context) {
	if a == nil || a.acpMgr == nil {
		return
	}
	if err := a.acpMgr.CloseAllConversations(ctx); err != nil {
		logging.Warnf(ctx, "app.app-acp", "[ACP] erro ao encerrar as sessões das conversas apagadas: %v", err)
	}
}

// resetACPRuntime derruba processos e sessões sem tocar no banco. É o que o
// reset do banco precisa: o arquivo inteiro foi recriado, então não há registro
// para apagar, e o que sobrou em memória descreve conversas de um banco que já
// não existe.
func (a *App) resetACPRuntime() {
	if a == nil || a.acpMgr == nil {
		return
	}
	a.acpMgr.DisconnectAll()
}
