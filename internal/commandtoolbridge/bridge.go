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
	"assistente/internal/jobprofilegrant"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"

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
	CommandID     string
	ToolName      string
	ToolCatalogID string
	// Authorize é uma porta de bootstrap. Ela roda no worker, via
	// ExecuteRequest.BeforeExecute, nunca durante Start/gate.
	Authorize      func(context.Context, commandexecution.Invocation) error
	ToolGeneration uint64
	// PrepareContext instala a origem privada/contexto de domínio no worker,
	// depois do snapshot da invocação e antes do executor comum. O release
	// devolvido é chamado ao fim ou quando a preparação falha.
	PrepareContext func(context.Context, commandexecution.Invocation) (context.Context, func(), error)
	SensitivePaths commandcatalog.SensitivePaths
	Contract       commandcatalog.HandlerContract
	Definition     commandcatalog.Definition
	// OutputAdapter converte o conteúdo da tool para o JSON de resultado do
	// comando. Sem adapter, conteúdo não-JSON não prova ausência de efeitos e
	// termina como outcome_unknown.
	OutputAdapter func(tools.ToolResult) (json.RawMessage, error)
	// InputAdapter transforma somente os argumentos entregues ao executor da
	// tool. O envelope autenticado e assinado permanece imutável.
	InputAdapter func(json.RawMessage) (json.RawMessage, error)
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
	if err := validateSubagentProfileArgument(route.ToolName, invocation.Envelope, arguments); err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	toolContext, origin, err := bridgeToolContext(invocation)
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	snapshot := cloneBridgeInvocation(invocation)

	toolCall := tools.ToolCall{
		ID: snapshot.ID, Type: "function",
		Function: tools.FunctionCall{Name: route.ToolName, Arguments: string(arguments)},
	}
	executionCtx, cancel := context.WithCancel(invocationctx.With(database.WithUserID(ctx, invocation.Principal.UserID), toolContext))
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
		var beforeExecute func(context.Context) error
		if route.Authorize != nil {
			beforeExecute = func(checkCtx context.Context) error {
				return route.Authorize(checkCtx, snapshot)
			}
		}
		serviceCtx := executionCtx
		var prepareRelease func()
		outcomeSent := false
		defer func() {
			if recover() != nil && !outcomeSent {
				if prepareRelease != nil {
					releaseBridgeContext(prepareRelease)
				}
				done <- commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
			}
		}()
		if route.PrepareContext != nil {
			preparedCtx, release, prepareErr := callPrepareContext(route.PrepareContext, executionCtx, snapshot)
			if release != nil {
				prepareRelease = onceRelease(release)
			}
			if prepareErr != nil || preparedCtx == nil {
				if prepareRelease != nil {
					prepareRelease()
				}
				if prepareErr != nil {
					done <- commandexecution.Outcome{Status: commandledger.Failed}
				} else {
					done <- commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
				}
				return
			}
			if err := executionCtx.Err(); err != nil || preparedCtx.Err() != nil {
				if prepareRelease != nil {
					releaseBridgeContext(prepareRelease)
				}
				done <- commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
				return
			}
			if owner, ok := database.UserIDFromContext(executionCtx); ok {
				preparedCtx = database.WithUserID(preparedCtx, owner)
			}
			if originalInvocationContext, ok := invocationctx.Get(executionCtx); ok {
				preparedCtx = invocationctx.With(preparedCtx, originalInvocationContext)
			}
			var cancelCombined func()
			serviceCtx, cancelCombined = mergeBridgeContexts(executionCtx, preparedCtx)
			defer cancelCombined()
		}
		if route.InputAdapter != nil {
			adapted, adaptErr := callInputAdapter(route.InputAdapter, arguments)
			if adaptErr != nil || len(adapted) == 0 || !json.Valid(adapted) {
				if prepareRelease != nil {
					releaseBridgeContext(prepareRelease)
				}
				done <- commandexecution.Outcome{Status: commandledger.Failed}
				outcomeSent = true
				return
			}
			toolCall.Function.Arguments = string(adapted)
		}
		result := b.service.Execute(serviceCtx, toolinvocations.ExecuteRequest{
			Call: toolCall, ToolCatalogID: route.ToolCatalogID,
			Origin:                        origin,
			SensitivePaths:                route.SensitivePaths,
			RequireCompleteResult:         true,
			RequireCanonicalToolCatalogID: true,
			ExpectedToolGeneration:        route.ToolGeneration,
			BeforeExecute:                 beforeExecute,
		})
		outcome := outcomeFor(route, result)
		if prepareRelease != nil && !releaseBridgeContext(prepareRelease) {
			outcome = commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
		}
		done <- outcome
		outcomeSent = true
	}()
	return handle, nil
}

func callInputAdapter(adapter func(json.RawMessage) (json.RawMessage, error), raw json.RawMessage) (result json.RawMessage, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, ErrInvalidInvocation
		}
	}()
	return adapter(append(json.RawMessage(nil), raw...))
}

func callPrepareContext(prepare func(context.Context, commandexecution.Invocation) (context.Context, func(), error), ctx context.Context, snapshot commandexecution.Invocation) (prepared context.Context, release func(), err error) {
	defer func() {
		if recover() != nil {
			err = ErrInvalidInvocation
		}
	}()
	return prepare(ctx, snapshot)
}

func onceRelease(release func()) func() {
	var once sync.Once
	return func() { once.Do(release) }
}

func releaseBridgeContext(release func()) (ok bool) {
	if release == nil {
		return true
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	release()
	return true
}

func mergeBridgeContexts(original, prepared context.Context) (context.Context, func()) {
	combined, cancel := context.WithCancel(prepared)
	stopOriginal := context.AfterFunc(original, cancel)
	merged := combined
	var cancelDeadline context.CancelFunc
	if deadline, ok := original.Deadline(); ok {
		merged, cancelDeadline = context.WithDeadline(combined, deadline)
	}
	return merged, func() {
		stopOriginal()
		if cancelDeadline != nil {
			cancelDeadline()
		}
		cancel()
	}
}

func cloneBridgeInvocation(invocation commandexecution.Invocation) commandexecution.Invocation {
	clone := invocation
	if invocation.Envelope != nil {
		envelope := invocation.Envelope.Clone()
		clone.Envelope = &envelope
	}
	return clone
}

func bridgeToolContext(invocation commandexecution.Invocation) (invocationctx.InvocationContext, toolinvocations.Origin, error) {
	envelope := invocation.Envelope
	if envelope == nil {
		return invocationctx.InvocationContext{}, toolinvocations.Origin{}, ErrInvalidInvocation
	}
	conversationID, turnID, err := pairedUUIDs(envelope.ConversationID, envelope.TurnID)
	if err != nil {
		return invocationctx.InvocationContext{}, toolinvocations.Origin{}, err
	}
	surfaceType, surfaceID, surfaceVersion, err := surfaceIdentity(envelope)
	if err != nil {
		return invocationctx.InvocationContext{}, toolinvocations.Origin{}, err
	}
	sourceProfile, _, err := profileIdentity(envelope)
	if err != nil {
		return invocationctx.InvocationContext{}, toolinvocations.Origin{}, err
	}
	if envelope.ActorType != "" && envelope.ActorID == "" {
		return invocationctx.InvocationContext{}, toolinvocations.Origin{}, ErrInvalidInvocation
	}
	if envelope.ActorType == commandcontract.ActorUser && envelope.ActorID != invocation.Principal.UserID {
		return invocationctx.InvocationContext{}, toolinvocations.Origin{}, ErrOwnerRequired
	}

	var surfaceContext map[string]any
	if surfaceType != "" {
		surfaceContext = map[string]any{
			"surfaceType":     surfaceType,
			"surfaceId":       surfaceID,
			"snapshotVersion": surfaceVersion,
		}
	}
	return invocationctx.InvocationContext{
			ConversationID: conversationID,
			TurnID:         turnID,
			ProfileSlug:    sourceProfile,
			Source:         string(invocation.Source),
			TabType:        surfaceType,
			SurfaceTabID:   surfaceID,
			SurfaceContext: surfaceContext,
		}, toolinvocations.Origin{
			Type:           toolinvocations.OriginCommandInvocation,
			ID:             invocation.ID,
			ConversationID: conversationID,
			TurnID:         turnID,
		}, nil
}

// ValidateInvocationContext verifica o lineage opcional que será propagado ao
// executor de tools sem reescrever o envelope autenticado da invocação.
func ValidateInvocationContext(invocation commandexecution.Invocation) error {
	_, _, err := bridgeToolContext(invocation)
	return err
}

// validateSubagentProfileArgument valida apenas a superfície especial da tool
// subagent. Tools genéricas não ganham semântica implícita para um campo
// chamado profile. Quando o envelope declara um destino, o argumento efetivo
// precisa ser esse destino; argumento omitido herda o source profile e só é
// aceito quando os dois perfis coincidem.
func validateSubagentProfileArgument(toolName string, envelope *commandcontract.Envelope, arguments json.RawMessage) error {
	if toolName != jobprofilegrant.ToolSubagent || envelope == nil || envelope.TargetProfileSlug == nil {
		return nil
	}
	target := *envelope.TargetProfileSlug
	source := ""
	if envelope.SourceProfileSlug != nil {
		source = *envelope.SourceProfileSlug
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &fields); err != nil || fields == nil {
		return ErrInvalidInvocation
	}
	rawProfile, present := fields["profile"]
	if !present {
		if source != target {
			return ErrInvalidInvocation
		}
		return nil
	}
	var profile *string
	if err := json.Unmarshal(rawProfile, &profile); err != nil || profile == nil || *profile != target {
		return ErrInvalidInvocation
	}
	return nil
}

func pairedUUIDs(conversationID, turnID *string) (string, string, error) {
	if (conversationID == nil) != (turnID == nil) {
		return "", "", ErrInvalidInvocation
	}
	if conversationID == nil {
		return "", "", nil
	}
	if !validUUIDv7(strings.TrimSpace(*conversationID)) || !validUUIDv7(strings.TrimSpace(*turnID)) || strings.TrimSpace(*conversationID) != *conversationID || strings.TrimSpace(*turnID) != *turnID {
		return "", "", ErrInvalidInvocation
	}
	return *conversationID, *turnID, nil
}

func surfaceIdentity(envelope *commandcontract.Envelope) (string, string, string, error) {
	values := []*string{envelope.SurfaceType, envelope.SurfaceID, envelope.SurfaceSnapshotVersion}
	present := 0
	for _, value := range values {
		if value != nil {
			present++
			if strings.TrimSpace(*value) == "" || strings.TrimSpace(*value) != *value || strings.ContainsRune(*value, '\x00') {
				return "", "", "", ErrInvalidInvocation
			}
		}
	}
	if present != 0 && present != len(values) {
		return "", "", "", ErrInvalidInvocation
	}
	if present == 0 {
		return "", "", "", nil
	}
	return *envelope.SurfaceType, *envelope.SurfaceID, *envelope.SurfaceSnapshotVersion, nil
}

func profileIdentity(envelope *commandcontract.Envelope) (string, string, error) {
	var source, target string
	if envelope.SourceProfileSlug != nil {
		source = *envelope.SourceProfileSlug
	}
	if envelope.TargetProfileSlug != nil {
		target = *envelope.TargetProfileSlug
	}
	if source != strings.TrimSpace(source) || target != strings.TrimSpace(target) ||
		(envelope.SourceProfileSlug != nil && source == "") ||
		(envelope.TargetProfileSlug != nil && target == "") ||
		strings.ContainsRune(source, '\x00') || strings.ContainsRune(target, '\x00') || (source == "" && target != "") {
		return "", "", ErrInvalidInvocation
	}
	if envelope.ActorType == commandcontract.ActorAgent && (source == "" || target == "") {
		return "", "", ErrInvalidInvocation
	}
	if envelope.ActorType == commandcontract.ActorUser && target != "" {
		return "", "", ErrInvalidInvocation
	}
	return source, target, nil
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
