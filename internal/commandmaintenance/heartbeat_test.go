package commandmaintenance

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type heartbeatFunc func(context.Context, Policy) (BatchResult, error)

func (f heartbeatFunc) Heartbeat(ctx context.Context, p Policy) (BatchResult, error) {
	return f(ctx, p)
}

func TestHeartbeatSharesCoordinatorPolicyAndBlocksRetentionWhilePending(t *testing.T) {
	var order []string
	p := validMaintenancePolicy(2)
	c, err := New(Ports{Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{}, Jobs: maintenanceRetention{"jobs", &order}, Tools: maintenanceTools{&order}, InvocationDB: maintenanceRetention{"inv", &order}, Activations: maintenanceRetention{"act", &order}, Compaction: maintenanceCompact{&order}})
	if err != nil {
		t.Fatal(err)
	}
	c.ports.Heartbeat = heartbeatFunc(func(ctx context.Context, got Policy) (BatchResult, error) {
		if got != p {
			t.Fatal("política divergente")
		}
		if _, err := c.Run(ctx, p); !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("owner reentrante=%v", err)
		}
		order = append(order, "heartbeat")
		return BatchResult{Processed: 2, More: true}, nil
	})
	r, err := c.Run(context.Background(), p)
	if err != nil || !r.MoreHeartbeat || r.HeartbeatProcessed != 2 || r.Compacted {
		t.Fatalf("report=%+v %v", r, err)
	}
	if !reflect.DeepEqual(order, []string{"heartbeat", "requeue", "drain"}) {
		t.Fatalf("ordem=%v", order)
	}
	for _, n := range []int{-1, 3} {
		c.ports.Heartbeat = heartbeatFunc(func(context.Context, Policy) (BatchResult, error) { return BatchResult{Processed: n}, nil })
		if _, err := c.Run(context.Background(), p); !errors.Is(err, ErrInvalidBatchResult) {
			t.Fatalf("contador=%d %v", n, err)
		}
	}
	failure := errors.New("parcial")
	c.ports.Heartbeat = heartbeatFunc(func(context.Context, Policy) (BatchResult, error) { return BatchResult{Processed: 1}, failure })
	r, err = c.Run(context.Background(), p)
	if !errors.Is(err, failure) || r.HeartbeatProcessed != 1 || !r.MoreHeartbeat || r.Compacted {
		t.Fatalf("erroparcial=%+v %v", r, err)
	}
}
