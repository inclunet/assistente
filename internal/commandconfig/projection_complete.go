package commandconfig

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandjson"
)

// TriggerPort é a porta confiável de um adapter de acionador. A configuração
// persistida fornece somente o tipo e o documento; a porta é quem conhece a
// gramática física/estruturada e produz a identidade que o resolver usa.
// Implementações devem ser registradas pelo bootstrap, nunca criadas a partir
// de dados da UI ou do banco.
type TriggerPort interface {
	Normalize(context.Context, []byte) (string, error)
}

// TriggerPortFunc adapta uma função de adapter à porta confiável.
type TriggerPortFunc func(context.Context, []byte) (string, error)

func (f TriggerPortFunc) Normalize(ctx context.Context, spec []byte) (string, error) {
	if f == nil {
		return "", ErrInvalid
	}
	return f(ctx, spec)
}

// TriggerIdentityValidator é opcional. Quando fornecido pela porta, também
// valida identidades de defaults builtin, que já chegam normalizadas pelo
// código e não possuem documento persistido para passar por Normalize.
type TriggerIdentityValidator interface {
	ValidateIdentity(context.Context, string) error
}

// CompleteProjection reúne apenas provas/serviços confiáveis do bootstrap.
// ActiveUserLayerIDs não é uma lista de configuração: é o resultado já
// autenticado do contexto ativo e não deve ser preenchido pela UI.
//
// ExecutionScope, quando necessário, também é uma porta confiável. Ela recebe
// a definição do catálogo e argumentos canônicos; não deve confiar em texto
// bruto nem conceder autorização. Comando sem alvo mutável usa "global" por
// padrão. Comando com alvo mutável sem essa porta falha fechado.
type CompleteProjection struct {
	Registry           *commandcatalog.Registry
	BuiltinLayers      []BuiltinLayer
	ActiveUserLayerIDs []string
	TriggerPorts       map[commandcatalog.Source]TriggerPort
	ExecutionScope     func(commandcatalog.Definition, []byte, Scope) (string, error)
}

// CompleteProjectionOptions é um nome compatível e mais explícito para
// CompleteProjection. O tipo canônico da API é CompleteProjection.
type CompleteProjectionOptions = CompleteProjection

// ProjectComplete materializa a configuração completa para o escopo do
// snapshot. Inclui defaults builtin e deltas de camadas globais e, quando o
// snapshot é local, também de workspace. Todo documento é validado antes de
// participar do resolver, inclusive quando o binding está desabilitado.
//
// A função é pura: não escreve, não ativa camadas, não registra adapters e não
// autoriza comandos. O chamador deve revalidar autenticação, geração e
// contexto antes de publicar/usar o resultado.
func ProjectComplete(ctx context.Context, snapshot Snapshot, options CompleteProjection) (*commandbindings.Configuration, error) {
	if ctx == nil || options.Registry == nil || !options.Registry.Complete() {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return nil, ErrInvalid
	}

	layers, active, err := completeUserLayers(snapshot, options.ActiveUserLayerIDs)
	if err != nil {
		return nil, err
	}
	builtins, defaultOwners, defaults, err := completeBuiltinLayers(ctx, options)
	if err != nil {
		return nil, err
	}

	custom := make([]commandbindings.Candidate, 0, len(snapshot.Bindings))
	deltas := make([]commandbindings.Delta, 0, len(snapshot.Bindings))
	for _, row := range snapshot.Bindings {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate, delta, isDelta, err := completeBinding(ctx, snapshot.Scope, row, layers, active, builtins, defaultOwners, options)
		if err != nil {
			return nil, ErrInvalid
		}
		if isDelta {
			deltas = append(deltas, delta)
		} else {
			custom = append(custom, candidate)
		}
	}

	configuration, err := commandbindings.NewConfiguration(defaults, deltas, custom)
	if err != nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return configuration, nil
}

type completeLayerState struct {
	layer  Layer
	active bool
}

func completeUserLayers(snapshot Snapshot, activeIDs []string) (map[string]completeLayerState, map[string]bool, error) {
	layers := make(map[string]completeLayerState, len(snapshot.Layers))
	for _, layer := range snapshot.Layers {
		if layer.Source != "user" || strings.TrimSpace(layer.Name) != layer.Name || strings.TrimSpace(layer.Description) != layer.Description {
			return nil, nil, ErrInvalid
		}
		if _, exists := layers[layer.ID]; exists {
			return nil, nil, ErrInvalid
		}
		layers[layer.ID] = completeLayerState{layer: layer}
	}

	active := make(map[string]bool, len(activeIDs))
	for _, id := range activeIDs {
		if strings.TrimSpace(id) != id || id == "" || active[id] {
			return nil, nil, ErrInvalid
		}
		state, exists := layers[id]
		if !exists || !state.layer.Enabled {
			return nil, nil, ErrInvalid
		}
		state.active = true
		layers[id] = state
		active[id] = true
	}
	return layers, active, nil
}

func completeBuiltinLayers(ctx context.Context, options CompleteProjection) (map[string]BuiltinLayer, map[string]string, []commandbindings.Default, error) {
	builtins := make(map[string]BuiltinLayer, len(options.BuiltinLayers))
	owners := make(map[string]string)
	defaults := make([]commandbindings.Default, 0)
	seenDefaultIDs := make(map[string]bool)
	for _, layer := range options.BuiltinLayers {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		if !namespacedID(layer.ID) || layer.ID != strings.TrimSpace(layer.ID) {
			return nil, nil, nil, ErrInvalid
		}
		if _, exists := builtins[layer.ID]; exists {
			return nil, nil, nil, ErrInvalid
		}
		builtins[layer.ID] = layer
		for _, input := range layer.Defaults {
			d := input
			if d.Candidate.ID == "" || seenDefaultIDs[d.Candidate.ID] {
				return nil, nil, nil, ErrInvalid
			}
			seenDefaultIDs[d.Candidate.ID] = true
			source, ok := sourceForTriggerIdentity(d.Candidate.Trigger)
			if !ok || !registeredTriggerPort(options.TriggerPorts, source) {
				return nil, nil, nil, ErrInvalid
			}
			definition, ok := options.Registry.Lookup(d.Candidate.CommandID)
			if !ok || !definition.IsComplete() || !definition.AllowsSource(source) {
				return nil, nil, nil, ErrInvalid
			}
			arguments, err := validateBindingArguments(options.Registry, definition, d.Candidate.ArgumentsKey)
			if err != nil || string(arguments) != d.Candidate.ArgumentsKey {
				return nil, nil, nil, ErrInvalid
			}
			if validator, ok := options.TriggerPorts[source].(TriggerIdentityValidator); ok {
				if err := validator.ValidateIdentity(ctx, d.Candidate.Trigger); err != nil {
					return nil, nil, nil, ErrInvalid
				}
			}
			d.Candidate.LayerActive = layer.Active
			if d.Candidate.LayerPriority != layer.ResolutionPriority {
				return nil, nil, nil, ErrInvalid
			}
			owners[d.Candidate.ID] = layer.ID
			defaults = append(defaults, d)
		}
	}
	return builtins, owners, defaults, nil
}

func completeBinding(ctx context.Context, scope Scope, row Binding, layers map[string]completeLayerState, active map[string]bool, builtins map[string]BuiltinLayer, defaultOwners map[string]string, options CompleteProjection) (commandbindings.Candidate, commandbindings.Delta, bool, error) {
	source, ok := completeSource(row.TriggerType)
	if !ok || !registeredTriggerPort(options.TriggerPorts, source) {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	trigger, err := normalizeTrustedTrigger(ctx, source, row.TriggerSpec, options.TriggerPorts)
	if err != nil {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	condition, err := decodeCondition(row.Condition)
	if err != nil {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	if err := validateCompletePresentation(row.Presentation); err != nil {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}

	layerActive, layerPriority, err := completeBindingLayer(row, layers, active, builtins)
	if err != nil {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	if row.Source != "user" {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	if row.ReviewStatus == "needs_review" && row.ReplacesDefaultID == nil {
		// commandbindings representa pendências de defaults, mas não possui um
		// estado de review para candidato novo. Recusar evita fallback implícito.
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}

	commandID := ""
	arguments := []byte("{}")
	var definition commandcatalog.Definition
	if row.CommandID != nil {
		commandID = *row.CommandID
		var exists bool
		definition, exists = options.Registry.Lookup(commandID)
		if !exists || !definition.IsComplete() || !definition.AllowsSource(source) {
			return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
		}
		arguments, err = validateBindingArguments(options.Registry, definition, row.Arguments)
		if err != nil {
			return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
		}
	} else if row.Effect != "suppress" || row.Arguments != "{}" {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	if row.Effect == "suppress" && row.CommandID != nil {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	if row.Effect != "execute" && row.Effect != "suppress" {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}

	if row.ReplacesDefaultID != nil {
		if row.ReplacesDefaultVersion == nil || row.ReplacesDefaultFingerprint == nil {
			return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
		}
		if row.LayerRefKind == "builtin" {
			if owner, exists := defaultOwners[*row.ReplacesDefaultID]; exists && owner != row.LayerRef {
				return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
			}
		}
		if row.Effect == "suppress" && row.Arguments != "{}" {
			return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
		}
		return commandbindings.Candidate{}, commandbindings.Delta{
			ID: row.ID, DefaultID: *row.ReplacesDefaultID, DefaultVersion: *row.ReplacesDefaultVersion,
			DefaultFingerprint: *row.ReplacesDefaultFingerprint, Trigger: trigger,
			Effect: commandbindings.DeltaEffect(row.Effect), CommandID: commandID, ArgumentsKey: deltaArgumentsKey(row.Effect, arguments),
			Condition: condition, Enabled: row.Enabled, LayerActive: layerActive,
			ReviewStatus: commandbindings.ReviewStatus(row.ReviewStatus), LayerPriority: layerPriority,
			BindingPriority: row.ResolutionPriority,
		}, true, nil
	}
	if row.LayerRefKind != "user" || row.ReviewStatus != "active" {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	executionScope, err := completeExecutionScope(definition, arguments, scope, options)
	if err != nil {
		return commandbindings.Candidate{}, commandbindings.Delta{}, false, ErrInvalid
	}
	return commandbindings.Candidate{ID: row.ID, Trigger: trigger, CommandID: commandID,
		ArgumentsKey: string(arguments), ExecutionScopeKey: executionScope, Scope: commandbindings.ExplicitLayer,
		Condition: condition, LayerPriority: layerPriority, BindingPriority: row.ResolutionPriority,
		Enabled: row.Enabled, LayerActive: layerActive}, commandbindings.Delta{}, false, nil
}

func deltaArgumentsKey(effect string, arguments []byte) string {
	if effect == "suppress" {
		return ""
	}
	return string(arguments)
}

func completeBindingLayer(row Binding, layers map[string]completeLayerState, active map[string]bool, builtins map[string]BuiltinLayer) (bool, int, error) {
	switch row.LayerRefKind {
	case "user":
		state, ok := layers[row.LayerRef]
		if !ok || !sameWorkspace(state.layer.WorkspaceID, row.WorkspaceID) {
			return false, 0, ErrInvalid
		}
		return active[row.LayerRef] && state.layer.Enabled, state.layer.ResolutionPriority, nil
	case "builtin":
		layer, ok := builtins[row.LayerRef]
		if !ok || row.ReplacesDefaultID == nil {
			return false, 0, ErrInvalid
		}
		return layer.Active, layer.ResolutionPriority, nil
	default:
		return false, 0, ErrInvalid
	}
}

func completeExecutionScope(definition commandcatalog.Definition, arguments []byte, scope Scope, options CompleteProjection) (string, error) {
	if options.ExecutionScope == nil {
		if definition.HasMutableTarget {
			return "", ErrInvalid
		}
		return "global", nil
	}
	value, err := options.ExecutionScope(definition, arguments, scope)
	if err != nil || value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", ErrInvalid
	}
	return value, nil
}

func validateBindingArguments(registry *commandcatalog.Registry, definition commandcatalog.Definition, raw string) ([]byte, error) {
	canonical, err := registry.ValidateArguments(definition.ID, []byte(raw))
	if err != nil {
		return nil, ErrInvalid
	}
	if definition.Persistence.Arguments == commandcatalog.PersistenceNever && string(canonical) != "{}" {
		return nil, ErrInvalid
	}
	for _, path := range definition.SensitivePaths.Input {
		present, err := jsonPointerPresent(canonical, path)
		if err != nil || present {
			return nil, ErrInvalid
		}
	}
	return canonical, nil
}

func normalizeTrustedTrigger(ctx context.Context, source commandcatalog.Source, raw string, ports map[commandcatalog.Source]TriggerPort) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := strictObject(raw); err != nil {
		return "", ErrInvalid
	}
	port := ports[source]
	identity, err := port.Normalize(ctx, []byte(raw))
	if err != nil || identity == "" || strings.TrimSpace(identity) != identity || !utf8.ValidString(identity) || strings.ContainsRune(identity, '\x00') || !strings.HasPrefix(identity, string(source)+":") {
		return "", ErrInvalid
	}
	return identity, nil
}

func registeredTriggerPort(ports map[commandcatalog.Source]TriggerPort, source commandcatalog.Source) bool {
	port, ok := ports[source]
	return ok && port != nil
}

func completeSource(raw string) (commandcatalog.Source, bool) {
	source := commandcatalog.Source(raw)
	switch source {
	case commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck,
		commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat, commandcatalog.CLI, commandcatalog.Event:
		return source, true
	default:
		return "", false
	}
}

func sourceForTriggerIdentity(identity string) (commandcatalog.Source, bool) {
	for _, source := range []commandcatalog.Source{
		commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck,
		commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat, commandcatalog.CLI, commandcatalog.Event,
	} {
		if strings.HasPrefix(identity, string(source)+":") {
			return source, true
		}
	}
	return "", false
}

func namespacedID(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return false
		}
		for i, char := range part {
			if (char < 'a' || char > 'z') && (i == 0 || char < '0' || char > '9' && char != '_') {
				return false
			}
		}
	}
	return true
}

func validateCompletePresentation(raw string) error {
	fields, err := strictObject(raw)
	if err != nil || !versionOne(fields) {
		return ErrInvalid
	}
	allowed := map[string]bool{"version": true, "title_key": true, "status_label_keys": true, "title_by_locale": true, "icon": true}
	for name := range fields {
		if !allowed[name] {
			return ErrInvalid
		}
	}
	if value, ok := fields["title_key"]; ok && !validPresentationTokenValue(value) {
		return ErrInvalid
	}
	if value, ok := fields["icon"]; ok && !validPresentationTokenValue(value) {
		return ErrInvalid
	}
	if value, ok := fields["status_label_keys"]; ok {
		if err := validatePresentationMap(value, true); err != nil {
			return err
		}
	}
	if value, ok := fields["title_by_locale"]; ok {
		if err := validatePresentationMap(value, false); err != nil {
			return err
		}
	}
	_, err = commandjson.Canonicalize([]byte(raw))
	if err != nil {
		return ErrInvalid
	}
	return nil
}

func validPresentationKey(raw json.RawMessage) bool {
	value, ok := jsonString(raw)
	return ok && value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00') && len([]rune(value)) <= 256
}

func validatePresentationMap(raw json.RawMessage, keysAreTokens bool) error {
	fields, err := strictObject(string(raw))
	if err != nil {
		return ErrInvalid
	}
	for key, value := range fields {
		if key == "" || strings.TrimSpace(key) != key || !utf8.ValidString(key) {
			return ErrInvalid
		}
		if keysAreTokens && !validPresentationTokenLocal(key) {
			return ErrInvalid
		}
		if !keysAreTokens {
			switch key {
			case "pt-BR", "en", "es":
			default:
				return ErrInvalid
			}
		}
		if !validPresentationKey(value) {
			return ErrInvalid
		}
	}
	return nil
}

func validPresentationTokenValue(raw json.RawMessage) bool {
	value, ok := jsonString(raw)
	return ok && validPresentationTokenLocal(value)
}

func validPresentationTokenLocal(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && !strings.ContainsRune("._:/-", char) {
			return false
		}
	}
	return true
}

func jsonPointerPresent(raw []byte, pointer string) (bool, error) {
	if pointer == "" {
		return true, nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return false, err
	}
	current := value
	for _, segment := range strings.Split(pointer[1:], "/") {
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		switch node := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = node[segment]
			if !ok {
				return false, nil
			}
		case []any:
			if segment == "" {
				return false, nil
			}
			index := 0
			for _, char := range segment {
				if char < '0' || char > '9' {
					return false, nil
				}
				index = index*10 + int(char-'0')
			}
			if index >= len(node) {
				return false, nil
			}
			current = node[index]
		default:
			return false, nil
		}
	}
	return true, nil
}
