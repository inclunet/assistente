package jobs

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

type jobExecutorTestLedger struct {
	mu          sync.Mutex
	invocations map[string]toolinvocations.Invocation
	nextID      int
	completeErr error
}

func newJobExecutorTestLedger() *jobExecutorTestLedger {
	return &jobExecutorTestLedger{invocations: make(map[string]toolinvocations.Invocation)}
}

func (r *jobExecutorTestLedger) Create(_ context.Context, inv *toolinvocations.Invocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	inv.ID = fmt.Sprintf("test-invocation-%d", r.nextID)
	inv.Attempt = 1
	r.invocations[inv.ID] = *inv
	return nil
}

func (r *jobExecutorTestLedger) MarkRunning(_ context.Context, id string, startedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv := r.invocations[id]
	inv.StartedAt = &startedAt
	inv.Status = toolinvocations.StatusRunning
	r.invocations[id] = inv
	return nil
}

func (r *jobExecutorTestLedger) Complete(_ context.Context, id string, inv *toolinvocations.Invocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completeErr != nil {
		return r.completeErr
	}
	r.invocations[id] = *inv
	return nil
}

func (r *jobExecutorTestLedger) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.invocations, id)
	return nil
}

func (r *jobExecutorTestLedger) Get(_ context.Context, id string) (*toolinvocations.Invocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.invocations[id]
	if !ok {
		return nil, fmt.Errorf("invocation not found")
	}
	return &inv, nil
}

func (r *jobExecutorTestLedger) List(context.Context, toolinvocations.Filter) ([]toolinvocations.Invocation, error) {
	return nil, nil
}

func (r *jobExecutorTestLedger) CleanOldDryRuns(context.Context, time.Duration) (int, error) {
	return 0, nil
}

func (r *jobExecutorTestLedger) CleanOldChat(context.Context, time.Duration) (int, error) {
	return 0, nil
}

func (r *jobExecutorTestLedger) CleanOrphanChat(context.Context) (int, error) {
	return 0, nil
}

func (r *jobExecutorTestLedger) ValidateChatOrigin(context.Context, string) error {
	return nil
}

func (r *jobExecutorTestLedger) ResolveToolCatalogID(context.Context, string) (string, error) {
	return "test-catalog", nil
}

func (r *jobExecutorTestLedger) IsToolCatalogIDVisible(context.Context, string) (bool, error) {
	return true, nil
}

func mustNewJobExecutor(t testing.TB, cfg ExecutorConfig) *JobExecutor {
	t.Helper()
	if cfg.ToolRegistry == nil {
		cfg.ToolRegistry = tools.NewRegistry()
	}
	if cfg.ToolInvocations == nil {
		cfg.ToolInvocations = toolinvocations.NewService(
			newJobExecutorTestLedger(),
			tools.NewExecutor(cfg.ToolRegistry, tools.DefaultExecutorConfig()),
		)
	}
	executor, err := NewJobExecutor(cfg)
	if err != nil {
		t.Fatalf("NewJobExecutor: %v", err)
	}
	return executor
}

func mustNewManager(t testing.TB, cfg ManagerConfig) *Manager {
	t.Helper()
	if cfg.ToolRegistry == nil {
		cfg.ToolRegistry = tools.NewRegistry()
	}
	if cfg.ToolInvocations == nil {
		cfg.ToolInvocations = toolinvocations.NewService(
			newJobExecutorTestLedger(),
			tools.NewExecutor(cfg.ToolRegistry, tools.DefaultExecutorConfig()),
		)
	}
	return NewManager(cfg)
}
