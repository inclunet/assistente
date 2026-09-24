package commandexecution

import (
	"context"
	"encoding/json"
	"errors"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandidentity"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

// ExternalService liga JWTs ao pipeline integral, sem sessão desktop sintética.
// O host ainda deve fornecer snapshot, resolução e autorização de recursos por
// usuário, além de publicar readiness explicitamente no autenticador.
type ExternalService struct {
	executor      *Service
	authenticator *auth.ExternalCommandAuthenticator
	identity      *commandidentity.Service
}

// A credencial existe somente no contexto da solicitação, nunca no serviço,
// envelope ou ledger. A chave privada impede substituição pelo transporte.
type externalCredentialKey struct{}
type externalCredential struct {
	service *ExternalService
	token   string
	userID  string
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
	s := &ExternalService{authenticator: authenticator, identity: policy}
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
	authorize := func(ctx context.Context, owner commandledger.FullOwnership, definition commandcatalog.Definition) error {
		bound, err := credential(ctx)
		if err != nil {
			return err
		}
		// Ainda não existe broker de interação externa. Sem aprovação implícita.
		if definition.Decision != commandcatalog.NoDecision || definition.HandlerClassification == commandcatalog.HandlerUI {
			return ErrDenied
		}
		req := commandidentity.ExternalTokenRequest{AccessToken: bound.token, Source: config.Source}
		p, err := policy.ResolveExternalTokenCached(ctx, req)
		if err != nil || p.UserID != bound.userID {
			return ErrDenied
		}
		expected := commandledger.FullOwnership{UserID: &p.UserID, AuthContextType: p.AuthContextType, AuthContextID: p.AuthContextID, ActorType: p.ActorType, ActorID: p.ActorID}
		if !sameEnvelopeOwner(owner, expected) {
			return ErrDenied
		}
		err = policy.Authorize(ctx, commandidentity.AuthorizationRequest{Identity: p, Definition: definition, ExternalToken: &req})
		if errors.Is(err, commandidentity.ErrCommandNotAuthorized) {
			return ErrDenied
		}
		return err
	}
	ports.Authorize = func(ctx context.Context, owner commandledger.FullOwnership, envelope commandcontract.Envelope, definition commandcatalog.Definition) error {
		if err := authorize(ctx, owner, definition); err != nil {
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
		if err := authorize(ctx, owner, definition); err != nil {
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

func (s *ExternalService) requestContext(ctx context.Context, token string) (context.Context, func(), error) {
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
	return context.WithValue(owned, externalCredentialKey{}, externalCredential{service: s, token: token, userID: p.UserID}), release, nil
}

func (s *ExternalService) ExecuteEnvelope(ctx context.Context, token string, candidate EnvelopeCandidate) (commandledger.FullRecord, error) {
	record, _, err := s.ExecuteEnvelopeWithResult(ctx, token, candidate)
	return record, err
}

func (s *ExternalService) ExecuteEnvelopeWithResult(ctx context.Context, token string, candidate EnvelopeCandidate) (commandledger.FullRecord, json.RawMessage, error) {
	bound, release, err := s.requestContext(ctx, token)
	if err != nil {
		return commandledger.FullRecord{}, nil, err
	}
	defer release()
	return s.executor.ExecuteEnvelopeWithResult(bound, token, candidate)
}

func (s *ExternalService) GetEnvelopeInvocation(ctx context.Context, token, id string) (commandledger.FullRecord, error) {
	bound, release, err := s.requestContext(ctx, token)
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
