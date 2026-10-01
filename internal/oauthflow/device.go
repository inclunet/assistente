package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrDeviceGrant = errors.New("oauth_device_grant_failed")

// DeviceGrantConfig contains only OAuth protocol parameters. The consumer owns
// presentation and persistence until its authorization records are migrated.
type DeviceGrantConfig struct {
	Resource, Audience, ClientID, DeviceEndpoint, TokenEndpoint string
	ClientSecret, AuthMethod                                    string
	Scopes                                                      []string
	AuthorizeNetwork                                            NetworkAuthorizer
}
type DeviceVerification struct{ UserCode, URL, BaseURL string }
type DeviceGrantResult struct {
	Token  *oauth2.Token
	Scopes []string
}
type deviceResponse struct {
	Error                   string `json:"error"`
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}
type deviceTokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"`
	Scope        *string `json:"scope"`
	Error        string  `json:"error"`
}

// DeviceGrantError exposes only allowlisted protocol codes, never response bodies.
type DeviceGrantError struct{ Code string }

func (e *DeviceGrantError) Error() string { return "oauth_device_grant_failed: " + e.Code }
func (e *DeviceGrantError) Unwrap() error { return ErrDeviceGrant }
func deviceFailure(code string) error {
	switch code {
	case "authorization_pending", "slow_down", "access_denied", "expired_token", "unauthorized_client", "invalid_client", "invalid_grant":
		return &DeviceGrantError{Code: code}
	default:
		return ErrDeviceGrant
	}
}
func DeviceGrantErrorCode(err error) string {
	var grant *DeviceGrantError
	if errors.As(err, &grant) {
		return grant.Code
	}
	return ""
}

func deviceJSON(ctx context.Context, client *http.Client, endpoint string, form url.Values, cfg DeviceGrantConfig, out any) (int, error) {
	if cfg.ClientSecret != "" {
		switch cfg.AuthMethod {
		case "client_secret_post":
			form.Set("client_secret", cfg.ClientSecret)
		case "client_secret_basic":
			form.Del("client_id")
		default:
			return 0, ErrDeviceGrant
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, ErrDeviceGrant
	}
	if cfg.ClientSecret != "" && cfg.AuthMethod == "client_secret_basic" {
		req.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape(cfg.ClientSecret))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.Join(ErrDeviceGrant, safeDeviceTransportError(ctx, err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil {
		return resp.StatusCode, errors.Join(ErrDeviceGrant, safeDeviceTransportError(ctx, err))
	}
	if len(body) > 64*1024 {
		return resp.StatusCode, ErrDeviceGrant
	}
	if json.Unmarshal(body, out) != nil {
		return resp.StatusCode, ErrDeviceGrant
	}
	return resp.StatusCode, nil
}
func safeDeviceTransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrNetworkAuthorization) {
		return ErrNetworkAuthorization
	}
	// A per-request timeout does not mean the device authorization expired.
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTransient
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return ErrTransient
}
func waitDevicePoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// AuthorizeDevice implements RFC 8628 without callbacks/listeners or persistence.
// Verification presentation is supplied by the consumer after destination consent.
func AuthorizeDevice(ctx context.Context, cfg DeviceGrantConfig, verify func(context.Context, DeviceVerification) error) (DeviceGrantResult, error) {
	result, err := authorizeDevice(ctx, cfg, verify, waitDevicePoll)
	if err != nil && ctx.Err() != nil {
		return result, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return result, errors.Join(deviceFailure("expired_token"), context.DeadlineExceeded)
	}
	return result, err
}
func authorizeDevice(parent context.Context, cfg DeviceGrantConfig, verify func(context.Context, DeviceVerification) error, wait func(context.Context, time.Duration) error) (DeviceGrantResult, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	if ctx.Value(networkOperationKey{}) == nil {
		ctx = WithNetworkOperation(ctx)
	}
	ctx = WithNetworkAuthorizer(ctx, cfg.AuthorizeNetwork)
	if cfg.ClientID == "" || verify == nil {
		return DeviceGrantResult{}, ErrDeviceGrant
	}
	client := NewNetworkHTTPClient(cfg.Resource, cfg.AuthorizeNetwork, 5*time.Second)
	form := url.Values{"client_id": {cfg.ClientID}}
	if cfg.Audience != "" {
		form.Set("resource", cfg.Audience)
	}
	if len(cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	var device deviceResponse
	status, err := deviceJSON(ctx, client, cfg.DeviceEndpoint, form, cfg, &device)
	if err != nil {
		return DeviceGrantResult{}, err
	}
	if status < 200 || status >= 300 || device.Error != "" {
		return DeviceGrantResult{}, deviceFailure(device.Error)
	}
	verifyURL := device.VerificationURIComplete
	if verifyURL == "" {
		verifyURL = device.VerificationURI
	}
	target, err := endpointURL(verifyURL)
	if err != nil || device.DeviceCode == "" || device.UserCode == "" || device.ExpiresIn <= 0 || device.Interval < 0 {
		return DeviceGrantResult{}, ErrDeviceGrant
	}
	lifetime := device.ExpiresIn
	if lifetime > 600 {
		lifetime = 600
	}
	ctx, expiryCancel := context.WithTimeout(ctx, time.Duration(lifetime)*time.Second)
	defer expiryCancel()
	// Validate the browser destination without issuing a request carrying its code.
	var destinationErr error
	err = runNetworkOperation(ctx, cfg.Resource, func(operationCtx context.Context) {
		checkCtx, done := context.WithTimeout(operationCtx, 5*time.Second)
		defer done()
		destinationErr = operationCtx.Value(discoveryNetworkKey{}).(*discoveryNetwork).checkDestination(checkCtx, target)
	})
	if err != nil {
		return DeviceGrantResult{}, err
	}
	if destinationErr != nil {
		return DeviceGrantResult{}, safeDeviceTransportError(ctx, destinationErr)
	}
	if err := verify(ctx, DeviceVerification{UserCode: device.UserCode, URL: verifyURL, BaseURL: device.VerificationURI}); err != nil {
		return DeviceGrantResult{}, err
	}
	interval := device.Interval
	if interval == 0 {
		interval = 5
	}
	if interval > 600 {
		interval = 600
	}
	for {
		if err := wait(ctx, time.Duration(interval)*time.Second); err != nil {
			return DeviceGrantResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return DeviceGrantResult{}, err
		}
		form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {cfg.ClientID}, "device_code": {device.DeviceCode}}
		if cfg.Audience != "" {
			form.Set("resource", cfg.Audience)
		}
		var token deviceTokenResponse
		status, err := deviceJSON(ctx, client, cfg.TokenEndpoint, form, cfg, &token)
		if err != nil {
			return DeviceGrantResult{}, err
		}
		// Providers in the field may return protocol errors in HTTP 200 or 400.
		if token.Error != "" {
			if status != 200 && status != 400 {
				return DeviceGrantResult{}, deviceFailure(token.Error)
			}
			switch token.Error {
			case "authorization_pending":
				continue
			case "slow_down":
				if interval < 600 {
					interval = min(interval+5, 600)
				}
				continue
			default:
				return DeviceGrantResult{}, deviceFailure(token.Error)
			}
		}
		if status < 200 || status >= 300 || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") || token.ExpiresIn < 0 || token.ExpiresIn > int64((1<<63-1)/int64(time.Second)) {
			return DeviceGrantResult{}, ErrDeviceGrant
		}
		result := &oauth2.Token{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType}
		if token.ExpiresIn > 0 {
			result.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		}
		scopes := append([]string(nil), cfg.Scopes...)
		if token.Scope != nil {
			scopes = strings.Fields(*token.Scope)
		}
		return DeviceGrantResult{Token: result, Scopes: scopes}, nil
	}
}
