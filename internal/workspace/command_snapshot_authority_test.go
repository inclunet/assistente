package workspace

import (
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func commandSnapshotManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "assistente-home"))
	if err := manager.Initialize(filepath.Join(root, "workspace")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := manager.SetProfile("profile-a"); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	active := manager.Active()
	if active == nil || active.ActiveTab() == nil {
		t.Fatal("fixture sem workspace/aba ativa")
	}
	if err := manager.UpdateTab(active.Tabs.Active, map[string]any{
		"title":            "apresentação-a",
		"state":            map[string]any{"semantic": "a"},
		"profile_override": map[string]any{"slug": "tab-profile-a", "model": "model-a"},
	}); err != nil {
		t.Fatalf("UpdateTab fixture: %v", err)
	}
	return manager
}

func TestCommandSnapshotAuthoritativoObservaABADeAbaEPerfil(t *testing.T) {
	manager := commandSnapshotManager(t)
	activeTabID := manager.Active().Tabs.Active

	base, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot base: %v", err)
	}

	if err := manager.UpdateTab(activeTabID, map[string]any{
		"title":            "apresentação-b",
		"state":            map[string]any{"semantic": "b"},
		"profile_override": map[string]any{"slug": "tab-profile-b"},
	}); err != nil {
		t.Fatalf("mutar aba para B: %v", err)
	}
	if err := manager.SetProfile("profile-b"); err != nil {
		t.Fatalf("mutar profile para B: %v", err)
	}
	changed, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot B: %v", err)
	}
	if changed.Version == base.Version || changed.StateVersion == base.StateVersion {
		t.Fatalf("mutação semântica não observada: base=%#v changed=%#v", base, changed)
	}

	if err := manager.UpdateTab(activeTabID, map[string]any{
		"title":            "apresentação-a-restaurada",
		"state":            map[string]any{"semantic": "a"},
		"profile_override": map[string]any{"slug": "tab-profile-a"},
	}); err != nil {
		t.Fatalf("reverter aba para A: %v", err)
	}
	if err := manager.SetProfile("profile-a"); err != nil {
		t.Fatalf("reverter profile para A: %v", err)
	}
	reverted, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot A restaurado: %v", err)
	}
	if reverted.Version == base.Version || reverted.Fingerprint != base.Fingerprint || reverted.StateVersion != base.StateVersion {
		t.Fatalf("ABA reutilizou versão ou não restaurou fingerprint: base=%#v reverted=%#v", base, reverted)
	}
}

func TestCommandSnapshotAuthoritativoObservaABADoWorkspaceEIgnoraApresentacaoENoop(t *testing.T) {
	manager := commandSnapshotManager(t)
	original := manager.Active().ID
	base, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot base: %v", err)
	}

	if err := manager.Rename("nome-apresentação"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := manager.SetProfile("profile-a"); err != nil {
		t.Fatalf("SetProfile no-op: %v", err)
	}
	if err := manager.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	unchanged, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após apresentação/no-op: %v", err)
	}
	if unchanged.Version != base.Version || unchanged.Fingerprint != base.Fingerprint {
		t.Fatalf("apresentação/no-op alterou versão: base=%#v unchanged=%#v", base, unchanged)
	}

	other, err := manager.Create("workspace-b")
	if err != nil {
		t.Fatalf("Create workspace B: %v", err)
	}
	if _, err := manager.Switch(other.ID); err != nil {
		t.Fatalf("Switch para B: %v", err)
	}
	if err := manager.SetProfile("profile-b"); err != nil {
		t.Fatalf("profile do workspace B: %v", err)
	}
	workspaceB, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot workspace B: %v", err)
	}
	if workspaceB.WorkspaceID != other.ID || workspaceB.Version == base.Version {
		t.Fatalf("troca para workspace B não observada: base=%#v B=%#v", base, workspaceB)
	}

	if _, err := manager.Switch(original); err != nil {
		t.Fatalf("Switch de volta para A: %v", err)
	}
	workspaceA, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot workspace A restaurado: %v", err)
	}
	if workspaceA.Version == base.Version || workspaceA.Fingerprint != base.Fingerprint {
		t.Fatalf("ABA de workspace reutilizou versão ou não restaurou fingerprint: base=%#v A=%#v", base, workspaceA)
	}
}

func TestCommandSnapshotAguardaLockEObservaMutacaoAtomica(t *testing.T) {
	manager := commandSnapshotManager(t)
	base, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.active.Profile = "profile-b"
	manager.active.Tabs.Items[0].State = map[string]any{"semantic": "b"}
	result := make(chan CommandSnapshot, 1)
	go func() {
		snapshot, snapshotErr := manager.CommandSnapshot()
		if snapshotErr == nil {
			result <- snapshot
		}
	}()
	select {
	case <-result:
		manager.mu.Unlock()
		t.Fatal("CommandSnapshot leu enquanto a mutação segurava o lock")
	case <-time.After(20 * time.Millisecond):
	}
	manager.mu.Unlock()

	select {
	case snapshot := <-result:
		if snapshot.Version == base.Version || snapshot.WorkspaceProfile != "profile-b" {
			t.Fatalf("snapshot não observou estado atômico: base=%#v atual=%#v", base, snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("CommandSnapshot não concluiu após liberar o lock")
	}
}

func TestCommandSnapshotRecusaEpochSaturadoParaNaoReutilizarVersao(t *testing.T) {
	manager := commandSnapshotManager(t)
	manager.mu.Lock()
	manager.commandEpoch = math.MaxUint64
	manager.mu.Unlock()

	if _, err := manager.CommandSnapshot(); !errors.Is(err, ErrCommandSnapshotEpochExhausted) {
		t.Fatalf("epoch saturado deveria recusar snapshot: %v", err)
	}
}

func TestCommandSnapshotMutacaoComSlugMalformadoFalhaFechadoSemPanic(t *testing.T) {
	manager := commandSnapshotManager(t)
	activeTabID := manager.Active().Tabs.Active

	if err := manager.UpdateTab(activeTabID, map[string]any{
		"profile_override": map[string]any{"slug": []string{"malformado"}},
	}); err != nil {
		t.Fatalf("UpdateTab com slug malformado: %v", err)
	}
	if _, err := manager.CommandSnapshot(); !errors.Is(err, ErrCommandSnapshotInvalidData) {
		t.Fatalf("slug malformado deveria invalidar snapshot: %v", err)
	}
}

func TestManagerDesanexaIngressosMutaveisDeAddTabEUpdateTab(t *testing.T) {
	manager := commandSnapshotManager(t)

	state := map[string]any{
		"nested": map[string]any{"value": "a"},
		"items":  []any{map[string]any{"value": "a"}},
	}
	override := map[string]any{
		"slug":  "ingress-profile",
		"meta":  map[string]any{"value": "a"},
		"items": []any{"a"},
	}
	tab := Tab{ID: "tab-ingress", Type: TabTypeEditor, State: state, ProfileOverride: override}
	if err := manager.AddTab(tab); err != nil {
		t.Fatalf("AddTab: %v", err)
	}
	afterAdd, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após AddTab: %v", err)
	}

	state["nested"].(map[string]any)["value"] = "caller-mutated"
	state["items"].([]any)[0].(map[string]any)["value"] = "caller-mutated"
	override["meta"].(map[string]any)["value"] = "caller-mutated"
	override["items"].([]any)[0] = "caller-mutated"
	afterCallerMutation, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após mutação do caller de AddTab: %v", err)
	}
	if afterCallerMutation.Version != afterAdd.Version || afterCallerMutation.Fingerprint != afterAdd.Fingerprint {
		t.Fatalf("caller reteve alias após AddTab: antes=%#v depois=%#v", afterAdd, afterCallerMutation)
	}

	updateState := map[string]any{"nested": map[string]any{"value": "b"}}
	updateOverride := map[string]any{"meta": map[string]any{"value": "b"}}
	if err := manager.UpdateTab("tab-ingress", map[string]any{
		"state":            updateState,
		"profile_override": updateOverride,
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	afterUpdate, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após UpdateTab: %v", err)
	}
	updateState["nested"].(map[string]any)["value"] = "caller-mutated"
	updateOverride["meta"].(map[string]any)["value"] = "caller-mutated"
	afterUpdateCallerMutation, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após mutação do caller de UpdateTab: %v", err)
	}
	if afterUpdateCallerMutation.Version != afterUpdate.Version || afterUpdateCallerMutation.Fingerprint != afterUpdate.Fingerprint {
		t.Fatalf("caller reteve alias após UpdateTab: antes=%#v depois=%#v", afterUpdate, afterUpdateCallerMutation)
	}
}

func TestCommandSnapshotRemoveUltimaEARecriaMesmoIDInvalidaABA(t *testing.T) {
	manager := commandSnapshotManager(t)
	base, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot base: %v", err)
	}
	tab := manager.Active().Tabs.Items[0]

	if err := manager.RemoveTab(tab.ID); err != nil {
		t.Fatalf("RemoveTab última aba: %v", err)
	}
	if err := manager.AddTab(tab); err != nil {
		t.Fatalf("AddTab recriando mesma aba: %v", err)
	}
	current, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após remoção/recriação: %v", err)
	}
	if current.Version == base.Version || current.Fingerprint != base.Fingerprint {
		t.Fatalf("remoção/recriação ABA reutilizou versão ou mudou fingerprint: base=%#v atual=%#v", base, current)
	}
}

func TestManagerLeiturasDeActiveESwitchNaoExibemAliasMutavel(t *testing.T) {
	manager := commandSnapshotManager(t)
	base, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot base: %v", err)
	}

	active := manager.Active()
	active.Profile = "caller-profile"
	active.Tabs.Items[0].State = map[string]any{"caller": "mutated"}
	got, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após mutação de Active: %v", err)
	}
	if got.Version != base.Version || got.Fingerprint != base.Fingerprint {
		t.Fatalf("Active expôs alias mutável: base=%#v atual=%#v", base, got)
	}

	other, err := manager.Create("workspace-switch-copy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if switched, err := manager.Switch(other.ID); err != nil {
		t.Fatalf("Switch: %v", err)
	} else {
		switched.Profile = "caller-profile"
		switched.Tabs.Items[0].State = map[string]any{"caller": "mutated"}
	}
	got, err = manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot após mutação do retorno de Switch: %v", err)
	}
	if got.WorkspaceProfile == "caller-profile" || got.Fingerprint == base.Fingerprint {
		t.Fatalf("Switch expôs alias ou não mudou workspace: base=%#v atual=%#v", base, got)
	}
}

func TestManagerActiveIDHotPathNaoAloca(t *testing.T) {
	manager := commandSnapshotManager(t)
	if _, err := manager.CommandSnapshot(); err != nil {
		t.Fatalf("snapshot existente: %v", err)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = manager.ActiveID()
	})
	if allocs != 0 {
		t.Fatalf("ActiveID deveria ser zero-allocation, obteve %v alocações", allocs)
	}
}
