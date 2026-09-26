package app

import (
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"github.com/google/uuid"
)

const commandDeckPagePresentationTTL = 8 * time.Second

// This lease is volatile presentation state only. Deck input and command
// authorization never read it; those paths continue to capture live context.
type commandDeckPagePresentationSnapshot struct {
	appPage, userID, sessionID, workspaceID, generation string
	revision                                            int64
	expiresAt                                           time.Time
}

func validCommandDeckPresentationGeneration(generation string) bool {
	id, err := uuid.Parse(generation)
	return err == nil && id.Version() == 7
}

// PublishCommandDeckPagePresentation updates only the physical Deck's visual
// page projection. Identity is derived from the authenticated local product;
// the caller supplies a map generation and monotonic UI revision, not an owner.
func (a *App) PublishCommandDeckPagePresentation(appPage, generation string, revision int64) error {
	if a == nil || !commandbindings.IsAppPage(appPage) || !validCommandDeckPresentationGeneration(generation) || revision <= 0 {
		return commandexecution.ErrInvalidRequest
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return err
	}
	currentGeneration, _, ready := p.localKeyboardState()
	if !ready || currentGeneration != generation || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
		return commandexecution.ErrStale
	}
	versions, err := p.host.Snapshot(a.commandBridgeContext(), p.principal)
	if err != nil || !versions.Unlocked {
		return commandexecution.ErrDenied
	}
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return commandexecution.ErrStale
	}

	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	if p.keyboardMap == nil || p.keyboardMap.view.Generation != generation {
		return commandexecution.ErrStale
	}
	p.deckPagePresentationMu.Lock()
	defer p.deckPagePresentationMu.Unlock()
	if p.deckPagePresentationRevisionGeneration != generation {
		p.deckPagePresentationRevisionGeneration = generation
		p.deckPagePresentationRev = 0
	}
	if revision <= p.deckPagePresentationRev {
		return commandexecution.ErrStale
	}
	p.deckPagePresentationRev = revision
	p.deckPagePresentation = &commandDeckPagePresentationSnapshot{
		appPage: appPage, userID: p.principal.UserID, sessionID: p.principal.SessionID,
		workspaceID: p.workspaceID, generation: generation, revision: revision,
		expiresAt: time.Now().Add(commandDeckPagePresentationTTL),
	}
	return nil
}

// ClearCommandDeckPagePresentation retires only the matching visual lease.
// A delayed clear from an older map generation cannot erase a newer snapshot.
func (a *App) ClearCommandDeckPagePresentation(generation string, revision int64) error {
	if a == nil || !validCommandDeckPresentationGeneration(generation) || revision <= 0 {
		return commandexecution.ErrInvalidRequest
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return err
	}
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
		return commandexecution.ErrStale
	}
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	if p.keyboardMap == nil || p.keyboardMap.view.Generation != generation {
		// Clears from a retired keyboard map are harmless and must not consume
		// revisions belonging to the currently published map.
		return nil
	}
	p.deckPagePresentationMu.Lock()
	defer p.deckPagePresentationMu.Unlock()
	if p.deckPagePresentationRevisionGeneration != generation {
		p.deckPagePresentationRevisionGeneration = generation
		p.deckPagePresentationRev = 0
	}
	if revision <= p.deckPagePresentationRev {
		return commandexecution.ErrStale
	}
	p.deckPagePresentationRev = revision
	if current := p.deckPagePresentation; current != nil && current.generation == generation &&
		current.userID == p.principal.UserID && current.sessionID == p.principal.SessionID && current.workspaceID == p.workspaceID {
		p.deckPagePresentation = nil
	}
	return nil
}

func (p *commandProductRuntime) clearDeckPagePresentation(generation string) {
	if p == nil {
		return
	}
	p.deckPagePresentationMu.Lock()
	if current := p.deckPagePresentation; current != nil && (generation == "" || current.generation == generation) {
		p.deckPagePresentation = nil
	}
	p.deckPagePresentationMu.Unlock()
}

func (p *commandProductRuntime) currentDeckPagePresentation() string {
	if p == nil {
		return ""
	}
	generation, _, ready := p.localKeyboardState()
	if !ready {
		return ""
	}
	p.deckPagePresentationMu.Lock()
	defer p.deckPagePresentationMu.Unlock()
	current := p.deckPagePresentation
	if current == nil || current.expiresAt.IsZero() || !time.Now().Before(current.expiresAt) ||
		current.generation != generation || current.userID != p.principal.UserID ||
		current.sessionID != p.principal.SessionID || current.workspaceID != p.workspaceID ||
		!commandbindings.IsAppPage(current.appPage) {
		if current != nil && !time.Now().Before(current.expiresAt) {
			p.deckPagePresentation = nil
		}
		return ""
	}
	return current.appPage
}
