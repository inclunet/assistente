// Package commandcli adapta o ingresso de terminal ao executor canônico de
// comandos. O pacote não conhece handlers, banco ou autenticação concreta:
// essas decisões continuam no runtime que monta o commandexecution.Service.
package commandcli

import (
	"context"
	"encoding/json"
	"errors"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

const privateExecutorToken = ""

var (
	ErrInvalidConfiguration = errors.New("configuração de CLI inválida")
	ErrInvalidRequest       = errors.New("solicitação de CLI inválida")
	ErrRequestIDOnExecute   = errors.New("request_id só é aceito em retry")
	ErrCommandMismatch      = errors.New("command_id não corresponde à invocação")
	ErrCommandFailed        = errors.New("execução de comando falhou")
)

// StatusError permite ao chamador traduzir um resultado terminal em código de
// saída não-zero sem perder a projeção Result que contém o request_id.
type StatusError struct {
	Status commandledger.Status
}

func (e *StatusError) Error() string {
	return "execução de comando: " + string(e.Status)
}

func (e *StatusError) Unwrap() error { return ErrCommandFailed }

// Config conecta a borda CLI ao catálogo e ao executor já montado pelo host.
// Authenticate é deliberadamente uma função sem token: o runtime resolve a
// sessão local e o escopo real antes de expor este serviço ao processo CLI.
type Config struct {
	Registry     *commandcatalog.Registry
	Executor     *commandexecution.Service
	Authenticate func(context.Context) error
}

// Service é uma fachada síncrona. Ele não possui scheduler nem executor
// alternativo; cada operação de execução converge para Executor.
type Service struct {
	registry     *commandcatalog.Registry
	executor     *commandexecution.Service
	authenticate func(context.Context) error
}

// Request é o DTO público da CLI.
type Request struct {
	CommandID string          `json:"command_id"`
	Arguments json.RawMessage `json:"arguments"`
	RequestID string          `json:"request_id"`
}

// Result é a projeção segura do ledger. Output só existe para uma execução
// nova que terminou com sucesso; consultas e replays não recuperam output.
type Result struct {
	RequestID     string          `json:"request_id"`
	Status        string          `json:"status"`
	ResultSummary *string         `json:"result_summary,omitempty"`
	ErrorCode     *string         `json:"error_code,omitempty"`
	Output        json.RawMessage `json:"output,omitempty"`
}

// Description é a projeção de catálogo consumível por terminal e automação.
type Description struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Description       string                  `json:"description"`
	AllowedSources    []commandcatalog.Source `json:"allowed_sources"`
	Executable        bool                    `json:"executable"`
	UnavailableReason string                  `json:"unavailable_reason"`
	ArgumentsSchema   map[string]any          `json:"arguments_schema,omitempty"`
}

// New valida as dependências obrigatórias. O executor deve ser o serviço
// completo já publicado para a origem CLI; a fachada não o reconstrói.
func New(config Config) (*Service, error) {
	if config.Registry == nil || config.Executor == nil || config.Authenticate == nil {
		return nil, ErrInvalidConfiguration
	}
	return &Service{
		registry:     config.Registry,
		executor:     config.Executor,
		authenticate: config.Authenticate,
	}, nil
}

// UnavailableReason aplica os gates headless comuns da CLI. O retorno é um
// código estável, não uma mensagem traduzida. A execução ainda passa pelo
// executor quando há uma definição conhecida, para que a recusa seja durável
// e revalidada pelo host.
func UnavailableReason(d commandcatalog.Definition) string {
	if d.Availability.Status != commandcatalog.Available {
		if d.Availability.Reason != "" {
			return d.Availability.Reason
		}
		return "unavailable"
	}
	if !d.AllowsSource(commandcatalog.CLI) {
		return "source_cli_not_allowed"
	}
	if d.Decision != commandcatalog.NoDecision {
		return "decision_required"
	}
	if d.MutatesEffectiveCapability {
		return "mutates_effective_capability"
	}
	if d.Effect == commandcatalog.Destructive {
		return "destructive_effect"
	}
	if d.HandlerClassification == commandcatalog.HandlerUI {
		return "handler_ui"
	}
	if !d.Context.None {
		return "visual_context_required"
	}
	return ""
}

// List autentica o escopo e devolve todo o catálogo, inclusive itens
// indisponíveis para terminal.
func (s *Service) List(ctx context.Context, locale string) ([]Description, error) {
	if err := s.authenticateContext(ctx); err != nil {
		return nil, err
	}
	definitions := s.registry.List()
	descriptions := make([]Description, 0, len(definitions))
	for _, definition := range definitions {
		descriptions = append(descriptions, s.describeDefinition(definition, locale))
	}
	if err := s.authenticateContext(ctx); err != nil {
		return nil, err
	}
	return descriptions, nil
}

// Describe autentica o escopo e exige o ID canônico exato.
func (s *Service) Describe(ctx context.Context, id, locale string) (Description, error) {
	if err := s.authenticateContext(ctx); err != nil {
		return Description{}, err
	}
	definition, ok := s.registry.Lookup(id)
	if !ok {
		return Description{}, ErrInvalidRequest
	}
	description := s.describeDefinition(definition, locale)
	if err := s.authenticateContext(ctx); err != nil {
		return Description{}, err
	}
	return description, nil
}

// Execute sempre cria um UUIDv7 novo na borda. RequestID fornecido pelo
// chamador é rejeitado: IDs reapresentados pertencem exclusivamente a Retry.
func (s *Service) Execute(ctx context.Context, request Request) (Result, error) {
	if request.RequestID != "" {
		return Result{}, ErrRequestIDOnExecute
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Result{}, err
	}
	result := Result{RequestID: id.String()}
	if err := s.authenticateContext(ctx); err != nil {
		return result, err
	}
	return s.execute(ctx, request.CommandID, request.Arguments, id.String(), result)
}

// Retry só aceita uma invocação existente e autenticada. O lookup ocorre
// antes de qualquer chamada de execução; o candidato usa o mesmo invocation
// ID e a correlação persistida, enquanto os argumentos fornecidos pelo caller
// entram novamente no fingerprint do executor.
func (s *Service) Retry(ctx context.Context, request Request) (Result, error) {
	result := Result{RequestID: request.RequestID}
	if !validUUIDv7(request.RequestID) {
		return result, ErrInvalidRequest
	}
	if err := s.authenticateContext(ctx); err != nil {
		return result, err
	}
	record, err := s.executor.GetEnvelopeInvocation(ctx, privateExecutorToken, request.RequestID)
	if err != nil {
		return result, err
	}
	if record.SourceType == nil || *record.SourceType != commandcontract.SourceCLI || record.Envelope.CommandID == nil || *record.Envelope.CommandID != request.CommandID {
		return result, ErrCommandMismatch
	}
	correlationID := record.Envelope.CorrelationID
	if !validUUIDv7(correlationID) {
		return result, ErrInvalidRequest
	}
	return s.executeWithCorrelation(ctx, request.CommandID, request.Arguments, request.RequestID, correlationID, result)
}

// Status consulta somente a projeção redigida do envelope autenticado. Não
// chama ExecuteEnvelope e nunca devolve payload vivo.
func (s *Service) Status(ctx context.Context, id string) (Result, error) {
	result := Result{RequestID: id}
	if !validUUIDv7(id) {
		return result, ErrInvalidRequest
	}
	if err := s.authenticateContext(ctx); err != nil {
		return result, err
	}
	record, err := s.executor.GetEnvelopeInvocation(ctx, privateExecutorToken, id)
	if err != nil {
		return result, err
	}
	return resultFromRecord(record, nil), nil
}

func (s *Service) execute(ctx context.Context, commandID string, arguments json.RawMessage, requestID string, result Result) (Result, error) {
	return s.executeWithCorrelation(ctx, commandID, arguments, requestID, requestID, result)
}

func (s *Service) executeWithCorrelation(ctx context.Context, commandID string, arguments json.RawMessage, requestID, correlationID string, result Result) (Result, error) {
	if s == nil || s.executor == nil || ctx == nil || commandID == "" || !validUUIDv7(requestID) || !validUUIDv7(correlationID) {
		return result, ErrInvalidRequest
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	record, output, err := s.executor.ExecuteEnvelopeWithResult(ctx, privateExecutorToken, commandexecution.EnvelopeCandidate{
		InvocationID:  requestID,
		CorrelationID: correlationID,
		CommandID:     commandID,
		Arguments:     append(json.RawMessage(nil), arguments...),
	})
	result = resultFromRecord(record, output)
	if result.RequestID == "" {
		result.RequestID = requestID
	}
	if statusErr := terminalStatusError(record.Status); statusErr != nil {
		return result, statusErr
	}
	return result, err
}

func terminalStatusError(status commandledger.Status) error {
	switch status {
	case commandledger.Denied, commandledger.RejectedStale, commandledger.Failed,
		commandledger.OutcomeUnknown, commandledger.TimedOut,
		commandledger.Cancelled, commandledger.CancelledStale:
		return &StatusError{Status: status}
	default:
		return nil
	}
}

func (s *Service) authenticateContext(ctx context.Context) error {
	if s == nil || s.registry == nil || s.executor == nil || s.authenticate == nil || ctx == nil {
		return ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.authenticate(ctx); err != nil {
		return err
	}
	return ctx.Err()
}

func (s *Service) describeDefinition(d commandcatalog.Definition, locale string) Description {
	name, description := localizedMetadata(d, locale)
	reason := UnavailableReason(d)
	return Description{
		ID:                d.ID,
		Name:              name,
		Description:       description,
		AllowedSources:    append([]commandcatalog.Source(nil), d.AllowedSources...),
		Executable:        reason == "",
		UnavailableReason: reason,
		ArgumentsSchema:   commandcatalog.JSONSchema(d.ArgumentsSchema),
	}
}

func localizedMetadata(d commandcatalog.Definition, locale string) (string, string) {
	if d.Presentation == nil || len(d.Presentation.Locales) == 0 {
		return "", ""
	}
	locales := []string{locale, "pt-BR", "en", "es"}
	seen := make(map[string]struct{}, len(locales))
	for _, candidate := range locales {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if metadata, ok := d.Presentation.Locales[candidate]; ok {
			return metadata.Name, metadata.Description
		}
	}
	return "", ""
}

func resultFromRecord(record commandledger.FullRecord, output json.RawMessage) Result {
	result := Result{
		RequestID:     record.InvocationID,
		Status:        string(record.Status),
		ResultSummary: cloneString(record.ResultSummary),
		ErrorCode:     cloneString(record.ErrorCode),
	}
	if record.Status == commandledger.Succeeded && len(output) != 0 {
		result.Output = append(json.RawMessage(nil), output...)
	}
	return result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func validUUIDv7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
