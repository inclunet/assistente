package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"assistente/internal/commandconfig"
)

// Only validated configuration fields enter this presenter. Identity, receipts,
// grants and physical device identifiers are deliberately not presentation data.
func renderCommandSettingsDiff(locale string, diff commandconfig.MutationDiff) (string, error) {
	if diff.Scope.UserID == "" {
		return "", commandconfig.ErrInvalid
	}
	words := commandSettingsDiffWords(locale)
	type entry map[string]any
	before, after := map[string]entry{}, map[string]entry{}
	layerNames := map[string]string{}
	for _, row := range append(append([]commandconfig.Layer{}, diff.BeforeLayers...), diff.AfterLayers...) {
		layerNames[row.ID] = row.Name
	}
	layerName := func(id string) string {
		if value, ok := layerNames[id]; ok {
			return value
		}
		name, _ := commandSettingsBuiltinText(locale, id)
		return name
	}
	fill := func(out map[string]entry, layers []commandconfig.Layer, bindings []commandconfig.Binding, rulesBefore bool) error {
		for _, row := range layers {
			out["layer:"+row.ID] = entry{words["kind"]: words["layer"], words["name"]: row.Name, words["description"]: row.Description, words["enabled"]: row.Enabled, words["priority"]: row.ResolutionPriority}
		}
		for _, row := range bindings {
			trigger := row.TriggerSpec
			if row.TriggerType == "streamdeck.key" {
				var key struct {
					Key int `json:"key"`
				}
				if err := json.Unmarshal([]byte(trigger), &key); err != nil {
					return commandconfig.ErrInvalid
				}
				trigger = fmt.Sprintf("Stream Deck · %s %d", words["key"], key.Key+1)
			}
			command := ""
			if row.CommandID != nil {
				command = *row.CommandID
			}
			out["binding:"+row.ID] = entry{words["kind"]: words["binding"], words["layer"]: layerName(row.LayerRef), words["command"]: command, words["trigger"]: trigger, words["arguments"]: row.Arguments, words["condition"]: row.Condition, words["enabled"]: row.Enabled, words["priority"]: row.ResolutionPriority, words["effect"]: row.Effect, words["review"]: row.ReviewStatus}
			out["binding:"+row.ID][words["presentation"]] = row.Presentation
			if row.ReplacesDefaultVersion != nil {
				out["binding:"+row.ID][words["version"]] = *row.ReplacesDefaultVersion
			}
		}
		rules := diff.AfterActivationRules
		if rulesBefore {
			rules = diff.BeforeActivationRules
		}
		for _, row := range rules {
			out["rule:"+row.ID] = entry{words["kind"]: words["rule"], words["layer"]: layerName(row.LayerRef), words["mode"]: string(row.Mode), words["condition"]: row.Condition, words["enabled"]: row.Enabled, words["lifecycle"]: string(row.Lifecycle), words["review"]: row.ReviewStatus}
			if row.EventName != nil {
				out["rule:"+row.ID][words["event"]] = *row.EventName
			}
			if row.AllowedInternalProducerTypes != nil {
				out["rule:"+row.ID][words["producers"]] = *row.AllowedInternalProducerTypes
			}
		}
		grants := diff.AfterAutomationGrants
		if rulesBefore {
			grants = diff.BeforeAutomationGrants
		}
		for _, row := range grants {
			out["grant:"+row.ID] = entry{words["kind"]: words["authorization"], words["layer"]: layerName(row.LayerRef.Ref), words["event"]: row.EventName, words["enabled"]: row.RevokedAt == nil, words["generation"]: row.AutomationGrantGeneration}
		}
		return nil
	}
	if err := fill(before, diff.BeforeLayers, diff.BeforeBindings, true); err != nil {
		return "", err
	}
	if err := fill(after, diff.AfterLayers, diff.AfterBindings, false); err != nil {
		return "", err
	}
	ids := map[string]bool{}
	for id := range before {
		ids[id] = true
	}
	for id := range after {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	scope := words["global"]
	if diff.Scope.WorkspaceID != nil {
		scope = words["workspace"]
	}
	lines := []string{words["scope"] + ": " + scope}
	for _, id := range ordered {
		left, right := before[id], after[id]
		if reflect.DeepEqual(left, right) {
			continue
		}
		encode := func(value entry) string {
			visible := make(entry, len(value))
			for key, item := range value {
				visible[key] = item
			}
			if raw, ok := visible[words["presentation"]].(string); ok {
				var presentation map[string]any
				if json.Unmarshal([]byte(raw), &presentation) == nil && presentation["image_ref"] != nil {
					presentation["image_ref"] = words["custom_image"]
					encoded, _ := json.Marshal(presentation)
					visible[words["presentation"]] = string(encoded)
				}
			}
			raw, _ := json.MarshalIndent(visible, "", "  ")
			return string(raw)
		}
		switch {
		case left == nil:
			lines = append(lines, words["add"]+":\n"+encode(right))
		case right == nil:
			lines = append(lines, words["remove"]+":\n"+encode(left))
		default:
			lines = append(lines, words["before"]+":\n"+encode(left)+"\n"+words["after"]+":\n"+encode(right))
		}
	}
	if len(lines) == 1 {
		return "", commandconfig.ErrInvalid
	}
	return strings.Join(lines, "\n\n"), nil
}

func commandSettingsDiffWords(locale string) map[string]string {
	keys := []string{"kind", "layer", "name", "description", "enabled", "priority", "key", "binding", "command", "trigger", "arguments", "condition", "effect", "review", "version", "rule", "mode", "lifecycle", "global", "workspace", "scope", "add", "remove", "before", "after", "event", "producers", "authorization", "generation"}
	values := []string{"Tipo", "Camada", "Nome", "Descrição", "Habilitado", "Prioridade", "Tecla", "Acionador", "Comando", "Combinação", "Argumentos", "Condição", "Efeito", "Revisão", "Versão do padrão", "Regra de ativação", "Modo", "Duração", "Todos os workspaces", "Workspace atual", "Escopo", "Adicionar", "Remover", "Antes", "Depois", "Evento", "Produtores autorizados", "Autorização de automação", "Geração"}
	if locale == "en" {
		values = []string{"Type", "Layer", "Name", "Description", "Enabled", "Priority", "Key", "Binding", "Command", "Trigger", "Arguments", "Condition", "Effect", "Review", "Default version", "Activation rule", "Mode", "Lifetime", "All workspaces", "Current workspace", "Scope", "Add", "Remove", "Before", "After", "Event", "Authorized producers", "Automation authorization", "Generation"}
	}
	if locale == "es" {
		values = []string{"Tipo", "Capa", "Nombre", "Descripción", "Habilitado", "Prioridad", "Tecla", "Activador", "Comando", "Combinación", "Argumentos", "Condición", "Efecto", "Revisión", "Versión predeterminada", "Regla de activación", "Modo", "Duración", "Todos los espacios de trabajo", "Espacio de trabajo actual", "Ámbito", "Añadir", "Eliminar", "Antes", "Después", "Evento", "Productores autorizados", "Autorización de automatización", "Generación"}
	}
	out := make(map[string]string, len(keys))
	for i, key := range keys {
		out[key] = values[i]
	}
	out["presentation"] = "Apresentação"
	out["custom_image"] = "Imagem personalizada"
	if locale == "en" {
		out["presentation"] = "Presentation"
		out["custom_image"] = "Custom image"
	} else if locale == "es" {
		out["presentation"] = "Presentación"
		out["custom_image"] = "Imagen personalizada"
	}
	return out
}
