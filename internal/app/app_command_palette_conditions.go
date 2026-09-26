package app

import (
	"encoding/json"
	"slices"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

// LocalCommandPaletteCondition é uma projeção somente de apresentação para
// comandos locais ou contextuais duráveis. Ela não autoriza nem executa nada e não é persistida.
type LocalCommandPaletteCondition struct {
	CommandID            string                                  `json:"commandId"`
	BySurface            map[string]bool                         `json:"bySurface"`
	BySurfaceID          map[string]map[string]bool              `json:"bySurfaceId,omitempty"`
	FallbackArguments    map[string]any                          `json:"fallbackArguments,omitempty"`
	BySurfaceArguments   map[string]map[string]any               `json:"bySurfaceArguments,omitempty"`
	BySurfaceIDArguments map[string]map[string]map[string]any    `json:"bySurfaceIdArguments,omitempty"`
	ByProfile            map[string]LocalCommandPaletteCondition `json:"byProfile,omitempty"`
	ByPage               map[string]LocalCommandPaletteCondition `json:"byPage,omitempty"`
	Fallback             bool                                    `json:"fallback"`
}

func localPaletteUIConditions(configuration *commandbindings.Configuration, registry *commandcatalog.Registry) []LocalCommandPaletteCondition {
	return paletteConditionsForClass(configuration, registry, commandExecutionLocalUI)
}

func contextualPaletteUIConditions(configuration *commandbindings.Configuration, registry *commandcatalog.Registry) []LocalCommandPaletteCondition {
	conditions := paletteConditionsForClass(configuration, registry, commandExecutionDurable)
	conditions = append(conditions, paletteConditionsForClass(configuration, registry, commandExecutionAuditedUI)...)
	slices.SortFunc(conditions, func(left, right LocalCommandPaletteCondition) int {
		return strings.Compare(left.CommandID, right.CommandID)
	})
	return conditions
}

func paletteConditionsForClass(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, class commandExecutionClass) []LocalCommandPaletteCondition {
	if configuration == nil || registry == nil {
		return []LocalCommandPaletteCondition{}
	}
	conditions := make([]LocalCommandPaletteCondition, 0)
	for _, identity := range configuration.TriggerIdentities() {
		if !strings.HasPrefix(identity, "palette:") {
			continue
		}
		if condition, ok := paletteConditionForClass(configuration, registry, identity, class); ok {
			conditions = append(conditions, condition)
		}
	}
	slices.SortFunc(conditions, func(left, right LocalCommandPaletteCondition) int {
		return strings.Compare(left.CommandID, right.CommandID)
	})
	return conditions
}

func localPaletteUICondition(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string) (LocalCommandPaletteCondition, bool) {
	return paletteConditionForClass(configuration, registry, identity, commandExecutionLocalUI)
}

func paletteConditionForClass(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, class commandExecutionClass) (LocalCommandPaletteCondition, bool) {
	if configuration == nil || registry == nil || !strings.HasPrefix(identity, "palette:") {
		return LocalCommandPaletteCondition{}, false
	}
	fields := configuration.RequiredFacts(identity)
	if len(fields) == 0 {
		return LocalCommandPaletteCondition{}, false
	}
	hasSurfaceType, hasSurfaceID, hasProfile, hasPage := false, false, false, false
	for _, field := range fields {
		switch field {
		case commandbindings.AppFocused:
		case commandbindings.SurfaceType:
			hasSurfaceType = true
		case commandbindings.SurfaceID:
			hasSurfaceID = true
		case commandbindings.Profile:
			hasProfile = true
		case commandbindings.AppPage:
			hasPage = true
		default:
			// Process/device values are intentionally not enumerated. A local
			// palette projection must never expose arbitrary user data.
			return LocalCommandPaletteCondition{}, false
		}
	}
	definitionID := strings.TrimPrefix(identity, "palette:")
	if hasSurfaceID && isContextualPagePaletteCommand(definitionID) {
		// Page targets are captured by their own protocol, never surface IDs.
		// RequiredFacts includes inherited conditions and suppression barriers.
		return LocalCommandPaletteCondition{}, false
	}
	definition, ok := registry.Lookup(definitionID)
	if !ok || definition.ID != definitionID || !paletteConditionClassEligible(definition, class) || hasSurfaceID && !hasSurfaceType {
		return LocalCommandPaletteCondition{}, false
	}
	unknownProfile := ""
	if hasProfile {
		unknownProfile = contextualFallbackProfile(configuration.FieldValues(identity, commandbindings.Profile))
	}
	buildBranch := func(profile, appPage string) LocalCommandPaletteCondition {
		branch := localPaletteConditionLeaf(configuration, registry, identity, definitionID, profile, appPage, hasSurfaceType, hasSurfaceID, hasProfile, class)
		if hasProfile {
			branch.ByProfile = make(map[string]LocalCommandPaletteCondition)
			for _, listedProfile := range configuration.FieldValues(identity, commandbindings.Profile) {
				branch.ByProfile[listedProfile] = localPaletteConditionLeaf(configuration, registry, identity, definitionID, listedProfile, appPage, hasSurfaceType, hasSurfaceID, true, class)
			}
		}
		return branch
	}
	condition := buildBranch(unknownProfile, "")
	if hasPage {
		condition = LocalCommandPaletteCondition{CommandID: definitionID, BySurface: map[string]bool{}, Fallback: false,
			ByPage: make(map[string]LocalCommandPaletteCondition)}
		for _, appPage := range commandbindings.AppPages() {
			condition.ByPage[appPage] = buildBranch(unknownProfile, appPage)
		}
	}
	if condition.CommandID == "" {
		return LocalCommandPaletteCondition{}, false
	}
	return condition, true
}

func localPaletteConditionLeaf(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity, definitionID, profile, appPage string, hasSurfaceType, hasSurfaceID, includeProfile bool, class commandExecutionClass) LocalCommandPaletteCondition {
	leaf := LocalCommandPaletteCondition{CommandID: definitionID, BySurface: map[string]bool{}}
	baseFacts := commandbindings.Facts{commandbindings.AppFocused: true}
	if includeProfile && profile != "" {
		baseFacts[commandbindings.Profile] = profile
	}
	if appPage != "" {
		baseFacts[commandbindings.AppPage] = appPage
	}
	leaf.Fallback, leaf.FallbackArguments = paletteConditionArguments(configuration, registry, identity, definitionID, baseFacts, class)
	if hasSurfaceType {
		leaf.BySurfaceArguments = make(map[string]map[string]any)
		for _, surface := range configuration.FieldValues(identity, commandbindings.SurfaceType) {
			facts := clonePaletteFacts(baseFacts)
			facts[commandbindings.SurfaceType] = surface
			selected, args := paletteConditionArguments(configuration, registry, identity, definitionID, facts, class)
			leaf.BySurface[surface] = selected
			if args != nil {
				leaf.BySurfaceArguments[surface] = args
			}
		}
	}
	if hasSurfaceID && hasSurfaceType {
		leaf.BySurfaceID = make(map[string]map[string]bool)
		leaf.BySurfaceIDArguments = make(map[string]map[string]map[string]any)
		for _, surface := range configuration.FieldValues(identity, commandbindings.SurfaceType) {
			leaf.BySurfaceID[surface] = make(map[string]bool)
			leaf.BySurfaceIDArguments[surface] = make(map[string]map[string]any)
			for _, surfaceID := range configuration.FieldValues(identity, commandbindings.SurfaceID) {
				facts := clonePaletteFacts(baseFacts)
				facts[commandbindings.SurfaceType] = surface
				facts[commandbindings.SurfaceID] = surfaceID
				selected, args := paletteConditionArguments(configuration, registry, identity, definitionID, facts, class)
				leaf.BySurfaceID[surface][surfaceID] = selected
				if args != nil {
					leaf.BySurfaceIDArguments[surface][surfaceID] = args
				}
			}
		}
	}
	return leaf
}

func paletteConditionArguments(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity, definitionID string, facts commandbindings.Facts, class commandExecutionClass) (bool, map[string]any) {
	selected, raw := paletteSelectionArgumentsForClass(configuration, registry, identity, definitionID, facts, class)
	if !selected || len(raw) == 0 {
		return selected, nil
	}
	arguments, valid := decodeJSONArgumentObject(raw)
	if !valid {
		return false, nil
	}
	return true, arguments
}

func localPaletteUISelection(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity, definitionID string, facts commandbindings.Facts) bool {
	return paletteSelectionForClass(configuration, registry, identity, definitionID, facts, commandExecutionLocalUI)
}

func paletteConditionClassEligible(definition commandcatalog.Definition, class commandExecutionClass) bool {
	return definition.AllowsSource(commandcatalog.Palette) && commandExecutionClassForDefinition(definition) == class &&
		(class == commandExecutionLocalUI || (class == commandExecutionDurable || class == commandExecutionAuditedUI) && isContextualPaletteWorkspaceCommand(definition.ID) ||
			class == commandExecutionDurable && (isContextualPagePaletteCommand(definition.ID) || isCommandLayerAction(definition.ID)))
}

func paletteSelectionForClass(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity, definitionID string, facts commandbindings.Facts, class commandExecutionClass) bool {
	selected, _ := paletteSelectionArgumentsForClass(configuration, registry, identity, definitionID, facts, class)
	return selected
}

func paletteSelectionArgumentsForClass(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity, definitionID string, facts commandbindings.Facts, class commandExecutionClass) (bool, json.RawMessage) {
	resolved, err := configuration.Resolve(identity, facts, nil)
	if err != nil || resolved.Status != commandbindings.Selected || resolved.CommandID != definitionID || resolved.ExecutionScopeKey != "global" {
		return false, nil
	}
	definition, ok := registry.Lookup(definitionID)
	if !ok || definition.ID != definitionID || !paletteConditionClassEligible(definition, class) {
		return false, nil
	}
	if isCommandLayerAction(definitionID) || definitionID == commandWorkspaceTabGoToID {
		if surface, present := facts[commandbindings.SurfaceType]; present {
			value, valid := surface.(string)
			if !valid || !localKeyboardWorkspaceSurface(value) {
				return false, nil
			}
		}
		canonical, err := definition.ValidateArguments([]byte(resolved.ArgumentsKey))
		if err != nil {
			return false, nil
		}
		if definitionID == commandWorkspaceTabGoToID {
			return true, canonical
		}
		return true, nil
	}
	return emptyPaletteArguments(resolved.ArgumentsKey), nil
}

func emptyPaletteArguments(raw string) bool {
	var object map[string]json.RawMessage
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &object) != nil {
		return false
	}
	return object != nil && len(object) == 0
}

func clonePaletteFacts(in commandbindings.Facts) commandbindings.Facts {
	out := make(commandbindings.Facts, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneLocalCommandPaletteConditions(in []LocalCommandPaletteCondition) []LocalCommandPaletteCondition {
	if in == nil {
		return nil
	}
	out := make([]LocalCommandPaletteCondition, len(in))
	for i, condition := range in {
		out[i] = cloneLocalCommandPaletteCondition(condition)
	}
	return out
}

func cloneLocalCommandPaletteCondition(in LocalCommandPaletteCondition) LocalCommandPaletteCondition {
	out := LocalCommandPaletteCondition{CommandID: in.CommandID, Fallback: in.Fallback, FallbackArguments: cloneJSONArgumentObject(in.FallbackArguments)}
	out.BySurface = make(map[string]bool, len(in.BySurface))
	for key, value := range in.BySurface {
		out.BySurface[key] = value
	}
	if in.BySurfaceArguments != nil {
		out.BySurfaceArguments = make(map[string]map[string]any, len(in.BySurfaceArguments))
		for key, value := range in.BySurfaceArguments {
			out.BySurfaceArguments[key] = cloneJSONArgumentObject(value)
		}
	}
	if in.BySurfaceIDArguments != nil {
		out.BySurfaceIDArguments = make(map[string]map[string]map[string]any, len(in.BySurfaceIDArguments))
		for surface, ids := range in.BySurfaceIDArguments {
			out.BySurfaceIDArguments[surface] = make(map[string]map[string]any, len(ids))
			for id, value := range ids {
				out.BySurfaceIDArguments[surface][id] = cloneJSONArgumentObject(value)
			}
		}
	}
	if in.BySurfaceID != nil {
		out.BySurfaceID = make(map[string]map[string]bool, len(in.BySurfaceID))
		for surface, ids := range in.BySurfaceID {
			out.BySurfaceID[surface] = make(map[string]bool, len(ids))
			for id, value := range ids {
				out.BySurfaceID[surface][id] = value
			}
		}
	}
	if in.ByProfile != nil {
		out.ByProfile = make(map[string]LocalCommandPaletteCondition, len(in.ByProfile))
		for profile, branch := range in.ByProfile {
			out.ByProfile[profile] = cloneLocalCommandPaletteCondition(branch)
		}
	}
	if in.ByPage != nil {
		out.ByPage = make(map[string]LocalCommandPaletteCondition, len(in.ByPage))
		for page, branch := range in.ByPage {
			out.ByPage[page] = cloneLocalCommandPaletteCondition(branch)
		}
	}
	return out
}
