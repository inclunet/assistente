package commandconfig

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

type activationHookFixture struct {
	config *Store
	db     *gorm.DB
	base   decisionTestFixture
	act    *commandactivation.Service
	grants *commandautomation.Store
	owner  commandactivation.Owner
	layer  Layer
}

func newActivationHookFixture(t *testing.T) activationHookFixture {
	t.Helper()
	base := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	ctx := context.Background()
	if err := commandactivation.Migrate(ctx, base.projection.db); err != nil {
		t.Fatalf("commandactivation.Migrate: %v", err)
	}
	if err := commandautomation.Migrate(ctx, base.projection.db); err != nil {
		t.Fatalf("commandautomation.Migrate: %v", err)
	}
	layer := storeTestLayer(t, base.projection.db, base.projection.scope.UserID, nil, "activation-hook-global")
	now := time.Now().UTC().Truncate(time.Millisecond)
	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: base.projection.scope.UserID}, AuthContextType: "local_session", AuthContextID: "activation-session", AuthGeneration: "auth-generation-fixture", SecurityGeneration: "security-generation-fixture"}
	act, err := commandactivation.New(base.projection.db, &commandsecurity.DispatchGate{}, commandactivation.Ports{
		Owner: commandactivation.OwnerPortFunc(func(context.Context, commandactivation.Owner) (commandactivation.Owner, error) { return owner, nil }),
		Layer: commandactivation.LayerPortFunc(func(context.Context, commandactivation.Owner, commandactivation.Ref) (commandactivation.Layer, error) {
			return commandactivation.Layer{}, commandactivation.ErrNotFound
		}),
		Origin: commandactivation.OriginPortFunc(func(context.Context, commandactivation.Owner, commandactivation.Origin) (commandactivation.Origin, error) {
			return commandactivation.Origin{}, commandactivation.ErrNotFound
		}),
	}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("commandactivation.New: %v", err)
	}
	grants, err := commandautomation.New(base.projection.db, func() time.Time { return now })
	if err != nil {
		t.Fatalf("commandautomation.New: %v", err)
	}
	return activationHookFixture{config: base.projection.store, db: base.projection.db, base: base, act: act, grants: grants, owner: owner, layer: layer}
}

func (f activationHookFixture) hook(t *testing.T) MutationTxHook {
	t.Helper()
	hook, err := NewActivationMutationHook(f.act, f.grants, func(context.Context, Scope) (commandactivation.Owner, error) {
		return f.owner, nil
	})
	if err != nil {
		t.Fatalf("NewActivationMutationHook: %v", err)
	}
	return hook
}

func insertActivationClaim(t *testing.T, db *gorm.DB, owner commandactivation.Owner, layerID, ruleID string) string {
	t.Helper()
	activationID := storeTestUUID7(t)
	stack := "manual:test-stack"
	now := time.Now().UTC()
	store, err := commandactivation.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		view, err := store.BindTx(tx)
		if err != nil {
			return err
		}
		return view.InsertClaim(context.Background(), commandactivation.Claim{
			ActivationID: activationID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
			RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, UserID: owner.UserID,
			WorkspaceID: cloneWorkspace(owner.WorkspaceID), AuthContextType: owner.AuthContextType,
			AuthContextID: owner.AuthContextID, AuthGeneration: owner.AuthGeneration,
			SecurityGeneration: owner.SecurityGeneration, SourceType: "manual",
			State: commandactivation.StateActive, ManualStackKey: &stack, ActivatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("inserir claim: %v", err)
	}
	return activationID
}

func insertAutomationGrant(t *testing.T, db *gorm.DB, owner commandautomation.Owner, layerID, ruleID string) string {
	t.Helper()
	id, decisionID := storeTestUUID7(t), storeTestUUID7(t)
	workspace := any(nil)
	if owner.WorkspaceID != nil {
		workspace = *owner.WorkspaceID
	}
	err := db.Exec(`INSERT INTO command_layer_automation_grants
        (id,user_id,workspace_id,layer_ref_kind,layer_ref,rule_ref_kind,rule_ref,rule_fingerprint,event_name,producer_types_fingerprint,automation_grant_generation,automation_grant_fingerprint,authorization_decision_id,granted_at,granted_by)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, owner.UserID, workspace, "user", layerID, "user", ruleID, "rule-fingerprint", commandautomation.JobRunStateEvent, "producer-fingerprint", 1, "grant-fingerprint", decisionID, time.Now().UTC(), owner.UserID).Error
	if err != nil {
		t.Fatalf("inserir grant: %v", err)
	}
	return id
}

func grantRevokedAt(t *testing.T, db *gorm.DB, id string) sql.NullTime {
	t.Helper()
	var value sql.NullTime
	if err := db.Raw("SELECT revoked_at FROM command_layer_automation_grants WHERE id = ?", id).Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	return value
}

func prepareLayerMutation(t *testing.T, f activationHookFixture, scope Scope, layerID string, operation Operation) *ConfirmedMutation {
	t.Helper()
	p, err := f.config.PrepareMutation(context.Background(), scope, MutationIntent{Operation: operation, ID: layerID}, completeTestValidator)
	if err != nil {
		t.Fatalf("PrepareMutation: %v", err)
	}
	confirmed, err := f.config.ConfirmMutation(context.Background(), p, f.base.epoch, f.base.receipts, "v1", func(context.Context, string) ([]byte, error) {
		return bytes.Repeat([]byte{0x42}, 32), nil
	}, time.Now().Add(time.Minute), func(MutationDiff) (string, error) { return "layer mutation", nil })
	if err != nil {
		t.Fatalf("ConfirmMutation: %v", err)
	}
	return confirmed
}

func TestActivationHookDisableRevogaGrantEAvancaGeracaoNoMesmoCommit(t *testing.T) {
	f := newActivationHookFixture(t)
	layer := f.layer
	ruleID := storeTestUUID7(t)
	claimID := insertActivationClaim(t, f.db, f.owner, layer.ID, ruleID)
	grantID := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, layer.ID, ruleID)
	confirmed := prepareLayerMutation(t, f, f.base.projection.scope, layer.ID, LayerDisable)
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, f.hook(t)); err != nil {
		t.Fatalf("CommitConfirmedMutation: %v", err)
	}
	if !grantRevokedAt(t, f.db, grantID).Valid {
		t.Fatal("grant ativo após disable")
	}
	generation, err := f.act.Store().SnapshotGeneration(context.Background(), f.owner)
	if err != nil || generation.Generation != 2 {
		t.Fatalf("geração de ativação = %+v, erro=%v", generation, err)
	}
	previewDiff := confirmed.prepared.Diff()
	var previewClaim commandactivation.Claim
	for _, candidate := range previewDiff.AfterActivationClaims {
		if candidate.ActivationID == claimID {
			previewClaim = candidate
			break
		}
	}
	claim, err := f.act.Store().GetClaim(context.Background(), f.owner, claimID)
	if err != nil || previewClaim.ActivationID == "" || claim.State != previewClaim.State || !sameOptionalString(claim.TerminalReason, previewClaim.TerminalReason) || !claim.UpdatedAt.Equal(previewClaim.UpdatedAt) {
		t.Fatalf("claim preview/banco divergente: preview=%+v banco=%+v erro=%v", previewClaim, claim, err)
	}
	previewGrant := commandautomation.Grant{}
	for _, candidate := range previewDiff.AfterAutomationGrants {
		if candidate.ID == grantID {
			previewGrant = candidate
			break
		}
	}
	actualGrantAt := grantRevokedAt(t, f.db, grantID)
	if previewGrant.RevokedAt == nil || !actualGrantAt.Valid || !actualGrantAt.Time.Equal(*previewGrant.RevokedAt) {
		t.Fatalf("grant preview/banco divergente: preview=%+v banco=%v", previewGrant.RevokedAt, actualGrantAt)
	}
	if got := loadDecisionReceiptProbe(t, f.db, confirmed.request.DecisionID); got.Status != commanddecision.Consumed {
		t.Fatalf("receipt = %q, esperado consumed", got.Status)
	}
}

func TestCompleteServiceLayerDisableValidaRegraDesabilitadaEResultadoExato(t *testing.T) {
	f := newActivationHookFixture(t)
	if err := f.db.Where("id = ?", f.base.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	ruleID := storeTestUUID7(t)
	grantID := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, f.layer.ID, ruleID)
	var decisionID string
	if err := f.db.Table("command_layer_automation_grants").Where("id = ?", grantID).Pluck("authorization_decision_id", &decisionID).Error; err != nil {
		t.Fatal(err)
	}
	eventName, producers := commandautomation.JobRunStateEvent, `["jobs.runtime"]`
	generation := int64(1)
	grantFingerprint := "grant-fingerprint"
	rule := commandactivation.Rule{ID: ruleID, UserID: f.owner.UserID, LayerRefKind: commandactivation.UserRef, LayerRef: f.layer.ID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent, Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName, AllowedInternalProducerTypes: &producers, AuthorizationDecisionID: &decisionID, AutomationGrantID: &grantID, AutomationGrantGeneration: &generation, AutomationGrantFingerprint: &grantFingerprint, Enabled: true, Source: "user", ReviewStatus: "active"}
	if err := f.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	claimID := insertActivationClaim(t, f.db, f.owner, f.layer.ID, ruleID)
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store: f.config, Sessions: mutationSessionFixture{auth.LocalSessionPrincipal{UserID: f.base.epoch.UserID, SessionID: f.base.epoch.SessionID}}, Epochs: epochs,
		Receipts: f.base.receipts, Keys: func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil }, KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil }, Version: func(context.Context) (string, error) { return "catalog-v1", nil },
		Render: func(MutationDiff) (string, error) { return "layer disable", nil }, OnMutationTx: f.hook(t),
	}, func(context.Context, Scope) (CompleteProjection, error) { return options, nil })
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.Apply(context.Background(), "token", nil, MutationIntent{Operation: LayerDisable, ID: f.layer.ID})
	if err != nil {
		t.Fatalf("LayerDisable com validator completo: %v", err)
	}
	var afterRule commandactivation.Rule
	if err := f.db.Where("id = ?", ruleID).Take(&afterRule).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := f.act.Store().GetClaim(context.Background(), f.owner, claimID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRule.Enabled || afterRule.AutomationGrantID != nil || claim.State != commandactivation.StateStale || claim.TerminalReason == nil || *claim.TerminalReason != "rule_disabled" {
		t.Fatalf("estado aplicado inesperado: rule=%+v claim=%+v", afterRule, claim)
	}
	var previewRule commandactivation.Rule
	var previewClaim commandactivation.Claim
	for _, candidate := range diff.AfterActivationRules {
		if candidate.ID == ruleID {
			previewRule = candidate
		}
	}
	for _, candidate := range diff.AfterActivationClaims {
		if candidate.ActivationID == claimID {
			previewClaim = candidate
		}
	}
	if previewRule.ID == "" || previewRule.Enabled || previewRule.AutomationGrantID != nil || previewClaim.ActivationID == "" || previewClaim.State != claim.State || !sameOptionalString(previewClaim.TerminalReason, claim.TerminalReason) || !previewClaim.UpdatedAt.Equal(claim.UpdatedAt) {
		t.Fatalf("preview/banco divergente: rule=%+v claimPreview=%+v claimBanco=%+v", previewRule, previewClaim, claim)
	}
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestProjectLayerDisablePreservaRegrasManualEContextNoReenable(t *testing.T) {
	user, layerID := storeTestUUID7(t), storeTestUUID7(t)
	manualRuleID, contextRuleID := storeTestUUID7(t), storeTestUUID7(t)
	before := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{{ID: layerID, UserID: user, Enabled: true}}, ActivationRules: []commandactivation.Rule{
		{ID: manualRuleID, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: manualRuleID, Mode: commandactivation.ModeManual, Enabled: true},
		{ID: contextRuleID, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: contextRuleID, Mode: commandactivation.ModeContext, Enabled: true},
	}, ActivationClaims: []commandactivation.Claim{
		{ActivationID: storeTestUUID7(t), UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: manualRuleID, State: commandactivation.StateActive},
		{ActivationID: storeTestUUID7(t), UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: contextRuleID, State: commandactivation.StateActive},
	}}
	disabled := cloneConfigSnapshot(before)
	disabled.Layers[0].Enabled = false
	projectActivationEffects(before, &disabled, LayerDisable, time.Now().UTC())
	for _, rule := range disabled.ActivationRules {
		if !rule.Enabled {
			t.Fatalf("regra não-evento desabilitada: %+v", rule)
		}
	}
	for _, claim := range disabled.ActivationClaims {
		if claim.State != commandactivation.StateActive || claim.TerminalReason != nil {
			t.Fatalf("claim não-evento alterada no disable: %+v", claim)
		}
	}
	reenabled := cloneConfigSnapshot(disabled)
	reenabled.Layers[0].Enabled = true
	projectActivationEffects(disabled, &reenabled, LayerEnable, time.Now().UTC())
	for _, rule := range reenabled.ActivationRules {
		if !rule.Enabled {
			t.Fatalf("regra não-evento permaneceu desabilitada no reenable: %+v", rule)
		}
	}
}

func TestActivationHookFalhaDesfazConfigGrantGeracaoEReceipt(t *testing.T) {
	f := newActivationHookFixture(t)
	layer := f.layer
	ruleID := storeTestUUID7(t)
	insertActivationClaim(t, f.db, f.owner, layer.ID, ruleID)
	grantID := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, layer.ID, ruleID)
	confirmed := prepareLayerMutation(t, f, f.base.projection.scope, layer.ID, LayerDisable)
	base := f.hook(t)
	want := errors.New("falha do host depois da reconciliação")
	hook := func(ctx context.Context, tx *gorm.DB, diff MutationDiff) error {
		if err := base(ctx, tx, diff); err != nil {
			return err
		}
		return want
	}
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, hook); !errors.Is(err, want) {
		t.Fatalf("erro do hook = %v", err)
	}
	var persisted Layer
	if err := f.db.Where("id = ?", layer.ID).Take(&persisted).Error; err != nil || !persisted.Enabled {
		t.Fatalf("layer não voltou no rollback: %+v %v", persisted, err)
	}
	if grantRevokedAt(t, f.db, grantID).Valid {
		t.Fatal("revogação sobreviveu ao rollback")
	}
	generation, err := f.act.Store().SnapshotGeneration(context.Background(), f.owner)
	if err != nil || generation.Generation != 1 {
		t.Fatalf("geração avançou no rollback: %+v %v", generation, err)
	}
	if got := loadDecisionReceiptProbe(t, f.db, confirmed.request.DecisionID); got.Status != commanddecision.Accepted {
		t.Fatalf("receipt após rollback = %q", got.Status)
	}
}

func TestCommitRejeitaAggregateDivergenteDoPreviewERollaReceipt(t *testing.T) {
	f := newActivationHookFixture(t)
	claimID := insertActivationClaim(t, f.db, f.owner, f.layer.ID, storeTestUUID7(t))
	grantID := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, f.layer.ID, storeTestUUID7(t))
	confirmed := prepareLayerMutation(t, f, f.base.projection.scope, f.layer.ID, LayerDisable)
	base := f.hook(t)
	hook := func(ctx context.Context, tx *gorm.DB, diff MutationDiff) error {
		if err := base(ctx, tx, diff); err != nil {
			return err
		}
		// Simula uma transição dinâmica que o preview não poderia afirmar.
		return tx.Model(&commandactivation.Claim{}).Where("activation_id = ?", claimID).Updates(map[string]any{"state": commandactivation.StateStale, "terminal_reason": "late_context"}).Error
	}
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, hook); !errors.Is(err, ErrStale) {
		t.Fatalf("aggregate divergente aceito: %v", err)
	}
	var layer Layer
	if err := f.db.Where("id = ?", f.layer.ID).Take(&layer).Error; err != nil || !layer.Enabled {
		t.Fatalf("layer não voltou após rollback: %+v, erro=%v", layer, err)
	}
	claim, err := f.act.Store().GetClaim(context.Background(), f.owner, claimID)
	if err != nil || claim.State != commandactivation.StateActive || claim.TerminalReason != nil {
		t.Fatalf("claim não voltou após rollback: %+v, erro=%v", claim, err)
	}
	if grantRevokedAt(t, f.db, grantID).Valid {
		t.Fatal("grant revogado sobreviveu ao rollback")
	}
	if got := loadDecisionReceiptProbe(t, f.db, confirmed.request.DecisionID); got.Status != commanddecision.Accepted {
		t.Fatalf("receipt após divergência = %q", got.Status)
	}
}

func TestActivationHookWorkspaceLocalNaoRevogaGlobal(t *testing.T) {
	f := newActivationHookFixture(t)
	workspace := "ws-hex"
	localScope := Scope{UserID: f.base.projection.scope.UserID, WorkspaceID: &workspace}
	if err := f.config.EnsureScope(context.Background(), localScope); err != nil {
		t.Fatal(err)
	}
	localLayer := storeTestLayer(t, f.db, f.base.projection.scope.UserID, &workspace, "local layer")
	ruleID := storeTestUUID7(t)
	globalLayer := f.layer
	insertActivationClaim(t, f.db, f.owner, globalLayer.ID, storeTestUUID7(t))
	globalGrant := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID}, globalLayer.ID, storeTestUUID7(t))
	localOwner := f.owner
	localOwner.WorkspaceID = &workspace
	localClaim := insertActivationClaim(t, f.db, localOwner, localLayer.ID, ruleID)
	localGrant := insertAutomationGrant(t, f.db, commandautomation.Owner{UserID: f.owner.UserID, WorkspaceID: &workspace}, localLayer.ID, ruleID)
	confirmed := prepareLayerMutation(t, f, localScope, localLayer.ID, LayerDelete)
	hook, err := NewActivationMutationHook(f.act, f.grants, func(context.Context, Scope) (commandactivation.Owner, error) { return localOwner, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := f.config.CommitConfirmedMutation(context.Background(), confirmed, f.base.epoch, hook); err != nil {
		t.Fatalf("commit local: %v", err)
	}
	if !grantRevokedAt(t, f.db, localGrant).Valid {
		t.Fatal("grant local não revogado")
	}
	claim, err := f.act.Store().GetClaim(context.Background(), localOwner, localClaim)
	if err != nil || claim.State != commandactivation.StateInactive || claim.TerminalReason == nil || *claim.TerminalReason != "layer_deleted" {
		t.Fatalf("claim local após delete = %+v, erro=%v", claim, err)
	}
	if grantRevokedAt(t, f.db, globalGrant).Valid {
		t.Fatal("grant global revogado pela mutação local")
	}
	globalGeneration, err := f.act.Store().SnapshotGeneration(context.Background(), f.owner)
	if err != nil || globalGeneration.Generation != 1 {
		t.Fatalf("geração global alterada: %+v %v", globalGeneration, err)
	}
	localGeneration, err := f.act.Store().SnapshotGeneration(context.Background(), localOwner)
	if err != nil || localGeneration.Generation != 2 {
		t.Fatalf("geração local = %+v, erro=%v", localGeneration, err)
	}
}

func TestActivationHookRecusaRestoreEAceitaBindingOnly(t *testing.T) {
	f := newActivationHookFixture(t)
	hook := f.hook(t)
	ctx := context.Background()
	if err := hook(ctx, f.db, MutationDiff{Operation: LayerRestore, Scope: f.base.projection.scope}); !errors.Is(err, ErrActivationRestoreRequiresExpandedContract) {
		t.Fatalf("LayerRestore = %v", err)
	}
	if err := hook(ctx, f.db, MutationDiff{Operation: ConfigRestore, Scope: f.base.projection.scope}); !errors.Is(err, ErrActivationRestoreRequiresExpandedContract) {
		t.Fatalf("ConfigRestore = %v", err)
	}
	if err := hook(ctx, f.db, MutationDiff{Operation: BindingRestore, Scope: f.base.projection.scope, BeforeLayers: []Layer{f.layer}, AfterLayers: []Layer{f.layer}}); err != nil {
		t.Fatalf("BindingRestore binding-only = %v", err)
	}
	if err := hook(ctx, f.db, MutationDiff{Operation: BindingUpdate, Scope: f.base.projection.scope}); err != nil {
		t.Fatalf("binding-only no-op = %v", err)
	}
}

func TestActivationHookRecusaDeltaForaDoEscopo(t *testing.T) {
	f := newActivationHookFixture(t)
	remoteWorkspace := "ws-foreign"
	remote := f.layer
	remote.ID = storeTestUUID7(t)
	remote.WorkspaceID = &remoteWorkspace
	called := false
	hook, err := NewActivationMutationHook(f.act, f.grants, func(context.Context, Scope) (commandactivation.Owner, error) {
		called = true
		return f.owner, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = hook(context.Background(), f.db, MutationDiff{Operation: BindingUpdate, Scope: f.base.projection.scope, BeforeLayers: []Layer{remote}, AfterLayers: nil})
	if !errors.Is(err, ErrActivationOwnerScopeMismatch) {
		t.Fatalf("delta fora do escopo = %v", err)
	}
	if called {
		t.Fatal("owner consultado para diff inválido")
	}
}
