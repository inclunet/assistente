package jobs

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/config"
)

type dynamicMaintenanceHeartbeatSpy struct {
	mu       sync.Mutex
	policies []commandmaintenance.Policy
	seen     []commandmaintenance.Policy
	matched  []bool
}

func (s *dynamicMaintenanceHeartbeatSpy) Heartbeat(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.BatchResult, error) {
	fromContext, ok := commandmaintenance.PolicyFromContext(ctx)
	s.mu.Lock()
	s.policies = append(s.policies, policy)
	s.seen = append(s.seen, fromContext)
	s.matched = append(s.matched, ok && fromContext == policy)
	s.mu.Unlock()
	return commandmaintenance.BatchResult{}, nil
}

func (s *dynamicMaintenanceHeartbeatSpy) snapshots() (policies, seen []commandmaintenance.Policy, matched []bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]commandmaintenance.Policy(nil), s.policies...), append([]commandmaintenance.Policy(nil), s.seen...), append([]bool(nil), s.matched...)
}

func newDynamicSettingsCoordinator(t *testing.T, heartbeat *dynamicMaintenanceHeartbeatSpy) *commandmaintenance.Coordinator {
	t.Helper()
	recorder := &recordingMaintenance{}
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Heartbeat:    heartbeat,
		Outbox:       recordingOutbox{},
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

func TestRunCommandMaintenanceReadsSavedSettingsSnapshotsSequentially(t *testing.T) {
	path := isolatedJobsConfigFile(t, []byte(`{"maintenance":{}}`))
	first := config.DefaultMaintenanceSettings()
	first.CommandInvocationRetentionDays = 11
	first.CommandInvocationsPerUserKeep = 17
	first.CommandInvocationsSystemKeep = 19
	first.CommandActivationTerminalRetentionDays = 23
	first.CommandActivationTerminalKeepPerUser = 29
	first.CommandJobActivationLeaseSeconds = 31
	if err := config.SaveMaintenance(first); err != nil {
		t.Fatalf("SaveMaintenance primeira política: %v", err)
	}

	heartbeat := &dynamicMaintenanceHeartbeatSpy{}
	manager := mustNewManager(t, ManagerConfig{MaintenanceCoordinator: newDynamicSettingsCoordinator(t, heartbeat)})
	manager.runCommandMaintenance(context.Background())

	second := first
	second.CommandInvocationRetentionDays = 37
	second.CommandInvocationsPerUserKeep = 41
	second.CommandInvocationsSystemKeep = 43
	second.CommandActivationTerminalRetentionDays = 47
	second.CommandActivationTerminalKeepPerUser = 53
	second.CommandJobActivationLeaseSeconds = 59
	if err := config.SaveMaintenance(second); err != nil {
		t.Fatalf("SaveMaintenance segunda política: %v", err)
	}
	manager.runCommandMaintenance(context.Background())

	policies, seen, matched := heartbeat.snapshots()
	if len(policies) != 2 || len(seen) != 2 || len(matched) != 2 {
		t.Fatalf("snapshots heartbeat=%d context=%d match=%d, want 2/2/2", len(policies), len(seen), len(matched))
	}
	for i, ok := range matched {
		if !ok {
			t.Fatalf("passagem %d recebeu PolicyFromContext divergente: arg=%+v ctx=%+v", i, policies[i], seen[i])
		}
	}
	if policies[0].InvocationRetention != 11*24*time.Hour || policies[0].InvocationsPerUser != 17 || policies[0].InvocationsSystemKeep != 19 || policies[0].ActivationRetention != 23*24*time.Hour || policies[0].ActivationsPerUser != 29 || policies[0].LeaseDuration != 31*time.Second {
		t.Fatalf("primeiro snapshot não refletiu SaveMaintenance: %+v", policies[0])
	}
	if policies[1].InvocationRetention != 37*24*time.Hour || policies[1].InvocationsPerUser != 41 || policies[1].InvocationsSystemKeep != 43 || policies[1].ActivationRetention != 47*24*time.Hour || policies[1].ActivationsPerUser != 53 || policies[1].LeaseDuration != 59*time.Second {
		t.Fatalf("segundo snapshot não refletiu SaveMaintenance: %+v", policies[1])
	}

	if got, err := config.GetMaintenance(); err != nil || got != second {
		t.Fatalf("config pessoal/temporária após passagens=%+v err=%v path=%s", got, err, path)
	}
}

func TestCommandMaintenancePolicySixSettingsRejectZeroAndOverflow(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.MaintenanceSettings)
	}{
		{name: "invocation retention zero", mutate: func(s *config.MaintenanceSettings) { s.CommandInvocationRetentionDays = 0 }},
		{name: "per user zero", mutate: func(s *config.MaintenanceSettings) { s.CommandInvocationsPerUserKeep = 0 }},
		{name: "system zero", mutate: func(s *config.MaintenanceSettings) { s.CommandInvocationsSystemKeep = 0 }},
		{name: "activation retention zero", mutate: func(s *config.MaintenanceSettings) { s.CommandActivationTerminalRetentionDays = 0 }},
		{name: "activation cap zero", mutate: func(s *config.MaintenanceSettings) { s.CommandActivationTerminalKeepPerUser = 0 }},
		{name: "lease zero", mutate: func(s *config.MaintenanceSettings) { s.CommandJobActivationLeaseSeconds = 0 }},
		{name: "invocation retention overflow", mutate: func(s *config.MaintenanceSettings) { s.CommandInvocationRetentionDays = math.MaxInt }},
		{name: "activation retention overflow", mutate: func(s *config.MaintenanceSettings) { s.CommandActivationTerminalRetentionDays = math.MaxInt }},
		{name: "lease overflow", mutate: func(s *config.MaintenanceSettings) { s.CommandJobActivationLeaseSeconds = math.MaxInt }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := config.DefaultMaintenanceSettings()
			tc.mutate(&settings)
			if _, err := commandMaintenancePolicy(settings); !errors.Is(err, commandmaintenance.ErrInvalid) {
				t.Fatalf("settings=%+v err=%v, want commandmaintenance.ErrInvalid", settings, err)
			}
		})
	}
}
