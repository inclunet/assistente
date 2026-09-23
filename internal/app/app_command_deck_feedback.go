package app

import (
	"context"
	"errors"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

const (
	commandDeckFeedbackLimit       = 64
	commandDeckFeedbackTerminalTTL = 3 * time.Second
)

// commandDeckFeedbackEntry is deliberately separate from the durable ledger.
// It is only the short-lived presentation projection for the physical Deck
// occurrence that already passed the normal authorization pipeline.
type commandDeckFeedbackEntry struct {
	ctx          context.Context
	identity     string
	instanceID   string
	versions     commandexecution.Versions
	generation   uint64
	invocationID string
	state        string
	expiresAt    time.Time
}

type commandDeckFeedbackState struct {
	active    map[string]commandDeckFeedbackEntry
	completed map[string]commandDeckFeedbackEntry
}

func newCommandDeckFeedbackState() *commandDeckFeedbackState {
	return &commandDeckFeedbackState{
		active:    make(map[string]commandDeckFeedbackEntry),
		completed: make(map[string]commandDeckFeedbackEntry),
	}
}

func (s *commandDeckExecution) feedbackLocked() *commandDeckFeedbackState {
	if s.feedback == nil {
		s.feedback = newCommandDeckFeedbackState()
	}
	if s.feedback.active == nil {
		s.feedback.active = make(map[string]commandDeckFeedbackEntry)
	}
	if s.feedback.completed == nil {
		s.feedback.completed = make(map[string]commandDeckFeedbackEntry)
	}
	return s.feedback
}

func (s *commandDeckExecution) pruneDeckFeedbackLocked(now time.Time) {
	if s == nil || s.feedback == nil {
		return
	}
	for identity, entry := range s.feedback.completed {
		if !now.Before(entry.expiresAt) || entry.ctx == nil || entry.ctx.Err() != nil {
			delete(s.feedback.completed, identity)
		}
	}
}

func (s *commandDeckExecution) feedbackCountLocked() int {
	if s == nil || s.feedback == nil {
		return 0
	}
	return len(s.feedback.active) + len(s.feedback.completed)
}

func (s *commandDeckExecution) latestFeedbackLocked(identity string) (commandDeckFeedbackEntry, bool) {
	if s == nil || s.feedback == nil {
		return commandDeckFeedbackEntry{}, false
	}
	if entry, ok := s.feedback.active[identity]; ok {
		return entry, true
	}
	entry, ok := s.feedback.completed[identity]
	return entry, ok
}

func (s *commandDeckExecution) registerDeckFeedbackLocked(invocationID string, occurrence commandDeckOccurrence) {
	if s == nil || invocationID == "" || occurrence.identity == "" {
		return
	}
	feedback := s.feedbackLocked()
	now := time.Now()
	s.pruneDeckFeedbackLocked(now)
	ctx := occurrence.ctx
	if ctx == nil {
		return
	}
	identity := occurrence.identity
	if _, exists := s.latestFeedbackLocked(identity); exists {
		delete(feedback.active, identity)
		delete(feedback.completed, identity)
	} else if s.feedbackCountLocked() >= commandDeckFeedbackLimit {
		return
	}
	feedback.active[identity] = commandDeckFeedbackEntry{
		ctx: ctx, identity: identity, instanceID: occurrence.instanceID, versions: occurrence.versions,
		generation: occurrence.generation, invocationID: invocationID,
		state: commandDeckFeedbackWaiting,
	}
}

const (
	commandDeckFeedbackWaiting = "waiting"
	commandDeckFeedbackRunning = "running"
)

func (s *commandDeckExecution) markDeckFeedbackRunning(invocationID string) {
	if s == nil || invocationID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneDeckFeedbackLocked(time.Now())
	for identity, entry := range s.feedbackLocked().active {
		if entry.invocationID == invocationID {
			entry.state = commandDeckFeedbackRunning
			s.feedback.active[identity] = entry
			return
		}
	}
}

func (s *commandDeckExecution) finishDeckFeedback(invocationID string, record commandledger.FullRecord, executeErr error) {
	if s == nil || invocationID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneDeckFeedbackLocked(now)
	feedback := s.feedbackLocked()
	for identity, entry := range feedback.active {
		if entry.invocationID != invocationID {
			continue
		}
		// Only the current invocation for an identity may publish a terminal
		// state. A late older invocation is intentionally ignored.
		delete(feedback.active, identity)
		entry.state = commandDeckFeedbackResult(record.Status, executeErr)
		entry.expiresAt = now.Add(commandDeckFeedbackTerminalTTL)
		feedback.completed[identity] = entry
		return
	}
}

// executeEnvelopeWithFeedback is the only Deck execution bridge. A nil error
// is not evidence of success: the authoritative ledger status is always
// converted, with an absent status becoming outcome_unknown.
func (s *commandDeckExecution) executeEnvelopeWithFeedback(ctx context.Context, token string, candidate commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
	if s == nil || s.service == nil {
		return commandledger.FullRecord{}, commandexecution.ErrInvalidRequest
	}
	record, err := s.service.ExecuteEnvelope(ctx, token, candidate)
	s.finishDeckFeedback(candidate.InvocationID, record, err)
	return record, err
}

func commandDeckFeedbackResult(status commandledger.Status, executeErr error) string {
	switch status {
	case commandledger.Evaluating, commandledger.Queued, commandledger.Running:
		return "outcome_unknown"
	case commandledger.Succeeded:
		return "succeeded"
	case commandledger.Failed:
		return "failed"
	case commandledger.Denied, commandledger.Suppressed:
		return "denied"
	case commandledger.Cancelled, commandledger.CancelledStale, commandledger.RejectedStale:
		return "cancelled"
	case commandledger.TimedOut:
		return "timed_out"
	case commandledger.OutcomeUnknown:
		return "outcome_unknown"
	}
	if errors.Is(executeErr, context.DeadlineExceeded) {
		return "timed_out"
	}
	if errors.Is(executeErr, context.Canceled) || errors.Is(executeErr, commandexecution.ErrStale) {
		return "cancelled"
	}
	if errors.Is(executeErr, commandexecution.ErrDenied) {
		return "denied"
	}
	if errors.Is(executeErr, commandexecution.ErrExecution) {
		return "failed"
	}
	return "outcome_unknown"
}

// deckFeedbackSnapshot is memory-only and deliberately repeats the physical
// occurrence guards. It never consults the database or the audit projection.
func (p *commandProductRuntime) deckFeedbackSnapshot(identity, instanceID string, versions commandexecution.Versions, generation uint64) (state string, invocationID string) {
	if p == nil || p.deckExecution == nil || identity == "" || instanceID == "" {
		return "", ""
	}
	execution := p.deckExecution
	now := time.Now()
	execution.mu.Lock()
	defer execution.mu.Unlock()
	execution.pruneDeckFeedbackLocked(now)
	feedback := execution.feedback
	if feedback == nil {
		return "", ""
	}
	entry, ok := feedback.active[identity]
	if !ok {
		entry, ok = feedback.completed[identity]
	}
	if !ok || entry.ctx == nil || entry.ctx.Err() != nil || entry.instanceID != instanceID || entry.versions != versions || entry.generation != generation {
		return "", ""
	}
	return entry.state, entry.invocationID
}

func copyDeckFeedbackHandlers(state *commandDeckExecution, handlers map[string]commandexecution.Handler) map[string]commandexecution.Handler {
	copyHandlers := make(map[string]commandexecution.Handler, len(handlers))
	for id, handler := range handlers {
		start := handler.Start
		if start == nil {
			copyHandlers[id] = handler
			continue
		}
		handler.Start = func(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
			state.markDeckFeedbackRunning(invocation.ID)
			return start(ctx, invocation)
		}
		copyHandlers[id] = handler
	}
	return copyHandlers
}
