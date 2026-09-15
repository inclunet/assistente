package commandmaintenance

import (
	"context"
	"errors"
	"testing"
)

var errPartialMaintenance = errors.New("falha depois de progresso parcial")

type partialOutbox struct {
	negative bool
}

func (p partialOutbox) RequeueExpiredLeases(context.Context, int) (int, bool, error) {
	if p.negative {
		return -1, false, errPartialMaintenance
	}
	return 1, false, nil
}

func (partialOutbox) Drain(context.Context, int) (BatchResult, error) {
	return BatchResult{Processed: 1, More: true}, errPartialMaintenance
}

type partialRetention struct {
	deleted int64
	err     error
	bounded bool
}

func (p partialRetention) Retain(context.Context, Policy) (int64, error) {
	return p.deleted, p.err
}

func (p partialRetention) RetainBatch(context.Context, Policy) (RetentionResult, error) {
	return RetentionResult{Deleted: p.deleted, More: p.bounded}, p.err
}

type partialTools struct {
	deleted  int64
	err      error
	negative bool
}

func (p partialTools) CleanOldDryRuns(context.Context, Policy) (int64, error) {
	if p.negative {
		return -2, p.err
	}
	return p.deleted, p.err
}

func (partialTools) CleanOrphanChat(context.Context, Policy) (int64, error) {
	return 0, nil
}

func (partialTools) CleanOldChat(context.Context, Policy) (int64, error) {
	return 0, nil
}

func newPartialReportCoordinator(t *testing.T, outbox OutboxPort, jobs RetentionPort, tools ToolRetentionPort) *Coordinator {
	t.Helper()
	order := new([]string)
	coordinator, err := New(Ports{
		Outbox:       outbox,
		Decisions:    maintenanceRecovery{},
		Invocations:  maintenanceRecovery{},
		Claims:       maintenanceRecovery{},
		Jobs:         jobs,
		Tools:        tools,
		InvocationDB: maintenanceRetention{name: "invocations", order: order},
		Activations:  maintenanceRetention{name: "activations", order: order},
		Compaction:   maintenanceCompact{order: order},
	})
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func TestCoordinatorPreservaOutboxParcialEmErro(t *testing.T) {
	coordinator := newPartialReportCoordinator(t, partialOutbox{}, maintenanceRetention{}, maintenanceTools{})
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if !errors.Is(err, errPartialMaintenance) {
		t.Fatalf("error = %v, want partial error", err)
	}
	if report.OutboxRequeued != 1 || !report.MoreOutbox || report.OutboxDrained || report.Compacted {
		t.Fatalf("partial outbox report = %+v", report)
	}
}

func TestCoordinatorNaoPublicaContadorNegativoOutbox(t *testing.T) {
	coordinator := newPartialReportCoordinator(t, partialOutbox{negative: true}, maintenanceRetention{}, maintenanceTools{})
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if !errors.Is(err, ErrInvalidBatchResult) {
		t.Fatalf("error = %v, want invalid batch", err)
	}
	if report.OutboxRequeued != 0 || report.MoreOutbox {
		t.Fatalf("negative outbox report = %+v", report)
	}
}

func TestCoordinatorPreservaJobsParciaisEmErro(t *testing.T) {
	coordinator := newPartialReportCoordinator(t, &maintenanceOutbox{order: new([]string)}, partialRetention{deleted: 3, err: errPartialMaintenance}, maintenanceTools{order: new([]string)})
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if !errors.Is(err, errPartialMaintenance) {
		t.Fatalf("error = %v, want partial error", err)
	}
	if report.JobsDeleted != 3 || !report.MoreRetention || report.Compacted {
		t.Fatalf("partial jobs report = %+v", report)
	}
}

func TestCoordinatorPreservaToolsParciaisEmErro(t *testing.T) {
	coordinator := newPartialReportCoordinator(t, &maintenanceOutbox{order: new([]string)}, partialRetention{}, partialTools{deleted: 2, err: errPartialMaintenance})
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if !errors.Is(err, errPartialMaintenance) {
		t.Fatalf("error = %v, want partial error", err)
	}
	if report.ToolsDeleted != 2 || !report.MoreRetention || report.Compacted {
		t.Fatalf("partial tools report = %+v", report)
	}
}

func TestCoordinatorNaoPublicaContadorNegativoTools(t *testing.T) {
	coordinator := newPartialReportCoordinator(t, &maintenanceOutbox{order: new([]string)}, partialRetention{}, partialTools{negative: true, err: errPartialMaintenance})
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if !errors.Is(err, ErrInvalidRetentionResult) {
		t.Fatalf("error = %v, want invalid retention", err)
	}
	if report.ToolsDeleted != 0 || report.Compacted {
		t.Fatalf("negative tools report = %+v", report)
	}
}
