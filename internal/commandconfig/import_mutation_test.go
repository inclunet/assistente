package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func importMutationUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func importMutationEventRule(t *testing.T, userID, layerID string) (commandactivation.Rule, commandautomation.Grant) {
	t.Helper()
	ruleID := importMutationUUID(t)
	decisionID := importMutationUUID(t)
	grantID := importMutationUUID(t)
	eventName := commandautomation.JobRunStateEvent
	producers := "[\"jobs.runtime\"]"
	generation := int64(1)
	fingerprint := "grant-fingerprint"
	rule := commandactivation.Rule{
		ID: ruleID, UserID: userID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeEvent,
		Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent, EventName: &eventName,
		AllowedInternalProducerTypes: &producers, AuthorizationDecisionID: &decisionID,
		AutomationGrantID: &grantID, AutomationGrantGeneration: &generation,
		AutomationGrantFingerprint: &fingerprint, Enabled: true, Source: "user", ReviewStatus: "active",
	}
	now := time.Now().UTC()
	grant := commandautomation.Grant{
		ID: grantID, Owner: commandautomation.Owner{UserID: userID},
		LayerRef:        commandautomation.RuleRef{Kind: "user", Ref: layerID},
		RuleRef:         commandautomation.RuleRef{Kind: "user", Ref: ruleID},
		RuleFingerprint: "rule-fingerprint", EventName: commandautomation.JobRunStateEvent,
		ProducerTypesFingerprint: "producer-fingerprint", AutomationGrantGeneration: generation,
		AutomationGrantFingerprint: fingerprint, AuthorizationDecisionID: decisionID,
		GrantedAt: now, GrantedBy: userID,
	}
	return rule, grant
}

func TestPrepareImportedSnapshotPreservaGrantDeRegraNaoTocada(t *testing.T) {
	userID := importMutationUUID(t)
	layerA := importMutationUUID(t)
	layerB := importMutationUUID(t)
	scope := Scope{UserID: userID}
	beforeLayerB := Layer{ID: layerB, UserID: userID, Name: "B", Enabled: true, Source: "user"}
	ruleB, grantB := importMutationEventRule(t, userID, layerB)
	before := Snapshot{
		Scope: scope,
		Layers: []Layer{
			{ID: layerA, UserID: userID, Name: "A", Enabled: true, Source: "user"},
			beforeLayerB,
		},
		ActivationRules:  []commandactivation.Rule{ruleB},
		AutomationGrants: []commandautomation.Grant{grantB},
	}
	imported := ImportedSnapshot{
		Snapshot: Snapshot{
			Scope: scope,
			Layers: []Layer{
				{ID: layerA, UserID: userID, Name: "A importada", Enabled: true, Source: "user"},
				beforeLayerB,
			},
			ActivationRules: []commandactivation.Rule{ruleB},
		},
		TouchedLayerIDs: []string{layerA},
	}

	prepared, err := (&Store{}).prepareImportedSnapshot(context.Background(), scope, before, imported, func(_ context.Context, snapshot Snapshot) error {
		return validateSnapshot(snapshot)
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	diff := prepared.Diff()
	var gotRule commandactivation.Rule
	for _, rule := range diff.AfterActivationRules {
		if rule.ID == ruleB.ID {
			gotRule = rule
		}
	}
	if gotRule.ID == "" || !gotRule.Enabled || !reflect.DeepEqual(gotRule, ruleB) {
		t.Fatalf("regra B preservada incorretamente: got=%+v want=%+v", gotRule, ruleB)
	}
	if len(diff.AfterAutomationGrants) != 1 || !reflect.DeepEqual(diff.AfterAutomationGrants[0], grantB) {
		t.Fatalf("grant B preservado incorretamente: got=%+v want=%+v", diff.AfterAutomationGrants, grantB)
	}
}

func TestPrepareImportedSnapshotRejeitaGrantNaRegraTocada(t *testing.T) {
	userID := importMutationUUID(t)
	layerID := importMutationUUID(t)
	rule, grant := importMutationEventRule(t, userID, layerID)
	scope := Scope{UserID: userID}
	before := Snapshot{
		Scope:            scope,
		Layers:           []Layer{{ID: layerID, UserID: userID, Name: "A", Enabled: true, Source: "user"}},
		ActivationRules:  []commandactivation.Rule{rule},
		AutomationGrants: []commandautomation.Grant{grant},
	}
	prepared, err := (&Store{}).prepareImportedSnapshot(context.Background(), scope, before, ImportedSnapshot{
		Snapshot:        before,
		TouchedLayerIDs: []string{layerID},
		TouchedRuleIDs:  []string{rule.ID},
	}, func(_ context.Context, snapshot Snapshot) error { return validateSnapshot(snapshot) })
	if prepared != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("grant no DTO de regra tocada: prepared=%v err=%v", prepared, err)
	}
}

func TestPrepareImportedSnapshotRejeitaOwnerDiferente(t *testing.T) {
	userID := importMutationUUID(t)
	otherUserID := importMutationUUID(t)
	scope := Scope{UserID: userID}
	before := Snapshot{Scope: scope}
	prepared, err := (&Store{}).prepareImportedSnapshot(context.Background(), scope, before, ImportedSnapshot{
		Snapshot: Snapshot{Scope: Scope{UserID: otherUserID}},
	}, func(_ context.Context, snapshot Snapshot) error { return validateSnapshot(snapshot) })
	if prepared != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("owner estrangeiro aceito: prepared=%v err=%v", prepared, err)
	}
}

func TestPrepareImportedSnapshotProvaRegraNaoTocada(t *testing.T) {
	userID := importMutationUUID(t)
	layerID := importMutationUUID(t)
	rule, grant := importMutationEventRule(t, userID, layerID)
	scope := Scope{UserID: userID}
	before := Snapshot{
		Scope:            scope,
		Layers:           []Layer{{ID: layerID, UserID: userID, Name: "A", Enabled: true, Source: "user"}},
		ActivationRules:  []commandactivation.Rule{rule},
		AutomationGrants: []commandautomation.Grant{grant},
	}
	changed := rule
	changed.Condition = "{\"changed\":true}"
	prepared, err := (&Store{}).prepareImportedSnapshot(context.Background(), scope, before, ImportedSnapshot{
		Snapshot: Snapshot{
			Scope:           scope,
			Layers:          before.Layers,
			ActivationRules: []commandactivation.Rule{changed},
		},
	}, func(_ context.Context, snapshot Snapshot) error { return validateSnapshot(snapshot) })
	if prepared != nil || !errors.Is(err, ErrStale) {
		t.Fatalf("alteração silenciosa da regra não tocada: prepared=%v err=%v", prepared, err)
	}
}

func TestCompleteMutationServiceImportUsaWriterConfirmadoERevalidaERollback(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	if err := f.projection.db.Where("id = ?", f.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.ActiveUserLayerIDs = []string{f.binding.LayerRef}
	authorizedOperation := Operation("")
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store:    f.projection.store,
		Sessions: mutationSessionFixture{auth.LocalSessionPrincipal{UserID: f.epoch.UserID, SessionID: f.epoch.SessionID}},
		Epochs:   epochs, Receipts: f.receipts,
		Keys:       func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{42}, 32), nil },
		KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize: func(_ context.Context, _ auth.LocalSessionPrincipal, _ Scope, operation Operation) error {
			authorizedOperation = operation
			return nil
		},
		Version:      func(context.Context) (string, error) { return "catalog-v1", nil },
		Render:       func(MutationDiff) (string, error) { return "import diff", nil },
		OnMutationTx: func(context.Context, *gorm.DB, MutationDiff) error { return errors.New("import rollback fixture") },
	}, func(context.Context, Scope) (CompleteProjection, error) { return options, nil })
	if err != nil {
		t.Fatal(err)
	}
	revalidated := 0
	_, err = service.Import(context.Background(), "token", nil, func(_ context.Context, _ Scope, current Snapshot) (ImportedSnapshot, error) {
		command := completeProjectionCommand
		binding := Binding{
			ID: importMutationUUID(t), UserID: current.Scope.UserID, LayerRefKind: "user",
			LayerRef: current.Layers[0].ID, TriggerType: "keyboard.local",
			TriggerSpec: completeProjectionTrigger, CommandID: &command, Arguments: "{}",
			Condition: completeProjectionCondition, Effect: "execute", Enabled: true,
			Source: "user", ReviewStatus: "active", Presentation: "{\"version\":1}",
		}
		current.Bindings = append(current.Bindings, binding)
		return ImportedSnapshot{
			Snapshot: current, TouchedBindingIDs: []string{binding.ID},
		}, nil
	}, func(context.Context, Scope) error {
		revalidated++
		return nil
	})
	if err == nil {
		t.Fatal("Import não propagou rollback do hook")
	}
	if authorizedOperation != ConfigImport || revalidated != 1 {
		t.Fatalf("percurso de import incompleto: operation=%q revalidated=%d", authorizedOperation, revalidated)
	}
	if got := countRows(t, f.projection.db, "command_bindings"); got != 0 {
		t.Fatalf("rollback deixou %d binding(s)", got)
	}
	if got := writeTestGenerationRow(t, f.projection.db, f.projection.snapshot.Generations[0].ID); got.Generation != 1 {
		t.Fatalf("geração após rollback=%d, esperado 1", got.Generation)
	}
	if got := countRows(t, f.projection.db, "command_config_mutations"); got != 0 {
		t.Fatalf("rollback deixou auditoria: %d", got)
	}
}
