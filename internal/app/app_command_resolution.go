package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
)

const (
	commandPaletteLayerID  = "application.palette"
	commandKeyboardLayerID = "application.keyboard"
)

type commandKeyboardDefaultSpec struct {
	ID         string
	Code       string
	CommandID  string
	Prefix     string
	Bare       bool
	Condition  commandbindings.Facts
	Scope      commandbindings.Scope
	Contextual bool
}

type commandKeyboardSequenceDefaultSpec struct {
	ID        string
	CommandID string
	Prefix    string
	Code      string
}

var commandKeyboardSequenceDefaultSpecs = []commandKeyboardSequenceDefaultSpec{
	{ID: "builtin.keyboard.ctrl-n.workspace.tab.chat.create", CommandID: commandWorkspaceTabChatCreateID, Prefix: "Control+KeyN", Code: "KeyC"},
	{ID: "builtin.keyboard.ctrl-n.workspace.tab.editor.create", CommandID: commandWorkspaceTabEditorCreateID, Prefix: "Control+KeyN", Code: "KeyE"},
	{ID: "builtin.keyboard.ctrl-n.workspace.tab.terminal.create", CommandID: commandWorkspaceTabTerminalCreateID, Prefix: "Control+KeyN", Code: "KeyR"},
	{ID: "builtin.keyboard.ctrl-n.workspace.tab.tasklist.create", CommandID: commandWorkspaceTabTasklistCreateID, Prefix: "Control+KeyN", Code: "KeyT"},
}

var commandKeyboardDefaultSpecs = []commandKeyboardDefaultSpec{
	{ID: "builtin.keyboard.alt-w.navigation.workspace.open", Code: "KeyW", CommandID: "navigation.workspace.open"},
	{ID: "builtin.keyboard.alt-c.navigation.settings.open", Code: "KeyC", CommandID: "navigation.settings.open"},
	{ID: "builtin.keyboard.alt-h.navigation.history.open", Code: "KeyH", CommandID: "navigation.history.open"},
	{ID: "builtin.keyboard.alt-l.navigation.memories.open", Code: "KeyL", CommandID: "navigation.memories.open"},
	{ID: "builtin.keyboard.alt-t.navigation.tasklists.open", Code: "KeyT", CommandID: "navigation.tasklists.open"},
	{ID: "builtin.keyboard.alt-j.navigation.jobs.open", Code: "KeyJ", CommandID: "navigation.jobs.open"},
	{ID: "builtin.keyboard.alt-p.navigation.profiles.open", Code: "KeyP", CommandID: "navigation.profiles.open"},
	{ID: "builtin.keyboard.alt-backspace.navigation.workspace.open", Code: "Backspace", CommandID: "navigation.workspace.open"},
	{ID: "builtin.keyboard.alt-m.navigation.menu.open", Code: "KeyM", CommandID: "navigation.menu.open"},
	{ID: "builtin.keyboard.ctrl-k.navigation.palette.open", Code: "KeyK", CommandID: "navigation.palette.open", Prefix: "Control+"},
	{ID: "builtin.keyboard.alt-e.navigation.data.export.open", Code: "KeyE", CommandID: "navigation.data.export.open"},
	{ID: "builtin.keyboard.alt-i.navigation.data.import.open", Code: "KeyI", CommandID: "navigation.data.import.open"},
	{ID: "builtin.keyboard.ctrl-t.workspace.tab.chat.create", Code: "KeyT", CommandID: commandWorkspaceTabChatCreateID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-w.workspace.tab.close", Code: "KeyW", CommandID: commandWorkspaceTabCloseID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-f4.workspace.tab.close", Code: "F4", CommandID: commandWorkspaceTabCloseID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-tab.workspace.tab.next", Code: "Tab", CommandID: commandWorkspaceTabNextID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-shift-tab.workspace.tab.previous", Code: "Tab", CommandID: commandWorkspaceTabPreviousID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-page-down.workspace.tab.next", Code: "PageDown", CommandID: commandWorkspaceTabNextID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-page-up.workspace.tab.previous", Code: "PageUp", CommandID: commandWorkspaceTabPreviousID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-1.workspace.tab.first", Code: "Digit1", CommandID: commandWorkspaceTabFirstID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-2.workspace.tab.second", Code: "Digit2", CommandID: commandWorkspaceTabSecondID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-3.workspace.tab.third", Code: "Digit3", CommandID: commandWorkspaceTabThirdID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-4.workspace.tab.fourth", Code: "Digit4", CommandID: commandWorkspaceTabFourthID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-5.workspace.tab.fifth", Code: "Digit5", CommandID: commandWorkspaceTabFifthID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-6.workspace.tab.sixth", Code: "Digit6", CommandID: commandWorkspaceTabSixthID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-7.workspace.tab.seventh", Code: "Digit7", CommandID: commandWorkspaceTabSeventhID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-8.workspace.tab.eighth", Code: "Digit8", CommandID: commandWorkspaceTabEighthID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-9.workspace.tab.ninth", Code: "Digit9", CommandID: commandWorkspaceTabNinthID, Prefix: "Control+"},
	{ID: "builtin.keyboard.f1.navigation.help.open", Code: "F1", CommandID: "navigation.help.open", Bare: true},
	{ID: "builtin.keyboard.ctrl-shift-n.workspace.create", Code: "KeyN", CommandID: commandWorkspaceCreateID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-shift-i.workspace.chat.open", Code: "KeyI", CommandID: commandWorkspaceChatOpenID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-m.chat.model.open", Code: "KeyM", CommandID: commandChatModelOpenID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-h.chat.history.open", Code: "KeyH", CommandID: commandChatHistoryOpenID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-p.chat.profile.open", Code: "KeyP", CommandID: commandChatProfileOpenID, Prefix: "Control+"},
	{ID: "builtin.keyboard.alt-s.editor.slides.open", Code: "KeyS", CommandID: commandEditorSlidesOpenID},
	{ID: "builtin.keyboard.f5.editor.presentation.fullscreen", Code: "F5", CommandID: commandEditorPresentationFullscreenID, Bare: true},
	{ID: "builtin.keyboard.alt-i.editor.menu.insert.open", Code: "KeyI", CommandID: commandEditorMenuInsertOpenID, Condition: commandbindings.Facts{commandbindings.SurfaceType: "editor"}, Scope: commandbindings.Surface, Contextual: true},
	{ID: "builtin.keyboard.alt-1.editor.mode.markdown", Code: "Digit1", CommandID: commandEditorModeMarkdownID},
	{ID: "builtin.keyboard.alt-2.editor.mode.rich", Code: "Digit2", CommandID: commandEditorModeRichID},
	{ID: "builtin.keyboard.alt-3.editor.mode.view", Code: "Digit3", CommandID: commandEditorModeViewID},
	// Como editor.mode.*, o binding é resolvido sem fatos DOM. O comando
	// durável exige editor ativo por snapshot hostside e lease da UI.
	{ID: "builtin.keyboard.ctrl-s.editor.file.save", Code: "KeyS", CommandID: commandEditorFileSaveID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-o.editor.file.open", Code: "KeyO", CommandID: commandEditorFileOpenID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-shift-s.editor.file.save-copy", Code: "KeyS", CommandID: commandEditorFileSaveCopyID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-b.editor.format.bold", Code: "KeyB", CommandID: commandEditorFormatBoldID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-i.editor.format.italic", Code: "KeyI", CommandID: commandEditorFormatItalicID, Prefix: "Control+"},
	{ID: "builtin.keyboard.ctrl-shift-x.editor.format.strike", Code: "KeyX", CommandID: commandEditorFormatStrikeID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-alt-0.editor.format.paragraph", Code: "Digit0", CommandID: commandEditorFormatParagraphID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-alt-1.editor.format.heading.h1", Code: "Digit1", CommandID: commandEditorFormatHeading1ID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-alt-2.editor.format.heading.h2", Code: "Digit2", CommandID: commandEditorFormatHeading2ID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-alt-3.editor.format.heading.h3", Code: "Digit3", CommandID: commandEditorFormatHeading3ID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-alt-4.editor.format.heading.h4", Code: "Digit4", CommandID: commandEditorFormatHeading4ID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-alt-5.editor.format.heading.h5", Code: "Digit5", CommandID: commandEditorFormatHeading5ID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-alt-6.editor.format.heading.h6", Code: "Digit6", CommandID: commandEditorFormatHeading6ID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-shift-b.editor.format.blockquote", Code: "KeyB", CommandID: commandEditorFormatBlockquoteID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-alt-c.editor.format.code-block", Code: "KeyC", CommandID: commandEditorFormatCodeBlockID, Prefix: "Control+Alt+"},
	{ID: "builtin.keyboard.ctrl-shift-8.editor.format.list.bullet", Code: "Digit8", CommandID: commandEditorFormatListBulletID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-shift-7.editor.format.list.ordered", Code: "Digit7", CommandID: commandEditorFormatListOrderedID, Prefix: "Control+Shift+"},
	{ID: "builtin.keyboard.ctrl-l.chat.conversation.clear", Code: "KeyL", CommandID: commandConversationClearID, Prefix: "Control+"},
	{ID: "builtin.keyboard.f6.navigation.landmark.next", Code: "F6", CommandID: "navigation.landmark.next", Bare: true},
	{ID: "builtin.keyboard.shift-f6.navigation.landmark.previous", Code: "F6", CommandID: "navigation.landmark.previous", Prefix: "Shift+"},
	{ID: "builtin.keyboard.ctrl-n.tasklists.create.open", Code: "KeyN", CommandID: "tasklists.create.open", Prefix: "Control+", Condition: commandbindings.Facts{commandbindings.SurfaceType: "tasklists"}, Scope: commandbindings.Surface, Contextual: true},
	{ID: "builtin.keyboard.ctrl-n.profiles.create.open", Code: "KeyN", CommandID: "profiles.create.open", Prefix: "Control+", Condition: commandbindings.Facts{commandbindings.SurfaceType: "profiles"}, Scope: commandbindings.Surface, Contextual: true},
	{ID: "builtin.keyboard.ctrl-n.history.workspace.open", Code: "KeyN", CommandID: "navigation.workspace.open", Prefix: "Control+", Condition: commandbindings.Facts{commandbindings.SurfaceType: "history"}, Scope: commandbindings.Surface, Contextual: true},
}

func commandPaletteCandidate(invocationID, correlationID, selection string, arguments json.RawMessage) (commandexecution.EnvelopeCandidate, error) {
	args, err := commandjson.Canonicalize(arguments)
	if err != nil || string(args) != "{}" {
		return commandexecution.EnvelopeCandidate{}, commandexecution.ErrInvalidRequest
	}
	spec, err := commandjson.Marshal(map[string]any{"version": 1, "selection": selection})
	if err != nil {
		return commandexecution.EnvelopeCandidate{}, commandexecution.ErrInvalidRequest
	}
	return commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: correlationID,
		TriggerType: string(commandcatalog.Palette), TriggerSpec: spec, Arguments: args}, nil
}

// Uma só configuração de projeção serve ao rebuild e ao despacho. Adicionar
// uma gramática não publica a origem física: ela ainda exige ingresso próprio.
func commandProductTriggerPorts() map[commandcatalog.Source]commandconfig.TriggerPort {
	return map[commandcatalog.Source]commandconfig.TriggerPort{
		commandcatalog.KeyboardLocal:  commandconfig.KeyboardLocalTriggerPort{},
		commandcatalog.KeyboardGlobal: commandconfig.KeyboardGlobalTriggerPort{},
		commandcatalog.StreamDeck:     commandconfig.StreamDeckTriggerPort{},
		commandcatalog.Palette:        commandconfig.PaletteTriggerPort{},
	}
}

func commandProductProjection(registry *commandcatalog.Registry, active []string) (commandconfig.CompleteProjection, error) {
	paletteLayer := commandconfig.BuiltinLayer{ID: commandPaletteLayerID, Active: true}
	for _, definition := range registry.List() {
		// A resolução é comum. Efeito UI ainda exige reserva/handoff próprio;
		// publicar o default não autoriza executá-lo pelo ingresso de backend.
		// Ações de camada dependem de regra/escopo escolhidos pelo usuário.
		// Não existe argumento padrão seguro para um binding automático.
		if !definition.AllowsSource(commandcatalog.Palette) || isCommandLayerAction(definition.ID) || isCommandToolExecutionID(definition.ID) {
			continue
		}
		args, err := definition.ValidateArguments(json.RawMessage(`{}`))
		if err != nil || (definition.HasMutableTarget && !isWorkspaceMutationCommand(definition.ID) && !isAuditedUIContextualCommand(definition.ID)) {
			return commandconfig.CompleteProjection{}, commandexecution.ErrInvalidConfiguration
		}
		candidate := commandbindings.Candidate{
			ID: "builtin.palette." + definition.ID, Trigger: "palette:" + definition.ID,
			CommandID: definition.ID, ArgumentsKey: string(args), ExecutionScopeKey: "global",
			Scope: commandbindings.Application, Enabled: true, LayerActive: true,
		}
		// Toda semântica executável participa; apresentação visual não participa.
		fingerprint, err := commandPaletteDefaultFingerprint(candidate, definition, paletteLayer.ID)
		if err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		paletteLayer.Defaults = append(paletteLayer.Defaults, commandbindings.Default{
			Candidate: candidate, Version: "1", Fingerprint: fingerprint,
		})
	}
	keyboardLayer := commandconfig.BuiltinLayer{ID: commandKeyboardLayerID, Active: true}
	for _, spec := range commandKeyboardDefaultSpecs {
		definition, exists := registry.Lookup(spec.CommandID)
		if !exists || !definition.AllowsSource(commandcatalog.KeyboardLocal) {
			return commandconfig.CompleteProjection{}, commandexecution.ErrInvalidConfiguration
		}
		args, err := definition.ValidateArguments(json.RawMessage(`{}`))
		if err != nil || (definition.HasMutableTarget && !isWorkspaceMutationCommand(definition.ID) && !isAuditedUIContextualCommand(definition.ID)) || string(args) != "{}" {
			return commandconfig.CompleteProjection{}, commandexecution.ErrInvalidConfiguration
		}
		scope := commandbindings.Application
		if spec.Contextual {
			scope = spec.Scope
		}
		candidate := commandbindings.Candidate{
			ID: spec.ID, Trigger: "keyboard.local:" + keyboardDefaultPrefix(spec) + spec.Code,
			CommandID: spec.CommandID, ArgumentsKey: string(args), ExecutionScopeKey: "global",
			Scope: scope, Condition: spec.Condition, Enabled: true, LayerActive: true,
		}
		fingerprint, err := commandKeyboardDefaultFingerprint(candidate, definition, keyboardLayer.ID)
		if err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		keyboardLayer.Defaults = append(keyboardLayer.Defaults, commandbindings.Default{
			Candidate: candidate, Version: "1", Fingerprint: fingerprint,
		})
	}
	for _, spec := range commandKeyboardSequenceDefaultSpecs {
		definition, exists := registry.Lookup(spec.CommandID)
		if !exists || !definition.AllowsSource(commandcatalog.KeyboardLocal) {
			return commandconfig.CompleteProjection{}, commandexecution.ErrInvalidConfiguration
		}
		args, err := definition.ValidateArguments(json.RawMessage(`{}`))
		if err != nil || string(args) != "{}" || (definition.HasMutableTarget && !isWorkspaceMutationCommand(definition.ID)) {
			return commandconfig.CompleteProjection{}, commandexecution.ErrInvalidConfiguration
		}
		candidate := commandbindings.Candidate{
			ID: spec.ID, Trigger: "keyboard.local:" + spec.Prefix + " " + spec.Code,
			CommandID: spec.CommandID, ArgumentsKey: string(args), ExecutionScopeKey: "global",
			Scope: commandbindings.Application, Enabled: true, LayerActive: true,
		}
		fingerprint, err := commandKeyboardSequenceDefaultFingerprint(candidate, definition, keyboardLayer.ID)
		if err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		keyboardLayer.Defaults = append(keyboardLayer.Defaults, commandbindings.Default{
			Candidate: candidate, Version: "2", Fingerprint: fingerprint,
		})
	}
	return commandconfig.CompleteProjection{Registry: registry, ActiveUserLayerIDs: active,
		BuiltinLayers: []commandconfig.BuiltinLayer{paletteLayer, keyboardLayer}, TriggerPorts: commandProductTriggerPorts(),
		// A configuração do binding permanece global; o alvo mutável do comando
		// contextual é capturado e validado pelo provider workspace/active_tab
		// (ExactVersion) e confirmado pelo CAS do handler. Sem esta porta, a
		// projeção completa rejeita o binding antes de apresentar a decisão.
		ExecutionScope: func(definition commandcatalog.Definition, _ []byte, _ commandconfig.Scope) (string, error) {
			// O bucket global é somente a chave de resolução dos bindings.
			// Para comandos read-only ele é também o escopo efetivo; para o
			// comando contextual, o alvo real vem do fact active_tab versionado.
			if definition.HasMutableTarget && definition.ID != commandGlobalJobID && !isWorkspaceMutationCommand(definition.ID) && !isAuditedUIContextualCommand(definition.ID) && !isCommandLayerAction(definition.ID) {
				return "", commandexecution.ErrInvalidConfiguration
			}
			return "global", nil
		}}, nil
}

func keyboardDefaultPrefix(spec commandKeyboardDefaultSpec) string {
	if spec.Bare {
		return ""
	}
	if spec.Prefix != "" {
		return spec.Prefix
	}
	return "Alt+"
}

func commandPaletteDefaultFingerprint(candidate commandbindings.Candidate, definition commandcatalog.Definition, layer string) (string, error) {
	definition.Presentation = nil
	raw, err := commandjson.Marshal(map[string]any{
		"version": 1, "binding": candidate, "definition": definition,
		"adapter": "palette.selection.v1", "layer": layer,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func commandKeyboardDefaultFingerprint(candidate commandbindings.Candidate, definition commandcatalog.Definition, layer string) (string, error) {
	definition.Presentation = nil
	raw, err := commandjson.Marshal(map[string]any{
		"version": 1, "binding": candidate, "definition": definition,
		"adapter": "keyboard.local.v1", "layer": layer,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func commandKeyboardSequenceDefaultFingerprint(candidate commandbindings.Candidate, definition commandcatalog.Definition, layer string) (string, error) {
	definition.Presentation = nil
	raw, err := commandjson.Marshal(map[string]any{
		"version": 2, "binding": candidate, "definition": definition,
		"adapter": "keyboard.local.v2.steps", "layer": layer,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// resolvePersistedTrigger roda dentro do gate do executor. Usa somente a
// projeção publicada e fontes locais; nunca roundtrip Wails ou leitura de disco.
func (p *commandProductRuntime) commandOccurrenceOriginFacts(ctx context.Context, candidate commandexecution.EnvelopeCandidate, required []commandbindings.Field) (commandOriginContext, error) {
	source := commandcatalog.Source(candidate.TriggerType)
	switch source {
	case commandcatalog.Palette:
		if occurrence := p.contextualPaletteLayerOccurrence(ctx, candidate.InvocationID); occurrence != nil {
			if ctx.Err() != nil || !occurrence.valid() {
				return commandOriginContext{}, commandexecution.ErrStale
			}
			return workspaceVisualCommandFacts(occurrence.proof, required)
		}
		if facts, exists, err := p.paletteWorkspaceOccurrenceFacts(ctx, candidate, required); exists {
			return facts, err
		}
		return p.commandOriginFacts(ctx, source, required)
	case commandcatalog.KeyboardGlobal:
		if occurrence, ok := p.globalOccurrence(candidate.InvocationID); ok {
			if occurrence.foreground != nil || containsPhysicalOriginField(required) {
				return p.commandOriginFactsFromSnapshot(ctx, source, required, "", occurrence.foreground)
			}
			if containsPhysicalOriginField(required) {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
		}
		if containsPhysicalOriginField(required) {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
		return p.commandOriginFacts(ctx, source, required)
	case commandcatalog.StreamDeck:
		if occurrence := p.contextualDeckLayerOccurrence(ctx, candidate.InvocationID); occurrence != nil && (occurrence.valid == nil || !occurrence.valid()) {
			return commandOriginContext{}, commandexecution.ErrStale
		}
		if occurrence, ok := commandDeckOccurrenceFor(p, candidate.InvocationID); ok {
			// Device-only bindings deliberately carry no foreground snapshot;
			// FromSnapshot still proves the canonical serial and workspace version.
			return p.deckOccurrenceOriginFacts(ctx, occurrence, required)
		}
		if containsPhysicalOriginField(required) {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
		identity, err := (commandconfig.StreamDeckTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
		if err != nil {
			return commandOriginContext{}, err
		}
		trigger, err := commandconfig.ParseStreamDeckTriggerIdentity(identity)
		if err != nil {
			return commandOriginContext{}, err
		}
		return p.commandOriginFactsWithDevice(ctx, source, required, trigger.Device)
	default:
		return p.commandOriginFacts(ctx, source, required)
	}
}

func (p *commandProductRuntime) resolvePersistedTrigger(ctx context.Context, owner auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate, envelope commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
	if p == nil || owner != p.principal || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) ||
		candidate.CommandID != "" || envelope.SourceType == nil || string(*envelope.SourceType) != candidate.TriggerType ||
		(candidate.TriggerType != string(commandcatalog.Palette) && candidate.TriggerType != string(commandcatalog.KeyboardLocal) && candidate.TriggerType != string(commandcatalog.StreamDeck) && candidate.TriggerType != string(commandcatalog.KeyboardGlobal)) {
		return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
	}
	configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, owner)
	if err != nil || envelope.RegistryVersion != versions.Registry ||
		envelope.GlobalConfigGeneration == nil || *envelope.GlobalConfigGeneration != versions.GlobalConfig ||
		envelope.ActiveLayersGeneration == nil || *envelope.ActiveLayersGeneration != versions.ActiveLayers {
		return commandexecution.EnvelopeResolution{}, commandexecution.ErrStale
	}
	var identity string
	if candidate.TriggerType == string(commandcatalog.KeyboardLocal) {
		identity, err = (commandconfig.KeyboardLocalTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
	} else if candidate.TriggerType == string(commandcatalog.KeyboardGlobal) {
		identity, err = (commandconfig.KeyboardGlobalTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
		o, ok := p.globalOccurrence(candidate.InvocationID)
		if !ok || o.binding.Identity != identity || o.versions != versions {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
	} else if candidate.TriggerType == string(commandcatalog.StreamDeck) {
		identity, err = (commandconfig.StreamDeckTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
	} else {
		identity, err = (commandconfig.PaletteTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
	}
	if err != nil {
		return commandexecution.EnvelopeResolution{}, err
	}
	// O ingresso baseado em trigger aceita somente o marcador vazio. Os
	// argumentos efetivos vêm da binding projetada, nunca do cliente.
	args, err := commandjson.Canonicalize(candidate.Arguments)
	if err != nil {
		return commandexecution.EnvelopeResolution{}, commandexecution.ErrInvalidRequest
	}
	if string(args) != "{}" {
		return commandexecution.EnvelopeResolution{}, commandexecution.ErrInvalidRequest
	}
	mermaid := false
	facts := commandbindings.Facts{}
	originVersion := ""
	foregroundVersion := ""
	var keyboardOccurrence localCommandKeyboardOccurrence
	if candidate.TriggerType == string(commandcatalog.KeyboardLocal) {
		p.keyboardMu.Lock()
		occurrence, exists := p.keyboardEvents[candidate.InvocationID]
		mermaid = exists && occurrence.mermaid && occurrence.identity == identity && occurrence.state == p.keyboardMap && occurrence.state.ctx.Err() == nil
		valid := exists && occurrence.identity == identity && occurrence.state != nil && occurrence.state == p.keyboardMap && occurrence.state.ctx.Err() == nil
		keyboardOccurrence = occurrence
		p.keyboardMu.Unlock()
		if !valid || !p.localKeyboardContextCurrent(occurrence.context) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrStale
		}
		facts[commandbindings.AppFocused] = true
		if occurrence.context != nil {
			facts[commandbindings.SurfaceType] = occurrence.context.observed.SurfaceType
			facts[commandbindings.SurfaceID] = occurrence.context.observed.SurfaceID
			if localKeyboardProfileRequired(configuration, identity) {
				profile := localKeyboardEffectiveProfile(occurrence.context.snapshot)
				if occurrence.context.observed.Profile == "" || occurrence.context.observed.Profile != profile || profile == "" {
					return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
				}
				facts[commandbindings.Profile] = profile
			}
		}
	} else {
		required := configuration.RequiredFacts(identity)
		paletteLocalUI := false
		if candidate.TriggerType == string(commandcatalog.Palette) && len(required) > 0 && p.registry != nil {
			commandID := strings.TrimPrefix(identity, "palette:")
			if definition, ok := p.registry.Lookup(commandID); ok {
				paletteLocalUI = commandExecutionClassForDefinition(definition) == commandExecutionLocalUI
			}
		}
		if paletteLocalUI {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		originContext, originErr := p.commandOccurrenceOriginFacts(ctx, candidate, required)
		if originErr != nil {
			return commandexecution.EnvelopeResolution{}, originErr
		}
		facts = originContext.facts
		// The snapshot is checked again below after resolution. Its version is
		// retained here so an A-B-A workspace change cannot pass silently.
		originVersion = originContext.version
		if originContext.foreground != nil {
			foregroundVersion = originContext.foreground.Version
		}
	}
	cacheOriginVersion := originVersion
	if keyboardOccurrence.context != nil {
		cacheOriginVersion = keyboardOccurrence.context.snapshot.Version
	}
	resolved, err := p.resolveCachedBinding(ctx, configuration, identity, facts, cacheOriginVersion, foregroundVersion, envelope)
	if err != nil {
		return commandexecution.EnvelopeResolution{}, err
	}
	// For trigger-based ingress, candidate arguments are transport metadata, not
	// authority. The selected binding owns its persisted argument key and the
	// executor replaces the candidate arguments with resolved.ArgumentsKey
	// below. Comparing them here would reject every configured binding whose
	// action needs arguments (while still allowing a client to influence the
	// request if the values were trusted).
	if candidate.TriggerType != string(commandcatalog.KeyboardLocal) && originVersion != "" {
		currentContext, contextErr := p.commandOccurrenceOriginFacts(ctx, candidate, configuration.RequiredFacts(identity))
		if contextErr != nil || currentContext.version != originVersion {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrStale
		}
	}
	if candidate.TriggerType == string(commandcatalog.KeyboardGlobal) {
		o, ok := p.globalOccurrence(candidate.InvocationID)
		if !ok || !globalBindingMatches(o.binding, resolved) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		if originVersion != "" && o.originVersion != originVersion {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrStale
		}
	}
	if mermaid {
		// Modal invariants have no configurable binding or layer provenance.
		// Only the dedicated host ingress can mint this occurrence.
		var shortcut LocalCommandShortcut
		if json.Unmarshal(candidate.TriggerSpec, &shortcut) != nil || !mermaidSubmitShortcut(shortcut) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		resolved, err = commandbindings.Result{Status: commandbindings.Selected, CommandID: commandEditorMermaidApplyID, ArgumentsKey: "{}", ExecutionScopeKey: "global", BindingIDs: []string{}, LayerRefs: []string{}}, nil
	}
	// RequiredFacts pertence às condições dos bindings, não à ContextPolicy da
	// definição. Nenhum binding contextual pode ignorar condições pessoais.
	if !mermaid {
		for _, field := range configuration.RequiredFacts(identity) {
			if _, ok := facts[field]; !ok {
				return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
			}
		}
		if candidate.TriggerType == string(commandcatalog.KeyboardLocal) && resolved.Status == commandbindings.Selected && resolved.CommandID != keyboardOccurrence.binding.CommandID {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrStale
		}
	}
	if mermaid && (resolved.Status != commandbindings.Selected || !isMermaidMutation(resolved.CommandID)) {
		return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
	}
	result := commandexecution.EnvelopeResolution{BindingIDs: resolved.BindingIDs, LayerRefs: resolved.LayerRefs, ContextVersion: originVersion}
	// A claim-only publication may preserve an admitted execution only when
	// this invocation is independent of all mutable foreground/surface facts
	// and has no contextual palette occurrence. Its authenticated session and
	// immutable persisted authority base are checked by the publication path.
	contextNeutralIngress := candidate.TriggerType == string(commandcatalog.Palette) || candidate.TriggerType == string(commandcatalog.KeyboardGlobal)
	if contextNeutralIngress && resolved.Status == commandbindings.Selected &&
		len(facts) == 0 && originVersion == "" && foregroundVersion == "" && envelope.ContextVersion == nil &&
		p.contextualPaletteLayerOccurrence(ctx, candidate.InvocationID) == nil {
		result.ProjectionDependency, err = configuration.CaptureExecutionDependency(identity, facts, nil, resolved)
		if err != nil {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
	}
	switch resolved.Status {
	case commandbindings.Suppressed:
		result.Mode = commandcontract.ResolutionSuppress
	case commandbindings.Selected:
		if occurrence := p.contextualDeckLayerOccurrence(ctx, candidate.InvocationID); occurrence != nil && resolved.CommandID != occurrence.commandID {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		if occurrence := p.contextualPaletteLayerOccurrence(ctx, candidate.InvocationID); occurrence != nil && resolved.CommandID != occurrence.commandID {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		definition, exists := p.registry.Lookup(resolved.CommandID)
		if !exists || resolved.ExecutionScopeKey != "global" {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		uiCommand, uiReserved := p.reservedUICommand(candidate.InvocationID)
		class := commandExecutionClassForDefinition(definition)
		if class == commandExecutionLocalUI {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		if class == commandExecutionAuditedUI || isWorkspaceMutationCommand(definition.ID) {
			if !uiReserved || uiCommand != definition.ID {
				return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
			}
		} else if class != commandExecutionDurable || uiReserved {
			// Uma preparação de efeito visual não pode ser redirecionada para
			// uma operação de backend pela configuração do binding.
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		result.Mode = commandcontract.ResolutionExecute
		result.CommandID = resolved.CommandID
		result.Arguments = json.RawMessage(resolved.ArgumentsKey)
		result.Provenance, err = commandSelectedJobProvenance(configuration.LayerProvenance(resolved.LayerRefs))
		if err != nil {
			return commandexecution.EnvelopeResolution{}, err
		}
	default:
		result.Mode = commandcontract.ResolutionDenied
	}
	return result, nil
}

func (p *commandProductRuntime) reservedUICommand(invocationID string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || invocationID == "" {
		return "", false
	}
	// Reservas limitadas a 64: leitura local sem I/O sob o gate do executor.
	for _, run := range p.uiRuns {
		if run.reservation.InvocationID == invocationID {
			return run.reservation.CommandID, true
		}
	}
	return "", false
}
