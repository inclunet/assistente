package commandconfig

import (
	"assistente/internal/auth"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"bytes"
	"context"
	"testing"
	"time"
)

func TestCompleteMutationServiceCannotBypassProjection(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	ctx := context.Background()
	// A fixture pertence exclusivamente a t.TempDir. Remove somente o binding
	// legado da fixture para exercitar a montagem completa com catálogo real.
	if err := f.projection.db.Where("id = ?", f.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	c := MutationServiceConfig{Store: f.projection.store, Sessions: mutationSessionFixture{auth.LocalSessionPrincipal{UserID: f.epoch.UserID, SessionID: f.epoch.SessionID}}, Epochs: epochs, Receipts: f.receipts, Keys: func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{42}, 32), nil }, KeyVersion: "v1", DecisionTTL: time.Minute, Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil }, Validate: completeTestValidator, Version: func(context.Context) (string, error) { return "v1", nil }, Render: func(MutationDiff) (string, error) { return "diff", nil }, OnMutationTx: completeNoopHook}
	s, err := NewCompleteMutationService(c, func(context.Context, Scope) (CompleteProjection, error) { return options, nil })
	if err != nil {
		t.Fatal(err)
	}
	layerDiff, err := s.Apply(ctx, "token", nil, MutationIntent{Operation: LayerCreate, Layer: &Layer{Name: "nova camada", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	layerID := ""
	for _, layer := range layerDiff.AfterLayers {
		if layer.Name == "nova camada" {
			layerID = layer.ID
		}
	}
	command := "unknown.command"
	row := Binding{LayerRefKind: "user", LayerRef: layerID, CommandID: &command, Arguments: "{}", Condition: completeProjectionCondition, TriggerType: "keyboard.local", TriggerSpec: completeProjectionTrigger, Effect: "execute", Enabled: true, ReviewStatus: "active"}
	row.Presentation = `{"version":1}`
	if _, err := s.Apply(ctx, "token", nil, MutationIntent{Operation: BindingCreate, Binding: &row}); err == nil {
		t.Fatal("validator permissivo contornou catálogo")
	}
	command = completeProjectionCommand
	diff, err := s.Apply(ctx, "token", nil, MutationIntent{Operation: BindingCreate, Binding: &row})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.AfterBindings) != 1 || !validID(diff.AfterBindings[0].ID) {
		t.Fatal("binding validado não persistido")
	}
	if countRows(t, f.projection.db, "command_config_mutations") != 2 {
		t.Fatal("auditoria incorreta")
	}
}
