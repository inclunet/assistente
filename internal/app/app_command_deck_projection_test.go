package app

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandadapter"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandinput"
	"github.com/google/uuid"
)

func TestCommandDeckCompiledProjectionMatchesOriginalMixedMatrix(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		deckConditionCandidate("durable", commandWorkspaceTabCloseID, commandbindings.Facts{commandbindings.SurfaceType: "chat"}),
		deckConditionCandidate("local", "navigation.settings.open", commandbindings.Facts{commandbindings.Profile: "other"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := p.host.Snapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	compiled := deckTriggerIdentities(configuration, p.registry, versions)
	triggers := compiled["test-deck"][0]
	if len(triggers) != 1 {
		t.Fatalf("projeção compilada perdeu a tecla: %+v", compiled)
	}
	trigger := triggers[0]
	wantConditions := contextualDeckUIConditions(configuration, p.registry, deckConditionTestTrigger)
	if !reflect.DeepEqual(trigger.conditions, wantConditions) {
		t.Fatalf("matriz compilada divergiu da matriz original: got=%+v want=%+v", trigger.conditions, wantConditions)
	}
	if !reflect.DeepEqual(trigger.required, configuration.RequiredFacts(deckConditionTestTrigger)) {
		t.Fatalf("required facts compilados divergiram: got=%v want=%v", trigger.required, configuration.RequiredFacts(deckConditionTestTrigger))
	}
	if trigger.configuration != configuration || trigger.registry != p.registry || trigger.versions != versions {
		t.Fatal("trigger compilado não reteve o snapshot exato")
	}
	ids := map[string]bool{}
	for _, condition := range trigger.conditions {
		ids[condition.CommandID] = true
	}
	if !ids[commandWorkspaceTabCloseID] || !ids["navigation.settings.open"] {
		t.Fatalf("a matriz mista não preservou os dois vencedores: %+v", trigger.conditions)
	}
}

func TestCommandDeckResolveRejectsRetiredSnapshot(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	configuration, _, versions, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	identities := deckTriggerIdentities(configuration, p.registry, versions)
	if _, _, ok := p.resolveDeckPress(context.Background(), identities, versions, "test-deck", 0); !ok {
		t.Fatal("snapshot publicado não resolveu a tecla configurada")
	}
	settings, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	var layerID string
	for _, layer := range settings.Layers {
		if !layer.Builtin && layer.Active {
			layerID = layer.ID
			break
		}
	}
	if layerID == "" {
		t.Fatal("fixture não publicou camada customizada ativa")
	}
	if _, err := a.SetCommandLayerActive(layerID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := p.resolveDeckPress(context.Background(), identities, versions, "test-deck", 0); ok {
		t.Fatal("mapa de snapshot aposentado resolveu após troca de geração")
	}
}

func TestCommandDeckProjectionRejectsMismatchedSnapshotEvenWithCurrentVersions(t *testing.T) {
	p, compiled := newDeckResolveBenchmarkFixture(t)
	ctx := context.Background()
	configuration := smallConfiguration(p, t)
	if err := p.host.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return p.principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, _, current, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := p.resolveDeckPress(ctx, compiled, current, "test-deck", 0); ok {
		t.Fatal("old projection accepted by replacing only the controller versions")
	}
	fresh := deckTriggerIdentities(configuration, p.registry, current)
	if _, _, ok := p.resolveDeckPress(ctx, fresh, current, "test-deck", 0); !ok {
		t.Fatal("fresh projection refused")
	}
	other, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fresh["test-deck"][0][0].configuration = other
	if _, _, ok := p.resolveDeckPress(ctx, fresh, current, "test-deck", 0); ok {
		t.Fatal("projection with a different configuration accepted")
	}
	fresh = deckTriggerIdentities(configuration, p.registry, current)
	fresh["test-deck"][0][0].registry = deckRegistryWithNonCandidates(t, p.registry, 0)
	if _, _, ok := p.resolveDeckPress(ctx, fresh, current, "test-deck", 0); ok {
		t.Fatal("projection with a different registry accepted")
	}
}

func TestCommandDeckControllerRejectsRetiredGeneration(t *testing.T) {
	a := deckConfiguredFixture(t)
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	configuration, _, versions, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &commandDeckController{
		p: p, ctx: ctx, versions: versions,
		identities: deckTriggerIdentities(configuration, p.registry, versions),
		pressed:    map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{},
		generation: p.deckInputGeneration(),
	}
	a.emitter = commandOSBootstrapEmitter(func(string, any) {})
	c.opened("test-deck")
	defer c.reset("")
	press := commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyDown}
	if ack, err := c.Input(ctx, press); err != nil || !ack.Accepted {
		t.Fatalf("press inicial não foi aceito: %+v %v", ack, err)
	}
	if _, err := c.Input(ctx, commandadapter.Event{SourceInstance: press.SourceInstance, Key: press.Key, Kind: commandinput.KeyUp}); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.deckCaptureGeneration++
	p.mu.Unlock()
	if ack, err := c.Input(ctx, press); !errors.Is(err, commandexecution.ErrStale) || ack.Accepted {
		t.Fatalf("geração aposentada foi aceita: %+v %v", ack, err)
	}
}

func TestCommandDeckResolveDoesNotEnumerateCatalog(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller não encontrou o arquivo de teste")
	}
	path := filepath.Join(filepath.Dir(file), "app_command_deck.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var resolve *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "resolveDeckPress" {
			resolve = function
			break
		}
	}
	if resolve == nil {
		t.Fatal("resolveDeckPress não encontrado")
	}
	var forbidden string
	ast.Inspect(resolve.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			switch function := call.Fun.(type) {
			case *ast.Ident:
				if function.Name == "contextualDeckUIConditions" {
					forbidden = function.Name
				}
			case *ast.SelectorExpr:
				if function.Sel.Name == "List" {
					forbidden = "registry.List"
				}
			}
		}
		return forbidden == ""
	})
	if forbidden != "" {
		t.Fatalf("resolveDeckPress voltou a enumerar catálogo: %s", forbidden)
	}
}

func TestCommandDeckResolveAllocationsDoNotScaleWithNonCandidates(t *testing.T) {
	base, small := newDeckResolveBenchmarkFixture(t)
	largeRegistry := deckRegistryWithNonCandidates(t, base.registry, 1024)
	large := &commandProductRuntime{app: base.app, host: base.host, principal: base.principal, registry: largeRegistry}
	_, _, versions, err := large.host.ResolutionSnapshot(context.Background(), large.principal)
	if err != nil {
		t.Fatal(err)
	}
	largeMap := deckTriggerIdentities(smallConfiguration(base, t), largeRegistry, versions)
	smallAllocs := testing.AllocsPerRun(50, func() {
		if _, _, ok := base.resolveDeckPress(context.Background(), small, versions, "test-deck", 0); !ok {
			t.Fatal("resolução pequena falhou")
		}
	})
	largeAllocs := testing.AllocsPerRun(50, func() {
		if _, _, ok := large.resolveDeckPress(context.Background(), largeMap, versions, "test-deck", 0); !ok {
			t.Fatal("resolução grande falhou")
		}
	})
	if smallAllocs != largeAllocs {
		t.Fatalf("allocations por press variaram com não-candidatos: small=%v large=%v", smallAllocs, largeAllocs)
	}
}

func BenchmarkCommandDeckResolvePressCatalogCardinality(b *testing.B) {
	base, small := newDeckResolveBenchmarkFixture(b)
	largeRegistry := deckRegistryWithNonCandidates(b, base.registry, 1024)
	large := &commandProductRuntime{app: base.app, host: base.host, principal: base.principal, registry: largeRegistry}
	_, _, versions, err := large.host.ResolutionSnapshot(context.Background(), large.principal)
	if err != nil {
		b.Fatal(err)
	}
	configuration := smallConfiguration(base, b)
	largeMap := deckTriggerIdentities(configuration, largeRegistry, versions)
	b.Run("catalogo-base", func(b *testing.B) {
		count := len(base.registry.List())
		b.ResetTimer()
		b.ReportAllocs()
		b.ReportMetric(float64(count), "catalog.commands")
		for i := 0; i < b.N; i++ {
			if _, _, ok := base.resolveDeckPress(context.Background(), small, versions, "test-deck", 0); !ok {
				b.Fatal("resolução pequena falhou")
			}
		}
	})
	b.Run("catalogo-com-1024-nao-candidatos", func(b *testing.B) {
		count := len(largeRegistry.List())
		b.ResetTimer()
		b.ReportAllocs()
		b.ReportMetric(float64(count), "catalog.commands")
		for i := 0; i < b.N; i++ {
			if _, _, ok := large.resolveDeckPress(context.Background(), largeMap, versions, "test-deck", 0); !ok {
				b.Fatal("resolução grande falhou")
			}
		}
	})
}

func newDeckResolveBenchmarkFixture(tb testing.TB) (*commandProductRuntime, map[string]map[int][]commandDeckCompiledTrigger) {
	tb.Helper()
	app := &App{}
	registry, _, err := app.commandProductCatalog()
	if registry == nil || (err != nil && !errors.Is(err, commandexecution.ErrInvalidConfiguration)) {
		tb.Fatalf("catálogo produtivo: registry=%v err=%v", registry != nil, err)
	}
	service, err := app.commandSecurityService()
	if err != nil {
		tb.Fatal(err)
	}
	host, err := commandexecution.NewHostState(service, commandProductRegistryVersion)
	if err != nil {
		tb.Fatal(err)
	}
	principal := auth.LocalSessionPrincipal{UserID: uuid.Must(uuid.NewV7()).String(), SessionID: uuid.Must(uuid.NewV7()).String()}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "deck-benchmark", Trigger: "streamdeck.key:test-deck:key:0", CommandID: "navigation.settings.open",
		ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true,
	}})
	if err != nil {
		tb.Fatal(err)
	}
	ctx := context.Background()
	if err := host.SetVaultUnlocked(ctx, true); err != nil {
		tb.Fatal(err)
	}
	if err := host.SetOSSessionState(ctx, true, false); err != nil {
		tb.Fatal(err)
	}
	if err := host.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return principal, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		tb.Fatal(err)
	}
	_, _, versions, err := host.ResolutionSnapshot(ctx, principal)
	if err != nil {
		tb.Fatal(err)
	}
	p := &commandProductRuntime{app: app, host: host, principal: principal, registry: registry}
	return p, deckTriggerIdentities(configuration, registry, versions)
}

func smallConfiguration(p *commandProductRuntime, t testing.TB) *commandbindings.Configuration {
	t.Helper()
	configuration, _, _, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	return configuration
}

func deckRegistryWithNonCandidates(tb testing.TB, base *commandcatalog.Registry, count int) *commandcatalog.Registry {
	tb.Helper()
	definitions := base.List()
	registrations := make([]commandcatalog.Registration, 0, len(definitions)+count)
	for _, definition := range definitions {
		registrations = append(registrations, commandcatalog.Registration{Definition: definition, Handler: commandcatalog.HandlerContract{Effect: definition.Effect, MutatesEffectiveCapability: definition.MutatesEffectiveCapability}})
	}
	template := definitions[0]
	for i := 0; i < count; i++ {
		definition := template
		definition.ID = fmt.Sprintf("benchmark.non_candidate.command%04d", i)
		registrations = append(registrations, commandcatalog.Registration{Definition: definition, Handler: commandcatalog.HandlerContract{Effect: definition.Effect, MutatesEffectiveCapability: definition.MutatesEffectiveCapability}})
	}
	registry, err := commandcatalog.New(registrations)
	if err != nil {
		tb.Fatalf("catálogo expandido: %v", err)
	}
	return registry
}
