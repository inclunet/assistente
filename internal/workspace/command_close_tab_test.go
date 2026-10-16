package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const replacementChatTabID = "01970a9e-aaaa-7000-8000-abcdef123456"

func commandWorkspaceWithTabs(t *testing.T) *Manager {
	t.Helper()
	manager := commandSnapshotManager(t)
	if err := manager.AddTab(Tab{ID: "editor-middle", Type: TabTypeEditor, Title: "Editor"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.AddTab(Tab{ID: "tasklist-last", Type: TabTypeTasklist, Title: "Tasklist"}); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestCloseActiveTabForCommandPreservaPosicaoDaSucessora(t *testing.T) {
	for _, position := range []int{0, 1, 2} {
		t.Run([]string{"primeira", "meio", "ultima"}[position], func(t *testing.T) {
			manager := commandWorkspaceWithTabs(t)
			before := manager.Active()
			source := before.Tabs.Items[position].ID
			successorPosition := position + 1
			if successorPosition == len(before.Tabs.Items) {
				successorPosition = position - 1
			}
			successor := before.Tabs.Items[successorPosition].ID
			if err := manager.SetActiveTab(source); err != nil {
				t.Fatal(err)
			}
			expected, err := manager.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			created, err := manager.CloseActiveTabForCommand(context.Background(), expected, "")
			if err != nil {
				t.Fatal(err)
			}
			if created.Tabs.Active != successor || created.FindTab(source) != nil || len(created.Tabs.Items) != 2 {
				t.Fatalf("sucessora/remoção incorreta: %#v", created.Tabs)
			}
			for index, tab := range created.Tabs.Items {
				if tab.Position != index {
					t.Fatalf("posição incorreta: %#v", tab)
				}
			}
			created.Tabs.Items[0].Title = "não vazar para Manager"
			if manager.Active().Tabs.Items[0].Title == created.Tabs.Items[0].Title {
				t.Fatal("retorno não é detached")
			}
		})
	}
}

func TestCloseActiveTabForCommandUltimaSubstituiPorChatVazio(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	created, err := manager.CloseActiveTabForCommand(context.Background(), expected, replacementChatTabID)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Tabs.Items) != 1 {
		t.Fatalf("workspace não manteve uma aba: %#v", created.Tabs)
	}
	replacement := created.Tabs.Items[0]
	if replacement.Type != TabTypeChat || replacement.ID != replacementChatTabID || replacement.ConversationID != "" || replacement.Position != 0 || replacement.ID != created.Tabs.Active {
		t.Fatalf("replacement incorreto: %#v", replacement)
	}
	if replacement.Title != "" || replacement.State != nil {
		t.Fatalf("replacement não está vazio: %#v", replacement)
	}
}

func TestCloseActiveTabForCommandRecusaStaleABAESemEfeito(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AddTab(Tab{ID: "aba", Type: TabTypeEditor}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveTab("aba"); err != nil {
		t.Fatal(err)
	}
	before := manager.Active()

	created, err := manager.CloseActiveTabForCommand(context.Background(), expected, replacementChatTabID)
	if !errors.Is(err, ErrCommandCreateTabStale) || created != nil {
		t.Fatalf("esperava stale sem retorno, created=%#v err=%v", created, err)
	}
	if manager.Active().Tabs.Active != before.Tabs.Active || len(manager.Active().Tabs.Items) != len(before.Tabs.Items) {
		t.Fatalf("stale produziu efeito: antes=%#v depois=%#v", before.Tabs, manager.Active().Tabs)
	}
}

func TestCloseActiveTabForCommandRecusaActiveEsperadaDivergenteMesmoVersion(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	expected.ActiveTabID = "forged-target"

	created, err := manager.CloseActiveTabForCommand(context.Background(), expected, replacementChatTabID)
	if !errors.Is(err, ErrCommandCreateTabStale) || created != nil {
		t.Fatalf("esperava active target divergente como stale, created=%#v err=%v", created, err)
	}
}

func TestCloseActiveTabForCommandRecusaReplacementComMesmoIDDaAbaFechada(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := manager.Active()

	created, err := manager.CloseActiveTabForCommand(context.Background(), expected, expected.ActiveTabID)
	if !errors.Is(err, ErrCommandCloseTabInvalidReplacementID) || created != nil {
		t.Fatalf("esperava replacement inválido, created=%#v err=%v", created, err)
	}
	if manager.Active().Tabs.Active != before.Tabs.Active || len(manager.Active().Tabs.Items) != len(before.Tabs.Items) {
		t.Fatalf("replacement inválido produziu efeito: antes=%#v depois=%#v", before.Tabs, manager.Active().Tabs)
	}
}

func TestCloseActiveTabForCommandRecusaCancelamento(t *testing.T) {
	manager := commandWorkspaceWithTabs(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := manager.Active()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	if created, err := manager.CloseActiveTabForCommand(canceled, expected, replacementChatTabID); !errors.Is(err, context.Canceled) || created != nil {
		t.Fatalf("esperava cancelamento sem retorno, created=%#v err=%v", created, err)
	}
	if manager.Active().Tabs.Active != before.Tabs.Active || len(manager.Active().Tabs.Items) != len(before.Tabs.Items) {
		t.Fatalf("cancelamento produziu efeito: antes=%#v depois=%#v", before.Tabs, manager.Active().Tabs)
	}
}

func TestCloseActiveTabForCommandPersistFailureFazRollback(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "home"))
	workspacePath := filepath.Join(root, "workspace")
	if err := manager.Initialize(workspacePath); err != nil {
		t.Fatal(err)
	}
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	beforeEpoch := manager.commandEpoch
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	manager.activePath = filepath.Join(blocker, "workspace.yaml")

	if created, err := manager.CloseActiveTabForCommand(context.Background(), expected, replacementChatTabID); err == nil || created != nil {
		t.Fatalf("esperava falha de persistência sem retorno, created=%#v err=%v", created, err)
	}
	after, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after != before || manager.commandEpoch != beforeEpoch || manager.Active().Tabs.Active != expected.ActiveTabID || len(manager.Active().Tabs.Items) != 1 {
		t.Fatalf("rollback incompleto: before=%#v after=%#v epoch=%d/%d active=%#v", before, after, beforeEpoch, manager.commandEpoch, manager.Active().Tabs)
	}
}

func TestCloseActiveTabForCommandReplayEConcorrencia(t *testing.T) {
	manager := commandWorkspaceWithTabs(t)
	if err := manager.SetActiveTab("editor-middle"); err != nil {
		t.Fatal(err)
	}
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CloseActiveTabForCommand(context.Background(), expected, ""); err != nil {
		t.Fatal(err)
	}
	if created, err := manager.CloseActiveTabForCommand(context.Background(), expected, ""); !errors.Is(err, ErrCommandCreateTabStale) || created != nil {
		t.Fatalf("replay não foi stale: created=%#v err=%v", created, err)
	}

	manager = commandWorkspaceWithTabs(t)
	if err := manager.SetActiveTab("editor-middle"); err != nil {
		t.Fatal(err)
	}
	expected, err = manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, callErr := manager.CloseActiveTabForCommand(context.Background(), expected, "")
			results <- callErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, stale int
	for callErr := range results {
		switch {
		case callErr == nil:
			successes++
		case errors.Is(callErr, ErrCommandCreateTabStale):
			stale++
		default:
			t.Fatalf("erro concorrente inesperado: %v", callErr)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("CAS concorrente incorreto: successes=%d stale=%d", successes, stale)
	}
}
