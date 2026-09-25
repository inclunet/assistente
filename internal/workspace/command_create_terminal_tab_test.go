package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type terminalCommandSessionValidator struct {
	live       map[string]bool
	closeCalls []string
}

func (f *terminalCommandSessionValidator) Has(id string) bool { return f.live[id] }

func terminalCommandTab(id, sessionID string) Tab {
	return Tab{
		ID:    id,
		Type:  TabTypeTerminal,
		Title: "Terminal",
		State: map[string]any{"sessionId": sessionID},
	}
}

func TestAddTerminalTabForCommandPersisteSessaoEnaoFechaAoRemover(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	sessions := &terminalCommandSessionValidator{live: map[string]bool{"session-live": true}}

	created, err := manager.AddTerminalTabForCommand(context.Background(), expected, terminalCommandTab("terminal-command", "session-live"), sessions)
	if err != nil {
		t.Fatal(err)
	}
	got := created.FindTab("terminal-command")
	if got == nil || got.Type != TabTypeTerminal || got.ConversationID != "" || len(got.State) != 1 || got.State["sessionId"] != "session-live" {
		t.Fatalf("aba terminal persistida incorretamente: %#v", got)
	}
	if err := manager.RemoveTab("terminal-command"); err != nil {
		t.Fatal(err)
	}
	if len(sessions.closeCalls) != 0 {
		t.Fatalf("workspace fechou sessão do orquestrador: %v", sessions.closeCalls)
	}
	data, err := os.ReadFile(filepath.Join(manager.activePath, assistenteDir, workspaceFile))
	if err != nil || len(data) == 0 {
		t.Fatalf("workspace terminal não foi persistido: err=%v", err)
	}
}

func TestAddTerminalTabForCommandRecusaSnapshotStaleEABA(t *testing.T) {
	t.Run("workspace stale", func(t *testing.T) {
		manager := commandSnapshotManager(t)
		expected, err := manager.CommandSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		other, err := manager.Create("Outro workspace")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Switch(other.ID); err != nil {
			t.Fatal(err)
		}
		sessions := &terminalCommandSessionValidator{live: map[string]bool{"session-live": true}}
		if created, err := manager.AddTerminalTabForCommand(context.Background(), expected, terminalCommandTab("terminal-stale", "session-live"), sessions); !errors.Is(err, ErrCommandCreateTabStale) || created != nil {
			t.Fatalf("workspace stale aceito: created=%#v err=%v", created, err)
		}
	})

	t.Run("active tab ABA", func(t *testing.T) {
		manager := commandSnapshotManager(t)
		expected, err := manager.CommandSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.AddTab(Tab{ID: "aba-terminal", Type: TabTypeChat}); err != nil {
			t.Fatal(err)
		}
		if err := manager.RemoveTab("aba-terminal"); err != nil {
			t.Fatal(err)
		}
		sessions := &terminalCommandSessionValidator{live: map[string]bool{"session-live": true}}
		if created, err := manager.AddTerminalTabForCommand(context.Background(), expected, terminalCommandTab("terminal-aba", "session-live"), sessions); !errors.Is(err, ErrCommandCreateTabStale) || created != nil {
			t.Fatalf("snapshot ABA aceito: created=%#v err=%v", created, err)
		}
	})
}

func TestAddTerminalTabForCommandRecusaSessaoMortaIDVazioEEstadoExtra(t *testing.T) {
	cases := []struct {
		name string
		tab  Tab
	}{
		{name: "dead session", tab: terminalCommandTab("terminal-dead", "session-dead")},
		{name: "empty id", tab: terminalCommandTab("", "session-live")},
		{name: "conversation", tab: func() Tab {
			tab := terminalCommandTab("terminal-conversation", "session-live")
			tab.ConversationID = "conversation-forbidden"
			return tab
		}()},
		{name: "extra state", tab: func() Tab {
			tab := terminalCommandTab("terminal-extra", "session-live")
			tab.State["cwd"] = "forbidden"
			return tab
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := commandSnapshotManager(t)
			expected, err := manager.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			sessions := &terminalCommandSessionValidator{live: map[string]bool{"session-live": true}}
			if _, err := manager.AddTerminalTabForCommand(context.Background(), expected, tc.tab, sessions); err == nil {
				t.Fatal("aba terminal inválida foi aceita")
			}
			if manager.Active().FindTab(tc.tab.ID) != nil {
				t.Fatalf("recusa deixou aba terminal: %#v", tc.tab)
			}
		})
	}
}

func TestAddTerminalTabForCommandFazRollbackQuandoPersistenciaFalha(t *testing.T) {
	manager := commandSnapshotManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := manager.Active()
	badBase := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badBase, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	manager.activePath = badBase
	sessions := &terminalCommandSessionValidator{live: map[string]bool{"session-live": true}}
	if created, err := manager.AddTerminalTabForCommand(context.Background(), expected, terminalCommandTab("terminal-rollback", "session-live"), sessions); err == nil || created != nil {
		t.Fatalf("persistência falha não retornou erro: created=%#v err=%v", created, err)
	}
	after := manager.Active()
	if after.Tabs.Active != before.Tabs.Active || len(after.Tabs.Items) != len(before.Tabs.Items) || after.FindTab("terminal-rollback") != nil {
		t.Fatalf("rollback deixou mutação em memória: antes=%#v depois=%#v", before.Tabs, after.Tabs)
	}
	if len(sessions.closeCalls) != 0 {
		t.Fatalf("workspace tentou fechar sessão em rollback: %v", sessions.closeCalls)
	}
}
