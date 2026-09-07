package wailsapi

import (
	"assistente/internal/acp"
	"assistente/internal/apidto"
	"assistente/internal/database"
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const acpWorkDirComponent = "wailsapi.acp-workdir"

// ACPWorkDir é o bind Wails do domínio acp_workdir (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// Helpers lowercase (agentConversationDir, conversationAgentDir, acpWorkDir)
// permanecem no *App — usados por install/providers/runtime.
type ACPWorkDir struct {
	mu      sync.RWMutex
	session Session
	mgr     *acp.Manager
}

// NewACPWorkDir cria o bind vazio; AttachACPWorkDir preenche session + manager no startup.
func NewACPWorkDir() *ACPWorkDir {
	return &ACPWorkDir{}
}

// AttachACPWorkDir associa Session e Manager após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachACPWorkDir(api *ACPWorkDir, session Session, mgr *acp.Manager) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.mgr = mgr
}

func (api *ACPWorkDir) deps() (Session, *acp.Manager, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.mgr == nil {
		return nil, nil, ErrACPWorkDirNotWired
	}
	return api.session, api.mgr, nil
}

// PendingRecreate diz que a escolha feita ainda não valeu porque a sessão de pé
// é de outro diretório. Ela será recriada no próximo turno, e o agente começará
// sem lembrar do que já foi conversado.
func PendingRecreate(w apidto.AgentWorkDir) bool {
	return w.SessionDir != "" && !acp.SameDir(w.SessionDir, w.Dir)
}

// GetAgentConversationWorkDir devolve onde o agente desta conversa trabalha.
//
// De propósito não sobe processo nem abre sessão: mostrar o alcance do agente
// na barra não pode custar um agente de código de pé.
func (api *ACPWorkDir) GetAgentConversationWorkDir(conversationID string) (apidto.AgentWorkDir, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := apidto.AgentWorkDir{ConversationID: conversationID}
	session, mgr, err := api.deps()
	if err != nil {
		return out, err
	}
	result, err := WithUser(session, func(ctx context.Context) (apidto.AgentWorkDir, error) {
		return readAgentConversationWorkDir(ctx, mgr, conversationID, out)
	})
	if err != nil {
		return out, err
	}
	return result, nil
}

// SetAgentConversationWorkDir prende esta conversa a um diretório, ou a devolve
// ao workspace ativo quando o caminho vem vazio (AEP-0084 D5).
//
// A troca não mexe na sessão em pé: quem manda encerrar é a montagem do próximo
// turno, que vê o diretório diferente, se despede da sessão antiga e abre outra.
// É por isso que a resposta diz que há recriação pendente — a pessoa precisa
// saber que o agente vai recomeçar sem a memória desta conversa antes de mandar
// a próxima mensagem, e não depois de estranhar a resposta.
func (api *ACPWorkDir) SetAgentConversationWorkDir(conversationID, dir string) (apidto.AgentWorkDir, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := apidto.AgentWorkDir{ConversationID: conversationID}
	session, mgr, err := api.deps()
	if err != nil {
		return out, err
	}
	result, err := WithUser(session, func(ctx context.Context) (apidto.AgentWorkDir, error) {
		if conversationID == "" {
			return out, errors.New("conversa sem identificador")
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
		return readAgentConversationWorkDir(ctx, mgr, conversationID, out)
	})
	if err != nil {
		return out, err
	}
	return result, nil
}

func readAgentConversationWorkDir(
	ctx context.Context,
	mgr *acp.Manager,
	conversationID string,
	out apidto.AgentWorkDir,
) (apidto.AgentWorkDir, error) {
	if conversationID == "" {
		return out, errors.New("conversa sem identificador")
	}

	conv, err := database.GetConversationInfoWithContext(ctx, conversationID)
	if err != nil {
		return out, err
	}
	out.Pinned = strings.TrimSpace(conv.AgentWorkDir) != ""

	if workspaceDir, err := mgr.WorkDir(); err == nil {
		out.WorkspaceDir = workspaceDir
	}
	dir, err := mgr.ConversationWorkDir(conversationID)
	if err != nil {
		return out, err
	}
	out.Dir = dir
	out.SessionDir = mgr.ConversationSessionDir(conversationID)
	// Ter sessão de pé é a prova de que esta conversa fala com agente de código;
	// ter escolha guardada é a de que alguém já decidiu onde ele age. Fora
	// desses dois, não há diretório de agente a mostrar: numa conversa de
	// provedor HTTP o caminho do workspace não descreve alcance nenhum.
	out.Available = out.Pinned || out.SessionDir != ""
	return out, nil
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
