package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestHTTPAPIExternalWorkspaceListDeniesJWTMappedToAnotherOwner(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	other := database.User{
		Username:     "workspace-list-other-owner",
		DisplayName:  "Other owner",
		PasswordHash: "test-only",
		Role:         database.UserRoleUser,
		IsActive:     true,
	}
	if err := database.DB().Create(&other).Error; err != nil {
		t.Fatalf("create separate active test owner: %v", err)
	}
	repository := auth.NewExternalIdentityRepository(database.DB())
	if _, err := repository.Create(t.Context(), auth.ExternalIdentityMappingParams{
		Issuer: externalUIFixtureIssuer, Subject: "workspace-list-other-owner", UserID: other.ID,
	}); err != nil {
		t.Fatalf("map second JWT subject to a different local owner: %v", err)
	}
	token := externalUIJWTForSubject(t, f.signer, other.ID)
	id, correlation := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	body, err := json.Marshal(map[string]any{
		"invocation_id": id, "correlation_id": correlation, "command_id": commandProductWorkspaceListID,
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/commands/ui/execute", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "invocação não encontrada") || strings.Contains(response.Body.String(), `"workspaces"`) {
		t.Fatalf("different mapped owner must not resolve another owner's invocation or workspace data: status=%d body=%s invocation=%s", response.Code, response.Body.String(), id)
	}
}

func TestHTTPAPIExternalWorkspaceListAllowsMatchingLinkedWorkspace(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	owner := f.app.commandProduct.Load().owner()
	target := commandui.ExternalUIDestination{
		WorkspaceID: owner.WorkspaceID,
		TabID:       "workspace-list-linked-tab",
		Surface: commandui.ExternalUISurfaceContext{
			SurfaceType: "chat", SurfaceID: "workspace-list-linked-chat", SnapshotVersion: "snapshot-linked-1",
		},
	}
	invitation, err := f.app.BeginExternalUIConnection(target)
	if err != nil {
		t.Fatalf("begin explicitly linked UI connection: %v", err)
	}
	claimBody, _ := json.Marshal(map[string]string{"invitation": invitation.Invitation})
	claim := externalUIHTTPRequest(f, http.MethodPost, "/auth/external/ui-connections/consume", string(claimBody))
	if claim.Code != http.StatusOK {
		t.Fatalf("claim explicitly linked UI connection: status=%d body=%s", claim.Code, claim.Body.String())
	}
	var connection struct {
		ConnectionID     string `json:"connectionId"`
		Generation       string `json:"generation"`
		TargetSnapshotID string `json:"targetSnapshotId"`
		ContextVersion   string `json:"contextVersion"`
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &connection); err != nil {
		t.Fatal(err)
	}
	id, correlation := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	body, err := json.Marshal(map[string]any{
		"invocation_id": id, "correlation_id": correlation, "command_id": commandProductWorkspaceListID,
		"arguments": map[string]any{}, "connectionId": connection.ConnectionID, "generation": connection.Generation,
		"targetSnapshotId": connection.TargetSnapshotID, "contextVersion": connection.ContextVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := externalUIHTTPRequest(f, http.MethodPost, "/commands/ui/execute", string(body))
	if response.Code != http.StatusOK {
		t.Fatalf("workspace.list with matching explicit UI link: status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		InvocationID string          `json:"invocationId"`
		Status       string          `json:"status"`
		Result       json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.InvocationID != id || result.Status != "succeeded" || !json.Valid(result.Result) || !strings.Contains(string(result.Result), `"workspaces"`) {
		t.Fatalf("workspace.list with matching UI link result = %s", response.Body.String())
	}
}

func TestWorkspaceListRejectsForgedExternalEnvelopeWithoutExecutorProof(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	userID := f.app.commandProduct.Load().principal.UserID
	invocationID, correlationID := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	commandID := commandProductWorkspaceListID
	source := commandcontract.SourceUI
	envelope := &commandcontract.Envelope{
		Version: commandcontract.EnvelopeVersion, InvocationID: invocationID, CorrelationID: correlationID,
		CommandID: &commandID, UserID: &userID, AuthContextType: commandcontract.AuthExternalToken,
		AuthContextID: "forged-but-well-formed-context", AuthGeneration: "forged-auth-generation",
		SecurityGeneration: "forged-security-generation", ActorType: commandcontract.ActorUser,
		ActorID: userID, SourceType: &source, BindingIDs: []string{}, RegistryVersion: commandProductRegistryVersion,
	}
	ctx := database.WithUserID(t.Context(), userID)
	_, _, err := f.app.validateWorkspaceListInvocation(ctx, commandexecution.Invocation{
		Envelope: envelope, ID: invocationID, CorrelationID: correlationID, CommandID: commandID,
		Principal: auth.LocalSessionPrincipal{UserID: userID}, Source: commandcatalog.UI,
	})
	if !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("structurally valid but unadmitted external envelope should be denied, got %v", err)
	}
}

func TestWorkspaceListRevalidatesOwnerAndManagerAfterAsyncRead(t *testing.T) {
	f := newExternalUIAppHTTPFixture(t)
	product := f.app.commandProduct.Load()
	ownerID := product.principal.UserID
	ctx := database.WithUserID(t.Context(), ownerID)
	if err := f.app.revalidateWorkspaceListOwner(ctx, ownerID, true, commandexecution.Invocation{}, product.workspaceMgr); err != nil {
		t.Fatalf("unchanged mounted owner should remain valid after read: %v", err)
	}
	replacement := workspace.NewManager(t.TempDir())
	if err := replacement.Initialize(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	f.app.authMu.Lock()
	original := f.app.workspaceMgr
	f.app.workspaceMgr = replacement
	f.app.authMu.Unlock()
	defer func() {
		f.app.authMu.Lock()
		f.app.workspaceMgr = original
		f.app.authMu.Unlock()
	}()
	if err := f.app.revalidateWorkspaceListOwner(ctx, ownerID, true, commandexecution.Invocation{}, product.workspaceMgr); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("replaced workspace manager must block the pending result before publication, got %v", err)
	}
}

func externalUIJWTForSubject(t *testing.T, signer *auth.TokenSigner, subject string) string {
	t.Helper()
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
		"iss": externalUIFixtureIssuer, "aud": externalUIFixtureAudience, "sub": subject,
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "scope": externalCommandExecuteScope,
		"roles": []string{database.UserRoleUser},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(privateKey), []byte(input)))
}
