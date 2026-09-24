package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandidentity"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	httpCommandIssuer = "https://commands.example.test"
	httpCommandToken  = "mapped-token"
	httpAdminToken    = "admin-token"
)

type httpCommandVerifier struct{}

func (httpCommandVerifier) Validate(ctx context.Context, token string) (*auth.ExternalClaims, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	subject, scope := "external-subject", "commands:execute"
	if token == httpAdminToken {
		subject, scope = "external-admin", "assistente:identity:admin"
	} else if token != httpCommandToken {
		return nil, context.Canceled
	}
	return &auth.ExternalClaims{Issuer: httpCommandIssuer, Subject: subject, Scope: scope, Roles: []string{"runner"}}, nil
}

func (httpCommandVerifier) ValidateCached(ctx context.Context, token string) (*auth.ExternalClaims, error) {
	return (httpCommandVerifier{}).Validate(ctx, token)
}

type httpCommandFixture struct {
	server       *Server
	services     map[string]*commandexecution.ExternalService
	queued       chan struct{}
	queueRelease chan struct{}
	db           *gorm.DB
}

func newHTTPCommandFixture(t *testing.T, waitInQueue bool) *httpCommandFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http-commands.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &auth.ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	user := &database.User{Username: "http-command-owner", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	repo := auth.NewExternalIdentityRepository(db)
	for _, subject := range []string{"external-subject", "external-admin"} {
		if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: httpCommandIssuer, Subject: subject, UserID: user.ID}); err != nil {
			t.Fatal(err)
		}
	}
	verifier := httpCommandVerifier{}
	authenticator := auth.NewExternalCommandAuthenticator(verifier, repo)
	authenticator.SetReadiness(repo.CheckReadiness)
	admin, err := auth.NewExternalIdentityAdminService(verifier, repo, auth.ExternalIdentityAdminConfig{Issuer: httpCommandIssuer, AdminScopes: []string{"assistente:identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{
		Definition: commandcatalog.Definition{
			ID: "safe.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
			AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat},
			Context:        commandcatalog.ContextPolicy{None: true},
			Presentation: &commandcatalog.Presentation{Version: "test-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
				"pt-BR": {Name: "safe.read", Description: "safe.read", Category: "test"},
				"en":    {Name: "safe.read", Description: "safe.read", Category: "test"},
				"es":    {Name: "safe.read", Description: "safe.read", Category: "test"},
			}},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, Risk: commandcatalog.RiskLow,
			Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
			Scopes:      []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: "internal/safe/read", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/safe/read", Classification: commandcatalog.HandlerInternal},
	}})
	if err != nil {
		t.Fatal(err)
	}
	gate := &commandsecurity.DispatchGate{}
	epochs, err := commandsecurity.NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	keyManager := credentials.NewManager(bytes.Repeat([]byte{0x31}, 32))
	if err := keyManager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x32}, 32))); err != nil {
		t.Fatal(err)
	}
	keys, err := commandledger.NewCredentialKeyProvider(keyManager)
	if err != nil {
		t.Fatal(err)
	}
	bus, err := commandcontext.NewFactBus(nil)
	if err != nil {
		t.Fatal(err)
	}
	queued := make(chan struct{}, 1)
	queueRelease := make(chan struct{})
	services := make(map[string]*commandexecution.ExternalService)
	for _, source := range []struct {
		key  string
		kind commandcatalog.Source
	}{{"palette", commandcatalog.Palette}, {"ui", commandcatalog.UI}, {"chat", commandcatalog.Chat}} {
		ports := &commandexecution.EnvelopeIdentityPorts{
			Snapshot: func(_ context.Context, owner commandledger.FullOwnership, _ commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
				return commandcontract.Envelope{RegistryVersion: "registry-v1", UserID: owner.UserID, GlobalConfigGeneration: httpStringPtr("global-v1"), ActiveLayersGeneration: httpStringPtr("layers-v1")}, nil
			},
			Resolve: func(_ context.Context, _ commandledger.FullOwnership, c commandexecution.EnvelopeCandidate, _ commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
				return commandexecution.EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: c.CommandID, Arguments: c.Arguments}, nil
			},
			Authorize: func(context.Context, commandledger.FullOwnership, commandcontract.Envelope, commandcatalog.Definition) error {
				return nil
			},
			AuthorizeLookup: func(context.Context, commandledger.FullOwnership, commandledger.FullRecord) error { return nil },
		}
		config := commandexecution.Config{
			Envelope: &commandexecution.EnvelopeConfig{Identity: ports, Context: bus}, Epochs: epochs, Store: store,
			Registry: registry, RegistryVersion: "registry-v1", Source: source.kind, Keys: keys, KeyVersion: "v1",
			Now: time.Now, Retention: time.Hour, ExecutionTimeout: 3 * time.Second, FinalizationTimeout: 3 * time.Second,
			Handlers: map[string]commandexecution.Handler{"safe.read": {
				Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/safe/read", Classification: commandcatalog.HandlerInternal},
				Start: func(context.Context, commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
					done := make(chan commandexecution.Outcome, 1)
					done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
					return commandexecution.ExecutionHandle{ID: httpNewUUID(), Done: done, Cancel: func() {}}, nil
				},
			}},
		}
		if waitInQueue && source.key == "ui" {
			config.Envelope.AwaitQueue = func(ctx context.Context, _ commandcontract.Envelope) error {
				select {
				case queued <- struct{}{}:
				default:
				}
				select {
				case <-queueRelease:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		service, err := commandexecution.NewExternal(config, authenticator, admin, []commandidentity.AuthorizationRule{{
			CommandID: "safe.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser}, RequiredRoles: []string{"runner"}, RequiredScopes: []string{"commands:execute"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		services[source.key] = service
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, service := range services {
			if err := service.Shutdown(shutdownCtx); err != nil {
				t.Errorf("shutdown external service: %v", err)
			}
		}
	})
	return &httpCommandFixture{server: New(Config{Mode: "external", ExternalCommands: services, AuthBurst: 100}), services: services, queued: queued, queueRelease: queueRelease, db: db}
}

func httpStringPtr(value string) *string { return &value }

func httpNewUUID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}

func httpCommandRequest(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestExternalCommandHTTPExecutesAndReturnsRedactedProjection(t *testing.T) {
	f := newHTTPCommandFixture(t, false)
	id, correlation := httpNewUUID(), httpNewUUID()
	body, _ := json.Marshal(map[string]any{"invocation_id": id, "correlation_id": correlation, "command_id": "safe.read", "arguments": map[string]any{}})
	rec := httpCommandRequest(f.server.Handler(), http.MethodPost, "/commands/palette/execute", "Bearer "+httpCommandToken, string(body))
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("execute status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || string(got["invocationId"]) != `"`+id+`"` || string(got["status"]) != `"succeeded"` || string(got["result"]) != `{}` {
		t.Fatalf("execute projection = %s", rec.Body.String())
	}
	for _, forbidden := range []string{"ownership", "envelope", "arguments", "requestFingerprint", "authContext", "userId"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(forbidden)) {
			t.Fatalf("response exposed %q: %s", forbidden, rec.Body.String())
		}
	}
	replay := httpCommandRequest(f.server.Handler(), http.MethodPost, "/commands/palette/execute", "Bearer "+httpCommandToken, string(body))
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	got = nil
	if err := json.Unmarshal(replay.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, hasResult := got["result"]; hasResult {
		t.Fatalf("replay exposed an ephemeral result: %s", replay.Body.String())
	}
	lookup := httpCommandRequest(f.server.Handler(), http.MethodGet, "/commands/palette/invocations/"+id, "Bearer "+httpCommandToken, "")
	if lookup.Code != http.StatusOK || lookup.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("lookup status=%d body=%s", lookup.Code, lookup.Body.String())
	}
	got = nil
	if err := json.Unmarshal(lookup.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["result"] != nil {
		t.Fatalf("lookup projection leaked transient result or record: %s", lookup.Body.String())
	}
}

func TestExternalCommandHTTPRejectsModeServiceTokenAndStrictBodies(t *testing.T) {
	for _, tc := range []struct {
		name, path, token, body string
		cfg                     Config
		want                    int
	}{
		{"local-mode", "/commands/palette/execute", "Bearer token", `{}`, Config{}, http.StatusNotFound},
		{"nil-service", "/commands/palette/execute", "Bearer token", `{}`, Config{Mode: "external"}, http.StatusServiceUnavailable},
		{"unknown-source", "/commands/keyboard.local/execute", "Bearer token", `{}`, Config{Mode: "external"}, http.StatusNotFound},
		{"missing-bearer", "/commands/palette/execute", "", `{}`, Config{Mode: "external", ExternalCommands: map[string]*commandexecution.ExternalService{}}, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httpCommandRequest(New(tc.cfg).Handler(), http.MethodPost, tc.path, tc.token, tc.body)
			if rec.Code != tc.want || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d want=%d headers=%v", rec.Code, tc.want, rec.Header())
			}
		})
	}
	f := newHTTPCommandFixture(t, false)
	for _, tc := range []struct {
		name, authorization, body string
		want                      int
	}{
		{"double-space-bearer", "Bearer  " + httpCommandToken, `{}`, http.StatusUnauthorized},
		{"basic-auth", "Basic " + httpCommandToken, `{}`, http.StatusUnauthorized},
		{"unknown-field", "Bearer " + httpCommandToken, `{"invocation_id":"` + httpNewUUID() + `","correlation_id":"` + httpNewUUID() + `","command_id":"safe.read","arguments":{},"ownership":{}}`, http.StatusBadRequest},
		{"trailing-json", "Bearer " + httpCommandToken, `{"invocation_id":"` + httpNewUUID() + `","correlation_id":"` + httpNewUUID() + `","command_id":"safe.read","arguments":{}} {}`, http.StatusBadRequest},
		{"non-object", "Bearer " + httpCommandToken, `[]`, http.StatusBadRequest},
		{"oversized-body", "Bearer " + httpCommandToken, `{"padding":"` + strings.Repeat("x", 9000) + `"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httpCommandRequest(f.server.Handler(), http.MethodPost, "/commands/palette/execute", tc.authorization, tc.body)
			if rec.Code != tc.want || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
	duplicateAuthorization := httptest.NewRequest(http.MethodGet, "/commands/palette/invocations/"+httpNewUUID(), nil)
	duplicateAuthorization.Header.Add("Authorization", "Bearer "+httpCommandToken)
	duplicateAuthorization.Header.Add("Authorization", "Bearer alternate-token")
	duplicateResponse := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(duplicateResponse, duplicateAuthorization)
	if duplicateResponse.Code != http.StatusUnauthorized || duplicateResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("multiple Authorization values were accepted: status=%d body=%s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	limited := New(Config{Mode: "external", ExternalCommands: f.services, AuthBurst: 1, AuthRate: 0.001})
	first := httpCommandRequest(limited.Handler(), http.MethodGet, "/commands/palette/invocations/"+httpNewUUID(), "", "")
	second := httpCommandRequest(limited.Handler(), http.MethodGet, "/commands/palette/invocations/"+httpNewUUID(), "", "")
	if first.Code != http.StatusUnauthorized || second.Code != http.StatusTooManyRequests || second.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("external route rate limit: first=%d second=%d headers=%v", first.Code, second.Code, second.Header())
	}
}

func TestExternalCommandConfigValidatesSourcesAndCopiesMap(t *testing.T) {
	f := newHTTPCommandFixture(t, false)
	wrongSource := New(Config{Mode: "external", ExternalCommands: map[string]*commandexecution.ExternalService{"palette": f.services["ui"]}})
	if wrongSource.externalCommands != nil {
		t.Fatal("source mismatch did not fail closed for the entire command map")
	}
	for _, source := range []string{"palette", "ui", "chat"} {
		rec := httpCommandRequest(wrongSource.Handler(), http.MethodGet, "/commands/"+source+"/invocations/"+httpNewUUID(), "Bearer "+httpCommandToken, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("invalid source map exposed %s service: status=%d body=%s", source, rec.Code, rec.Body.String())
		}
	}

	callerMap := map[string]*commandexecution.ExternalService{
		"palette": f.services["palette"],
		"ui":      f.services["ui"],
		"chat":    f.services["chat"],
	}
	server := New(Config{Mode: "external", ExternalCommands: callerMap, AuthBurst: 100})
	if len(server.externalCommands) != len(callerMap) {
		t.Fatalf("server retained incomplete map: %#v", server.externalCommands)
	}
	callerMap["palette"] = nil
	callerMap["untrusted"] = f.services["palette"]
	if server.externalCommands["palette"] != f.services["palette"] || server.externalCommands["untrusted"] != nil {
		t.Fatal("server command map aliases caller-owned map")
	}
	id, correlation := httpNewUUID(), httpNewUUID()
	body, _ := json.Marshal(map[string]any{"invocation_id": id, "correlation_id": correlation, "command_id": "safe.read", "arguments": map[string]any{}})
	rec := httpCommandRequest(server.Handler(), http.MethodPost, "/commands/palette/execute", "Bearer "+httpCommandToken, string(body))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("mutating caller map changed published services: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalCommandConfigRejectsServicesWithUnrelatedSecurity(t *testing.T) {
	paletteFixture := newHTTPCommandFixture(t, false)
	uiFixture := newHTTPCommandFixture(t, false)
	server := New(Config{Mode: "external", ExternalCommands: map[string]*commandexecution.ExternalService{
		"palette": paletteFixture.services["palette"],
		"ui":      uiFixture.services["ui"],
	}})
	if server.externalCommands != nil {
		t.Fatal("services with unrelated epochs/authenticator/admin were not rejected")
	}
	for _, source := range []string{"palette", "ui", "chat"} {
		rec := httpCommandRequest(server.Handler(), http.MethodPost, "/commands/"+source+"/execute", "Bearer "+httpCommandToken, `{}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("security mismatch did not fail closed for %s: status=%d body=%s", source, rec.Code, rec.Body.String())
		}
	}
}

func TestExternalIdentityRevokeHTTPInvalidatesInvocationAcrossSources(t *testing.T) {
	f := newHTTPCommandFixture(t, true)
	id, correlation := httpNewUUID(), httpNewUUID()
	body, _ := json.Marshal(map[string]any{"invocation_id": id, "correlation_id": correlation, "command_id": "safe.read", "arguments": map[string]any{}})
	execute := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		execute <- httpCommandRequest(f.server.Handler(), http.MethodPost, "/commands/ui/execute", "Bearer "+httpCommandToken, string(body))
	}()
	select {
	case <-f.queued:
	case rec := <-execute:
		t.Fatalf("UI execution finished before queue: status=%d body=%s", rec.Code, rec.Body.String())
	case <-time.After(2 * time.Second):
		t.Fatal("UI execution did not enter queue")
	}
	revoke := httpCommandRequest(f.server.Handler(), http.MethodPost, "/auth/external/identities/revoke", "Bearer "+httpAdminToken, `{"issuer":"`+httpCommandIssuer+`","subject":"external-subject"}`)
	if revoke.Code != http.StatusNoContent || revoke.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoke status=%d body=%s", revoke.Code, revoke.Body.String())
	}
	close(f.queueRelease)
	select {
	case rec := <-execute:
		var got struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || got.Status != string(commandledger.CancelledStale) {
			t.Fatalf("cross-source execution status=%d result=%s", rec.Code, rec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("palette revoke did not cancel pending UI execution")
	}
}
