package app

import (
	"context"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Configuration is setup here, not the operation under test. Each caller owns
// its database and App; the real loader/projector still validates publication.
// Confirmed edits remain exercised by the settings contract/security tests.
type contextualCommandConfigFixture struct {
	layers   []commandconfig.Layer
	rules    []commandactivation.Rule
	bindings []commandconfig.Binding
}

func (f *contextualCommandConfigFixture) layer(t *testing.T, name, mode string) (layerID, ruleID string) {
	t.Helper()
	layerID, ruleID = uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	rule, err := commandSettingsRuleInput(&CommandSettingsRuleInput{LayerID: layerID, Mode: mode, Lifecycle: "persistent", Enabled: true}, ruleID)
	if err != nil {
		t.Fatal(err)
	}
	f.layers = append(f.layers, commandconfig.Layer{ID: layerID, Name: name, Enabled: true, Source: "user"})
	f.rules = append(f.rules, rule)
	return layerID, ruleID
}

func (f *contextualCommandConfigFixture) binding(t *testing.T, input CommandSettingsBindingInput) string {
	t.Helper()
	binding, err := commandSettingsBindingInput(&input, uuid.Must(uuid.NewV7()).String())
	if err != nil {
		t.Fatal(err)
	}
	f.bindings = append(f.bindings, binding)
	return binding.ID
}

func (f *contextualCommandConfigFixture) publish(t *testing.T, a *App) {
	t.Helper()
	uid := a.commandProduct.Load().principal.UserID
	for i := range f.layers {
		f.layers[i].UserID = uid
	}
	for i := range f.rules {
		f.rules[i].UserID = uid
	}
	for i := range f.bindings {
		f.bindings[i].UserID = uid
	}
	if err := database.DB().Transaction(func(tx *gorm.DB) error {
		for _, rows := range []any{&f.layers, &f.rules, &f.bindings} {
			if err := tx.Create(rows).Error; err != nil {
				return err
			}
		}
		return tx.Model(&commandconfig.Generation{}).Where("user_id = ? AND workspace_id IS NULL", uid).
			Update("generation", gorm.Expr("generation + 1")).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
}
