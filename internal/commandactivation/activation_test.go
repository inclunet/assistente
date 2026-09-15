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
