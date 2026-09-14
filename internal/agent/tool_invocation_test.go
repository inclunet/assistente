package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

type agentTestLedger struct {
	mu     sync.Mutex
	nextID int
	rows   map[string]toolinvocations.Invocation
}

func newAgentTestToolInvocations(registry *tools.Registry) *toolinvocations.Service {
	return toolinvocations.NewService(
		&agentTestLedger{rows: make(map[string]toolinvocations.Invocation)},
		tools.NewExecutor(registry, tools.DefaultExecutorConfig()),
	)
}

func (r *agentTestLedger) Create(_ context.Context, inv *toolinvocations.Invocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	inv.ID = fmt.Sprintf("agent-test-invocation-%d", r.nextID)
	inv.Attempt = 1
	r.rows[inv.ID] = *inv
	return nil
}

func (r *agentTestLedger) MarkRunning(_ context.Context, id string, startedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv := r.rows[id]
	inv.Status = toolinvocations.StatusRunning
	inv.StartedAt = &startedAt
	r.rows[id] = inv
	return nil
}

func (r *agentTestLedger) Complete(_ context.Context, id string, inv *toolinvocations.Invocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[id] = *inv
	return nil
}

func (r *agentTestLedger) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}

func (r *agentTestLedger) Get(_ context.Context, id string) (*toolinvocations.Invocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.rows[id]
	if !ok {
		return nil, fmt.Errorf("invocation not found")
	}
	return &inv, nil
}

func (r *agentTestLedger) List(context.Context, toolinvocations.Filter) ([]toolinvocations.Invocation, error) {
	return nil, nil
}
func (r *agentTestLedger) CleanOldDryRuns(context.Context, time.Duration) (int, error) {
	return 0, nil
}
func (r *agentTestLedger) CleanOldChat(context.Context, time.Duration) (int, error) {
	return 0, nil
}
func (r *agentTestLedger) CleanOrphanChat(context.Context) (int, error) {
	return 0, nil
}
func (r *agentTestLedger) ValidateChatOrigin(context.Context, string) error { return nil }
func (r *agentTestLedger) ResolveToolCatalogID(context.Context, string) (string, error) {
	return "agent-test-catalog", nil
}
func (r *agentTestLedger) IsToolCatalogIDVisible(context.Context, string) (bool, error) {
	return true, nil
}
