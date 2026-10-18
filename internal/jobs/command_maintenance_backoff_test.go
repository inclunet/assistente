package jobs

import (
	"assistente/internal/commandmaintenance"
	"errors"
	"testing"
	"time"
)

func TestMaintenanceDelayDoesNotAccelerateDeferredRetention(t *testing.T) {
	p := commandMaintenanceTestPolicy()
	p.LeaseDuration = 30 * time.Second
	r := commandmaintenance.Report{MoreRetention: true, RetentionRetryAfter: time.Minute}
	for _, err := range []error{nil, errors.New("busy")} {
		if got := commandMaintenanceDelay(p, r, err); got != 10*time.Second {
			t.Fatalf("delay=%v err=%v", got, err)
		}
	}
	r.RetentionRetryAfter = 2 * time.Second
	if got := commandMaintenanceDelay(p, r, nil); got != 2*time.Second {
		t.Fatalf("deadline=%v", got)
	}
	r.MoreHeartbeat = true
	if got := commandMaintenanceDelay(p, r, nil); got != time.Second {
		t.Fatalf("heartbeat=%v", got)
	}
}
