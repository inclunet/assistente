package jobs

import (
	"testing"
	"time"
)

func TestCircuitBreaker_RateLimitAllowsUnderThreshold(t *testing.T) {
	cb := NewCircuitBreaker()

	for i := 0; i < 5; i++ {
		if err := cb.CheckRateLimit("job-1", 10); err != nil {
			t.Fatalf("should allow run %d: %v", i, err)
		}
		cb.RecordRun("job-1")
	}
}

func TestCircuitBreaker_RateLimitBlocksOverThreshold(t *testing.T) {
	cb := NewCircuitBreaker()

	for i := 0; i < 3; i++ {
		cb.RecordRun("job-1")
	}

	err := cb.CheckRateLimit("job-1", 3)
	if err == nil {
		t.Error("expected rate limit error after 3 runs with limit 3")
	}
}

func TestCircuitBreaker_RateLimitUsesJobOverride(t *testing.T) {
	cb := NewCircuitBreaker()

	cb.RecordRun("job-1")
	cb.RecordRun("job-1")

	if err := cb.CheckRateLimit("job-1", 5); err != nil {
		t.Errorf("should allow with override limit 5: %v", err)
	}

	err := cb.CheckRateLimit("job-1", 2)
	if err == nil {
		t.Error("should block with override limit 2")
	}
}

func TestCircuitBreaker_RateLimitDefaultGlobal(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.SetMaxRunsPerHour(2)

	cb.RecordRun("job-1")
	cb.RecordRun("job-1")

	err := cb.CheckRateLimit("job-1", 0)
	if err == nil {
		t.Error("expected rate limit using global default of 2")
	}
}

func TestCircuitBreaker_DetectLoop(t *testing.T) {
	cb := NewCircuitBreaker()

	if err := cb.DetectLoop([]string{"A", "B"}, "C"); err != nil {
		t.Errorf("no loop expected: %v", err)
	}

	if err := cb.DetectLoop([]string{"A", "B", "C"}, "A"); err == nil {
		t.Error("expected loop detection: A -> B -> C -> A")
	}
}

func TestCircuitBreaker_ChainDepth(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.SetMaxChainDepth(3)

	if got := cb.MaxChainDepth(); got != 3 {
		t.Errorf("expected max depth 3, got %d", got)
	}
}

func TestCircuitBreaker_PruneOldRuns(t *testing.T) {
	cb := NewCircuitBreaker()

	cb.mu.Lock()
	cb.runCounts["job-1"] = []time.Time{
		time.Now().Add(-2 * time.Hour),
		time.Now().Add(-90 * time.Minute),
		time.Now().Add(-30 * time.Minute),
		time.Now().Add(-5 * time.Minute),
	}
	cb.mu.Unlock()

	if err := cb.CheckRateLimit("job-1", 3); err != nil {
		t.Errorf("after pruning old runs, should have 2 recent: %v", err)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker()

	for i := 0; i < 5; i++ {
		cb.RecordRun("job-1")
	}

	cb.Reset()

	if err := cb.CheckRateLimit("job-1", 1); err != nil {
		t.Errorf("after reset, should have 0 runs: %v", err)
	}
}

func TestCircuitBreaker_IsolatedPerJob(t *testing.T) {
	cb := NewCircuitBreaker()

	for i := 0; i < 3; i++ {
		cb.RecordRun("job-1")
	}

	if err := cb.CheckRateLimit("job-2", 3); err != nil {
		t.Errorf("job-2 should not be affected by job-1 runs: %v", err)
	}
}
