package app

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/core/ports"
	"assistente/internal/logging"
)

const acpOptionsComponent = "app.app-acp-options"

// AgentConfigValue é um valor que o agente oferece para uma opção.
type AgentConfigValue struct {
	Value string `json:"value"`
	// Name é o rótulo do agente. Pode vir vazio — o modo no formato anterior não
	// traz um —, e aí quem exibe cai no próprio valor.
	Name string `json:"name,omitempty"`
}

// AgentConfigOption é uma escolha que o agente expõe para a sessão: o modelo, o
// modo (AEP-0084 D6).
type AgentConfigOption struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Category diz o que a opção significa (`model`, `mode`). É por ela que a
	// tela sabe que seletor está desenhando: o identificador é escolha do agente
	// e varia entre implementações.
	Category     string             `json:"category,omitempty"`
	CurrentValue string             `json:"currentValue"`
	Values       []AgentConfigValue `json:"values"`
}

// AgentSessionOptions é o que a conversa pode escolher no agente agora.
type AgentSessionOptions struct {
	ConversationID string `json:"conversationId"`
	// Available é falso quando não há o que escolher: conversa que não é de
	// agente, sessão que ainda não nasceu, agente que decide o próprio modelo. A
	// tela esconde o seletor em vez de mostrar um controle vazio.
	Available bool                `json:"available"`
	Options   []AgentConfigOption `json:"options"`
}

// AgentSessionOptionsEvent é o payload de `chat:agent_options`: o agente contou
// que o modelo ou o modo da sessão mudou (AEP-0084 D6).
//
// O agente troca de modelo por conta própria — fallback de limite de uso, por
// exemplo — e a pessoa precisa saber com quem está falando. Announce diz se essa
// mudança merece ser falada: o agente também repete o estado sem nada ter
// mudado, e anunciar cada repetição atropelaria a leitura da resposta em curso.
type AgentSessionOptionsEvent struct {
	ConversationID string              `json:"conversationId"`
	Options        []AgentConfigOption `json:"options"`
	Model          string              `json:"model,omitempty"`
	Mode           string              `json:"mode,omitempty"`
	ModelChanged   bool                `json:"modelChanged"`
	ModeChanged    bool                `json:"modeChanged"`
	Announce       bool                `json:"announce"`
}

// GetAgentSessionOptions devolve o que o agente desta conversa oferece — em que
// modelo e modo ele está, e o que há para escolher.
//
// De propósito não sobe processo nem abre sessão: a conversa que ainda não falou
// com o agente não tem estado a mostrar, e subir um agente de código porque uma
// barra de ferramentas renderizou seria pagar um processo por um seletor.
func (a *App) GetAgentSessionOptions(conversationID string) (AgentSessionOptions, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := AgentSessionOptions{ConversationID: conversationID, Options: []AgentConfigOption{}}
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return out, err
	}
	if conversationID == "" {
		return out, errors.New("conversa sem identificador")
	}
	if a.acpMgr == nil {
		return out, nil
	}
	options := agentOptionsFrom(a.acpMgr.ConversationOptions(conversationID))
	out.Options = options
	out.Available = len(options) > 0
	return out, nil
}

// SetAgentSessionOption troca o modelo ou o modo da sessão desta conversa e
// devolve o estado resultante — trocar de modelo pode mexer no que o agente
// oferece para o resto.
//
// A troca vale para o turno seguinte, e é isso que a pessoa espera de um seletor
// no meio de uma conversa: o turno em andamento já saiu para o agente.
func (a *App) SetAgentSessionOption(conversationID, optionID, value string) (AgentSessionOptions, error) {
	conversationID = strings.TrimSpace(conversationID)
	out := AgentSessionOptions{ConversationID: conversationID, Options: []AgentConfigOption{}}
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
	// O modo de antes é lido aqui, e não depois: o aviso de que a barreira de
	// permissão caiu é da transição, e depois da troca só se sabe onde a sessão
	// foi parar. Ler não sobe processo nem abre sessão.
	previousMode := currentModeOf(a.acpMgr.ConversationOptions(conversationID))
	options, err := a.acpMgr.SetConversationOption(ctx, conversationID, optionID, value)
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
	a.noticePermissionBarrier(conversationID, previousMode, currentModeOf(options), out.Options)
	return out, nil
}

// noticePermissionBarrier conta à conversa que o modo do agente mudou o que vale
// para o pedido de permissão (AEP-0084 D9, Fase 7).
//
// Há modos que dispensam o `session/request_permission`, e ele é a única
// barreira que o app tem para autorizar o que o agente faz na máquina. Escolher
// um deles muda o comportamento daí em diante, e o seletor que recebeu a escolha
// não fica na tela contando isso — é o mesmo caso do "permitir sempre".
//
// O aviso é da transição: quem já estava sem barreira e trocou para outro modo
// que também não pergunta não é avisado de novo, pelo mesmo motivo que a
// autorização permanente não se repete a cada pedido.
func (a *App) noticePermissionBarrier(conversationID, previousMode, currentMode string, options []AgentConfigOption) {
	if a == nil || a.emitter == nil {
		return
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" || strings.TrimSpace(currentMode) == "" {
		// Sem saber em que modo a sessão ficou não há transição a contar, e
		// chutar diria que a barreira voltou justamente quando ela pode ter
		// caído.
		return
	}
	before := acp.ModeSkipsPermissionPrompt(previousMode)
	now := acp.ModeSkipsPermissionPrompt(currentMode)
	if before == now {
		return
	}
	kind := ports.ChatNoticeKindModeAsksPermission
	if now {
		kind = ports.ChatNoticeKindModeSkipsPermission
		logging.Warnf(context.Background(), acpOptionsComponent,
			"[ACP] a conversa %s passou para um modo que dispensa o pedido de permissão", conversationID)
	}
	a.emitter.Emit("chat:notice", ports.ChatNoticeEvent{
		ConversationID: conversationID,
		Kind:           kind,
		Mode:           agentModeName(options, currentMode),
	})
}

// currentModeOf lê o modo corrente do conjunto que o agente mandou. Por
// categoria, nunca pelo identificador, que é escolha dele.
func currentModeOf(options []acp.ConfigOption) string {
	option, ok := acp.OptionByCategory(options, acp.CategoryMode)
	if !ok {
		return ""
	}
	return strings.TrimSpace(option.CurrentValue)
}

// agentModeName nomeia o modo como o seletor o nomeia: o rótulo que o agente deu
// e, quando ele não deu nenhum, o valor cru — o último recurso, mas melhor do
// que um aviso que não diz de que modo fala.
//
// O rótulo é texto do agente, então passa pelo saneamento como todo texto dele
// que vira UI ou anúncio (AEP-0084 D11).
func agentModeName(options []AgentConfigOption, mode string) string {
	wanted := strings.TrimSpace(mode)
	for _, option := range options {
		if !strings.EqualFold(option.Category, acp.CategoryMode) {
			continue
		}
		for _, value := range option.Values {
			if strings.TrimSpace(value.Value) != wanted {
				continue
			}
			if name := acp.SanitizeLabel(value.Name); name != "" {
				return name
			}
		}
	}
	return acp.SanitizeLabel(wanted)
}

// agentSessionOptionsChanged leva ao frontend o que o agente contou. Roda na
// goroutine de entrega do transporte, então só monta o payload e emite: falar
// com o agente daqui pararia o protocolo.
func (a *App) agentSessionOptionsChanged(event acp.SessionOptionsEvent) {
	if a == nil || a.emitter == nil {
		return
	}
	options := agentOptionsFrom(event.Options)
	if len(options) == 0 && !event.Announceable() {
		// Conjunto do qual nada é aproveitável não descreve seletor nenhum, e
		// nada mudou: não há o que dizer nem o que desenhar.
		return
	}
	// Conjunto vazio com troca dentro segue viagem: o agente pode contar que
	// mudou de modelo sem repetir a lista de opções, e engolir esse aviso seria
	// engolir justamente a troca que a pessoa não viu acontecer. Quem exibe sabe
	// que vazio aqui não é ordem de apagar os seletores.
	if event.Announceable() {
		logging.Infof(context.Background(), acpOptionsComponent,
			"[ACP] o agente da conversa %s mudou de configuração: modelo=%q modo=%q",
			event.ConversationID, event.Model, event.Mode)
	}
	a.emitter.Emit("chat:agent_options", AgentSessionOptionsEvent{
		ConversationID: event.ConversationID,
		Options:        options,
		Model:          event.Model,
		Mode:           event.Mode,
		ModelChanged:   event.ModelChanged,
		ModeChanged:    event.ModeChanged,
		Announce:       event.Announceable(),
	})
	if event.ModeChanged {
		// Também quando ninguém clicou: o aviso é sobre a barreira ter caído, e
		// não sobre quem a derrubou. O modo muda por `config_option_update` sem
		// ninguém ter escolhido, e nesse caso a pessoa tem ainda menos como
		// saber — o seletor passa a mostrar outro nome, e nome de modo não diz
		// que o agente parou de pedir autorização.
		//
		// ModeChanged já exclui a primeira leitura de uma sessão, que é o estado
		// inicial dela e não uma troca.
		a.noticePermissionBarrier(event.ConversationID, event.PreviousMode, event.Mode, options)
	}
}

// agentOptionsFrom traduz as opções do transporte para o que a tela consome. As
// que não têm valor para escolher ficam de fora: um seletor sem opções é um
// controle mudo, e há opções no protocolo que este app ainda não desenha.
func agentOptionsFrom(options []acp.ConfigOption) []AgentConfigOption {
	out := make([]AgentConfigOption, 0, len(options))
	for _, option := range options {
		if len(option.Values) == 0 {
			continue
		}
		converted := AgentConfigOption{
			ID:           option.ID,
			Name:         option.Name,
			Category:     option.Category,
			CurrentValue: option.CurrentValue,
			Values:       make([]AgentConfigValue, 0, len(option.Values)),
		}
		for _, value := range option.Values {
			converted.Values = append(converted.Values, AgentConfigValue{Value: value.Value, Name: value.Name})
		}
		out = append(out, converted)
	}
	return out
}
