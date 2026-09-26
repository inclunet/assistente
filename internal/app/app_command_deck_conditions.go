package app

import (
	"slices"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

type localDeckConditionContext struct {
	profile, appPage, surface, surfaceID string
	fallback                             bool
	commandID                            string
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
		if field == commandbindings.AppFocused || field == commandbindings.AppPage || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID {
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
	return deckUIConditionsObservedWithPageMode(configuration, registry, identity, eligible, "", false, observe)
}

// deckUIConditionsObservedForPage retains the complete condition projection,
// but limits its optional presentation observer to the supplied live page.
// Page-independent bindings remain observable on every page.
func deckUIConditionsObservedForPage(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, eligible func(commandcatalog.Definition) bool, pageFilter string, observe func(commandbindings.Result)) []LocalCommandPaletteCondition {
	return deckUIConditionsObservedWithPageMode(configuration, registry, identity, eligible, pageFilter, false, observe)
}

// deckUIConditionsObservedWithoutPage evaluates presentation-only bindings
// without inventing a page when the live UI snapshot is unavailable.
func deckUIConditionsObservedWithoutPage(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, eligible func(commandcatalog.Definition) bool, observe func(commandbindings.Result)) []LocalCommandPaletteCondition {
	return deckUIConditionsObservedWithPageMode(configuration, registry, identity, eligible, "", true, observe)
}

func deckUIConditionsObservedWithPageMode(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, eligible func(commandcatalog.Definition) bool, pageFilter string, omitPage bool, observe func(commandbindings.Result)) []LocalCommandPaletteCondition {
	if configuration == nil || registry == nil || !strings.HasPrefix(identity, "streamdeck.key:") {
		return nil
	}
	if pageFilter != "" && !commandbindings.IsAppPage(pageFilter) {
		return nil
	}
	fields := configuration.RequiredFacts(identity)
	hasMermaid, hasPageCommand := false, false
	for _, definition := range registry.List() {
		if isMermaidMutation(definition.ID) && eligible(definition) {
			hasMermaid = true
		}
		if isContextualPagePaletteCommand(definition.ID) && eligible(definition) {
			hasPageCommand = true
		}
	}
	hasType, hasID, hasProfile, hasPage := false, false, false, false
	for _, field := range fields {
		switch field {
		case commandbindings.AppFocused:
		case commandbindings.SurfaceType:
			hasType = true
		case commandbindings.SurfaceID:
			hasID = true
		case commandbindings.Profile:
			hasProfile = true
		case commandbindings.AppPage:
			hasPage = true
		default:
			return nil
		}
	}
	if len(fields) == 0 || (hasID && !hasType) {
		return nil
	}
	contexts := make([]localDeckConditionContext, 0)
	add := func(profile, appPage, surface, surfaceID string, fallback bool) {
		facts := commandbindings.Facts{commandbindings.AppFocused: true}
		if profile != "" {
			facts[commandbindings.Profile] = profile
		}
		if appPage != "" {
			facts[commandbindings.AppPage] = appPage
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
					pageAllowed := !isContextualPagePaletteCommand(definition.ID) || (!hasID && deckPageCommandSurface(definition.ID, surface) &&
						(!hasPage || deckPageCommandPageSurface(definition.ID, appPage, surface)))
					mermaidAllowed := !isMermaidMutation(definition.ID) || surface == "editor"
					if pageAllowed && mermaidAllowed {
						commandID = definition.ID
						if observe != nil {
							pageMatches := pageFilter == "" || !hasPage || appPage == pageFilter
							if pageFilter != "" && !hasPage && isContextualPagePaletteCommand(definition.ID) {
								pageMatches = deckPageCommandPageSurface(definition.ID, pageFilter, surface)
							}
							if pageMatches {
								observe(resolved)
							}
						}
					}
				}
			}
		}
		contexts = append(contexts, localDeckConditionContext{profile: profile, appPage: appPage, surface: surface, surfaceID: surfaceID, fallback: fallback, commandID: commandID})
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
		if hasPage || hasPageCommand {
			// Page-target commands still require the live surface at admission.
			// Enumerate their closed canonical surfaces even for profile-only
			// bindings; this does not synthesize app.page or save surface.type.
			surfaces = append(surfaces, "profiles", "tasklists", "tasklist")
		}
	}
	ids := configuration.FieldValues(identity, commandbindings.SurfaceID)
	if hasMermaid && !slices.Contains(surfaces, "editor") {
		surfaces = append(surfaces, "editor")
	}
	if !hasID {
		ids = []string{""}
	}
	appPages := []string{""}
	if hasPage && !omitPage {
		appPages = commandbindings.AppPages()
	}
	for _, appPage := range appPages {
		for _, profile := range profiles {
			add(profile, appPage, "", "", true)
			for _, surface := range surfaces {
				add(profile, appPage, surface, "", false)
				if hasID {
					for _, surfaceID := range ids {
						add(profile, appPage, surface, surfaceID, false)
					}
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
		if !hasType && hasPage {
			outputSurfaces = slices.DeleteFunc(slices.Clone(outputSurfaces), func(surface string) bool { return surface == "" })
		}
		buildBranch := func(appPage, profile string) LocalCommandPaletteCondition {
			branch := LocalCommandPaletteCondition{CommandID: commandID, BySurface: map[string]bool{}}
			if hasType || isContextualPagePaletteCommand(commandID) || isMermaidMutation(commandID) {
				for _, surface := range outputSurfaces {
					branch.BySurface[surface] = deckConditionSelected(contexts, commandID, appPage, profile, surface, "")
				}
			}
			if hasID {
				branch.BySurfaceID = make(map[string]map[string]bool)
				for _, surface := range outputSurfaces {
					branch.BySurfaceID[surface] = make(map[string]bool)
					for _, surfaceID := range configuration.FieldValues(identity, commandbindings.SurfaceID) {
						branch.BySurfaceID[surface][surfaceID] = deckConditionSelected(contexts, commandID, appPage, profile, surface, surfaceID)
					}
				}
			}
			branch.Fallback = deckConditionFallbackSelected(contexts, commandID, appPage, profile)
			return branch
		}
		attachProfiles := func(branch *LocalCommandPaletteCondition, appPage string) {
			if !hasProfile {
				return
			}
			branch.ByProfile = make(map[string]LocalCommandPaletteCondition)
			for _, profile := range configuration.FieldValues(identity, commandbindings.Profile) {
				branch.ByProfile[profile] = buildBranch(appPage, profile)
			}
		}
		condition := buildBranch("", unknownProfile)
		attachProfiles(&condition, "")
		if hasPage {
			condition = LocalCommandPaletteCondition{CommandID: commandID, BySurface: map[string]bool{}, Fallback: false,
				ByPage: make(map[string]LocalCommandPaletteCondition)}
			for _, appPage := range appPages {
				branch := buildBranch(appPage, unknownProfile)
				attachProfiles(&branch, appPage)
				condition.ByPage[appPage] = branch
			}
		}
		result = append(result, condition)
	}
	return result
}

func deckConditionSelected(contexts []localDeckConditionContext, commandID, appPage, profile, surface, surfaceID string) bool {
	for _, current := range contexts {
		if !current.fallback && current.appPage == appPage && current.profile == profile && current.surface == surface && current.surfaceID == surfaceID && current.commandID == commandID {
			return true
		}
	}
	return false
}

func deckConditionFallbackSelected(contexts []localDeckConditionContext, commandID, appPage, profile string) bool {
	for _, current := range contexts {
		if current.fallback && current.appPage == appPage && current.profile == profile && current.commandID == commandID {
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
