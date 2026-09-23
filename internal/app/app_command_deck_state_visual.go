package app

import (
	"slices"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

var commandDeckPresentationStates = [...]string{"on", "off", "waiting", "running", "succeeded", "failed", "denied", "cancelled", "timed_out", "outcome_unknown"}

type commandDeckVisual struct{ title, icon, imageRef string }

func commandDeckPresentationState(binding commandDeckBinding) string {
	if binding.feedbackState != "" {
		return binding.feedbackState
	}
	return binding.persistentState
}

// Variants are presentation only. Resolution, authority and execution never
// consult them, and decoded image bytes are still loaded only for changed frames.
func commandDeckPresentedBinding(binding commandDeckBinding) commandDeckBinding {
	if visual, ok := binding.variants[commandDeckPresentationState(binding)]; ok {
		if binding.imageRef != visual.imageRef {
			binding.imagePNG = nil
		}
		binding.title, binding.icon, binding.imageRef = visual.title, visual.icon, visual.imageRef
	}
	return binding
}

func commandDeckStateVisuals(configuration *commandbindings.Configuration, ids []string, definition commandcatalog.Definition, locale string) map[string]commandDeckVisual {
	visuals := make(map[string]commandDeckVisual, len(commandDeckPresentationStates))
	for _, state := range commandDeckPresentationStates {
		presentation := configuration.PresentationForState(ids, state)
		title := presentation.TitleByLocale[locale]
		if title == "" {
			title = definition.ID
			if definition.Presentation != nil {
				if localized, ok := definition.Presentation.Locales[locale]; ok && strings.TrimSpace(localized.Name) != "" {
					title = localized.Name
				}
			}
		}
		visuals[state] = commandDeckVisual{title: title, icon: presentation.Icon, imageRef: presentation.ImageRef}
	}
	return visuals
}

func localDeckPresentations(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, conditions []LocalCommandPaletteCondition, locale string) (commandDeckVisual, map[string]commandDeckVisual) {
	// Enumerate possible contextual resolutions once, outside the key-down.
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
	namesByState := make(map[string][]string)
	var allIDs []string
	seen := make(map[string]bool)
	for _, condition := range conditions {
		if seen[condition.CommandID] {
			continue
		}
		seen[condition.CommandID] = true
		ids := idsByCommand[condition.CommandID]
		allIDs = append(allIDs, ids...)
		definition, ok := registry.Lookup(condition.CommandID)
		if !ok {
			definition.ID = condition.CommandID
		}
		names = append(names, commandDeckTitle(configuration, ids, definition, locale))
		for state, visual := range commandDeckStateVisuals(configuration, ids, definition, locale) {
			namesByState[state] = append(namesByState[state], visual.title)
		}
	}
	slices.Sort(names)
	base := commandDeckVisual{title: strings.Join(names, " / "), icon: configuration.IconForBindings(allIDs), imageRef: configuration.ImageForBindings(allIDs)}
	variants := make(map[string]commandDeckVisual, len(commandDeckPresentationStates))
	for _, state := range commandDeckPresentationStates {
		stateNames := namesByState[state]
		slices.Sort(stateNames)
		presentation := configuration.PresentationForState(allIDs, state)
		variants[state] = commandDeckVisual{title: strings.Join(stateNames, " / "), icon: presentation.Icon, imageRef: presentation.ImageRef}
	}
	return base, variants
}
