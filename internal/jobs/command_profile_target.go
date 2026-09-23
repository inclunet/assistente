package jobs

import (
	"context"
	"strings"
	"time"

	"assistente/internal/jobprofilegrant"
)

// CommandProfileTarget is a preparation snapshot, not an authorization. The
// App must bind the exact slug to a grant for Expression and its generation.
type CommandProfileTarget struct {
	Slug                  string
	Expression            string
	DefinitionFingerprint string
}

// PrepareCommandProfileTarget uses the authoritative job and the same template
// resolver as execution. Direct command/hotkey ingress has no external event
// payload. Only profile is resolved here; other inputs are left to the runtime.
func (m *Manager) PrepareCommandProfileTarget(ctx context.Context, databaseID string) (CommandProfileTarget, error) {
	job, err := m.PrepareCommandJob(ctx, databaseID)
	if err != nil {
		return CommandProfileTarget{}, err
	}
	fingerprint, err := DefinitionFingerprint(job)
	if err != nil {
		return CommandProfileTarget{}, err
	}
	result := CommandProfileTarget{DefinitionFingerprint: fingerprint}
	if job.Tool != jobprofilegrant.ToolSubagent {
		return result, nil
	}
	raw, present := job.Inputs["profile"]
	if !present {
		return result, nil
	}
	expression, valid := raw.(string)
	if !valid || strings.ContainsRune(expression, '\x00') {
		return CommandProfileTarget{}, ErrCommandJobDenied
	}
	result.Expression = strings.TrimSpace(expression)
	if result.Expression == "" {
		return result, nil
	}
	resolved, err := ResolveInputs(map[string]any{"profile": expression}, &TemplateContext{
		Event: map[string]any{}, Now: time.Now(),
		Secrets: func(key string) (string, error) { return m.resolveSecret(ctx, key) },
	})
	// Template failures may contain secrets; do not propagate their payload.
	if err != nil || ctx.Err() != nil {
		return CommandProfileTarget{}, ErrCommandJobDenied
	}
	slug, valid := resolved["profile"].(string)
	if !valid || isEmptyTemplateValue(slug) || strings.ContainsRune(slug, '\x00') {
		return CommandProfileTarget{}, ErrCommandJobDenied
	}
	result.Slug = strings.TrimSpace(slug)
	return result, nil
}
