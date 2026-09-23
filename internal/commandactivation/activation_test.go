package commandactivation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func activationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "activation.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func activationID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func activationOwner(t *testing.T) Owner {
	t.Helper()
	return Owner{Scope: Scope{UserID: activationID(t)}, AuthContextType: "local_session", AuthContextID: "session-1", AuthGeneration: "auth-1", SecurityGeneration: "security-1"}
}

func activationOrigin(origin Origin) Origin { return origin }

func manualRule(t *testing.T, owner Owner, layer Ref, lifecycle Lifecycle) Rule {
	t.Helper()
	id := activationID(t)
	return Rule{ID: id, UserID: owner.UserID, LayerRefKind: layer.Kind, LayerRef: layer.ID, RuleRefKind: UserRef, RuleRef: id, Mode: ModeManual, Condition: "{}", Lifecycle: lifecycle, Enabled: true, Source: "user", ReviewStatus: "active"}
}

func serviceFixture(t *testing.T, now *time.Time) (*Service, *Store, Owner, Ref, Rule, map[string]Layer) {
	t.Helper()
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	owner := activationOwner(t)
	layer := Ref{Kind: UserRef, ID: activationID(t)}
	rule := manualRule(t, owner, layer, LifecyclePersistent)
	if err := store.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}
	layers := map[string]Layer{layer.ID: {Ref: layer, UserID: owner.UserID, Enabled: true}}
	ports := Ports{
		Owner: OwnerPortFunc(func(context.Context, Owner) (Owner, error) { return owner, nil }),
		Layer: LayerPortFunc(func(_ context.Context, _ Owner, ref Ref) (Layer, error) {
			value, ok := layers[ref.ID]
			if !ok {
				return Layer{}, ErrNotFound
			}
			return value, nil
		}),
		Origin: OriginPortFunc(func(_ context.Context, _ Owner, origin Origin) (Origin, error) { return activationOrigin(origin), nil }),
	}
	service, err := New(db, &commandsecurity.DispatchGate{}, ports, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	return service, store, owner, layer, rule, layers
}

func TestMigratePreservaNULLNosCamposOpcionaisDeRegra(t *testing.T) {
	db := activationDB(t)
	owner := activationOwner(t)
	layer := Ref{Kind: BuiltinRef, ID: "builtin.layer"}
	rule := manualRule(t, owner, layer, LifecyclePersistent)
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Raw(`SELECT COUNT(*) FROM command_layer_activation_rules WHERE event_name IS NULL AND allowed_internal_producer_types IS NULL AND authorization_decision_id IS NULL AND automation_grant_id IS NULL AND automation_grant_generation IS NULL AND automation_grant_fingerprint IS NULL`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("campos opcionais não permaneceram NULL: %d", count)
	}
}

func TestPinToggleBackRespeitamOrigemEClaimsIndependentes(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	ctx := context.Background()
	originA := Origin{Type: "ui_action", SessionID: "session-1", DeviceID: "keyboard-a"}
	originB := Origin{Type: "ui_action", SessionID: "session-1", DeviceID: "keyboard-b"}
	first, err := service.Pin(ctx, owner, layer, Ref{Kind: UserRef, ID: rule.ID}, originA, nil)
	if err != nil || !first.Changed {
		t.Fatalf("pin inicial: %+v %v", first, err)
	}
	second, err := service.Pin(ctx, owner, layer, Ref{Kind: UserRef, ID: rule.ID}, originB, nil)
	if err != nil || !second.Changed {
		t.Fatalf("pin de outra origem: %+v %v", second, err)
	}
	if first.Claim.ManualStackKey == nil || second.Claim.ManualStackKey == nil || *first.Claim.ManualStackKey == *second.Claim.ManualStackKey {
		t.Fatal("origens diferentes compartilharam manual_stack_key")
	}
	if _, err := service.Back(ctx, owner, layer, Ref{Kind: UserRef, ID: rule.ID}, originA); err != nil {
		t.Fatalf("back da origem A: %v", err)
	}
	claims, err := store.ListClaims(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, claim := range claims {
		if claim.State == StateActive {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("back encerrou claim de outra origem: %+v", claims)
	}
	if _, err := service.Back(ctx, owner, layer, Ref{Kind: UserRef, ID: rule.ID}, originA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("back deveria ser isolado por origem: %v", err)
	}
	if _, err := service.Toggle(ctx, owner, layer, Ref{Kind: UserRef, ID: rule.ID}, originB, nil); err != nil {
		t.Fatalf("toggle da origem B: %v", err)
	}
	claims, err = store.ListClaims(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, claim := range claims {
		if claim.ActivationID == second.Claim.ActivationID && claim.State != StateDeactivated {
			t.Fatalf("toggle não encerrou somente a claim B: %+v", claim)
		}
	}
}

func TestPinEToggleRejeitamClaimExistenteDeEpochInvalidado(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, _, owner, layer, rule, _ := serviceFixture(t, &now)
	origin := Origin{Type: "ui_action", SessionID: "session-same", DeviceID: "keyboard-a"}
	if _, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil); err != nil {
		t.Fatal(err)
	}
	invalidated := owner
	invalidated.AuthGeneration = "auth-2"
	invalidated.SecurityGeneration = "security-2"
	service.ports.Owner = OwnerPortFunc(func(context.Context, Owner) (Owner, error) { return invalidated, nil })
	if _, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil); !errors.Is(err, ErrStale) {
		t.Fatalf("pin deveria rejeitar claim de epoch anterior: %v", err)
	}
	if _, err := service.Toggle(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil); !errors.Is(err, ErrStale) {
		t.Fatalf("toggle deveria rejeitar claim de epoch anterior: %v", err)
	}
}

func TestExpireÉIdempotenteENãoRessuscita(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	rule.Lifecycle = LifecycleTemporary
	if err := store.db.Save(&rule).Error; err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Minute)
	created, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, Origin{Type: "ui_action", SessionID: "session-1", DeviceID: "keyboard-a"}, &expires)
	if err != nil {
		t.Fatal(err)
	}
	now = expires
	first, err := service.Expire(context.Background(), owner)
	if err != nil || !first.Changed {
		t.Fatalf("expiração: %+v %v", first, err)
	}
	second, err := service.Expire(context.Background(), owner)
	if err != nil || second.Changed {
		t.Fatalf("expiração não idempotente: %+v %v", second, err)
	}
	claim, err := store.GetClaim(context.Background(), owner, created.Claim.ActivationID)
	if err != nil || claim.State != StateExpired {
		t.Fatalf("claim ressuscitada ou ausente: %+v %v", claim, err)
	}
	nextExpires := expires.Add(time.Minute)
	newClaim, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, Origin{Type: "ui_action", SessionID: "session-1", DeviceID: "keyboard-a"}, &nextExpires)
	if err == nil && newClaim.Claim.ActivationID == created.Claim.ActivationID {
		t.Fatal("novo ciclo reutilizou activation_id terminal")
	}
}

func TestRestorePersistentRebindSóComNovosEpochsEOrigem(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	originA := Origin{Type: "ui_action", SessionID: "session-1", DeviceID: "keyboard-a"}
	created, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, originA, nil)
	if err != nil {
		t.Fatal(err)
	}
	newOwner := owner
	newOwner.AuthContextID = "session-2"
	newOwner.AuthGeneration = "auth-2"
	newOwner.SecurityGeneration = "security-2"
	service.ports.Owner = OwnerPortFunc(func(context.Context, Owner) (Owner, error) { return newOwner, nil })
	originB := Origin{Type: "ui_action", SessionID: "session-2", DeviceID: "keyboard-b"}
	restored, err := service.RestorePersistent(context.Background(), owner, originB)
	if err != nil || !restored.Changed {
		t.Fatalf("restore persistente: %+v %v", restored, err)
	}
	claim, err := store.GetClaim(context.Background(), newOwner, created.Claim.ActivationID)
	if err != nil || claim.State != StateActive || claim.AuthGeneration != newOwner.AuthGeneration {
		t.Fatalf("claim não foi rebindada: %+v %v", claim, err)
	}
	key, err := ManualStackKey(originB)
	if err != nil || claim.ManualStackKey == nil || *claim.ManualStackKey != key {
		t.Fatalf("manual_stack_key não foi derivada da nova origem: %+v", claim.ManualStackKey)
	}

	service.ports.Owner = OwnerPortFunc(func(context.Context, Owner) (Owner, error) {
		newOwner.AuthContextID = "session-3"
		newOwner.AuthGeneration = "auth-3"
		newOwner.SecurityGeneration = "security-3"
		return newOwner, nil
	})
	service.ports.Origin = OriginPortFunc(func(context.Context, Owner, Origin) (Origin, error) { return Origin{}, ErrNotFound })
	if _, err := service.RestorePersistent(context.Background(), owner, originB); err != nil {
		t.Fatalf("origem indisponível deveria reconciliar para revisão: %v", err)
	}
	claim, err = store.GetClaim(context.Background(), newOwner, created.Claim.ActivationID)
	if err != nil || claim.State != StateInactive || claim.TerminalReason == nil || *claim.TerminalReason != "origin_unavailable" {
		t.Fatalf("claim sem origem não ficou inativa para revisão: %+v %v", claim, err)
	}
}

func TestRestoreForWorkspacePreservaClaimEfemeraExataMasRestartNaoPreserva(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	rule.Lifecycle = LifecycleSession
	if err := store.db.Save(&rule).Error; err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "session-current", DeviceID: "keyboard-a"}
	created, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := service.RestoreForWorkspace(context.Background(), owner, origin)
	if err != nil || mutation.Changed {
		t.Fatalf("troca de workspace alterou claim efêmera exata: %+v %v", mutation, err)
	}
	claim, err := store.GetClaim(context.Background(), owner, created.Claim.ActivationID)
	if err != nil || claim.State != StateActive {
		t.Fatalf("claim não preservada na troca de workspace: %+v %v", claim, err)
	}
	if _, err := service.RestorePersistent(context.Background(), owner, origin); err != nil {
		t.Fatal(err)
	}
	claim, err = store.GetClaim(context.Background(), owner, created.Claim.ActivationID)
	if err != nil || claim.State != StateInactive {
		t.Fatalf("restart preservou claim efêmera indevidamente: %+v %v", claim, err)
	}
}

func TestRestorePersistentMistoMantémClaimsAtivasNoMesmoEscopo(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	owner := activationOwner(t)
	globalLayer := Ref{Kind: UserRef, ID: activationID(t)}
	localLayer := Ref{Kind: UserRef, ID: activationID(t)}
	globalRule := manualRule(t, owner, globalLayer, LifecyclePersistent)
	localRule := manualRule(t, owner, localLayer, LifecyclePersistent)
	globalRule.RuleRefKind, globalRule.RuleRef = BuiltinRef, "restore.global"
	localRule.RuleRefKind, localRule.RuleRef = BuiltinRef, "restore.local"
	if err := store.CreateRule(context.Background(), globalRule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), localRule); err != nil {
		t.Fatal(err)
	}
	layers := map[string]Layer{
		globalLayer.ID: {Ref: globalLayer, UserID: owner.UserID, Enabled: true},
		localLayer.ID:  {Ref: localLayer, UserID: owner.UserID, Enabled: true},
	}
	service, err := New(db, &commandsecurity.DispatchGate{}, Ports{
		Owner: OwnerPortFunc(func(context.Context, Owner) (Owner, error) { return owner, nil }),
		Layer: LayerPortFunc(func(_ context.Context, _ Owner, ref Ref) (Layer, error) {
			layer, ok := layers[ref.ID]
			if !ok {
				return Layer{}, ErrNotFound
			}
			return layer, nil
		}),
		Origin: OriginPortFunc(func(_ context.Context, _ Owner, origin Origin) (Origin, error) { return origin, nil }),
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "restore-session-1", DeviceID: "keyboard-a"}
	globalPersistent, err := service.Pin(context.Background(), owner, globalLayer, Ref{Kind: BuiltinRef, ID: globalRule.RuleRef}, origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	localPersistent, err := service.Pin(context.Background(), owner, localLayer, Ref{Kind: BuiltinRef, ID: localRule.RuleRef}, origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	newOwner := owner
	newOwner.AuthContextID, newOwner.AuthGeneration, newOwner.SecurityGeneration = "restore-session-2", "auth-2", "security-2"
	service.ports.Owner = OwnerPortFunc(func(context.Context, Owner) (Owner, error) { return newOwner, nil })
	mutation, err := service.RestorePersistent(context.Background(), owner, Origin{Type: "ui_action", SessionID: "restore-session-2", DeviceID: "keyboard-b"})
	if err != nil {
		t.Fatalf("restore misto: %+v %v", mutation, err)
	}
	if !mutation.Changed || mutation.ActiveLayersChanged || len(mutation.Generations) != 0 {
		t.Fatalf("escopos/gerações incorretos: %+v", mutation)
	}
	claims, err := store.ListClaims(context.Background(), newOwner)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]State{}
	for _, claim := range claims {
		states[claim.ActivationID] = claim.State
	}
	if states[globalPersistent.Claim.ActivationID] != StateActive || states[localPersistent.Claim.ActivationID] != StateActive {
		t.Fatalf("restore reativou/encerrou claim errada: %+v", states)
	}
}

func TestRestorePersistentResolveRegraRealPreservaGlobalEWorkspaceSemTocarOutro(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	baseOwner := activationOwner(t)
	currentWorkspace := "workspace-current"
	otherWorkspace := "workspace-other"
	globalOwner := baseOwner
	globalOwner.WorkspaceID = nil
	currentOwner := baseOwner
	currentOwner.WorkspaceID = &currentWorkspace
	otherOwner := baseOwner
	otherOwner.WorkspaceID = &otherWorkspace

	globalLayer := Ref{Kind: UserRef, ID: activationID(t)}
	currentLayer := Ref{Kind: UserRef, ID: activationID(t)}
	otherLayer := Ref{Kind: UserRef, ID: activationID(t)}
	globalRule := manualRule(t, globalOwner, globalLayer, LifecyclePersistent)
	currentRule := manualRule(t, currentOwner, currentLayer, LifecyclePersistent)
	otherRule := manualRule(t, otherOwner, otherLayer, LifecyclePersistent)
	currentRule.WorkspaceID = &currentWorkspace
	otherRule.WorkspaceID = &otherWorkspace
	if err := store.CreateRule(context.Background(), globalRule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), currentRule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), otherRule); err != nil {
		t.Fatal(err)
	}
	layers := map[string]Layer{
		globalLayer.ID:  {Ref: globalLayer, UserID: baseOwner.UserID, Enabled: true},
		currentLayer.ID: {Ref: currentLayer, UserID: baseOwner.UserID, WorkspaceID: &currentWorkspace, Enabled: true},
		otherLayer.ID:   {Ref: otherLayer, UserID: baseOwner.UserID, WorkspaceID: &otherWorkspace, Enabled: true},
	}
	owners := map[string]Owner{
		"":               globalOwner,
		currentWorkspace: currentOwner,
		otherWorkspace:   otherOwner,
	}
	service, err := New(db, &commandsecurity.DispatchGate{}, Ports{
		Owner: OwnerPortFunc(func(_ context.Context, asserted Owner) (Owner, error) {
			owner, ok := owners[workspaceKey(asserted.WorkspaceID)]
			if !ok {
				return Owner{}, ErrForeignOwner
			}
			return owner, nil
		}),
		Layer: LayerPortFunc(func(_ context.Context, owner Owner, ref Ref) (Layer, error) {
			for _, layer := range layers {
				if layer.Ref == ref && layer.UserID == owner.UserID && sameWorkspace(layer.WorkspaceID, owner.WorkspaceID) {
					return layer, nil
				}
			}
			return Layer{}, ErrNotFound
		}),
		// RulePort fica omitido de propósito: New instala o Store real.
		Origin: OriginPortFunc(func(_ context.Context, _ Owner, origin Origin) (Origin, error) { return origin, nil }),
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	oldOrigin := Origin{Type: "ui_action", SessionID: "restore-old", DeviceID: "keyboard-a"}
	globalClaim, err := service.Pin(context.Background(), globalOwner, globalLayer, Ref{Kind: UserRef, ID: globalRule.ID}, oldOrigin, nil)
	if err != nil {
		t.Fatal(err)
	}
	currentClaim, err := service.Pin(context.Background(), currentOwner, currentLayer, Ref{Kind: UserRef, ID: currentRule.ID}, oldOrigin, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherClaim, err := service.Pin(context.Background(), otherOwner, otherLayer, Ref{Kind: UserRef, ID: otherRule.ID}, oldOrigin, nil)
	if err != nil {
		t.Fatal(err)
	}

	newCurrentOwner := currentOwner
	newCurrentOwner.AuthContextID = "restore-new-session"
	newCurrentOwner.AuthGeneration = "restore-auth-2"
	newCurrentOwner.SecurityGeneration = "restore-security-2"
	owners[currentWorkspace] = newCurrentOwner
	mutation, err := service.RestorePersistent(context.Background(), currentOwner, Origin{Type: "ui_action", SessionID: "restore-new-session", DeviceID: "keyboard-b"})
	if err != nil {
		t.Fatalf("restore global+workspace: %+v %v", mutation, err)
	}

	for _, want := range []struct {
		owner Owner
		claim Claim
	}{
		{newCurrentOwner, globalClaim.Claim},
		{newCurrentOwner, currentClaim.Claim},
	} {
		claim, err := store.GetClaim(context.Background(), want.owner, want.claim.ActivationID)
		if err != nil {
			t.Fatalf("claim restaurada ausente (%s): %v", want.claim.ActivationID, err)
		}
		if claim.State != StateActive || claim.AuthContextID != newCurrentOwner.AuthContextID || claim.AuthGeneration != newCurrentOwner.AuthGeneration {
			t.Fatalf("claim global/workspace não restaurada: %+v", claim)
		}
	}
	untouched, err := store.GetClaim(context.Background(), otherOwner, otherClaim.Claim.ActivationID)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.State != StateActive || untouched.AuthContextID != otherClaim.Claim.AuthContextID || untouched.AuthGeneration != otherClaim.Claim.AuthGeneration {
		t.Fatalf("claim de outro workspace foi alterada: %+v", untouched)
	}
}

func TestReconcileContextSóMudaGeraçãoQuandoConjuntoMuda(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, _, rule, _ := serviceFixture(t, &now)
	rule.Mode = ModeContext
	if err := store.db.Save(&rule).Error; err != nil {
		t.Fatal(err)
	}
	active := true
	version := "context-1"
	service.ports.Context = ContextPortFunc(func(context.Context, Owner, Rule) (ContextResult, error) {
		return ContextResult{Active: active, Version: version}, nil
	})
	first, err := service.ReconcileContext(context.Background(), owner)
	if err != nil || !first.ActiveLayersChanged || len(first.Generations) != 1 {
		t.Fatalf("ativação contextual: %+v %v", first, err)
	}
	version = "context-2"
	second, err := service.ReconcileContext(context.Background(), owner)
	if err != nil || second.ActiveLayersChanged || len(second.Generations) != 0 {
		t.Fatalf("mudança de versão alterou geração efetiva: %+v %v", second, err)
	}
	active = false
	third, err := service.ReconcileContext(context.Background(), owner)
	if err != nil || !third.ActiveLayersChanged || len(third.Generations) != 1 {
		t.Fatalf("desativação contextual: %+v %v", third, err)
	}
}

func TestReconcileLayersTxMantémClaimNoDisableEReverteGeração(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	claim, err := service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, Origin{Type: "ui_action", SessionID: "session-1", DeviceID: "keyboard-a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := store.SnapshotGeneration(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	change := []LayerChange{{Ref: layer, BeforePresent: true, AfterPresent: true, BeforeEnabled: true, AfterEnabled: false}}
	var returned Mutation
	rollback := errors.New("rollback de teste")
	if err := store.db.Transaction(func(tx *gorm.DB) error {
		var err error
		returned, err = service.ReconcileLayersTx(context.Background(), tx, owner, change)
		if err != nil {
			return err
		}
		if !returned.ActiveLayersChanged || len(returned.Generations) != 1 {
			t.Fatalf("hook não marcou geração: %+v", returned)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal("rollback de teste não ocorreu")
	}
	restored, err := store.GetClaim(context.Background(), owner, claim.Claim.ActivationID)
	if err != nil || restored.State != StateActive {
		t.Fatalf("claim não sobreviveu ao rollback: %+v %v", restored, err)
	}
	generation, err := store.SnapshotGeneration(context.Background(), owner)
	if err != nil || generation.Generation != baseline.Generation {
		t.Fatalf("contador avançou fora do rollback: %+v %v", generation, err)
	}

	if _, err := service.ReconcileLayersTx(context.Background(), store.db, owner, []LayerChange{{Ref: layer, BeforePresent: true, AfterPresent: false, BeforeEnabled: false, AfterEnabled: false}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("hook fora de transação = %v, esperado ErrInvalid", err)
	}
}
