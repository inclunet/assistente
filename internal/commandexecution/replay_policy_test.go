package commandexecution

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func TestServiceReplayUsesStoredKeyVersionWithoutFallbackForNewRequest(t *testing.T) {
	f := newExecutionFixture(t)
	request := f.request(t, "workspace.read")
	first, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || first.Status != commandledger.Succeeded {
		t.Fatal(first, err)
	}
	config := f.service.config
	config.KeyVersion = "v2" // Cofre de fixture mantém somente v1.
	reconfigured, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := reconfigured.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || replay.ID != first.ID || replay.Status != commandledger.Succeeded {
		t.Fatal(replay, err)
	}
	if record, err := reconfigured.Execute(context.Background(), f.pair.AccessToken, f.request(t, "workspace.read")); !errors.Is(err, commandledger.ErrFingerprintKeyUnavailable) || record != (commandledger.Record{}) {
		t.Fatal(record, err)
	}
	if f.startCalls.Load() != 1 {
		t.Fatal("fallback/replay iniciou handler")
	}
}

func TestServiceRealPolicyRejectsChangedRoleDespiteValidJWT(t *testing.T) {
	f := newExecutionFixture(t)
	authorize, err := NewLocalReadAuthorizer(f.db, map[string][]string{"workspace.read": {database.UserRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	config := f.service.config
	config.Authorize = authorize
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Execute(context.Background(), f.pair.AccessToken, f.request(t, "workspace.read"))
	if err != nil || first.Status != commandledger.Succeeded {
		t.Fatal(first, err)
	}
	if err := f.epochs.MutateSecurity(context.Background(), func() error { return f.db.Model(f.user).Update("role", database.UserRoleAdmin).Error }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sessions.VerifyAccessToken(f.pair.AccessToken); err != nil {
		t.Fatal("JWT da fixture deixou de ser válido", err)
	}
	denied, err := service.Execute(context.Background(), f.pair.AccessToken, f.request(t, "workspace.read"))
	if err != nil || denied.Status != commandledger.Denied || f.startCalls.Load() != 1 {
		t.Fatal("role antiga do token concedeu acesso", denied, err)
	}
}
