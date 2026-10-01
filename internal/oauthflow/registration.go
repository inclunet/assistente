package oauthflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrRegistration = errors.New("oauth_registration_failed")

// RegistrationRequest describes RFC 7591 metadata. The caller owns callback
// reservation and grant policy; registration never opens a browser or persists secrets.
type RegistrationRequest struct {
	RedirectURIs []string `json:"redirect_uris,omitempty"`
	ClientName   string   `json:"client_name"`
	GrantTypes   []string `json:"grant_types"`
	// Nil keeps the RFC 7591 default (code); an explicit empty slice disables it.
	ResponseTypes           []string `json:"response_types,omitzero"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope,omitempty"`
}

// RegistrationResponse is secret material and must not be exposed in UI DTOs.
type RegistrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

const registrationBodyLimit = 256 * 1024

// RegisterDynamicClient registers at the explicitly selected endpoint, with a
// bounded lifetime and response. Redirects cannot move registration to a new origin.
func RegisterDynamicClient(ctx context.Context, resourceURL, endpoint string, metadata RegistrationRequest) (*RegistrationResponse, error) {
	var result *RegistrationResponse
	var resultErr error
	err := runNetworkOperation(ctx, resourceURL, func(operationCtx context.Context) {
		result, resultErr = registerDynamicClientOnce(operationCtx, resourceURL, endpoint, metadata)
	})
	if err != nil {
		return nil, errors.Join(ErrRegistration, err)
	}
	return result, resultErr
}
func registerDynamicClientOnce(ctx context.Context, resourceURL, endpoint string, metadata RegistrationRequest) (*RegistrationResponse, error) {
	_, err := endpointURL(endpoint)
	if err != nil {
		return nil, ErrRegistration
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, ErrRegistration
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ErrRegistration
	}
	request.Header.Set("Content-Type", "application/json")
	client := NewHTTPClient()
	ctx = withDiscoveryNetwork(ctx, resourceURL)
	client.Transport = discoveryRoundTripper{policy: ctx.Value(discoveryNetworkKey{}).(*discoveryNetwork)}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Join(ErrRegistration, ctx.Err())
		}
		return nil, errors.Join(ErrRegistration, ErrTransient)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrRegistration, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, registrationBodyLimit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.Join(ErrRegistration, ctx.Err())
		}
		return nil, errors.Join(ErrRegistration, ErrTransient)
	}
	if len(raw) > registrationBodyLimit {
		return nil, ErrRegistration
	}
	var registration RegistrationResponse
	if json.Unmarshal(raw, &registration) != nil || strings.TrimSpace(registration.ClientID) == "" {
		return nil, ErrRegistration
	}
	return &registration, nil
}
