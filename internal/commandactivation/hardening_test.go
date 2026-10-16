package commandactivation

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	hardeningEventName = "command-context.job-run-state.v1"
	hardeningProducers = `["jobs.runtime"]`
)

func eventRule(t *testing.T, owner Owner, layer Ref, enabled bool) Rule {
	t.Helper()
	id := activationID(t)
	eventName, producers := hardeningEventName, hardeningProducers
	return Rule{ID: id, UserID: owner.UserID, WorkspaceID: cloneString(owner.WorkspaceID), LayerRefKind: layer.Kind, LayerRef: layer.ID, RuleRefKind: UserRef, RuleRef: id, Mode: ModeEvent, Condition: "{}", Lifecycle: LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, Enabled: enabled, Source: "user", ReviewStatus: "active"}
}

func TestCreateRuleEventContractRejectsForgedGrantAndKeepsLegacyRows(t *testing.T) {
	db := activationDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	owner := activationOwner(t)
	layer := Ref{Kind: BuiltinRef, ID: "builtin.execution"}
	valid := eventRule(t, owner, layer, false)
	if err := store.CreateRule(context.Background(), valid); err != nil {
		t.Fatalf("event disabled válido: %v", err)
	}

	for name, candidate := range map[string]Rule{
		"event enabled": func() Rule { r := eventRule(t, owner, layer, true); return r }(),
		"decision forged": func() Rule {
			r := eventRule(t, owner, layer, false)
			value := activationID(t)
			r.AuthorizationDecisionID = &value
			return r
		}(),
		"grant id forged": func() Rule {
			r := eventRule(t, owner, layer, false)
			value := activationID(t)
			r.AutomationGrantID = &value
			return r
		}(),
		"grant generation forged": func() Rule {
			r := eventRule(t, owner, layer, false)
			value := int64(1)
			r.AutomationGrantGeneration = &value
			return r
		}(),
		"grant fingerprint forged": func() Rule {
			r := eventRule(t, owner, layer, false)
			value := "grant-fingerprint"
			r.AutomationGrantFingerprint = &value
			return r
		}(),
		"event name aberto": func() Rule {
			r := eventRule(t, owner, layer, false)
			value := "other-event"
			r.EventName = &value
			return r
		}(),
		"producer aberto": func() Rule {
			r := eventRule(t, owner, layer, false)
			value := `["other.producer"]`
			r.AllowedInternalProducerTypes = &value
			return r
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.CreateRule(context.Background(), candidate); !errors.Is(err, ErrInvalid) {
				t.Fatalf("CreateRule = %v, esperado ErrInvalid", err)
			}
		})
	}

	legacy := eventRule(t, owner, layer, true)
	legacy.ID = activationID(t)
	legacy.RuleRef = legacy.ID
	decisionID, grantID, fingerprint := activationID(t), activationID(t), "legacy-grant-fingerprint"
	legacy.AuthorizationDecisionID, legacy.AutomationGrantID, legacy.AutomationGrantFingerprint = &decisionID, &grantID, &fingerprint
	generation := int64(7)
	legacy.AutomationGrantGeneration = &generation
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("inserir legado estrutural: %v", err)
	}
	loaded, err := store.ResolveRule(context.Background(), owner, Ref{Kind: UserRef, ID: legacy.ID})
	if err != nil {
		t.Fatalf("carregar legado: %v", err)
	}
	if loaded.AutomationGrantID == nil || *loaded.AutomationGrantID != grantID || loaded.AutomationGrantGeneration == nil || *loaded.AutomationGrantGeneration != generation {
		t.Fatalf("legado alterado ao carregar: %+v", loaded)
	}
	var count int64
	if err := db.Model(&Rule{}).Where("id IN ?", []string{valid.ID, legacy.ID}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("linhas válidas/legadas removidas: count=%d err=%v", count, err)
	}
}

func TestReconcileContextNuncaAvaliaModeEvent(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	service, store, owner, layer, rule, _ := serviceFixture(t, &now)
	eventName, producers := hardeningEventName, hardeningProducers
	if err := store.db.Model(&Rule{}).Where("id = ?", rule.ID).Updates(map[string]any{"mode": ModeEvent, "event_name": eventName, "allowed_internal_producer_types": producers}).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	service.ports.Context = ContextPortFunc(func(context.Context, Owner, Rule) (ContextResult, error) {
		calls++
		return ContextResult{Active: true, Version: "must-not-run"}, nil
	})
	mutation, err := service.ReconcileContext(context.Background(), owner)
	if err != nil {
		t.Fatalf("ReconcileContext: %v", err)
	}
	if calls != 0 || mutation.Changed || mutation.ActiveLayersChanged {
		t.Fatalf("ModeEvent foi tratado como contextual: calls=%d mutation=%+v", calls, mutation)
	}
	claims, err := store.ListClaims(context.Background(), owner)
	if err != nil || len(claims) != 0 {
		t.Fatalf("ModeEvent criou claim: claims=%+v err=%v", claims, err)
	}
	if generation, err := store.SnapshotGeneration(context.Background(), owner); err != nil || generation.Generation != 1 {
		t.Fatalf("geração alterada por ModeEvent: %+v err=%v", generation, err)
	}
	_ = layer
}

func newHardeningService(t *testing.T, db *gorm.DB, gate *commandsecurity.DispatchGate, now *time.Time, owner Owner, layers map[string]Layer) (*Service, *Store) {
	t.Helper()
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(db, gate, Ports{
		Owner: OwnerPortFunc(func(_ context.Context, asserted Owner) (Owner, error) { return asserted, nil }),
		Layer: LayerPortFunc(func(_ context.Context, owner Owner, ref Ref) (Layer, error) {
			for _, layer := range layers {
				if layer.Ref == ref && layer.UserID == owner.UserID && sameWorkspace(layer.WorkspaceID, owner.WorkspaceID) {
					return layer, nil
				}
			}
			return Layer{}, ErrNotFound
		}),
		Origin: OriginPortFunc(func(_ context.Context, _ Owner, origin Origin) (Origin, error) { return origin, nil }),
	}, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func manualRuleAtScope(t *testing.T, owner Owner, layer Ref) Rule {
	rule := manualRule(t, owner, layer, LifecyclePersistent)
	rule.WorkspaceID = cloneString(owner.WorkspaceID)
	return rule
}

func TestConcurrentPinToggleExpireAndLayerMutationShareGate(t *testing.T) {
	db := activationDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	gate := &commandsecurity.DispatchGate{}
	owner := activationOwner(t)
	layer := Ref{Kind: UserRef, ID: activationID(t)}
	layers := map[string]Layer{layer.ID: {Ref: layer, UserID: owner.UserID, Enabled: true}}
	service, store := newHardeningService(t, db, gate, &now, owner, layers)
	rule := manualRuleAtScope(t, owner, layer)
	if err := store.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "session-concurrent", DeviceID: "keyboard-a"}
	type result struct {
		mutation Mutation
		err      error
	}
	results := make(chan result, 40)
	var wg sync.WaitGroup
	launch := func(fn func() (Mutation, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mutation, err := fn()
			results <- result{mutation: mutation, err: err}
		}()
	}
	for i := 0; i < 12; i++ {
		launch(func() (Mutation, error) {
			return service.Pin(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil)
		})
		launch(func() (Mutation, error) {
			return service.Toggle(context.Background(), owner, layer, Ref{Kind: UserRef, ID: rule.ID}, origin, nil)
		})
	}
	for i := 0; i < 4; i++ {
		launch(func() (Mutation, error) { return service.Expire(context.Background(), owner) })
	}
	launch(func() (Mutation, error) {
		var mutation Mutation
		err := gate.WithMutation(context.Background(), func() error {
			layers[layer.ID] = Layer{Ref: layer, UserID: owner.UserID, Enabled: false}
			return db.Transaction(func(tx *gorm.DB) error {
				var err error
				mutation, err = service.ReconcileLayersTx(context.Background(), tx, owner, []LayerChange{{Ref: layer, BeforePresent: true, AfterPresent: true, BeforeEnabled: true, AfterEnabled: false}})
				return err
			})
		})
		return mutation, err
	})
	wg.Wait()
	close(results)
	expectedGeneration := int64(1)
	for result := range results {
		if result.err != nil && !errors.Is(result.err, ErrLayerDisabled) && !errors.Is(result.err, ErrNotFound) {
			t.Fatalf("operação concorrente: %v", result.err)
		}
		if result.err == nil && result.mutation.ActiveLayersChanged {
			expectedGeneration++
		}
	}
	claims, err := store.ListClaims(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, claim := range claims {
		if claim.State == StateActive && claim.LayerRef == layer.ID && claim.RuleRef == rule.ID {
			active++
		}
	}
	if active > 1 {
		t.Fatalf("claims ativas duplicadas: %d (%+v)", active, claims)
	}
	generation, err := store.SnapshotGeneration(context.Background(), owner)
	if err != nil || generation.Generation != expectedGeneration {
		t.Fatalf("geração não corresponde às mudanças efetivas: got=%+v expected=%d err=%v", generation, expectedGeneration, err)
	}
}

func TestRestartSQLitePreservaTerminaisENaoRessuscita(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activation-restart.db")
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	owner := activationOwner(t)
	open := func() (*gorm.DB, *Store, *Service) {
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewStore(db)
		if err != nil {
			t.Fatal(err)
		}
		service, err := New(db, &commandsecurity.DispatchGate{}, Ports{
			Owner: OwnerPortFunc(func(_ context.Context, asserted Owner) (Owner, error) { return asserted, nil }),
			Layer: LayerPortFunc(func(_ context.Context, _ Owner, ref Ref) (Layer, error) {
				return Layer{Ref: ref, UserID: owner.UserID, Enabled: true}, nil
			}),
			Origin: OriginPortFunc(func(_ context.Context, _ Owner, origin Origin) (Origin, error) { return origin, nil }),
		}, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		return db, store, service
	}
	db, store, service := open()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	persistentLayer := Ref{Kind: UserRef, ID: activationID(t)}
	temporaryLayer := Ref{Kind: UserRef, ID: activationID(t)}
	persistentRule := manualRuleAtScope(t, owner, persistentLayer)
	temporaryRule := manualRuleAtScope(t, owner, temporaryLayer)
	temporaryRule.Lifecycle = LifecycleTemporary
	if err := store.CreateRule(context.Background(), persistentRule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), temporaryRule); err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "restart-session", DeviceID: "keyboard-a"}
	deactivated, err := service.Pin(context.Background(), owner, persistentLayer, Ref{Kind: UserRef, ID: persistentRule.ID}, origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Back(context.Background(), owner, persistentLayer, Ref{Kind: UserRef, ID: persistentRule.ID}, origin); err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Minute)
	expired, err := service.Pin(context.Background(), owner, temporaryLayer, Ref{Kind: UserRef, ID: temporaryRule.ID}, origin, &expires)
	if err != nil {
		t.Fatal(err)
	}
	now = expires
	if _, err := service.Expire(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	db, store, service = open()
	defer func() {
		if conn, err := db.DB(); err == nil {
			_ = conn.Close()
		}
	}()
	claims, err := store.ListClaims(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	assertTerminal := func(id string, state State) {
		t.Helper()
		for _, claim := range claims {
			if claim.ActivationID == id {
				if claim.State != state {
					t.Fatalf("claim %s após restart = %s, esperado %s", id, claim.State, state)
				}
				return
			}
		}
		t.Fatalf("claim %s ausente após restart", id)
	}
	assertTerminal(deactivated.Claim.ActivationID, StateDeactivated)
	assertTerminal(expired.Claim.ActivationID, StateExpired)

	nextPersistent, err := service.Pin(context.Background(), owner, persistentLayer, Ref{Kind: UserRef, ID: persistentRule.ID}, origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	nextExpiry := now.Add(time.Minute)
	nextTemporary, err := service.Pin(context.Background(), owner, temporaryLayer, Ref{Kind: UserRef, ID: temporaryRule.ID}, origin, &nextExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if nextPersistent.Claim.ActivationID == deactivated.Claim.ActivationID || nextTemporary.Claim.ActivationID == expired.Claim.ActivationID {
		t.Fatal("ciclo novo reutilizou claim terminal")
	}
	claims, err = store.ListClaims(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, claim := range claims {
		if claim.ActivationID == deactivated.Claim.ActivationID && claim.State != StateDeactivated || claim.ActivationID == expired.Claim.ActivationID && claim.State != StateExpired {
			t.Fatalf("claim terminal ressuscitada: %+v", claim)
		}
	}
}

func TestWorkspaceOrthogonalityConcurrentPin(t *testing.T) {
	db := activationDB(t)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	gate := &commandsecurity.DispatchGate{}
	global := activationOwner(t)
	workspace := "ws-restart-hex"
	local := global
	local.WorkspaceID = &workspace
	globalLayer := Ref{Kind: UserRef, ID: activationID(t)}
	localLayer := Ref{Kind: UserRef, ID: activationID(t)}
	layers := map[string]Layer{
		globalLayer.ID: {Ref: globalLayer, UserID: global.UserID, Enabled: true},
		localLayer.ID:  {Ref: localLayer, UserID: local.UserID, WorkspaceID: &workspace, Enabled: true},
	}
	service, store := newHardeningService(t, db, gate, &now, global, layers)
	globalRule := manualRuleAtScope(t, global, globalLayer)
	localRule := manualRuleAtScope(t, local, localLayer)
	if err := store.CreateRule(context.Background(), globalRule); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRule(context.Background(), localRule); err != nil {
		t.Fatal(err)
	}
	origin := Origin{Type: "ui_action", SessionID: "workspace-session", DeviceID: "keyboard-a"}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := service.Pin(context.Background(), global, globalLayer, Ref{Kind: UserRef, ID: globalRule.ID}, origin, nil)
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := service.Pin(context.Background(), local, localLayer, Ref{Kind: UserRef, ID: localRule.ID}, origin, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("pin concorrente por workspace: %v", err)
		}
	}
	claims, err := store.ListClaims(context.Background(), local)
	if err != nil {
		t.Fatal(err)
	}
	activeGlobal, activeLocal := 0, 0
	for _, claim := range claims {
		if claim.State != StateActive {
			continue
		}
		if claim.LayerRef == globalLayer.ID && claim.RuleRef == globalRule.ID && claim.WorkspaceID == nil {
			activeGlobal++
		}
		if claim.LayerRef == localLayer.ID && claim.RuleRef == localRule.ID && claim.WorkspaceID != nil && *claim.WorkspaceID == workspace {
			activeLocal++
		}
	}
	if activeGlobal != 1 || activeLocal != 1 {
		t.Fatalf("claims ativas por workspace: global=%d local=%d claims=%+v", activeGlobal, activeLocal, claims)
	}
	globalGeneration, err := store.SnapshotGeneration(context.Background(), global)
	if err != nil || globalGeneration.Generation != 2 {
		t.Fatalf("geração global = %+v err=%v", globalGeneration, err)
	}
	localGeneration, err := store.SnapshotGeneration(context.Background(), local)
	if err != nil || localGeneration.Generation != 2 {
		t.Fatalf("geração local = %+v err=%v", localGeneration, err)
	}
}
