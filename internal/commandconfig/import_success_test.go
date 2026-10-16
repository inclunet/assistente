package commandconfig

import (
	"context"
	"errors"
	"testing"
)

func TestImportCommitsThroughRealHookOrRejectsChangedReferences(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "commit"
		if stale {
			name = "stale_references"
		}
		t.Run(name, func(t *testing.T) {
			f := newActivationHookFixture(t)
			service := completeServiceWithActiveLayerProof(t, f, func() []string { return []string{f.layer.ID} })
			id := importMutationUUID(t)
			checks := 0
			diff, err := service.Import(context.Background(), "token", nil, func(_ context.Context, scope Scope, current Snapshot) (ImportedSnapshot, error) {
				command := completeProjectionCommand
				current.Bindings = append(current.Bindings, Binding{
					ID: id, UserID: scope.UserID, LayerRefKind: "user", LayerRef: f.layer.ID,
					TriggerType: "keyboard.local", TriggerSpec: completeProjectionTrigger, CommandID: &command,
					Arguments: "{}", Condition: completeProjectionCondition, Effect: "execute", Enabled: true,
					Source: "user", ReviewStatus: "active", Presentation: `{"version":1}`,
				})
				return ImportedSnapshot{Snapshot: current, TouchedBindingIDs: []string{id}}, nil
			}, func(context.Context, Scope) error {
				checks++
				if stale {
					return ErrStale
				}
				return nil
			})
			if checks != 1 {
				t.Fatalf("reference checks=%d", checks)
			}
			if stale {
				if !errors.Is(err, ErrStale) {
					t.Fatalf("stale import=%v", err)
				}
				if countRows(t, f.db, "command_bindings") != 0 || countRows(t, f.db, "command_config_mutations") != 0 {
					t.Fatal("stale plan changed database")
				}
				return
			}
			if err != nil {
				t.Fatalf("confirmed import=%v", err)
			}
			if diff.Operation != ConfigImport || f.base.presenter.calls != 1 {
				t.Fatalf("decision/operation=%+v calls=%d", diff, f.base.presenter.calls)
			}
			var persisted Binding
			if err := f.db.Where("id = ? AND user_id = ?", id, f.owner.UserID).Take(&persisted).Error; err != nil {
				t.Fatal(err)
			}
			if persisted.LayerRef != f.layer.ID || persisted.CommandID == nil || *persisted.CommandID != completeProjectionCommand {
				t.Fatalf("persisted=%+v", persisted)
			}
			var audit struct {
				Operation                         string
				BeforeGeneration, AfterGeneration int64
			}
			if err := f.db.Table("command_config_mutations").Where("mutation_id = ?", diff.MutationID).Take(&audit).Error; err != nil {
				t.Fatal(err)
			}
			if audit.Operation != string(ConfigImport) || audit.AfterGeneration != audit.BeforeGeneration+1 {
				t.Fatalf("audit=%+v", audit)
			}
			var consumed int64
			if err := f.db.Table("command_decision_receipts").Where("consumed_at IS NOT NULL").Count(&consumed).Error; err != nil {
				t.Fatal(err)
			}
			if consumed != 1 {
				t.Fatalf("consumed receipts=%d", consumed)
			}
		})
	}
}

func TestWorkspaceImportCannotWriteInheritedGlobalRows(t *testing.T) {
	user, globalID, localID := importMutationUUID(t), importMutationUUID(t), importMutationUUID(t)
	workspace := "workspace-a"
	scope := Scope{UserID: user, WorkspaceID: &workspace}
	before := Snapshot{Scope: scope, Layers: []Layer{
		{ID: globalID, UserID: user, Name: "global", Enabled: true, Source: "user"},
		{ID: localID, UserID: user, WorkspaceID: &workspace, Name: "local", Enabled: true, Source: "user"},
	}}
	for _, remove := range []bool{false, true} {
		after := cloneConfigSnapshot(before)
		if remove {
			after.Layers = after.Layers[1:]
		} else {
			after.Layers[0].Name = "altered"
		}
		if _, err := (&Store{}).prepareImportedSnapshot(context.Background(), scope, before,
			ImportedSnapshot{Snapshot: after, TouchedLayerIDs: []string{globalID}},
			func(context.Context, Snapshot) error { return nil }); !errors.Is(err, ErrInvalid) {
			t.Fatalf("workspace wrote global (delete=%v): %v", remove, err)
		}
	}
	after := cloneConfigSnapshot(before)
	after.Layers[1].Name = "local changed"
	if _, err := (&Store{}).prepareImportedSnapshot(context.Background(), scope, before,
		ImportedSnapshot{Snapshot: after, TouchedLayerIDs: []string{localID}},
		func(context.Context, Snapshot) error { return nil }); err != nil {
		t.Fatalf("untouched inherited global rejected: %v", err)
	}
}
