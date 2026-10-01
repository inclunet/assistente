package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Managed OAuth is opt-in for new servers. Legacy records are never converted
// implicitly: their migration requires the encrypted snapshot from AEP-0112.
func (m *Manager) saveManagedOAuth(slug string, cfg ServerConfig, secret *string) error {
	if err := validateServerSlug(cfg.Slug); err != nil {
		return err
	}
	ctx := m.credentialContext()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if m.credMgr == nil || (cfg.AuthType != AuthOAuth2PKCE && cfg.AuthType != AuthOAuth2ClientCredentials) || (cfg.Transport != TransportSSE && cfg.Transport != TransportStreamable) {
		return oauthflow.ErrResource
	}
	repo, ok := m.repository().(*DBRepository)
	if !ok {
		return oauthflow.ErrResource
	}
	existing, err := repo.GetServer(ctx, slug)
	creating := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !creating {
		return err
	}
	store, err := m.credMgr.OAuthStore(ctx)
	if err != nil {
		return err
	}
	var r oauthflow.Record
	if creating {
		if cfg.OAuthAuthorizationID != "" {
			return oauthflow.ErrResource
		}
		cfg.ID = uuid.NewString()
		r = oauthflow.Record{Version: 1, Revision: 1, ID: uuid.NewString(), UserID: user, ConsumerID: cfg.ID, Integration: "mcp", State: "pending"}
	} else {
		if existing.OAuthAuthorizationID == "" || (cfg.OAuthAuthorizationID != "" && cfg.OAuthAuthorizationID != existing.OAuthAuthorizationID) {
			return oauthflow.ErrResource
		}
		cfg.ID = existing.ID
		r, err = store.Load(ctx, existing.OAuthAuthorizationID)
		if err != nil {
			return err
		}
		if err = validateMCPAuthorization(*existing, r); err != nil {
			return err
		}
	}
	if r.AuthorizationActive() || r.RefreshActive() {
		return oauthflow.ErrConflict
	}
	previous := r
	r.Resource = cfg.URL
	if creating || previous.Resource != cfg.URL {
		r.Audience = cfg.URL
	}
	r.GrantType = "authorization_code"
	if cfg.AuthType == AuthOAuth2ClientCredentials {
		r.GrantType = "client_credentials"
	}
	r.Endpoints.Authorization = cfg.OAuth2AuthURL
	r.Endpoints.Token = cfg.OAuth2TokenURL
	r.Endpoints.Registration = cfg.OAuth2RegistrationURL
	r.Endpoints.Device = cfg.OAuth2DeviceAuthURL
	r.RequestedScopes = append([]string(nil), cfg.OAuth2Scopes...)
	r.Callback = oauthflow.CallbackConfig{Host: cfg.OAuth2CallbackHost, Port: cfg.OAuth2CallbackPort, Path: "/callback", PortPolicy: "ephemeral"}
	if r.Callback.Port != 0 {
		r.Callback.PortPolicy = "fixed"
	}
	if r.Client.ID != cfg.OAuth2ClientID || previous.Resource != r.Resource || previous.Endpoints.Token != r.Endpoints.Token {
		r.Client.Secret = ""
		r.Client.GrantType = ""
		r.Client.Method = "manual"
	}
	r.Client.ID = cfg.OAuth2ClientID
	if secret != nil {
		r.Client.Secret = *secret
	}
	r.Client.AuthMethod = cfg.OAuth2TokenAuthMethod
	if r.Client.AuthMethod == "" || r.Client.AuthMethod == "none" {
		r.Client.AuthMethod = "client_secret_post"
	}
	if _, err = oauthflow.NewConfigured(r, m.authorizeOAuthNetwork); err != nil {
		return err
	}
	changed := !reflect.DeepEqual(r, previous)
	if !creating && changed {
		r.State = "pending"
		r.Tokens = oauthflow.Tokens{}
		r.GrantedScopes = nil
		r.AuthorizationAttempt = ""
		r.AuthorizationUntil = time.Time{}
		r.RefreshPending = false
		r.RefreshUntil = time.Time{}
	}
	cfg.Slug = slug
	cfg.UserID = user
	cfg.OAuthManaged = true
	cfg.OAuthAuthorizationID = r.ID
	clearOAuthConfiguration(&cfg)
	save := func(tx *gorm.DB) error {
		// Recheck the binding inside the same transaction as the encrypted envelope.
		if !creating {
			var row database.MCPServer
			if err := tx.Where("id = ? AND user_id = ? AND oauth_authorization_id = ?", cfg.ID, user, r.ID).First(&row).Error; err != nil {
				return err
			}
		}
		if creating {
			row, err := serverConfigToModel(cfg)
			if err != nil {
				return err
			}
			return tx.Create(&row).Error
		}
		return NewDBRepository(tx).SaveServer(ctx, &cfg)
	}
	if creating {
		atomicStore, ok := store.(interface {
			CreateWithConsumer(context.Context, oauthflow.Record, func(*gorm.DB) error) error
		})
		if !ok {
			return oauthflow.ErrResource
		}
		err = atomicStore.CreateWithConsumer(ctx, r, save)
	} else {
		atomicStore, ok := store.(interface {
			CompareAndSwapWithConsumer(context.Context, oauthflow.Record, uint64, func(*gorm.DB) error) error
		})
		if !ok {
			return oauthflow.ErrResource
		}
		r.Revision++
		err = atomicStore.CompareAndSwapWithConsumer(ctx, r, previous.Revision, save)
	}
	if err != nil {
		return err
	}
	publish := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if current := m.servers[slug]; current != nil {
			current.ID = cfg.ID
			current.Config = cfg
		} else {
			m.servers[slug] = &ServerStatus{ID: cfg.ID, Slug: slug, Config: cfg, Status: StatusDisconnected, Tools: []MCPToolInfo{}}
		}
		return nil
	}
	if scoped, ok := store.(interface {
		WithSession(context.Context, func() error) error
	}); ok {
		err = scoped.WithSession(ctx, publish)
	} else {
		err = publish()
	}
	if err != nil {
		return err
	}
	if changed && !creating {
		_ = m.Disconnect(slug)
	}
	m.emit("mcp:config_changed", map[string]string{"slug": slug})
	return nil
}

func clearOAuthConfiguration(cfg *ServerConfig) {
	cfg.OAuth2TokenAuthMethod = ""
	cfg.OAuth2ClientID = ""
	cfg.OAuth2AuthURL = ""
	cfg.OAuth2TokenURL = ""
	cfg.OAuth2Scopes = nil
	cfg.OAuth2CallbackHost = ""
	cfg.OAuth2CallbackPort = 0
	cfg.OAuth2RegistrationURL = ""
	cfg.OAuth2DeviceAuthURL = ""
}
func projectOAuthConfiguration(cfg ServerConfig, r oauthflow.Record) ServerConfig {
	cfg.OAuth2TokenAuthMethod = r.Client.AuthMethod
	cfg.OAuth2ClientID = r.Client.ID
	cfg.OAuth2AuthURL = r.Endpoints.Authorization
	cfg.OAuth2TokenURL = r.Endpoints.Token
	cfg.OAuth2RegistrationURL = r.Endpoints.Registration
	cfg.OAuth2DeviceAuthURL = r.Endpoints.Device
	cfg.OAuth2Scopes = append([]string(nil), r.RequestedScopes...)
	cfg.OAuth2CallbackHost = r.Callback.Host
	cfg.OAuth2CallbackPort = r.Callback.Port
	return cfg
}
func validateMCPAuthorization(cfg ServerConfig, r oauthflow.Record) error {
	grant := "authorization_code"
	if cfg.AuthType == AuthOAuth2ClientCredentials {
		grant = "client_credentials"
	}
	if !cfg.OAuthManaged || cfg.OAuthAuthorizationID != r.ID || cfg.ID != r.ConsumerID || cfg.UserID != r.UserID || cfg.URL != r.Resource || r.Integration != "mcp" || r.GrantType != grant || (cfg.AuthType != AuthOAuth2PKCE && cfg.AuthType != AuthOAuth2ClientCredentials) {
		return oauthflow.ErrResource
	}
	return nil
}
func (m *Manager) managedOAuth(ctx context.Context, cfg ServerConfig) (oauthflow.Store, oauthflow.Record, *oauthflow.Service, error) {
	if m.credMgr == nil {
		return nil, oauthflow.Record{}, nil, oauthflow.ErrResource
	}
	store, err := m.credMgr.OAuthStore(ctx)
	if err != nil {
		return nil, oauthflow.Record{}, nil, err
	}
	r, err := store.Load(ctx, cfg.OAuthAuthorizationID)
	if err != nil {
		return nil, r, nil, err
	}
	if err = validateMCPAuthorization(cfg, r); err != nil {
		return nil, r, nil, err
	}
	service, err := oauthflow.NewConfigured(r, m.authorizeOAuthNetwork)
	return store, r, service, err
}
func (m *Manager) projectManagedOAuth(cfg *ServerConfig) (*ServerConfig, error) {
	if cfg.OAuthAuthorizationID == "" {
		return cfg, nil
	}
	_, r, _, err := m.managedOAuth(m.credentialContext(), *cfg)
	if err != nil {
		return nil, err
	}
	result := projectOAuthConfiguration(*cfg, r)
	return &result, nil
}
func (m *Manager) resolveManagedOAuth(ctx context.Context, cfg ServerConfig, rejected string) (oauthflow.Record, error) {
	store, _, service, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		return oauthflow.Record{}, err
	}
	r, err := service.Resolve(oauthflow.WithNetworkOperation(ctx), store, cfg.OAuthAuthorizationID, cfg.URL, rejected)
	if err != nil {
		return r, err
	}
	latest, err := store.Load(ctx, r.ID)
	if err != nil {
		return r, err
	}
	if latest.Revision != r.Revision || latest.State != "connected" || latest.RefreshPending {
		return oauthflow.Record{}, oauthflow.ErrConflict
	}
	return r, nil
}
func (m *Manager) beginManagedAttempt(ctx context.Context, slug string) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	m.mu.Lock()
	if m.managedAttempts == nil {
		m.managedAttempts = make(map[string]context.CancelFunc)
	}
	if m.managedAttempts[slug] != nil {
		m.mu.Unlock()
		stop()
		cancel()
		return nil, nil, oauthflow.ErrConflict
	}
	m.managedAttempts[slug] = cancel
	m.mu.Unlock()
	return ctx, func() { stop(); cancel(); m.mu.Lock(); delete(m.managedAttempts, slug); m.mu.Unlock() }, nil
}

func (m *Manager) authorizeManagedOAuth(ctx context.Context, slug string, cfg ServerConfig) error {
	ctx, done, err := m.beginManagedAttempt(ctx, slug)
	if err != nil {
		return err
	}
	defer done()
	return m.authorizeManagedOAuthInAttempt(ctx, slug, cfg)
}

func (m *Manager) authorizeManagedOAuthInAttempt(ctx context.Context, slug string, cfg ServerConfig) error {
	store, _, service, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		return err
	}
	_, err = service.AuthorizeUsing(ctx, store, cfg.OAuthAuthorizationID, func(flowCtx context.Context, r oauthflow.Record) (oauthflow.Record, error) {
		// This adapter runs only the existing protocol choreography. It has no vault,
		// config writer or live connection: only the shared service commits its result.
		rt := &pkceRoundTripper{
			cfg: projectOAuthConfiguration(cfg, r), protocolOnly: true, serverSlug: slug, emitEvent: m.emitEvent,
			resolvedClientID: r.Client.ID, resolvedClientSecret: r.Client.Secret, clientGrantType: r.Client.GrantType,
			clientAuthMethod: r.Client.AuthMethod, resourceURL: r.Audience, networkAuthorizer: m.authorizeOAuthNetwork,
			authCtxProvider: func() context.Context { return flowCtx }, lifetimeCtx: flowCtx,
		}
		if err := rt.authorize(flowCtx); err != nil {
			return r, err
		}
		token := rt.issuedToken
		if token == nil || token.AccessToken == "" || !strings.EqualFold(token.Type(), "Bearer") {
			return r, oauthflow.ErrReauthorize
		}
		r.Client.ID = rt.effectiveClientID()
		r.Client.Secret = rt.effectiveClientSecret()
		r.Client.GrantType = rt.clientGrantType
		if rt.clientGrantType != "" {
			r.Client.Method = "dcr"
		}
		r.Endpoints.Authorization = rt.cfg.OAuth2AuthURL
		r.Endpoints.Token = rt.cfg.OAuth2TokenURL
		r.Endpoints.Device = rt.cfg.OAuth2DeviceAuthURL
		r.Endpoints.Registration = rt.cfg.OAuth2RegistrationURL
		r.Callback.Host = rt.cfg.OAuth2CallbackHost
		r.Callback.Port = rt.cfg.OAuth2CallbackPort
		if r.Callback.Port != 0 {
			r.Callback.PortPolicy = "fixed"
		}
		r.Audience = rt.resourceURL
		r.RequestedScopes = rt.effectiveScopes()
		r.GrantedScopes = append([]string(nil), rt.oauthCfg.Scopes...)
		if scope, ok := token.Extra("scope").(string); ok {
			r.GrantedScopes = strings.Fields(scope)
		}
		r.Tokens = oauthflow.Tokens{Access: token.AccessToken, Refresh: token.RefreshToken, Type: "Bearer", ExpiresAt: token.Expiry}
		return r, nil
	})
	return err
}

// A managed transport never opens a browser or falls back to legacy credentials.
// A 401 can renew once; replay is limited to requests with a recreatable body.
type managedOAuthTransport struct {
	manager *Manager
	cfg     ServerConfig
	store   oauthflow.Store
	base    http.RoundTripper
	ctx     context.Context
}

func (t *managedOAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	if t.ctx.Err() != nil {
		cancel()
	}
	cleanup := func() { stop(); cancel() }
	user, err := database.RequireUserID(t.ctx)
	if err != nil {
		cleanup()
		return nil, err
	}
	ctx = database.WithUserID(ctx, user)
	if scoped, ok := t.store.(interface {
		SessionContext(context.Context) (context.Context, context.CancelFunc)
	}); ok {
		var sessionCancel context.CancelFunc
		ctx, sessionCancel = scoped.SessionContext(ctx)
		old := cleanup
		cleanup = func() { sessionCancel(); old() }
	}
	r, err := t.manager.resolveManagedOAuth(ctx, t.cfg, "")
	if err != nil {
		cleanup()
		return nil, err
	}
	first := req.Clone(ctx)
	first.Header.Set("Authorization", "Bearer "+r.Tokens.Access)
	response, err := t.base.RoundTrip(first)
	if err == nil && response.StatusCode == http.StatusUnauthorized {
		fresh, refreshErr := t.manager.resolveManagedOAuth(ctx, t.cfg, r.Tokens.Access)
		if refreshErr != nil {
			_ = response.Body.Close()
			cleanup()
			return nil, refreshErr
		}
		if fresh.Tokens.Access != r.Tokens.Access && (req.Body == nil || req.Body == http.NoBody || req.GetBody != nil) {
			retry := req.Clone(ctx)
			if req.GetBody != nil {
				retry.Body, err = req.GetBody()
				if err != nil {
					_ = response.Body.Close()
					cleanup()
					return nil, err
				}
			}
			if err == nil {
				_ = response.Body.Close()
				retry.Header.Set("Authorization", "Bearer "+fresh.Tokens.Access)
				response, err = t.base.RoundTrip(retry)
			}
		}
	}
	if err != nil {
		cleanup()
		return nil, err
	}
	response.Body = &managedOAuthBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}

type managedOAuthBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *managedOAuthBody) Close() error { err := b.ReadCloser.Close(); b.cleanup(); return err }
func (m *Manager) managedHTTPClient(ctx context.Context, cfg ServerConfig) *http.Client {
	// Capture the authenticated user and vault session once, never from a later login.
	user, userErr := database.RequireUserID(m.credentialContext())
	if userErr != nil {
		return oauthflow.NewResourceHTTPClient(cfg.URL, oauthErrorTransport{userErr})
	}
	ctx = database.WithUserID(ctx, user)
	store, _, _, err := m.managedOAuth(ctx, cfg)
	if err != nil {
		return oauthflow.NewResourceHTTPClient(cfg.URL, oauthErrorTransport{err})
	}
	return oauthflow.NewResourceHTTPClient(cfg.URL, &managedOAuthTransport{manager: m, cfg: cfg, ctx: ctx, store: store, base: oauthflow.NewResourceTransport(cfg.URL, m.authorizeOAuthNetwork)})
}

type oauthErrorTransport struct{ err error }

func (t oauthErrorTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		_ = r.Body.Close()
	}
	return nil, t.err
}

func (m *Manager) detachManagedOAuth(ctx context.Context, slug string, cfg ServerConfig, remove bool) error {
	repo, ok := m.repository().(*DBRepository)
	if !ok {
		return oauthflow.ErrResource
	}
	existing, err := repo.GetServer(ctx, slug)
	if err != nil {
		return err
	}
	store, r, _, err := m.managedOAuth(ctx, *existing)
	if err != nil {
		return err
	}
	atomicStore, ok := store.(interface {
		DeleteWithConsumer(context.Context, string, uint64, func(*gorm.DB) error) error
	})
	if !ok {
		return oauthflow.ErrResource
	}
	cfg.OAuthAuthorizationID = ""
	cfg.OAuthManaged = false
	clearOAuthConfiguration(&cfg)
	err = atomicStore.DeleteWithConsumer(ctx, r.ID, r.Revision, func(tx *gorm.DB) error {
		var bound database.MCPServer
		if err := tx.Where("id = ? AND user_id = ? AND slug = ? AND oauth_authorization_id = ?", existing.ID, r.UserID, slug, r.ID).First(&bound).Error; err != nil {
			return err
		}
		if remove {
			return NewDBRepository(tx).DeleteServer(ctx, slug)
		}
		return NewDBRepository(tx).SaveServer(ctx, &cfg)
	})
	if err != nil {
		return err
	}
	_ = m.Disconnect(slug)
	m.mu.Lock()
	if current := m.servers[slug]; current != nil && current.ID == existing.ID && current.Config.UserID == r.UserID {
		if remove {
			delete(m.servers, slug)
		} else {
			current.Config = cfg
		}
	}
	m.mu.Unlock()
	m.emit("mcp:config_changed", map[string]string{"slug": slug})
	return nil
}

// Transport fallback changes only transport metadata, never the OAuth envelope.
func (m *Manager) persistManagedPolling(ctx context.Context, cfg ServerConfig) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	repo, ok := m.repository().(*DBRepository)
	if !ok {
		return oauthflow.ErrResource
	}
	result := repo.db.WithContext(ctx).Model(&database.MCPServer{}).Where("id = ? AND user_id = ? AND slug = ? AND oauth_authorization_id = ?", cfg.ID, user, cfg.Slug, cfg.OAuthAuthorizationID).Update("disable_sse", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return oauthflow.ErrConflict
	}
	return nil
}

// SaveConfigWithOAuthSecret commits configuration and supplied secret together.
func (m *Manager) SaveConfigWithOAuthSecret(slug string, cfg ServerConfig, secret string) error {
	slug = strings.TrimSpace(slug)
	cfg.Slug = slug
	if !cfg.OAuthManaged {
		return oauthflow.ErrResource
	}
	return m.saveManagedOAuth(slug, cfg, &secret)
}
