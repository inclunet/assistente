package mcp

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// Persisted legacy CC consumers share PKCE's durable exclusion, but their
// non-rotating grants remain retryable and tokens remain local to this transport.
type legacyClientGrantTransport struct {
	manager                *Manager
	cfg                    ServerConfig
	ctx                    context.Context
	store                  oauthflow.Store
	base                   http.RoundTripper
	mu                     sync.Mutex
	token                  *oauth2.Token
	clientID, clientSecret string
}

func (m *Manager) legacyClientGrantHTTPClient(ctx context.Context, cfg ServerConfig) *http.Client {
	user, err := database.RequireUserID(m.credentialContext())
	if err != nil || user != cfg.UserID || m.credMgr == nil {
		return oauthflow.NewResourceHTTPClient(cfg.URL, oauthErrorTransport{oauthflow.ErrConflict})
	}
	ctx = database.WithUserID(ctx, user)
	store, err := m.credMgr.OAuthStore(ctx)
	if err != nil {
		return oauthflow.NewResourceHTTPClient(cfg.URL, oauthErrorTransport{err})
	}
	return oauthflow.NewResourceHTTPClient(cfg.URL, &legacyClientGrantTransport{manager: m, cfg: cfg, ctx: ctx, store: store, base: oauthflow.NewResourceTransport(cfg.URL, m.authorizeOAuthNetwork)})
}

func (t *legacyClientGrantTransport) validate(tx *gorm.DB) error {
	var row database.MCPServer
	if err := tx.Where("id = ? AND user_id = ?", t.cfg.ID, t.cfg.UserID).First(&row).Error; err != nil {
		return oauthflow.ErrConflict
	}
	actual, err := serverModelToConfig(row)
	actual.DisableSSE = t.cfg.DisableSSE
	if err != nil || !reflect.DeepEqual(persistedLegacyConfig(actual), persistedLegacyConfig(t.cfg)) {
		return oauthflow.ErrConflict
	}
	return nil
}

func (t *legacyClientGrantTransport) resolve(ctx context.Context) (*oauth2.Token, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ctx = oauthflow.WithNetworkOperation(ctx)
	preflight := func() error {
		var err error
		ctx, err = oauthflow.PreflightOAuthEndpoint(ctx, t.cfg.URL, t.cfg.OAuth2TokenURL, t.manager.authorizeOAuthNetwork)
		return err
	}
	prepared := !t.token.Valid()
	if prepared {
		if err := preflight(); err != nil {
			return nil, err
		}
	}
	begin := func() (*credentials.LegacyOAuthOperation, *credentials.AuthConfig, error) {
		return t.manager.credMgr.BeginLegacyClientGrant(ctx, t.cfg.Slug, t.cfg.ID, t.validate)
	}
	op, latest, err := begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if op != nil {
			op.End()
		}
	}()
	clientID := func(auth *credentials.AuthConfig) (string, error) {
		if t.cfg.OAuth2ClientID != "" {
			if auth.ClientID != "" && auth.ClientID != t.cfg.OAuth2ClientID {
				return "", oauthflow.ErrConflict
			}
			return t.cfg.OAuth2ClientID, nil
		}
		if auth.ClientID == "" {
			return "", oauthflow.ErrClientConfiguration
		}
		return auth.ClientID, nil
	}
	id, err := clientID(latest)
	if err != nil {
		return nil, err
	}
	if t.token.Valid() && id == t.clientID && latest.ClientSecret == t.clientSecret {
		if err := op.FinishClientGrant(ctx); err != nil {
			return nil, err
		}
		return t.token, nil
	}
	// A changed client invalidates local material. Human consent must precede
	// the short durable lease, even if another instance changed the credentials.
	t.token = nil
	if !prepared {
		op.End()
		if err := preflight(); err != nil {
			return nil, err
		}
		op, latest, err = begin()
		if err != nil {
			return nil, err
		}
		id, err = clientID(latest)
		if err != nil {
			return nil, err
		}
	}
	grantCtx, cancel := context.WithDeadline(ctx, op.Deadline())
	defer cancel()
	token, err := oauthflow.RequestClientCredentialsToken(grantCtx, oauthflow.ClientCredentialsConfig{
		Resource: t.cfg.URL, ClientID: id, ClientSecret: latest.ClientSecret,
		TokenEndpoint: t.cfg.OAuth2TokenURL, Scopes: t.cfg.OAuth2Scopes,
	}, t.manager.authorizeOAuthNetwork)
	if err != nil {
		// oauth2 errors may include the provider body and secrets.
		if grantCtx.Err() != nil {
			return nil, grantCtx.Err()
		}
		if errors.Is(err, oauthflow.ErrNetworkAuthorization) {
			return nil, oauthflow.ErrNetworkAuthorization
		}
		return nil, oauthflow.ErrTransient
	}
	if token == nil || token.AccessToken == "" || !strings.EqualFold(token.Type(), "Bearer") {
		return nil, oauthflow.ErrPermission
	}
	if err := op.FinishClientGrant(ctx); err != nil {
		return nil, err
	}
	token.RefreshToken = ""
	t.token, t.clientID, t.clientSecret = token, id, latest.ClientSecret
	return token, nil
}

func (t *legacyClientGrantTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	if t.ctx.Err() != nil {
		cancel()
	}
	ctx = database.WithUserID(ctx, t.cfg.UserID)
	scoped, ok := t.store.(interface {
		SessionContext(context.Context) (context.Context, context.CancelFunc)
	})
	if !ok {
		stop()
		cancel()
		return nil, oauthflow.ErrConflict
	}
	ctx, sessionCancel := scoped.SessionContext(ctx)
	cleanup := func() { sessionCancel(); stop(); cancel() }
	token, err := t.resolve(ctx)
	if err != nil {
		cleanup()
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errors.Join(errLegacyOAuthResolution, err)
	}
	request := req.Clone(ctx)
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response, err := t.base.RoundTrip(request)
	if err != nil {
		cleanup()
		return nil, err
	}
	response.Body = &managedOAuthBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}
