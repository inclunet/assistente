package app

import (
	"encoding/json"
	"slices"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

type localDeckConditionContext struct {
	profile, surface, surfaceID string
	fallback                    bool
	commandID                   string
	arguments                   json.RawMessage
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
		var arguments json.RawMessage
		if err == nil && resolved.Status == commandbindings.Selected && resolved.ExecutionScopeKey == "global" {
			if definition, ok := registry.Lookup(resolved.CommandID); ok && eligible(definition) {
				canonicalArguments, argsErr := definition.ValidateArguments([]byte(resolved.ArgumentsKey))
				if argsErr == nil && (commandExecutionClassForDefinition(definition) != commandExecutionLocalUI || emptyPaletteArguments(resolved.ArgumentsKey) || definition.ID == commandWorkspaceTabGoToID) {
					pageAllowed := !isContextualPagePaletteCommand(definition.ID) || (!hasID && deckPageCommandSurface(definition.ID, surface))
					mermaidAllowed := !isMermaidMutation(definition.ID) || surface == "editor"
					if pageAllowed && mermaidAllowed {
						commandID = definition.ID
						if definition.ID == commandWorkspaceTabGoToID {
							arguments = canonicalArguments
						}
						if observe != nil {
							observe(resolved)
						}
					}
				}
			}
		}
		contexts = append(contexts, localDeckConditionContext{profile: profile, surface: surface, surfaceID: surfaceID, fallback: fallback, commandID: commandID, arguments: arguments})
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
		if commandID == commandWorkspaceTabGoToID {
			condition.BySurfaceArguments = make(map[string]map[string]any)
		}
		if hasType || isContextualPagePaletteCommand(commandID) || isMermaidMutation(commandID) {
			for _, surface := range outputSurfaces {
				selected := deckConditionSelected(contexts, commandID, unknownProfile, surface, "")
				if commandID == commandWorkspaceTabGoToID {
					if args, valid := deckConditionArgumentObject(contexts, commandID, unknownProfile, surface, ""); !valid {
						selected = false
					} else if args != nil {
						condition.BySurfaceArguments[surface] = args
					}
				}
				condition.BySurface[surface] = selected
			}
		}
		if hasID {
			condition.BySurfaceID = make(map[string]map[string]bool)
			if commandID == commandWorkspaceTabGoToID {
				condition.BySurfaceIDArguments = make(map[string]map[string]map[string]any)
			}
			for _, surface := range outputSurfaces {
				condition.BySurfaceID[surface] = make(map[string]bool)
				if commandID == commandWorkspaceTabGoToID {
					condition.BySurfaceIDArguments[surface] = make(map[string]map[string]any)
				}
				for _, surfaceID := range configuration.FieldValues(identity, commandbindings.SurfaceID) {
					selected := deckConditionSelected(contexts, commandID, unknownProfile, surface, surfaceID)
					if commandID == commandWorkspaceTabGoToID {
						if args, valid := deckConditionArgumentObject(contexts, commandID, unknownProfile, surface, surfaceID); !valid {
							selected = false
						} else if args != nil {
							condition.BySurfaceIDArguments[surface][surfaceID] = args
						}
					}
					condition.BySurfaceID[surface][surfaceID] = selected
				}
			}
		}
		condition.Fallback = deckConditionFallbackSelected(contexts, commandID, unknownProfile)
		if commandID == commandWorkspaceTabGoToID {
			condition.FallbackArguments, _ = deckConditionArgumentObject(contexts, commandID, unknownProfile, "", "")
			if condition.Fallback && condition.FallbackArguments == nil {
				condition.Fallback = false
			}
		}
		if hasProfile {
			condition.ByProfile = make(map[string]LocalCommandPaletteCondition)
			for _, profile := range configuration.FieldValues(identity, commandbindings.Profile) {
				branch := LocalCommandPaletteCondition{CommandID: commandID, BySurface: map[string]bool{}}
				if commandID == commandWorkspaceTabGoToID {
					branch.BySurfaceArguments = make(map[string]map[string]any)
				}
				if hasType || isContextualPagePaletteCommand(commandID) || isMermaidMutation(commandID) {
					for _, surface := range outputSurfaces {
						selected := deckConditionSelected(contexts, commandID, profile, surface, "")
						if commandID == commandWorkspaceTabGoToID {
							if args, valid := deckConditionArgumentObject(contexts, commandID, profile, surface, ""); !valid {
								selected = false
							} else if args != nil {
								branch.BySurfaceArguments[surface] = args
							}
						}
						branch.BySurface[surface] = selected
					}
				}
				if hasID {
					branch.BySurfaceID = make(map[string]map[string]bool)
					if commandID == commandWorkspaceTabGoToID {
						branch.BySurfaceIDArguments = make(map[string]map[string]map[string]any)
					}
					for _, surface := range outputSurfaces {
						branch.BySurfaceID[surface] = make(map[string]bool)
						if commandID == commandWorkspaceTabGoToID {
							branch.BySurfaceIDArguments[surface] = make(map[string]map[string]any)
						}
						for _, surfaceID := range configuration.FieldValues(identity, commandbindings.SurfaceID) {
							selected := deckConditionSelected(contexts, commandID, profile, surface, surfaceID)
							if commandID == commandWorkspaceTabGoToID {
								if args, valid := deckConditionArgumentObject(contexts, commandID, profile, surface, surfaceID); !valid {
									selected = false
								} else if args != nil {
									branch.BySurfaceIDArguments[surface][surfaceID] = args
								}
							}
							branch.BySurfaceID[surface][surfaceID] = selected
						}
					}
				}
				branch.Fallback = deckConditionFallbackSelected(contexts, commandID, profile)
				if commandID == commandWorkspaceTabGoToID {
					branch.FallbackArguments, _ = deckConditionArgumentObject(contexts, commandID, profile, "", "")
					if branch.Fallback && branch.FallbackArguments == nil {
						branch.Fallback = false
					}
				}
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

func deckConditionArguments(contexts []localDeckConditionContext, commandID, profile, surface, surfaceID string) json.RawMessage {
	for _, current := range contexts {
		if !current.fallback && current.profile == profile && current.surface == surface && current.surfaceID == surfaceID && current.commandID == commandID {
			return append(json.RawMessage(nil), current.arguments...)
		}
	}
	return nil
}

func deckConditionArgumentObject(contexts []localDeckConditionContext, commandID, profile, surface, surfaceID string) (map[string]any, bool) {
	var raw json.RawMessage
	if surface == "" && surfaceID == "" {
		raw = deckConditionFallbackArguments(contexts, commandID, profile)
	} else {
		raw = deckConditionArguments(contexts, commandID, profile, surface, surfaceID)
	}
	if len(raw) == 0 {
		return nil, true
	}
	return decodeJSONArgumentObject(raw)
}

func deckConditionFallbackArguments(contexts []localDeckConditionContext, commandID, profile string) json.RawMessage {
	for _, current := range contexts {
		if current.fallback && current.profile == profile && current.commandID == commandID {
			return append(json.RawMessage(nil), current.arguments...)
		}
	}
	return nil
}

func commandDeckLocalUIEligible(definition commandcatalog.Definition) bool {
	return commandDeckDefinitionEligible(definition) && commandExecutionClassForDefinition(definition) == commandExecutionLocalUI && definition.AllowsSource(commandcatalog.StreamDeck)
}

// All potential branches for a command must agree on its custom title. The
// host does not know which visual branch is active, so divergence falls back
// to the localized command name, just like divergent equivalent bindings.
