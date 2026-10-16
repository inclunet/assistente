package commandexecution

import (
	"context"
	"testing"

	"assistente/internal/commandcatalog"
)

func TestValidateCompleteHandlerContractsExigeContratoRealDoCatalogo(t *testing.T) {
	registration := completeEngineRegistration()
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{
		Contract: registration.Handler,
		Start: func(context.Context, Invocation) (ExecutionHandle, error) {
			return ExecutionHandle{}, nil
		},
	}
	if err := validateCompleteHandlerContracts(registry, map[string]Handler{registration.Definition.ID: handler}); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*commandcatalog.HandlerContract){
		"rota":            func(contract *commandcatalog.HandlerContract) { contract.Route = "internal/other" },
		"classificacao":   func(contract *commandcatalog.HandlerContract) { contract.Classification = commandcatalog.HandlerBackend },
		"efeito":          func(contract *commandcatalog.HandlerContract) { contract.Effect = commandcatalog.Destructive },
		"capability":      func(contract *commandcatalog.HandlerContract) { contract.MutatesEffectiveCapability = true },
		"alvo_mutavel":    func(contract *commandcatalog.HandlerContract) { contract.HasMutableTarget = false },
	} {
		t.Run(name, func(t *testing.T) {
			changed := handler
			mutate(&changed.Contract)
			if err := validateCompleteHandlerContracts(registry, map[string]Handler{registration.Definition.ID: changed}); err == nil {
				t.Fatal("contrato divergente aceito")
			}
		})
	}
}

func TestValidateCompleteHandlerContractsExigeCoberturaExata(t *testing.T) {
	registration := completeEngineRegistration()
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompleteHandlerContracts(registry, nil); err == nil {
		t.Fatal("catálogo sem handlers aceito")
	}
	if err := validateCompleteHandlerContracts(registry, map[string]Handler{
		"other.command": {Contract: registration.Handler, Start: func(context.Context, Invocation) (ExecutionHandle, error) { return ExecutionHandle{}, nil }},
	}); err == nil {
		t.Fatal("handler extra/descriptor ausente aceito")
	}
}

func completeEngineRegistration() commandcatalog.Registration {
	return commandcatalog.Registration{
		Definition: commandcatalog.Definition{
			ID: "workspace.tab.new", Effect: commandcatalog.Write,
			Decision: commandcatalog.NoDecision, HasMutableTarget: true,
			AllowedSources: []commandcatalog.Source{commandcatalog.UI},
			Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
			Presentation: &commandcatalog.Presentation{Version: "1", Locales: map[string]commandcatalog.LocalizedMetadata{
				"pt-BR": {Name: "Novo", Description: "Novo", Category: "Workspace"},
				"en":    {Name: "New", Description: "New", Category: "Workspace"},
				"es":    {Name: "Nuevo", Description: "Nuevo", Category: "Workspace"},
			}},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{
				Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted,
			},
			Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace},
			Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: "internal/workspace/tab/new", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Write, HasMutableTarget: true, Route: "internal/workspace/tab/new", Classification: commandcatalog.HandlerInternal},
	}
}
