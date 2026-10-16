package app

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbridge"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/httpapi"
)

func (a *App) startHTTPAPI() error {
	authCfg, err := config.LoadAuthConfig()
	if err != nil {
		return fmt.Errorf("carregar auth.json: %w", err)
	}
	if !authCfg.HTTP.Enabled {
		return nil
	}
	if err := httpapi.ValidateBindSecurity(authCfg.HTTP.BindAddress, authCfg.HTTP.TLSEnabled, authCfg.HTTP.DevInsecure); err != nil {
		return err
	}

	// B17: DevInsecure faz HTTP plain. Se config caiu em produção por
	// engano (env var não setada, copy-paste), credenciais e JWTs
	// trafegam em texto. Aborta hard quando ASSISTENTE_ENV=production
	// e grita no log quando bind não é loopback (LAN exposta).
	if err := guardDevInsecure(authCfg); err != nil {
		return err
	}

	handler, err := a.newHTTPAPIHandler(authCfg)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", authCfg.HTTP.BindAddress)
	if err != nil {
		// M25: detecta porta em uso e dá uma mensagem útil — antes vinha
		// só "listen tcp 127.0.0.1:17652: bind: address already in use"
		// que é genérico para o usuário final.
		if isAddrInUseErr(err) {
			return fmt.Errorf("iniciar HTTP API em %s: porta já está em uso (outra instância do assistente rodando ou outro processo escutando)", authCfg.HTTP.BindAddress)
		}
		return fmt.Errorf("iniciar HTTP API em %s: %w", authCfg.HTTP.BindAddress, err)
	}

	// B15: timeouts conservadores para evitar Slowloris e clientes lentos
	// presos. Auth API processa requests pequenos (login/refresh/JWKS),
	// então 30s para read/write é folga grande sem deixar atacante
	// segurar conexão indefinidamente.
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	a.httpAPIServer = server

	go func() {
		var serveErr error
		if authCfg.HTTP.TLSEnabled {
			serveErr = server.ServeTLS(listener, authCfg.HTTP.TLSCertFile, authCfg.HTTP.TLSKeyFile)
		} else {
			serveErr = server.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logging.Errorf(context.Background(), "app.app-httpapi", "[httpapi] servidor encerrado com erro: %v", serveErr)
		}
	}()
	logging.Infof(context.Background(), "app.app-httpapi", "[httpapi] escutando em %s (mode=%s tls=%v)", listener.Addr().String(), authCfg.Mode, authCfg.HTTP.TLSEnabled)
	return nil
}

// newHTTPAPIHandler monta a autenticação e as rotas antes de abrir o listener.
// Cadastro administrativo não publica readiness do executor externo.
func (a *App) newHTTPAPIHandler(cfg *config.AuthConfig) (http.Handler, error) {
	if a == nil || cfg == nil {
		return nil, errors.New("configuração HTTP indisponível")
	}
	var external *auth.ExternalAuthenticator
	var mappings *auth.ExternalIdentityRepository
	var commandAuthenticator *auth.ExternalCommandAuthenticator
	if cfg.Mode == "external" {
		mappings = auth.NewExternalIdentityRepository(database.DB())
		external = auth.NewExternalAuthenticator(auth.ExternalAuthConfig{
			Issuer: cfg.External.Issuer, Audience: cfg.External.Audience,
			JWKSURL: cfg.External.JWKSURL, AllowedAlgorithms: cfg.External.AllowedAlgorithms,
			RequiredScopes: cfg.External.RequiredScopes, RoleClaim: cfg.External.RoleClaim,
		})
		commandAuthenticator = auth.NewExternalCommandAuthenticator(external, mappings)
		commandAuthenticator.SetReadiness(mappings.CheckReadiness)
	}
	var admin *auth.ExternalIdentityAdminService
	if external != nil && len(cfg.External.IdentityAdminScopes) > 0 {
		var err error
		admin, err = auth.NewExternalIdentityAdminService(external,
			mappings, auth.ExternalIdentityAdminConfig{
				Issuer: cfg.External.Issuer, AdminScopes: cfg.External.IdentityAdminScopes,
			})
		if err != nil {
			return nil, fmt.Errorf("configurar cadastro de identidades externas: %w", err)
		}
	}
	var externalCommandProvider httpapi.ExternalCommandProvider
	var externalUIConnections httpapi.ExternalUIConnectionPort
	if commandAuthenticator != nil && admin != nil && hasConfiguredExternalCommandScope(cfg) {
		provider := &appExternalCommandProvider{app: a, authenticator: commandAuthenticator, admin: admin, config: cfg}
		if previous := a.commandExternalHTTP.Swap(provider); previous != nil {
			previous.Close()
		}
		externalCommandProvider = provider
		externalUIConnections = &appExternalUIConnectionPort{app: a, manager: a.ensureExternalUIConnections()}
	}
	return httpapi.New(httpapi.Config{
		Vault: a.vaultSvc, IDs: a.identitySvc, Sessions: a.currentSessionService,
		Mode: cfg.Mode, External: external, ExternalIdentityAdmin: admin,
		ExternalIdentities: mappings, ExternalCommandProvider: externalCommandProvider, ExternalUIConnections: externalUIConnections,
		ExternalCommandWriteTimeout: externalCommandHTTPExecutionTimeout + externalCommandHTTPWriteMargin,
	}).Handler(), nil
}

type appExternalCommandProvider struct {
	mu            sync.Mutex
	app           *App
	authenticator *auth.ExternalCommandAuthenticator
	admin         *auth.ExternalIdentityAdminService
	config        *config.AuthConfig
	product       *commandProductRuntime
	service       *commandexecution.ExternalService
	initialized   bool
	closed        bool
}

func (p *appExternalCommandProvider) ExternalCommandService(source string) *commandexecution.ExternalService {
	if p == nil || source != "ui" || p.app == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	product := p.app.commandProduct.Load()
	if p.initialized && p.product == product && p.service != nil {
		service := p.service
		p.mu.Unlock()
		return service
	}
	var next *commandexecution.ExternalService
	if product != nil {
		next, _ = p.app.newExternalUICommandExecutor(product, p.authenticator, p.admin, p.config)
		if p.app.commandProduct.Load() != product || !product.dependenciesMatch(p.app) {
			if next != nil {
				closeExternalCommandService(next)
			}
			next = nil
		}
	}
	previous := p.service
	p.product, p.service, p.initialized = product, next, true
	p.mu.Unlock()
	if previous != nil && previous != next {
		closeExternalCommandService(previous)
	}
	return next
}

func (p *appExternalCommandProvider) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	service := p.service
	p.service = nil
	p.mu.Unlock()
	if service != nil {
		closeExternalCommandService(service)
	}
}

func closeExternalCommandService(service *commandexecution.ExternalService) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = service.Shutdown(ctx)
}

type appExternalUIConnectionPort struct {
	app     *App
	manager *commandui.ExternalUIConnections
}

func (p *appExternalUIConnectionPort) validate(ctx context.Context, principal auth.ExternalCommandPrincipal) (commandbridge.Owner, commandui.ExternalUIPrincipal, error) {
	if p == nil || p.app == nil || p.manager == nil || ctx == nil || ctx.Err() != nil {
		if ctx != nil && ctx.Err() != nil {
			return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, ctx.Err()
		}
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, httpapi.ErrExternalUIConnectionUnavailable
	}
	product := p.app.commandProduct.Load()
	if product == nil || principal.UserID == "" || principal.UserID != product.principal.UserID || !product.dependenciesMatch(p.app) {
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, httpapi.ErrExternalUIConnectionUnavailable
	}
	current, err := product.sessionSvc.RevalidateLocalSession(ctx, product.principal)
	if err != nil {
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, err
	}
	if current != product.principal {
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, commandexecution.ErrDenied
	}
	versions, err := product.host.Snapshot(ctx, product.principal)
	if err != nil {
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, err
	}
	if !versions.Unlocked {
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, commandexecution.ErrDenied
	}
	if p.app.commandProduct.Load() != product || !product.dependenciesMatch(p.app) {
		return commandbridge.Owner{}, commandui.ExternalUIPrincipal{}, commandexecution.ErrStale
	}
	owner := product.owner()
	identity := commandui.ExternalUIPrincipal{Issuer: principal.Issuer, Subject: principal.Subject, UserID: principal.UserID, AuthContextID: principal.AuthContextID}
	return owner, identity, nil
}

func (p *appExternalUIConnectionPort) ConsumeInvitation(ctx context.Context, invitation string, principal auth.ExternalCommandPrincipal) (httpapi.ExternalUIConnectionContext, error) {
	owner, identity, err := p.validate(ctx, principal)
	if err != nil {
		return httpapi.ExternalUIConnectionContext{}, err
	}
	product := p.app.commandProduct.Load()
	if product == nil || product.owner() != owner {
		return httpapi.ExternalUIConnectionContext{}, commandexecution.ErrStale
	}
	product.mu.Lock()
	defer product.mu.Unlock()
	if product.closed || p.app.commandProduct.Load() != product {
		return httpapi.ExternalUIConnectionContext{}, commandexecution.ErrStale
	}
	status, err := p.manager.Claim(invitation, identity)
	if err != nil {
		return httpapi.ExternalUIConnectionContext{}, commandexecution.ErrDenied
	}
	return httpapi.ExternalUIConnectionContext{ConnectionID: status.ConnectionID, Generation: status.Generation, TargetSnapshotID: status.TargetSnapshotID, ContextVersion: status.ContextVersion, ExpiresAt: status.ExpiresAt}, nil
}

func (p *appExternalUIConnectionPort) ReadConnection(ctx context.Context, principal auth.ExternalCommandPrincipal, connectionID, generation string) (httpapi.ExternalUIConnectionContext, error) {
	owner, identity, err := p.validate(ctx, principal)
	if err != nil {
		return httpapi.ExternalUIConnectionContext{}, err
	}
	status, err := p.manager.ReadForPrincipal(identity, connectionID, generation)
	if err != nil || status.Owner != owner {
		return httpapi.ExternalUIConnectionContext{}, commandexecution.ErrDenied
	}
	return httpapi.ExternalUIConnectionContext{ConnectionID: status.ConnectionID, Generation: status.Generation, TargetSnapshotID: status.TargetSnapshotID, ContextVersion: status.ContextVersion, ExpiresAt: status.ExpiresAt}, nil
}

func (p *appExternalUIConnectionPort) RevokePrincipal(_ context.Context, issuer, subject string) {
	if p != nil && p.manager != nil {
		p.manager.RevokePrincipal(issuer, subject)
	}
}

// guardDevInsecure aplica heurísticas para evitar que dev_insecure=true
// vaze para produção sem aviso (B17 do review do AEP-0052):
//
//   - ASSISTENTE_ENV=production: aborta hard. Não há caso de uso legítimo.
//   - Bind não-loopback com dev_insecure: log explícito com WARNING.
//     Loopback (127.0.0.1/localhost) já é coberto por ValidateBindSecurity.
func guardDevInsecure(cfg *config.AuthConfig) error {
	if !cfg.HTTP.DevInsecure {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ASSISTENTE_ENV")), "production") {
		return errors.New("HTTP API: dev_insecure=true não é permitido com ASSISTENTE_ENV=production")
	}
	host, _, err := net.SplitHostPort(cfg.HTTP.BindAddress)
	if err != nil {
		host = cfg.HTTP.BindAddress
	}
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !loopback {
		logging.Warnf(context.Background(), "app.app-httpapi", "[httpapi] WARNING: dev_insecure=true em bind não-loopback %q — credenciais e tokens trafegam em texto", cfg.HTTP.BindAddress)
	}
	return nil
}

// isAddrInUseErr reconhece o erro de porta já alocada em Linux/macOS/Windows
// para que o caller possa dar uma mensagem amigável (M25 do review).
func isAddrInUseErr(err error) bool {
	var sysErr *net.OpError
	if !errors.As(err, &sysErr) {
		return false
	}
	if errors.Is(sysErr.Err, syscall.EADDRINUSE) {
		return true
	}
	// Windows: WSAEADDRINUSE (10048). syscall.EADDRINUSE no Windows
	// é reexportado como 10048, mas em Linux/macOS é 98/48 — errors.Is
	// acima já cobre. O fallback de string é um seguro extra.
	return strings.Contains(strings.ToLower(err.Error()), "address already in use") ||
		strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address")
}
