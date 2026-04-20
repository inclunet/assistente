package jobs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_IntervalFiresMultipleTimes(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		count.Add(1)
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "test-interval",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "50ms"},
		},
	}

	if err := s.Schedule(job); err != nil {
		t.Fatal(err)
	}

	time.Sleep(180 * time.Millisecond)
	s.Unschedule("test-interval")

	got := int(count.Load())
	if got < 2 {
		t.Errorf("expected at least 2 firings in 180ms with 50ms interval, got %d", got)
	}
}

func TestScheduler_IntervalStopsOnUnschedule(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		count.Add(1)
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "test-unsched",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "30ms"},
		},
	}

	if err := s.Schedule(job); err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond)
	s.Unschedule("test-unsched")
	afterUnsched := int(count.Load())

	time.Sleep(100 * time.Millisecond)
	afterWait := int(count.Load())

	if afterWait != afterUnsched {
		t.Errorf("expected no more firings after unschedule, got %d then %d", afterUnsched, afterWait)
	}
}

func TestScheduler_DisabledJobNotScheduled(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		count.Add(1)
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "disabled-job",
		Enabled: false,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "20ms"},
		},
	}

	if err := s.Schedule(job); err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("disabled job should not fire, got %d", got)
	}
}

func TestScheduler_RescheduleReplacesTimer(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		count.Add(1)
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "resched-job",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "5s"},
		},
	}

	if err := s.Schedule(job); err != nil {
		t.Fatal(err)
	}

	job.Triggers = []Trigger{
		{Type: TriggerInterval, Every: "30ms"},
	}
	if err := s.Reschedule(job); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if got := int(count.Load()); got < 2 {
		t.Errorf("expected at least 2 firings after reschedule to 30ms, got %d", got)
	}
}

func TestScheduler_ScheduledJobsReturnsIDs(t *testing.T) {
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {})
	s.Start()
	defer s.Stop()

	job1 := &Job{
		ID:      "job-a",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "1h"},
		},
	}
	job2 := &Job{
		ID:      "job-b",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "1h"},
		},
	}

	_ = s.Schedule(job1)
	_ = s.Schedule(job2)

	ids := s.ScheduledJobs()
	found := make(map[string]bool)
	for _, id := range ids {
		found[id] = true
	}

	if !found["job-a"] || !found["job-b"] {
		t.Errorf("expected both jobs, got %v", ids)
	}
}

func TestScheduler_StopCancelsAllIntervals(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		count.Add(1)
	})
	s.Start()

	job := &Job{
		ID:      "stop-test",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "50ms"},
		},
	}
	_ = s.Schedule(job)

	time.Sleep(150 * time.Millisecond)
	s.Stop()
	time.Sleep(20 * time.Millisecond) // drain in-flight callback
	afterStop := int(count.Load())

	time.Sleep(200 * time.Millisecond)
	afterWait := int(count.Load())

	if afterWait != afterStop {
		t.Errorf("expected no firings after Stop(), got %d then %d", afterStop, afterWait)
	}
}

func TestScheduler_IntervalSurvivesPanic(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		n := count.Add(1)
		if n == 1 {
			panic("simulated crash")
		}
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "panic-job",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "30ms"},
		},
	}

	if err := s.Schedule(job); err != nil {
		t.Fatal(err)
	}

	time.Sleep(150 * time.Millisecond)

	got := int(count.Load())
	if got < 3 {
		t.Errorf("expected at least 3 firings (1 panic + 2 ok) in 150ms @ 30ms, got %d", got)
	}
}

func TestScheduler_CronFiresOnSchedule(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		count.Add(1)
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "cron-job",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerCron, Expression: "* * * * *"},
		},
	}

	if err := s.Schedule(job); err != nil {
		t.Fatal(err)
	}

	ids := s.ScheduledJobs()
	found := false
	for _, id := range ids {
		if id == "cron-job" {
			found = true
		}
	}
	if !found {
		t.Error("cron-job should appear in ScheduledJobs")
	}
}

func TestScheduler_InvalidIntervalReturnsError(t *testing.T) {
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {})

	job := &Job{
		ID:      "bad-interval",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "not-a-duration"},
		},
	}

	err := s.Schedule(job)
	if err == nil {
		t.Error("expected error for invalid interval")
	}
}

func TestScheduler_InvalidCronReturnsError(t *testing.T) {
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {})

	job := &Job{
		ID:      "bad-cron",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerCron, Expression: "not-a-cron"},
		},
	}

	err := s.Schedule(job)
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestScheduler_IntervalTriggerContext(t *testing.T) {
	var receivedType TriggerType
	var mu sync.Mutex
	done := make(chan struct{})

	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {
		mu.Lock()
		receivedType = trigCtx.Type
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
	})
	s.Start()
	defer s.Stop()

	job := &Job{
		ID:      "ctx-test",
		Enabled: true,
		Triggers: []Trigger{
			{Type: TriggerInterval, Every: "20ms"},
		},
	}
	_ = s.Schedule(job)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for interval trigger")
	}

	mu.Lock()
	if receivedType != TriggerInterval {
		t.Errorf("expected TriggerInterval, got %v", receivedType)
	}
	mu.Unlock()
}

func TestScheduler_DoubleStartIsIdempotent(t *testing.T) {
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {})
	s.Start()
	s.Start()
	s.Stop()
}

func TestScheduler_DoubleStopIsIdempotent(t *testing.T) {
	s := NewScheduler(func(ctx context.Context, job *Job, trigCtx *TriggerContext) {})
	s.Start()
	s.Stop()
	s.Stop()
}
