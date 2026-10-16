package commandmaintenance

import (
	"context"
	"reflect"
	"testing"
)

type boundedToolsForTest struct {
	order       *[]string
	legacyCalls int
}

func (p *boundedToolsForTest) legacy(name string) (int64, error) {
	p.legacyCalls++
	*p.order = append(*p.order, name+"-legacy")
	return 0, nil
}

func (p *boundedToolsForTest) CleanOldDryRuns(context.Context, Policy) (int64, error) {
	return p.legacy("dry-runs")
}

func (p *boundedToolsForTest) CleanOrphanChat(context.Context, Policy) (int64, error) {
	return p.legacy("orphan-chat")
}

func (p *boundedToolsForTest) CleanOldChat(context.Context, Policy) (int64, error) {
	return p.legacy("old-chat")
}

func (p *boundedToolsForTest) CleanOldDryRunsBatch(context.Context, Policy) (RetentionResult, error) {
	*p.order = append(*p.order, "dry-runs-batch")
	return RetentionResult{Deleted: 1, More: true}, nil
}

func (p *boundedToolsForTest) CleanOrphanChatBatch(context.Context, Policy) (RetentionResult, error) {
	*p.order = append(*p.order, "orphan-chat-batch")
	return RetentionResult{Deleted: 1}, nil
}

func (p *boundedToolsForTest) CleanOldChatBatch(context.Context, Policy) (RetentionResult, error) {
	*p.order = append(*p.order, "old-chat-batch")
	return RetentionResult{Deleted: 1}, nil
}

func TestCoordinatorUsesBoundedToolRetentionAndBlocksCompaction(t *testing.T) {
	var order []string
	tools := &boundedToolsForTest{order: &order}
	coordinator, err := New(Ports{
		Outbox:       &maintenanceOutbox{order: &order},
		Decisions:    maintenanceRecovery{},
		Invocations:  maintenanceRecovery{},
		Claims:       maintenanceRecovery{},
		Jobs:         maintenanceRetention{name: "jobs", order: &order},
		Tools:        tools,
		InvocationDB: maintenanceRetention{name: "invocations", order: &order},
		Activations:  maintenanceRetention{name: "activations", order: &order},
		Compaction:   maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := coordinator.Run(context.Background(), validMaintenancePolicy(2))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"requeue", "drain", "jobs", "dry-runs-batch", "orphan-chat-batch", "old-chat-batch", "invocations", "activations"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("bounded tool order=%v, want %v", order, want)
	}
	if tools.legacyCalls != 0 {
		t.Fatalf("legacy tool methods called %d times", tools.legacyCalls)
	}
	if report.ToolsDeleted != 3 || !report.MoreRetention || report.Compacted {
		t.Fatalf("bounded tool report=%+v", report)
	}
}
