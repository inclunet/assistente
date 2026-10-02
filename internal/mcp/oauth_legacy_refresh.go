package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type legacyTokenSource struct {
	rt  *pkceRoundTripper
	cfg *oauth2.Config
}

func (s *legacyTokenSource) Token() (*oauth2.Token, error) {
	return s.rt.resolveLegacyToken(s.rt.longLivedCtx(), s.cfg, false)
}

func (rt *pkceRoundTripper) coordinatedLegacy() bool {
	return !rt.protocolOnly && rt.credMgr != nil && rt.cfg.ID != "" && rt.cfg.UserID != ""
}

func (rt *pkceRoundTripper) newTokenSource(token *oauth2.Token, cfg *oauth2.Config) oauth2.TokenSource {
	if rt.coordinatedLegacy() {
		return &legacyTokenSource{rt: rt, cfg: cfg}
	}
	return rt.wrapWithPersistence(newScopedTokenSource(rt.longLivedCtx(), token, cfg.TokenSource))
}

func tokenFromLegacyAuth(auth *credentials.AuthConfig) *oauth2.Token {
	token := &oauth2.Token{AccessToken: auth.Token, RefreshToken: auth.RefreshURL, TokenType: "Bearer"}
	if auth.ExpiresAt != 0 {
		token.Expiry = time.Unix(auth.ExpiresAt, 0)
	}
	return token
}

// Preserve Basic/Post negotiation only after a definitive client rejection.
// Ambiguous results must never replay a rotating refresh token.
type singleRefreshTransport struct {
	base     http.RoundTripper
	mu       sync.Mutex
	sent     bool
	attempts int
}

func (t *singleRefreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if t.sent || t.attempts >= 2 {
		t.mu.Unlock()
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, oauthflow.ErrReauthorize
	}
	t.sent = true
	t.attempts++
	t.mu.Unlock()
	response, err := t.base.RoundTrip(req)
	// Only a definitive OAuth invalid_client rejection permits the SDK's
	// existing Basic/Post negotiation. Ambiguous failures never replay refresh.
	if err == nil && response != nil && (response.StatusCode == 400 || response.StatusCode == 401) {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
		response.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), response.Body), response.Body}
		var problem struct {
			Error string `json:"error"`
		}
		if readErr == nil && len(body) <= 64*1024 && json.Unmarshal(body, &problem) == nil && problem.Error == "invalid_client" {
			t.mu.Lock()
			t.sent = false
			t.mu.Unlock()
		}
	}
	return response, err
}

func (rt *pkceRoundTripper) resolveLegacyToken(ctx context.Context, cfg *oauth2.Config, force bool, rejected ...string) (*oauth2.Token, error) {
	return rt.resolveLegacyTokenWithValidity(ctx, cfg, force, 0, rejected...)
}

func (rt *pkceRoundTripper) resolveLegacyTokenWithValidity(ctx context.Context, cfg *oauth2.Config, force bool, minimum time.Duration, rejected ...string) (*oauth2.Token, error) {
	valid := func(token *oauth2.Token) bool {
		return token.Valid() && (token.Expiry.IsZero() || time.Until(token.Expiry) > minimum)
	}
	ctx = database.WithUserID(ctx, rt.cfg.UserID)
	if rt.legacySession != nil {
		var cancel context.CancelFunc
		ctx, cancel = rt.legacySession.SessionContext(ctx)
		defer cancel()
	}
	auth, err := rt.credMgr.ReadLegacyOAuthToken(ctx, rt.serverSlug, rt.cfg.ID, rt.validateLegacyConsumer)
	if err != nil {
		return nil, err
	}
	token := tokenFromLegacyAuth(auth)
	if len(rejected) > 0 && token.AccessToken != rejected[0] && valid(token) {
		return token, nil
	}
	if valid(token) && !force {
		return token, nil
	}
	ctx, err = oauthflow.PreflightOAuthEndpoint(ctx, rt.cfg.URL, cfg.Endpoint.TokenURL, rt.networkAuthorizer)
	if err != nil {
		return nil, err
	}
	op, latest, err := rt.credMgr.BeginLegacyOAuth(ctx, rt.serverSlug, rt.cfg.ID, false, false, rt.validateLegacyConsumer)
	if err != nil {
		return nil, err
	}
	defer op.End()
	latestToken := tokenFromLegacyAuth(latest)
	if valid(latestToken) && (latest.Token != auth.Token || !force) {
		if err := op.Commit(ctx, latest); err != nil {
			return nil, errOAuthPersistence
		}
		return latestToken, nil
	}
	ctx, cancel := op.SessionContext(ctx)
	defer cancel()
	ctx, deadlineCancel := context.WithDeadline(ctx, op.Deadline())
	defer deadlineCancel()
	client := rt.oauthHTTPClient(15 * time.Second)
	client.Transport = &singleRefreshTransport{base: client.Transport}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	currentConfig := *cfg
	if latest.ClientID != "" {
		currentConfig.ClientID = latest.ClientID
	}
	currentConfig.ClientSecret = latest.ClientSecret
	newToken, err := currentConfig.TokenSource(ctx, &oauth2.Token{RefreshToken: latest.RefreshURL, Expiry: time.Now().Add(-time.Hour)}).Token()
	if err != nil {
		// SDK errors may contain the server's response body and credentials.
		return nil, oauthflow.ErrReauthorize
	}
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = latest.RefreshURL
	}
	updated := &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: newToken.AccessToken, RefreshURL: newToken.RefreshToken}
	if !newToken.Expiry.IsZero() {
		updated.ExpiresAt = newToken.Expiry.Unix()
	}
	if err := op.Commit(ctx, updated); err != nil {
		return nil, errors.Join(errOAuthPersistence, oauthflow.ErrReauthorize)
	}
	return newToken, nil
}

func (rt *pkceRoundTripper) validateLegacyConsumer(tx *gorm.DB) error {
	var row database.MCPServer
	if err := tx.Where("id = ? AND user_id = ?", rt.cfg.ID, rt.cfg.UserID).First(&row).Error; err != nil {
		return oauthflow.ErrConflict
	}
	actual, err := serverModelToConfig(row)
	if err != nil {
		return oauthflow.ErrConflict
	}
	// Polling may be a local probe adaptation; OAuth identity/configuration is not.
	actual.DisableSSE = rt.cfg.DisableSSE
	if !reflect.DeepEqual(persistedLegacyConfig(actual), persistedLegacyConfig(rt.cfg)) {
		return oauthflow.ErrConflict
	}
	return nil
}
