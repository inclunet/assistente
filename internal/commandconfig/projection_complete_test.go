package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"github.com/google/uuid"
)

const (
	completeProjectionCommand   = "workspace.tab.new"
	completeProjectionTrigger   = `{"version":1,"code":"KeyA","modifiers":[]}`
	completeProjectionCondition = `{"version":1,"clauses":[]}`
)

func completeProjectionUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func completeProjectionRegistry(t *testing.T, extra ...commandcatalog.Registration) *commandcatalog.Registry {
	return completeProjectionRegistryWithSources(t, []commandcatalog.Source{commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal}, extra...)
}

func completeProjectionRegistryWithSources(t *testing.T, sources []commandcatalog.Source, extra ...commandcatalog.Registration) *commandcatalog.Registry {
	t.Helper()
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Novo", Description: "Cria", Category: "Workspace"},
		"en":    {Name: "New", Description: "Create", Category: "Workspace"},
		"es":    {Name: "Nuevo", Description: "Crea", Category: "Workspace"},
	}
	registration := commandcatalog.Registration{
		Definition: commandcatalog.Definition{
			ID: completeProjectionCommand, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
			AllowedSources:  sources,
			Context:         commandcatalog.ContextPolicy{None: true},
			Presentation:    &commandcatalog.Presentation{Version: "1", Locales: locales},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			Risk:            commandcatalog.RiskLow,
			Persistence: commandcatalog.PersistencePolicy{
				Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted,
			},
			Scopes:       []commandcatalog.Scope{commandcatalog.ScopeGlobal},
			Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: "internal/workspace/tab/new", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/workspace/tab/new", Classification: commandcatalog.HandlerInternal},
	}
	registrations := append([]commandcatalog.Registration{registration}, extra...)
	registry, err := commandcatalog.NewComplete(registrations)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func completeProjectionArgumentRegistration() commandcatalog.Registration {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Renomear", Description: "Renomeia", Category: "Workspace"},
		"en":    {Name: "Rename", Description: "Rename", Category: "Workspace"},
		"es":    {Name: "Renombrar", Description: "Renombra", Category: "Workspace"},
	}
	return commandcatalog.Registration{
		Definition: commandcatalog.Definition{
			ID: "workspace.tab.rename", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
			AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal}, Context: commandcatalog.ContextPolicy{None: true},
			Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
				"label": {Type: commandcatalog.SchemaString},
			}, Required: []string{"label"}},
			ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, Risk: commandcatalog.RiskLow,
			Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
			Scopes:      []commandcatalog.Scope{commandcatalog.ScopeGlobal}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: "internal/workspace/tab/rename", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/workspace/tab/rename", Classification: commandcatalog.HandlerInternal},
	}
}

func completeProjectionOptions(registry *commandcatalog.Registry) CompleteProjection {
	port := TriggerPortFunc(func(_ context.Context, raw []byte) (string, error) {
		if string(raw) != completeProjectionTrigger {
			return "", errors.New("fixture: trigger não reconhecido")
		}
		return "keyboard.local:KeyA", nil
	})
	return CompleteProjection{
		Registry:     registry,
		TriggerPorts: map[commandcatalog.Source]TriggerPort{commandcatalog.KeyboardLocal: port},
		BuiltinLayers: []BuiltinLayer{{
			ID: "application.defaults", Active: true, ResolutionPriority: 1,
			Defaults: []commandbindings.Default{{
				Candidate: commandbindings.Candidate{
					ID: "builtin.tab.new", Trigger: "keyboard.local:KeyA", CommandID: completeProjectionCommand,
					ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global,
					LayerPriority: 1, Enabled: true, LayerActive: true,
				}, Version: "1", Fingerprint: "fp-v1",
			}},
		}},
	}
}

func completeProjectionLayer(t *testing.T, user, workspace string, priority int) Layer {
	t.Helper()
	return Layer{ID: completeProjectionUUID(t), UserID: user, WorkspaceID: &workspace,
		Name: "Camada", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: priority}
}

func completeProjectionBinding(t *testing.T, user string, workspace *string, layer Layer, id string) Binding {
	t.Helper()
	command := completeProjectionCommand
	return Binding{ID: id, UserID: user, WorkspaceID: workspace, LayerRefKind: "user", LayerRef: layer.ID,
		TriggerType: string(commandcatalog.KeyboardLocal), TriggerSpec: completeProjectionTrigger,
		CommandID: &command, Arguments: "{}", Condition: completeProjectionCondition, Effect: "execute",
		Enabled: true, Source: "user", ResolutionPriority: 2, ReviewStatus: "active", Presentation: `{"version":1,"title_key":"commands.tab.new"}`}
}

func TestProjectCompleteMaterializaGlobalEWorkspaceComPortaConfiavel(t *testing.T) {
	user := completeProjectionUUID(t)
	workspace := "workspace-opaco"
	globalLayer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 3}
	workspaceLayer := completeProjectionLayer(t, user, workspace, 4)
	globalID, workspaceID := completeProjectionUUID(t), completeProjectionUUID(t)
	globalBinding := completeProjectionBinding(t, user, nil, globalLayer, globalID)
	workspaceBinding := completeProjectionBinding(t, user, &workspace, workspaceLayer, workspaceID)
	snapshot := Snapshot{Scope: Scope{UserID: user, WorkspaceID: &workspace}, Layers: []Layer{globalLayer, workspaceLayer}, Bindings: []Binding{globalBinding, workspaceBinding}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.ActiveUserLayerIDs = []string{globalLayer.ID, workspaceLayer.ID}

	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || got.Status != commandbindings.Selected || got.CommandID != completeProjectionCommand {
		t.Fatalf("configuração global/workspace não projetada: got=%+v err=%v", got, err)
	}
	if !slices.Contains(got.BindingIDs, workspaceID) || len(got.BindingIDs) != 3 {
		t.Fatalf("composição global/workspace não preservou a proveniência: got=%v", got.BindingIDs)
	}
	if !reflect.DeepEqual(got.LayerRefs, []string{globalLayer.ID, workspaceLayer.ID, "application.defaults"}) {
		t.Fatalf("camadas efetivas incorretas: got=%v", got.LayerRefs)
	}
}

func TestProjectCompleteRejeitaTriggerSemPortaOuIdentidadeLivre(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{row}}
	registry := completeProjectionRegistry(t)
	options := completeProjectionOptions(registry)
	options.BuiltinLayers = nil
	options.TriggerPorts = nil
	if configuration, err := ProjectComplete(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || configuration != nil {
		t.Fatalf("trigger sem porta aceito: configuration=%v err=%v", configuration, err)
	}
	options = completeProjectionOptions(registry)
	options.TriggerPorts[commandcatalog.KeyboardLocal] = TriggerPortFunc(func(context.Context, []byte) (string, error) {
		return "custom.identity", nil
	})
	if configuration, err := ProjectComplete(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || configuration != nil {
		t.Fatalf("identidade não namespaceada pela origem aceita: configuration=%v err=%v", configuration, err)
	}
}

func TestProjectCompleteGlobalExigePortaGlobalEAllowedSource(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	row.TriggerType = string(commandcatalog.KeyboardGlobal)
	row.TriggerSpec = `{"version":1,"code":"KeyA","modifiers":["Control"]}`
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{row}}
	registry := completeProjectionRegistryWithSources(t, []commandcatalog.Source{commandcatalog.KeyboardGlobal})

	options := completeProjectionOptions(registry)
	options.BuiltinLayers = nil
	options.TriggerPorts = map[commandcatalog.Source]TriggerPort{}
	if configuration, err := ProjectComplete(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || configuration != nil {
		t.Fatalf("global sem porta aceita: configuration=%v err=%v", configuration, err)
	}

	options.TriggerPorts[commandcatalog.KeyboardGlobal] = KeyboardGlobalTriggerPort{}
	options.ActiveUserLayerIDs = []string{layer.ID}
	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve("keyboard.global:Control+KeyA", nil, nil)
	if err != nil || got.Status != commandbindings.Selected || got.CommandID != completeProjectionCommand || len(got.BindingIDs) != 1 || got.BindingIDs[0] != row.ID {
		t.Fatalf("global explicitamente projetado de forma inesperada: got=%+v err=%v", got, err)
	}
}

func TestProjectCompleteNormalizaArgumentosEValidaApresentacao(t *testing.T) {
	registration := completeProjectionRegistry(t, completeProjectionArgumentRegistration())
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	command := "workspace.tab.rename"
	row.CommandID = &command
	row.Arguments = `{"label":"renomeado"}`
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{row}}
	options := completeProjectionOptions(registration)
	options.BuiltinLayers = nil
	options.ActiveUserLayerIDs = []string{layer.ID}
	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || got.ArgumentsKey != row.Arguments || got.ExecutionScopeKey != "global" {
		t.Fatalf("argumentos/escopo não normalizados: got=%+v err=%v", got, err)
	}
	row.Presentation = `{"version":1,"unexpected":true}`
	snapshot.Bindings[0] = row
	if configuration, err := ProjectComplete(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || configuration != nil {
		t.Fatalf("apresentação inválida aceita: configuration=%v err=%v", configuration, err)
	}
}

func TestProjectCompleteRejeitaImageRefInvalidaOuImagemEmbutida(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers = nil
	options.ActiveUserLayerIDs = []string{layer.ID}
	invalidPresentations := []string{
		`{"version":1,"image_ref":""}`,
		`{"version":1,"image_ref":null}`,
		`{"version":1,"image_ref":123}`,
		`{"version":1,"image_ref":"` + strings.Repeat("a", 63) + `"}`,
		`{"version":1,"image_ref":"` + strings.Repeat("a", 65) + `"}`,
		`{"version":1,"image_ref":"` + strings.Repeat("A", 64) + `"}`,
		`{"version":1,"image_ref":"` + strings.Repeat("g", 64) + `"}`,
		`{"version":1,"image_ref":" ` + strings.Repeat("a", 63) + ` "}`,
		`{"version":1,"image_data":"` + strings.Repeat("a", 64) + `"}`,
	}
	for _, presentation := range invalidPresentations {
		row.Presentation = presentation
		snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{row}}
		if configuration, err := ProjectComplete(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || configuration != nil {
			t.Fatalf("image_ref inválida aceita: presentation=%s configuration=%v err=%v", presentation, configuration, err)
		}
	}
}

func TestProjectCompleteNeedsReviewRespeitaContextoEAdjustmentDeVersao(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	row.LayerRefKind, row.LayerRef = "builtin", "application.defaults"
	row.ReplacesDefaultID, row.ReplacesDefaultVersion, row.ReplacesDefaultFingerprint = stringPtr("builtin.tab.new"), stringPtr("0"), stringPtr("fp-v1")
	row.ReviewStatus = "needs_review"
	row.Condition = `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`
	snapshot := Snapshot{Scope: Scope{UserID: user}, Bindings: []Binding{row}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers[0].Defaults[0].Version = "2"

	configuration, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	adjustments := configuration.Adjustments()
	if len(adjustments) != 1 || adjustments[0].DefaultVersion != "2" {
		t.Fatalf("avanço de versão não foi comunicado: %+v", adjustments)
	}
	outside, err := configuration.Resolve("keyboard.local:KeyA", commandbindings.Facts{commandbindings.Profile: "prod"}, nil)
	if err != nil || outside.Status != commandbindings.Selected {
		t.Fatalf("pendência vazou para fora do contexto: got=%+v err=%v", outside, err)
	}
	inside, err := configuration.Resolve("keyboard.local:KeyA", commandbindings.Facts{commandbindings.Profile: "dev"}, nil)
	if err != nil || inside.Status != commandbindings.ReviewRequired || !reflect.DeepEqual(inside.BindingIDs, []string{row.ID}) {
		t.Fatalf("contexto pendente não foi bloqueado: got=%+v err=%v", inside, err)
	}
}

func stringPtr(value string) *string { return &value }
