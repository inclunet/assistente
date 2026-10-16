package oauthflow

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Exercise the full authorization boundary with signed JWTs: malformed input
// alone would not prove that issuer, audience, signature and nonce are checked.
func TestAuthorizationRejectsUntrustedIdentityWithoutReplacingGrant(t *testing.T) {
	trusted, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"valid", "wrong_nonce", "missing_nonce", "wrong_issuer", "wrong_audience", "untrusted_signature", "expired", "wrong_subject", "missing_subject"} {
		t.Run(variant, func(t *testing.T) {
			type authorizationInput struct{ nonce, redirect, challenge string }
			var input, issuer, issuedJWT atomic.Value
			var exchanges atomic.Int32
			s, store, server := fixture(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.URL.Path == "/jwks" {
					_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &trusted.PublicKey, KeyID: "trusted", Algorithm: "RS256", Use: "sig"}}})
					return
				}
				if req.URL.Path != "/token" || req.Method != http.MethodPost {
					t.Errorf("unexpected protocol request: %s %s", req.Method, req.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				exchanges.Add(1)
				captured, ok := input.Load().(authorizationInput)
				if !ok {
					t.Error("exchange happened before authorization")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if err := req.ParseForm(); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				digest := sha256.Sum256([]byte(req.Form.Get("code_verifier")))
				if base64.RawURLEncoding.EncodeToString(digest[:]) != captured.challenge || req.Form.Get("redirect_uri") != captured.redirect || req.Form.Get("code") != "fixture-code" || req.Form.Get("client_id") != "client" {
					t.Error("exchange did not preserve PKCE, callback or client binding")
				}
				claims := jwt.Claims{Issuer: issuer.Load().(string), Subject: "subject", Audience: jwt.Audience{"client"}, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))}
				nonce, signingKey := captured.nonce, trusted
				switch variant {
				case "wrong_nonce":
					nonce = "different-private-nonce"
				case "missing_nonce":
					nonce = ""
				case "wrong_issuer":
					claims.Issuer = "https://other-issuer.example"
				case "wrong_audience":
					claims.Audience = jwt.Audience{"other-client"}
				case "untrusted_signature":
					signingKey = untrusted // Same kid; only cryptographic verification can reject it.
				case "expired":
					claims.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Hour))
				case "wrong_subject":
					claims.Subject = "another-account"
				case "missing_subject":
					claims.Subject = ""
				}
				signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signingKey}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "trusted"))
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				identityClaims := map[string]any{"email": "new@example.test"}
				if variant != "missing_nonce" {
					identityClaims["nonce"] = nonce
				}
				signed, err := jwt.Signed(signer).Claims(claims).Claims(identityClaims).Serialize()
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				issuedJWT.Store(signed)
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new-private-access", "refresh_token": "new-private-refresh", "id_token": signed, "token_type": "Bearer", "expires_in": 3600})
			})
			issuer.Store(server.URL)
			store.r.Email = "previous@example.test"
			before := store.r
			summary, err := s.Authorize(context.Background(), store, before.ID, "host", func(raw string) error {
				u, err := url.Parse(raw)
				if err != nil {
					return err
				}
				q := u.Query()
				if q.Get("nonce") == "" || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
					t.Fatal("authorization omitted nonce or PKCE")
				}
				input.Store(authorizationInput{q.Get("nonce"), q.Get("redirect_uri"), q.Get("code_challenge")})
				callback := q.Get("redirect_uri") + "?" + url.Values{"state": {q.Get("state")}, "code": {"fixture-code"}, "client_id": {"client"}}.Encode()
				response, err := http.Get(callback)
				if err != nil {
					return err
				}
				defer func() { _ = response.Body.Close() }()
				if response.StatusCode != http.StatusOK {
					t.Errorf("callback rejected valid state: %d", response.StatusCode)
				}
				return nil
			}, "Return to app")
			if exchanges.Load() != 1 {
				t.Fatalf("expected exactly one exchange, got %d", exchanges.Load())
			}
			after, loadErr := store.Load(context.Background(), before.ID)
			if loadErr != nil || after.AuthorizationAttempt != "" || !after.AuthorizationUntil.IsZero() {
				t.Fatal("authorization attempt was not released", loadErr)
			}
			if variant == "valid" {
				if err != nil || summary.State != "connected" || after.Tokens.Access != "new-private-access" || after.Tokens.Refresh != "new-private-refresh" || after.Tokens.ID != issuedJWT.Load().(string) || after.Email != "new@example.test" {
					t.Fatal("trusted identity was not committed", err)
				}
				return
			}
			if !errors.Is(err, ErrReauthorize) || err.Error() != ErrReauthorize.Error() || summary.State != "" {
				t.Fatalf("untrusted identity did not return the safe rejection: %v", err)
			}
			if !reflect.DeepEqual(after.Tokens, before.Tokens) || after.Subject != before.Subject || after.Email != before.Email || after.State != before.State || !reflect.DeepEqual(after.GrantedScopes, before.GrantedScopes) {
				t.Fatal("rejected identity replaced the previous grant")
			}
			for _, secret := range []string{issuedJWT.Load().(string), "new-private-access", "new-private-refresh", input.Load().(authorizationInput).nonce} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("identity rejection exposed protocol material")
				}
			}
		})
	}
}
