package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
)

func TestCommandDefaultsApplierRejectsMissingComposition(t *testing.T) {
	for _, applier := range []*commandMutationApplier{nil, {}} {
		if _, err := applier.UpgradeDefaults(context.Background(), "", nil); !errors.Is(err, commandconfig.ErrInvalid) {
			t.Fatalf("upgrade: %v", err)
		}
		if _, err := applier.RebaseDefault(context.Background(), "", nil, commandconfig.DefaultRebaseRequest{}); !errors.Is(err, commandconfig.ErrInvalid) {
			t.Fatalf("rebase: %v", err)
		}
		if _, err := applier.RegrantEventRule(context.Background(), "", nil, "rule"); !errors.Is(err, commandconfig.ErrInvalid) {
			t.Fatalf("regrant: %v", err)
		}
	}
}

func TestCommandDefaultsUpgradePublishesConfirmedVersion(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var defaultID string
	for _, layer := range projection.BuiltinLayers {
		for _, item := range layer.Defaults {
			if !item.Invariant {
				defaultID = item.Candidate.ID
				break
			}
		}
		if defaultID != "" {
			break
		}
	}
	if defaultID == "" {
		t.Fatal("no configurable default")
	}
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) { return a.SetDefaultCommandSuppressed(defaultID, true) })
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	if out := settingsSecurityFinish(t, done); out.err != nil || !out.result.Published {
		t.Fatalf("suppression: %+v", out)
	}
	inputs, err := a.commandDesktopMutationInputs(p, a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	baseProjection := inputs.Projection
	inputs.Projection = func(ctx context.Context, scope commandconfig.Scope) (commandconfig.CompleteProjection, error) {
		out, err := baseProjection(ctx, scope)
		if err != nil {
			return out, err
		}
		for i := range out.BuiltinLayers {
			for j := range out.BuiltinLayers[i].Defaults {
				if out.BuiltinLayers[i].Defaults[j].Candidate.ID == defaultID {
					out.BuiltinLayers[i].Defaults[j].Version = "settings-upgrade-test"
				}
			}
		}
		return out, nil
	}
	applier, err := a.newCommandDesktopMutationApplier(inputs)
	if err != nil {
		t.Fatal(err)
	}
	done = settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		out, err := applier.UpgradeDefaults(a.ctx, "", nil)
		return CommandSettingsMutation{Committed: out.Committed, Published: out.Rebuilt}, err
	})
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	if out := settingsSecurityFinish(t, done); out.err != nil || !out.result.Committed || !out.result.Published {
		t.Fatalf("upgrade: %+v", out)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(a.ctx, commandconfig.Scope{UserID: p.principal.UserID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range snapshot.Bindings {
		if row.ReplacesDefaultID != nil && *row.ReplacesDefaultID == defaultID {
			found = true
			if row.ReplacesDefaultVersion == nil || *row.ReplacesDefaultVersion != "settings-upgrade-test" || row.ReviewStatus != "active" || row.Effect != "suppress" {
				t.Fatalf("lost suppression during upgrade: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("upgrade removed personalization")
	}
}
