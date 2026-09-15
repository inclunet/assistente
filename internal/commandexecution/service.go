package commandexecution

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

type Service struct {
	config    Config
	complete  bool
	lifecycle executionLifecycle
}

var commandIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
var keyVersionPattern = regexp.MustCompile(`^v[1-9][0-9]*$`)

func New(config Config) (*Service, error) {
	if config.Sessions == nil || config.Epochs == nil || config.Store == nil || config.Registry == nil ||
		config.Snapshot == nil || config.Authorize == nil || config.Keys == nil || config.Now == nil ||
		!nonblank(config.RegistryVersion) || !keyVersionPattern.MatchString(config.KeyVersion) ||
		config.Retention <= 0 || config.ExecutionTimeout <= 0 || config.FinalizationTimeout <= 0 {
		return nil, ErrInvalidConfiguration
	}
	if config.Source != commandcatalog.Palette && config.Source != commandcatalog.UI && config.Source != commandcatalog.CLI {
		return nil, ErrInvalidConfiguration
	}
	if len(config.Handlers) == 0 {
		return nil, ErrInvalidConfiguration
	}
	copyHandlers := make(map[string]Handler, len(config.Handlers))
	for id, handler := range config.Handlers {
		definition, err := config.Registry.CheckReadiness(id, config.Source)
		if err != nil || !definition.Context.None || len(definition.Context.Facts) != 0 || handler.Start == nil ||
			handler.Contract.Effect != definition.Effect || handler.Contract.MutatesEffectiveCapability != definition.MutatesEffectiveCapability {
			return nil, ErrInvalidConfiguration
		}
		copyHandlers[id] = handler
	}
	config.Handlers = copyHandlers
	return &Service{config: config}, nil
}

func nonblank(s string) bool { return s != "" && strings.TrimSpace(s) == s }
func validID(s string) bool {
	id, err := uuid.Parse(s)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == s
}

type prepared struct {
	request    commandledger.LocalReadRequest
	epoch      commandsecurity.EpochSnapshot
	versions   Versions
	invocation Invocation
}

// prepare nunca aceita identidade ou gerações do payload. A consulta de versões
// ocorre junto à autenticação sob o gate exclusivo. A chave vem apenas do host
// ou da versão de um ledger encontrado no mesmo escopo autenticado.
func (s *Service) prepare(ctx context.Context, token string, request Request) (prepared, error) {
	if !validID(request.InvocationID) || !validID(request.CorrelationID) || !commandIDPattern.MatchString(request.CommandID) {
		return prepared{}, ErrInvalidRequest
	}
	var versions Versions
	epoch, err := s.config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		principal, err := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return "", "", err
		}
		versions, err = s.config.Snapshot(ctx, principal)
		if err != nil {
			return "", "", ErrExecution
		}
		if !nonblank(versions.Registry) || !nonblank(versions.GlobalConfig) || !nonblank(versions.ActiveLayers) {
			return "", "", ErrExecution
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return prepared{}, err
	}
	// A projeção HMAC atual não representa argumentos, providers ou escrita.
	// Essas solicitações não entram neste executor restrito, nem por metadata.
	definition, err := s.config.Registry.CheckReadiness(request.CommandID, s.config.Source)
	if err != nil || !definition.Context.None || len(definition.Context.Facts) != 0 {
		return prepared{}, ErrDenied
	}
	if _, ok := s.config.Handlers[request.CommandID]; !ok {
		return prepared{}, ErrDenied
	}
	now := s.config.Now()
	if now.IsZero() {
		return prepared{}, ErrExecution
	}
	owner := commandledger.Owner{UserID: epoch.UserID, AuthContextID: epoch.SessionID}
	version := s.config.KeyVersion
	prior, err := s.config.Store.Get(ctx, owner, request.InvocationID)
	if err == nil {
		version = prior.RequestFingerprintVersion
	} else if !errors.Is(err, commandledger.ErrNotFound) {
		return prepared{}, err
	}
	unsigned := commandledger.LocalReadRequest{
		InvocationID: request.InvocationID, CorrelationID: request.CorrelationID, CommandID: request.CommandID,
		Owner: owner, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		RegistryVersion: versions.Registry, GlobalConfigGeneration: versions.GlobalConfig, ActiveLayersGeneration: versions.ActiveLayers,
		SourceType: string(s.config.Source), ReceivedAt: now, ExpiresAt: now.Add(s.config.Retention),
	}
	signed, err := commandledger.SignLocalRead(ctx, unsigned, version, s.config.Keys)
	if err != nil {
		return prepared{}, err
	}
	return prepared{request: signed, epoch: epoch, versions: versions, invocation: Invocation{
		ID: request.InvocationID, CorrelationID: request.CorrelationID, CommandID: request.CommandID,
		Principal: auth.LocalSessionPrincipal{UserID: epoch.UserID, SessionID: epoch.SessionID}, Source: s.config.Source,
	}}, nil
}

// check é chamado em CADA gate, inclusive replay. Não usa role do JWT nem
// AllowsSource como autorização; Authorize é obrigatório e vem do host.
func (s *Service) check(ctx context.Context, token string, p prepared) error {
	principal, err := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ErrStale
	}
	if principal != p.invocation.Principal {
		return ErrStale
	}
	versions, err := s.config.Snapshot(ctx, principal)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ErrExecution
	}
	if !versions.Unlocked || versions != p.versions || versions.Registry != s.config.RegistryVersion {
		return ErrStale
	}
	err = s.config.Authorize(ctx, principal, p.invocation.CommandID, s.config.Source)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return ErrDenied
	}
	return ctx.Err()
}

// Execute é síncrono para o chamador, mas espera o handle FORA do DispatchGate.
// Os dois CAS representam admissão e retirada imediata da fila lógica; não há
// scheduler compartilhado nem bootstrap de produto neste recorte. Reentregas
// nunca assumem propriedade de invocações já reservadas, mesmo não terminais.
func (s *Service) Execute(ctx context.Context, token string, request Request) (record commandledger.Record, err error) {
	if s == nil || ctx == nil || s.complete {
		return commandledger.Record{}, ErrInvalidRequest
	}
	ctx, release, err := s.lifecycle.enter(ctx)
	if err != nil {
		return commandledger.Record{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, s.config.ExecutionTimeout)
	defer cancel()
	// Antes da reserva, não há invocação confiável para concluir.
	var p prepared
	if err = protect(func() error { var e error; p, e = s.prepare(ctx, token, request); return e }); err != nil {
		return commandledger.Record{}, err
	}
	reservation, err := s.config.Store.Reserve(ctx, p.request)
	if err != nil {
		return commandledger.Record{}, err
	}
	if !reservation.Created {
		err = protect(func() error {
			return s.config.Epochs.Admit(ctx, p.epoch, func(ctx context.Context) error { return s.check(ctx, token, p) }, func() error {
				var e error
				record, e = s.config.Store.Get(ctx, p.request.Owner, p.request.InvocationID)
				return e
			})
		})
		if err != nil {
			return commandledger.Record{}, err
		}
		return record, nil
	}
	state := commandledger.Evaluating
	enteredStart := false
	// Recuperação cobre callbacks do host. Nunca declara failed se Start entrou.
	defer func() {
		if recover() != nil {
			to := commandledger.Failed
			if enteredStart {
				to = commandledger.OutcomeUnknown
			}
			record, err = s.finish(p, state, to)
		}
	}()
	var releaseExecution func()
	defer func() {
		if releaseExecution != nil {
			releaseExecution()
		}
	}()
	admit := func(from, to commandledger.Status, handoff func(context.Context)) error {
		check := func(ctx context.Context) error { return s.check(ctx, token, p) }
		commit := func(runCtx context.Context) error {
			changed, err := s.config.Store.CompareAndSwap(ctx, p.request.Owner, p.request.InvocationID, from, to)
			if err != nil {
				return err
			}
			if !changed {
				return commandledger.ErrConflict
			}
			state = to
			if handoff != nil {
				handoff(runCtx)
			}
			return nil
		}
		if handoff == nil {
			return s.config.Epochs.Admit(ctx, p.epoch, check, func() error { return commit(ctx) })
		}
		var err error
		releaseExecution, err = s.config.Epochs.AdmitExecution(ctx, p.epoch, check, commit)
		return err
	}
	if err := admit(commandledger.Evaluating, commandledger.Queued, nil); err != nil {
		return s.failedAdmission(p, state, err)
	}
	var handle ExecutionHandle
	var startErr error
	var executionCtx context.Context
	if err := admit(commandledger.Queued, commandledger.Running, func(runCtx context.Context) {
		executionCtx = runCtx
		startErr = s.lifecycle.handoff(runCtx, func() error {
			enteredStart = true
			var err error
			handle, err = s.config.Handlers[p.invocation.CommandID].Start(runCtx, p.invocation)
			return err
		})
	}); err != nil {
		return s.failedAdmission(p, state, err)
	}
	if startErr != nil {
		safeCancel(handle.Cancel)
		return s.finish(p, state, commandledger.OutcomeUnknown)
	}
	return s.finish(p, state, awaitOutcome(executionCtx, handle))
}

func protect(fn func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrExecution
		}
	}()
	return fn()
}

func (s *Service) failedAdmission(p prepared, from commandledger.Status, cause error) (commandledger.Record, error) {
	to := commandledger.CancelledStale
	switch {
	case errors.Is(cause, context.DeadlineExceeded):
		to = commandledger.TimedOut
	case errors.Is(cause, context.Canceled):
		to = commandledger.Cancelled
	case errors.Is(cause, ErrDenied) && from == commandledger.Evaluating:
		to = commandledger.Denied
	case errors.Is(cause, ErrExecution):
		to = commandledger.Failed
	case errors.Is(cause, ErrStale), errors.Is(cause, ErrDenied), errors.Is(cause, commandsecurity.ErrStaleEpoch):
	default:
		// Erro de persistência/CAS: não presume que pode começar ou recuperar.
		return commandledger.Record{}, cause
	}
	return s.finish(p, from, to)
}

// A persistência terminal usa prazo próprio: cancelar o caller não pode impedir
// registrar outcome_unknown. Erros de commit são devolvidos; nunca há retry do
// handler. CAS perdido retorna o vencedor persistido sem sobrescrever terminal.
func (s *Service) finish(p prepared, from, to commandledger.Status) (commandledger.Record, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.FinalizationTimeout)
	defer cancel()
	_, err := s.config.Store.CompareAndSwap(ctx, p.request.Owner, p.request.InvocationID, from, to)
	if err != nil {
		return commandledger.Record{}, err
	}
	return s.config.Store.Get(ctx, p.request.Owner, p.request.InvocationID)
}

// GetInvocation consulta sem reservar nem executar. O recorte atual exige o
// request original e fingerprint ainda reproduzível nas gerações atuais; após
// mudança de configuração/segurança falha fechado como conflito. Consulta por
// ID independente de gerações e resultados redigidos ricos é evolução futura.
func (s *Service) GetInvocation(ctx context.Context, token string, request Request) (record commandledger.Record, err error) {
	if s == nil || ctx == nil || s.complete {
		return commandledger.Record{}, ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, s.config.ExecutionTimeout)
	defer cancel()
	err = protect(func() error {
		p, err := s.prepare(ctx, token, request)
		if err != nil {
			return err
		}
		return s.config.Epochs.Admit(ctx, p.epoch, func(ctx context.Context) error { return s.check(ctx, token, p) }, func() error {
			found, err := s.config.Store.Get(ctx, p.request.Owner, request.InvocationID)
			if err != nil {
				return err
			}
			if found.SourceType != p.request.SourceType || found.RequestFingerprintVersion != p.request.RequestFingerprintVersion || found.RequestFingerprint != p.request.RequestFingerprint {
				return commandledger.ErrConflict
			}
			record = found
			return nil
		})
	})
	if err != nil {
		return commandledger.Record{}, err
	}
	return record, nil
}
