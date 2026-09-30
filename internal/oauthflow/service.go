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

	"assistente/internal/logging"
)

// An explicit invalid-grant response prevents access/refresh token reuse.
var errRejectedGrant = errors.New("oauth_grant_rejected")

type Service struct {
	HTTP         *http.Client
	integrations map[string]Integration
	gates        sync.Map
}

func New(integrations ...Integration) *Service {
	s := &Service{HTTP: NewHTTPClient(), integrations: make(map[string]Integration)}
	for _, i := range integrations {
		s.integrations[i.ID] = i
	}
	return s
}
func (s *Service) gate(ctx context.Context, id string) (func(), error) {
	value, _ := s.gates.LoadOrStore(id, make(chan struct{}, 1))
	ch := value.(chan struct{})
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
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
func (s *Service) Resolve(ctx context.Context, store Store, id, resource, rejectedAccess string) (Record, error) {
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
	if r.Resource != resource {
		return Record{}, ErrResource
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
	if now.Before(r.Tokens.EarliestRefreshAt) {
		if rejected || (!r.Tokens.ExpiresAt.IsZero() && !now.Before(r.Tokens.ExpiresAt)) {
			return Record{}, ErrTransient
		}
		return r, nil
	}
	if r.Tokens.Refresh == "" {
		return Record{}, ErrReauthorize
	}
	before := r.Revision
	r.Revision++
	r.RefreshPending = true
	if err = store.CompareAndSwap(ctx, r, before); err != nil {
		return Record{}, err
	}
	started := time.Now()
	reason := "expiry"
	if rejected {
		reason = "http_401"
	}
	outcome := "failure"
	defer func() {
		logging.Infof(ctx, "oauthflow", "oauth_refresh credential_id=%s integration=%s reason=%s duration_ms=%d outcome=%s", r.ID, r.Integration, reason, time.Since(started).Milliseconds(), outcome)
	}()
	response, err := s.exchange(ctx, r, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {r.Tokens.Refresh}, "client_id": {r.Client.ID}, "resource": {r.Resource}})
	if err != nil {
		if errors.Is(err, errRejectedGrant) {
			// Keep the validated identity hint for explicit reconnection to this account.
			r.Tokens = Tokens{ID: r.Tokens.ID}
			r.State = "reauthorization_required"
			r.RefreshPending = false
			r.Revision++
			if saveErr := store.CompareAndSwap(ctx, r, r.Revision-1); saveErr != nil {
				return Record{}, saveErr
			}
			return Record{}, ErrReauthorize
		}
		// Keep the durable pending marker on ambiguous network/server responses.
		// The old rotating token must not be sent again, even after a restart.
		return Record{}, err
	}
	updated, err := s.applyTokens(ctx, i, r, response, "", false)
	if err != nil {
		return Record{}, err
	}
	updated.Revision++
	updated.RefreshPending = false
	if err = store.CompareAndSwap(ctx, updated, r.Revision); err != nil {
		return Record{}, err
	}
	outcome = "success"
	if updated.State != "connected" {
		return Record{}, ErrPermission
	}
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

func (s *Service) exchange(ctx context.Context, r Record, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoints.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, ErrResource
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return tokenResponse{}, ErrTransient
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// Never return error_description: providers may echo credential material.
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&failure)
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			switch failure.Error {
			case "invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
				return tokenResponse{}, errors.Join(ErrReauthorize, errRejectedGrant)
			}
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return tokenResponse{}, ErrTransient
		}
		return tokenResponse{}, ErrReauthorize
	}
	var result tokenResponse
	if json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&result) != nil {
		return result, ErrReauthorize
	}
	if result.Access == "" || !strings.EqualFold(result.Type, "bearer") || result.ExpiresIn < 0 {
		return result, ErrReauthorize
	}
	return result, nil
}
func (s *Service) applyTokens(ctx context.Context, i Integration, r Record, t tokenResponse, nonce string, initial bool) (Record, error) {
	if initial && (t.ID == "" || t.Scope == nil) {
		return Record{}, ErrReauthorize
	}
	if t.ID != "" {
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
	old := r.Tokens
	// Keep the validated identity hint for explicit reconnection to this account.
	r.Tokens = Tokens{ID: r.Tokens.ID}
	r.RefreshPending = false
	r.State = "disconnected"
	r.Revision++
	if err = store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		return false, err
	}
	if old.Refresh == "" {
		return true, nil
	}
	form := url.Values{"token": {old.Refresh}, "token_type_hint": {"refresh_token"}, "client_id": {r.Client.ID}}
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
				return true, nil
			}
			if resp.StatusCode < 500 {
				return false, nil
			}
		}
	}
	return false, nil
}
