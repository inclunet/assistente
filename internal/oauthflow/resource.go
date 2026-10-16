package oauthflow

import (
	"fmt"
	"net/http"
	"net/url"
)

// A network approval does not change the audience of an OAuth access token.
var ErrResourceDestination = fmt.Errorf("oauth_resource_destination_blocked: %w", ErrNetworkAuthorization)

func validateResourceDestination(resource string, target *url.URL) error {
	configured, err := endpointURL(resource)
	if err != nil || target == nil {
		return ErrResourceDestination
	}
	if _, err := endpointURL(target.String()); err != nil || networkOrigin(configured) != networkOrigin(target) {
		return ErrResourceDestination
	}
	return nil
}

// NewResourceHTTPClient validates the destination before invoking authentication,
// including requests originating from SSE endpoint events, not just redirects.
// Only paths on the configured origin can receive this resource's credentials.
func NewResourceHTTPClient(resource string, authenticated http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: resourceOriginTransport{resource: resource, next: authenticated},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return ErrResourceDestination
			}
			return validateResourceDestination(resource, request.URL)
		},
	}
}

type resourceOriginTransport struct {
	resource string
	next     http.RoundTripper
}

func (t resourceOriginTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := validateResourceDestination(t.resource, request.URL); err != nil {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(request)
}

// NewResourceTransport reuses endpoint consent and the actual-socket guard, but
// does not impose an OAuth response-body deadline on long-lived resource streams.
func NewResourceTransport(resource string, authorize NetworkAuthorizer) http.RoundTripper {
	return &authorizedOAuthTransport{resource: resource, authorize: authorize, resourceOnly: true}
}
