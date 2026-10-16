package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func commandJobConditionRule(a *App, condition string) commandactivation.Rule {
	workspaceID := a.workspaceMgr.Active().ID
	return commandactivation.Rule{
		ID:           uuid.Must(uuid.NewV7()).String(),
		UserID:       a.currentUserID,
		WorkspaceID:  &workspaceID,
		Condition:    condition,
		Enabled:      true,
		Source:       "user",
		ReviewStatus: "active",
		Mode:         commandactivation.ModeCondition,
		LayerRefKind: commandactivation.UserRef,
		RuleRefKind:  commandactivation.UserRef,
		LayerRef:     uuid.Must(uuid.NewV7()).String(),
		RuleRef:      uuid.Must(uuid.NewV7()).String(),
		Lifecycle:    commandactivation.LifecyclePersistent,
	}
}

func TestCommandJobLayerUsesOnlyEnabledOwnedUserLayer(t *testing.T) {
	tests := []struct {
		name      string
		insert    bool
		layerUser string
		enabled   bool
		want      bool
	}{
		{name: "ativa do usuario", insert: true, enabled: true, want: true},
		{name: "desabilitada", insert: true, enabled: false, want: false},
		{name: "ausente", want: false},
		{name: "foreign", insert: true, layerUser: uuid.Must(uuid.NewV7()).String(), enabled: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := commandMaintenanceAppFixture(t)
			db := database.DB()
			layerID := uuid.Must(uuid.NewV7()).String()
			layerUser := a.currentUserID
			if tt.layerUser != "" {
				layerUser = tt.layerUser
			}
			if tt.insert {
				if err := db.Create(&commandconfig.Layer{ID: layerID, UserID: layerUser, Name: "fixture", Enabled: tt.enabled, Source: "user"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			rule := commandactivation.Rule{Enabled: true, ReviewStatus: "active", UserID: a.currentUserID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID}
			got, err := a.commandJobLayer(context.Background(), db, commandactivation.Owner{Scope: commandactivation.Scope{UserID: a.currentUserID}}, rule)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("camada: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommandJobConditionUsesAuthoritativeProfileAndRejectsUnknownOrInvalid(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	if err := a.workspaceMgr.SetProfile("profile-fixture"); err != nil {
		t.Fatal(err)
	}
	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: a.currentUserID, WorkspaceID: func() *string { id := a.workspaceMgr.Active().ID; return &id }()}, AuthContextID: a.currentAuthUser.SessionID}
	db := database.DB()

	tests := []struct {
		name      string
		condition string
		want      bool
		wantErr   error
	}{
		{name: "perfil correspondente", condition: `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"profile-fixture"}]}`, want: true},
		{name: "perfil diferente", condition: `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"outro"}]}`, want: false},
		{name: "foco ausente não significa false", condition: `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":false}]}`, want: false},
		{name: "surface não inferida", condition: `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"}]}`, want: false},
		{name: "condição vazia", condition: `{"version":1,"clauses":[]}`, want: true},
		{name: "campo desconhecido", condition: `{"version":1,"clauses":[{"field":"surface.focused","op":"eq","value":true}]}`, want: false, wantErr: commandconfig.ErrInvalid},
		{name: "json invalido", condition: `{`, want: false, wantErr: commandconfig.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := commandJobConditionRule(a, tt.condition)
			var got bool
			err := a.withCommandJobContext(context.Background(), func(ctx context.Context) error {
				var conditionErr error
				got, conditionErr = a.commandJobCondition(ctx, db, owner, rule, structFact())
				return conditionErr
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("erro: got %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("condição: got %v, want %v", got, tt.want)
			}
		})
	}

	foreign := owner
	foreign.UserID = uuid.Must(uuid.NewV7()).String()
	rule := commandJobConditionRule(a, `{"version":1,"clauses":[]}`)
	if got, err := a.commandJobCondition(context.Background(), db, foreign, rule, structFact()); err != nil || got {
		t.Fatalf("owner foreign: got %v, err %v", got, err)
	}
}

func structFact() commandjobevents.Fact { return commandjobevents.Fact{} }

func TestCommandJobLayerUsesPublishedBuiltinCatalog(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: a.currentUserID}, AuthContextID: a.currentAuthUser.SessionID}
	for _, ref := range []string{commandPaletteLayerID, "missing.builtin"} {
		rule := commandactivation.Rule{UserID: owner.UserID, Enabled: true, ReviewStatus: "active", LayerRefKind: commandactivation.BuiltinRef, LayerRef: ref}
		err := a.withCommandJobContext(context.Background(), func(ctx context.Context) error {
			got, err := a.commandJobLayer(ctx, database.DB(), owner, rule)
			if err != nil {
				return err
			}
			if got != (ref == commandPaletteLayerID) {
				t.Errorf("builtin %s: %v", ref, got)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommandJobContextDoesNotRetryTransactionFailureAsMissingSnapshot(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	want := errors.New("transaction failed")
	calls := 0
	err := a.withCommandJobContext(context.Background(), func(ctx context.Context) error {
		calls++
		state, ok := ctx.Value(commandJobWorkspaceKey{}).(commandJobWorkspace)
		if !ok || !state.available {
			t.Fatal("snapshot válido ausente")
		}
		return want
	})
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = a.withCommandJobContext(ctx, func(context.Context) error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancelamento err=%v calls=%d", err, calls)
	}
}
