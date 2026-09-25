package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/database"
	"assistente/internal/tools/filesystem"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func contextualPaletteFileFixture(t *testing.T, commandID string) (editorFileCommandFlow, LocalCommandKeyboardMap) {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	tabID := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: workspace.TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Arquivos contextuais", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{
		LayerID: layer.ID, CommandID: commandID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"` + commandID + `"}`, Effect: "execute", Enabled: true,
		Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "editor"}, {Field: "surface.id", Value: tabID}, {Field: "profile", Value: "dev"}}},
	}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.BeginContextualPaletteUICommand(view.Generation, commandID, LocalCommandKeyboardContext{SurfaceType: "editor", SurfaceID: tabID, Profile: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	h := takeUICommandFor(t, a, r.Ticket, commandID)
	return editorFileCommandFlow{a: a, ctx: database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID), reservation: r, handoff: h}, view
}

func TestContextualPaletteFileNativeContinuationAfterMapReset(t *testing.T) {
	for _, commandID := range []string{commandEditorFileSaveID, commandEditorFileSaveCopyID, commandEditorFileOpenID} {
		t.Run(commandID, func(t *testing.T) {
			f, view := contextualPaletteFileFixture(t, commandID)
			operation, _, err := f.a.editorCommandTarget(f.ctx, f.reservation.Ticket, f.handoff.HandoffID)
			if err != nil {
				t.Fatal(err)
			}
			// Only an already admitted native preparation survives the map's blur reset.
			f.a.ResetLocalCommandKeyboard(view.Generation)
			path := filepath.Join(t.TempDir(), "contextual.md")
			content := "conteúdo contextual"
			var opened *apidto.EditorOpenResult
			if operation == "open" {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				opened = &apidto.EditorOpenResult{Path: path, Content: content}
				content = ""
			}
			prepareEditorFileCommandAt(t, &f, operation, content, path, opened)
			writes := 0
			write := func(_ context.Context, path, content string, _ filesystem.FileVersion) error {
				writes++
				return os.WriteFile(path, []byte(content), 0600)
			}
			result, err := commitEditorFileCommand(t, &f, true, write)
			if err != nil || result == nil || result.Status != "succeeded" {
				t.Fatalf("native continuation: %+v %v", result, err)
			}
			expectedWrites := 1
			if operation == "open" {
				expectedWrites = 0
			}
			if writes != expectedWrites {
				t.Fatalf("writes=%d", writes)
			}
			if _, err := commitEditorFileCommand(t, &f, true, write); err == nil || writes != expectedWrites {
				t.Fatalf("duplicate write: %d %v", writes, err)
			}
			if result := getUIResultEventually(t, f.a, f.reservation.Ticket); result.Status != "succeeded" {
				t.Fatalf("ledger: %+v", result)
			}
			assertEditorFileLedger(t, f.reservation.InvocationID, "conteúdo contextual", path)
		})
	}
}

func TestContextualPaletteFileNativeContinuationRejectsStale(t *testing.T) {
	for _, transition := range []string{"before-prepare", "profile-aba", "vault", "deadline", "cancel"} {
		t.Run(transition, func(t *testing.T) {
			f, view := contextualPaletteFileFixture(t, commandEditorFileSaveID)
			if transition == "before-prepare" {
				f.a.ResetLocalCommandKeyboard(view.Generation)
				if _, _, err := f.a.editorCommandTarget(f.ctx, f.reservation.Ticket, f.handoff.HandoffID); err == nil {
					t.Fatal("stale source created native continuation")
				}
				return
			}
			prepareEditorFileCommand(t, &f, "must not write", filepath.Join(t.TempDir(), "stale.md"), nil)
			f.a.ResetLocalCommandKeyboard(view.Generation)
			switch transition {
			case "profile-aba":
				if err := f.a.workspaceMgr.SetProfile("other"); err != nil {
					t.Fatal(err)
				}
				if err := f.a.workspaceMgr.SetProfile("dev"); err != nil {
					t.Fatal(err)
				}
			case "vault":
				if err := f.a.commandHost.SetVaultUnlocked(f.ctx, false); err != nil {
					t.Fatal(err)
				}
			case "deadline":
				p, run, err := f.a.commandUIRun(f.reservation.Ticket)
				if err != nil {
					t.Fatal(err)
				}
				p.mu.Lock()
				run.editorFile.paletteValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.mu.Unlock()
			case "cancel":
				if err := f.a.CancelUICommand(f.reservation.Ticket); err != nil {
					t.Fatal(err)
				}
			}
			writes := 0
			_, err := commitEditorFileCommand(t, &f, true, func(context.Context, string, string, filesystem.FileVersion) error { writes++; return nil })
			if err == nil || writes != 0 {
				t.Fatalf("%s wrote after invalidation: %d %v", transition, writes, err)
			}
		})
	}
}
