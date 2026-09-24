package commandidentity

import (
	"context"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/database"
)

type Config struct {
	Sessions           *auth.SessionService
	External           *auth.ExternalCommandAuthenticator
	ExternalAdmin      *auth.ExternalIdentityAdminService
	Epochs             EpochPort
	JobRuntime         JobRuntime
	JobGrants          JobGrantStore
	AuthorizationRules []AuthorizationRule
}

type Service struct {
	sessions   *auth.SessionService
	external   *auth.ExternalCommandAuthenticator
	admin      *auth.ExternalIdentityAdminService
	epochs     EpochPort
	jobRuntime JobRuntime
	jobGrants  JobGrantStore
	rules      map[string]AuthorizationRule
}

func New(cfg Config) (*Service, error) {
	if cfg.Epochs == nil {
		return nil, ErrEpochUnavailable
	}
	if cfg.JobRuntime != nil && cfg.JobGrants == nil {
		return nil, ErrJobExecutionDenied
	}
	rules := make(map[string]AuthorizationRule, len(cfg.AuthorizationRules))
	for _, rule := range cfg.AuthorizationRules {
		if !commandIDValid(rule.CommandID) || len(rule.Actors) == 0 {
			return nil, ErrInvalidIdentityRequest
		}
		if _, exists := rules[rule.CommandID]; exists {
			return nil, ErrInvalidIdentityRequest
		}
		rule.RequiredRoles = clean(rule.RequiredRoles)
		rule.RequiredScopes = clean(rule.RequiredScopes)
		rule.Actors = append([]commandcontract.ActorType(nil), rule.Actors...)
		rules[rule.CommandID] = rule
	}
	return &Service{
		sessions: cfg.Sessions, external: cfg.External, admin: cfg.ExternalAdmin,
		epochs:     cfg.Epochs,
		jobRuntime: cfg.JobRuntime, jobGrants: cfg.JobGrants, rules: rules,
	}, nil
}

func commandIDValid(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for i, r := range part {
			if i == 0 && (r < 'a' || r > 'z') {
				return false
			}
			if i > 0 && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
	}
	return true
}

func (s *Service) ResolveLocalSession(ctx context.Context, req LocalSessionRequest) (TrustedIdentity, error) {
	return s.resolveLocalSession(ctx, req, true)
}

func (s *Service) resolveLocalSession(ctx context.Context, req LocalSessionRequest, capture bool) (TrustedIdentity, error) {
	if s == nil || s.sessions == nil || !localSource(req.Source) {
		return TrustedIdentity{}, ErrInvalidIdentityRequest
	}
	principal, err := s.sessions.AuthenticateLocalAccess(ctx, req.AccessToken)
	if err != nil {
		return TrustedIdentity{}, err
	}
	role, err := s.sessions.ActiveUserRole(ctx, principal.UserID)
	if err != nil {
		return TrustedIdentity{}, err
	}
	return s.localIdentity(ctx, principal, req.Source, role, capture)
}

func (s *Service) ResolveAgent(ctx context.Context, req LocalSessionRequest, source AgentSource) (TrustedIdentity, error) {
	return s.resolveAgent(ctx, req, source, true)
}

func (s *Service) resolveAgent(ctx context.Context, req LocalSessionRequest, source AgentSource, capture bool) (TrustedIdentity, error) {
	if s == nil || source == nil || !agentSource(req.Source) || s.sessions == nil {
		return TrustedIdentity{}, ErrInvalidIdentityRequest
	}
	principal, err := s.sessions.AuthenticateLocalAccess(ctx, req.AccessToken)
	if err != nil {
		return TrustedIdentity{}, err
	}
	role, err := s.sessions.ActiveUserRole(ctx, principal.UserID)
	if err != nil {
		return TrustedIdentity{}, err
	}
	identity, err := s.localIdentity(ctx, principal, req.Source, role, capture)
	if err != nil {
		return TrustedIdentity{}, err
	}
	agentID, err := source.ResolveAgent(ctx, principal)
	if err != nil || strings.TrimSpace(agentID) == "" {
		return TrustedIdentity{}, ErrInvalidIdentityRequest
	}
	identity.ActorType = commandcontract.ActorAgent
	identity.ActorID = strings.TrimSpace(agentID)
	return identity, nil
}

func (s *Service) localIdentity(ctx context.Context, principal auth.LocalSessionPrincipal, source commandcatalog.Source, role string, capture bool) (TrustedIdentity, error) {
	identity := TrustedIdentity{
		AuthContextType: commandcontract.AuthLocalSession, AuthContextID: principal.SessionID,
		UserID: principal.UserID, SessionID: principal.SessionID, ActorType: commandcontract.ActorUser,
		ActorID: principal.UserID, Source: source, Role: role,
	}
	if !capture {
		return identity, nil
	}
	epoch, err := s.capture(ctx, ContextPrincipal{UserID: principal.UserID, Type: string(commandcontract.AuthLocalSession), ID: principal.SessionID})
	if err != nil {
		return TrustedIdentity{}, err
	}
	identity.AuthGeneration, identity.SecurityGeneration = epoch.AuthGeneration, epoch.SecurityGeneration
	return identity, nil
}

func (s *Service) ResolveExternalToken(ctx context.Context, req ExternalTokenRequest) (TrustedIdentity, error) {
	if s == nil || s.external == nil || !externalSource(req.Source) {
		return TrustedIdentity{}, ErrExternalNotReady
	}
	return s.resolveExternal(ctx, req, false, true)
}

// ResolveExternalTokenCached é a porta usada pelo gate final. Ela nunca busca
// JWKS: se o cache verificado expirou, a admissão falha e o host recomeça fora
// do DispatchGate para atualizar a validação.
func (s *Service) ResolveExternalTokenCached(ctx context.Context, req ExternalTokenRequest) (TrustedIdentity, error) {
	if s == nil || s.external == nil || !externalSource(req.Source) {
		return TrustedIdentity{}, ErrExternalNotReady
	}
	return s.resolveExternal(ctx, req, true, false)
}

func (s *Service) resolveExternal(ctx context.Context, req ExternalTokenRequest, cached, capture bool) (TrustedIdentity, error) {
	var principal auth.ExternalCommandPrincipal
	var err error
	if cached {
		principal, err = s.external.AuthenticateCached(ctx, req.AccessToken)
	} else {
		principal, err = s.external.Authenticate(ctx, req.AccessToken)
	}
	if err != nil {
		return TrustedIdentity{}, err
	}
	var epoch Epoch
	if capture {
		if s.epochs == nil {
			return TrustedIdentity{}, ErrEpochUnavailable
		}
		// A validação anterior aquece JWKS fora do gate. Dentro dele, releia
		// token e vínculo sem rede para não capturar uma prova já revogada.
		epoch, err = s.epochs.CaptureContextAuthenticated(ctx, func() (ContextPrincipal, error) {
			principal, err = s.external.AuthenticateCached(ctx, req.AccessToken)
			if err != nil {
				return ContextPrincipal{}, err
			}
			return ContextPrincipal{UserID: principal.UserID, Type: string(commandcontract.AuthExternalToken), ID: principal.AuthContextID, GroupID: auth.ExternalIdentityContextID(principal.Issuer, principal.Subject)}, nil
		})
		if err != nil {
			return TrustedIdentity{}, err
		}
	}
	identity := TrustedIdentity{
		AuthContextType: commandcontract.AuthExternalToken, AuthContextID: principal.AuthContextID,
		UserID: principal.UserID, ActorType: commandcontract.ActorUser, ActorID: principal.UserID, Role: principal.Role,
		Source: req.Source, Scopes: append([]string(nil), principal.Scopes...), Roles: append([]string(nil), principal.Roles...),
	}
	if !capture {
		return identity, nil
	}
	identity.AuthGeneration, identity.SecurityGeneration = epoch.AuthGeneration, epoch.SecurityGeneration
	return identity, nil
}

func (s *Service) ResolveJobService(ctx context.Context, req JobServiceRequest) (TrustedIdentity, error) {
	return s.resolveJobService(ctx, req, true)
}

func (s *Service) resolveJobService(ctx context.Context, req JobServiceRequest, capture bool) (TrustedIdentity, error) {
	if s == nil || s.jobRuntime == nil || req.Capability == nil || !jobSource(req.Source) || req.JobDatabaseID == "" || req.TargetProfileSlug == "" || req.RunID == "" {
		return TrustedIdentity{}, ErrJobExecutionDenied
	}
	job, err := s.jobRuntime.ResolveCommandJob(ctx, req.Capability, req)
	if err != nil || job.OwnerUserID == "" || job.DatabaseID == "" || job.DatabaseID != req.JobDatabaseID || job.Slug == "" || job.TargetProfileSlug == "" || job.TargetProfileSlug != req.TargetProfileSlug || job.JobDefinitionFingerprint == "" || job.RunID == "" || job.RunID != req.RunID {
		return TrustedIdentity{}, ErrJobExecutionDenied
	}
	if err := s.validateJobGrant(ctx, job); err != nil {
		return TrustedIdentity{}, err
	}
	identity := TrustedIdentity{
		AuthContextType: commandcontract.AuthJobService, AuthContextID: "job-service:" + job.DatabaseID + ":" + job.RunID,
		UserID: job.OwnerUserID, ActorType: commandcontract.ActorAutomation, ActorID: job.Slug,
		Source: req.Source, Job: &job,
	}
	if !capture {
		return identity, nil
	}
	epoch, err := s.capture(ctx, ContextPrincipal{UserID: job.OwnerUserID, Type: string(commandcontract.AuthJobService), ID: job.DatabaseID})
	if err != nil {
		return TrustedIdentity{}, err
	}
	identity.AuthGeneration, identity.SecurityGeneration = epoch.AuthGeneration, epoch.SecurityGeneration
	return identity, nil
}

func (s *Service) ResolveSystem(ctx context.Context, req SystemRequest) (TrustedIdentity, error) {
	return s.resolveSystem(ctx, req, true)
}

func (s *Service) resolveSystem(ctx context.Context, req SystemRequest, capture bool) (TrustedIdentity, error) {
	if s == nil || ctx == nil || req.Capability == nil {
		return TrustedIdentity{}, ErrSystemCapability
	}
	if _, ok := req.Capability.(systemCapability); !ok {
		return TrustedIdentity{}, ErrSystemCapability
	}
	identity := TrustedIdentity{
		AuthContextType: commandcontract.AuthSystem,
		ActorType:       commandcontract.ActorAutomation, ActorID: "system", Source: commandcatalog.System,
	}
	if !capture {
		return identity, nil
	}
	epoch, err := s.capture(ctx, ContextPrincipal{Type: string(commandcontract.AuthSystem), ID: "system"})
	if err != nil {
		return TrustedIdentity{}, err
	}
	identity.AuthContextID = "system:" + epoch.SecurityGeneration
	identity.AuthGeneration, identity.SecurityGeneration = epoch.AuthGeneration, epoch.SecurityGeneration
	return identity, nil
}

// Authorize primeiro refaz a autenticação/resolução adequada à origem. A
// projeção recebida, inclusive Role=admin, nunca é fonte de decisão.
func (s *Service) Authorize(ctx context.Context, req AuthorizationRequest) error {
	if s == nil || ctx == nil {
		return ErrCommandNotAuthorized
	}
	if req.JobService != nil {
		if req.LocalSession != nil || req.ExternalToken != nil || req.System != nil || req.AgentSource != nil {
			return ErrCommandNotAuthorized
		}
		return s.AuthorizeJob(ctx, req)
	}
	var fresh TrustedIdentity
	var err error
	switch {
	case req.LocalSession != nil && req.ExternalToken == nil && req.JobService == nil && req.System == nil:
		fresh, err = s.resolveLocalSession(ctx, *req.LocalSession, false)
		if req.AgentSource != nil {
			fresh, err = s.resolveAgent(ctx, *req.LocalSession, req.AgentSource, false)
		}
	case req.ExternalToken != nil && req.LocalSession == nil && req.JobService == nil && req.System == nil && req.AgentSource == nil:
		fresh, err = s.ResolveExternalTokenCached(ctx, *req.ExternalToken)
	case req.System != nil && req.LocalSession == nil && req.ExternalToken == nil && req.JobService == nil:
		if req.AgentSource != nil {
			return ErrCommandNotAuthorized
		}
		fresh, err = s.resolveSystem(ctx, *req.System, false)
	default:
		return ErrCommandNotAuthorized
	}
	if err != nil || !sameProjection(req.Identity, fresh) {
		return ErrCommandNotAuthorized
	}
	return s.authorizeFresh(ctx, fresh, req.Definition)
}

func (s *Service) authorizeFresh(ctx context.Context, identity TrustedIdentity, definition commandcatalog.Definition) error {
	rule, ok := s.rules[definition.ID]
	if !ok || !containsActor(rule.Actors, identity.ActorType) || !definition.AllowsSource(identity.Source) {
		return ErrCommandNotAuthorized
	}
	if !identityHasRequiredRole(identity, rule.RequiredRoles) || !containsAll(identity.Scopes, rule.RequiredScopes) {
		return ErrCommandNotAuthorized
	}
	if identity.AuthContextType == commandcontract.AuthSystem {
		if !rule.AllowSystem || definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision || definition.HasMutableTarget || definition.MutatesEffectiveCapability || !definition.Context.None || definition.HandlerClassification != commandcatalog.HandlerInternal {
			return ErrCommandNotAuthorized
		}
	}
	return nil
}

func (s *Service) AuthorizeJob(ctx context.Context, req AuthorizationRequest) error {
	if req.JobService == nil || req.Identity.Job == nil || s == nil || s.jobRuntime == nil {
		return ErrCommandNotAuthorized
	}
	fresh, err := s.resolveJobService(ctx, *req.JobService, false)
	if err != nil || !sameProjection(req.Identity, fresh) {
		return ErrCommandNotAuthorized
	}
	if err := s.authorizeFreshWithoutJobRevalidate(fresh, req.Definition); err != nil {
		return err
	}
	if err := s.jobRuntime.RevalidateCommandJob(ctx, req.JobService.Capability, *fresh.Job); err != nil {
		return err
	}
	return s.validateJobGrant(ctx, *fresh.Job)
}

func (s *Service) validateJobGrant(ctx context.Context, job TrustedJob) error {
	if ctx == nil || s.jobGrants == nil || job.OwnerUserID == "" || job.DelegationFingerprint == "" {
		return ErrJobExecutionDenied
	}
	valid, err := s.jobGrants.HasValidGeneration(database.WithUserID(ctx, job.OwnerUserID), job.DatabaseID, job.TargetProfileSlug, job.DelegationFingerprint, job.GrantGeneration)
	if err != nil || !valid {
		return ErrJobExecutionDenied
	}
	return nil
}

func (s *Service) authorizeFreshWithoutJobRevalidate(identity TrustedIdentity, definition commandcatalog.Definition) error {
	rule, ok := s.rules[definition.ID]
	if !ok || !containsActor(rule.Actors, identity.ActorType) || !definition.AllowsSource(identity.Source) || !containsRole(identity.Role, rule.RequiredRoles) || !containsAll(identity.Scopes, rule.RequiredScopes) {
		return ErrCommandNotAuthorized
	}
	if identity.AuthContextType == commandcontract.AuthJobService && (identity.Job == nil || definition.Decision != commandcatalog.NoDecision) {
		return ErrJobExecutionDenied
	}
	return nil
}

func sameProjection(want, got TrustedIdentity) bool {
	if want.AuthContextType != got.AuthContextType || (got.AuthContextID != "" && want.AuthContextID != got.AuthContextID) || want.UserID != got.UserID || want.SessionID != got.SessionID || want.ActorType != got.ActorType || want.ActorID != got.ActorID || want.Source != got.Source {
		return false
	}
	// A final revalidation intentionally does not call CaptureContextAuthenticated
	// (the caller may already hold DispatchGate). Epoch equality is checked by
	// the executor's gate against the snapshot captured during preparation.
	if got.AuthGeneration != "" && want.AuthGeneration != got.AuthGeneration || got.SecurityGeneration != "" && want.SecurityGeneration != got.SecurityGeneration {
		return false
	}
	if len(want.Scopes) != len(got.Scopes) || len(want.Roles) != len(got.Roles) {
		return false
	}
	for i := range want.Scopes {
		if want.Scopes[i] != got.Scopes[i] {
			return false
		}
	}
	for i := range want.Roles {
		if want.Roles[i] != got.Roles[i] {
			return false
		}
	}
	if want.Role != got.Role {
		return false
	}
	if (want.Job == nil) != (got.Job == nil) {
		return false
	}
	if want.Job != nil && (*want.Job != *got.Job) {
		return false
	}
	return true
}

func (s *Service) RevokeExternal(ctx context.Context, adminToken, issuer, subject string) error {
	if s == nil || s.admin == nil || s.epochs == nil {
		return ErrEpochUnavailable
	}
	prepared, err := s.admin.PrepareRevoke(ctx, adminToken, issuer, subject)
	if err != nil {
		return err
	}
	if err := s.epochs.MutateContextGroup(ctx, ContextPrincipal{
		UserID:  prepared.UserID(),
		Type:    string(commandcontract.AuthExternalToken),
		GroupID: prepared.ContextID(),
	}, func() error {
		return s.admin.RevokePrepared(ctx, prepared)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) capture(ctx context.Context, principal ContextPrincipal) (Epoch, error) {
	if s == nil || s.epochs == nil {
		return Epoch{}, ErrEpochUnavailable
	}
	return s.epochs.CaptureContextAuthenticated(ctx, func() (ContextPrincipal, error) {
		return principal, nil
	})
}

func localSource(source commandcatalog.Source) bool {
	switch source {
	case commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck,
		commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat, commandcatalog.CLI:
		return true
	default:
		return false
	}
}
func agentSource(source commandcatalog.Source) bool { return source == commandcatalog.Chat }
func externalSource(source commandcatalog.Source) bool {
	return source == commandcatalog.Palette || source == commandcatalog.UI || source == commandcatalog.Chat
}
func jobSource(source commandcatalog.Source) bool { return source == commandcatalog.Event }
func containsActor(values []commandcontract.ActorType, value commandcontract.ActorType) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func containsRole(value string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, expected := range required {
		if value == expected {
			return true
		}
	}
	return false
}

// Roles externas vêm exclusivamente das claims revalidadas. A role local
// continua servindo às origens locais, nunca como fallback para JWT externo.
// RequiredRoles preserva a semântica de alternativas (qualquer uma).
func identityHasRequiredRole(identity TrustedIdentity, required []string) bool {
	if identity.AuthContextType != commandcontract.AuthExternalToken {
		return containsRole(identity.Role, required)
	}
	if len(required) == 0 {
		return true
	}
	for _, role := range identity.Roles {
		if containsRole(role, required) {
			return true
		}
	}
	return false
}
func containsAll(values, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, expected := range required {
		if !set[expected] {
			return false
		}
	}
	return true
}
func clean(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}
