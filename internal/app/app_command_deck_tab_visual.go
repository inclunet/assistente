package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/workspace"
)

type workspaceTabDeckTarget struct {
	title string
	icon  string
	state string
	key   string
}

func (p *commandProductRuntime) deckPresentationWorkspaceSnapshot() *workspace.Workspace {
	if p == nil || p.app == nil || p.app.workspaceMgr == nil {
		return nil
	}
	return p.app.workspaceMgr.Active()
}

const (
	workspaceTabDeckTargetReady       = "ready"
	workspaceTabDeckTargetUnavailable = "unavailable"
	workspaceTabDeckTargetAmbiguous   = "ambiguous"
)

func applyWorkspaceTabDeckVisual(
	configuration *commandbindings.Configuration,
	registry *commandcatalog.Registry,
	identity string,
	bindingIDs []string,
	conditions []LocalCommandPaletteCondition,
	arguments []byte,
	active *workspace.Workspace,
	locale string,
	base commandDeckVisual,
	variants map[string]commandDeckVisual,
	commandID string,
) (commandDeckVisual, map[string]commandDeckVisual) {
	var targets []workspaceTabDeckTarget
	if commandID == commandWorkspaceTabGoToID {
		targets = append(targets, workspaceTabDeckTargetFromArguments(arguments, active, locale))
	} else if commandID != "" {
		if _, position, ok := workspaceTabNavigationForCommand(commandID); ok && position > 0 {
			targets = append(targets, workspaceTabDeckTargetAtPosition(active, position, locale))
		}
	} else {
		for _, condition := range conditions {
			if !isWorkspaceTabDeckVisualCommand(condition.CommandID) {
				continue
			}
			collectWorkspaceTabDeckTargets(condition, active, locale, &targets)
		}
	}
	if len(targets) == 0 {
		return base, variants
	}
	if len(bindingIDs) == 0 && commandID == "" {
		seen := make(map[string]bool)
		deckUIConditionsObserved(configuration, registry, identity, func(definition commandcatalog.Definition) bool {
			return commandDeckLocalUIEligible(definition) || commandDeckContextualUIEligible(definition)
		}, func(result commandbindings.Result) {
			if isWorkspaceTabDeckVisualCommand(result.CommandID) {
				for _, id := range result.BindingIDs {
					if !seen[id] {
						seen[id] = true
						bindingIDs = append(bindingIDs, id)
					}
				}
			}
		})
	}
	target := targets[0]
	for _, candidate := range targets {
		if candidate.state == workspaceTabDeckTargetUnavailable {
			target = workspaceTabDeckUnavailable(locale)
			break
		}
	}
	if target.state != workspaceTabDeckTargetUnavailable {
		for _, candidate := range targets[1:] {
			if candidate.state != target.state || candidate.key != target.key || candidate.title != target.title || candidate.icon != target.icon {
				target = workspaceTabDeckTarget{state: workspaceTabDeckTargetAmbiguous}
				break
			}
		}
	}
	if target.state == workspaceTabDeckTargetAmbiguous {
		target.title = workspaceTabDeckText(locale, "ambiguous")
	}
	preserveCombinedTitle := commandID == "" && hasNonTabWorkspaceTabDeckVisualCommand(conditions)

	apply := func(visual commandDeckVisual, state string) commandDeckVisual {
		customTitle := hasCustomDeckTitle(configuration, bindingIDs, state, locale)
		if !customTitle {
			switch target.state {
			case workspaceTabDeckTargetReady:
				if preserveCombinedTitle {
					visual.title = deckVisualSuffix(visual.title, target.title)
				} else {
					visual.title = target.title
				}
			case workspaceTabDeckTargetUnavailable:
				if preserveCombinedTitle {
					visual.title = deckVisualSuffix(visual.title, workspaceTabDeckText(locale, "unavailable"))
				} else {
					visual.title = workspaceTabDeckText(locale, "unavailable")
				}
			case workspaceTabDeckTargetAmbiguous:
				if preserveCombinedTitle {
					visual.title = deckVisualSuffix(visual.title, target.title)
				} else {
					visual.title = target.title
				}
			}
		} else if target.state == workspaceTabDeckTargetUnavailable || target.state == workspaceTabDeckTargetAmbiguous {
			visual.title = deckVisualSuffix(visual.title, target.title)
		}
		if target.state == workspaceTabDeckTargetReady && !hasCustomDeckIcon(configuration, bindingIDs, state) {
			visual.icon = target.icon
		}
		return visual
	}
	base = apply(base, "")
	for state, visual := range variants {
		variants[state] = apply(visual, state)
	}
	return base, variants
}

func hasNonTabWorkspaceTabDeckVisualCommand(conditions []LocalCommandPaletteCondition) bool {
	for _, condition := range conditions {
		if condition.CommandID != "" && !isWorkspaceTabDeckVisualCommand(condition.CommandID) {
			return true
		}
		if hasNonTabWorkspaceTabDeckVisualCommandFromMap(condition.ByProfile) {
			return true
		}
	}
	return false
}

func hasNonTabWorkspaceTabDeckVisualCommandFromMap(conditions map[string]LocalCommandPaletteCondition) bool {
	for _, condition := range conditions {
		if condition.CommandID != "" && !isWorkspaceTabDeckVisualCommand(condition.CommandID) {
			return true
		}
		if hasNonTabWorkspaceTabDeckVisualCommandFromMap(condition.ByProfile) {
			return true
		}
	}
	return false
}

func isWorkspaceTabDeckVisualCommand(commandID string) bool {
	if commandID == commandWorkspaceTabGoToID {
		return true
	}
	_, position, ok := workspaceTabNavigationForCommand(commandID)
	return ok && position > 0
}

func collectWorkspaceTabDeckTargets(condition LocalCommandPaletteCondition, active *workspace.Workspace, locale string, output *[]workspaceTabDeckTarget) {
	if isWorkspaceTabDeckVisualCommand(condition.CommandID) {
		appendTarget := func(arguments map[string]any) {
			if condition.CommandID == commandWorkspaceTabGoToID {
				raw, err := json.Marshal(arguments)
				if err != nil {
					*output = append(*output, workspaceTabDeckUnavailable(locale))
					return
				}
				*output = append(*output, workspaceTabDeckTargetFromArguments(raw, active, locale))
				return
			}
			_, position, _ := workspaceTabNavigationForCommand(condition.CommandID)
			*output = append(*output, workspaceTabDeckTargetAtPosition(active, position, locale))
		}
		if condition.Fallback {
			appendTarget(condition.FallbackArguments)
		}
		for surface, selected := range condition.BySurface {
			if selected {
				appendTarget(condition.BySurfaceArguments[surface])
			}
		}
		for surface, byID := range condition.BySurfaceID {
			for surfaceID, selected := range byID {
				if selected {
					appendTarget(condition.BySurfaceIDArguments[surface][surfaceID])
				}
			}
		}
	}
	for _, branch := range condition.ByProfile {
		collectWorkspaceTabDeckTargets(branch, active, locale, output)
	}
}

func workspaceTabDeckTargetFromArguments(raw []byte, active *workspace.Workspace, locale string) workspaceTabDeckTarget {
	var arguments struct {
		WorkspaceID string `json:"workspace_id"`
		TargetMode  string `json:"target_mode"`
		Position    int    `json:"position"`
		TabID       string `json:"tab_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) == 0 || decoder.Decode(&arguments) != nil || decoder.Decode(&struct{}{}) != io.EOF || active == nil || arguments.WorkspaceID == "" || arguments.WorkspaceID != active.ID {
		return workspaceTabDeckUnavailable(locale)
	}
	switch arguments.TargetMode {
	case "position":
		if arguments.Position < 1 || arguments.TabID != "" {
			return workspaceTabDeckUnavailable(locale)
		}
		return workspaceTabDeckTargetAtPosition(active, arguments.Position, locale)
	case "specific":
		if arguments.TabID == "" || arguments.Position != 0 {
			return workspaceTabDeckUnavailable(locale)
		}
		var found *workspace.Tab
		for index := range active.Tabs.Items {
			if active.Tabs.Items[index].ID == arguments.TabID {
				if found != nil {
					return workspaceTabDeckUnavailable(locale)
				}
				found = &active.Tabs.Items[index]
			}
		}
		if found == nil {
			return workspaceTabDeckUnavailable(locale)
		}
		return workspaceTabDeckTargetForTab(*found, locale)
	default:
		return workspaceTabDeckUnavailable(locale)
	}
}

func workspaceTabDeckTargetAtPosition(active *workspace.Workspace, position int, locale string) workspaceTabDeckTarget {
	if active == nil || position < 1 || position > len(active.Tabs.Items) {
		return workspaceTabDeckUnavailable(locale)
	}
	tabs := append([]workspace.Tab(nil), active.Tabs.Items...)
	sort.Slice(tabs, func(i, j int) bool { return tabs[i].Position < tabs[j].Position })
	for index := 1; index < len(tabs); index++ {
		if tabs[index-1].Position == tabs[index].Position {
			return workspaceTabDeckUnavailable(locale)
		}
	}
	return workspaceTabDeckTargetForTab(tabs[position-1], locale)
}

func workspaceTabDeckTargetForTab(tab workspace.Tab, locale string) workspaceTabDeckTarget {
	title := strings.TrimSpace(tab.Title)
	if title == "" {
		title = workspaceTabDeckTypeName(tab.Type, locale)
	}
	icon, ok := workspaceTabDeckIcon(tab.Type)
	if !ok || tab.ID == "" {
		return workspaceTabDeckUnavailable(locale)
	}
	return workspaceTabDeckTarget{title: title, icon: icon, state: workspaceTabDeckTargetReady, key: tab.ID}
}

func workspaceTabDeckUnavailable(locale string) workspaceTabDeckTarget {
	return workspaceTabDeckTarget{title: workspaceTabDeckText(locale, "unavailable"), state: workspaceTabDeckTargetUnavailable, key: "unavailable"}
}

func workspaceTabDeckIcon(tabType workspace.TabType) (string, bool) {
	switch tabType {
	case workspace.TabTypeChat:
		return "workspace-tab-chat", true
	case workspace.TabTypeEditor:
		return "workspace-tab-editor", true
	case workspace.TabTypeTerminal:
		return "workspace-tab-terminal", true
	case workspace.TabTypeTasklist:
		return "workspace-tab-tasklist", true
	default:
		return "", false
	}
}

func workspaceTabDeckTypeName(tabType workspace.TabType, locale string) string {
	return workspaceTabDeckText(locale, string(tabType))
}

func workspaceTabDeckText(locale, key string) string {
	texts := map[string]map[string]string{
		"pt-BR": {"unavailable": "Aba indisponível", "ambiguous": "Destino varia por contexto", "chat": "Conversa", "editor": "Editor", "terminal": "Terminal", "tasklist": "Lista de tarefas"},
		"en":    {"unavailable": "Tab unavailable", "ambiguous": "Context-dependent destination", "chat": "Chat", "editor": "Editor", "terminal": "Terminal", "tasklist": "Task list"},
		"es":    {"unavailable": "Pestaña no disponible", "ambiguous": "Destino según el contexto", "chat": "Chat", "editor": "Editor", "terminal": "Terminal", "tasklist": "Lista de tareas"},
	}
	if byLocale, ok := texts[locale]; ok {
		if text, ok := byLocale[key]; ok {
			return text
		}
	}
	return texts["en"][key]
}

func deckVisualSuffix(title, marker string) string {
	if strings.Contains(title, marker) {
		return title
	}
	if strings.TrimSpace(title) == "" {
		return marker
	}
	return fmt.Sprintf("%s — %s", title, marker)
}

func hasCustomDeckTitle(configuration *commandbindings.Configuration, ids []string, state, locale string) bool {
	if configuration == nil || len(ids) == 0 {
		return false
	}
	if state != "" {
		return strings.TrimSpace(configuration.PresentationForState(ids, state).TitleByLocale[locale]) != ""
	}
	title, ok := configuration.TitleForBindings(ids, locale)
	return ok && strings.TrimSpace(title) != ""
}

func hasCustomDeckIcon(configuration *commandbindings.Configuration, ids []string, state string) bool {
	if configuration == nil || len(ids) == 0 {
		return false
	}
	if state != "" {
		return strings.TrimSpace(configuration.PresentationForState(ids, state).Icon) != ""
	}
	return strings.TrimSpace(configuration.IconForBindings(ids)) != ""
}
