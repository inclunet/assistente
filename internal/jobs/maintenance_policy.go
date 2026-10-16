package jobs

import (
	"math"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/config"
)

// commandMaintenancePolicy converte uma fotografia da única fonte de settings.
// A cadência relê config.json; nenhum ponteiro de bootstrap congela a política.
// Rejeitar overflow antes da multiplicação evita transformar retenção longa em
// uma janela curta positiva e apagar auditoria prematuramente.
func commandMaintenancePolicy(s config.MaintenanceSettings) (commandmaintenance.Policy, error) {
	p := commandmaintenance.Policy{
		RunsPerJobKeep:        s.RunsPerJobKeep,
		VacuumMinFreeBytes:    s.VacuumMinFreeBytes,
		InvocationsPerUser:    s.CommandInvocationsPerUserKeep,
		InvocationsSystemKeep: s.CommandInvocationsSystemKeep,
		ActivationsPerUser:    s.CommandActivationTerminalKeepPerUser,
	}
	for _, field := range []struct {
		value  int
		unit   time.Duration
		target *time.Duration
	}{
		{s.JobRetentionHours, time.Hour, &p.JobRetention},
		{s.ChatToolCallsRetentionDays, 24 * time.Hour, &p.ChatRetention},
		{s.CommandInvocationRetentionDays, 24 * time.Hour, &p.InvocationRetention},
		{s.CommandActivationTerminalRetentionDays, 24 * time.Hour, &p.ActivationRetention},
		{s.CommandJobActivationLeaseSeconds, time.Second, &p.LeaseDuration},
	} {
		if field.value < 0 || int64(field.value) > math.MaxInt64/int64(field.unit) {
			return commandmaintenance.Policy{}, commandmaintenance.ErrInvalid
		}
		*field.target = time.Duration(field.value) * field.unit
	}
	if err := p.Validate(); err != nil {
		return commandmaintenance.Policy{}, err
	}
	return p, nil
}
