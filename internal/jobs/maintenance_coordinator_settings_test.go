package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/config"
	"assistente/internal/configdir"
)

type recordingMaintenance struct {
	mu          sync.Mutex
	policies    []commandmaintenance.Policy
	compactions []int64
	calls       []string
}

func (r *recordingMaintenance) Retain(_ context.Context, policy commandmaintenance.Policy) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies = append(r.policies, policy)
	r.calls = append(r.calls, "retain")
	return 0, nil
}

func (r *recordingMaintenance) CleanOldDryRuns(context.Context, commandmaintenance.Policy) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "dry-runs")
	return 0, nil
}

func (r *recordingMaintenance) CleanOrphanChat(context.Context, commandmaintenance.Policy) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "orphan-chat")
	return 0, nil
}

func (r *recordingMaintenance) CleanOldChat(context.Context, commandmaintenance.Policy) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "old-chat")
	return 0, nil
}

func (r *recordingMaintenance) Compact(_ context.Context, minFreeBytes int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compactions = append(r.compactions, minFreeBytes)
	r.calls = append(r.calls, "compact")
	return nil
}

func (r *recordingMaintenance) policySnapshots() []commandmaintenance.Policy {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]commandmaintenance.Policy, len(r.policies))
	copy(out, r.policies)
	return out
}

func (r *recordingMaintenance) compactionSnapshots() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int64, len(r.compactions))
	copy(out, r.compactions)
	return out
}

func (r *recordingMaintenance) callSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

type recordingOutbox struct {
	more bool
}

func (r recordingOutbox) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	return 0, false, nil
}

func (r recordingOutbox) Drain(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{Processed: 0, More: r.more}, nil
}

type emptyMaintenanceRecovery struct{}

func (emptyMaintenanceRecovery) Recover(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{}, nil
}

func newRecordingCoordinator(t *testing.T, recorder *recordingMaintenance, outbox commandmaintenance.OutboxPort) *commandmaintenance.Coordinator {
	t.Helper()
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Outbox:       outbox,
		Decisions:    emptyMaintenanceRecovery{},
		Invocations:  emptyMaintenanceRecovery{},
		Claims:       emptyMaintenanceRecovery{},
		Jobs:         recorder,
		Tools:        recorder,
		InvocationDB: recorder,
		Activations:  recorder,
		Compaction:   recorder,
	})
	if err != nil {
		t.Fatalf("commandmaintenance.New: %v", err)
	}
	return coordinator
}

func isolatedJobsConfigFile(t *testing.T, raw []byte) string {
	t.Helper()
	tmpDir := t.TempDir()
	envNames := []string{
		"ASSISTENTE_HOME",
		"HOME",
		"USERPROFILE",
		"APPDATA",
		"LOCALAPPDATA",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
		"XDG_STATE_HOME",
		"XDG_CACHE_HOME",
	}
	t.Cleanup(func() {
		// Os valores são restaurados pelos t.Setenv registrados abaixo antes
		// deste cleanup, mantendo o cache coerente com o ambiente restaurado.
		configdir.ResetForTests()
	})
	for _, name := range envNames {
		t.Setenv(name, tmpDir)
	}
	configdir.ResetForTests()
	home := filepath.Clean(configdir.GetHomeDir())
	wantHome := filepath.Join(filepath.Clean(tmpDir), ".assistente")
	if home != filepath.Clean(wantHome) {
		t.Fatalf("configdir home=%q, want temporário %q", home, wantHome)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir config home temporário: %v", err)
	}
	path, err := config.GetConfigPath()
	if err != nil {
		t.Fatalf("config.GetConfigPath: %v", err)
	}
	if filepath.Clean(path) != filepath.Join(home, "config.json") {
		t.Fatalf("config path=%q, want dentro do home temporário %q", path, home)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config temporário: %v", err)
	}
	return path
}

func writeIsolatedMaintenanceSettings(t *testing.T, path string, settings config.MaintenanceSettings) {
	t.Helper()
	payload, err := json.Marshal(struct {
		Maintenance config.MaintenanceSettings `json:"maintenance"`
	}{Maintenance: settings})
	if err != nil {
		t.Fatalf("marshal maintenance settings: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("rewrite maintenance settings: %v", err)
	}
}

func TestRunRetentionRelêSettingsEntrePassagensNoCoordinatorReal(t *testing.T) {
	settings := config.DefaultMaintenanceSettings()
	settings.JobRetentionHours = 2
	settings.RunsPerJobKeep = 7
	settings.VacuumMinFreeBytes = 11
	path := isolatedJobsConfigFile(t, []byte(`{"maintenance":{}}`))
	writeIsolatedMaintenanceSettings(t, path, settings)

	repo, _, _ := setupJobsRepositoryTest(t)
	recorder := &recordingMaintenance{}
	coordinator := newRecordingCoordinator(t, recorder, recordingOutbox{})
	manager := mustNewManager(t, ManagerConfig{Repository: repo, MaintenanceCoordinator: coordinator})

	manager.runRetention(context.Background())
	settings.JobRetentionHours = 9
	settings.RunsPerJobKeep = 13
	settings.VacuumMinFreeBytes = 22
	settings.CommandInvocationRetentionDays = 12
	settings.CommandInvocationsPerUserKeep = 123
	settings.CommandInvocationsSystemKeep = 23
	settings.CommandActivationTerminalRetentionDays = 17
	settings.CommandActivationTerminalKeepPerUser = 321
	settings.CommandJobActivationLeaseSeconds = 240
	writeIsolatedMaintenanceSettings(t, path, settings)
	manager.runRetention(context.Background())

	policies := recorder.policySnapshots()
	if len(policies) != 6 {
		t.Fatalf("políticas recebidas=%d, want 6 (três portas em duas passagens)", len(policies))
	}
	if policies[0].JobRetention != 2*time.Hour || policies[0].RunsPerJobKeep != 7 {
		t.Fatalf("primeira política=%+v", policies[0])
	}
	if policies[3].JobRetention != 9*time.Hour || policies[3].RunsPerJobKeep != 13 {
		t.Fatalf("segunda política não refletiu config alterada: %+v", policies[3])
	}
	for _, policy := range policies[3:] {
		if policy.InvocationRetention != 12*24*time.Hour || policy.InvocationsPerUser != 123 || policy.InvocationsSystemKeep != 23 || policy.ActivationRetention != 17*24*time.Hour || policy.ActivationsPerUser != 321 || policy.LeaseDuration != 240*time.Second {
			t.Fatalf("settings de comandos não propagadas: %+v", policy)
		}
	}
	compactions := recorder.compactionSnapshots()
	if len(compactions) != 2 || compactions[0] != 11 || compactions[1] != 22 {
		t.Fatalf("limites de compactação=%v, want [11 22]", compactions)
	}
}

func TestRunRetentionConfigJSONInvalidoImpedeTodasAsLimpezas(t *testing.T) {
	isolatedJobsConfigFile(t, []byte(`{"maintenance":`))
	repo, _, _ := setupJobsRepositoryTest(t)
	recorder := &recordingMaintenance{}
	coordinator := newRecordingCoordinator(t, recorder, recordingOutbox{})
	manager := mustNewManager(t, ManagerConfig{Repository: repo, MaintenanceCoordinator: coordinator})

	manager.runRetention(context.Background())

	if got := len(recorder.policySnapshots()); got != 0 {
		t.Fatalf("limpezas de retenção executadas com JSON inválido: %d", got)
	}
	if got := len(recorder.compactionSnapshots()); got != 0 {
		t.Fatalf("compactação executada com JSON inválido: %d", got)
	}
	if calls := recorder.callSnapshot(); len(calls) != 0 {
		t.Fatalf("portas chamadas com JSON inválido: %v", calls)
	}
}

func TestRunRetentionMoreOutboxImpedeJobsECompactacao(t *testing.T) {
	settings := config.DefaultMaintenanceSettings()
	path := isolatedJobsConfigFile(t, []byte(`{"maintenance":{}}`))
	writeIsolatedMaintenanceSettings(t, path, settings)
	repo, _, _ := setupJobsRepositoryTest(t)
	recorder := &recordingMaintenance{}
	coordinator := newRecordingCoordinator(t, recorder, recordingOutbox{more: true})
	manager := mustNewManager(t, ManagerConfig{Repository: repo, MaintenanceCoordinator: coordinator})

	manager.runRetention(context.Background())

	if got := len(recorder.policySnapshots()); got != 0 {
		t.Fatalf("Jobs/retention executados com MoreOutbox: %d", got)
	}
	if got := len(recorder.compactionSnapshots()); got != 0 {
		t.Fatalf("compactação executada com MoreOutbox: %d", got)
	}
	if calls := recorder.callSnapshot(); len(calls) != 0 {
		t.Fatalf("portas de limpeza chamadas com MoreOutbox: %v", calls)
	}
}
