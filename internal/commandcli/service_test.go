package commandcli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

func TestNewAndDiscoveryUseAuthenticatedCatalog(t *testing.T) {
	registry := testRegistry(t)
	authCalls := 0
	service, err := New(Config{
		Registry: registry,
		Executor: &commandexecution.Service{},
		Authenticate: func(context.Context) error {
			authCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.List(context.Background(), "en")
	if err != nil || len(items) != 1 || items[0].Name != "Read command" || items[0].Executable != true {
		t.Fatalf("list=%+v err=%v", items, err)
	}
	description, err := service.Describe(context.Background(), "test.read", "fr")
	if err != nil || description.Name != "Comando de leitura" || description.ID != "test.read" {
		t.Fatalf("describe=%+v err=%v", description, err)
	}
	if authCalls != 4 {
		t.Fatalf("autenticações=%d, esperado 4 (antes e depois de cada leitura)", authCalls)
	}
}

func TestDescriptionJSONUsesSnakeCaseAndSharedSchema(t *testing.T) {
	registry := testRegistry(t)
	service, err := New(Config{Registry: registry, Executor: &commandexecution.Service{}, Authenticate: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	description, err := service.Describe(context.Background(), "test.read", "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	wantSchema := commandcatalog.JSONSchema(registryDefinition(t).ArgumentsSchema)
	if !reflect.DeepEqual(description.ArgumentsSchema, wantSchema) {
		t.Fatalf("schema divergente: got=%#v want=%#v", description.ArgumentsSchema, wantSchema)
	}
	encoded, err := json.Marshal(description)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"allowed_sources", "unavailable_reason", "arguments_schema"} {
		if !containsJSONField(encoded, field) {
			t.Fatalf("campo não está em snake_case: %s (%s)", field, encoded)
		}
	}
}

func TestExecuteGeneratesUUIDv7AndKeepsItOnExecutorError(t *testing.T) {
	service, err := New(Config{
		Registry: testRegistry(t),
		Executor: &commandexecution.Service{},
		Authenticate: func(context.Context) error {
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), Request{CommandID: "test.read", Arguments: json.RawMessage(`{}`)})
	if err == nil || result.RequestID == "" || !validUUIDv7(result.RequestID) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestExecuteRejectsCallerRequestIDAndRetryStatusRequireUUIDv7(t *testing.T) {
	service, err := New(Config{Registry: testRegistry(t), Executor: &commandexecution.Service{}, Authenticate: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(context.Background(), Request{CommandID: "test.read", RequestID: "caller-id"}); !errors.Is(err, ErrRequestIDOnExecute) {
		t.Fatalf("request_id de execute: %v", err)
	}
	if result, err := service.Retry(context.Background(), Request{CommandID: "test.read", RequestID: "caller-id"}); !errors.Is(err, ErrInvalidRequest) || result.RequestID != "caller-id" {
		t.Fatalf("retry inválido: %+v %v", result, err)
	}
	if result, err := service.Status(context.Background(), "caller-id"); !errors.Is(err, ErrInvalidRequest) || result.RequestID != "caller-id" {
		t.Fatalf("status inválido: %+v %v", result, err)
	}
}

func TestTerminalStatusesBecomeErrorsButStatusProjectionDoesNot(t *testing.T) {
	for _, status := range []commandledger.Status{
		commandledger.Denied,
		commandledger.RejectedStale,
		commandledger.Failed,
		commandledger.OutcomeUnknown,
		commandledger.TimedOut,
	} {
		err := terminalStatusError(status)
		var statusErr *StatusError
		if !errors.As(err, &statusErr) || statusErr.Status != status || !errors.Is(err, ErrCommandFailed) {
			t.Fatalf("status=%s err=%v", status, err)
		}
	}
	if terminalStatusError(commandledger.Succeeded) != nil {
		t.Fatal("sucesso virou erro")
	}
}

func TestUnavailableReasonUsesHeadlessGates(t *testing.T) {
	base := commandcatalog.Definition{
		ID: "test.command", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.CLI}, Context: commandcatalog.ContextPolicy{None: true},
		Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerClassification: commandcatalog.HandlerBackend,
	}
	tests := []struct {
		name   string
		mutate func(*commandcatalog.Definition)
		want   string
	}{
		{"source", func(d *commandcatalog.Definition) { d.AllowedSources = nil }, "source_cli_not_allowed"},
		{"decision", func(d *commandcatalog.Definition) { d.Decision = commandcatalog.Interactive }, "decision_required"},
		{"capability", func(d *commandcatalog.Definition) { d.MutatesEffectiveCapability = true }, "mutates_effective_capability"},
		{"destructive", func(d *commandcatalog.Definition) { d.Effect = commandcatalog.Destructive }, "destructive_effect"},
		{"ui", func(d *commandcatalog.Definition) { d.HandlerClassification = commandcatalog.HandlerUI }, "handler_ui"},
		{"context", func(d *commandcatalog.Definition) { d.Context = commandcatalog.ContextPolicy{} }, "visual_context_required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := base
			test.mutate(&definition)
			if got := UnavailableReason(definition); got != test.want {
				t.Fatalf("got=%q want=%q", got, test.want)
			}
		})
	}
	if got := UnavailableReason(base); got != "" {
		t.Fatalf("comando permitido ficou indisponível: %q", got)
	}
}

func testRegistry(t *testing.T) *commandcatalog.Registry {
	t.Helper()
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: registryDefinition(t), Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/test/read", Classification: commandcatalog.HandlerBackend}}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func registryDefinition(t *testing.T) commandcatalog.Definition {
	t.Helper()
	return commandcatalog.Definition{
		ID: "test.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.CLI}, Context: commandcatalog.ContextPolicy{None: true},
		Presentation: &commandcatalog.Presentation{Version: "test", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Comando de leitura", Description: "Lê dados", Category: "Teste"},
			"en":    {Name: "Read command", Description: "Reads data", Category: "Test"},
			"es":    {Name: "Comando de lectura", Description: "Lee datos", Category: "Prueba"},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
			"limit": {Type: commandcatalog.SchemaInteger, Optional: true},
		}},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          "internal/test/read",
		HandlerClassification: commandcatalog.HandlerBackend,
	}
}

func containsJSONField(raw []byte, field string) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value[field] != nil
}
