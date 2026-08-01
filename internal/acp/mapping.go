package acp

import (
	"errors"
	"strings"

	sdk "github.com/coder/acp-go-sdk"
)

func capabilitiesFrom(resp sdk.InitializeResponse) Capabilities {
	caps := Capabilities{
		LoadSession:           resp.AgentCapabilities.LoadSession,
		CloseSession:          resp.AgentCapabilities.SessionCapabilities.Close != nil,
		PromptImage:           resp.AgentCapabilities.PromptCapabilities.Image,
		PromptAudio:           resp.AgentCapabilities.PromptCapabilities.Audio,
		PromptEmbeddedContext: resp.AgentCapabilities.PromptCapabilities.EmbeddedContext,
	}
	if resp.AgentInfo != nil {
		caps.AgentName = resp.AgentInfo.Name
		caps.AgentVersion = resp.AgentInfo.Version
	}
	for _, method := range resp.AuthMethods {
		if auth, ok := authMethodFrom(method); ok {
			caps.AuthMethods = append(caps.AuthMethods, auth)
		}
	}
	return caps
}

// authMethodFrom achata as variantes de autenticação do protocolo. O tipo
// importa para quem for exibir o estado: "o agente resolve" (o caso do Cursor)
// pede uma instrução de terminal, enquanto env_var pediria uma credencial.
func authMethodFrom(method sdk.AuthMethod) (AuthMethod, bool) {
	switch {
	case method.Agent != nil:
		return AuthMethod{
			ID:          method.Agent.Id,
			Name:        method.Agent.Name,
			Description: derefString(method.Agent.Description),
			Kind:        AuthKindAgent,
		}, true
	case method.EnvVar != nil:
		return AuthMethod{
			ID:          method.EnvVar.Id,
			Name:        method.EnvVar.Name,
			Description: derefString(method.EnvVar.Description),
			Kind:        AuthKindEnvVar,
		}, true
	case method.Terminal != nil:
		return AuthMethod{
			ID:          method.Terminal.Id,
			Name:        method.Terminal.Name,
			Description: derefString(method.Terminal.Description),
			Kind:        AuthKindTerminal,
		}, true
	}
	return AuthMethod{}, false
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// updateFrom traduz uma notificação do agente. O segundo retorno é falso para
// o que este pacote ainda não mapeia (planos, comandos disponíveis, uso de
// contexto): descartar no transporte é melhor do que entregar meio evento.
func updateFrom(update sdk.SessionUpdate) (Update, bool) {
	switch {
	case update.AgentMessageChunk != nil:
		text := textOf(update.AgentMessageChunk.Content)
		if text == "" {
			return Update{}, false
		}
		return Update{Kind: UpdateText, Text: text}, true

	case update.AgentThoughtChunk != nil:
		text := textOf(update.AgentThoughtChunk.Content)
		if text == "" {
			return Update{}, false
		}
		return Update{Kind: UpdateThought, Text: text}, true

	case update.ToolCall != nil:
		call := toolCallFromStart(update.ToolCall)
		return Update{Kind: UpdateToolStart, Tool: &call}, true

	case update.ToolCallUpdate != nil:
		call := toolCallFromProgress(update.ToolCallUpdate)
		return Update{Kind: UpdateToolProgress, Tool: &call}, true

	case update.ConfigOptionUpdate != nil:
		return Update{
			Kind:          UpdateConfigOptions,
			ConfigOptions: configOptionsFrom(update.ConfigOptionUpdate.ConfigOptions),
		}, true

	case update.CurrentModeUpdate != nil:
		return Update{Kind: UpdateMode, Mode: string(update.CurrentModeUpdate.CurrentModeId)}, true

	case update.SessionInfoUpdate != nil:
		if update.SessionInfoUpdate.Title == nil {
			return Update{}, false
		}
		title := strings.TrimSpace(*update.SessionInfoUpdate.Title)
		if title == "" {
			return Update{}, false
		}
		return Update{Kind: UpdateTitle, Title: title}, true
	}

	return Update{}, false
}

func textOf(block sdk.ContentBlock) string {
	if block.Text == nil {
		return ""
	}
	return block.Text.Text
}

func toolCallFromStart(call *sdk.SessionUpdateToolCall) ToolCall {
	return ToolCall{
		ID:     normalizeID(string(call.ToolCallId)),
		Kind:   string(call.Kind),
		Title:  call.Title,
		Status: string(call.Status),
	}
}

func toolCallFromProgress(call *sdk.SessionToolCallUpdate) ToolCall {
	out := ToolCall{ID: normalizeID(string(call.ToolCallId))}
	if call.Kind != nil {
		out.Kind = string(*call.Kind)
	}
	if call.Title != nil {
		out.Title = *call.Title
	}
	if call.Status != nil {
		out.Status = string(*call.Status)
	}
	return out
}

func toolCallFromUpdate(call sdk.ToolCallUpdate) ToolCall {
	out := ToolCall{ID: normalizeID(string(call.ToolCallId))}
	if call.Kind != nil {
		out.Kind = string(*call.Kind)
	}
	if call.Title != nil {
		out.Title = *call.Title
	}
	if call.Status != nil {
		out.Status = string(*call.Status)
	}
	return out
}

func configOptionsFrom(options []sdk.SessionConfigOption) []ConfigOption {
	var out []ConfigOption
	for _, option := range options {
		// Opções booleanas ainda não têm uso no app; mapear só o seletor evita
		// inventar semântica para algo que ninguém consome.
		if option.Select == nil {
			continue
		}
		converted := ConfigOption{
			ID:           string(option.Select.Id),
			Name:         option.Select.Name,
			CurrentValue: string(option.Select.CurrentValue),
			Values:       configValuesFrom(option.Select.Options),
		}
		if option.Select.Category != nil {
			converted.Category = string(*option.Select.Category)
		}
		out = append(out, converted)
	}
	return out
}

func configValuesFrom(options sdk.SessionConfigSelectOptions) []ConfigValue {
	var out []ConfigValue
	if options.Ungrouped != nil {
		for _, value := range *options.Ungrouped {
			out = append(out, ConfigValue{Value: string(value.Value), Name: value.Name})
		}
	}
	if options.Grouped != nil {
		// Os grupos são organização visual; para escolher um modelo importa a
		// lista de valores, então ela é achatada.
		for _, group := range *options.Grouped {
			for _, value := range group.Options {
				out = append(out, ConfigValue{Value: string(value.Value), Name: value.Name})
			}
		}
	}
	return out
}

// modeOptionFrom converte o formato legado de modos numa ConfigOption, para o
// app ter um caminho só. O Cursor manda os dois formatos no mesmo payload.
//
// O rótulo fica vazio de propósito: o formato legado não traz um, e inventar um
// aqui seria enfiar texto de interface — em inglês — dentro do transporte. Quem
// exibe traduz a partir da categoria.
func modeOptionFrom(state *sdk.SessionModeState) ConfigOption {
	option := ConfigOption{
		ID:           "mode",
		Category:     string(sdk.SessionConfigOptionCategoryMode),
		CurrentValue: string(state.CurrentModeId),
	}
	for _, mode := range state.AvailableModes {
		option.Values = append(option.Values, ConfigValue{Value: string(mode.Id), Name: mode.Name})
	}
	return option
}

// withModeOption acrescenta o modo legado só quando o agente não mandou a
// mesma informação no formato estável, para a lista não vir duplicada.
func withModeOption(options []ConfigOption, state *sdk.SessionModeState) []ConfigOption {
	if state == nil {
		return options
	}
	for _, option := range options {
		if option.Category == string(sdk.SessionConfigOptionCategoryMode) {
			return options
		}
	}
	return append(options, modeOptionFrom(state))
}

func permissionOptionsFrom(options []sdk.PermissionOption) []PermissionOption {
	out := make([]PermissionOption, 0, len(options))
	for _, option := range options {
		out = append(out, PermissionOption{
			ID:   string(option.OptionId),
			Name: option.Name,
			Kind: string(option.Kind),
		})
	}
	return out
}

// permissionOutcomeToSDK converte a decisão em resposta de protocolo. Sem
// escolha, nega: prefere-se a opção de recusa que o próprio agente ofereceu,
// porque "cancelado" no ACP significa que o turno inteiro foi cancelado, e não
// que esta ação foi negada (AEP-0084 D9).
//
// A escolha é conferida contra o que o agente ofereceu. Uma opção inventada —
// erro de quem decide, ou uma allowlist guardando um identificador que o agente
// já não usa — viraria resposta inválida e derrubaria o turno; negar
// pontualmente é o desfecho seguro.
func permissionOutcomeToSDK(outcome PermissionOutcome, offered []sdk.PermissionOption) sdk.RequestPermissionOutcome {
	if id := strings.TrimSpace(outcome.OptionID); id != "" && isOffered(id, offered) {
		return sdk.RequestPermissionOutcome{
			Selected: &sdk.RequestPermissionOutcomeSelected{
				OptionId: sdk.PermissionOptionId(id),
				Outcome:  "selected",
			},
		}
	}
	if id, ok := rejectOptionID(offered); ok {
		return sdk.RequestPermissionOutcome{
			Selected: &sdk.RequestPermissionOutcomeSelected{
				OptionId: id,
				Outcome:  "selected",
			},
		}
	}
	return sdk.RequestPermissionOutcome{
		Cancelled: &sdk.RequestPermissionOutcomeCancelled{Outcome: "cancelled"},
	}
}

func isOffered(id string, offered []sdk.PermissionOption) bool {
	for _, option := range offered {
		if string(option.OptionId) == id {
			return true
		}
	}
	return false
}

// rejectOptionID escolhe a recusa pontual. A recusa permanente é evitada de
// propósito: negar uma vez por falta de resposta é diferente de calar o agente
// para sempre sem ninguém ter decidido isso.
func rejectOptionID(offered []sdk.PermissionOption) (sdk.PermissionOptionId, bool) {
	for _, option := range offered {
		if option.Kind == sdk.PermissionOptionKindRejectOnce {
			return option.OptionId, true
		}
	}
	return "", false
}

func promptBlocks(content []Content) ([]sdk.ContentBlock, error) {
	blocks := make([]sdk.ContentBlock, 0, len(content))
	for _, item := range content {
		switch {
		case item.ImageData != "":
			blocks = append(blocks, sdk.ImageBlock(item.ImageData, item.ImageMIME))
		case item.Text != "":
			blocks = append(blocks, sdk.TextBlock(item.Text))
		}
	}
	if len(blocks) == 0 {
		return nil, errors.New("turno ACP sem conteúdo para enviar")
	}
	return blocks, nil
}
