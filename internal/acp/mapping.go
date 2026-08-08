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
// o que este pacote ainda não mapeia (planos, uso de contexto): descartar no
// transporte é melhor do que entregar meio evento.
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
		options := configOptionsFrom(update.ConfigOptionUpdate.ConfigOptions)
		// Este evento é o conjunto completo, então um conjunto vazio diz "não
		// há mais opção nenhuma". Nada mapeado não significa isso: significa
		// que o agente só mandou coisas que ainda não consumimos. Entregar o
		// vazio faria o seletor de modelo desaparecer da tela sem motivo.
		if len(options) == 0 {
			return Update{}, false
		}
		return Update{Kind: UpdateConfigOptions, ConfigOptions: options}, true

	case update.CurrentModeUpdate != nil:
		return Update{Kind: UpdateMode, Mode: string(update.CurrentModeUpdate.CurrentModeId)}, true

	case update.AvailableCommandsUpdate != nil:
		// Lista vazia é resposta, e não ausência de resposta: o agente está
		// dizendo que não oferece comando nenhum, e o app precisa tirar da tela
		// os que ofereceu antes.
		return Update{
			Kind:     UpdateCommands,
			Commands: commandsFrom(update.AvailableCommandsUpdate.AvailableCommands),
		}, true

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

// commandsFrom traduz os comandos que o agente oferece. Nome e descrição vão
// para a tela e para o leitor de telas, então passam pelo saneamento de rótulo
// (AEP-0084 D11) — texto vindo do agente é dado não confiável.
//
// Comando sem nome, ou com nome que não sobrevive ao saneamento, é descartado:
// não há o que digitar depois da barra, e um item mudo no menu só atrapalha
// quem navega por teclado. Nome com espaço sai pelo mesmo motivo: tudo que vem
// depois do primeiro espaço é argumento para o agente, então "/criar plano"
// invocaria "criar" com "plano" de entrada. Nome repetido também sai: dois itens
// iguais no menu fazem a pessoa escolher às cegas.
func commandsFrom(commands []sdk.AvailableCommand) []Command {
	out := make([]Command, 0, len(commands))
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		name := SanitizeLabel(command.Name)
		if name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		if _, repeated := seen[name]; repeated {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, Command{
			Name:         name,
			Description:  SanitizeLabel(command.Description),
			AcceptsInput: command.Input != nil,
		})
	}
	return out
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
// modeCategory é a categoria reservada do ACP para o modo da sessão.
const modeCategory = string(sdk.SessionConfigOptionCategoryMode)

func modeOptionFrom(state *sdk.SessionModeState) ConfigOption {
	option := ConfigOption{
		ID:           "mode",
		Category:     modeCategory,
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
	if hasCategory(options, modeCategory) {
		return options
	}
	return append(options, modeOptionFrom(state))
}

// legacyModelState é o formato anterior ao configOptions para dizer quais
// modelos o agente tem e qual está valendo. O SDK não o tipa — ele é anterior
// ao padrão e tende a sair —, e é por isso que a estrutura mora aqui, do lado
// de dentro do transporte, onde a saída de baixo nível existe justamente para
// os campos que o SDK não acompanha (AEP-0084 D2).
type legacyModelState struct {
	AvailableModels []legacyModel `json:"availableModels"`
	CurrentModelID  string        `json:"currentModelId"`
}

type legacyModel struct {
	ModelID string `json:"modelId"`
	Name    string `json:"name"`
}

// modelOptionFrom converte o formato legado de modelos numa ConfigOption, pelo
// mesmo desenho do modo: o app tem um caminho só, e quem escolhe modelo procura
// pela categoria sem saber por qual dos dois formatos ela chegou.
//
// Um modelo sem identificador é descartado: o identificador é o que a troca
// manda de volta ao agente, e uma linha na lista que não dá para escolher só
// serviria para a pessoa tentar.
func modelOptionFrom(state *legacyModelState) ConfigOption {
	option := ConfigOption{
		ID:       "model",
		Category: CategoryModel,
		// Os identificadores vão aparados dos dois lados, e o corrente pelo
		// mesmo critério dos oferecidos: é ele que a tela compara com a lista
		// para marcar o escolhido, e é o valor da lista que volta ao agente na
		// troca. Guardar um com espaço e o outro sem faria a tela não achar o
		// modelo em uso e a troca mandar de volta um espaço que ninguém pediu.
		CurrentValue: strings.TrimSpace(state.CurrentModelID),
	}
	for _, model := range state.AvailableModels {
		id := strings.TrimSpace(model.ModelID)
		if id == "" {
			continue
		}
		option.Values = append(option.Values, ConfigValue{Value: id, Name: model.Name})
	}
	return option
}

// withModelOption acrescenta os modelos legados só quando o agente não mandou a
// mesma informação no formato estável. Sem modelo nenhum na lista não há opção:
// um seletor vazio diria que a escolha existe e não deixaria escolher.
func withModelOption(options []ConfigOption, state *legacyModelState) []ConfigOption {
	if state == nil || hasCategory(options, CategoryModel) {
		return options
	}
	option := modelOptionFrom(state)
	if len(option.Values) == 0 {
		return options
	}
	return append(options, option)
}

// withKnownLegacy preserva o que a sessão já conhecia e o conjunto novo não
// trouxe. Acontece com quem anuncia modo ou modelo pelo formato legado: aquilo
// chega uma vez, na abertura da sessão, e o conjunto que o agente manda depois
// fala só do que ele guarda em configOptions. Sem isto, o seletor sumiria da
// tela no meio da conversa por causa de uma troca na outra categoria.
func withKnownLegacy(fresh, known []ConfigOption) []ConfigOption {
	if len(fresh) == 0 {
		return fresh
	}
	for _, category := range []string{modeCategory, CategoryModel} {
		if hasCategory(fresh, category) {
			continue
		}
		if option, ok := OptionByCategory(known, category); ok {
			fresh = append(fresh, option)
		}
	}
	return fresh
}

func hasCategory(options []ConfigOption, category string) bool {
	_, ok := OptionByCategory(options, category)
	return ok
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
			// Imagem sem tipo viraria um mimeType vazio no fio, e o agente
			// falharia de um jeito difícil de ligar de volta ao anexo.
			if strings.TrimSpace(item.ImageMIME) == "" {
				return nil, errors.New("anexo de imagem sem tipo MIME")
			}
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
