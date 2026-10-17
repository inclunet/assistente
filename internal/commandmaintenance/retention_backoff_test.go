package commandmaintenance

import (
	"context"
	"errors"
	"testing"
	"time"
)

type backoffRetention struct {
	calls int
	err   error
}

type backoffRecovery struct{ calls int }

func (r *backoffRecovery) Recover(context.Context, int) (BatchResult, error) {
	r.calls++
	return BatchResult{}, nil
}

type backoffCompaction struct{ err error }

func (c backoffCompaction) Compact(context.Context, int64) error { return c.err }

func (r *backoffRetention) Retain(context.Context, Policy) (int64, error) {
	r.calls++
	return 0, r.err
}

func TestRetentionBackoffPreservesCriticalMaintenanceAndResumes(t *testing.T) {
	var order []string
	jobs := &backoffRetention{err: errors.New("database busy")}
	heartbeats := 0
	recovery := &backoffRecovery{}
	c, err := New(Ports{
		Heartbeat: heartbeatFunc(func(context.Context, Policy) (BatchResult, error) { heartbeats++; return BatchResult{}, nil }),
		Outbox:    &maintenanceOutbox{order: &order},
		Decisions: recovery, Invocations: recovery, Claims: recovery,
		Jobs: jobs, Tools: maintenanceTools{order: &order},
		InvocationDB: maintenanceRetention{name: "invocations", order: &order},
		Activations:  maintenanceRetention{name: "activations", order: &order},
		Compaction:   maintenanceCompact{order: &order},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	c.now = func() time.Time { return now }
	p := validMaintenancePolicy(2)
	for _, delay := range []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		r, runErr := c.Run(context.Background(), p)
		if !errors.Is(runErr, jobs.err) || r.RetentionRetryAfter != delay {
			t.Fatalf("report=%+v err=%v want=%v", r, runErr, delay)
		}
		calls, beats, recovered := jobs.calls, heartbeats, recovery.calls
		now = now.Add(delay - time.Nanosecond)
		order = nil
		r, runErr = c.Run(context.Background(), p)
		if runErr != nil || r.Stage != "retention.deferred" || r.Compacted || jobs.calls != calls || heartbeats != beats+1 || recovery.calls != recovered+3 || len(order) != 2 || order[0] != "requeue" || order[1] != "drain" {
			t.Fatalf("deferred report=%+v err=%v jobs=%d heartbeat=%d order=%v", r, runErr, jobs.calls, heartbeats, order)
		}
		now = now.Add(time.Nanosecond)
	}
	jobs.err = nil
	r, err := c.Run(context.Background(), p)
	if err != nil || !r.Compacted || c.retentionBackoff != 0 {
		t.Fatalf("recovery=%+v err=%v", r, err)
	}
	jobs.err = errors.New("another failure")
	r, err = c.Run(context.Background(), p)
	if err == nil || r.RetentionRetryAfter != 30*time.Second {
		t.Fatalf("reset=%+v err=%v", r, err)
	}
}

func TestRetentionBackoffAlsoDefersAfterLateFailure(t *testing.T) {
	for _, stage := range []string{"retention.invocations", "retention.activations", "compaction"} {
		t.Run(stage, func(t *testing.T) {
			var order []string
			failure := errors.New("busy")
			ports := Ports{
				Outbox: &maintenanceOutbox{order: &order}, Decisions: maintenanceRecovery{}, Invocations: maintenanceRecovery{}, Claims: maintenanceRecovery{},
				Jobs: maintenanceRetention{name: "jobs", order: &order}, Tools: maintenanceTools{order: &order},
				InvocationDB: maintenanceRetention{name: "invocations", order: &order}, Activations: maintenanceRetention{name: "activations", order: &order},
				Compaction: maintenanceCompact{order: &order},
			}
			switch stage {
			case "retention.invocations":
				ports.InvocationDB = &backoffRetention{err: failure}
			case "retention.activations":
				ports.Activations = &backoffRetention{err: failure}
			case "compaction":
				ports.Compaction = backoffCompaction{err: failure}
			}
			c, err := New(ports)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			c.now = func() time.Time { return now }
			r, err := c.Run(context.Background(), validMaintenancePolicy(2))
			if !errors.Is(err, failure) || r.Stage != stage || r.JobsDeleted != 1 || r.RetentionRetryAfter != 30*time.Second {
				t.Fatalf("report=%+v err=%v", r, err)
			}
			order = nil
			r, err = c.Run(context.Background(), validMaintenancePolicy(2))
			if err != nil || r.Stage != "retention.deferred" || len(order) != 2 {
				t.Fatalf("report=%+v err=%v order=%v", r, err, order)
			}
		})
	}
}
