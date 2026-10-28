package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// An explicit invalid-grant response prevents access/refresh token reuse.
var errRejectedGrant = errors.New("oauth_grant_rejected")

type authorizationGate struct {
	channel chan struct{}
	users   int
}

type Service struct {
	gateOwner     *Service
	BeforeRefresh func(context.Context, Record) error
	HTTP          *http.Client
	integrations  map[string]Integration
	gatesMu       sync.Mutex
	gates         map[string]*authorizationGate
}

func New(integrations ...Integration) *Service {
	s := &Service{HTTP: NewHTTPClient(), integrations: make(map[string]Integration)}
	for _, i := range integrations {
		s.integrations[i.ID] = i
	}
	return s
}
func (s *Service) gate(ctx context.Context, id string) (func(), error) {
	if s.gateOwner != nil {
		return s.gateOwner.gate(ctx, id)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.gatesMu.Lock()
	if s.gates == nil {
		s.gates = make(map[string]*authorizationGate)
	}
	gate := s.gates[id]
	if gate == nil {
		gate = &authorizationGate{channel: make(chan struct{}, 1)}
		s.gates[id] = gate
	}
	// Count holders and waiters before leaving the map lock, so cleanup cannot
	// create a second gate while an existing caller still references the first.
	gate.users++
	s.gatesMu.Unlock()
	leave := func() {
		s.gatesMu.Lock()
		gate.users--
		if gate.users == 0 {
			delete(s.gates, id)
		}
		s.gatesMu.Unlock()
	}
	select {
	case gate.channel <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-gate.channel; leave() }) }, nil
	case <-ctx.Done():
		leave()
		return nil, ctx.Err()
	}
}

func (s *Service) integration(r Record) (Integration, error) {
	i, ok := s.integrations[r.Integration]
	if !ok {
		return Integration{}, ErrResource
	}
	return i, i.Validate(r)
}
func (s *Service) Pending(id, userID, integration string) (Record, error) {
	i, ok := s.integrations[integration]
	if !ok {
		return Record{}, ErrResource
	}
	return Record{Version: 1, ID: id, UserID: userID, Integration: i.ID, Revision: 1, State: "pending", Issuer: i.Issuer, Resource: i.Resource, RequestedScopes: append([]string(nil), i.Scopes...), Endpoints: i.Endpoints, Callback: i.Callback, Client: ClientRegistration{Method: i.RegistrationMethod, AuthMethod: "none"}}, nil
}

// Resolve never starts an interactive flow. Unknown expiry is not inferred from
// a token's shape. RejectedAccess requests at most one refresh of that token set.
func (s *Service) Resolve(ctx context.Context, store Store, id, resource, rejectedAccess string) (resolved Record, resolveErr error) {
	ctx, sessionCancel := sessionContext(ctx, store)
	defer sessionCancel()
	release, err := s.gate(ctx, id)
	if err != nil {
		return Record{}, err
	}
	defer release()
	r, err := store.Load(ctx, id)
	if err != nil {
		return Record{}, err
	}
	i, err := s.integration(r)
	if err != nil {
		return Record{}, err
	}
	if r.AuthorizationActive() {
		return Record{}, ErrTransient
	}
	if r.Resource != resource {
		return Record{}, ErrResource
	}
	if i.ClientCredentials {
		return s.resolveClientGrant(ctx, store, i, r, rejectedAccess)
	}
	if r.RefreshActive() {
		return Record{}, ErrTransient
	}
	if r.RefreshPending || r.State != "connected" || r.Tokens.Access == "" {
		return Record{}, ErrReauthorize
	}
	if !i.permits(r.GrantedScopes) {
		return Record{}, ErrPermission
	}
	now := time.Now()
	rejected := rejectedAccess != "" && r.Tokens.Access == rejectedAccess
	due := !r.Tokens.ExpiresAt.IsZero() && !now.Before(r.Tokens.ExpiresAt.Add(-time.Minute))
	if !rejected && !due {
		return r, nil
	}
	if r.Tokens.Refresh == "" {
		// A token that cannot refresh remains usable until its actual expiry.
		if !rejected && now.Before(r.Tokens.ExpiresAt) {
			return r, nil
		}
		r.Tokens = Tokens{ID: r.Tokens.ID}
		r.State = "reauthorization_required"
		r.RefreshPending = false
		r.RefreshUntil = time.Time{}
		r.Revision++
		if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
			return Record{}, err
		}
		return Record{}, ErrReauthorize
	}
	if now.Before(r.Tokens.EarliestRefreshAt) {
		if rejected || (!r.Tokens.ExpiresAt.IsZero() && !now.Before(r.Tokens.ExpiresAt)) {
			return Record{}, ErrTransient
		}
		return r, nil
	}
	started := time.Now()
	reason := "expiry"
	if rejected {
		reason = "http_401"
	}
	stage := "prepare_refresh"
	var diagnosticErr error
	defer func() {
		if diagnosticErr == nil {
			diagnosticErr = resolveErr
		}
		logRefreshResult(ctx, r, reason, stage, started, diagnosticErr)
	}()
	if s.BeforeRefresh != nil {
		if err = s.BeforeRefresh(ctx, r); err != nil {
			return Record{}, err
		}
	}
	stage = "persist_pending"
	before := r.Revision
	r.Revision++
	r.RefreshPending = true
	r.RefreshUntil = time.Now().Add(30 * time.Second)
	ctx, refreshCancel := context.WithDeadline(ctx, r.RefreshUntil)
	defer refreshCancel()
	if err = store.CompareAndSwap(ctx, r, before); err != nil {
		return Record{}, err
	}
	outcome := "failure"
	pendingRevision := r.Revision
	defer func() {
		if outcome != "success" {
			finishRefreshAttempt(ctx, store, r.ID, pendingRevision)
		}
	}()
	stage = "exchange"
	response, err := s.exchange(ctx, r, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {r.Tokens.Refresh}, "client_id": {r.Client.ID}, "resource": {r.Resource}})
	if err != nil {
		if errors.Is(err, errRejectedGrant) {
			// Keep the validated identity hint for explicit reconnection to this account.
			r.Tokens = Tokens{ID: r.Tokens.ID}
			r.State = "reauthorization_required"
			r.RefreshPending = false
			r.RefreshUntil = time.Time{}
			r.Revision++
			stage = "persist_rejection"
			if saveErr := store.CompareAndSwap(ctx, r, r.Revision-1); saveErr != nil {
				return Record{}, errors.Join(ErrReauthorize, saveErr)
			}
			stage = "exchange"
			diagnosticErr = err
			return Record{}, ErrReauthorize
		}
		// Keep the durable pending marker on ambiguous network/server responses.
		// The old rotating token must not be sent again, even after a restart.
		return Record{}, errors.Join(ErrReauthorize, err)
	}
	stage = "validate_tokens"
	updated, err := s.applyTokens(ctx, i, r, response, "", false)
	if err != nil {
		return Record{}, errors.Join(ErrReauthorize, err)
	}
	updated.Revision++
	updated.RefreshPending = false
	updated.RefreshUntil = time.Time{}
	stage = "persist_tokens"
	if err = store.CompareAndSwap(ctx, updated, r.Revision); err != nil {
		return Record{}, errors.Join(ErrReauthorize, err)
	}
	outcome = "success"
	stage = "validate_permissions"
	if updated.State != "connected" {
		return Record{}, ErrPermission
	}
	stage = "complete"
	return updated, nil
}

type tokenResponse struct {
	Access    string          `json:"access_token"`
	Refresh   string          `json:"refresh_token"`
	ID        string          `json:"id_token"`
	Type      string          `json:"token_type"`
	ExpiresIn int64           `json:"expires_in"`
	Scope     *string         `json:"scope"`
	Earliest  json.RawMessage `json:"earliest_refresh_at"`
}

func (s *Service) exchange(ctx context.Context, r Record, form url.Values) (response tokenResponse, exchangeErr error) {
	failure := exchangeFailure{category: "request_invalid"}
	defer func() {
		if exchangeErr != nil {
			failure.err = exchangeErr
			exchangeErr = &failure
		}
	}()
	if r.GrantType != "" {
		form.Del("resource")
		if r.Audience != "" {
			form.Set("resource", r.Audience)
		}
	}
	if r.Client.AuthMethod == "client_secret_post" && r.Client.Secret != "" {
		form.Set("client_secret", r.Client.Secret)
	}
	if r.Client.AuthMethod == "client_secret_basic" && r.Client.Secret != "" {
		form.Del("client_id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoints.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, ErrResource
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if r.Client.AuthMethod == "client_secret_basic" && r.Client.Secret != "" {
		req.SetBasicAuth(url.QueryEscape(r.Client.ID), url.QueryEscape(r.Client.Secret))
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		failure.category = exchangeErrorCategory(err, "transport")
		return tokenResponse{}, ErrTransient
	}
	defer func() { _ = resp.Body.Close() }()
	failure.status = resp.StatusCode
	failure.category = "http_rejected"
	if resp.StatusCode != http.StatusOK {
		// Never return error_description: providers may echo credential material.
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&payload)
		failure.code = safeOAuthErrorCode(payload.Error)
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			switch payload.Error {
			case "invalid_client":
				if form.Get("grant_type") == "client_credentials" {
					return tokenResponse{}, ErrClientConfiguration
				}
			case "invalid_scope":
				if form.Get("grant_type") == "client_credentials" {
					return tokenResponse{}, ErrPermission
				}
			case "invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
				return tokenResponse{}, errors.Join(ErrReauthorize, errRejectedGrant)
			}
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return tokenResponse{}, ErrTransient
		}
		return tokenResponse{}, ErrReauthorize
	}
	failure.category = "invalid_response"
	var result tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&result); err != nil {
		failure.category = exchangeErrorCategory(err, "invalid_response")
		return result, ErrReauthorize
	}
	if result.Access == "" || !strings.EqualFold(result.Type, "bearer") || result.ExpiresIn < 0 {
		return result, ErrReauthorize
	}
	return result, nil
}
func (s *Service) applyTokens(ctx context.Context, i Integration, r Record, t tokenResponse, nonce string, initial bool) (Record, error) {
	if initial && !i.IdentityOptional && t.ID == "" {
		return Record{}, ErrReauthorize
	}
	if t.ID != "" && !i.IdentityOptional {
		sub, email, err := verifyIdentity(ctx, s.HTTP, r, t.ID, nonce)
		if err != nil {
			return Record{}, err
		}
		r.Subject = sub
		r.Email = email
		r.Tokens.ID = t.ID
	}
	if t.Scope != nil {
		r.GrantedScopes = strings.Fields(*t.Scope)
	} else if initial {
		// RFC 6749 section 5.1: omitted scope equals the requested scope.
		r.GrantedScopes = append([]string(nil), r.RequestedScopes...)
	}
	r.Tokens.Access = t.Access
	r.Tokens.Type = "Bearer"
	if t.Refresh != "" {
		r.Tokens.Refresh = t.Refresh
	}
	r.Tokens.ExpiresAt = time.Time{}
	if t.ExpiresIn > 0 && t.ExpiresIn <= 315360000 {
		r.Tokens.ExpiresAt = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	}
	r.Tokens.EarliestRefreshAt = time.Time{}
	if len(t.Earliest) > 0 && string(t.Earliest) != "null" {
		var seconds int64
		if json.Unmarshal(t.Earliest, &seconds) == nil {
			r.Tokens.EarliestRefreshAt = time.Unix(seconds, 0)
		} else {
			var stamp string
			if json.Unmarshal(t.Earliest, &stamp) != nil {
				return Record{}, ErrReauthorize
			}
			parsed, err := time.Parse(time.RFC3339, stamp)
			if err != nil {
				return Record{}, ErrReauthorize
			}
			r.Tokens.EarliestRefreshAt = parsed
		}
	}
	r.State = "connected"
	if !i.permits(r.GrantedScopes) {
		r.State = "permission_required"
	}
	return r, nil
}

// Disconnect invalidates the record before contacting the remote service, so
// concurrent refreshes cannot restore it. Registration survives local sign-out.
func (s *Service) Disconnect(ctx context.Context, store Store, id string) (bool, error) {
	ctx, sessionCancel := sessionContext(ctx, store)
	defer sessionCancel()
	release, err := s.gate(ctx, id)
	if err != nil {
		return false, err
	}
	defer release()
	r, err := store.Load(ctx, id)
	if err != nil {
		return false, err
	}
	if _, err = s.integration(r); err != nil {
		return false, err
	}
	if r.AuthorizationActive() {
		return false, ErrConflict
	}
	if r.RefreshActive() {
		return false, ErrTransient
	}
	ambiguousRefresh := r.RefreshPending
	r.AuthorizationAttempt = ""
	r.AuthorizationUntil = time.Time{}
	old := r.Tokens
	// Keep the validated identity hint for explicit reconnection to this account.
	r.Tokens = Tokens{ID: r.Tokens.ID}
	r.RefreshPending = false
	r.RefreshUntil = time.Time{}
	r.State = "disconnected"
	r.Revision++
	if err = store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		return false, err
	}
	token, hint := old.Refresh, "refresh_token"
	if token == "" {
		token, hint = old.Access, "access_token"
	}
	if token == "" {
		return !ambiguousRefresh, nil
	}
	form := url.Values{"token": {token}, "token_type_hint": {hint}, "client_id": {r.Client.ID}}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return false, nil
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoints.Revocation, strings.NewReader(form.Encode()))
		if err != nil {
			return false, nil
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := s.HTTP.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return !ambiguousRefresh, nil
			}
			if resp.StatusCode < 500 {
				return false, nil
			}
		}
	}
	return false, nil
}

// End the live-operation lease after a known local failure while preserving the
// durable ambiguous-rotation marker. A failed vault write still expires safely.
func finishRefreshAttempt(ctx context.Context, store Store, id string, revision uint64) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, err := store.Load(cleanup, id)
	if err != nil || current.Revision != revision || !current.RefreshPending {
		return
	}
	current.RefreshUntil = time.Time{}
	current.Revision++
	_ = store.CompareAndSwap(cleanup, current, revision)
}
