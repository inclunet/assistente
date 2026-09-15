package workspace

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandjson"
)

func TestCommandSnapshotCapturaApenasProjecaoSemantica(t *testing.T) {
	manager := &Manager{active: &Workspace{
		ID:       "ws-command",
		Name:     "nome que não deve sair",
		Profile:  "workspace-profile",
		LastUsed: time.Unix(10, 0),
		Tabs: TabsState{
			Active: "tab-editor",
			Items: []Tab{
				{ID: "tab-chat", Type: TabTypeChat, Title: "outro"},
				{
					ID:             "tab-editor",
					Type:           TabTypeEditor,
					ConversationID: "conversation-1",
					Title:          "título privado",
					ProfileOverride: map[string]any{
						"slug":  "profile-tab",
						"model": "não semântico para o snapshot",
					},
					State: map[string]any{
						"filePath": "C:/segredo/documento.md",
						"text":     "texto privado",
					},
				},
			},
		},
	}}

	snapshot, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	if snapshot.WorkspaceID != "ws-command" || snapshot.ActiveTabID != "tab-editor" {
		t.Fatalf("identidade do workspace/aba: %#v", snapshot)
	}
	if snapshot.WorkspaceProfile != "workspace-profile" {
		t.Fatalf("workspace profile: %q", snapshot.WorkspaceProfile)
	}
	if snapshot.Tab.ID != "tab-editor" || snapshot.Tab.Type != TabTypeEditor || snapshot.Tab.ConversationID != "conversation-1" || snapshot.Tab.ProfileOverrideSlug != "profile-tab" {
		t.Fatalf("projeção da aba: %#v", snapshot.Tab)
	}
	if snapshot.StateVersion == "" || len(snapshot.Fingerprint) != 64 || snapshot.Version == "" {
		t.Fatalf("versões/fingerprint ausentes: %#v", snapshot)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal(snapshot): %v", err)
	}
	serialized := string(encoded)
	for _, forbidden := range []string{"título privado", "C:/segredo/documento.md", "texto privado", "model", "surfaceId", "surfaceSnapshotVersion"} {
		if strings.Contains(serialized, forbidden) {
			t.Errorf("campo proibido vazou no snapshot: %q (%s)", forbidden, serialized)
		}
	}
}

func TestCommandSnapshotEStableEIgnoraLastUsedEApresentacao(t *testing.T) {
	workspace := &Workspace{
		ID:       "ws-stable",
		Name:     "nome antes",
		Profile:  "profile",
		LastUsed: time.Unix(1, 0),
		Tabs: TabsState{
			Active: "tab-1",
			Items: []Tab{{
				ID:    "tab-1",
				Type:  TabTypeTerminal,
				Title: "título antes",
				State: map[string]any{"b": float64(2), "a": "um"},
			}},
		},
	}
	manager := &Manager{active: workspace}

	first, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("primeiro snapshot: %v", err)
	}
	workspace.Name = "nome depois"
	workspace.LastUsed = time.Unix(99, 0)
	workspace.Tabs.Items[0].Title = "título depois"
	second, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("segundo snapshot: %v", err)
	}
	if first.Version != second.Version || first.Fingerprint != second.Fingerprint || first.StateVersion != second.StateVersion {
		t.Fatalf("campos não semânticos alteraram snapshot: primeiro=%#v segundo=%#v", first, second)
	}

	workspace.Tabs.Items[0].State["a"] = "dois"
	third, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após state: %v", err)
	}
	if third.Version == first.Version || third.Fingerprint == first.Fingerprint || third.StateVersion == first.StateVersion {
		t.Fatal("alteração semântica de State não alterou as versões")
	}
}

func TestCommandSnapshotVersaoObservaTodasAsAbasEOrdem(t *testing.T) {
	workspace := &Workspace{
		ID: "ws-all-tabs",
		Tabs: TabsState{
			Active: "tab-active",
			Items: []Tab{
				{ID: "tab-active", Type: TabTypeChat, State: map[string]any{"state": "active"}},
				{ID: "tab-other", Type: TabTypeEditor, Title: "não projetado"},
			},
		},
	}
	manager := &Manager{active: workspace}

	base, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot base: %v", err)
	}

	workspace.Tabs.Items[1].Title = "título alterado"
	unchanged, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após título: %v", err)
	}
	if base.Version != unchanged.Version {
		t.Fatal("título de aba não ativa não deveria alterar a versão")
	}

	workspace.Tabs.Items[1].State = map[string]any{"filePath": "C:/arquivo"}
	changedState, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após state não ativo: %v", err)
	}
	if base.Version == changedState.Version {
		t.Fatal("State de aba não ativa deveria alterar a versão")
	}

	workspace.Tabs.Items[1].State = nil
	workspace.Tabs.Items = []Tab{workspace.Tabs.Items[1], workspace.Tabs.Items[0]}
	reordered, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após reordenação: %v", err)
	}
	if base.Version == reordered.Version {
		t.Fatal("reordenação deveria alterar a versão")
	}

	workspace.Tabs.Items = []Tab{workspace.Tabs.Items[1]}
	removed, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após remoção: %v", err)
	}
	if base.Version == removed.Version {
		t.Fatal("remoção deveria alterar a versão")
	}
	workspace.Tabs.Items = append(workspace.Tabs.Items, Tab{ID: "tab-new", Type: TabTypeEditor})
	added, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após adição: %v", err)
	}
	if base.Version == added.Version {
		t.Fatal("adição deveria alterar a versão")
	}
}

func TestCommandSnapshotStateJCSDeterministicoEClonado(t *testing.T) {
	manager := &Manager{active: &Workspace{
		ID: "ws-jcs",
		Tabs: TabsState{
			Active: "tab-1",
			Items: []Tab{{
				ID:    "tab-1",
				Type:  TabTypeChat,
				State: map[string]any{"z": float64(3), "a": map[string]any{"y": true, "x": "v"}},
			}},
		},
	}}

	first, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("primeiro snapshot: %v", err)
	}
	manager.active.Tabs.Items[0].State["z"] = float64(4)
	manager.active.Tabs.Items[0].State["z"] = float64(3)
	second, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("segundo snapshot: %v", err)
	}
	if first.Version != second.Version || first.StateVersion != second.StateVersion {
		t.Fatal("a mesma semântica de State deveria manter a versão")
	}

	manager.active.Tabs.Items[0].State = map[string]any{"a": map[string]any{"x": "v", "y": true}, "z": float64(3)}
	third, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot com chaves reordenadas: %v", err)
	}
	if first.Version != third.Version || first.StateVersion != third.StateVersion {
		t.Fatal("a ordem das chaves de State não deveria alterar a versão")
	}

	manager.active.Tabs.Items[0].State["new"] = "mudei depois"
	if first.Tab.ID != "tab-1" || first.StateVersion != second.StateVersion {
		t.Fatal("snapshot retornado não permaneceu independente do manager")
	}
}

func TestCommandSnapshotFalhaParaManagerAusenteOuStateNaoJSON(t *testing.T) {
	var nilManager *Manager
	if _, err := nilManager.CommandSnapshot(); !errors.Is(err, ErrCommandSnapshotNilManager) {
		t.Fatalf("manager nil: %v", err)
	}
	if _, err := (&Manager{}).CommandSnapshot(); !errors.Is(err, ErrCommandSnapshotUninitialized) {
		t.Fatalf("manager não inicializado: %v", err)
	}

	for name, state := range map[string]any{
		"func":    func() {},
		"channel": make(chan int),
	} {
		t.Run(name, func(t *testing.T) {
			manager := &Manager{active: &Workspace{
				ID:   "ws-invalid-state",
				Tabs: TabsState{Active: "tab-1", Items: []Tab{{ID: "tab-1", Type: TabTypeEditor, State: map[string]any{"bad": state}}}},
			}}
			_, err := manager.CommandSnapshot()
			if !errors.Is(err, commandjson.ErrInvalidValue) {
				t.Fatalf("state não JSON: %v", err)
			}
		})
	}

	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	manager := &Manager{active: &Workspace{
		ID:   "ws-cyclic-state",
		Tabs: TabsState{Active: "tab-1", Items: []Tab{{ID: "tab-1", Type: TabTypeEditor, State: cyclic}}},
	}}
	if _, err := manager.CommandSnapshot(); !errors.Is(err, commandjson.ErrInvalidValue) {
		t.Fatalf("state cíclico: %v", err)
	}
}

func TestCommandSnapshotValidaIDsTiposESlug(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Workspace)
	}{
		{name: "workspace id vazio", setup: func(ws *Workspace) { ws.ID = " " }},
		{name: "active tab id vazio", setup: func(ws *Workspace) { ws.Tabs.Active = " " }},
		{name: "tab id não canônico", setup: func(ws *Workspace) { ws.Tabs.Items[0].ID = " tab-1" }},
		{name: "conversation id com NUL", setup: func(ws *Workspace) { ws.Tabs.Items[0].ConversationID = "conversation\x00id" }},
		{name: "id acima do limite", setup: func(ws *Workspace) { ws.ID = strings.Repeat("x", maxCommandSnapshotIdentifierLength+1) }},
		{name: "tipo inválido", setup: func(ws *Workspace) { ws.Tabs.Items[0].Type = TabType("unknown") }},
		{name: "slug não string", setup: func(ws *Workspace) { ws.Tabs.Items[0].ProfileOverride = map[string]any{"slug": 42} }},
		{name: "slug vazio", setup: func(ws *Workspace) { ws.Tabs.Items[0].ProfileOverride = map[string]any{"slug": ""} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ws := &Workspace{
				ID: "ws-valid",
				Tabs: TabsState{
					Active: "tab-1",
					Items:  []Tab{{ID: "tab-1", Type: TabTypeChat}},
				},
			}
			test.setup(ws)
			if _, err := (&Manager{active: ws}).CommandSnapshot(); !errors.Is(err, ErrCommandSnapshotInvalidData) {
				t.Fatalf("erro de validação: %v", err)
			}
		})
	}

	ws := &Workspace{
		ID: "ws opaco/real",
		Tabs: TabsState{
			Active: "tab opaca/real",
			Items: []Tab{{
				ID:             "tab opaca/real",
				Type:           TabTypeChat,
				ConversationID: "conversation opaca/real",
			}},
		},
	}
	if snapshot, err := (&Manager{active: ws}).CommandSnapshot(); err != nil {
		t.Fatalf("IDs opacos válidos foram rejeitados: %v", err)
	} else if snapshot.WorkspaceID != ws.ID || snapshot.ActiveTabID != ws.Tabs.Active || snapshot.Tab.ID != ws.Tabs.Active || snapshot.Tab.ConversationID != ws.Tabs.Items[0].ConversationID {
		t.Fatalf("IDs opacos não foram preservados: %#v", snapshot)
	}
}

func TestCommandSnapshotWorkspaceSemAbaAtivaFicaIndisponivelSemEcoarID(t *testing.T) {
	for _, activeTabID := range []string{"", "tab-id-que-nao-deve-vazar"} {
		manager := &Manager{active: &Workspace{
			ID:   "ws-empty",
			Tabs: TabsState{Active: activeTabID},
		}}
		_, err := manager.CommandSnapshot()
		if !errors.Is(err, ErrCommandSnapshotActiveTabUnavailable) {
			t.Fatalf("erro de aba ativa ausente para %q: %v", activeTabID, err)
		}
		if activeTabID != "" && strings.Contains(err.Error(), activeTabID) {
			t.Fatalf("erro ecoou valor de payload: %v", err)
		}
	}
}

func TestCommandSnapshotNaoCriaIdentidadeDeSurface(t *testing.T) {
	manager := &Manager{active: &Workspace{
		ID:   "ws-surface",
		Tabs: TabsState{Active: "tab-1", Items: []Tab{{ID: "tab-1", Type: TabTypeTasklist}}},
	}}
	snapshot, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	if snapshot.Tab.ID != "tab-1" || snapshot.ActiveTabID != "tab-1" {
		t.Fatalf("snapshot de aba incorreto: %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"surface_id"`) || strings.Contains(string(encoded), `"surface_snapshot_version"`) {
		t.Fatalf("snapshot de aba criou identidade de surface: %s", encoded)
	}
}

func BenchmarkCommandSnapshot100Tabs(b *testing.B) {
	items := make([]Tab, 100)
	for i := range items {
		items[i] = Tab{
			ID:       "tab-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Type:     TabTypeEditor,
			State:    map[string]any{"filePath": "C:/workspace/file.md", "version": float64(i)},
			Position: i,
		}
	}
	manager := &Manager{active: &Workspace{
		ID:   "ws-benchmark",
		Tabs: TabsState{Active: items[0].ID, Items: items},
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := manager.CommandSnapshot(); err != nil {
			b.Fatal(err)
		}
	}
}
