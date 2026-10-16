package app

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/config"
	"assistente/internal/database"
	"github.com/google/uuid"
)

const (
	externalUIFixtureIssuer   = "https://external-ui.example"
	externalUIFixtureAudience = "assistente"
)

type externalUIAppHTTPFixture struct {
	app     *App
	handler http.Handler
	token   string
	signer  *auth.TokenSigner
}

// This fixture exercises the actual App HTTP assembly, a real local owner and
// session, an explicit external-identity mapping, and a JWT verified against
// the configured JWKS endpoint. It deliberately does not fake an executor or
// a Wails bridge.
func newExternalUIAppHTTPFixture(t *testing.T) *externalUIAppHTTPFixture {
	t.Helper()

	a := readyCommandProduct(t)
	db := database.DB()
	if err := db.AutoMigrate(&auth.ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}

	var owner database.User
	if err := db.Where("id = ? AND is_active = ?", a.currentUserID, true).First(&owner).Error; err != nil {
		t.Fatalf("load active local UI owner: %v", err)
	}
	if a.currentAuthUser == nil || a.currentAuthUser.UserID != owner.ID || a.currentAuthUser.SessionID == "" {
		t.Fatalf("fixture lacks a real local UI session: user=%q auth=%+v", owner.ID, a.currentAuthUser)
	}
	var session database.Session
	if err := db.Where("id = ? AND user_id = ? AND revoked_at IS NULL", a.currentAuthUser.SessionID, owner.ID).First(&session).Error; err != nil {
		t.Fatalf("load active local UI session: %v", err)
	}
	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(signer.JWKSet())
	}))
	t.Cleanup(jwks.Close)

	cfg := config.DefaultAuthConfig()
	cfg.Mode = "external"
	cfg.External.Issuer = externalUIFixtureIssuer
	cfg.External.Audience = externalUIFixtureAudience
	cfg.External.JWKSURL = jwks.URL
	cfg.External.RequiredScopes = []string{externalCommandExecuteScope}
	cfg.External.IdentityAdminScopes = []string{"identity:admin"}
	handler, err := a.newHTTPAPIHandler(cfg)
	if err != nil {
		t.Fatalf("mount production App HTTP API: %v", err)
	}

	encodedKey, err := signer.ExportPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := base64.RawURLEncoding.DecodeString(encodedKey)
	if err != nil {
		t.Fatal(err)
	}
	header, err := json.Marshal(map[string]string{"alg": "EdDSA", "kid": signer.JWKSet().Keys[0].KeyID})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claims, err := json.Marshal(map[string]any{
		"iss": externalUIFixtureIssuer, "aud": externalUIFixtureAudience, "sub": owner.ID,
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "scope": "identity:admin commands:execute",
		"roles": []string{database.UserRoleUser},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	token := payload + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(privateKey), []byte(payload)))

	bootstrap := httptest.NewRequest(http.MethodPost, "/auth/external/identities/bootstrap", strings.NewReader(`{}`))
	bootstrap.Header.Set("Authorization", "Bearer "+token)
	bootstrapResponse := httptest.NewRecorder()
	handler.ServeHTTP(bootstrapResponse, bootstrap)
	if bootstrapResponse.Code != http.StatusCreated {
		t.Fatalf("bootstrap external owner through production HTTP route: status=%d body=%s", bootstrapResponse.Code, bootstrapResponse.Body.String())
	}

	return &externalUIAppHTTPFixture{app: a, handler: handler, token: token, signer: signer}
}

func TestHTTPAPIExternalIdentityExecutesBackendCommandWithoutUILink(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)

	identityRequest := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	identityRequest.Header.Set("Authorization", "Bearer "+f.token)
	identityResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(identityResponse, identityRequest)
	if identityResponse.Code != http.StatusOK {
		t.Fatalf("mapped JWT should resolve to the existing local owner: status=%d body=%s", identityResponse.Code, identityResponse.Body.String())
	}
	if identityResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("external identity response must not be cacheable")
	}
	var identity struct {
		UserID    string `json:"userId"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(identityResponse.Body.Bytes(), &identity); err != nil {
		t.Fatalf("decode identity response: %v", err)
	}
	if identity.UserID != f.app.currentUserID || identity.SessionID != "" {
		t.Fatalf("external identity must map to the local user without exporting the desktop session: got=%+v localUser=%q", identity, f.app.currentUserID)
	}

	id, correlation := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	body, err := json.Marshal(map[string]any{
		"invocation_id": id, "correlation_id": correlation, "command_id": commandProductWorkspaceListID,
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	executeResponse := externalUIHTTPRequest(f, http.MethodPost, "/commands/ui/execute", string(body))
	if executeResponse.Code != http.StatusOK || executeResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("backend command without an external UI link: status=%d body=%s", executeResponse.Code, executeResponse.Body.String())
	}
	var result struct {
		InvocationID string          `json:"invocationId"`
		Status       string          `json:"status"`
		Result       json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(executeResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.InvocationID != id || result.Status != "succeeded" || !json.Valid(result.Result) {
		t.Fatalf("unlinked backend response = %s", executeResponse.Body.String())
	}
	if manager := f.app.commandExternalUI.Load(); manager != nil {
		status, err := manager.ReadForOwner(f.app.commandProduct.Load().owner())
		if err != nil || status.State != "disconnected" {
			t.Fatalf("backend command unexpectedly created an external UI link: status=%+v err=%v", status, err)
		}
	}
}

func TestHTTPAPIExternalUIConsumeTakeCompleteReplayTargetAndRevoke(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	p := f.app.commandProduct.Load()
	if p == nil {
		t.Fatal("fixture did not mount the production command product")
	}
	owner := p.owner()
	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "test-tab",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "chat", SurfaceID: "test-chat", SnapshotVersion: "snapshot-1",
		},
	}
	invitation, err := f.app.BeginExternalUIConnection(target)
	if err != nil || invitation.Invitation == "" {
		t.Fatalf("BeginExternalUIConnection: invitation=%+v err=%v", invitation, err)
	}
	consumeBody, _ := json.Marshal(map[string]string{"invitation": invitation.Invitation})
	consume := externalUIHTTPRequest(f, http.MethodPost, "/auth/external/ui-connections/consume", string(consumeBody))
	if consume.Code != http.StatusOK || consume.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("consume real invite: status=%d body=%s", consume.Code, consume.Body.String())
	}
	var connection struct {
		ConnectionID     string `json:"connectionId"`
		Generation       string `json:"generation"`
		TargetSnapshotID string `json:"targetSnapshotId"`
		ContextVersion   string `json:"contextVersion"`
	}
	if err := json.Unmarshal(consume.Body.Bytes(), &connection); err != nil {
		t.Fatal(err)
	}
	if connection.ConnectionID == "" || connection.Generation == "" || connection.TargetSnapshotID == "" || connection.ContextVersion == "" {
		t.Fatalf("consume response lacks bound connection stamps: %+v", connection)
	}
	localStatus, err := f.app.ReadExternalUIConnection()
	if err != nil || localStatus.State != "connected" || localStatus.Owner != owner ||
		localStatus.Target.WorkspaceID != target.WorkspaceID || localStatus.Target.TabID != target.TabID ||
		localStatus.Target.Surface.SurfaceType != target.Surface.SurfaceType || localStatus.Target.Surface.SurfaceID != target.Surface.SurfaceID {
		t.Fatalf("App-local connection status does not bind the consumed owner/target: status=%+v err=%v", localStatus, err)
	}
	contextPath := "/auth/external/ui-connections/context?connectionId=" + connection.ConnectionID + "&generation=" + connection.Generation
	contextResponse := externalUIHTTPRequest(f, http.MethodGet, contextPath, "")
	if contextResponse.Code != http.StatusOK {
		t.Fatalf("read explicit external UI target context: status=%d body=%s", contextResponse.Code, contextResponse.Body.String())
	}
	var returnedContext struct {
		ConnectionID     string `json:"connectionId"`
		Generation       string `json:"generation"`
		TargetSnapshotID string `json:"targetSnapshotId"`
		ContextVersion   string `json:"contextVersion"`
	}
	if err := json.Unmarshal(contextResponse.Body.Bytes(), &returnedContext); err != nil {
		t.Fatal(err)
	}
	if returnedContext.ConnectionID != connection.ConnectionID || returnedContext.Generation != connection.Generation || returnedContext.TargetSnapshotID != connection.TargetSnapshotID || returnedContext.ContextVersion != connection.ContextVersion {
		t.Fatalf("context response changed connection stamps: %+v want %+v", returnedContext, connection)
	}

	readyEvents := make(chan commandui.ExternalUIReadyEvent, 1)
	f.app.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name != "external:command:ready" {
			return
		}
		ready, ok := payload.(commandui.ExternalUIReadyEvent)
		if !ok {
			t.Errorf("external ready event has unexpected payload type %T", payload)
			return
		}
		select {
		case readyEvents <- ready:
		default:
			t.Errorf("unexpected duplicate external ready event: %+v", ready)
		}
	})
	invocationID, correlationID := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	executeBody := func(targetSnapshotID, contextVersion string) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"invocation_id": invocationID, "correlation_id": correlationID, "command_id": "navigation.history.open", "arguments": map[string]any{},
			"connectionId": connection.ConnectionID, "generation": connection.Generation,
			"targetSnapshotId": targetSnapshotID, "contextVersion": contextVersion,
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	executeResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		executeResult <- externalUIHTTPRequest(f, http.MethodPost, "/commands/ui/execute", executeBody(connection.TargetSnapshotID, connection.ContextVersion))
	}()
	var ready commandui.ExternalUIReadyEvent
	select {
	case ready = <-readyEvents:
	case response := <-executeResult:
		t.Fatalf("UI execution finished before a ready handoff: status=%d body=%s", response.Code, response.Body.String())
	case <-time.After(5 * time.Second):
		t.Fatal("real HTTP UI execution did not emit an external ready event")
	}
	if ready.InvocationID != invocationID || ready.CommandID != "navigation.history.open" || ready.ConnectionID != connection.ConnectionID || ready.Generation != connection.Generation || ready.TargetSnapshotID != connection.TargetSnapshotID || ready.ContextVersion != connection.ContextVersion {
		t.Fatalf("ready event did not preserve the authenticated target tuple: %+v", ready)
	}
	readyJSON, err := json.Marshal(ready)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"arguments", "receipt", "secret", "token"} {
		if strings.Contains(strings.ToLower(string(readyJSON)), forbidden) {
			t.Fatalf("ready event exposed %q: %s", forbidden, readyJSON)
		}
	}

	handoff, err := f.app.TakeExternalUICommand(ExternalUICommandRequest{
		Owner: owner, ConnectionID: ready.ConnectionID, Generation: ready.Generation, InvocationID: ready.InvocationID,
		TargetSnapshotID: ready.TargetSnapshotID, ContextVersion: ready.ContextVersion,
	})
	if err != nil || handoff.ReceiptID == "" || handoff.CommandID != "navigation.history.open" || handoff.InvocationID != invocationID || string(handoff.Arguments) != `{}` || handoff.Target.TabID != "test-tab" {
		t.Fatalf("TakeExternalUICommand: handoff=%+v err=%v", handoff, err)
	}
	ack, err := f.app.CompleteExternalUICommand(ExternalUICommandCompletion{
		Owner: owner, ConnectionID: ready.ConnectionID, Generation: ready.Generation, InvocationID: ready.InvocationID,
		ReceiptID: handoff.ReceiptID, TargetSnapshotID: ready.TargetSnapshotID, ContextVersion: ready.ContextVersion, Outcome: "succeeded",
	})
	if err != nil || !ack.Accepted {
		t.Fatalf("CompleteExternalUICommand: ack=%+v err=%v", ack, err)
	}
	var completed *httptest.ResponseRecorder
	select {
	case completed = <-executeResult:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP execute did not return after the Wails receipt")
	}
	var completedBody struct {
		InvocationID string `json:"invocationId"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(completed.Body.Bytes(), &completedBody); err != nil {
		t.Fatal(err)
	}
	if completed.Code != http.StatusOK || completedBody.InvocationID != invocationID || completedBody.Status != "succeeded" {
		t.Fatalf("completed UI invocation: status=%d body=%s", completed.Code, completed.Body.String())
	}

	updatedTarget := target
	updatedTarget.TabID = "test-tab-after-navigation"
	updatedTarget.Surface.SnapshotVersion = "snapshot-2"
	updated, err := f.app.PublishExternalUIContext(commandui.ExternalUIContextPublication{
		Owner: owner, ConnectionID: connection.ConnectionID, Generation: connection.Generation,
		ExpectedTargetSnapshot: connection.TargetSnapshotID, ExpectedContextVersion: connection.ContextVersion, Target: updatedTarget,
	})
	if err != nil || updated.TargetSnapshotID == connection.TargetSnapshotID || updated.ContextVersion == connection.ContextVersion {
		t.Fatalf("advance target snapshot: updated=%+v err=%v", updated, err)
	}
	replay := externalUIHTTPRequest(f, http.MethodPost, "/commands/ui/execute", executeBody(updated.TargetSnapshotID, updated.ContextVersion))
	if replay.Code != http.StatusConflict {
		t.Fatalf("replay under changed target must conflict, not rebind old invocation: status=%d body=%s", replay.Code, replay.Body.String())
	}
	select {
	case extra := <-readyEvents:
		t.Fatalf("changed-target replay emitted a second UI handoff: %+v", extra)
	default:
	}

	revocation := externalUIHTTPRequest(f, http.MethodPost, "/auth/external/identities/revoke", `{"issuer":"`+externalUIFixtureIssuer+`","subject":"`+owner.UserID+`"}`)
	if revocation.Code != http.StatusNoContent {
		t.Fatalf("revoke mapped UI identity: status=%d body=%s", revocation.Code, revocation.Body.String())
	}
	status, err := f.app.ReadExternalUIConnection()
	if err != nil || status.State != "disconnected" {
		t.Fatalf("revoking the external principal did not clear its UI link: status=%+v err=%v", status, err)
	}
	contextAfterRevoke := externalUIHTTPRequest(f, http.MethodGet, contextPath, "")
	if contextAfterRevoke.Code != http.StatusNotFound {
		t.Fatalf("revoked external principal still reads linked context: status=%d body=%s", contextAfterRevoke.Code, contextAfterRevoke.Body.String())
	}
}

func externalUIHTTPRequest(f *externalUIAppHTTPFixture, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+f.token)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func TestExternalUIAppConnectionAPIsRequireRealOwnerAndWorkspace(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	p := f.app.commandProduct.Load()
	if p == nil {
		t.Fatal("fixture did not mount the production command product")
	}
	owner := p.owner()
	read := func(want string) commandui.ExternalUIConnectionStatus {
		t.Helper()
		status, err := f.app.ReadExternalUIConnection()
		if err != nil {
			t.Fatalf("ReadExternalUIConnection: %v", err)
		}
		if status.State != want || status.Owner != owner {
			t.Fatalf("connection status = %+v, want state=%q owner=%+v", status, want, owner)
		}
		return status
	}
	read("disconnected")

	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "tab-lifecycle",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "editor", SurfaceID: "surface-lifecycle", SnapshotVersion: "snapshot-v1",
		},
	}
	wrongTarget := target
	wrongTarget.WorkspaceID = owner.WorkspaceID + "-other"
	if _, err := f.app.BeginExternalUIConnection(wrongTarget); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("BeginExternalUIConnection accepted another workspace: %v", err)
	}

	invitation, err := f.app.BeginExternalUIConnection(target)
	if err != nil {
		t.Fatalf("BeginExternalUIConnection: %v", err)
	}
	if invitation.Invitation == "" || !invitation.ExpiresAt.After(time.Now()) {
		t.Fatalf("begin returned invalid one-time invitation: %+v", invitation)
	}
	waiting := read("waiting_claim")
	if waiting.ConnectionID != "" || waiting.Generation != "" {
		t.Fatalf("unclaimed invitation exposed a connection: %+v", waiting)
	}

	// Claim uses the concrete App-owned manager that the HTTP adapter calls;
	// this test does not inject a fake HTTP port or start UI command execution.
	manager := f.app.commandExternalUI.Load()
	if manager == nil {
		t.Fatal("Begin did not install the production external UI manager")
	}
	principal := commandui.ExternalUIPrincipal{
		Issuer: externalUIFixtureIssuer, Subject: owner.UserID, UserID: owner.UserID,
		AuthContextID: `opaque-test-context-fingerprint`,
	}
	claimed, err := manager.Claim(invitation.Invitation, principal)
	if err != nil {
		t.Fatalf("claim with matching existing local owner: %v", err)
	}
	connected := read("connected")
	if connected.ConnectionID != claimed.ConnectionID || connected.Generation != claimed.Generation ||
		connected.TargetSnapshotID != claimed.TargetSnapshotID || connected.ContextVersion != claimed.ContextVersion {
		t.Fatalf("Wails read disagrees with manager claim: read=%+v claim=%+v", connected, claimed)
	}
	if _, err := manager.Claim(invitation.Invitation, principal); !errors.Is(err, commandui.ErrExternalConnectionDenied) {
		t.Fatalf("invitation replay = %v", err)
	}

	lease := commandui.ExternalUILease{Owner: owner, ConnectionID: connected.ConnectionID, Generation: connected.Generation}
	forgedOwner := owner
	forgedOwner.UserID = uuid.Must(uuid.NewV7()).String()
	forgedLease := lease
	forgedLease.Owner = forgedOwner
	if _, err := f.app.HeartbeatExternalUIConnection(forgedLease); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("HeartbeatExternalUIConnection accepted forged owner: %v", err)
	}
	if err := f.app.DisconnectExternalUIConnection(forgedLease); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("DisconnectExternalUIConnection accepted forged owner: %v", err)
	}
	forgedPublication := commandui.ExternalUIContextPublication{
		Owner: forgedOwner, ConnectionID: lease.ConnectionID, Generation: lease.Generation,
		ExpectedTargetSnapshot: connected.TargetSnapshotID, ExpectedContextVersion: connected.ContextVersion,
		Target: target,
	}
	if _, err := f.app.PublishExternalUIContext(forgedPublication); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("PublishExternalUIContext accepted forged owner: %v", err)
	}
	if afterForgery := read("connected"); afterForgery.TargetSnapshotID != connected.TargetSnapshotID || afterForgery.ContextVersion != connected.ContextVersion {
		t.Fatalf("forged owner changed live connection: %+v", afterForgery)
	}

	publication := commandui.ExternalUIContextPublication{
		Owner: owner, ConnectionID: lease.ConnectionID, Generation: lease.Generation,
		ExpectedTargetSnapshot: connected.TargetSnapshotID, ExpectedContextVersion: connected.ContextVersion,
		Target: target,
	}
	publication.Target.Surface.SnapshotVersion = "snapshot-v2"
	updated, err := f.app.PublishExternalUIContext(publication)
	if err != nil {
		t.Fatalf("PublishExternalUIContext: %v", err)
	}
	if updated.Generation != connected.Generation || updated.TargetSnapshotID == connected.TargetSnapshotID || updated.ContextVersion == connected.ContextVersion {
		t.Fatalf("context publication must rotate only snapshot/version: before=%+v after=%+v", connected, updated)
	}
	lease.Generation = updated.Generation
	heartbeat, err := f.app.HeartbeatExternalUIConnection(lease)
	if err != nil {
		t.Fatalf("HeartbeatExternalUIConnection: %v", err)
	}
	if heartbeat.Generation != updated.Generation || heartbeat.TargetSnapshotID != updated.TargetSnapshotID || heartbeat.ContextVersion != updated.ContextVersion {
		t.Fatalf("heartbeat changed connection or context stamps: %+v", heartbeat)
	}
	if err := f.app.DisconnectExternalUIConnection(lease); err != nil {
		t.Fatalf("DisconnectExternalUIConnection: %v", err)
	}
	read("disconnected")
}

func TestExternalUIAppLogoutClearsClaimedConnection(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	a := f.app
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("fixture did not mount the production command product")
	}
	owner := p.owner()
	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "tab-lifecycle",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "editor", SurfaceID: "surface-lifecycle", SnapshotVersion: "snapshot-v1",
		},
	}
	invitation, err := a.BeginExternalUIConnection(target)
	if err != nil {
		t.Fatal(err)
	}
	manager := a.commandExternalUI.Load()
	principal := commandui.ExternalUIPrincipal{
		Issuer: externalUIFixtureIssuer, Subject: owner.UserID, UserID: owner.UserID,
		AuthContextID: `opaque-test-context-fingerprint`,
	}
	claimed, err := manager.Claim(invitation.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := a.ReadExternalUIConnection(); err != nil || status.State != "connected" {
		t.Fatalf("connection not established before logout: status=%+v err=%v", status, err)
	}

	// readyCommandProduct focuses command lifecycle and does not mount auth
	// services; supply real DB-backed services so this invokes the normal logout
	// path instead of testing only its error rollback.
	a.identitySvc = auth.NewIdentityService(database.DB())
	a.vaultSvc = auth.NewVaultService(nil, nil)
	a.authKeyringDelete = func() error { return nil }
	if err := a.Logout(LogoutRequest{}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if status, err := manager.ReadForOwner(owner); err != nil || status.State != "disconnected" {
		t.Fatalf("logout retained external UI link: status=%+v err=%v", status, err)
	}
	if _, err := a.ReadExternalUIConnection(); err == nil {
		t.Fatal("ReadExternalUIConnection succeeded after logout")
	}
	if claimed.ConnectionID == "" {
		t.Fatal("fixture did not claim a connection before logout")
	}
}

func TestExternalUIAppLogoutClearsPendingInvitation(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	a := f.app
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("fixture did not mount the production command product")
	}
	owner := p.owner()
	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "tab-lifecycle",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "editor", SurfaceID: "surface-lifecycle", SnapshotVersion: "snapshot-v1",
		},
	}
	invitation, err := a.BeginExternalUIConnection(target)
	if err != nil {
		t.Fatal(err)
	}
	manager := a.commandExternalUI.Load()
	if manager == nil {
		t.Fatal("Begin did not install the production external UI manager")
	}
	if status, err := a.ReadExternalUIConnection(); err != nil || status.State != "waiting_claim" {
		t.Fatalf("invitation not pending before logout: status=%+v err=%v", status, err)
	}

	// Mount real auth services so Logout takes its successful production path.
	a.identitySvc = auth.NewIdentityService(database.DB())
	a.vaultSvc = auth.NewVaultService(nil, nil)
	a.authKeyringDelete = func() error { return nil }
	if err := a.Logout(LogoutRequest{}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if status, err := manager.ReadForOwner(owner); err != nil || status.State != "disconnected" {
		t.Fatalf("logout retained pending invitation: status=%+v err=%v", status, err)
	}
	principal := commandui.ExternalUIPrincipal{
		Issuer: externalUIFixtureIssuer, Subject: owner.UserID, UserID: owner.UserID,
		AuthContextID: `opaque-test-context-fingerprint`,
	}
	if _, err := manager.Claim(invitation.Invitation, principal); !errors.Is(err, commandui.ErrExternalConnectionDenied) {
		t.Fatalf("logout left invitation claimable: %v", err)
	}
}

func TestExternalUIAppBeginRejectedAfterProductShutdown(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	a := f.app
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("fixture did not mount the production command product")
	}
	owner := p.owner()
	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "tab-lifecycle",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "editor", SurfaceID: "surface-lifecycle", SnapshotVersion: "snapshot-v1",
		},
	}
	invitation, err := a.BeginExternalUIConnection(target)
	if err != nil {
		t.Fatalf("Begin before product shutdown: %v", err)
	}
	manager := a.commandExternalUI.Load()
	if manager == nil {
		t.Fatal("Begin did not install the production external UI manager")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("command product shutdown: %v", err)
	}
	if _, err := a.BeginExternalUIConnection(target); err == nil {
		t.Fatal("BeginExternalUIConnection succeeded after product shutdown")
	}
	if status, err := manager.ReadForOwner(owner); err != nil || status.State != "disconnected" {
		t.Fatalf("shutdown retained or recreated a connection: status=%+v err=%v", status, err)
	}
	principal := commandui.ExternalUIPrincipal{
		Issuer: externalUIFixtureIssuer, Subject: owner.UserID, UserID: owner.UserID,
		AuthContextID: `opaque-test-context-fingerprint`,
	}
	if _, err := manager.Claim(invitation.Invitation, principal); !errors.Is(err, commandui.ErrExternalConnectionDenied) {
		t.Fatalf("shutdown left preexisting invitation claimable: %v", err)
	}
}

func TestExternalUIAppConcurrentBeginAndProductShutdownCannotResurrectInvite(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	a := f.app
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("fixture did not mount the production command product")
	}
	owner := p.owner()
	manager := a.ensureExternalUIConnections()
	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "tab-lifecycle",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "editor", SurfaceID: "surface-lifecycle", SnapshotVersion: "snapshot-v1",
		},
	}
	type beginResult struct {
		invitation commandui.ExternalUIInvitation
		err        error
	}
	const attempts = 4
	start := make(chan struct{})
	ready := make(chan struct{}, attempts+1)
	results := make(chan beginResult, attempts)
	var shutdownErr error
	shutdownDone := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(attempts + 1)
	for i := 0; i < attempts; i++ {
		go func() {
			defer workers.Done()
			ready <- struct{}{}
			<-start
			invitation, err := a.BeginExternalUIConnection(target)
			results <- beginResult{invitation: invitation, err: err}
		}()
	}
	go func() {
		defer workers.Done()
		ready <- struct{}{}
		<-start
		shutdownErr = p.Shutdown(context.Background())
		close(shutdownDone)
	}()
	for i := 0; i < attempts+1; i++ {
		<-ready
	}
	close(start)
	collected := make([]beginResult, 0, attempts)
	for i := 0; i < attempts; i++ {
		collected = append(collected, <-results)
	}
	<-shutdownDone
	workers.Wait()
	if shutdownErr != nil {
		t.Fatalf("command product shutdown: %v", shutdownErr)
	}
	status, err := manager.ReadForOwner(owner)
	if err != nil || status.State != "disconnected" {
		t.Fatalf("concurrent admission left connection state after shutdown: status=%+v err=%v", status, err)
	}
	principal := commandui.ExternalUIPrincipal{
		Issuer: externalUIFixtureIssuer, Subject: owner.UserID, UserID: owner.UserID,
		AuthContextID: `opaque-test-context-fingerprint`,
	}
	for _, result := range collected {
		if result.err != nil {
			continue
		}
		if result.invitation.Invitation == "" {
			t.Fatal("successful concurrent Begin returned an empty invitation")
		}
		if _, err := manager.Claim(result.invitation.Invitation, principal); !errors.Is(err, commandui.ErrExternalConnectionDenied) {
			t.Fatalf("shutdown left concurrently-created invitation claimable: %v", err)
		}
	}
}
