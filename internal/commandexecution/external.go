package commandexecution

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandidentity"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

func externalCommandMayRunWithoutUI(definition commandcatalog.Definition) bool {
	backend := definition.HandlerClassification == commandcatalog.HandlerInternal || definition.HandlerClassification == commandcatalog.HandlerBackend
	return backend && definition.Effect == commandcatalog.Read && definition.Decision == commandcatalog.NoDecision &&
		definition.Context.None && !definition.HasMutableTarget && !definition.MutatesEffectiveCapability
}

// ExternalService liga JWTs ao pipeline integral, sem sessão desktop sintética.
// O host ainda deve fornecer snapshot, resolução e autorização de recursos por
// usuário, além de publicar readiness explicitamente no autenticador.
type ExternalService struct {
	executor      *Service
	authenticator *auth.ExternalCommandAuthenticator
	identity      *commandidentity.Service
	admin         *auth.ExternalIdentityAdminService
}

// Source é a origem fixada pelo bootstrap, nunca pelo corpo HTTP.
func (s *ExternalService) Source() commandcatalog.Source {
	if s == nil || s.executor == nil {
		return ""
	}
	return s.executor.config.Source
}

// SharesSecurityWith permite validar a composição de múltiplas origens antes
// de publicá-las: revogação deve invalidar todas sob a mesma autoridade/gate.
func (s *ExternalService) SharesSecurityWith(other *ExternalService) bool {
	return s != nil && other != nil && s.executor != nil && other.executor != nil &&
		s.executor.config.Epochs == other.executor.config.Epochs &&
		s.authenticator == other.authenticator && s.admin == other.admin
}

// A credencial existe somente no contexto da solicitação, nunca no serviço,
// envelope ou ledger. A chave privada impede substituição pelo transporte.
type externalCredentialKey struct{}
type externalCredential struct {
	service   *ExternalService
	token     string
	userID    string
	principal ExternalUIPrincipal
	binding   *ExternalUIBinding
}

func (externalCredential) String() string { return "external credential [redacted]" }

// NewExternal substitui Authenticate por JWT validado e compõe a política de
// roles/scopes com as autorizações de recursos do host. Não publica readiness
// nem instala rotas. Nenhum callback de autenticação local é reaproveitado.
func NewExternal(config Config, authenticator *auth.ExternalCommandAuthenticator, admin *auth.ExternalIdentityAdminService, rules []commandidentity.AuthorizationRule) (*ExternalService, error) {
	if authenticator == nil || admin == nil || config.Envelope == nil || config.Envelope.Identity == nil || config.Epochs == nil {
		return nil, ErrInvalidConfiguration
	}
	switch config.Source {
	case commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat:
	default:
		return nil, ErrInvalidConfiguration
	}
	host := *config.Envelope.Identity
	if host.Snapshot == nil || host.Resolve == nil || host.Authorize == nil || host.AuthorizeLookup == nil {
		return nil, ErrInvalidConfiguration
	}
	epochs, err := commandidentity.NewCoreEpochs(config.Epochs)
	if err != nil {
		return nil, err
	}
	policy, err := commandidentity.New(commandidentity.Config{External: authenticator, ExternalAdmin: admin, Epochs: epochs, AuthorizationRules: rules})
	if err != nil {
		return nil, err
	}
	s := &ExternalService{authenticator: authenticator, identity: policy, admin: admin}
	credential := func(ctx context.Context) (externalCredential, error) {
		if ctx == nil {
			return externalCredential{}, ErrDenied
		}
		v, ok := ctx.Value(externalCredentialKey{}).(externalCredential)
		if !ok || v.service != s || v.token == "" {
			return externalCredential{}, ErrDenied
		}
		return v, nil
	}
	ports := host
	ports.Authenticate = func(ctx context.Context, token string) (EnvelopeAuthenticatedIdentity, error) {
		bound, err := credential(ctx)
		if err != nil || bound.token != token {
			return EnvelopeAuthenticatedIdentity{}, ErrDenied
		}
		p, err := authenticator.AuthenticateCached(ctx, token)
		if err != nil || p.UserID != bound.userID {
			return EnvelopeAuthenticatedIdentity{}, ErrDenied
		}
		return EnvelopeAuthenticatedIdentity{
			Ownership:        commandledger.FullOwnership{UserID: &p.UserID, AuthContextType: commandcontract.AuthExternalToken, AuthContextID: p.AuthContextID, ActorType: commandcontract.ActorUser, ActorID: p.UserID},
			ContextPrincipal: commandsecurity.ContextPrincipal{UserID: p.UserID, Type: string(commandcontract.AuthExternalToken), ID: p.AuthContextID, GroupID: auth.ExternalIdentityContextID(p.Issuer, p.Subject)},
		}, nil
	}
	authorize := func(ctx context.Context, owner commandledger.FullOwnership, definition commandcatalog.Definition, requireLiveUI bool) error {
		bound, err := credential(ctx)
		if err != nil {
			return err
		}
		needsUI := definition.Decision != commandcatalog.NoDecision || definition.HandlerClassification == commandcatalog.HandlerUI
		if requireLiveUI && needsUI && bound.binding == nil {
			return ErrDenied
		}
		if requireLiveUI && bound.binding == nil && !externalCommandMayRunWithoutUI(definition) {
			return ErrDenied
		}
		if requireLiveUI && bound.binding != nil {
			if config.ExternalUI == nil || config.ExternalUI.Validate == nil {
				return ErrDenied
			}
			uiPrincipal := ExternalUIPrincipal{Issuer: bound.principal.Issuer, Subject: bound.principal.Subject, UserID: bound.principal.UserID, AuthContextID: bound.principal.AuthContextID}
			if err := config.ExternalUI.Validate(ctx, uiPrincipal, *bound.binding); err != nil {
				return ErrDenied
			}
		}
		req := commandidentity.ExternalTokenRequest{AccessToken: bound.token, Source: config.Source}
		p, err := policy.ResolveExternalTokenCached(ctx, req)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || p.UserID != bound.userID {
			return ErrDenied
		}
		expected := commandledger.FullOwnership{UserID: &p.UserID, AuthContextType: p.AuthContextType, AuthContextID: p.AuthContextID, ActorType: p.ActorType, ActorID: p.ActorID}
		if !sameEnvelopeOwner(owner, expected) {
			return ErrDenied
		}
		err = policy.Authorize(ctx, commandidentity.AuthorizationRequest{Identity: p, Definition: definition, ExternalToken: &req})
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, commandidentity.ErrCommandNotAuthorized) {
			return ErrDenied
		}
		return err
	}
	ports.Authorize = func(ctx context.Context, owner commandledger.FullOwnership, envelope commandcontract.Envelope, definition commandcatalog.Definition) error {
		if err := authorize(ctx, owner, definition, true); err != nil {
			return err
		}
		return host.Authorize(ctx, owner, envelope, definition)
	}
	ports.AuthorizeLookup = func(ctx context.Context, owner commandledger.FullOwnership, record commandledger.FullRecord) error {
		if record.Envelope.CommandID == nil || record.SourceType == nil || string(*record.SourceType) != string(config.Source) {
			return ErrDenied
		}
		definition, ok := config.Registry.Lookup(*record.Envelope.CommandID)
		if !ok {
			return ErrDenied
		}
		if err := authorize(ctx, owner, definition, false); err != nil {
			return err
		}
		return host.AuthorizeLookup(ctx, owner, record)
	}
	envelope := *config.Envelope
	envelope.Identity = &ports
	config.Envelope = &envelope
	s.executor, err = NewComplete(config)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ExternalService) requestContext(ctx context.Context, token string, binding *ExternalUIBinding) (context.Context, func(), error) {
	if s == nil || s.executor == nil || ctx == nil {
		return nil, nil, ErrInvalidRequest
	}
	operation, release, err := s.executor.lifecycle.enter(ctx)
	if err != nil {
		return nil, nil, err
	}
	// Único ponto que pode buscar JWKS: antes de entrar no DispatchGate.
	// Participa do lifecycle para que shutdown também cancele esta preparação.
	authCtx, cancel := context.WithTimeout(operation, s.executor.config.ExecutionTimeout)
	defer cancel()
	var p auth.ExternalCommandPrincipal
	err = protect(func() error {
		var authErr error
		p, authErr = s.authenticator.Authenticate(authCtx, token)
		return authErr
	})
	if err != nil {
		release()
		return nil, nil, ErrDenied
	}
	owned := database.WithUserID(operation, p.UserID)
	var copiedBinding *ExternalUIBinding
	if binding != nil {
		copy := *binding
		copiedBinding = &copy
	}
	principal := ExternalUIPrincipal{Issuer: p.Issuer, Subject: p.Subject, UserID: p.UserID, AuthContextID: p.AuthContextID}
	return context.WithValue(owned, externalCredentialKey{}, externalCredential{service: s, token: token, userID: p.UserID, principal: principal, binding: copiedBinding}), release, nil
}

// ExternalUIBindingFromContext expõe apenas os stamps efêmeros da requisição
// já autenticada ao bootstrap; o caller não consegue instalar esse valor.
func ExternalUIBindingFromContext(ctx context.Context) (ExternalUIPrincipal, ExternalUIBinding, bool) {
	if ctx == nil {
		return ExternalUIPrincipal{}, ExternalUIBinding{}, false
	}
	value, ok := ctx.Value(externalCredentialKey{}).(externalCredential)
	if !ok || value.binding == nil {
		return ExternalUIPrincipal{}, ExternalUIBinding{}, false
	}
	return value.principal, *value.binding, true
}

// WithAuthenticatedPrincipal mantém a operação de ingresso sob o mesmo gate
// de epochs usado por revogação. O callback deve ser curto, local e não pode
// aguardar UI/rede nem reentrar no executor; isso fecha a corrida entre validar
// o mapeamento JWT e consumir/criar um grant de conexão.
func (s *ExternalService) WithAuthenticatedPrincipal(ctx context.Context, token string, action func(context.Context, auth.ExternalCommandPrincipal) error) error {
	if s == nil || s.executor == nil || s.authenticator == nil || ctx == nil || action == nil || token == "" {
		return ErrInvalidRequest
	}
	operation, release, err := s.executor.lifecycle.enter(ctx)
	if err != nil {
		return err
	}
	defer release()
	bounded, cancel := context.WithTimeout(operation, s.executor.config.ExecutionTimeout)
	defer cancel()
	// A primeira validação pode precisar buscar JWKS. Faça a rede antes do
	// gate; só a revalidação pelo cache fica dentro da captura autenticada.
	principal, err := s.authenticator.Authenticate(bounded, token)
	if err != nil {
		return ErrDenied
	}
	var contextPrincipal commandsecurity.ContextPrincipal
	snapshot, err := s.executor.config.Epochs.CaptureContextAuthenticated(bounded, func(check context.Context) (commandsecurity.ContextPrincipal, error) {
		current, authErr := s.authenticator.AuthenticateCached(check, token)
		if authErr != nil || !sameExternalCommandPrincipal(current, principal) {
			return commandsecurity.ContextPrincipal{}, ErrDenied
		}
		contextPrincipal = commandsecurity.ContextPrincipal{UserID: principal.UserID, Type: string(commandcontract.AuthExternalToken), ID: principal.AuthContextID, GroupID: auth.ExternalIdentityContextID(principal.Issuer, principal.Subject)}
		return contextPrincipal, nil
	})
	if err != nil {
		return ErrDenied
	}
	return s.executor.config.Epochs.Admit(bounded, snapshot, func(check context.Context) error {
		current, authErr := s.authenticator.AuthenticateCached(check, token)
		if authErr != nil || !sameExternalCommandPrincipal(current, principal) {
			return ErrDenied
		}
		return nil
	}, func() error {
		if bounded.Err() != nil {
			return bounded.Err()
		}
		return action(bounded, principal)
	})
}

func sameExternalCommandPrincipal(left, right auth.ExternalCommandPrincipal) bool {
	return left.Issuer == right.Issuer && left.Subject == right.Subject && left.UserID == right.UserID &&
		left.AuthContextID == right.AuthContextID && left.Role == right.Role &&
		slices.Equal(left.Scopes, right.Scopes) && slices.Equal(left.Roles, right.Roles)
}

func (s *ExternalService) ExecuteEnvelope(ctx context.Context, token string, candidate EnvelopeCandidate) (commandledger.FullRecord, error) {
	record, _, err := s.ExecuteEnvelopeWithResult(ctx, token, candidate)
	return record, err
}

func (s *ExternalService) ExecuteEnvelopeWithResult(ctx context.Context, token string, candidate EnvelopeCandidate) (commandledger.FullRecord, json.RawMessage, error) {
	return s.executeEnvelopeWithResult(ctx, token, candidate, nil)
}

// ExecuteEnvelopeWithExternalUI mantém os stamps no contexto privado do
// executor. Eles também entram no fingerprint de ingresso para impedir que um
// replay transfira a mesma invocação para outro destino/contexto.
func (s *ExternalService) ExecuteEnvelopeWithExternalUI(ctx context.Context, token string, candidate EnvelopeCandidate, binding ExternalUIBinding) (commandledger.FullRecord, json.RawMessage, error) {
	return s.executeEnvelopeWithResult(ctx, token, candidate, &binding)
}

func (s *ExternalService) executeEnvelopeWithResult(ctx context.Context, token string, candidate EnvelopeCandidate, binding *ExternalUIBinding) (commandledger.FullRecord, json.RawMessage, error) {
	bound, release, err := s.requestContext(ctx, token, binding)
	if err != nil {
		return commandledger.FullRecord{}, nil, err
	}
	defer release()
	return s.executor.ExecuteEnvelopeWithResult(bound, token, candidate)
}

func (s *ExternalService) GetEnvelopeInvocation(ctx context.Context, token, id string) (commandledger.FullRecord, error) {
	bound, release, err := s.requestContext(ctx, token, nil)
	if err != nil {
		return commandledger.FullRecord{}, err
	}
	defer release()
	return s.executor.GetEnvelopeInvocation(bound, token, id)
}

func (s *ExternalService) RevokeIdentity(ctx context.Context, adminToken, issuer, subject string) error {
	if s == nil || s.identity == nil || s.executor == nil || ctx == nil {
		return ErrInvalidConfiguration
	}
	operation, release, err := s.executor.lifecycle.enter(ctx)
	if err != nil {
		return err
	}
	defer release()
	bounded, cancel := context.WithTimeout(operation, s.executor.config.ExecutionTimeout)
	defer cancel()
	return protect(func() error { return s.identity.RevokeExternal(bounded, adminToken, issuer, subject) })
}

func (s *ExternalService) Shutdown(ctx context.Context) error {
	if s == nil || s.executor == nil {
		return ErrInvalidConfiguration
	}
	return s.executor.Shutdown(ctx)
}
