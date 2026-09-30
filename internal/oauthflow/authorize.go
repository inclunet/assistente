package oauthflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
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
	if hostID == "" || openBrowser == nil {
		return Summary{}, ErrResource
	}
	if i.Callback.Host != "127.0.0.1" || !strings.HasPrefix(i.Callback.Path, "/") {
		return Summary{}, ErrResource
	}
	port := i.Callback.Port
	if i.Callback.PortPolicy == "ephemeral" {
		port = 0
	} else if i.Callback.PortPolicy != "fixed" || port < 1 || port > 65535 {
		return Summary{}, ErrResource
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(i.Callback.Host, strconv.Itoa(port)))
	if err != nil {
		return Summary{}, errors.New("oauth_callback_port_unavailable")
	}
	defer func() { _ = listener.Close() }()
	redirect := "http://" + listener.Addr().String() + i.Callback.Path
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
	type callback struct{ values url.Values }
	results := make(chan callback, 1)
	var consumed atomic.Bool
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		if req.Method != http.MethodGet || req.Host != listener.Addr().String() || req.URL.Path != i.Callback.Path {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		v, parseErr := url.ParseQuery(req.URL.RawQuery)
		if parseErr != nil || len(v["state"]) != 1 || subtle.ConstantTimeCompare([]byte(v.Get("state")), []byte(state)) != 1 {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		for _, key := range []string{"code", "error", "client_id"} {
			if len(v[key]) > 1 {
				http.Error(w, "", http.StatusBadRequest)
				return
			}
		}
		if !consumed.CompareAndSwap(false, true) {
			http.Error(w, "", http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(completionText)))
		_, _ = w.Write([]byte(completionText))
		_ = http.NewResponseController(w).Flush()
		select {
		case results <- callback{v}:
		default:
		}
	})}
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()
	if err = ctx.Err(); err != nil {
		return Summary{}, err
	}
	if err = openBrowser(i.Endpoints.Authorization + "?" + values.Encode()); err != nil {
		return Summary{}, errors.New("oauth_browser_unavailable")
	}
	var result callback
	select {
	case <-ctx.Done():
		return Summary{}, ctx.Err()
	case result = <-results:
	}
	if result.values.Get("error") != "" || result.values.Get("code") == "" {
		return Summary{}, errors.New("oauth_consent_declined")
	}
	clientID := r.Client.ID
	if i.CallbackClientID != nil {
		clientID, err = i.CallbackClientID(r, result.values)
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
	response, err := s.exchange(ctx, r, url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {result.values.Get("code")}, "code_verifier": {verifier}, "redirect_uri": {redirect}, "resource": {r.Resource}})
	if err != nil {
		return Summary{}, err
	}
	updated, err := s.applyTokens(ctx, i, r, response, nonce, true)
	if err != nil {
		return Summary{}, err
	}
	updated.Revision++
	updated.RefreshPending = false
	if err = store.CompareAndSwap(ctx, updated, r.Revision); err != nil {
		return Summary{}, err
	}
	return updated.Summary(), nil
}
