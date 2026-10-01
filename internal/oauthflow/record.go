// Package oauthflow owns the authorization lifecycle, independently of its consumers.
package oauthflow

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("oauth_authorization_not_found")
	ErrReauthorize = errors.New("oauth_reauthorization_required")
	ErrConflict    = errors.New("oauth_authorization_changed")
	ErrPermission  = errors.New("oauth_permission_missing")
	ErrResource    = errors.New("oauth_resource_not_authorized")
	ErrTransient   = errors.New("oauth_temporarily_unavailable")
)

// Record is secret material: only the vault may serialize it for persistence.
// UI contracts must use Summary, never Record.
type Record struct {
	PendingRegistration *RegistrationCandidate `json:"pendingRegistration,omitempty"`
	ConsumerID          string                 `json:"consumerId,omitempty"`
	GrantType           string                 `json:"grantType,omitempty"`
	Audience            string                 `json:"audience,omitempty"`
	Version             int                    `json:"version"`
	ID                  string                 `json:"id"`
	UserID              string                 `json:"userId"`
	Integration         string                 `json:"integration"`
	Revision            uint64                 `json:"revision"`
	State               string                 `json:"state"`
	Issuer              string                 `json:"issuer"`
	Resource            string                 `json:"resource"`
	RequestedScopes     []string               `json:"requestedScopes"`
	GrantedScopes       []string               `json:"grantedScopes"`
	Client              ClientRegistration     `json:"client"`
	Endpoints           Endpoints              `json:"endpoints"`
	Callback            CallbackConfig         `json:"callback"`
	Subject             string                 `json:"subject,omitempty"`
	Email               string                 `json:"email,omitempty"`
	Tokens              Tokens                 `json:"tokens"`
	// RefreshPending is persisted BEFORE sending a potentially rotating refresh.
	// A crash or ambiguous response must never cause the old token to be reused.
	RefreshPending       bool      `json:"refreshPending,omitempty"`
	RefreshUntil         time.Time `json:"refreshUntil,omitempty"`
	AuthorizationAttempt string    `json:"authorizationAttempt,omitempty"`
	AuthorizationUntil   time.Time `json:"authorizationUntil,omitempty"`
}

// RegistrationCandidate belongs to an unfinished authorization, never to the
// active token grant. It is encrypted in the same vault record.
type RegistrationCandidate struct {
	Client          ClientRegistration `json:"client"`
	Endpoints       Endpoints          `json:"endpoints"`
	Callback        CallbackConfig     `json:"callback"`
	Audience        string             `json:"audience"`
	RequestedScopes []string           `json:"requestedScopes"`
}

type ClientRegistration struct {
	GrantType  string `json:"grantType,omitempty"`
	Method     string `json:"method"`
	ID         string `json:"id"`
	Secret     string `json:"secret,omitempty"`
	AuthMethod string `json:"authMethod"`
}
type Endpoints struct {
	Authorization string `json:"authorization"`
	Registration  string `json:"registration,omitempty"`
	Device        string `json:"device,omitempty"`
	Token         string `json:"token"`
	Revocation    string `json:"revocation"`
	JWKS          string `json:"jwks"`
}
type CallbackConfig struct {
	Host       string `json:"host"`
	Path       string `json:"path"`
	Port       int    `json:"port"`
	PortPolicy string `json:"portPolicy"`
}
type Tokens struct {
	Access            string    `json:"access,omitempty"`
	Refresh           string    `json:"refresh,omitempty"`
	ID                string    `json:"id,omitempty"`
	Type              string    `json:"type,omitempty"`
	ExpiresAt         time.Time `json:"expiresAt,omitempty"`
	EarliestRefreshAt time.Time `json:"earliestRefreshAt,omitempty"`
}
type Summary struct {
	ID              string `json:"id"`
	Integration     string `json:"integration"`
	State           string `json:"state"`
	Email           string `json:"email,omitempty"`
	HasRefreshToken bool   `json:"hasRefreshToken"`
}

func (r Record) Summary() Summary {
	state := r.State
	if r.RefreshActive() {
		state = "refreshing"
	} else if r.RefreshPending {
		state = "reauthorization_required"
	}
	return Summary{r.ID, r.Integration, state, r.Email, r.Tokens.Refresh != ""}
}

// Store is scoped to one authenticated local user and one session generation.
// CompareAndSwap must atomically check revision/existence and persist all fields.
// It must never recreate a deleted record. No network IO runs under a vault lock.
type Store interface {
	Load(context.Context, string) (Record, error)
	Create(context.Context, Record) error
	CompareAndSwap(context.Context, Record, uint64) error
}

// AuthorizationActive fences interactive consent across processes sharing a vault.
func (r Record) AuthorizationActive() bool {
	return r.AuthorizationAttempt != "" && time.Now().Before(r.AuthorizationUntil)
}

func (r Record) RefreshActive() bool {
	return r.RefreshPending && time.Now().Before(r.RefreshUntil)
}
