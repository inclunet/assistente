package wailsapi

import (
	"assistente/internal/acp"
	"assistente/internal/apidto"
	"context"
	"errors"
	"strings"
	"sync"
)

// ACPOptions é o bind Wails do domínio acp_options (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// agentSessionOptionsChanged e noticePermissionBarrier permanecem no *App
// (handlers/eventos lowercase).
type ACPOptions struct {
	mu                      sync.RWMutex
	session                 Session
	mgr                     *acp.Manager
	noticePermissionBarrier func(conversationID, previousMode, currentMode string, options []apidto.AgentConfigOption)
}

// NewACPOptions cria o bind vazio; AttachACPOptions preenche deps no startup.
func NewACPOptions() *ACPOptions {
	return &ACPOptions{}
}

// AttachACPOptions associa Session, Manager e o aviso de barreira de permissão
// após o startup. Função de pacote (não método) para não entrar no Bind do Wails.
// noticePermissionBarrier pode ser nil (testes sem emitter).
func AttachACPOptions(
	api *ACPOptions,
	session Session,
	mgr *acp.Manager,
	noticePermissionBarrier func(conversationID, previousMode, currentMode string, options []apidto.AgentConfigOption),
) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.mgr = mgr
	api.noticePermissionBarrier = noticePermissionBarrier
}

func (api *ACPOptions) deps() (Session, *acp.Manager, func(string, string, string, []apidto.AgentConfigOption), error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.mgr == nil {
		return nil, nil, nil, ErrACPOptionsNotWired
	}
	return api.session, api.mgr, api.noticePermissionBarrier, nil
}

// GetAgentSessionOptions devolve o que o agente desta conversa oferece — em que
// modelo e modo ele está, e o que há para escolher.
//
// De propósito não sobe processo nem abre sessão: a conversa que ainda não falou
// com o agente não tem estado a mostrar, e subir um agente de código porque uma
// barra de ferramentas renderizou seria pagar um processo por um seletor.
func (api *ACPOptions) GetAgentSessionOptions(conversationID string) (apidto.AgentSessionOptions, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := apidto.AgentSessionOptions{ConversationID: conversationID, Options: []apidto.AgentConfigOption{}}
	session, mgr, _, err := api.deps()
	if err != nil {
		return out, err
	}
	result, err := WithUser(session, func(ctx context.Context) (apidto.AgentSessionOptions, error) {
		_ = ctx
		if conversationID == "" {
			return out, errors.New("conversa sem identificador")
		}
		options := agentOptionsFrom(mgr.ConversationOptions(conversationID))
		out.Options = options
		out.Available = len(options) > 0
		return out, nil
	})
	if err != nil {
		return out, err
	}
	return result, nil
}

// SetAgentSessionOption troca o modelo ou o modo da sessão desta conversa e
// devolve o estado resultante — trocar de modelo pode mexer no que o agente
// oferece para o resto.
//
// A troca vale para o turno seguinte, e é isso que a pessoa espera de um seletor
// no meio de uma conversa: o turno em andamento já saiu para o agente.
func (api *ACPOptions) SetAgentSessionOption(conversationID, optionID, value string) (apidto.AgentSessionOptions, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := apidto.AgentSessionOptions{ConversationID: conversationID, Options: []apidto.AgentConfigOption{}}
	session, mgr, notice, err := api.deps()
	if err != nil {
		return out, err
	}
	result, err := WithUser(session, func(ctx context.Context) (apidto.AgentSessionOptions, error) {
		if conversationID == "" {
			return out, errors.New("conversa sem identificador")
		}
		// O modo de antes é lido aqui, e não depois: o aviso de que a barreira de
		// permissão caiu é da transição, e depois da troca só se sabe onde a sessão
		// foi parar. Ler não sobe processo nem abre sessão.
		previousMode := currentModeOf(mgr.ConversationOptions(conversationID))
		options, err := mgr.SetConversationOption(ctx, conversationID, optionID, value)
		if err != nil {
			// Troca recusada pelo agente não mudou barreira nenhuma: a sessão segue
			// no modo em que estava, e avisar aqui contaria uma mudança que não
			// houve.
			return out, err
		}
		out.Options = agentOptionsFrom(options)
		out.Available = len(out.Options) > 0
		// Pelo estado que voltou, e não pelo valor pedido: o agente às vezes acomoda
		// o pedido em outro modo, e o aviso precisa falar do que passou a valer.
		if notice != nil {
			notice(conversationID, previousMode, currentModeOf(options), out.Options)
		}
		return out, nil
	})
	if err != nil {
		return out, err
	}
	return result, nil
}

func currentModeOf(options []acp.ConfigOption) string {
	option, ok := acp.OptionByCategory(options, acp.CategoryMode)
	if !ok {
		return ""
	}
	return strings.TrimSpace(option.CurrentValue)
}

func agentOptionsFrom(options []acp.ConfigOption) []apidto.AgentConfigOption {
	out := make([]apidto.AgentConfigOption, 0, len(options))
	for _, option := range options {
		if len(option.Values) == 0 {
			continue
		}
		converted := apidto.AgentConfigOption{
			ID:           option.ID,
			Name:         option.Name,
			Category:     option.Category,
			CurrentValue: option.CurrentValue,
			Values:       make([]apidto.AgentConfigValue, 0, len(option.Values)),
		}
		for _, value := range option.Values {
			converted.Values = append(converted.Values, apidto.AgentConfigValue{Value: value.Value, Name: value.Name})
		}
		out = append(out, converted)
	}
	return out
}
