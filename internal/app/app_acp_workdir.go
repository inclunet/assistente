package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/database"
	"assistente/internal/logging"
)

const acpWorkDirComponent = "app.app-acp-workdir"

// AgentWorkDir descreve onde o agente de código desta conversa trabalha
// (AEP-0084 D5). O diretório é o alcance do que ele pode ler e editar, e por
// isso fica visível na barra da conversa em vez de implícito.
type AgentWorkDir struct {
	ConversationID string `json:"conversationId"`
	// Dir é o diretório que vale para o próximo turno.
	Dir string `json:"dir"`
	// WorkspaceDir é o diretório do app, que é o padrão de quem não escolheu.
	WorkspaceDir string `json:"workspaceDir"`
	// Pinned diz que esta conversa escolheu o diretório dela, e por isso não
	// acompanha mais a troca de workspace do app.
	Pinned bool `json:"pinned"`
	// SessionDir é o diretório da sessão que o agente tem de pé para esta
	// conversa, quando há uma. Diferente de Dir, ele conta o que ainda vale: a
	// escolha nova só chega ao agente quando a sessão for recriada.
	SessionDir string `json:"sessionDir,omitempty"`
}

// PendingRecreate diz que a escolha feita ainda não valeu porque a sessão de pé
// é de outro diretório. Ela será recriada no próximo turno, e o agente começará
// sem lembrar do que já foi conversado.
func (w AgentWorkDir) PendingRecreate() bool {
	return w.SessionDir != "" && !acp.SameDir(w.SessionDir, w.Dir)
}

// GetAgentConversationWorkDir devolve onde o agente desta conversa trabalha.
//
// De propósito não sobe processo nem abre sessão: mostrar o alcance do agente
// na barra não pode custar um agente de código de pé.
func (a *App) GetAgentConversationWorkDir(conversationID string) (AgentWorkDir, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := AgentWorkDir{ConversationID: conversationID}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return out, err
	}
	if conversationID == "" {
		return out, errors.New("conversa sem identificador")
	}
	if a.acpMgr == nil {
		return out, nil
	}

	conv, err := database.GetConversationInfoWithContext(ctx, conversationID)
	if err != nil {
		return out, err
	}
	out.Pinned = strings.TrimSpace(conv.AgentWorkDir) != ""

	if workspaceDir, err := a.acpMgr.WorkDir(); err == nil {
		out.WorkspaceDir = workspaceDir
	}
	dir, err := a.acpMgr.ConversationWorkDir(conversationID)
	if err != nil {
		return out, err
	}
	out.Dir = dir
	out.SessionDir = a.acpMgr.ConversationSessionDir(conversationID)
	return out, nil
}

// SetAgentConversationWorkDir prende esta conversa a um diretório, ou a devolve
// ao workspace ativo quando o caminho vem vazio (AEP-0084 D5).
//
// A troca não mexe na sessão em pé: quem manda encerrar é a montagem do próximo
// turno, que vê o diretório diferente, se despede da sessão antiga e abre outra.
// É por isso que a resposta diz que há recriação pendente — a pessoa precisa
// saber que o agente vai recomeçar sem a memória desta conversa antes de mandar
// a próxima mensagem, e não depois de estranhar a resposta.
func (a *App) SetAgentConversationWorkDir(conversationID, dir string) (AgentWorkDir, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := AgentWorkDir{ConversationID: conversationID}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return out, err
	}
	if conversationID == "" {
		return out, errors.New("conversa sem identificador")
	}
	if a.acpMgr == nil {
		return out, errors.New("o serviço de agentes de código não está disponível")
	}

	resolved, err := resolveAgentWorkDir(dir)
	if err != nil {
		return out, err
	}
	if err := database.UpdateConversationAgentWorkDirWithContext(ctx, conversationID, resolved); err != nil {
		return out, err
	}
	logging.Infof(ctx, acpWorkDirComponent,
		"[ACP] conversa %s passou a trabalhar em %q", conversationID, resolved)
	return a.GetAgentConversationWorkDir(conversationID)
}

// resolveAgentWorkDir confere o caminho escrito antes de ele virar o alcance do
// agente. Caminho vazio é resposta legítima: significa "volte ao workspace
// ativo".
//
// A conferência é aqui, e não na hora do turno, porque errar uma letra no
// caminho só apareceria como um agente dizendo que não acha arquivo nenhum —
// e, pior, um caminho existente mas errado seria descoberto por edição feita no
// lugar errado.
func resolveAgentWorkDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("caminho inválido: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("o diretório %s não existe", absolute)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s não é um diretório", absolute)
	}
	return absolute, nil
}

// agentConversationDir é o que o serviço de agentes consulta para saber onde
// pôr o agente de uma conversa. Roda no caminho do turno, então lê o registro e
// nada mais.
func (a *App) agentConversationDir(conversationID string) (string, error) {
	if a == nil {
		return "", nil
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		// Sem sessão autenticada não há registro a ler. O turno em si já vai
		// falhar por conta própria; devolver erro aqui só trocaria a mensagem
		// por uma que não explica nada.
		return "", nil
	}
	return a.conversationAgentDir(ctx, conversationID)
}

func (a *App) conversationAgentDir(ctx context.Context, conversationID string) (string, error) {
	conv, err := database.GetConversationInfoWithContext(ctx, conversationID)
	if err != nil {
		// Conversa que ainda não está no banco não escolheu diretório nenhum, e
		// tratar isso como falha impediria o primeiro turno de uma conversa que
		// nasce junto com ele.
		return "", nil
	}
	return strings.TrimSpace(conv.AgentWorkDir), nil
}
