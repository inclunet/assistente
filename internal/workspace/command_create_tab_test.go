package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func commandChatTab(id string) Tab {
	return Tab{
		ID:             id,
		Type:           TabTypeChat,
		ConversationID: "01970a9e-9999-7000-8000-abcdef123456",
		Title:          "Comando",
		State:          map[string]any{"nested": map[string]any{"value": "original"}},
	}
}

func commandTabForType(id string, tabType TabType) Tab {
	if tabType == TabTypeChat {
		return commandChatTab(id)
	}
	return Tab{ID: id, Type: tabType, Title: string(tabType)}
}

var commandCreateTabTypes = []struct {
	name    string
	tabType TabType
}{
	{name: "chat", tabType: TabTypeChat},
	{name: "editor", tabType: TabTypeEditor},
	{name: "tasklist", tabType: TabTypeTasklist},
}

func TestAddTabForCommandCriaUmaAbaNoAlvoEDevolveSnapshotDetached(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	before := manager.Active()

	created, err := manager.AddTabForCommand(context.Background(), expected, commandChatTab("command-chat"))
	if err != nil {
		t.Fatalf("AddTabForCommand: %v", err)
	}
	if created == nil || created.ID != expected.WorkspaceID {
		t.Fatalf("workspace retornado incorreto: %#v", created)
	}
	if len(created.Tabs.Items) != len(before.Tabs.Items)+1 || created.Tabs.Active != "command-chat" {
		t.Fatalf("criação não foi única/ativa: antes=%d depois=%d active=%q", len(before.Tabs.Items), len(created.Tabs.Items), created.Tabs.Active)
	}
	if created.FindTab("command-chat") == nil || created.FindTab("command-chat").Type != TabTypeChat {
		t.Fatalf("aba criada incorreta: %#v", created.FindTab("command-chat"))
	}

	created.FindTab("command-chat").State["nested"].(map[string]any)["value"] = "caller-mutated"
	stored := manager.Active().FindTab("command-chat")
	if stored.State["nested"].(map[string]any)["value"] != "original" {
		t.Fatalf("retorno não foi detached: %#v", stored.State)
	}
}

func TestAddTabForCommandRecusaWorkspaceAlterado(t *testing.T) {
	for _, tc := range commandCreateTabTypes {
		t.Run(tc.name, func(t *testing.T) {
			manager := commandSnapshotManager(t)
			expected, err := manager.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			other, err := manager.Create("Outro")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Switch(other.ID); err != nil {
				t.Fatal(err)
			}

			created, err := manager.AddTabForCommand(context.Background(), expected, commandTabForType("wrong-"+tc.name, tc.tabType))
			if !errors.Is(err, ErrCommandCreateTabStale) {
				t.Fatalf("esperava workspace stale, got %v", err)
			}
			if created != nil || manager.Active().FindTab("wrong-"+tc.name) != nil {
				t.Fatalf("snapshot stale produziu efeito: created=%#v active=%#v", created, manager.Active())
			}
		})
	}
}

func TestAddTabForCommandRecusaABADoActiveTabMesmoComWorkspaceIgual(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AddTab(commandChatTab("temporary-chat")); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveTab("temporary-chat"); err != nil {
		t.Fatal(err)
	}
	current, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if current.WorkspaceID != expected.WorkspaceID || current.ActiveTabID != expected.ActiveTabID || current.Version == expected.Version {
		t.Fatalf("fixture não produziu ABA de aba/versão: expected=%#v current=%#v", expected, current)
	}

	if _, err := manager.AddTabForCommand(context.Background(), expected, commandChatTab("stale-chat")); !errors.Is(err, ErrCommandCreateTabStale) {
		t.Fatalf("esperava snapshot stale após ABA, got %v", err)
	}
}

func TestAddTabForCommandRecusaContextoCanceladoETabInvalida(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.AddTabForCommand(canceled, expected, commandChatTab("canceled")); !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava contexto cancelado, got %v", err)
	}
	if _, err := manager.AddTabForCommand(context.Background(), expected, Tab{ID: "terminal", Type: TabTypeTerminal}); !errors.Is(err, ErrCommandCreateTabUnsupportedType) {
		t.Fatalf("esperava tipo não suportado, got %v", err)
	}
	if _, err := manager.AddTabForCommand(context.Background(), expected, Tab{ID: "unknown", Type: TabType("unknown")}); !errors.Is(err, ErrCommandCreateTabUnsupportedType) {
		t.Fatalf("esperava tipo desconhecido não suportado, got %v", err)
	}
	if _, err := manager.AddTabForCommand(context.Background(), expected, Tab{Type: TabTypeChat}); err == nil {
		t.Fatal("esperava aba inválida")
	}
}

func TestAddTabForCommandCriaChatEditorETasklistSemRecursosExtras(t *testing.T) {
	manager := commandSnapshotManager(t)
	cases := []struct {
		name string
		tab  Tab
	}{
		{name: "chat", tab: commandChatTab("command-chat")},
		{name: "editor", tab: Tab{ID: "command-editor", Type: TabTypeEditor, Title: "Editor"}},
		{name: "tasklist", tab: Tab{ID: "command-tasklist", Type: TabTypeTasklist, Title: "Tasklist"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected, err := manager.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			created, err := manager.AddTabForCommand(context.Background(), expected, tc.tab)
			if err != nil {
				t.Fatalf("AddTabForCommand: %v", err)
			}
			got := created.FindTab(tc.tab.ID)
			if got == nil || got.Type != tc.tab.Type {
				t.Fatalf("aba criada incorreta: %#v", got)
			}
			if tc.tab.Type != TabTypeChat && (got.ConversationID != "" || got.State != nil) {
				t.Fatalf("aba não-chat criou recursos/estado: %#v", got)
			}
		})
	}
}

func TestAddTabForCommandRecusaEstadoNaoVazioEmEditorETasklist(t *testing.T) {
	for _, tabType := range []TabType{TabTypeEditor, TabTypeTasklist} {
		t.Run(string(tabType), func(t *testing.T) {
			for _, tc := range []struct {
				name string
				edit func(*Tab)
			}{
				{name: "conversation-only", edit: func(tab *Tab) { tab.ConversationID = "conversation" }},
				{name: "state-only", edit: func(tab *Tab) { tab.State = map[string]any{"resource": "already-created"} }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					manager := commandSnapshotManager(t)
					expected, err := manager.CommandSnapshot()
					if err != nil {
						t.Fatal(err)
					}
					tab := Tab{ID: "nonempty-" + string(tabType) + "-" + tc.name, Type: tabType}
					tc.edit(&tab)
					if _, err := manager.AddTabForCommand(context.Background(), expected, tab); err == nil {
						t.Fatal("esperava estado não vazio ser recusado")
					}
					if manager.Active().FindTab(tab.ID) != nil {
						t.Fatal("recusa deixou aba em memória")
					}
				})
			}
		})
	}
}

func TestAddTabForCommandRecusaSnapshotEsperadoVazioEIDDuplicado(t *testing.T) {
	for _, tc := range commandCreateTabTypes {
		t.Run(tc.name, func(t *testing.T) {
			manager := commandSnapshotManager(t)
			if _, err := manager.AddTabForCommand(context.Background(), CommandSnapshot{}, commandTabForType("empty-snapshot-"+tc.name, tc.tabType)); !errors.Is(err, ErrCommandCreateTabStale) {
				t.Fatalf("esperava snapshot vazio stale, got %v", err)
			}

			expected, err := manager.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.AddTabForCommand(context.Background(), expected, commandTabForType(expected.ActiveTabID, tc.tabType)); !errors.Is(err, ErrCommandCreateTabDuplicateID) {
				t.Fatalf("esperava ID duplicado, got %v", err)
			}
		})
	}
}

func TestAddTabForCommandMesmoSnapshotConcorrentePermiteUmaMutacao(t *testing.T) {
	for _, tc := range commandCreateTabTypes {
		t.Run(tc.name, func(t *testing.T) {
			manager := commandSnapshotManager(t)
			expected, err := manager.CommandSnapshot()
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
					_, callErr := manager.AddTabForCommand(context.Background(), expected, commandTabForType("concurrent-"+tc.name, tc.tabType))
					results <- callErr
				}()
			}
			close(start)
			wg.Wait()
			close(results)

			successes := 0
			stale := 0
			for callErr := range results {
				if callErr == nil {
					successes++
				} else if errors.Is(callErr, ErrCommandCreateTabStale) {
					stale++
				} else {
					t.Fatalf("erro concorrente inesperado: %v", callErr)
				}
			}
			if successes != 1 || stale != 1 {
				t.Fatalf("snapshot foi consumido incorretamente: successes=%d stale=%d", successes, stale)
			}
			active := manager.Active()
			count := 0
			for _, tab := range active.Tabs.Items {
				if tab.ID == "concurrent-"+tc.name {
					count++
				}
			}
			if count != 1 || active.Tabs.Active != "concurrent-"+tc.name {
				t.Fatalf("mutação concorrente não foi exatamente uma: count=%d active=%q", count, active.Tabs.Active)
			}
		})
	}
}

func TestAddTabForCommandPersistFailureNaoMutaMemoria(t *testing.T) {
	for _, tc := range commandCreateTabTypes {
		t.Run(tc.name, func(t *testing.T) {
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
			blocker := filepath.Join(root, "not-a-directory")
			if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
				t.Fatal(err)
			}
			manager.activePath = filepath.Join(blocker, "workspace.yaml")
			tab := commandTabForType("persist-fails-"+tc.name, tc.tabType)

			if _, err := manager.AddTabForCommand(context.Background(), expected, tab); err == nil {
				t.Fatal("esperava falha de persistência")
			}
			after, err := manager.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("falha de persistência alterou snapshot em memória: antes=%#v depois=%#v", before, after)
			}
			if manager.Active().FindTab(tab.ID) != nil {
				t.Fatal("falha de persistência deixou aba em memória")
			}
		})
	}
}

func TestAddTabForCommandNaoDeixaTemporarioAposPersistencia(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "workspace")
	manager := NewManager(filepath.Join(root, "home"))
	if err := manager.Initialize(workspacePath); err != nil {
		t.Fatal(err)
	}
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddTabForCommand(context.Background(), expected, commandChatTab("atomic-chat")); err != nil {
		t.Fatal(err)
	}
	workspaceFile := filepath.Join(workspacePath, assistenteDir, workspaceFile)
	data, err := os.ReadFile(workspaceFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "atomic-chat") {
		t.Fatalf("arquivo persistido não contém a aba: %s", data)
	}
	temporary, err := filepath.Glob(filepath.Join(workspacePath, assistenteDir, ".workspace.yaml-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporários de persistência ficaram no diretório: %v", temporary)
	}
}

func TestSaveWorkspaceForCommandCanceladoAntesDoReplacePreservaArquivo(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "home"))
	workspacePath := filepath.Join(root, "workspace")
	if err := manager.Initialize(workspacePath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspacePath, assistenteDir, workspaceFile)
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.saveWorkspaceForCommand(canceled, manager.Active(), workspacePath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava cancelamento antes do replace, got %v", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("cancelamento alterou arquivo existente")
	}
	temporary, err := filepath.Glob(filepath.Join(workspacePath, assistenteDir, ".workspace.yaml-command-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporário não foi removido após cancelamento: %v", temporary)
	}
}

func TestAddTabMantemDeduplicacaoExistente(t *testing.T) {
	manager := commandSnapshotManager(t)
	initial := manager.Active()
	tab := commandChatTab("duplicate-chat")
	if err := manager.AddTab(tab); err != nil {
		t.Fatal(err)
	}
	count := len(manager.Active().Tabs.Items)
	if err := manager.AddTab(Tab{ID: "different-id", Type: TabTypeChat, ConversationID: tab.ConversationID}); err != nil {
		t.Fatal(err)
	}
	active := manager.Active()
	if len(active.Tabs.Items) != count || active.Tabs.Active != "duplicate-chat" || initial.ID != active.ID {
		t.Fatalf("AddTab perdeu deduplicação: %#v", active.Tabs)
	}
}
