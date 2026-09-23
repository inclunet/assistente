package commandmaintenance

import (
	"context"
	"testing"
)

type policySnapshotProbe struct {
	maintenanceOutbox
	t    *testing.T
	want Policy
	seen []Policy
}

func (p *policySnapshotProbe) Drain(ctx context.Context, _ int) (BatchResult, error) {
	got, ok := PolicyFromContext(ctx)
	if !ok || got != p.want {
		p.t.Fatalf("snapshot=%+v presente=%v esperado=%+v", got, ok, p.want)
	}
	p.seen = append(p.seen, got)
	got.LeaseDuration = 0
	again, _ := PolicyFromContext(ctx)
	if again != p.want {
		p.t.Fatal("alterar cópia modificou snapshot")
	}
	return BatchResult{}, nil
}

func TestCoordinatorPublishesValidatedIndependentPolicySnapshots(t *testing.T) {
	ctx := context.Background()
	order := []string{}
	probe := &policySnapshotProbe{maintenanceOutbox: maintenanceOutbox{order: &order}, t: t}
	coordinator, err := New(Ports{Outbox: probe, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{}, Jobs: maintenanceRetention{order: &order}, Tools: maintenanceTools{order: &order}, InvocationDB: maintenanceRetention{order: &order}, Activations: maintenanceRetention{order: &order}, Compaction: maintenanceCompact{order: &order}})
	if err != nil {
		t.Fatal(err)
	}
	first := validMaintenancePolicy(2)
	second := first
	second.LeaseDuration *= 2
	second.ActivationRetention *= 2
	for _, policy := range []Policy{first, second} {
		probe.want = policy
		if report, err := coordinator.Run(ctx, policy); err != nil || report.Stage != "complete" {
			t.Fatalf("report=%+v err=%v", report, err)
		}
	}
	invalid := second
	invalid.LeaseDuration = 0
	if _, err := coordinator.Run(ctx, invalid); err == nil {
		t.Fatal("política inválida aceita")
	}
	if len(probe.seen) != 2 || probe.seen[0] != first || probe.seen[1] != second {
		t.Fatal("snapshot vazou entre passagens")
	}
	if _, ok := PolicyFromContext(ctx); ok {
		t.Fatal("contexto do chamador alterado")
	}
	if _, ok := PolicyFromContext(nil); ok {
		t.Fatal("nil contém política")
	}
}
