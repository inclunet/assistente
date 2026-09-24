package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrExternalCommandAuthenticator = errors.New("autenticador externo de comandos não inicializado")
	ErrExternalAdminScopeRequired   = errors.New("escopo administrativo externo obrigatório")
	ErrExternalIssuerRequired       = errors.New("issuer externo configurado obrigatório")
)

// ExternalTokenVerifier é a porta mínima para o validador JWKS real ou para
// um validador temporário de teste. Claims já validados são a única fonte de
// issuer/subject/scopes; o chamador não pode fabricá-los no request.
type ExternalTokenVerifier interface {
	Validate(context.Context, string) (*ExternalClaims, error)
}

type CachedExternalTokenVerifier interface {
	ValidateCached(context.Context, string) (*ExternalClaims, error)
}

// ExternalCommandPrincipal é uma identidade externa depois de JWT, JWKS,
// issuer/subject e usuário local terem sido revalidados. O token bruto não
// atravessa esta porta.
type ExternalCommandPrincipal struct {
	Issuer        string
	Subject       string
	UserID        string
	Role          string
	AuthContextID string
	Scopes        []string
	Roles         []string
}

type ExternalCommandAuthenticator struct {
	verifier  ExternalTokenVerifier
	mappings  *ExternalIdentityRepository
	readiness func(context.Context) error
}

func NewExternalCommandAuthenticator(verifier ExternalTokenVerifier, mappings *ExternalIdentityRepository) *ExternalCommandAuthenticator {
	return &ExternalCommandAuthenticator{verifier: verifier, mappings: mappings}
}

// SetReadiness instala a flag publicada pelo host depois da migração, do lote
// administrativo e da adoção pelo middleware. Sem essa flag explícita, o
// autenticador permanece desabilitado mesmo que a tabela exista.
func (a *ExternalCommandAuthenticator) SetReadiness(check func(context.Context) error) {
	if a != nil {
		a.readiness = check
	}
}

func (a *ExternalCommandAuthenticator) Authenticate(ctx context.Context, token string) (ExternalCommandPrincipal, error) {
	return a.authenticate(ctx, token, false)
}

func (a *ExternalCommandAuthenticator) AuthenticateCached(ctx context.Context, token string) (ExternalCommandPrincipal, error) {
	return a.authenticate(ctx, token, true)
}

func (a *ExternalCommandAuthenticator) authenticate(ctx context.Context, token string, cached bool) (ExternalCommandPrincipal, error) {
	if a == nil || a.verifier == nil || a.mappings == nil || ctx == nil {
		return ExternalCommandPrincipal{}, ErrExternalCommandAuthenticator
	}
	if a.readiness == nil {
		return ExternalCommandPrincipal{}, ErrExternalIdentityNotReady
	}
	if err := ctx.Err(); err != nil {
		return ExternalCommandPrincipal{}, err
	}
	if a.readiness != nil {
		if err := a.readiness(ctx); err != nil {
			return ExternalCommandPrincipal{}, err
		}
	}
	if strings.TrimSpace(token) == "" {
		return ExternalCommandPrincipal{}, ErrUnauthenticatedExternalCommand
	}
	var claims *ExternalClaims
	var err error
	if cached {
		verifier, ok := a.verifier.(CachedExternalTokenVerifier)
		if !ok {
			return ExternalCommandPrincipal{}, ErrExternalJWKSCacheUnavailable
		}
		claims, err = verifier.ValidateCached(ctx, token)
	} else {
		claims, err = a.verifier.Validate(ctx, token)
	}
	if err != nil || claims == nil {
		return ExternalCommandPrincipal{}, ErrUnauthenticatedExternalCommand
	}
	mapping, err := a.mappings.Resolve(ctx, claims.Issuer, claims.Subject)
	if err != nil {
		return ExternalCommandPrincipal{}, ErrUnauthenticatedExternalCommand
	}
	role, err := a.mappings.ActiveUserRole(ctx, mapping.UserID)
	if err != nil {
		return ExternalCommandPrincipal{}, ErrUnauthenticatedExternalCommand
	}
	return ExternalCommandPrincipal{
		Issuer:        mapping.Issuer,
		Subject:       mapping.Subject,
		UserID:        mapping.UserID,
		Role:          role,
		AuthContextID: ExternalTokenContextID(mapping.Issuer, mapping.Subject, token),
		Scopes:        splitExternalScopes(claims.Scope),
		Roles:         append([]string(nil), claims.Roles...),
	}, nil
}

var ErrUnauthenticatedExternalCommand = errors.New("token externo não autenticado para comandos")

func splitExternalScopes(value string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, scope := range strings.Fields(value) {
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result
}

type ExternalIdentityAdminConfig struct {
	Issuer      string
	AdminScopes []string
	AdminRoles  []string
}

// ExternalIdentityAdminService concentra operações administrativas e exige
// que o token seja validado pelo IdP antes de criar/revogar qualquer vínculo.
type ExternalIdentityAdminService struct {
	verifier ExternalTokenVerifier
	repo     *ExternalIdentityRepository
	cfg      ExternalIdentityAdminConfig
}

// ExternalIdentityRevocation é uma decisão administrativa já autenticada,
// preparada fora do gate. Seus dados não são configuráveis por consumidores
// externos; RevokePrepared só aceita valores emitidos por PrepareRevoke.
type ExternalIdentityRevocation struct {
	issuer  string
	subject string
	userID  string
}

func (r ExternalIdentityRevocation) UserID() string { return r.userID }

func (r ExternalIdentityRevocation) ContextID() string {
	return ExternalIdentityContextID(r.issuer, r.subject)
}

// ExternalIdentityContextID usa framing estrutural para que issuer e subject
// não possam colidir por separadores presentes no conteúdo. Esse helper é a
// única codificação usada tanto na captura quanto na revogação do contexto.
func ExternalIdentityContextID(issuer, subject string) string {
	if !validExternalIdentityPart(issuer) || !validExternalIdentityPart(subject) {
		return ""
	}
	value, _ := json.Marshal([]string{issuer, subject})
	return string(value)
}

// ExternalTokenContextID identifica uma credencial externa sem persistir o
// JWT. O par issuer/subject e o fingerprint são serializados como uma tupla
// JSON para manter o enquadramento inequívoco mesmo com separadores nos campos.
func ExternalTokenContextID(issuer, subject, token string) string {
	if !validExternalIdentityPart(issuer) || !validExternalIdentityPart(subject) || token == "" {
		return ""
	}
	fingerprint := sha256.Sum256([]byte(token))
	value, _ := json.Marshal([]string{issuer, subject, hex.EncodeToString(fingerprint[:])})
	return string(value)
}

func NewExternalIdentityAdminService(verifier ExternalTokenVerifier, repo *ExternalIdentityRepository, cfg ExternalIdentityAdminConfig) (*ExternalIdentityAdminService, error) {
	if repo == nil || repo.db == nil {
		return nil, ErrExternalIdentityNotReady
	}
	if !validExternalIdentityPart(cfg.Issuer) {
		return nil, ErrExternalIssuerRequired
	}
	if verifier == nil || (len(cleanValues(cfg.AdminScopes)) == 0 && len(cleanValues(cfg.AdminRoles)) == 0) {
		return nil, ErrExternalAdministratorRequired
	}
	cfg.AdminScopes = cleanValues(cfg.AdminScopes)
	cfg.AdminRoles = cleanValues(cfg.AdminRoles)
	return &ExternalIdentityAdminService{verifier: verifier, repo: repo, cfg: cfg}, nil
}

func (s *ExternalIdentityAdminService) authorize(ctx context.Context, token string) (*ExternalClaims, error) {
	if s == nil || s.verifier == nil || s.repo == nil || strings.TrimSpace(token) == "" {
		return nil, ErrExternalAdministratorRequired
	}
	claims, err := s.verifier.Validate(ctx, token)
	if err != nil || claims == nil {
		return nil, ErrExternalAdministratorRequired
	}
	if !validExternalIdentityPart(s.cfg.Issuer) || claims.Issuer != s.cfg.Issuer {
		return nil, ErrExternalAdministratorRequired
	}
	if !containsAll(splitExternalScopes(claims.Scope), s.cfg.AdminScopes) && !containsAny(claims.Roles, s.cfg.AdminRoles) {
		return nil, ErrExternalAdminScopeRequired
	}
	return claims, nil
}

func (s *ExternalIdentityAdminService) Create(ctx context.Context, adminToken string, params ExternalIdentityMappingParams) (*ExternalIdentityMapping, error) {
	claims, err := s.authorize(ctx, adminToken)
	if err != nil {
		return nil, err
	}
	issuer, _, _, err := normalizeExternalMapping(params)
	if err != nil {
		return nil, err
	}
	if issuer != claims.Issuer {
		return nil, ErrExternalAdministratorRequired
	}
	return s.repo.Create(ctx, params)
}

// PrepareRevoke valida o administrador e relê o vínculo antes do gate. A
// aplicação da revogação deve ocorrer depois via RevokePrepared como uma das
// mutações agrupadas em MutateContextGroup no serviço de epochs.
// Consumidores de comandos devem usar commandidentity.Service.RevokeExternal;
// não há atalho Prepare+Apply que dispense a invalidação das execuções em voo.
func (s *ExternalIdentityAdminService) PrepareRevoke(ctx context.Context, adminToken, issuer, subject string) (ExternalIdentityRevocation, error) {
	claims, err := s.authorize(ctx, adminToken)
	if err != nil {
		return ExternalIdentityRevocation{}, err
	}
	// Um administrador do issuer configurado não administra vínculos de outro
	// provedor, mesmo que compartilhem o mesmo subject ou escopo administrativo.
	if claims.Issuer != s.cfg.Issuer || issuer != s.cfg.Issuer {
		return ExternalIdentityRevocation{}, ErrExternalAdministratorRequired
	}
	mapping, err := s.repo.Resolve(ctx, issuer, subject)
	if err != nil {
		return ExternalIdentityRevocation{}, err
	}
	return ExternalIdentityRevocation{issuer: mapping.Issuer, subject: mapping.Subject, userID: mapping.UserID}, nil
}

// RevokePrepared não autentica nem busca JWKS. É deliberadamente uma operação
// local curta para ser chamada dentro do MutateContextGroup após a preparação.
func (s *ExternalIdentityAdminService) RevokePrepared(ctx context.Context, prepared ExternalIdentityRevocation) error {
	if s == nil || s.repo == nil || !validExternalIdentityPart(s.cfg.Issuer) || !validExternalIdentityPart(prepared.issuer) || !validExternalIdentityPart(prepared.subject) || !canonicalSessionUUID(prepared.userID) {
		return ErrExternalIdentityNotReady
	}
	if prepared.issuer != s.cfg.Issuer {
		return ErrExternalAdministratorRequired
	}
	return s.repo.Revoke(ctx, prepared.issuer, prepared.subject)
}

func (s *ExternalIdentityAdminService) Enable(ctx context.Context, adminToken, issuer, subject string) error {
	claims, err := s.authorize(ctx, adminToken)
	if err != nil {
		return err
	}
	if issuer != claims.Issuer {
		return ErrExternalAdministratorRequired
	}
	return s.repo.Enable(ctx, issuer, subject)
}

func cleanValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsAll(values, required []string) bool {
	if len(required) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	for _, value := range required {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func containsAny(values, candidates []string) bool {
	if len(candidates) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	for _, value := range candidates {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}
