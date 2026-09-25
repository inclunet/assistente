package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func workspaceTabSelectionManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	m := NewManager(filepath.Join(t.TempDir(), "home"))
	if err := m.Initialize(filepath.Join(t.TempDir(), "active")); err != nil {
		t.Fatal(err)
	}
	active := m.Active()
	if active == nil || len(active.Tabs.Items) == 0 {
		t.Fatal("fixture sem aba")
	}
	sourceID := active.Tabs.Items[0].ID
	if err := m.AddTab(Tab{ID: "selection-editor", Type: TabTypeEditor, Title: "Editor"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetActiveTab(sourceID); err != nil {
		t.Fatal(err)
	}
	return m, sourceID, "selection-editor"
}

func TestSetActiveWorkspaceTabForWorkspaceSelecionaECarimbaSnapshot(t *testing.T) {
	m, sourceID, targetID := workspaceTabSelectionManager(t)
	workspaceID := m.Active().ID
	beforeEpoch := m.commandEpoch

	got, err := m.SetActiveWorkspaceTabForWorkspace(context.Background(), workspaceID, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != workspaceID || got.Tabs.Active != targetID || got.SnapshotEpoch == "" || got.SnapshotSequence == "" {
		t.Fatalf("retorno inválido: %#v", got)
	}
	if m.Active().Tabs.Active != targetID || m.commandEpoch <= beforeEpoch {
		t.Fatalf("seleção não persistiu semanticamente: active=%q epoch=%d/%d", m.Active().Tabs.Active, m.commandEpoch, beforeEpoch)
	}
	if sourceID == targetID {
		t.Fatal("fixture não criou abas distintas")
	}
}

func TestSetActiveWorkspaceTabForWorkspaceNoopNaoGravaNemAvancaEpoch(t *testing.T) {
	m, _, targetID := workspaceTabSelectionManager(t)
	workspaceID := m.Active().ID
	if err := m.SetActiveTab(targetID); err != nil {
		t.Fatal(err)
	}
	beforeEpoch := m.commandEpoch
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	m.activePath = filepath.Join(blocker, "workspace.yaml")

	got, err := m.SetActiveWorkspaceTabForWorkspace(context.Background(), workspaceID, targetID)
	if err != nil || got == nil {
		t.Fatalf("no-op falhou sem persistência: got=%#v err=%v", got, err)
	}
	if m.commandEpoch != beforeEpoch || m.Active().Tabs.Active != targetID {
		t.Fatalf("no-op alterou estado: epoch=%d/%d active=%q", m.commandEpoch, beforeEpoch, m.Active().Tabs.Active)
	}
}

func TestSetActiveWorkspaceTabForWorkspaceRecusaWorkspaceOuAbaSemEfeito(t *testing.T) {
	m, sourceID, targetID := workspaceTabSelectionManager(t)
	workspaceID := m.Active().ID
	before := m.Active()
	beforeEpoch := m.commandEpoch

	other, err := m.Create("other")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		wsID string
		tab  string
		want error
	}{
		{name: "wrong-active-workspace", wsID: other.ID, tab: targetID, want: ErrWorkspaceSelectionWorkspaceStale},
		{name: "missing-tab", wsID: workspaceID, tab: "missing-tab", want: ErrWorkspaceSelectionTabNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, callErr := m.SetActiveWorkspaceTabForWorkspace(context.Background(), tc.wsID, tc.tab)
			if !errors.Is(callErr, tc.want) || got != nil {
				t.Fatalf("recusa inesperada: got=%#v err=%v want=%v", got, callErr, tc.want)
			}
			current := m.Active()
			if current.ID != before.ID || current.Tabs.Active != before.Tabs.Active || m.commandEpoch != beforeEpoch {
				t.Fatalf("recusa produziu efeito: before=%#v after=%#v epoch=%d/%d", before.Tabs, current.Tabs, beforeEpoch, m.commandEpoch)
			}
		})
	}
	if sourceID == targetID {
		t.Fatal("fixture inválida")
	}
}

func TestSetActiveWorkspaceTabForWorkspaceRollbackCancelamentoEABA(t *testing.T) {
	t.Run("persist failure rolls back memory and epoch", func(t *testing.T) {
		m, sourceID, targetID := workspaceTabSelectionManager(t)
		workspaceID := m.Active().ID
		before := m.Active()
		beforeEpoch := m.commandEpoch
		blocker := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
			t.Fatal(err)
		}
		m.activePath = filepath.Join(blocker, "workspace.yaml")
		if got, err := m.SetActiveWorkspaceTabForWorkspace(context.Background(), workspaceID, targetID); err == nil || got != nil {
			t.Fatalf("falha de persistência aceita: got=%#v err=%v", got, err)
		}
		if current := m.Active(); current.Tabs.Active != before.Tabs.Active || m.commandEpoch != beforeEpoch {
			t.Fatalf("rollback incompleto: source=%q current=%q epoch=%d/%d", sourceID, current.Tabs.Active, beforeEpoch, m.commandEpoch)
		}
	})

	t.Run("cancelled request has no effect", func(t *testing.T) {
		m, _, targetID := workspaceTabSelectionManager(t)
		workspaceID := m.Active().ID
		before := m.Active()
		beforeEpoch := m.commandEpoch
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := m.SetActiveWorkspaceTabForWorkspace(ctx, workspaceID, targetID); !errors.Is(err, context.Canceled) || got != nil {
			t.Fatalf("cancelamento: got=%#v err=%v", got, err)
		}
		if current := m.Active(); current.Tabs.Active != before.Tabs.Active || m.commandEpoch != beforeEpoch {
			t.Fatalf("cancelamento produziu efeito: current=%q epoch=%d/%d", current.Tabs.Active, m.commandEpoch, beforeEpoch)
		}
	})

	t.Run("workspace ABA is guarded by current workspace ID", func(t *testing.T) {
		m, sourceID, targetID := workspaceTabSelectionManager(t)
		a := m.Active()
		b, err := m.Create("B")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Switch(b.ID); err != nil {
			t.Fatal(err)
		}
		if got, err := m.SetActiveWorkspaceTabForWorkspace(context.Background(), a.ID, targetID); !errors.Is(err, ErrWorkspaceSelectionWorkspaceStale) || got != nil {
			t.Fatalf("request de A afetou B: got=%#v err=%v", got, err)
		}
		if _, err := m.Switch(a.ID); err != nil {
			t.Fatal(err)
		}
		got, err := m.SetActiveWorkspaceTabForWorkspace(context.Background(), a.ID, targetID)
		if err != nil || got == nil || got.Tabs.Active != targetID {
			t.Fatalf("request válida após A/B/A falhou: got=%#v err=%v", got, err)
		}
		if sourceID == targetID {
			t.Fatal("fixture inválida")
		}
	})
}

func TestSetActiveWorkspaceTabForWorkspaceSnapshotSequenceIsDecimal(t *testing.T) {
	m, _, targetID := workspaceTabSelectionManager(t)
	got, err := m.SetActiveWorkspaceTabForWorkspace(context.Background(), m.Active().ID, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.ParseUint(got.SnapshotSequence, 10, 64); err != nil {
		t.Fatalf("snapshot_sequence inválido: %q", got.SnapshotSequence)
	}
}
