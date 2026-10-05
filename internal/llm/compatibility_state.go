package llm

import (
	"context"
	"sync"

	"assistente/internal/llmcapabilities"
)

// CompatibilityState compartilha o estado de capability descoberto durante
// um único turno, inclusive entre cópias de ChatParams no loop agêntico.
type CompatibilityState struct {
	mu          sync.Mutex
	modelID     string
	revision    int
	unsupported map[llmcapabilities.Capability]map[llmcapabilities.FieldKey]struct{}
	retryUsed   bool
	persist     func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error
}

func NewCompatibilityState(modelID string, revision int, unsupported []CompatibilityField, persist func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error) *CompatibilityState {
	state := &CompatibilityState{modelID: modelID, revision: revision, unsupported: make(map[llmcapabilities.Capability]map[llmcapabilities.FieldKey]struct{}), persist: persist}
	for _, item := range unsupported {
		if !llmcapabilities.HasField(item.Capability, item.Field) {
			continue
		}
		if state.unsupported[item.Capability] == nil {
			state.unsupported[item.Capability] = make(map[llmcapabilities.FieldKey]struct{})
		}
		state.unsupported[item.Capability][item.Field] = struct{}{}
	}
	return state
}

type CompatibilityField struct {
	Capability llmcapabilities.Capability
	Field      llmcapabilities.FieldKey
}

func (s *CompatibilityState) IsUnsupported(capability llmcapabilities.Capability, field llmcapabilities.FieldKey) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.unsupported[capability][field]
	return ok
}

// LearnAndClaimRetry persists the restriction before permitting one retry.
// The shared budget is consumed even if persistence fails, preventing loops.
func (s *CompatibilityState) LearnAndClaimRetry(ctx context.Context, capability llmcapabilities.Capability, field llmcapabilities.FieldKey, recognizerID string) bool {
	if s == nil || s.persist == nil || s.modelID == "" || s.revision < 1 || !llmcapabilities.HasField(capability, field) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retryUsed {
		return false
	}
	s.retryUsed = true
	if err := s.persist(ctx, capability, field, recognizerID); err != nil {
		return false
	}
	if s.unsupported[capability] == nil {
		s.unsupported[capability] = make(map[llmcapabilities.FieldKey]struct{})
	}
	s.unsupported[capability][field] = struct{}{}
	return true
}
