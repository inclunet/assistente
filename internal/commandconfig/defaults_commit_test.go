package commandconfig

import (
	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"bytes"
	"context"
	"testing"
	"time"
)

func TestDefaultsUpgradeAndRebaseShareConfirmedCommit(t *testing.T) {
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-one", "1", "fp-one", "active", completeProjectionCondition)
	ctx := context.Background()
	if err := commanddecision.Migrate(ctx, store.db); err != nil {
		t.Fatal(err)
	}
	receipts, err := commanddecision.New(store.db, mutationPresenterFunc(func(_ context.Context, r commanddecision.Request) (commanddecision.Response, error) {
		return commanddecision.Response{DecisionID: r.DecisionID, ActionID: commanddecision.ApplyAction}, nil
	}), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	gate := &commandsecurity.DispatchGate{}
	epochs, err := commandsecurity.NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	version := "v2"
	s, err := NewCompleteMutationService(MutationServiceConfig{Store: store, Sessions: mutationSessionFixture{auth.LocalSessionPrincipal{UserID: scope.UserID, SessionID: storeTestUUID7(t)}}, Epochs: epochs, Receipts: receipts, Keys: func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{2}, 32), nil }, KeyVersion: "v1", DecisionTTL: time.Minute, Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil }, Version: func(context.Context) (string, error) { return version, nil }, Render: func(MutationDiff) (string, error) { return "diff de default", nil }, OnMutationTx: completeNoopHook}, func(context.Context, Scope) (CompleteProjection, error) { return options, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpgradeDefaults(ctx, "token", nil); err != nil {
		t.Fatal(err)
	}
	if err := gate.WithMutation(ctx, func() error {
		options.BuiltinLayers[0].Defaults[0].Version = "3"
		options.BuiltinLayers[0].Defaults[0].Fingerprint = "fp-two"
		version = "v3"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpgradeDefaults(ctx, "token", nil); err != nil {
		t.Fatal(err)
	}
	request := DefaultRebaseRequest{BindingID: row.ID, Default: DefaultReference{ID: *row.ReplacesDefaultID, Version: "3", Fingerprint: "fp-two"}, Condition: commandbindings.Facts{}}
	if _, err := s.RebaseDefault(ctx, "token", nil, request); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Bindings) != 1 || snapshot.Bindings[0].ID != row.ID || snapshot.Bindings[0].ReviewStatus != "active" || *snapshot.Bindings[0].ReplacesDefaultFingerprint != "fp-two" {
		t.Fatal("rebase perdeu override/identidade")
	}
	var count int64
	if err := store.db.Table("command_config_mutations").Where("operation IN ?", []string{"default_upgrade", "default_rebase"}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("auditoria defaults: %d %v", count, err)
	}
}
