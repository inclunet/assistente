package app

import (
	"context"
	"os"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/acptrust"
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
	if a.acpTrust == nil {
		a.acpTrust = acptrust.NewStore()
	}
	handler := &acpRequestHandler{
		questions: func() *questionnaire.Manager { return a.questionnaireMgr },
		surfaces:  a.questionnaireRouter(),
		origin:    a.acpConversationSurface,
		notices:   func() ports.Emitter { return a.emitter },
		trust:     func() *acptrust.Store { return a.acpTrust },
		activeProfile: func() string {
			if a.profileManager == nil {
				return ""
			}
			return a.profileManager.GetActiveSlug()
		},
	}
	a.acpMgr = acp.NewManager(acp.ManagerConfig{
		// O banco é buscado a cada uso, não guardado: resetá-lo fecha a conexão
		// e abre outra, e uma conexão guardada aqui ficaria apontando para a
		// fechada até o app reiniciar.
		Store:   acp.NewDBSessionStore(database.DB),
		Handler: handler,
		WorkDir: a.acpWorkDir,
		// O agente troca de modelo sozinho e avisa. A tela precisa refletir isso,
		// e quem usa leitor de telas precisa ouvi-lo (AEP-0084 D6).
		OnSessionOptions: a.agentSessionOptionsChanged,
		ClientName:       "assistente",
		ClientVersion:    AppVersion,
	})
	handler.owner = a.acpMgr.TurnOwnerOf
}

// questionnaireRouter é por onde qualquer diálogo do backend chega a quem
// decide: a tela, quando há alguém nela, ou o canal de onde a conversa veio
// (AEP-0084 Fase 5). As duas pontas são resolvidas na hora do uso, e não agora:
// o questionário e o gateway de mensageria nascem depois deste ponto, e um valor
// guardado aqui congelaria um nulo.
func (a *App) questionnaireRouter() *questionnaire.Router {
	return questionnaire.NewRouter(
		func() *questionnaire.Manager { return a.questionnaireMgr },
		func() questionnaire.ChannelAsker {
			if a == nil || a.msgGateway == nil {
				return nil
			}
			return a.msgGateway.ChannelQuestions()
		},
	)
}

// acpConversationSurface descobre de onde veio a conversa de um turno sem tela.
// Conversa de canal pergunta pelo próprio canal; o que não veio de canal — job
// agendado, subagente, CLI — não tem a quem perguntar.
//
// O contexto é montado aqui com o dono do turno porque o pedido do agente chega
// pelo contexto do transporte, sem escopo de usuário: sem ele a consulta falha
// (fail-closed do AEP-0052), e com o dono errado leria a conversa de outra
// pessoa.
func (a *App) acpConversationSurface(owner acp.TurnOwner) questionnaire.Surface {
	return conversationSurface(owner, database.GetConversationInfoWithContext)
}

// conversationSurface é a regra de descoberta, separada de onde a conversa é
// lida para poder ser exercitada sem banco.
func conversationSurface(owner acp.TurnOwner, lookup func(context.Context, string) (*database.Conversation, error)) questionnaire.Surface {
	conversationID := strings.TrimSpace(owner.ConversationID)
	userID := strings.TrimSpace(owner.UserID)
	if conversationID == "" || userID == "" || lookup == nil {
		return questionnaire.NoSurface(conversationID)
	}
	ctx := database.WithUserID(context.Background(), userID)
	conv, err := lookup(ctx, conversationID)
	if err != nil || conv == nil {
		logging.Warnf(ctx, "app.app-acp",
			"[ACP] não foi possível descobrir a origem da conversa %s para perguntar: %v", conversationID, err)
		return questionnaire.NoSurface(conversationID)
	}
	// ChannelSurface recusa o que não estiver completo: conversa local, ou de
	// canal sem contato, cai em superfície nenhuma.
	return questionnaire.ChannelSurface(conversationID, conv.Channel, conv.ContactID)
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
