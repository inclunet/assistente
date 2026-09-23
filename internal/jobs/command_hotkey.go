package jobs

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"assistente/internal/database"
)

var errPreparedHotkeyDenied = errors.New("prepared hotkey binding denied")

// preparedHotkeyBinding is private on purpose: it is only a claim captured
// while registering the legacy OS adapter. It is not a command catalog or a
// second execution path.
type preparedHotkeyBinding struct {
	ownerUserID           string
	jobDatabaseID         string
	jobSlug               string
	keys                  string
	when                  string
	definitionFingerprint string
	bindingFingerprint    string
}

type preparedHotkeyDispatchKey struct{}

type hotkeyRegistrationLifetime struct {
	ctx    context.Context
	cancel context.CancelFunc
	active atomic.Bool
}

func withPreparedHotkeyDispatch(ctx context.Context, binding preparedHotkeyBinding) context.Context {
	return context.WithValue(ctx, preparedHotkeyDispatchKey{}, binding)
}

func (m *Manager) prepareHotkeyBinding(ctx context.Context, job *Job, keys, when string) (preparedHotkeyBinding, error) {
	if m == nil || job == nil || ctx == nil || ctx.Err() != nil || m.cfg.Repository == nil {
		return preparedHotkeyBinding{}, errPreparedHotkeyDenied
	}
	owner, err := database.RequireUserID(ctx)
	if err != nil || owner == "" || !m.hotkeyOwnerMatches(ctx, owner) {
		return preparedHotkeyBinding{}, errPreparedHotkeyDenied
	}
	keys = strings.TrimSpace(keys)
	when = strings.TrimSpace(when)
	if !validJobDefinitionDatabaseID(job.DatabaseID) || keys == "" {
		return preparedHotkeyBinding{}, errPreparedHotkeyDenied
	}

	current, err := m.PrepareCommandJob(ctx, job.DatabaseID)
	if err != nil || current == nil || current.ID != job.ID || !m.effectiveJobEnabled(current) {
		return preparedHotkeyBinding{}, errPreparedHotkeyDenied
	}
	fingerprint, err := DefinitionFingerprint(current)
	if err != nil || !hasHotkeyTrigger(current, keys, when) {
		return preparedHotkeyBinding{}, errPreparedHotkeyDenied
	}
	bindingFingerprint, err := commandHotkeyBindingFingerprint(fingerprint, keys, when)
	if err != nil {
		return preparedHotkeyBinding{}, errPreparedHotkeyDenied
	}
	return preparedHotkeyBinding{
		ownerUserID:           owner,
		jobDatabaseID:         current.DatabaseID,
		jobSlug:               current.ID,
		keys:                  keys,
		when:                  when,
		definitionFingerprint: fingerprint,
		bindingFingerprint:    bindingFingerprint,
	}, nil
}

func (m *Manager) revalidateHotkeyBinding(ctx context.Context, binding preparedHotkeyBinding) (*Job, error) {
	if ctx == nil || ctx.Err() != nil || binding.ownerUserID == "" ||
		!validJobDefinitionDatabaseID(binding.jobDatabaseID) ||
		strings.TrimSpace(binding.jobDatabaseID) != binding.jobDatabaseID ||
		binding.jobSlug == "" || strings.TrimSpace(binding.jobSlug) != binding.jobSlug ||
		binding.keys == "" || binding.keys != strings.TrimSpace(binding.keys) ||
		binding.when != strings.TrimSpace(binding.when) ||
		binding.definitionFingerprint == "" || binding.bindingFingerprint == "" ||
		!m.hotkeyOwnerMatches(ctx, binding.ownerUserID) {
		return nil, errPreparedHotkeyDenied
	}

	current, err := m.PrepareCommandJob(ctx, binding.jobDatabaseID)
	if err != nil || current == nil || !m.effectiveJobEnabled(current) || current.ID != binding.jobSlug {
		return nil, errPreparedHotkeyDenied
	}
	fingerprint, err := DefinitionFingerprint(current)
	if err != nil || fingerprint != binding.definitionFingerprint || !hasHotkeyTrigger(current, binding.keys, binding.when) {
		return nil, errPreparedHotkeyDenied
	}
	bindingFingerprint, err := commandHotkeyBindingFingerprint(fingerprint, binding.keys, binding.when)
	if err != nil || bindingFingerprint != binding.bindingFingerprint {
		return nil, errPreparedHotkeyDenied
	}
	return current, nil
}

func (m *Manager) hotkeyOwnerMatches(ctx context.Context, owner string) bool {
	if m == nil || ctx == nil || ctx.Err() != nil {
		return false
	}
	ctxOwner, ctxOK := database.UserIDFromContext(ctx)
	managerCtx := m.context()
	managerOwner, ok := database.UserIDFromContext(managerCtx)
	return ctxOK && ctxOwner == owner && managerCtx != nil && managerCtx.Err() == nil && ok && managerOwner == owner
}

func hasHotkeyTrigger(job *Job, keys, when string) bool {
	if job == nil {
		return false
	}
	for _, trigger := range job.Triggers {
		if trigger.Type == TriggerHotkey && strings.TrimSpace(trigger.Keys) == keys && strings.TrimSpace(trigger.When) == when {
			return true
		}
	}
	return false
}

func (m *Manager) preparedHotkeyStillCurrent(ctx context.Context, binding preparedHotkeyBinding, registryJob *Job) bool {
	current, err := m.revalidateHotkeyBinding(ctx, binding)
	if err != nil || current == nil || registryJob == nil || registryJob.DatabaseID != current.DatabaseID || registryJob.ID != current.ID {
		return false
	}
	registryFingerprint, err := DefinitionFingerprint(registryJob)
	return err == nil && registryFingerprint == binding.definitionFingerprint
}

func (m *Manager) addHotkeyLifetime(jobID string, id int, lifetime *hotkeyRegistrationLifetime) {
	m.hotkeyLifetimeMu.Lock()
	defer m.hotkeyLifetimeMu.Unlock()
	if m.hotkeyLifetimes == nil {
		m.hotkeyLifetimes = make(map[string]map[int]*hotkeyRegistrationLifetime)
	}
	byID := m.hotkeyLifetimes[jobID]
	if byID == nil {
		byID = make(map[int]*hotkeyRegistrationLifetime)
		m.hotkeyLifetimes[jobID] = byID
	}
	lifetime.active.Store(true)
	byID[id] = lifetime
}

func (m *Manager) invalidateHotkeyLifetime(jobID string, id int) {
	m.hotkeyLifetimeMu.Lock()
	byID := m.hotkeyLifetimes[jobID]
	lifetime := byID[id]
	delete(byID, id)
	if len(byID) == 0 {
		delete(m.hotkeyLifetimes, jobID)
	}
	m.hotkeyLifetimeMu.Unlock()
	if lifetime != nil {
		lifetime.active.Store(false)
		lifetime.cancel()
	}
}
