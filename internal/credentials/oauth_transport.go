package credentials

import (
	"context"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/url"
	"strings"

	"assistente/internal/oauthflow"
)

func (t *CredentialTransport) roundTripOAuth(req *http.Request) (result *http.Response, resultErr error) {
	id := OAuthCredentialID(t.CredPattern)
	store, err := t.CredMgr.OAuthStore(req.Context())
	if err != nil {
		return nil, err
	}
	ctx, cancel := store.(*oauthStore).SessionContext(req.Context())
	requestID := uuid.NewString()
	t.CredMgr.mu.Lock()
	if t.CredMgr.oauthRequests == nil {
		t.CredMgr.oauthRequests = make(map[string]map[string]context.CancelFunc)
	}
	if t.CredMgr.oauthRequests[id] == nil {
		t.CredMgr.oauthRequests[id] = make(map[string]context.CancelFunc)
	}
	t.CredMgr.oauthRequests[id][requestID] = cancel
	t.CredMgr.mu.Unlock()
	cleanup := func() {
		cancel()
		t.CredMgr.mu.Lock()
		delete(t.CredMgr.oauthRequests[id], requestID)
		if len(t.CredMgr.oauthRequests[id]) == 0 {
			delete(t.CredMgr.oauthRequests, id)
		}
		t.CredMgr.mu.Unlock()
	}
	defer func() {
		if resultErr != nil || result == nil || result.Body == nil {
			cleanup()
		} else {
			result.Body = &oauthResponseBody{ReadCloser: result.Body, cancel: cleanup}
		}
	}()
	req = req.Clone(ctx)
	record, err := store.Load(req.Context(), id)
	if err != nil {
		return nil, err
	}
	resource, err := url.Parse(record.Resource)
	if err != nil {
		return nil, oauthflow.ErrResource
	}
	if req.URL.Scheme != resource.Scheme || req.URL.Host != resource.Host || req.URL.User != nil || !strings.HasPrefix(req.URL.Path, strings.TrimSuffix(resource.Path, "/")+"/") {
		return nil, oauthflow.ErrResource
	}
	t.CredMgr.mu.RLock()
	service := t.CredMgr.oauthService
	t.CredMgr.mu.RUnlock()
	if service == nil || !service.AllowsRequest(record, req.Method, req.URL) {
		return nil, oauthflow.ErrResource
	}
	auth, err := t.CredMgr.resolveOAuth(req.Context(), id, record.Resource, "")
	if err != nil {
		return nil, err
	}
	first := req.Clone(req.Context())
	if err = ApplyAuth(first, auth); err != nil {
		return nil, err
	}
	response, err := t.Base.RoundTrip(first)
	if err != nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		return response, err
	}
	fresh, err := t.CredMgr.resolveOAuth(req.Context(), id, record.Resource, auth.Token)
	if err != nil || fresh.Token == auth.Token {
		return response, nil
	}
	// Refresh also repairs future requests when this body cannot be replayed.
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return response, nil
	}
	retry := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		retry.Body, err = req.GetBody()
		if err != nil {
			return response, nil
		}
	}
	if err = ApplyAuth(retry, fresh); err != nil {
		if retry.Body != nil {
			_ = retry.Body.Close()
		}
		return response, nil
	}
	_ = response.Body.Close()
	return t.Base.RoundTrip(retry)
}

type oauthResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *oauthResponseBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }
