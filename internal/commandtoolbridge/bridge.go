// Package commandtoolbridge adapta handlers tipados de comandos para o
// executor comum de tools. A ponte não conhece nem chama Tool.Execute: toda
// execução passa por toolinvocations.Service e pelo ledger de tools.
package commandtoolbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/google/uuid"
)

var (
	ErrInvalidConfiguration = errors.New("configuração da ponte de tools inválida")
	ErrInvalidInvocation    = errors.New("invocação de comando inválida para ponte de tools")
	ErrOwnerRequired        = errors.New("owner autenticado é obrigatório para delegação de tool")
	ErrSystemDelegation     = errors.New("contexto system não pode delegar para tool")
)

var canonicalCommandID = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Route é a prova confiável de que um command_id delega para uma tool
// específica. O ID do catálogo é obrigatório e nunca é resolvido por nome.
// Definition é opcional para permitir a montagem em duas fases; quando
// presente, seus metadados de handler e paths sensíveis são verificados.
type Route struct {
	CommandID      string
	ToolName       string
	ToolCatalogID  string
	SensitivePaths commandcatalog.SensitivePaths
	Contract       commandcatalog.HandlerContract
	Definition     commandcatalog.Definition
	// OutputAdapter converte o conteúdo da tool para o JSON de resultado do
	// comando. Sem adapter, conteúdo não-JSON não prova ausência de efeitos e
	// termina como outcome_unknown.
	OutputAdapter func(tools.ToolResult) (json.RawMessage, error)
}

type Config struct {
	Service *toolinvocations.Service
	Routes  []Route
}

type Bridge struct {
	service *toolinvocations.Service
	routes  map[string]Route
}

// New valida e copia o mapa de delegações. Nenhuma rota parcial ou duplicada
// é aceita, pois isso poderia transformar um command_id canônico em outra
// tool após o bootstrap.
func New(config Config) (*Bridge, error) {
	if config.Service == nil || len(config.Routes) == 0 {
		return nil, ErrInvalidConfiguration
	}
	routes := make(map[string]Route, len(config.Routes))
	for _, route := range config.Routes {
		if !validCommandID(route.CommandID) || strings.TrimSpace(route.CommandID) != route.CommandID ||
			strings.TrimSpace(route.ToolName) == "" || route.ToolName != strings.TrimSpace(route.ToolName) ||
			strings.TrimSpace(route.ToolCatalogID) == "" || route.ToolCatalogID != strings.TrimSpace(route.ToolCatalogID) {
			return nil, fmt.Errorf("%w: rota incompleta", ErrInvalidConfiguration)
		}
		if _, exists := routes[route.CommandID]; exists {
			return nil, fmt.Errorf("%w: command_id duplicado", ErrInvalidConfiguration)
		}
		if route.Definition.ID != "" {
			if route.Definition.ID != route.CommandID || route.Definition.HandlerClassification != commandcatalog.HandlerTool {
				return nil, fmt.Errorf("%w: definição não é uma rota de tool", ErrInvalidConfiguration)
			}
			if route.Contract == (commandcatalog.HandlerContract{}) {
				return nil, fmt.Errorf("%w: contrato explícito obrigatório com definição", ErrInvalidConfiguration)
			}
			merged := route.Definition
			merged.SensitivePaths.Input = unionSensitivePaths(route.Definition.SensitivePaths.Input, route.SensitivePaths.Input)
			merged.SensitivePaths.Output = unionSensitivePaths(route.Definition.SensitivePaths.Output, route.SensitivePaths.Output)
			if err := commandcatalog.ValidateDefinitionComplete(merged, route.Contract); err != nil {
				return nil, fmt.Errorf("%w: contrato/paths incompatíveis: %v", ErrInvalidConfiguration, err)
			}
			route.SensitivePaths = merged.SensitivePaths
		}
		if route.Definition.ID == "" && route.Contract.Classification == "" {
			route.Contract.Classification = commandcatalog.HandlerTool
		}
		if route.Contract.Classification != "" && route.Contract.Classification != commandcatalog.HandlerTool {
			return nil, fmt.Errorf("%w: classificação de handler não é tool", ErrInvalidConfiguration)
		}
		route.SensitivePaths.Input = append([]string(nil), route.SensitivePaths.Input...)
		route.SensitivePaths.Output = append([]string(nil), route.SensitivePaths.Output...)
		routes[route.CommandID] = route
	}
	return &Bridge{service: config.Service, routes: routes}, nil
}

// Handler retorna o handler do command_id exato. O booleano distingue
// ausência de rota de uma rota cujo contrato ainda esteja sendo validado pelo
// executor de comandos.
func (b *Bridge) Handler(commandID string) (commandexecution.Handler, bool) {
	if b == nil {
		return commandexecution.Handler{}, false
	}
	route, ok := b.routes[commandID]
	if !ok {
		return commandexecution.Handler{}, false
	}
	return commandexecution.Handler{
		Contract: route.Contract,
		Start: func(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
			return b.start(ctx, route, invocation)
		},
	}, true
}

// Handlers devolve uma cópia das rotas para o wiring do commandexecution
// Service. Os handlers compartilham o mesmo Service e, portanto, o mesmo
// executor/ledger.
func (b *Bridge) Handlers() map[string]commandexecution.Handler {
	result := make(map[string]commandexecution.Handler)
	if b == nil {
		return result
	}
	for commandID := range b.routes {
		if handler, ok := b.Handler(commandID); ok {
			result[commandID] = handler
		}
	}
	return result
}

func (b *Bridge) start(ctx context.Context, route Route, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	if ctx == nil || ctx.Err() != nil || b == nil || b.service == nil {
		return commandexecution.ExecutionHandle{}, ErrInvalidInvocation
	}
	if invocation.Source == commandcatalog.System {
		return commandexecution.ExecutionHandle{}, ErrSystemDelegation
	}
	if invocation.Envelope != nil && invocation.Envelope.AuthContextType == commandcontract.AuthSystem {
		return commandexecution.ExecutionHandle{}, ErrSystemDelegation
	}
	if !validUUIDv7(invocation.ID) || invocation.CommandID != route.CommandID ||
		!validUUIDv7(invocation.Principal.UserID) {
		if strings.TrimSpace(invocation.Principal.UserID) == "" {
			return commandexecution.ExecutionHandle{}, ErrOwnerRequired
		}
		return commandexecution.ExecutionHandle{}, ErrInvalidInvocation
	}
	if existing, ok := database.UserIDFromContext(ctx); ok && existing != invocation.Principal.UserID {
		return commandexecution.ExecutionHandle{}, ErrOwnerRequired
	}
	if invocation.Envelope == nil {
		return commandexecution.ExecutionHandle{}, ErrInvalidInvocation
	}
	if invocation.Envelope.InvocationID != invocation.ID || invocation.Envelope.UserID == nil ||
		!validUUIDv7(*invocation.Envelope.UserID) || *invocation.Envelope.UserID != invocation.Principal.UserID ||
		invocation.Envelope.CommandID == nil || *invocation.Envelope.CommandID != route.CommandID {
		return commandexecution.ExecutionHandle{}, ErrInvalidInvocation
	}
	arguments := json.RawMessage(`{}`)
	if invocation.Envelope.Arguments != nil {
		arguments = append(arguments[:0], (*invocation.Envelope.Arguments)...)
	}
	if !json.Valid(arguments) {
		return commandexecution.ExecutionHandle{}, ErrInvalidInvocation
	}

	toolCall := tools.ToolCall{
		ID: invocation.ID, Type: "function",
		Function: tools.FunctionCall{Name: route.ToolName, Arguments: string(arguments)},
	}
	executionCtx, cancel := context.WithCancel(database.WithUserID(ctx, invocation.Principal.UserID))
	done := make(chan commandexecution.Outcome, 1)
	var cancelOnce sync.Once
	handle := commandexecution.ExecutionHandle{ID: invocation.ID, Done: done, Cancel: func() { cancelOnce.Do(cancel) }}

	go func() {
		defer cancel()
		defer close(done)
		defer func() {
			if recover() != nil {
				done <- commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
			}
		}()
		result := b.service.Execute(executionCtx, toolinvocations.ExecuteRequest{
			Call: toolCall, ToolCatalogID: route.ToolCatalogID,
			Origin:                        toolinvocations.Origin{Type: toolinvocations.OriginCommandInvocation, ID: invocation.ID},
			SensitivePaths:                route.SensitivePaths,
			RequireCompleteResult:         true,
			RequireCanonicalToolCatalogID: true,
		})
		done <- outcomeFor(route, result)
	}()
	return handle, nil
}

func outcomeFor(route Route, result toolinvocations.ExecuteResult) commandexecution.Outcome {
	if result.Execution.ErrorKind == tools.ErrorKindCancelled {
		return commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
	}
	if !result.Persisted && result.Invocation.ID != "" && result.Execution.Error == nil && !result.Execution.Result.IsError {
		return commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
	}
	if result.Execution.Error != nil || result.Execution.Result.IsError || result.Execution.ErrorKind != tools.ErrorKindNone {
		return commandexecution.Outcome{Status: commandledger.Failed}
	}
	var (
		payload json.RawMessage
		err     error
	)
	if route.OutputAdapter != nil {
		payload, err = route.OutputAdapter(result.Execution.Result)
	} else {
		payload = json.RawMessage(strings.TrimSpace(result.Execution.Result.Content))
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if err != nil || !json.Valid(payload) {
		return commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
	}
	return commandexecution.Outcome{Status: commandledger.Succeeded, Result: payload}
}

func unionSensitivePaths(required, additional []string) []string {
	result := make([]string, 0, len(required)+len(additional))
	seen := make(map[string]struct{}, len(required)+len(additional))
	for _, paths := range [][]string{required, additional} {
		for _, path := range paths {
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			result = append(result, path)
		}
	}
	return slices.Clone(result)
}

func validCommandID(value string) bool {
	return canonicalCommandID.MatchString(value)
}

func validUUIDv7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
