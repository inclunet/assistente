package wailsapi

import (
	"assistente/internal/acp"
	"assistente/internal/apidto"
	"context"
	"errors"
	"strings"
	"sync"
)

// ACPCommands é o bind Wails do domínio acp_commands (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// agentSessionCommandsChanged e a emissão de chat:agent_commands permanecem no
// *App (handler de evento lowercase).
type ACPCommands struct {
	mu      sync.RWMutex
	session Session
	mgr     *acp.Manager
}

// NewACPCommands cria o bind vazio; AttachACPCommands preenche session + manager no startup.
func NewACPCommands() *ACPCommands {
	return &ACPCommands{}
}

// AttachACPCommands associa Session e Manager após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachACPCommands(api *ACPCommands, session Session, mgr *acp.Manager) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.mgr = mgr
}

func (api *ACPCommands) deps() (Session, *acp.Manager, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.mgr == nil {
		return nil, nil, ErrACPCommandsNotWired
	}
	return api.session, api.mgr, nil
}

// GetAgentSessionCommands devolve os comandos que o agente desta conversa
// oferece. Como o seletor de modelo, de propósito não sobe processo nem abre
// sessão: abrir o menu de comandos não pode fazer nascer um agente de código.
func (api *ACPCommands) GetAgentSessionCommands(conversationID string) (apidto.AgentSessionCommands, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := apidto.AgentSessionCommands{ConversationID: conversationID, Commands: []apidto.AgentCommand{}}
	session, mgr, err := api.deps()
	if err != nil {
		return out, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.AgentSessionCommands, error) {
		if conversationID == "" {
			return out, errors.New("conversa sem identificador")
		}
		out.Commands = agentCommandsFrom(mgr.ConversationCommands(conversationID))
		return out, nil
	})
}

func agentCommandsFrom(commands []acp.Command) []apidto.AgentCommand {
	out := make([]apidto.AgentCommand, 0, len(commands))
	for _, command := range commands {
		out = append(out, apidto.AgentCommand{
			Name:         command.Name,
			Description:  command.Description,
			AcceptsInput: command.AcceptsInput,
		})
	}
	return out
}
