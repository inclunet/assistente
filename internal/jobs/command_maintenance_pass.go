package jobs

import (
	"context"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/config"
	"assistente/internal/logging"
)

// A mesma goroutine de retenção passa a cuidar do heartbeat quando o host monta
// o coordenador. Não há loop adicional nem fallback para limpeza legada em erro.
func (m *Manager) runCommandMaintenance(ctx context.Context) time.Duration {
	settings, err := config.GetMaintenance()
	if err != nil {
		logging.Errorf(ctx, "jobs.manager", "instance maintenance skipped: settings unavailable: %v", err)
		return time.Minute
	}
	policy, err := commandMaintenancePolicy(settings)
	if err != nil {
		logging.Errorf(ctx, "jobs.manager", "instance maintenance skipped: invalid complete policy: %v", err)
		return time.Minute
	}
	report, err := m.cfg.MaintenanceCoordinator.Run(ctx, policy)
	if err != nil {
		logging.Errorf(ctx, "jobs.manager", "instance maintenance failed: stage=%s heartbeat=%d outbox_requeued=%d outbox_purged=%d recovered=%d jobs_deleted=%d tools_deleted=%d invocations_deleted=%d activations_deleted=%d: %v", report.Stage, report.HeartbeatProcessed, report.OutboxRequeued, report.OutboxPurged, report.Recovered, report.JobsDeleted, report.ToolsDeleted, report.InvocationsDeleted, report.ActivationsDeleted, err)
	}
	return commandMaintenanceDelay(policy, report, err)
}

func commandMaintenanceDelay(policy commandmaintenance.Policy, report commandmaintenance.Report, runErr error) time.Duration {
	// Menos da metade do TTL, deixando margem para trabalho local. Se o lote
	// ultrapassar a capacidade/TTL, a lease expira e não é ressuscitada.
	delay := policy.LeaseDuration / 3
	if delay <= 0 {
		delay = time.Nanosecond
	}
	if delay > time.Minute {
		delay = time.Minute
	}
	if (runErr != nil || report.MoreHeartbeat || report.MoreOutbox || report.MoreRecovery || report.MoreRetention) && delay > time.Second {
		delay = time.Second
	}
	return delay
}
