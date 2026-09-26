package app

import (
	"strings"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

func TestCommandDeckContextualProfileOnlyKeepsNativeUnlessMixed(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	candidates := []commandbindings.Candidate{
		deckConditionCandidate("backend", commandWorkspaceTabCloseID, commandbindings.Facts{commandbindings.Profile: "dev"}),
	}
	configuration, err := commandbindings.NewConfiguration(nil, nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger); len(got) != 0 {
		t.Fatalf("profile-only backend must retain native resolution: %+v", got)
	}
	candidates = append(candidates, deckConditionCandidate("local", "navigation.settings.open", commandbindings.Facts{commandbindings.Profile: "other"}))
	configuration, err = commandbindings.NewConfiguration(nil, nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger); len(got) != 2 {
		t.Fatalf("mixed profile-only key must have one union matrix: %+v", got)
	}
}

func TestCommandDeckLocalUIConditionsProjectApplicationPageIndependently(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("settings-page", commandProductShortcutsShowID, commandbindings.Facts{commandbindings.AppPage: "settings"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	conditions := localDeckUIConditions(configuration, registry, deckConditionTestTrigger)
	if len(conditions) != 1 || !conditions[0].ByPage["settings"].Fallback || conditions[0].ByPage["workspace"].Fallback {
		t.Fatalf("Deck route projection = %+v", conditions)
	}
}

func TestCommandDeckPageOnlyConditionProjectsLivePageSurfaces(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, test := range []struct {
		id, page, surface string
	}{{"profiles.activate", "profiles", "profiles"}, {"tasklists.delete", "tasklists", "tasklists"}, {"tasklists.duplicate", "tasklists", "tasklists"}, {"tasklists.duplicate", "workspace", "tasklist"}, {"tasklists.clear", "workspace", "tasklist"}} {
		t.Run(test.id, func(t *testing.T) {
			configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
				deckConditionCandidate("page-only", test.id, commandbindings.Facts{commandbindings.AppPage: test.page}),
			})
			if err != nil {
				t.Fatal(err)
			}
			conditions := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger)
			if len(conditions) != 1 {
				t.Fatalf("page-only condition was not projected: %+v", conditions)
			}
			page := conditions[0].ByPage[test.page]
			if !page.BySurface[test.surface] || page.Fallback || page.BySurface[""] {
				t.Fatalf("live page/surface branch = %+v", page)
			}
			for _, other := range []string{"profiles", "tasklists", "tasklist"} {
				if other != test.surface && page.BySurface[other] {
					t.Fatalf("page-only binding leaked to surface %q: %+v", other, page)
				}
			}
			for _, otherPage := range commandbindings.AppPages() {
				if otherPage != test.page && conditions[0].ByPage[otherPage].BySurface[test.surface] {
					t.Fatalf("page-only binding leaked to app.page=%q: %+v", otherPage, conditions[0])
				}
			}
		})
	}
}

func TestCommandDeckPageObserverFiltersProfileOnlyBindingByCanonicalPageSurface(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("profile-only", "profiles.activate", commandbindings.Facts{commandbindings.Profile: "dev"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		page string
		want int
	}{{page: "profiles", want: 1}, {page: "workspace", want: 0}, {page: "tasklists", want: 0}} {
		t.Run(test.page, func(t *testing.T) {
			var observed []commandbindings.Result
			got := deckUIConditionsObservedForPage(configuration, registry, deckConditionTestTrigger,
				commandDeckContextualUIEligible, test.page, func(result commandbindings.Result) { observed = append(observed, result) })
			if test.page == "profiles" && len(got) != 1 {
				t.Fatalf("profile-only command projection missing: %+v", got)
			}
			if len(observed) != test.want {
				t.Fatalf("page %q observed %d times, want %d; results=%+v", test.page, len(observed), test.want, observed)
			}
			for _, result := range observed {
				if result.Status != commandbindings.Selected || result.CommandID != "profiles.activate" {
					t.Fatalf("unexpected observed result: %+v", result)
				}
			}
		})
	}
}

func TestCommandDeckPageObserverKeepsPageIndependentProfileOnlyBinding(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("profile-only", "help.shortcuts.show", commandbindings.Facts{commandbindings.Profile: "dev"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"workspace", "settings"} {
		t.Run(page, func(t *testing.T) {
			var observed []commandbindings.Result
			deckUIConditionsObservedForPage(configuration, registry, deckConditionTestTrigger, commandDeckLocalUIEligible, page,
				func(result commandbindings.Result) { observed = append(observed, result) })
			if len(observed) == 0 {
				t.Fatalf("page-independent command hidden for page %q: %+v", page, observed)
			}
			for _, result := range observed {
				if result.Status != commandbindings.Selected || result.CommandID != "help.shortcuts.show" {
					t.Fatalf("unexpected page-independent observation for %q: %+v", page, observed)
				}
			}
		})
	}
}

func TestCommandDeckContextualConditionsRegisteredScope(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	count := 0
	for _, definition := range registry.List() {
		if !commandDeckContextualUIEligible(definition) {
			continue
		}
		count++
		t.Run(definition.ID, func(t *testing.T) {
			for _, variant := range []string{"exact", "process", "device", "scope", "arguments"} {
				candidate := deckConditionCandidate("binding", definition.ID, commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one", commandbindings.Profile: "dev"})
				candidate.ArgumentsKey = deckContextualTestArguments(definition.ID)
				workspaceSurface, workspaceID := "chat", "chat-one"
				if isMermaidMutation(definition.ID) {
					workspaceSurface, workspaceID = "editor", "editor-one"
					candidate.Condition[commandbindings.SurfaceType] = workspaceSurface
					candidate.Condition[commandbindings.SurfaceID] = workspaceID
				}
				page := isContextualPagePaletteCommand(definition.ID)
				pageSurface := "tasklists"
				if strings.HasPrefix(definition.ID, "profiles.") {
					pageSurface = "profiles"
				}
				if page {
					delete(candidate.Condition, commandbindings.SurfaceID)
					candidate.Condition[commandbindings.SurfaceType] = pageSurface
				}
				switch variant {
				case "process":
					candidate.LayerConditions = []commandbindings.Facts{{commandbindings.Process: "app.exe"}}
				case "device":
					candidate.LayerConditions = []commandbindings.Facts{{commandbindings.Device: "DECK"}}
				case "scope":
					candidate.ExecutionScopeKey = "workspace:foreign"
				case "arguments":
					candidate.ArgumentsKey = `{"forged":true}`
				}
				configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
				if err != nil {
					t.Fatal(err)
				}
				got := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger)
				if variant != "exact" {
					if len(got) != 0 {
						t.Fatalf("%s published: %+v", variant, got)
					}
					continue
				}
				if len(got) != 1 || got[0].Fallback || got[0].ByProfile["dev"].Fallback {
					t.Fatalf("matrix: %+v", got)
				}
				if page {
					if !got[0].ByProfile["dev"].BySurface[pageSurface] || len(got[0].BySurfaceID) != 0 {
						t.Fatalf("page matrix: %+v", got)
					}
				} else if !got[0].ByProfile["dev"].BySurfaceID[workspaceSurface][workspaceID] {
					t.Fatalf("workspace matrix: %+v", got)
				}
				if len(localDeckUIConditions(configuration, registry, deckConditionTestTrigger)) != 0 {
					t.Fatal("ledger command became LOCAL_UI")
				}
			}
		})
	}
	if count != 81 {
		t.Fatalf("eligible=%d; want previous79 plus 2 Mermaid mutations", count)
	}
	for _, id := range []string{"profiles.create", "tasklists.update", "workspace.list"} {
		definition, exists := registry.Lookup(id)
		if !exists || commandDeckContextualUIEligible(definition) {
			t.Fatalf("negative registration: %s exists=%v", id, exists)
		}
	}
}

func deckContextualTestArguments(id string) string {
	if id == commandLayerBackID {
		return `{"scope":"workspace","rule_id":"","duration_seconds":0}`
	}
	if isCommandLayerAction(id) {
		return `{"scope":"workspace","rule_id":"configured-rule","duration_seconds":60}`
	}
	return `{}`
}

func TestCommandDeckContextualMermaidEditorOnly(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{commandEditorMermaidApplyID, commandEditorMermaidRemoveID} {
		t.Run(id, func(t *testing.T) {
			for _, surface := range []string{"", "editor", "chat", "terminal", "tasklist", "tasklists", "profiles"} {
				for _, mixed := range []bool{false, true} {
					facts := commandbindings.Facts{commandbindings.Profile: "dev"}
					if surface != "" {
						facts[commandbindings.SurfaceType] = surface
					}
					candidate := deckConditionCandidate("mermaid", id, facts)
					rows := []commandbindings.Candidate{candidate}
					if mixed {
						rows = append(rows, deckConditionCandidate("local", "help.shortcuts.show", commandbindings.Facts{commandbindings.Profile: "local"}))
					}
					config, err := commandbindings.NewConfiguration(nil, nil, rows)
					if err != nil {
						t.Fatal(err)
					}
					got := contextualDeckUIConditions(config, registry, candidate.Trigger)
					found := false
					for _, c := range got {
						if c.CommandID != id {
							if c.CommandID != "help.shortcuts.show" || !c.ByProfile["local"].Fallback {
								t.Fatalf("local changed: %+v", c)
							}
							continue
						}
						found = true
						if c.Fallback || c.ByProfile["dev"].Fallback || c.ByProfile["local"].BySurface["editor"] || !c.ByProfile["dev"].BySurface["editor"] {
							t.Fatalf("profile-only leaked: %+v", c)
						}
						for _, other := range []string{"chat", "terminal", "tasklist", "tasklists", "profiles"} {
							if c.ByProfile["dev"].BySurface[other] {
								t.Fatalf("wrong surface: %+v", c)
							}
						}
					}
					if found != (surface == "" || surface == "editor") {
						t.Fatalf("surface=%s mixed=%v: %+v", surface, mixed, got)
					}
					if mixed && len(got) == 0 {
						t.Fatal("local branch lost")
					}
				}
			}
			unconditional := deckConditionCandidate("native", id, nil)
			config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{unconditional})
			if err != nil {
				t.Fatal(err)
			}
			if got := contextualDeckUIConditions(config, registry, unconditional.Trigger); len(got) != 0 {
				t.Fatalf("unconditional became contextual: %+v", got)
			}
			resolved, err := config.Resolve(unconditional.Trigger, nil, nil)
			if err != nil || resolved.Status != commandbindings.Selected || resolved.CommandID != id {
				t.Fatalf("native lost: %+v %v", resolved, err)
			}
		})
	}
}

func TestCommandDeckContextualPageSurfacesAndUnionBarriers(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{"tasklists.duplicate", "tasklists.delete", "tasklists.clear", "profiles.duplicate", "profiles.delete", "profiles.activate"} {
		t.Run(id, func(t *testing.T) {
			definition, exists := registry.Lookup(id)
			if !exists || !commandDeckContextualUIEligible(definition) {
				t.Fatal("missing durable page")
			}
			definition.HandlerClassification = commandcatalog.HandlerUI
			if commandDeckContextualUIEligible(definition) {
				t.Fatal("page admitted as audited UI")
			}
			for _, surface := range []string{"", "profiles", "tasklists", "tasklist", "chat", "editor", "terminal"} {
				facts := commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.Profile: "dev"}
				if surface != "" {
					facts[commandbindings.SurfaceType] = surface
				}
				candidate := deckConditionCandidate("page", id, facts)
				config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
				if err != nil {
					t.Fatal(err)
				}
				got := contextualDeckUIConditions(config, registry, candidate.Trigger)
				allowed := surface == "" || deckPageCommandSurface(id, surface)
				if !allowed {
					if len(got) != 0 {
						t.Fatalf("unsupported %s: %+v", surface, got)
					}
					continue
				}
				if len(got) != 1 || got[0].Fallback || got[0].ByProfile["dev"].Fallback || len(got[0].BySurfaceID) != 0 {
					t.Fatalf("page fallback/ID: %+v", got)
				}
				for _, target := range []string{"profiles", "tasklists", "tasklist", "chat", "editor", "terminal"} {
					want := deckPageCommandSurface(id, target) && (surface == "" || target == surface)
					if got[0].ByProfile["dev"].BySurface[target] != want || got[0].BySurface[target] {
						t.Fatalf("surface=%s target=%s matrix=%+v", surface, target, got)
					}
				}
			}
			for _, barrier := range []commandbindings.Field{commandbindings.SurfaceID, commandbindings.Process, commandbindings.Device} {
				page := deckConditionCandidate("page", id, commandbindings.Facts{commandbindings.Profile: "page"})
				other := deckConditionCandidate("other", commandWorkspaceTabCloseID, commandbindings.Facts{commandbindings.Profile: "workspace", commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one"})
				if barrier != commandbindings.SurfaceID {
					other.LayerConditions = []commandbindings.Facts{{barrier: "app.exe"}}
				}
				local := deckConditionCandidate("local", "help.shortcuts.show", commandbindings.Facts{commandbindings.Profile: "local"})
				config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{page, other, local})
				if err != nil {
					t.Fatal(err)
				}
				got := contextualDeckUIConditions(config, registry, page.Trigger)
				for _, c := range got {
					if c.CommandID == id {
						t.Fatalf("page published with %s: %+v", barrier, got)
					}
				}
				if barrier == commandbindings.SurfaceID {
					if len(got) != 2 {
						t.Fatalf("unrelated branches lost: %+v", got)
					}
					for _, c := range got {
						if c.CommandID == other.CommandID && !c.ByProfile["workspace"].BySurfaceID["chat"]["chat-one"] {
							t.Fatalf("workspace lost: %+v", c)
						}
						if c.CommandID == local.CommandID && !c.ByProfile["local"].Fallback {
							t.Fatalf("local lost: %+v", c)
						}
					}
				} else if len(got) != 0 {
					t.Fatalf("physical facts projected: %+v", got)
				}
			}
		})
	}
}

func TestCommandDeckContextualLayerArgumentsAndNativeProfile(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		t.Run(id, func(t *testing.T) {
			definition, ok := registry.Lookup(id)
			if !ok || !commandDeckContextualUIEligible(definition) {
				t.Fatal("registered durable layer must be eligible")
			}
			definition.HandlerClassification = commandcatalog.HandlerUI
			if commandDeckContextualUIEligible(definition) {
				t.Fatal("layer cannot become audited UI")
			}
			valid := deckContextualTestArguments(id)
			for _, raw := range []string{valid, `{}`, `null`, `[]`, `{"scope":"other","rule_id":"r","duration_seconds":0}`, `{"scope":"global","rule_id":"r","duration_seconds":-1}`, `{"scope":"global","rule_id":"r","duration_seconds":1.5}`, `{"scope":"global","rule_id":"r","duration_seconds":86401}`, `{"scope":"global","rule_id":"r","duration_seconds":0,"forged":true}`} {
				candidate := deckConditionCandidate("layer", id, commandbindings.Facts{commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one", commandbindings.Profile: "dev"})
				candidate.ArgumentsKey = raw
				config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
				if err != nil {
					t.Fatal(err)
				}
				got := contextualDeckUIConditions(config, registry, deckConditionTestTrigger)
				if raw != valid {
					if len(got) != 0 {
						t.Fatalf("invalid args=%s published: %+v", raw, got)
					}
				} else if len(got) != 1 || got[0].CommandID != id || !got[0].ByProfile["dev"].BySurfaceID["chat"]["chat-one"] || got[0].Fallback {
					t.Fatalf("valid args not projected: %+v", got)
				}
			}
			candidate := deckConditionCandidate("layer", id, commandbindings.Facts{commandbindings.Profile: "dev"})
			candidate.ArgumentsKey = valid
			for _, mixed := range []bool{false, true} {
				rows := []commandbindings.Candidate{candidate}
				if mixed {
					rows = append(rows, deckConditionCandidate("local", "help.shortcuts.show", commandbindings.Facts{commandbindings.Profile: "other"}))
				}
				config, err := commandbindings.NewConfiguration(nil, nil, rows)
				if err != nil {
					t.Fatal(err)
				}
				got := contextualDeckUIConditions(config, registry, deckConditionTestTrigger)
				if !mixed && len(got) != 0 {
					t.Fatalf("profile-only must stay native: %+v", got)
				}
				if mixed && len(got) != 2 {
					t.Fatalf("mixed union: %+v", got)
				}
				for _, branch := range got {
					if branch.ByProfile["dev"].Fallback != (branch.CommandID == id) || branch.ByProfile["other"].Fallback != (branch.CommandID == "help.shortcuts.show") {
						t.Fatalf("cross-binding: %+v", branch)
					}
				}
			}
		})
	}
}

func TestCommandDeckContextualConditionsResolveUnionOnce(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	local := deckConditionCandidate("local", "help.shortcuts.show", commandbindings.Facts{commandbindings.Profile: "local"})
	durable := deckConditionCandidate("durable", commandWorkspaceTabCloseID, commandbindings.Facts{commandbindings.Profile: "durable"})
	audited := deckConditionCandidate("audited", commandEditorFormatBoldID, commandbindings.Facts{commandbindings.Profile: "audited"})
	for _, conflict := range []bool{false, true} {
		rows := []commandbindings.Candidate{local, durable, audited}
		if conflict {
			duplicate := durable
			duplicate.ID = "conflict"
			duplicate.CommandID = local.CommandID
			rows = append(rows, duplicate)
		}
		config, err := commandbindings.NewConfiguration(nil, nil, rows)
		if err != nil {
			t.Fatal(err)
		}
		got := contextualDeckUIConditions(config, registry, deckConditionTestTrigger)
		wantCount := 3
		if conflict {
			wantCount = 2
		}
		if len(got) != wantCount {
			t.Fatalf("union conflict=%v: %+v", conflict, got)
		}
		for _, c := range got {
			if c.ByProfile["durable"].Fallback != (!conflict && c.CommandID == durable.CommandID) || c.ByProfile["local"].Fallback != (c.CommandID == local.CommandID) || c.ByProfile["audited"].Fallback != (c.CommandID == audited.CommandID) {
				t.Fatalf("cross-binding selection: %+v", c)
			}
		}
		if got := localDeckUIConditions(config, registry, deckConditionTestTrigger); len(got) != 1 || got[0].CommandID != local.CommandID {
			t.Fatalf("LOCAL_UI changed: %+v", got)
		}
	}
	base := commandbindings.Default{Version: "1", Fingerprint: "fp", Candidate: deckConditionCandidate("base", durable.CommandID, nil)}
	for _, review := range []commandbindings.ReviewStatus{commandbindings.Active, commandbindings.NeedsReview} {
		config, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{{ID: "barrier", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: deckConditionTestTrigger, Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: review, Condition: commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one"}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := contextualDeckUIConditions(config, registry, deckConditionTestTrigger)
		if len(got) != 1 || !got[0].Fallback || got[0].ByProfile["dev"].BySurfaceID["chat"]["chat-one"] {
			t.Fatalf("barrier lost: %+v", got)
		}
	}
}

const deckConditionTestTrigger = "streamdeck.key:test-deck:key:0"

func deckConditionCandidate(id, commandID string, condition commandbindings.Facts) commandbindings.Candidate {
	return commandbindings.Candidate{
		ID:                id,
		Trigger:           deckConditionTestTrigger,
		CommandID:         commandID,
		ArgumentsKey:      `{}`,
		ExecutionScopeKey: "global",
		Scope:             commandbindings.Application,
		Condition:         condition,
		Enabled:           true,
		LayerActive:       true,
	}
}

func TestCommandDeckConditionsAppFocusedOnlyAndProfileOnly(t *testing.T) {
	registry := paletteConditionTestRegistry(t)

	focused, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("focused", "navigation.settings.open", commandbindings.Facts{commandbindings.AppFocused: true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	focusedConditions := localDeckUIConditions(focused, registry, deckConditionTestTrigger)
	if len(focusedConditions) != 1 || !focusedConditions[0].Fallback {
		t.Fatalf("app.focused=true não projetou fallback: %+v", focusedConditions)
	}
	if resolved, err := focused.Resolve(deckConditionTestTrigger, commandbindings.Facts{commandbindings.AppFocused: false}, nil); err != nil || resolved.Status == commandbindings.Selected {
		t.Fatalf("app.focused=false autorizou seleção: %+v err=%v", resolved, err)
	}
	unfocused, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("unfocused", "navigation.settings.open", commandbindings.Facts{commandbindings.AppFocused: false}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := localDeckUIConditions(unfocused, registry, deckConditionTestTrigger); len(got) != 0 {
		t.Fatalf("ação restrita a app sem foco ganhou uma projeção executável: %+v", got)
	}

	profile, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("profile", "navigation.settings.open", commandbindings.Facts{commandbindings.Profile: "dev"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	profileConditions := localDeckUIConditions(profile, registry, deckConditionTestTrigger)
	if len(profileConditions) != 1 || profileConditions[0].Fallback || !profileConditions[0].ByProfile["dev"].Fallback {
		t.Fatalf("perfil-only perdeu ramo dev/fallback desconhecido: %+v", profileConditions)
	}
}

func TestCommandDeckConditionsSurfaceIDProfileSuppressionKeepsFallbacks(t *testing.T) {
	base := commandbindings.Default{
		Version: "1", Fingerprint: "deck-base",
		Candidate: deckConditionCandidate("base", "navigation.settings.open", nil),
	}
	configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{
		{
			ID: "suppress-exact", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "deck-base",
			Trigger: deckConditionTestTrigger, Effect: commandbindings.Suppress,
			Condition: commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-1"},
			Enabled:   true, LayerActive: true, ReviewStatus: commandbindings.Active,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	conditions := localDeckUIConditions(configuration, paletteConditionTestRegistry(t), deckConditionTestTrigger)
	if len(conditions) != 1 {
		t.Fatalf("projeção não criada: %+v", conditions)
	}
	condition := conditions[0]
	profile, ok := condition.ByProfile["dev"]
	if !ok || !condition.Fallback || !condition.BySurface["chat"] || !condition.BySurfaceID["chat"]["chat-1"] {
		t.Fatalf("fallback desconhecido perdeu seleção: %+v", condition)
	}
	if profile.Fallback != true || !profile.BySurface["chat"] || profile.BySurfaceID["chat"]["chat-1"] {
		t.Fatalf("supressão não ficou restrita a profile+surface+id: %+v", profile)
	}
}

func TestCommandDeckConditionsBlockAmbiguityReviewAndDurable(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	ambiguous, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("chat", "navigation.settings.open", commandbindings.Facts{commandbindings.SurfaceType: "chat"}),
		deckConditionCandidate("chat-other", "navigation.menu.open", commandbindings.Facts{commandbindings.SurfaceType: "chat"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := localDeckUIConditions(ambiguous, registry, deckConditionTestTrigger); len(got) != 0 {
		t.Fatalf("ambiguidade virou condição autorizadora: %+v", got)
	}

	base := commandbindings.Default{
		Version: "1", Fingerprint: "review-base",
		Candidate: deckConditionCandidate("base", "navigation.settings.open", nil),
	}
	review, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{
		{ID: "pending", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "review-base", Trigger: deckConditionTestTrigger,
			Effect: commandbindings.Execute, CommandID: "navigation.settings.open", ArgumentsKey: `{}`, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat"},
			Enabled: true, LayerActive: true, ReviewStatus: commandbindings.NeedsReview},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewConditions := localDeckUIConditions(review, registry, deckConditionTestTrigger)
	if len(reviewConditions) != 1 || reviewConditions[0].BySurface["chat"] || !reviewConditions[0].Fallback {
		t.Fatalf("NeedsReview não ficou como barreira: %+v", reviewConditions)
	}

	durable, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("durable", commandWorkspaceTabCloseID, commandbindings.Facts{commandbindings.SurfaceType: "chat"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := localDeckUIConditions(durable, registry, deckConditionTestTrigger); len(got) != 0 {
		t.Fatalf("comando durable foi convertido em local_ui: %+v", got)
	}
}

func TestCommandDeckConditionsRejectPhysicalAndIncompleteVisualFacts(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for name, condition := range map[string]commandbindings.Facts{
		"process": {commandbindings.Process: "editor.exe"},
		"device":  {commandbindings.Device: "other-deck"},
	} {
		t.Run(name, func(t *testing.T) {
			configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
				deckConditionCandidate(name, "navigation.settings.open", condition),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := localDeckUIConditions(configuration, registry, deckConditionTestTrigger); len(got) != 0 {
				t.Fatalf("fato físico foi enumerado: %+v", got)
			}
		})
	}

	incomplete, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("id-without-surface", "navigation.settings.open", commandbindings.Facts{commandbindings.SurfaceID: "chat-1"}),
	})
	if err == nil || incomplete != nil {
		t.Fatalf("surface.id sem surface.type não foi rejeitado: configuration=%v err=%v", incomplete, err)
	}
}
