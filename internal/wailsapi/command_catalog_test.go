package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/commandcatalog"
)

func TestCommandCatalogNotWired(t *testing.T) {
	t.Parallel()
	api := NewCommandCatalog()
	if _, err := api.ListCommands(apidto.CommandCatalogFilter{}); !errors.Is(err, ErrCommandCatalogNotWired) {
		t.Fatalf("ListCommands: got %v", err)
	}
	if _, err := api.DescribeCommand("fixture.ready", apidto.CommandCatalogFilter{}); !errors.Is(err, ErrCommandCatalogNotWired) {
		t.Fatalf("DescribeCommand: got %v", err)
	}
}

func TestCommandCatalogRequiresAuthenticatedSession(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sem sessão")
	api := NewCommandCatalog()
	AttachCommandCatalog(api, stubSession{err: wantErr}, commandCatalogFixture(t), nil)
	if _, err := api.ListCommands(apidto.CommandCatalogFilter{}); !errors.Is(err, wantErr) {
		t.Fatalf("ListCommands: got %v, want %v", err, wantErr)
	}
}

func TestCommandCatalogWithoutBackendReadinessIsUnavailable(t *testing.T) {
	t.Parallel()
	api := NewCommandCatalog()
	AttachCommandCatalog(api, stubSession{ctx: context.Background()}, commandCatalogFixture(t), nil)
	list, err := api.ListCommands(apidto.CommandCatalogFilter{Source: string(commandcatalog.Palette)})
	if err != nil {
		t.Fatal(err)
	}
	if list[1].Available || !strings.Contains(list[1].ReadinessReason, "runtime") {
		t.Fatalf("catálogo sem callback não fechou: %#v", list[1])
	}
}

func TestCommandCatalogListsSearchesAndDescribes(t *testing.T) {
	t.Parallel()
	api := NewCommandCatalog()
	AttachCommandCatalog(api, stubSession{ctx: context.Background()}, commandCatalogFixture(t), func(context.Context, commandcatalog.Definition, commandcatalog.Source) error { return nil })

	list, err := api.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Source: string(commandcatalog.Palette)})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "fixture.blocked" || list[1].ID != "fixture.ready" {
		t.Fatalf("ordem/lista = %#v", list)
	}
	if list[0].Available {
		t.Fatal("comando indisponível no catálogo não deve aparecer como disponível")
	}
	if !list[1].Available || list[1].Name != "Abrir paleta" || list[1].Category != "Navegação" {
		t.Fatalf("item disponível inesperado: %#v", list[1])
	}

	search, err := api.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Query: "atalho", Source: string(commandcatalog.Palette)})
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 || search[0].ID != "fixture.ready" {
		t.Fatalf("busca por alias = %#v", search)
	}

	keyboard, err := api.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Query: "paleta", Source: string(commandcatalog.KeyboardGlobal)})
	if err != nil {
		t.Fatal(err)
	}
	if len(keyboard) != 1 || keyboard[0].Available || !strings.Contains(keyboard[0].ReadinessReason, "origem") {
		t.Fatalf("readiness por origem = %#v", keyboard)
	}

	detail, err := api.DescribeCommand("fixture.ready", apidto.CommandCatalogFilter{Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Name != "Open palette" || detail.ArgumentsSchema == nil || detail.ResultSchema == nil || !detail.ContextNone {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestCommandCatalogUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "command_catalog.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("command_catalog.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("command_catalog.go deve chamar WithUser(session,")
	}
}

func TestCommandCatalogWriteRequiresTrustedRuntimePolicy(t *testing.T) {
	t.Parallel()
	definition, _ := commandCatalogFixture(t).Lookup("fixture.ready")
	definition.Effect = commandcatalog.Write
	definition.HasMutableTarget = true
	definition.HandlerClassification = commandcatalog.HandlerBackend
	definition.Context = commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: commandcatalog.HandlerContract{
		Effect: definition.Effect, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: definition.HandlerClassification,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"absent", "denied", "ready"} {
		t.Run(mode, func(t *testing.T) {
			api := NewCommandCatalog()
			var readiness CommandCatalogReadiness
			if mode != "absent" {
				readiness = func(_ context.Context, got commandcatalog.Definition, source commandcatalog.Source) error {
					if got.ID != definition.ID || source != commandcatalog.Palette || mode == "denied" {
						return commandcatalog.ErrNotReady
					}
					return nil
				}
			}
			AttachCommandCatalog(api, stubSession{ctx: context.Background()}, registry, readiness)
			items, err := api.ListCommands(apidto.CommandCatalogFilter{Source: "palette"})
			if err != nil || len(items) != 1 || items[0].Available != (mode == "ready") {
				t.Fatalf("policy=%s items=%+v error=%v", mode, items, err)
			}
			items, err = api.ListCommands(apidto.CommandCatalogFilter{Source: "keyboard.global"})
			if err != nil || len(items) != 1 || items[0].Available {
				t.Fatalf("origem não permitida: items=%+v error=%v", items, err)
			}
		})
	}
}

func commandCatalogFixture(t *testing.T) *commandcatalog.Registry {
	t.Helper()
	schema := &commandcatalog.Schema{Type: commandcatalog.SchemaObject}
	ready := commandcatalog.Definition{
		ID:             "fixture.ready",
		Effect:         commandcatalog.Read,
		Decision:       commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette},
		Context:        commandcatalog.ContextPolicy{None: true},
		Presentation: &commandcatalog.Presentation{Version: "1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Abrir paleta", Description: "Abre a paleta", Category: "Navegação", Aliases: []string{"atalho"}},
			"en":    {Name: "Open palette", Description: "Opens the palette", Category: "Navigation", Aliases: []string{"shortcut"}},
			"es":    {Name: "Abrir paleta", Description: "Abre la paleta", Category: "Navegación", Aliases: []string{"atajo"}},
		}},
		ArgumentsSchema:       schema,
		ResultSchema:          schema,
		Risk:                  commandcatalog.RiskLow,
		Persistence:           commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted},
		Scopes:                []commandcatalog.Scope{commandcatalog.ScopeSession},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          "fixture/ready",
		HandlerClassification: commandcatalog.HandlerInternal,
	}
	blocked := ready
	blocked.ID = "fixture.blocked"
	blocked.Presentation = &commandcatalog.Presentation{Version: "1", Locales: map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Comando bloqueado", Description: "Indisponível", Category: "Diagnóstico", Aliases: []string{"bloqueado"}},
		"en":    {Name: "Blocked command", Description: "Unavailable", Category: "Diagnostics", Aliases: []string{"blocked"}},
		"es":    {Name: "Comando bloqueado", Description: "No disponible", Category: "Diagnóstico", Aliases: []string{"bloqueado"}},
	}}
	blocked.Availability = commandcatalog.Availability{Status: commandcatalog.Unavailable, Reason: "missing_runtime"}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{
		{Definition: ready, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "fixture/ready", Classification: commandcatalog.HandlerInternal}},
		{Definition: blocked, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "fixture/ready", Classification: commandcatalog.HandlerInternal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
