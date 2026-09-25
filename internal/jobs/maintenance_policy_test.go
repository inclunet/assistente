package jobs

import (
	"errors"
	"math"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/config"
)

func TestCommandMaintenancePolicyFromSettings(t *testing.T) {
	s := config.DefaultMaintenanceSettings()
	p, err := commandMaintenancePolicy(s)
	if err != nil {
		t.Fatal(err)
	}
	if p.InvocationRetention != 30*24*time.Hour || p.ActivationRetention != 30*24*time.Hour || p.LeaseDuration != 180*time.Second || p.InvocationsPerUser != 10000 || p.InvocationsSystemKeep != 1000 || p.ActivationsPerUser != 10000 {
		t.Fatalf("policy = %+v", p)
	}
	s.CommandInvocationRetentionDays = 42
	s.CommandInvocationsPerUserKeep = 71
	s.CommandInvocationsSystemKeep = 13
	s.CommandActivationTerminalRetentionDays = 19
	s.CommandActivationTerminalKeepPerUser = 81
	s.CommandJobActivationLeaseSeconds = 240
	s.RunsPerJobKeep = 0
	s.ChatToolCallsRetentionDays = 0
	q, err := commandMaintenancePolicy(s)
	if err != nil {
		t.Fatal(err)
	}
	if q.InvocationRetention != 42*24*time.Hour || q.ActivationRetention != 19*24*time.Hour || q.LeaseDuration != 240*time.Second || q.InvocationsPerUser != 71 || q.InvocationsSystemKeep != 13 || q.ActivationsPerUser != 81 || q.RunsPerJobKeep != 0 || q.ChatRetention != 0 {
		t.Fatalf("changed policy = %+v", q)
	}
}

func TestCommandMaintenancePolicyRejectsDurationOverflow(t *testing.T) {
	for _, mutate := range []func(*config.MaintenanceSettings){
		func(s *config.MaintenanceSettings) { s.JobRetentionHours = math.MaxInt },
		func(s *config.MaintenanceSettings) { s.ChatToolCallsRetentionDays = math.MaxInt },
		func(s *config.MaintenanceSettings) { s.CommandInvocationRetentionDays = math.MaxInt },
		func(s *config.MaintenanceSettings) { s.CommandActivationTerminalRetentionDays = math.MaxInt },
		func(s *config.MaintenanceSettings) { s.CommandJobActivationLeaseSeconds = math.MaxInt },
		func(s *config.MaintenanceSettings) { s.CommandInvocationsSystemKeep = 0 },
	} {
		s := config.DefaultMaintenanceSettings()
		mutate(&s)
		if _, err := commandMaintenancePolicy(s); !errors.Is(err, commandmaintenance.ErrInvalid) {
			t.Fatalf("settings %+v: %v", s, err)
		}
	}
}
