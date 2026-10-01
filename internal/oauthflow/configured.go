package oauthflow

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

var configuredGates Service

// NewConfigured constructs a lifecycle for an encrypted, consumer-bound OAuth2
// record. Its caller must validate the consumer and resource before resolving it.
// OIDC integrations continue using New and their fixed identity contract.
func NewConfigured(r Record, authorize NetworkAuthorizer) (*Service, error) {
	if r.GrantType != "authorization_code" && r.GrantType != "client_credentials" {
		return nil, ErrResource
	}
	if r.Integration == "" || r.ConsumerID == "" {
		return nil, ErrResource
	}
	if _, err := endpointURL(r.Resource); err != nil {
		return nil, ErrResource
	}
	for _, endpoint := range []string{r.Endpoints.Token, r.Endpoints.Authorization, r.Endpoints.Device, r.Endpoints.Registration} {
		if endpoint != "" {
			if _, err := endpointURL(endpoint); err != nil {
				return nil, ErrResource
			}
		}
	}
	switch r.Client.AuthMethod {
	case "none", "client_secret_post", "client_secret_basic":
	default:
		return nil, ErrResource
	}
	s := New(Integration{ID: r.Integration, Issuer: r.Issuer, Resource: r.Resource, Endpoints: r.Endpoints, Callback: r.Callback, GrantType: r.GrantType, ConsumerID: r.ConsumerID, Scopes: append([]string(nil), r.RequestedScopes...), RequiredScopes: append([]string(nil), r.RequestedScopes...), IdentityOptional: true, ClientCredentials: r.GrantType == "client_credentials"})
	s.gateOwner = &configuredGates
	s.BeforeRefresh = func(ctx context.Context, r Record) error {
		target, err := endpointURL(r.Endpoints.Token)
		if err != nil {
			return ErrResource
		}
		var checkErr error
		err = runNetworkOperation(WithNetworkAuthorizer(ctx, authorize), r.Resource, func(op context.Context) {
			bounded, cancel := context.WithTimeout(op, 5*time.Second)
			defer cancel()
			checkErr = op.Value(discoveryNetworkKey{}).(*discoveryNetwork).checkDestination(bounded, target)
		})
		if err != nil {
			return err
		}
		return checkErr
	}
	s.HTTP = NewNetworkHTTPClient(r.Resource, authorize, 30*time.Second)
	return s, nil
}

// AuthorizeUsing owns the durable lease and commit around a protocol adapter.
// The adapter must arbitrate interactive presentation, return secret material only
// to this method, and never persist credentials or install a background refresher.
// A failed attempt leaves the previous authorization intact.
func (s *Service) AuthorizeUsing(ctx context.Context, store Store, id string, grant func(context.Context, Record) (Record, error)) (Summary, error) {
	if grant == nil {
		return Summary{}, ErrResource
	}
	return s.AuthorizeUsingCheckpoint(ctx, store, id, func(ctx context.Context, r Record, _ func(Record) (Record, error)) (Record, error) {
		return grant(ctx, r)
	})
}

// AuthorizeUsingCheckpoint lets the service persist an issued registration before
// user consent completes. Only registration metadata changes; prior tokens remain.
func (s *Service) AuthorizeUsingCheckpoint(ctx context.Context, store Store, id string, grant func(context.Context, Record, func(Record) (Record, error)) (Record, error)) (Summary, error) {
	ctx, cancelSession := sessionContext(ctx, store)
	defer cancelSession()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	release, err := s.gate(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	defer release()
	r, err := store.Load(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	i, err := s.integration(r)
	if err != nil || !i.IdentityOptional || grant == nil {
		return Summary{}, ErrResource
	}
	if r.AuthorizationActive() || r.RefreshActive() {
		return Summary{}, ErrConflict
	}
	r.AuthorizationAttempt = randomValue()
	r.AuthorizationUntil, _ = ctx.Deadline()
	r.Revision++
	if err = store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		return Summary{}, err
	}
	defer clearAuthorizationAttempt(ctx, store, id, r.AuthorizationAttempt)
	withCandidate := func(base Record) Record {
		if candidate := base.PendingRegistration; candidate != nil {
			base.Client, base.Endpoints, base.Callback = candidate.Client, candidate.Endpoints, candidate.Callback
			base.Audience = candidate.Audience
			base.RequestedScopes = append([]string(nil), candidate.RequestedScopes...)
		}
		base.PendingRegistration = nil
		return base
	}
	checkpoint := func(registration Record) (Record, error) {
		if err := checkAuthorizationAttempt(ctx, store, r); err != nil {
			return Record{}, err
		}
		next := r
		next.PendingRegistration = &RegistrationCandidate{
			Client: registration.Client, Endpoints: registration.Endpoints, Callback: registration.Callback,
			Audience: registration.Audience, RequestedScopes: append([]string(nil), registration.RequestedScopes...),
		}
		candidate := withCandidate(next)
		if candidate.Client.ID == "" || candidate.Client.Method != "dcr" {
			return Record{}, ErrResource
		}
		if _, err := NewConfigured(candidate, nil); err != nil {
			return Record{}, err
		}
		next.Revision++
		if err := store.CompareAndSwap(ctx, next, r.Revision); err != nil {
			return Record{}, err
		}
		r = next
		return withCandidate(r), nil
	}
	updated, err := grant(ctx, withCandidate(r), checkpoint)
	if err != nil {
		return Summary{}, err
	}
	if updated.ID != r.ID || updated.UserID != r.UserID || updated.Integration != r.Integration || updated.ConsumerID != r.ConsumerID || updated.Resource != r.Resource || updated.GrantType != r.GrantType || updated.Revision != r.Revision || updated.Tokens.Access == "" || !strings.EqualFold(updated.Tokens.Type, "Bearer") {
		return Summary{}, ErrResource
	}
	if _, err = NewConfigured(updated, nil); err != nil {
		return Summary{}, err
	}
	if !(Integration{RequiredScopes: updated.RequestedScopes}).permits(updated.GrantedScopes) {
		return Summary{}, ErrPermission
	}
	if err = checkAuthorizationAttempt(ctx, store, r); err != nil {
		return Summary{}, err
	}
	updated.PendingRegistration = nil
	updated.State = "connected"
	updated.AuthorizationAttempt = ""
	updated.AuthorizationUntil = time.Time{}
	updated.RefreshPending = false
	updated.RefreshUntil = time.Time{}
	updated.Revision++
	if err = store.CompareAndSwap(ctx, updated, r.Revision); err != nil {
		return Summary{}, err
	}
	return updated.Summary(), nil
}

// Client credentials obtains a new grant, never reuses a rotating refresh token.
// The durable lease still serializes processes; after a crash it can safely retry.
func (s *Service) resolveClientGrant(ctx context.Context, store Store, i Integration, r Record, rejected string) (Record, error) {
	if r.State == "permission_required" {
		return Record{}, ErrPermission
	}
	if r.State == "disconnected" {
		return Record{}, ErrReauthorize
	}
	if r.RefreshActive() {
		return Record{}, ErrTransient
	}
	if r.Client.ID == "" || r.Client.Secret == "" || r.Endpoints.Token == "" {
		return Record{}, ErrReauthorize
	}
	if !r.RefreshPending && r.State == "connected" && r.Tokens.Access != "" && r.Tokens.Access != rejected && (r.Tokens.ExpiresAt.IsZero() || time.Now().Before(r.Tokens.ExpiresAt.Add(-time.Minute))) {
		return r, nil
	}
	if s.BeforeRefresh != nil {
		if err := s.BeforeRefresh(ctx, r); err != nil {
			return Record{}, err
		}
	}
	r.RefreshPending = true
	r.RefreshUntil = time.Now().Add(30 * time.Second)
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		return Record{}, err
	}
	before := r.Revision
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {r.Client.ID}}
	if len(r.RequestedScopes) != 0 {
		form.Set("scope", strings.Join(r.RequestedScopes, " "))
	}
	response, err := s.exchange(ctx, r, form)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		r.RefreshPending = false
		r.RefreshUntil = time.Time{}
		if errors.Is(err, ErrPermission) {
			r.State = "permission_required"
			r.Tokens = Tokens{}
			r.GrantedScopes = nil
		}
		r.Revision++
		return Record{}, errors.Join(err, store.CompareAndSwap(cleanup, r, before))
	}
	updated, err := s.applyTokens(ctx, i, r, response, "", true)
	if err != nil {
		return Record{}, err
	}
	updated.Tokens.Refresh = ""
	updated.RefreshPending = false
	updated.RefreshUntil = time.Time{}
	updated.Revision++
	if err = store.CompareAndSwap(ctx, updated, before); err != nil {
		return Record{}, err
	}
	if updated.State != "connected" {
		return Record{}, ErrPermission
	}
	return updated, nil
}

// Invalidate is local sign-out. It refuses active leases without claiming remote
// revocation and is separate from Disconnect's optional revocation request.
func (s *Service) Invalidate(ctx context.Context, store Store, id string) error {
	return s.invalidate(ctx, store, id, false)
}

// InvalidateAndClearClientSecret removes grant and client secret in one revision.
func (s *Service) InvalidateAndClearClientSecret(ctx context.Context, store Store, id string) error {
	return s.invalidate(ctx, store, id, true)
}

func (s *Service) invalidate(ctx context.Context, store Store, id string, clearSecret bool) error {
	r, err := store.Load(ctx, id)
	if err != nil {
		return err
	}
	if _, err = s.integration(r); err != nil {
		return err
	}
	if r.AuthorizationActive() || r.RefreshActive() {
		return ErrConflict
	}
	r.PendingRegistration = nil
	r.State = "disconnected"
	if clearSecret {
		r.Client.Secret = ""
	}
	r.Tokens = Tokens{}
	r.RefreshPending = false
	r.RefreshUntil = time.Time{}
	r.AuthorizationAttempt = ""
	r.AuthorizationUntil = time.Time{}
	r.Revision++
	return store.CompareAndSwap(ctx, r, r.Revision-1)
}
