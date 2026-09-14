package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"gorm.io/gorm"
)

const (
	projectionCommandID      = "workspace.tab.new"
	projectionOtherCommandID = "workspace.tab.close"
	projectionThirdCommandID = "workspace.tab.previous"
	projectionTrigger        = "keyboard.local:Control+KeyK"
	projectionTriggerType    = "keyboard.local"
	projectionTriggerSpec    = `{"version":1,"code":"KeyK","modifiers":["Control"]}`
	projectionCondition      = `{"version":1,"clauses":[]}`
	projectionLayerPriority  = 3
)

type projectionTestFixture struct {
	db       *gorm.DB
	store    *Store
	scope    Scope
	snapshot Snapshot
	registry *commandcatalog.Registry
	options  LocalReadProjection
}

func newProjectionTestFixture(t *testing.T) projectionTestFixture {
	t.Helper()
	db := storeTestDB(t)
	user := storeTestUUID7(t)
	storeTestGeneration(t, db, user, nil, 1)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{UserID: user}
	snapshot, err := store.Load(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	registry := projectionTestRegistry(t, commandcatalog.KeyboardLocal)
	return projectionTestFixture{
		db: db, store: store, scope: scope, snapshot: snapshot,
		registry: registry, options: projectionTestOptions(registry),
	}
}

func projectionTestRegistry(t *testing.T, sources ...commandcatalog.Source) *commandcatalog.Registry {
	return projectionTestRegistryWithIDs(t, []string{projectionCommandID}, sources...)
}

func projectionTestRegistryWithIDs(t *testing.T, ids []string, sources ...commandcatalog.Source) *commandcatalog.Registry {
	t.Helper()
	if len(sources) == 0 {
		sources = []commandcatalog.Source{commandcatalog.KeyboardLocal}
	}
	registrations := make([]commandcatalog.Registration, 0, len(ids))
	for _, id := range ids {
		registrations = append(registrations, commandcatalog.Registration{
			Definition: commandcatalog.Definition{
				ID:                         id,
				Effect:                     commandcatalog.Read,
				Decision:                   commandcatalog.NoDecision,
				AllowedSources:             sources,
				Context:                    commandcatalog.ContextPolicy{None: true},
				HasMutableTarget:           false,
				MutatesEffectiveCapability: false,
			},
			Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read},
		})
	}
	registry, err := commandcatalog.New(registrations)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func projectionTestOptions(registry *commandcatalog.Registry) LocalReadProjection {
	return LocalReadProjection{
		Registry:           registry,
		NoArgumentCommands: []string{projectionCommandID},
		BuiltinLayers: []BuiltinLayer{{
			ID:                 "application.defaults",
			Active:             true,
			ResolutionPriority: projectionLayerPriority,
			Defaults: []commandbindings.Default{projectionTestDefault(
				"builtin.key-k", "fp-v1", projectionLayerPriority,
			)},
		}},
	}
}

func projectionTestDefault(id, fingerprint string, layerPriority int) commandbindings.Default {
	return commandbindings.Default{
		Candidate: commandbindings.Candidate{
			ID:                id,
			Trigger:           projectionTrigger,
			CommandID:         projectionCommandID,
			ArgumentsKey:      "{}",
			ExecutionScopeKey: "global",
			Scope:             commandbindings.Global,
			LayerPriority:     layerPriority,
			Enabled:           true,
			LayerActive:       true,
		},
		Version:     "1",
		Fingerprint: fingerprint,
	}
}

func projectionTestLayer(t *testing.T, fixture projectionTestFixture, enabled bool) Layer {
	t.Helper()
	layer := Layer{
		ID:                 storeTestUUID7(t),
		UserID:             fixture.scope.UserID,
		Name:               "Atalhos locais",
		Description:        "fixture",
		Enabled:            enabled,
		Source:             "user",
		ResolutionPriority: 10,
	}
	if err := fixture.db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	return layer
}

func projectionTestBinding(t *testing.T, fixture projectionTestFixture, layer Layer) Binding {
	t.Helper()
	commandID := projectionCommandID
	row := Binding{
		ID:                 storeTestUUID7(t),
		UserID:             fixture.scope.UserID,
		LayerRefKind:       "user",
		LayerRef:           layer.ID,
		TriggerType:        projectionTriggerType,
		TriggerSpec:        projectionTriggerSpec,
		CommandID:          &commandID,
		Arguments:          "{}",
		Condition:          projectionCondition,
		Effect:             "execute",
		Enabled:            true,
		Source:             "user",
		ResolutionPriority: 20,
		ReviewStatus:       "active",
		Presentation:       "{}",
	}
	if err := fixture.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func projectionTestReload(t *testing.T, fixture projectionTestFixture) Snapshot {
	t.Helper()
	snapshot, err := fixture.store.Load(context.Background(), fixture.scope)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func projectionTestGlobalBinding(t *testing.T, fixture projectionTestFixture, layerID, effect, review, commandID, fingerprint string) Binding {
	t.Helper()
	row := Binding{
		ID:                 storeTestUUID7(t),
		UserID:             fixture.scope.UserID,
		LayerRefKind:       "builtin",
		LayerRef:           layerID,
		TriggerType:        projectionTriggerType,
		TriggerSpec:        projectionTriggerSpec,
		Arguments:          "{}",
		Condition:          projectionCondition,
		Effect:             effect,
		Enabled:            true,
		Source:             "user",
		ResolutionPriority: 20,
		ReviewStatus:       review,
		Presentation:       "{}",
	}
	if commandID != "" {
		row.CommandID = &commandID
	}
	if fingerprint != "" {
		row.ReplacesDefaultID = storeTestPtr("builtin.key-k")
		row.ReplacesDefaultVersion = storeTestPtr("1")
		row.ReplacesDefaultFingerprint = storeTestPtr(fingerprint)
	}
	if err := fixture.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func projectionTestExpectInvalid(t *testing.T, snapshot Snapshot, options LocalReadProjection) {
	t.Helper()
	configuration, err := ProjectLocalRead(context.Background(), snapshot, options)
	if !errors.Is(err, ErrInvalid) || configuration != nil {
		t.Fatalf("projeção inválida aceita: configuration=%v err=%v", configuration, err)
	}
}

func TestProjectLocalReadMigrateLoadResolveNormalizaKeyboardLocal(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	row := projectionTestBinding(t, fixture, layer)
	snapshot := projectionTestReload(t, fixture)
	fixture.options.ActiveUserLayerIDs = []string{layer.ID}

	configuration, err := ProjectLocalRead(context.Background(), snapshot, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{row.ID, "builtin.key-k"}
	if got.Status != commandbindings.Selected || got.CommandID != projectionCommandID ||
		got.ArgumentsKey != "{}" || got.ExecutionScopeKey != "global" ||
		!reflect.DeepEqual(got.BindingIDs, wantIDs) {
		t.Fatalf("resultado projetado inesperado: got=%+v wantIDs=%v", got, wantIDs)
	}
}

func TestProjectLocalReadNaoMascaraGeracaoGlobalAusente(t *testing.T) {
	db := storeTestDB(t)
	user := storeTestUUID7(t)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), Scope{UserID: user}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load sem geração = %v, esperado ErrInvalid", err)
	}
}

func TestProjectLocalReadAtivacaoDeCamadasControlaSelecao(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	projectionTestBinding(t, fixture, layer)
	snapshot := projectionTestReload(t, fixture)

	configuration, err := ProjectLocalRead(context.Background(), snapshot, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil || got.Status != commandbindings.Selected || got.BindingIDs[0] != "builtin.key-k" {
		t.Fatalf("camada de host inativa não preservou fallback: got=%+v err=%v", got, err)
	}

	fixture.options.ActiveUserLayerIDs = []string{layer.ID}
	configuration, err = ProjectLocalRead(context.Background(), snapshot, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err = configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil || got.Status != commandbindings.Selected || got.BindingIDs[0] == "builtin.key-k" {
		t.Fatalf("camada ativa não selecionou override: got=%+v err=%v", got, err)
	}

	disabled := snapshot
	disabled.Layers = append([]Layer(nil), snapshot.Layers...)
	disabled.Layers[0].Enabled = false
	fixture.options.ActiveUserLayerIDs = []string{layer.ID}
	projectionTestExpectInvalid(t, disabled, fixture.options)

	inactiveBuiltin := fixture.options
	inactiveBuiltin.BuiltinLayers = append([]BuiltinLayer(nil), fixture.options.BuiltinLayers...)
	inactiveBuiltin.BuiltinLayers[0].Active = false
	inactiveBuiltin.ActiveUserLayerIDs = nil
	configuration, err = ProjectLocalRead(context.Background(), snapshot, inactiveBuiltin)
	if err != nil {
		t.Fatal(err)
	}
	got, err = configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil || got.Status != commandbindings.NoMatch {
		t.Fatalf("builtin inativo foi selecionado: got=%+v err=%v", got, err)
	}
}

func TestProjectLocalReadExigePrioridadeDoDefaultIgualACamada(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	options := fixture.options
	options.BuiltinLayers = []BuiltinLayer{{
		ID:                 "application.defaults",
		Active:             true,
		ResolutionPriority: projectionLayerPriority,
		Defaults: []commandbindings.Default{projectionTestDefault(
			"builtin.key-k", "fp-v1", projectionLayerPriority+1,
		)},
	}}
	projectionTestExpectInvalid(t, fixture.snapshot, options)
}

func TestProjectLocalReadRecusaDefaultTriggerNaoCanonico(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	options := fixture.options
	options.BuiltinLayers = append([]BuiltinLayer(nil), fixture.options.BuiltinLayers...)
	options.BuiltinLayers[0].Defaults = append([]commandbindings.Default(nil), fixture.options.BuiltinLayers[0].Defaults...)
	options.BuiltinLayers[0].Defaults[0].Candidate.Trigger = "keyboard.local:Shift+Control+KeyK"
	projectionTestExpectInvalid(t, fixture.snapshot, options)
}

func TestProjectLocalReadDeltaBuiltinUsaPrioridadeDaCamada(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	fixture.registry = projectionTestRegistryWithIDs(t, []string{projectionCommandID, projectionOtherCommandID, projectionThirdCommandID})
	fixture.options.Registry = fixture.registry
	fixture.options.NoArgumentCommands = []string{projectionCommandID, projectionOtherCommandID, projectionThirdCommandID}
	otherDefault := projectionTestDefault("builtin.other", "fp-other", 10)
	otherDefault.Candidate.CommandID = projectionThirdCommandID
	fixture.options.BuiltinLayers = []BuiltinLayer{
		{
			ID:                 "application.defaults",
			Active:             true,
			ResolutionPriority: 20,
			Defaults: []commandbindings.Default{
				projectionTestDefault("builtin.key-k", "fp-v1", 20),
			},
		},
		{
			ID:                 "workspace.defaults",
			Active:             true,
			ResolutionPriority: 10,
			Defaults:           []commandbindings.Default{otherDefault},
		},
	}
	row := projectionTestGlobalBinding(t, fixture, "application.defaults", "execute", "active", projectionOtherCommandID, "fp-v1")
	if err := fixture.db.Model(&row).Update("resolution_priority", 0).Error; err != nil {
		t.Fatal(err)
	}

	configuration, err := ProjectLocalRead(context.Background(), projectionTestReload(t, fixture), fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil || got.Status != commandbindings.Selected || got.CommandID != projectionOtherCommandID ||
		!reflect.DeepEqual(got.BindingIDs, []string{row.ID}) {
		t.Fatalf("delta não herdou a prioridade da camada builtin: got=%+v err=%v", got, err)
	}
}

func TestProjectLocalReadValidaAllowlistCatalogoEOrigem(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *projectionTestFixture)
	}{
		{
			name:   "sem allowlist",
			change: func(_ *testing.T, f *projectionTestFixture) { f.options.NoArgumentCommands = nil },
		},
		{
			name: "comando desconhecido na allowlist",
			change: func(_ *testing.T, f *projectionTestFixture) {
				f.options.BuiltinLayers = nil
				f.options.NoArgumentCommands = []string{"unknown.command"}
			},
		},
		{
			name: "origem não permitida",
			change: func(t *testing.T, f *projectionTestFixture) {
				f.registry = projectionTestRegistry(t, commandcatalog.UI)
				f.options.Registry = f.registry
			},
		},
		{
			name: "comando desconhecido no binding",
			change: func(t *testing.T, f *projectionTestFixture) {
				layer := projectionTestLayer(t, *f, true)
				unknown := "unknown.command"
				row := projectionTestBinding(t, *f, layer)
				if err := f.db.Model(&row).Update("command_id", unknown).Error; err != nil {
					t.Fatal(err)
				}
				f.snapshot = projectionTestReload(t, *f)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			// The closure receives a value-backed fixture only to keep each case
			// independent; all database mutations remain in this subtest's DB.
			tc.change(t, &fixture)
			projectionTestExpectInvalid(t, fixture.snapshot, fixture.options)
		})
	}
}

func TestProjectLocalReadRecusaEscopoEOwnerIncoerentes(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	projectionTestBinding(t, fixture, layer)
	base := projectionTestReload(t, fixture)
	otherUser := storeTestUUID7(t)
	otherWorkspace := storeTestUUID7(t)

	cases := []struct {
		name   string
		change func(*Snapshot)
	}{
		{name: "owner da camada", change: func(s *Snapshot) { s.Layers[0].UserID = otherUser }},
		{name: "owner do binding", change: func(s *Snapshot) { s.Bindings[0].UserID = otherUser }},
		{name: "workspace fora do escopo", change: func(s *Snapshot) { s.Bindings[0].WorkspaceID = &otherWorkspace }},
		{name: "scope do snapshot", change: func(s *Snapshot) { s.Scope.UserID = otherUser }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := base
			snapshot.Layers = append([]Layer(nil), base.Layers...)
			snapshot.Bindings = append([]Binding(nil), base.Bindings...)
			tc.change(&snapshot)
			projectionTestExpectInvalid(t, snapshot, fixture.options)
		})
	}
}

func TestProjectLocalReadRecusaCustomNeedsReviewMesmoDesabilitado(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	row := projectionTestBinding(t, fixture, layer)
	if err := fixture.db.Model(&row).Updates(map[string]any{"review_status": "needs_review", "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := projectionTestReload(t, fixture)
	projectionTestExpectInvalid(t, snapshot, fixture.options)
}

func TestProjectLocalReadRecusaDocumentosNaoSuportadosMesmoDesabilitados(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Binding)
	}{
		{name: "trigger versionado desconhecido", change: func(row *Binding) { row.TriggerSpec = `{"version":2,"code":"KeyK","modifiers":["Control"]}` }},
		{name: "tipo de trigger desconhecido", change: func(row *Binding) { row.TriggerType = "hotkey" }},
		{name: "condição versionada desconhecida", change: func(row *Binding) { row.Condition = `{"version":2,"clauses":[]}` }},
		{name: "argumentos não vazios", change: func(row *Binding) { row.Arguments = `{"version":1}` }},
		{name: "apresentação não vazia", change: func(row *Binding) { row.Presentation = `{"version":1}` }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			layer := projectionTestLayer(t, fixture, true)
			row := projectionTestBinding(t, fixture, layer)
			row.Enabled = false
			tc.change(&row)
			if err := fixture.db.Model(&row).Updates(map[string]any{
				"trigger_type": row.TriggerType, "trigger_spec": row.TriggerSpec,
				"condition": row.Condition, "arguments": row.Arguments,
				"presentation": row.Presentation, "enabled": false,
			}).Error; err != nil {
				t.Fatal(err)
			}
			projectionTestExpectInvalid(t, projectionTestReload(t, fixture), fixture.options)
		})
	}
}

func TestProjectLocalReadMaterializaTombstoneSemFallback(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	row := projectionTestGlobalBinding(t, fixture, "application.defaults", "suppress", "active", "", "fp-v1")
	configuration, err := ProjectLocalRead(context.Background(), projectionTestReload(t, fixture), fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != commandbindings.Suppressed || got.CommandID != "" || !reflect.DeepEqual(got.BindingIDs, []string{row.ID}) {
		t.Fatalf("tombstone inesperado: got=%+v", got)
	}
}

func TestProjectLocalReadMudancaSemanticaFicaNeedsReviewSemFallback(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	row := projectionTestGlobalBinding(t, fixture, "application.defaults", "execute", "active", projectionCommandID, "old-fingerprint")
	configuration, err := ProjectLocalRead(context.Background(), projectionTestReload(t, fixture), fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != commandbindings.ReviewRequired || got.CommandID != "" || !reflect.DeepEqual(got.BindingIDs, []string{row.ID}) {
		t.Fatalf("mudança semântica caiu em fallback: got=%+v", got)
	}
}

func TestProjectLocalReadRecusaTombstoneNoBuiltinErrado(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	fixture.options.BuiltinLayers = []BuiltinLayer{
		{ID: "application.defaults", Active: true, ResolutionPriority: 10, Defaults: []commandbindings.Default{
			projectionTestDefault("builtin.key-k", "fp-v1", 10),
		}},
		{ID: "workspace.defaults", Active: true, ResolutionPriority: 20},
	}
	row := projectionTestGlobalBinding(t, fixture, "application.defaults", "suppress", "active", "", "fp-v1")
	if _, err := ProjectLocalRead(context.Background(), projectionTestReload(t, fixture), fixture.options); err != nil {
		t.Fatalf("referência builtin válida foi recusada: %v", err)
	}
	if err := fixture.db.Model(&row).Update("layer_ref", "workspace.defaults").Error; err != nil {
		t.Fatal(err)
	}
	projectionTestExpectInvalid(t, projectionTestReload(t, fixture), fixture.options)
}

func TestProjectLocalReadIsolaSnapshotProjetadoEResultados(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	row := projectionTestBinding(t, fixture, layer)
	snapshot := projectionTestReload(t, fixture)
	options := fixture.options
	options.ActiveUserLayerIDs = []string{layer.ID}
	configuration, err := ProjectLocalRead(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}

	snapshot.Bindings[0].CommandID = storeTestPtr("mutated.command")
	snapshot.Bindings[0].TriggerSpec = `{"version":2}`
	options.BuiltinLayers[0].Defaults[0].Candidate.CommandID = "mutated.command"
	options.ActiveUserLayerIDs[0] = "mutated-layer"

	got, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil || got.Status != commandbindings.Selected || got.CommandID != projectionCommandID || got.BindingIDs[0] != row.ID {
		t.Fatalf("entrada mutada vazou para configuração: got=%+v err=%v", got, err)
	}
	got.BindingIDs[0] = "mutated-result"
	again, err := configuration.Resolve(projectionTrigger, nil, nil)
	if err != nil || again.CommandID != projectionCommandID || again.BindingIDs[0] != row.ID {
		t.Fatalf("resultado mutado vazou para snapshot: got=%+v err=%v", again, err)
	}
}
