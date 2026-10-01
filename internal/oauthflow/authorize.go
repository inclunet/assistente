package oauthflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"
)

func randomValue() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Authorize is invoked only by an explicit connection action. The listener is
// reserved before opening the system browser and remains reserved through the
// exchange. Callback values and authorization URLs must never be logged.
func (s *Service) Authorize(ctx context.Context, store Store, id, hostID string, openBrowser func(string) error, completionText string) (Summary, error) {
	ctx, sessionCancel := sessionContext(ctx, store)
	defer sessionCancel()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	release, err := s.gate(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	defer release()
	interactiveRelease, err := Interactive.Acquire(ctx)
	if err != nil {
		return Summary{}, err
	}
	defer interactiveRelease()
	r, err := store.Load(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	i, err := s.integration(r)
	if err != nil {
		return Summary{}, err
	}
	if r.AuthorizationActive() || r.RefreshActive() {
		return Summary{}, ErrConflict
	}
	previous := r
	if hostID == "" || openBrowser == nil {
		return Summary{}, ErrResource
	}
	callback, err := ReserveCallback(i.Callback)
	if err != nil {
		return Summary{}, err
	}
	defer callback.Close()
	redirect := callback.RedirectURI()
	state, nonce, verifier := randomValue(), randomValue(), randomValue()
	digest := sha256.Sum256([]byte(verifier))
	values := url.Values{"response_type": {"code"}, "client_id": {r.Client.ID}, "redirect_uri": {redirect}, "scope": {strings.Join(i.Scopes, " ")}, "resource": {i.Resource}, "state": {state}, "nonce": {nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
	if r.Client.ID == "" {
		values.Set("client_id", i.InitialClientID)
	}
	if r.State == "permission_required" {
		values.Set("prompt", "consent")
	}
	if i.AuthorizationParameters != nil {
		for key, v := range i.AuthorizationParameters(r, hostID) {
			values[key] = v
		}
	}
	// Record exactly what this consent requested, including integration overrides.
	r.RequestedScopes = strings.Fields(values.Get("scope"))
	if err = callback.Start(state, CallbackPage{Success: completionText, Failure: completionText}); err != nil {
		return Summary{}, err
	}
	if err = ctx.Err(); err != nil {
		return Summary{}, err
	}
	// Persist ownership before any browser interaction; another process must not
	// delete this disconnected grant or begin a second consent concurrently.
	r.AuthorizationAttempt = randomValue()
	r.AuthorizationUntil, _ = ctx.Deadline()
	r.Revision++
	if err = store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		return Summary{}, err
	}
	attempt := r.AuthorizationAttempt
	defer clearAuthorizationAttempt(ctx, store, id, attempt)
	if err = openBrowser(i.Endpoints.Authorization + "?" + values.Encode()); err != nil {
		return Summary{}, errors.New("oauth_browser_unavailable")
	}
	result, err := callback.Wait(ctx)
	if err != nil {
		return Summary{}, err
	}
	if result.Get("error") != "" || result.Get("code") == "" {
		return Summary{}, errors.New("oauth_consent_declined")
	}
	clientID := r.Client.ID
	if i.CallbackClientID != nil {
		clientID, err = i.CallbackClientID(r, result)
		if err != nil {
			return Summary{}, err
		}
	}
	if clientID == "" {
		return Summary{}, ErrReauthorize
	}
	if r.Client.ID == "" {
		// Persist issued registration before exchange, so invalid_grant can be
		// recovered without creating a second registration on the server.
		r.Client.ID = clientID
		r.Revision++
		if err = store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
			return Summary{}, err
		}
	}
	if err = checkAuthorizationAttempt(ctx, store, r); err != nil {
		return Summary{}, err
	}
	response, err := s.exchange(ctx, r, url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {result.Get("code")}, "code_verifier": {verifier}, "redirect_uri": {redirect}, "resource": {r.Resource}})
	if err != nil {
		return Summary{}, err
	}
	updated, err := s.applyTokens(ctx, i, r, response, nonce, true)
	if err != nil {
		return Summary{}, err
	}
	if updated.State != "connected" && previous.State == "connected" && previous.Tokens.Access != "" && !previous.RefreshPending && i.permits(previous.GrantedScopes) {
		return previous.Summary(), ErrPermission
	}
	if err = checkAuthorizationAttempt(ctx, store, r); err != nil {
		return Summary{}, err
	}
	updated.AuthorizationAttempt = ""
	updated.AuthorizationUntil = time.Time{}
	updated.Revision++
	updated.RefreshPending = false
	updated.RefreshUntil = time.Time{}
	if err = store.CompareAndSwap(ctx, updated, r.Revision); err != nil {
		return Summary{}, err
	}
	return updated.Summary(), nil
}

func checkAuthorizationAttempt(ctx context.Context, store Store, expected Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := store.Load(ctx, expected.ID)
	if err != nil {
		return err
	}
	if current.AuthorizationAttempt != expected.AuthorizationAttempt || !current.AuthorizationActive() || current.Revision != expected.Revision {
		return ErrConflict
	}
	return nil
}

func clearAuthorizationAttempt(ctx context.Context, store Store, id, attempt string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, err := store.Load(cleanup, id)
	if err != nil || current.AuthorizationAttempt != attempt {
		return
	}
	current.AuthorizationAttempt = ""
	current.AuthorizationUntil = time.Time{}
	current.Revision++
	// Session validation still belongs to Store. Failed cleanup expires naturally.
	_ = store.CompareAndSwap(cleanup, current, current.Revision-1)
}
