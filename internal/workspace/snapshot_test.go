package workspace

import (
	"context"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestActiveSnapshotHasStableEpochAndDecimalPublicationSequence(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	first := m.Active()
	second := m.Active()
	if first == nil || second == nil {
		t.Fatal("expected active snapshots")
	}
	if first.SnapshotEpoch == "" || first.SnapshotEpoch != second.SnapshotEpoch {
		t.Fatalf("epoch de transporte inconsistente: first=%q second=%q", first.SnapshotEpoch, second.SnapshotEpoch)
	}
	firstSequence, err := strconv.ParseUint(first.SnapshotSequence, 10, 64)
	if err != nil || first.SnapshotSequence == "" {
		t.Fatalf("primeira sequência não decimal: %q err=%v", first.SnapshotSequence, err)
	}
	secondSequence, err := strconv.ParseUint(second.SnapshotSequence, 10, 64)
	if err != nil || secondSequence <= firstSequence {
		t.Fatalf("sequência não crescente: first=%q second=%q", first.SnapshotSequence, second.SnapshotSequence)
	}
	if m.active.SnapshotEpoch != "" || m.active.SnapshotSequence != "" {
		t.Fatalf("metadados vazaram para o root ativo: %#v", m.active)
	}
}

func TestActiveConcurrentSnapshotsHaveUniqueSequencesAndDetachedClones(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	const count = 64
	results := make(chan *Workspace, count)
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			results <- m.Active()
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, count)
	var first *Workspace
	for snapshot := range results {
		if snapshot == nil || snapshot.SnapshotEpoch == "" || snapshot.SnapshotSequence == "" {
			t.Fatalf("snapshot concorrente sem metadados: %#v", snapshot)
		}
		if _, err := strconv.ParseUint(snapshot.SnapshotSequence, 10, 64); err != nil {
			t.Fatalf("sequence não decimal: %q", snapshot.SnapshotSequence)
		}
		if _, exists := seen[snapshot.SnapshotSequence]; exists {
			t.Fatalf("sequence duplicada: %q", snapshot.SnapshotSequence)
		}
		seen[snapshot.SnapshotSequence] = struct{}{}
		if first == nil {
			first = snapshot
		}
	}
	if len(seen) != count {
		t.Fatalf("quantidade de sequences=%d, want=%d", len(seen), count)
	}
	first.Tabs.Items[0].Title = "mutação no clone"
	if got := m.Active().Tabs.Items[0].Title; got == "mutação no clone" {
		t.Fatal("mutação de clone concorrente vazou para o root")
	}
}

func TestActiveDoesNotChangeCommandEpochOrCommandSnapshotVersion(t *testing.T) {
	m := commandSnapshotManager(t)
	before, err := m.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot antes: %v", err)
	}
	epoch := m.commandEpoch
	for i := 0; i < 10; i++ {
		if got := m.Active(); got == nil {
			t.Fatal("Active retornou nil")
		}
	}
	after, err := m.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot depois: %v", err)
	}
	if m.commandEpoch != epoch || before.Version != after.Version {
		t.Fatalf("leitura de transporte alterou comando: epoch=%d/%d version=%q/%q", epoch, m.commandEpoch, before.Version, after.Version)
	}
}

func TestSwitchABAReturnsIncreasingSequencesInSameEpoch(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	a := m.Active()
	bWorkspace, err := m.Create("B")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	b, err := m.Switch(bWorkspace.ID)
	if err != nil {
		t.Fatalf("Switch B: %v", err)
	}
	aAgain, err := m.Switch(a.ID)
	if err != nil {
		t.Fatalf("Switch A: %v", err)
	}
	aSequence, _ := strconv.ParseUint(a.SnapshotSequence, 10, 64)
	bSequence, _ := strconv.ParseUint(b.SnapshotSequence, 10, 64)
	aAgainSequence, _ := strconv.ParseUint(aAgain.SnapshotSequence, 10, 64)
	if a.SnapshotEpoch == "" || a.SnapshotEpoch != b.SnapshotEpoch || b.SnapshotEpoch != aAgain.SnapshotEpoch {
		t.Fatalf("epoch ABA inconsistente: A=%q B=%q A2=%q", a.SnapshotEpoch, b.SnapshotEpoch, aAgain.SnapshotEpoch)
	}
	if aSequence >= bSequence || bSequence >= aAgainSequence {
		t.Fatalf("sequences ABA não crescentes: A=%d B=%d A2=%d", aSequence, bSequence, aAgainSequence)
	}
}

func assertStampedLaterThanActive(t *testing.T, m *Manager, returned *Workspace) {
	t.Helper()
	if returned == nil || returned.SnapshotEpoch == "" || returned.SnapshotSequence == "" {
		t.Fatalf("mutator não retornou snapshot carimbado: %#v", returned)
	}
	later := m.Active()
	returnedSequence, err := strconv.ParseUint(returned.SnapshotSequence, 10, 64)
	if err != nil {
		t.Fatalf("sequence retornada não decimal: %q", returned.SnapshotSequence)
	}
	laterSequence, err := strconv.ParseUint(later.SnapshotSequence, 10, 64)
	if err != nil || returned.SnapshotEpoch != later.SnapshotEpoch || returnedSequence >= laterSequence {
		t.Fatalf("snapshot posterior inválido: returned=%#v later=%#v", returned, later)
	}
	returnedSemantic := cloneWorkspace(returned)
	laterSemantic := cloneWorkspace(later)
	returnedSemantic.SnapshotEpoch = ""
	returnedSemantic.SnapshotSequence = ""
	laterSemantic.SnapshotEpoch = ""
	laterSemantic.SnapshotSequence = ""
	if !reflect.DeepEqual(returnedSemantic, laterSemantic) {
		t.Fatalf("retorno e Active posterior divergem semanticamente: returned=%#v later=%#v", returnedSemantic, laterSemantic)
	}
}

func TestCommandMutatorsReturnStampedSnapshots(t *testing.T) {
	createManager := commandSnapshotManager(t)
	expected, err := createManager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot create: %v", err)
	}
	created, err := createManager.AddTabForCommand(context.Background(), expected, commandTabForType("created-command-tab", TabTypeEditor))
	if err != nil {
		t.Fatalf("AddTabForCommand: %v", err)
	}
	assertStampedLaterThanActive(t, createManager, created)

	closeManager := commandWorkspaceWithTabs(t)
	closeExpected, err := closeManager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot close: %v", err)
	}
	closed, err := closeManager.CloseActiveTabForCommand(context.Background(), closeExpected, "")
	if err != nil {
		t.Fatalf("CloseActiveTabForCommand: %v", err)
	}
	assertStampedLaterThanActive(t, closeManager, closed)

	selectionManager, _, target := workspaceTabSelectionManager(t)
	selected, err := selectionManager.SetActiveWorkspaceTabForWorkspace(
		context.Background(), selectionManager.Active().ID, target,
	)
	if err != nil {
		t.Fatalf("SetActiveWorkspaceTabForWorkspace: %v", err)
	}
	assertStampedLaterThanActive(t, selectionManager, selected)
}

func TestSnapshotLazilyInitializesEpochForManagerLiteral(t *testing.T) {
	m := &Manager{active: &Workspace{
		ID:   "ws-literal",
		Name: "Literal",
		Tabs: TabsState{Active: "tab-1", Items: []Tab{{ID: "tab-1", Type: TabTypeChat}}},
	}}
	got := m.Active()
	if got == nil || got.SnapshotEpoch == "" || got.SnapshotSequence != "1" {
		t.Fatalf("snapshot de Manager literal inválido: %#v", got)
	}
	if m.snapshotEpoch == "" || m.snapshotSequence != 1 {
		t.Fatalf("estado lazy não foi preservado: epoch=%q sequence=%d", m.snapshotEpoch, m.snapshotSequence)
	}
}

func TestSnapshotSequenceOverflowFailsClosedWithoutReset(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	m.snapshotMu.Lock()
	m.snapshotSequence = math.MaxUint64
	epoch := m.snapshotEpoch
	m.snapshotMu.Unlock()

	if got := m.Active(); got != nil {
		t.Fatalf("overflow publicou snapshot: %#v", got)
	}
	m.snapshotMu.Lock()
	defer m.snapshotMu.Unlock()
	if m.snapshotSequence != math.MaxUint64 || m.snapshotEpoch != epoch {
		t.Fatalf("overflow resetou estado: epoch=%q sequence=%d", m.snapshotEpoch, m.snapshotSequence)
	}
}

func TestSnapshotCloneClearsIncomingTransportMetadata(t *testing.T) {
	m := NewManager(t.TempDir())
	m.active = &Workspace{
		ID:               "ws-metadata",
		Name:             "Metadata",
		SnapshotEpoch:    "old-epoch",
		SnapshotSequence: "99",
	}
	got := m.Active()
	if got == nil || got.SnapshotEpoch == "old-epoch" || got.SnapshotSequence == "99" {
		t.Fatalf("metadados antigos vazaram para novo clone: %#v", got)
	}
	if m.active.SnapshotEpoch != "old-epoch" || m.active.SnapshotSequence != "99" {
		t.Fatalf("clone alterou o root: %#v", m.active)
	}
}

func TestWorkspaceSnapshotMetadataIsExcludedFromYAML(t *testing.T) {
	ws := &Workspace{
		ID:               "ws-1",
		Name:             "Workspace",
		SnapshotEpoch:    "epoch-runtime",
		SnapshotSequence: "18446744073709551615",
	}
	data, err := yaml.Marshal(ws)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "snapshot_epoch") || strings.Contains(text, "snapshot_sequence") {
		t.Fatalf("metadados de transporte persistidos no YAML: %s", text)
	}
}

func TestCreateDoesNotReplaceActiveSnapshotRoot(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	active := m.Active()
	created, err := m.Create("created")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == active.ID {
		t.Fatalf("Create retornou o workspace ativo: created=%q active=%q", created.ID, active.ID)
	}
	if created.SnapshotEpoch != "" || created.SnapshotSequence != "" {
		t.Fatalf("Create recebeu metadados de snapshot ativo: %#v", created)
	}
	if got := m.Active(); got.ID != active.ID || got.SnapshotEpoch != active.SnapshotEpoch {
		t.Fatalf("Create substituiu a raiz ativa: got=%#v before=%#v", got, active)
	}
}

func TestSwitchReturnsPublicationSnapshot(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	target, err := m.Create("target")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := m.Switch(target.ID)
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got.SnapshotEpoch == "" || got.SnapshotSequence == "" {
		t.Fatalf("Switch não retornou snapshot de publicação: %#v", got)
	}
	active := m.Active()
	if got.SnapshotEpoch != active.SnapshotEpoch {
		t.Fatalf("retorno de Switch diverge do epoch atual: returned=%#v active=%#v", got, active)
	}
	returnedSequence, err := strconv.ParseUint(got.SnapshotSequence, 10, 64)
	if err != nil {
		t.Fatalf("sequence de Switch não decimal: %q", got.SnapshotSequence)
	}
	activeSequence, err := strconv.ParseUint(active.SnapshotSequence, 10, 64)
	if err != nil || activeSequence <= returnedSequence {
		t.Fatalf("leitura posterior não recebeu sequence maior: returned=%q active=%q", got.SnapshotSequence, active.SnapshotSequence)
	}
}
