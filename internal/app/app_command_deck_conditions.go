package app

import (
	"slices"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

type localDeckConditionContext struct {
	profile, surface, surfaceID string
	fallback                    bool
	commandID                   string
}

// localDeckUIConditions resolves one physical trigger across the finite visual
// context matrix. It deliberately collects only local_ui selections; a durable
// selection, suppression, review or ambiguity is a false branch, never a
// backend fallback.
func localDeckUIConditions(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string) []LocalCommandPaletteCondition {
	return deckUIConditions(configuration, registry, identity, commandDeckLocalUIEligible)
}

func isContextualDeckUICommand(id string) bool {
	return isContextualPaletteWorkspaceCommand(id) || isCommandLayerAction(id) || isContextualPagePaletteCommand(id)
}

func commandDeckContextualUIEligible(definition commandcatalog.Definition) bool {
	class := commandExecutionClassForDefinition(definition)
	if isContextualPagePaletteCommand(definition.ID) {
		return class == commandExecutionDurable && definition.AllowsSource(commandcatalog.StreamDeck)
	}
	if isCommandLayerAction(definition.ID) && class != commandExecutionDurable {
		return false
	}
	return (class == commandExecutionDurable || class == commandExecutionAuditedUI) && commandDeckLedgerCommand(definition) && isContextualDeckUICommand(definition.ID)
}

// Resolve each matrix cell once, so local and ledger selections cannot both win.
func contextualDeckUIConditions(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string) []LocalCommandPaletteCondition {
	conditions := deckUIConditions(configuration, registry, identity, func(definition commandcatalog.Definition) bool {
		return commandDeckLocalUIEligible(definition) || commandDeckContextualUIEligible(definition)
	})
	if len(conditions) == 0 {
		return nil
	}
	// Profile alone is canonical backend state. Preserve native resolution and
	// frame refresh unless a local branch requires a single mixed UI selection.
	for _, field := range configuration.RequiredFacts(identity) {
		if field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID {
			return conditions
		}
	}
	for _, condition := range conditions {
		if isContextualPagePaletteCommand(condition.CommandID) || isMermaidMutation(condition.CommandID) {
			return conditions
		}
		if definition, ok := registry.Lookup(condition.CommandID); ok && commandDeckLocalUIEligible(definition) {
			return conditions
		}
	}
	return nil
}

func deckUIConditions(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, eligible func(commandcatalog.Definition) bool) []LocalCommandPaletteCondition {
	return deckUIConditionsObserved(configuration, registry, identity, eligible, nil)
}

// observe receives only selections that survived all branch eligibility checks.
// It is used by frame presentation; the input projection keeps the same contract.
func deckUIConditionsObserved(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, eligible func(commandcatalog.Definition) bool, observe func(commandbindings.Result)) []LocalCommandPaletteCondition {
	if configuration == nil || registry == nil || !strings.HasPrefix(identity, "streamdeck.key:") {
		return nil
	}
	fields := configuration.RequiredFacts(identity)
	hasPage, hasMermaid := false, false
	for _, definition := range registry.List() {
		if isContextualPagePaletteCommand(definition.ID) && eligible(definition) {
			hasPage = true
		}
		if isMermaidMutation(definition.ID) && eligible(definition) {
			hasMermaid = true
		}
	}
	hasType, hasID, hasProfile := false, false, false
	for _, field := range fields {
		switch field {
		case commandbindings.AppFocused:
		case commandbindings.SurfaceType:
			hasType = true
		case commandbindings.SurfaceID:
			hasID = true
		case commandbindings.Profile:
			hasProfile = true
		default:
			return nil
		}
	}
	if len(fields) == 0 || (hasID && !hasType) {
		return nil
	}
	contexts := make([]localDeckConditionContext, 0)
	add := func(profile, surface, surfaceID string, fallback bool) {
		facts := commandbindings.Facts{commandbindings.AppFocused: true}
		if profile != "" {
			facts[commandbindings.Profile] = profile
		}
		if surface != "" {
			facts[commandbindings.SurfaceType] = surface
		}
		if surfaceID != "" {
			facts[commandbindings.SurfaceID] = surfaceID
		}
		resolved, err := configuration.Resolve(identity, facts, nil)
		commandID := ""
		if err == nil && resolved.Status == commandbindings.Selected && resolved.ExecutionScopeKey == "global" {
			if definition, ok := registry.Lookup(resolved.CommandID); ok && eligible(definition) {
				_, argsErr := definition.ValidateArguments([]byte(resolved.ArgumentsKey))
				if argsErr == nil && (commandExecutionClassForDefinition(definition) != commandExecutionLocalUI || emptyPaletteArguments(resolved.ArgumentsKey)) {
					pageAllowed := !isContextualPagePaletteCommand(definition.ID) || (!hasID && deckPageCommandSurface(definition.ID, surface))
					mermaidAllowed := !isMermaidMutation(definition.ID) || surface == "editor"
					if pageAllowed && mermaidAllowed {
						commandID = definition.ID
						if observe != nil {
							observe(resolved)
						}
					}
				}
			}
		}
		contexts = append(contexts, localDeckConditionContext{profile: profile, surface: surface, surfaceID: surfaceID, fallback: fallback, commandID: commandID})
	}
	unknownProfile := ""
	if hasProfile {
		unknownProfile = contextualFallbackProfile(configuration.FieldValues(identity, commandbindings.Profile))
	}
	profiles := []string{unknownProfile}
	if hasProfile {
		profiles = append(profiles, configuration.FieldValues(identity, commandbindings.Profile)...)
	}
	surfaces := configuration.FieldValues(identity, commandbindings.SurfaceType)
	if !hasType {
		surfaces = []string{""}
	}
	if hasPage {
		for _, surface := range []string{"profiles", "tasklists", "tasklist"} {
			if !slices.Contains(surfaces, surface) {
				surfaces = append(surfaces, surface)
			}
		}
	}
	ids := configuration.FieldValues(identity, commandbindings.SurfaceID)
	if hasMermaid && !slices.Contains(surfaces, "editor") {
		surfaces = append(surfaces, "editor")
	}
	if !hasID {
		ids = []string{""}
	}
	for _, profile := range profiles {
		add(profile, "", "", true)
		for _, surface := range surfaces {
			add(profile, surface, "", false)
			if hasID {
				for _, surfaceID := range ids {
					add(profile, surface, surfaceID, false)
				}
			}
		}
	}

	commandIDs := map[string]struct{}{}
	for _, current := range contexts {
		if current.commandID != "" {
			commandIDs[current.commandID] = struct{}{}
		}
	}
	if len(commandIDs) == 0 {
		return nil
	}
	idsSorted := make([]string, 0, len(commandIDs))
	for commandID := range commandIDs {
		idsSorted = append(idsSorted, commandID)
	}
	slices.Sort(idsSorted)
	result := make([]LocalCommandPaletteCondition, 0, len(idsSorted))
	for _, commandID := range idsSorted {
		outputSurfaces := configuration.FieldValues(identity, commandbindings.SurfaceType)
		if isContextualPagePaletteCommand(commandID) || isMermaidMutation(commandID) {
			outputSurfaces = surfaces
		}
		condition := LocalCommandPaletteCondition{CommandID: commandID, BySurface: map[string]bool{}}
		if hasType || isContextualPagePaletteCommand(commandID) || isMermaidMutation(commandID) {
			for _, surface := range outputSurfaces {
				condition.BySurface[surface] = deckConditionSelected(contexts, commandID, unknownProfile, surface, "")
			}
		}
		if hasID {
			condition.BySurfaceID = make(map[string]map[string]bool)
			for _, surface := range outputSurfaces {
				condition.BySurfaceID[surface] = make(map[string]bool)
				for _, surfaceID := range configuration.FieldValues(identity, commandbindings.SurfaceID) {
					condition.BySurfaceID[surface][surfaceID] = deckConditionSelected(contexts, commandID, unknownProfile, surface, surfaceID)
				}
			}
		}
		condition.Fallback = deckConditionFallbackSelected(contexts, commandID, unknownProfile)
		if hasProfile {
			condition.ByProfile = make(map[string]LocalCommandPaletteCondition)
			for _, profile := range configuration.FieldValues(identity, commandbindings.Profile) {
				branch := LocalCommandPaletteCondition{CommandID: commandID, BySurface: map[string]bool{}}
				if hasType || isContextualPagePaletteCommand(commandID) || isMermaidMutation(commandID) {
					for _, surface := range outputSurfaces {
						branch.BySurface[surface] = deckConditionSelected(contexts, commandID, profile, surface, "")
					}
				}
				if hasID {
					branch.BySurfaceID = make(map[string]map[string]bool)
					for _, surface := range outputSurfaces {
						branch.BySurfaceID[surface] = make(map[string]bool)
						for _, surfaceID := range configuration.FieldValues(identity, commandbindings.SurfaceID) {
							branch.BySurfaceID[surface][surfaceID] = deckConditionSelected(contexts, commandID, profile, surface, surfaceID)
						}
					}
				}
				branch.Fallback = deckConditionFallbackSelected(contexts, commandID, profile)
				condition.ByProfile[profile] = branch
			}
		}
		result = append(result, condition)
	}
	return result
}

func deckConditionSelected(contexts []localDeckConditionContext, commandID, profile, surface, surfaceID string) bool {
	for _, current := range contexts {
		if !current.fallback && current.profile == profile && current.surface == surface && current.surfaceID == surfaceID && current.commandID == commandID {
			return true
		}
	}
	return false
}

func deckConditionFallbackSelected(contexts []localDeckConditionContext, commandID, profile string) bool {
	for _, current := range contexts {
		if current.fallback && current.profile == profile && current.commandID == commandID {
			return true
		}
	}
	return false
}

func commandDeckLocalUIEligible(definition commandcatalog.Definition) bool {
	return commandDeckDefinitionEligible(definition) && commandExecutionClassForDefinition(definition) == commandExecutionLocalUI && definition.AllowsSource(commandcatalog.StreamDeck)
}

// All potential branches for a command must agree on its custom title. The
// host does not know which visual branch is active, so divergence falls back
// to the localized command name, just like divergent equivalent bindings.
func localDeckPresentation(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, conditions []LocalCommandPaletteCondition, locale string) (string, string, string) {
	idsByCommand := make(map[string][]string)
	deckUIConditionsObserved(configuration, registry, identity, func(definition commandcatalog.Definition) bool {
		return commandDeckLocalUIEligible(definition) || commandDeckContextualUIEligible(definition)
	}, func(result commandbindings.Result) {
		for _, id := range result.BindingIDs {
			if !slices.Contains(idsByCommand[result.CommandID], id) {
				idsByCommand[result.CommandID] = append(idsByCommand[result.CommandID], id)
			}
		}
	})
	names := make([]string, 0, len(conditions))
	var bindingIDs []string
	seen := make(map[string]struct{}, len(conditions))
	for _, condition := range conditions {
		if _, ok := seen[condition.CommandID]; ok {
			continue
		}
		seen[condition.CommandID] = struct{}{}
		bindingIDs = append(bindingIDs, idsByCommand[condition.CommandID]...)
		name := condition.CommandID
		if definition, ok := registry.Lookup(condition.CommandID); ok {
			name = commandDeckTitle(configuration, idsByCommand[condition.CommandID], definition, locale)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, " / "), configuration.IconForBindings(bindingIDs), configuration.ImageForBindings(bindingIDs)
}
