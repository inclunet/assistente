package commandconfig

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

// LocalReadProjection contém somente dependências confiáveis do bootstrap, nunca
// documentos importados. NoArgumentCommands declara contratos de handlers que o
// catálogo atual ainda não representa. Fingerprints vêm do catálogo semântico
// completo; este projetor não os calcula a partir de Candidate.
type LocalReadProjection struct {
	Registry           *commandcatalog.Registry
	NoArgumentCommands []string
	BuiltinLayers      []BuiltinLayer
	ActiveUserLayerIDs []string
}

type BuiltinLayer struct {
	ID                 string
	Active             bool
	ResolutionPriority int
	Defaults           []commandbindings.Default
}

// ProjectLocalRead materializa a união global+workspace, keyboard.local, read/none,
// sem argumentos. Não registra teclas, executa handlers, restaura claims ou
// autoriza uma execução. Documentos não suportados recusam o snapshot inteiro,
// inclusive bindings desabilitados: nunca são ignorados com fallback implícito.
// O chamador ainda deve revalidar o stamp e autenticação antes da publicação.
func ProjectLocalRead(ctx context.Context, snapshot Snapshot, options LocalReadProjection) (*commandbindings.Configuration, error) {
	if ctx == nil || options.Registry == nil || !validScope(snapshot.Scope) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, id := range options.NoArgumentCommands {
		definition, ok := options.Registry.Lookup(id)
		if !ok || allowed[id] || definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision ||
			!definition.Context.None || definition.HasMutableTarget || definition.MutatesEffectiveCapability || !definition.AllowsSource(commandcatalog.KeyboardLocal) {
			return nil, ErrInvalid
		}
		allowed[id] = true
	}
	layers := map[string]Layer{}
	layersByID := map[string]Layer{}
	for _, layer := range snapshot.Layers {
		if layer.UserID != snapshot.Scope.UserID || !inScope(layer.WorkspaceID, snapshot.Scope) {
			return nil, ErrInvalid
		}
		key := layer.ID + "\x00" + workspaceProjectionKey(layer.WorkspaceID)
		if _, exists := layers[key]; exists {
			return nil, ErrInvalid
		}
		if _, exists := layersByID[layer.ID]; exists {
			return nil, ErrInvalid
		}
		layers[key] = layer
		layersByID[layer.ID] = layer
	}
	active := map[string]bool{}
	for _, id := range options.ActiveUserLayerIDs {
		layer, exists := layersByID[id]
		if !exists || !layer.Enabled || active[id] {
			return nil, ErrInvalid
		}
		active[id] = true
	}
	userActivation := map[string]layerActivationProjection{}
	for _, layer := range snapshot.Layers {
		projected, err := projectLayerActivation(snapshot, commandactivation.UserRef, layer.ID, layer.WorkspaceID, layer.Enabled, active[layer.ID])
		if err != nil {
			return nil, ErrInvalid
		}
		userActivation[layer.ID] = projected
	}
	builtins := map[string]BuiltinLayer{}
	defaultLayers := map[string]string{}
	var defaults []commandbindings.Default
	for _, layer := range options.BuiltinLayers {
		if !strings.Contains(layer.ID, ".") || strings.TrimSpace(layer.ID) != layer.ID {
			return nil, ErrInvalid
		}
		if _, exists := builtins[layer.ID]; exists {
			return nil, ErrInvalid
		}
		builtins[layer.ID] = layer
		for _, d := range layer.Defaults {
			c := d.Candidate
			if _, exists := defaultLayers[c.ID]; exists {
				return nil, ErrInvalid
			}
			if !allowed[c.CommandID] || c.ArgumentsKey != "{}" || c.ExecutionScopeKey != "global" || c.Scope != commandbindings.Global || c.LayerPriority != layer.ResolutionPriority ||
				!canonicalLocalTrigger(c.Trigger) || c.DialogID != "" {
				return nil, ErrInvalid
			}
			d.Candidate.LayerActive = layer.Active
			d.Candidate.LayerConditions = nil
			defaultLayers[c.ID] = layer.ID
			defaults = append(defaults, d)
		}
	}
	var custom []commandbindings.Candidate
	var deltas []commandbindings.Delta
	for _, row := range snapshot.Bindings {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		trigger, err := decodeKeyboard(row.TriggerType, row.TriggerSpec)
		if err != nil {
			return nil, ErrInvalid
		}
		condition, err := decodeCondition(row.Condition)
		if err != nil || !emptyDocument(row.Arguments) || !emptyDocument(row.Presentation) {
			return nil, ErrInvalid
		}
		command, arguments := "", ""
		if row.CommandID != nil {
			command = *row.CommandID
			if !allowed[command] {
				return nil, ErrInvalid
			}
			arguments = "{}"
		}
		layerActive, layerPriority := false, 0
		if row.LayerRefKind == "user" {
			layer, exists := findProjectionLayer(layers, row.LayerRef, row.WorkspaceID)
			if !exists {
				return nil, ErrInvalid
			}
			projected, ok := userActivation[row.LayerRef]
			if !ok {
				return nil, ErrInvalid
			}
			layerActive, layerPriority = projected.active, layer.ResolutionPriority
			layerConditions := projected.conditions
			if row.ReplacesDefaultID != nil {
				if owner, exists := defaultLayers[*row.ReplacesDefaultID]; exists && row.LayerRefKind == "builtin" && owner != row.LayerRef {
					return nil, ErrInvalid
				}
				deltas = append(deltas, commandbindings.Delta{
					ID: row.ID, DefaultID: *row.ReplacesDefaultID, DefaultVersion: *row.ReplacesDefaultVersion,
					DefaultFingerprint: *row.ReplacesDefaultFingerprint, Trigger: trigger,
					Effect: commandbindings.DeltaEffect(row.Effect), CommandID: command, ArgumentsKey: arguments,
					Condition: condition, Enabled: row.Enabled, LayerActive: layerActive, LayerConditions: layerConditions,
					ReviewStatus: commandbindings.ReviewStatus(row.ReviewStatus), LayerPriority: layerPriority, BindingPriority: row.ResolutionPriority,
				})
			} else {
				if row.ReviewStatus != "active" {
					return nil, ErrInvalid
				}
				custom = append(custom, commandbindings.Candidate{ID: row.ID, Trigger: trigger, CommandID: command,
					ArgumentsKey: arguments, ExecutionScopeKey: "global", Scope: commandbindings.ExplicitLayer,
					Condition: condition, Enabled: row.Enabled, LayerActive: layerActive, LayerConditions: layerConditions,
					LayerPriority: layerPriority, BindingPriority: row.ResolutionPriority})
			}
			continue
		} else {
			layer, exists := builtins[row.LayerRef]
			if !exists {
				return nil, ErrInvalid
			}
			layerActive, layerPriority = layer.Active, layer.ResolutionPriority
		}
		if row.ReplacesDefaultID != nil {
			if owner, exists := defaultLayers[*row.ReplacesDefaultID]; exists && row.LayerRefKind == "builtin" && owner != row.LayerRef {
				return nil, ErrInvalid
			}
			deltas = append(deltas, commandbindings.Delta{
				ID: row.ID, DefaultID: *row.ReplacesDefaultID, DefaultVersion: *row.ReplacesDefaultVersion,
				DefaultFingerprint: *row.ReplacesDefaultFingerprint, Trigger: trigger,
				Effect: commandbindings.DeltaEffect(row.Effect), CommandID: command, ArgumentsKey: arguments,
				Condition: condition, Enabled: row.Enabled, LayerActive: layerActive,
				LayerConditions: nil,
				ReviewStatus:    commandbindings.ReviewStatus(row.ReviewStatus), LayerPriority: layerPriority, BindingPriority: row.ResolutionPriority,
			})
		} else {
			// Um binding novo em revisão não pode ser reinterpretado como ativo.
			if row.ReviewStatus != "active" {
				return nil, ErrInvalid
			}
			custom = append(custom, commandbindings.Candidate{ID: row.ID, Trigger: trigger, CommandID: command,
				ArgumentsKey: arguments, ExecutionScopeKey: "global", Scope: commandbindings.ExplicitLayer,
				Condition: condition, Enabled: row.Enabled, LayerActive: layerActive,
				LayerConditions: nil,
				LayerPriority:   layerPriority, BindingPriority: row.ResolutionPriority})
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

func workspaceProjectionKey(workspace *string) string {
	if workspace == nil {
		return "global"
	}
	return *workspace
}

func findProjectionLayer(layers map[string]Layer, id string, workspace *string) (Layer, bool) {
	layer, ok := layers[id+"\x00"+workspaceProjectionKey(workspace)]
	return layer, ok
}

func canonicalLocalTrigger(value string) bool {
	const prefix = "keyboard.local"
	if !strings.HasPrefix(value, prefix+":") {
		return false
	}
	rest := strings.TrimPrefix(value, prefix+":")
	if strings.Contains(rest, " ") {
		parts := strings.Split(rest, " ")
		if len(parts) != 2 {
			return false
		}
		first := strings.Split(parts[0], "+")
		if len(first) < 2 {
			return false
		}
		modifiers := first[:len(first)-1]
		ordered, ok := canonicalKeyboardModifierValues(modifiers, true, false)
		if !ok || !slicesEqual(ordered, modifiers) {
			return false
		}
		if parts[1] == "Escape" {
			return false
		}
		return keyboardSequenceIdentity(
			KeyboardLocalStep{Code: first[len(first)-1], Modifiers: modifiers},
			KeyboardLocalStep{Code: parts[1]},
		) == value && validKeyboardCode(first[len(first)-1]) && validKeyboardCode(parts[1])
	}
	return canonicalKeyboardV1Trigger(prefix, value)
}

func canonicalKeyboardV1Trigger(prefix, value string) bool {
	fullPrefix := prefix + ":"
	if !strings.HasPrefix(value, fullPrefix) || strings.Contains(strings.TrimPrefix(value, fullPrefix), " ") {
		return false
	}
	rest := strings.TrimPrefix(value, fullPrefix)
	parts := strings.Split(rest, "+")
	if len(parts) == 0 {
		return false
	}
	// Marshal de dados confiáveis, reutilizando a mesma validação do documento:
	// defaults não podem usar outra ordem de modificadores ou alias de código.
	raw, err := json.Marshal(struct {
		Version   int      `json:"version"`
		Code      string   `json:"code"`
		Modifiers []string `json:"modifiers"`
	}{1, parts[len(parts)-1], parts[:len(parts)-1]})
	if err != nil {
		return false
	}
	fields, err := strictObject(string(raw))
	if err != nil {
		return false
	}
	normalized, err := decodeKeyboardV1WithPrefix(prefix, fields)
	return err == nil && normalized == value
}
