package app

import (
	"context"
	"strings"
	"time"
	"unicode"

	"assistente/internal/commandruntime"
)

// This event describes a rendered execution result, not a new authority to run
// a command. Device identifiers never leave the native presentation boundary.
type commandDeckFeedbackEvent struct {
	InvocationID string `json:"invocationId"`
	State        string `json:"state"`
	Title        string `json:"title"`
	UserID       string `json:"userId"`
	SessionID    string `json:"sessionId"`
	WorkspaceID  string `json:"workspaceId"`
	Generation   string `json:"generation"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// Called only after a successful frame write. Recheck ownership and the live
// occurrence so a frame superseded during I/O cannot announce stale feedback.
func (c *commandDeckController) announceDeckFeedback(ctx context.Context, serial string, bindings map[int]commandDeckBinding) {
	if c == nil || c.p == nil || ctx == nil || ctx.Err() != nil {
		return
	}
	p := c.p
	if p.app == nil || p.app.emitter == nil || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) || !p.deckExecutionAllowed(c.generation) {
		return
	}
	generation, versions, ready := p.localKeyboardState()
	if !ready || generation == "" || versions != c.versions {
		return
	}
	current, err := p.host.Snapshot(ctx, p.principal)
	if err != nil || current != c.versions || !current.Unlocked {
		return
	}
	lifecycle, err := CommandLifecycleSnapshot(p.app)
	if err != nil || lifecycle.State != commandruntime.StateReady || !lifecycle.Published {
		return
	}
	owner, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
	if err != nil || owner != p.principal {
		return
	}
	c.mu.Lock()
	instance := c.instances[serial]
	c.mu.Unlock()
	if instance.ctx == nil || instance.ctx.Err() != nil {
		return
	}
	for _, binding := range bindings {
		if binding.feedbackState == "" || binding.feedbackInvocationID == "" {
			continue
		}
		state, invocation := p.deckFeedbackSnapshot(binding.identity, instance.id, c.versions, c.generation)
		if state != binding.feedbackState || invocation != binding.feedbackInvocationID {
			continue
		}
		token := invocation + ":" + state
		c.mu.Lock()
		if c.feedbackAnnounced == nil {
			c.feedbackAnnounced = make(map[string]string)
		}
		duplicate := c.feedbackAnnounced[binding.identity] == token
		if !duplicate {
			// Hardware/layout cardinality is bounded independently of execution
			// history. Never grow a ledger of presses in the presentation layer.
			if len(c.feedbackAnnounced) >= 64 {
				clear(c.feedbackAnnounced)
			}
			c.feedbackAnnounced[binding.identity] = token
		}
		c.mu.Unlock()
		if duplicate || ctx.Err() != nil || instance.ctx.Err() != nil || p.app.commandProduct.Load() != p || !p.deckExecutionAllowed(c.generation) {
			continue
		}
		p.app.emitter.Emit("command:deck-feedback", commandDeckFeedbackEvent{
			InvocationID: invocation, State: state, Title: commandDeckFeedbackTitle(binding.title),
			UserID: owner.UserID, SessionID: owner.SessionID, WorkspaceID: p.workspaceID,
			Generation: generation, ExpiresAt: time.Now().Add(3 * time.Second).UnixMilli(),
		})
	}
}

func commandDeckFeedbackTitle(title string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, title)
	runes := []rune(strings.TrimSpace(clean))
	if len(runes) > 256 {
		return string(runes[:255]) + "…"
	}
	return string(runes)
}
