package app

import (
	"context"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/apidto"
	"assistente/internal/core/ports"
	"assistente/internal/logging"
)

const acpOptionsComponent = "app.app-acp-options"

// AgentSessionOptionsEvent é o payload de `chat:agent_options`: o agente contou
// que o modelo ou o modo da sessão mudou (AEP-0084 D6).
//
// O agente troca de modelo por conta própria — fallback de limite de uso, por
// exemplo — e a pessoa precisa saber com quem está falando. Announce diz se essa
// mudança merece ser falada: o agente também repete o estado sem nada ter
// mudado, e anunciar cada repetição atropelaria a leitura da resposta em curso.
//
// GetAgentSessionOptions / SetAgentSessionOption migraram para wailsapi.ACPOptions
// (AEP-0088); este handler lowercase e noticePermissionBarrier permanecem no App.
type AgentSessionOptionsEvent struct {
	ConversationID string                     `json:"conversationId"`
	Options        []apidto.AgentConfigOption `json:"options"`
	Model          string                     `json:"model,omitempty"`
	Mode           string                     `json:"mode,omitempty"`
	ModelChanged   bool                       `json:"modelChanged"`
	ModeChanged    bool                       `json:"modeChanged"`
	Announce       bool                       `json:"announce"`
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
func (a *App) noticePermissionBarrier(conversationID, previousMode, currentMode string, options []apidto.AgentConfigOption) {
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
//
// O valor corrente é procurado na lista sem depender da caixa, pelo mesmo motivo
// que ModeSkipsPermissionPrompt não depende dela: os dois vêm do agente pelo
// fio, e um `DONTASK` corrente que não casasse com o `dontAsk` da lista faria o
// aviso reconhecer o modo para alertar e não reconhecê-lo para nomear.
func agentModeName(options []apidto.AgentConfigOption, mode string) string {
	wanted := strings.TrimSpace(mode)
	for _, option := range options {
		if !strings.EqualFold(option.Category, acp.CategoryMode) {
			continue
		}
		for _, value := range option.Values {
			if !strings.EqualFold(strings.TrimSpace(value.Value), wanted) {
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
