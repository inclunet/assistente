package commandactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

func TestBackLatestEncerraMaisRecenteDaMesmaOrigemEIsolaStack(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, firstLayer, firstRule, layers := serviceFixture(t, &now)
	secondLayer := Ref{Kind: UserRef, ID: activationID(t)}
	secondRule := manualRule(t, owner, secondLayer, LifecyclePersistent)
	layers[secondLayer.ID] = Layer{Ref: secondLayer, UserID: owner.UserID, Enabled: true}
	if err := store.CreateRule(context.Background(), secondRule); err != nil {
		t.Fatal(err)
	}
	originA := Origin{Type: "ui_action", SessionID: "session-back", DeviceID: "keyboard-a"}
	originB := Origin{Type: "ui_action", SessionID: "session-back", DeviceID: "keyboard-b"}
	first, err := service.Pin(context.Background(), owner, firstLayer, Ref{Kind: UserRef, ID: firstRule.ID}, originA, nil)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	second, err := service.Pin(context.Background(), owner, secondLayer, Ref{Kind: UserRef, ID: secondRule.ID}, originA, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Pin(context.Background(), owner, firstLayer, Ref{Kind: UserRef, ID: firstRule.ID}, originB, nil)
	if err != nil {
		t.Fatal(err)
	}
	layers[secondLayer.ID] = Layer{Ref: secondLayer, UserID: owner.UserID, Enabled: false}

	mutation, err := service.BackLatest(context.Background(), owner, originA)
	if err != nil || mutation.Claim.ActivationID != second.Claim.ActivationID {
		t.Fatalf("back latest incorreto: %+v %v", mutation, err)
	}
	loaded, err := store.GetClaim(context.Background(), owner, second.Claim.ActivationID)
	if err != nil || loaded.State != StateDeactivated {
		t.Fatalf("claim mais recente não encerrada: %+v %v", loaded, err)
	}
	loaded, err = store.GetClaim(context.Background(), owner, first.Claim.ActivationID)
	if err != nil || loaded.State != StateActive {
		t.Fatalf("back latest encerrou claim antiga: %+v %v", loaded, err)
	}
	loaded, err = store.GetClaim(context.Background(), owner, other.Claim.ActivationID)
	if err != nil || loaded.State != StateActive {
		t.Fatalf("back latest atravessou manual_stack_key: %+v %v", loaded, err)
	}
	mutation, err = service.BackLatest(context.Background(), owner, originA)
	if err != nil || mutation.Claim.ActivationID != first.Claim.ActivationID {
		t.Fatalf("segunda retirada não encerrou a claim antiga: %+v %v", mutation, err)
	}
	if _, err := service.BackLatest(context.Background(), owner, originA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("terceira retirada deveria não encontrar claim ativa: %v", err)
	}
}

func TestPinEToggleExpiramClaimActiveAntesDeNovoCiclo(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	rule.Lifecycle = LifecycleTemporary
	if err := store.db.Save(&rule).Error; err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "session-expiry", DeviceID: "keyboard-a"}
	firstExpiry := now.Add(time.Minute)
	first, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, &firstExpiry)
	if err != nil {
		t.Fatal(err)
	}
	now = firstExpiry
	secondExpiry := now.Add(time.Minute)
	second, err := service.Toggle(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, &secondExpiry)
	if err != nil || second.Claim.ActivationID == first.Claim.ActivationID {
		t.Fatalf("toggle foi bloqueado por claim vencida: %+v %v", second, err)
	}
	old, err := store.GetClaim(context.Background(), owner, first.Claim.ActivationID)
	if err != nil || old.State != StateExpired {
		t.Fatalf("claim vencida não foi terminalizada: %+v %v", old, err)
	}
	now = secondExpiry
	thirdExpiry := now.Add(time.Minute)
	third, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, &thirdExpiry)
	if err != nil || third.Claim.ActivationID == second.Claim.ActivationID {
		t.Fatalf("pin foi bloqueado por claim vencida: %+v %v", third, err)
	}
	old, err = store.GetClaim(context.Background(), owner, second.Claim.ActivationID)
	if err != nil || old.State != StateExpired {
		t.Fatalf("segunda claim vencida não foi terminalizada: %+v %v", old, err)
	}
}

func TestBackLatestFazRollbackDaClaimENaoBumpQuandoGeracaoFalha(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	origin := Origin{Type: "ui_action", SessionID: "session-rollback", DeviceID: "keyboard-a"}
	created, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	service.ports.GenerationTx = GenerationTxPortFunc(func(context.Context, *gorm.DB, Owner) (GenerationSnapshot, error) {
		return GenerationSnapshot{}, errors.New("falha de geração")
	})
	if _, err := service.BackLatest(context.Background(), owner, origin); err == nil {
		t.Fatal("back latest deveria falhar quando o bump falha")
	}
	claim, err := store.GetClaim(context.Background(), owner, created.Claim.ActivationID)
	if err != nil || claim.State != StateActive {
		t.Fatalf("rollback não preservou claim ativa: %+v %v", claim, err)
	}
}

func TestExpireBumpPorEscopoEIgnoraClaimDeJob(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	db := activationDB(t)
	workspace := "ws-expire"
	globalOwner := activationOwner(t)
	workspaceOwner := globalOwner
	workspaceOwner.WorkspaceID = &workspace
	globalLayer := Ref{Kind: UserRef, ID: activationID(t)}
	localLayer := Ref{Kind: UserRef, ID: activationID(t)}
	layers := map[string]Layer{
		globalLayer.ID: {Ref: globalLayer, UserID: globalOwner.UserID, Enabled: true},
		localLayer.ID:  {Ref: localLayer, UserID: globalOwner.UserID, WorkspaceID: &workspace, Enabled: true},
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(db, &commandsecurity.DispatchGate{}, Ports{
		Owner: OwnerPortFunc(func(_ context.Context, asserted Owner) (Owner, error) { return asserted, nil }),
		Layer: LayerPortFunc(func(_ context.Context, requested Owner, ref Ref) (Layer, error) {
			layer, ok := layers[ref.ID]
			if !ok || layer.UserID != requested.UserID || layer.WorkspaceID != nil && !sameWorkspace(layer.WorkspaceID, requested.WorkspaceID) {
				return Layer{}, ErrNotFound
			}
			return layer, nil
		}),
		Origin: OriginPortFunc(func(_ context.Context, _ Owner, origin Origin) (Origin, error) { return origin, nil }),
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	globalRule := manualRuleAtScope(t, globalOwner, globalLayer)
	localRule := manualRuleAtScope(t, workspaceOwner, localLayer)
	globalRule.Lifecycle, localRule.Lifecycle = LifecycleTemporary, LifecycleTemporary
	if err := store.CreateRule(context.Background(), globalRule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), localRule); err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "session-expire-scopes", DeviceID: "keyboard-a"}
	expires := now.Add(time.Minute)
	stackKey, err := ManualStackKey(origin)
	if err != nil {
		t.Fatal(err)
	}
	globalClaim := Claim{ActivationID: activationID(t), LayerRefKind: globalLayer.Kind, LayerRef: globalLayer.ID, RuleRefKind: globalRule.RuleRefKind, RuleRef: globalRule.RuleRef, UserID: workspaceOwner.UserID, AuthContextType: workspaceOwner.AuthContextType, AuthContextID: workspaceOwner.AuthContextID, AuthGeneration: workspaceOwner.AuthGeneration, SecurityGeneration: workspaceOwner.SecurityGeneration, SourceType: "manual", State: StateActive, ManualStackKey: &stackKey, ActivatedAt: now, ExpiresAt: &expires, UpdatedAt: now}
	localClaim := globalClaim
	localClaim.ActivationID = activationID(t)
	localClaim.LayerRef = localLayer.ID
	localClaim.RuleRef = localRule.RuleRef
	localClaim.WorkspaceID = cloneString(workspaceOwner.WorkspaceID)
	if err := store.WithTx(context.Background(), func(tx *Tx) error {
		if err := tx.InsertClaim(context.Background(), globalClaim); err != nil {
			return err
		}
		return tx.InsertClaim(context.Background(), localClaim)
	}); err != nil {
		t.Fatal(err)
	}
	eventID := activationID(t)
	if err := store.WithTx(context.Background(), func(tx *Tx) error {
		return tx.InsertClaim(context.Background(), Claim{
			ActivationID: eventID, LayerRefKind: localLayer.Kind, LayerRef: localLayer.ID,
			RuleRefKind: localRule.RuleRefKind, RuleRef: localRule.RuleRef, UserID: workspaceOwner.UserID,
			WorkspaceID: cloneString(workspaceOwner.WorkspaceID), AuthContextType: workspaceOwner.AuthContextType,
			AuthContextID: workspaceOwner.AuthContextID, AuthGeneration: workspaceOwner.AuthGeneration,
			SecurityGeneration: workspaceOwner.SecurityGeneration, SourceType: "job", State: StateActive,
			ActivatedAt: now, ExpiresAt: cloneTime(&expires), UpdatedAt: now,
		})
	}); err != nil {
		t.Fatal(err)
	}
	now = expires
	mutation, err := service.Expire(context.Background(), workspaceOwner)
	if err != nil || len(mutation.Generations) != 2 {
		t.Fatalf("expire não publicou os dois escopos: %+v %v", mutation, err)
	}
	globalLoaded, err := store.GetClaim(context.Background(), workspaceOwner, globalClaim.ActivationID)
	if err != nil || globalLoaded.State != StateExpired {
		t.Fatalf("claim global não expirou: %+v %v", globalLoaded, err)
	}
	localLoaded, err := store.GetClaim(context.Background(), workspaceOwner, localClaim.ActivationID)
	if err != nil || localLoaded.State != StateExpired {
		t.Fatalf("claim local não expirou: %+v %v", localLoaded, err)
	}
	eventLoaded, err := store.GetClaim(context.Background(), workspaceOwner, eventID)
	if err != nil || eventLoaded.State != StateActive {
		t.Fatalf("claim de job foi expirada pelo caminho geral: %+v %v", eventLoaded, err)
	}
	globalGeneration, err := store.SnapshotGeneration(context.Background(), globalOwner)
	if err != nil || globalGeneration.Generation != 2 {
		t.Fatalf("geração global incorreta: %+v %v", globalGeneration, err)
	}
	localGeneration, err := store.SnapshotGeneration(context.Background(), workspaceOwner)
	if err != nil || localGeneration.Generation != 2 {
		t.Fatalf("geração local incorreta: %+v %v", localGeneration, err)
	}
}
