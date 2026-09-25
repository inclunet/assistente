package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
)

// A source watch survives configuration publication but not auth/security
// invalidation. Guards only inspect its context: no reentrant gate or SQL.
func (p *commandProductRuntime) commandManualClaimAuthority(ctx context.Context) (commandsecurity.EpochSnapshot, func(context.Context) error, error) {
	if p == nil {
		return commandsecurity.EpochSnapshot{}, nil, commandexecution.ErrInvalidConfiguration
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
			return "", "", commandexecution.ErrDenied
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return epoch, nil, err
	}
	watch, release, err := p.epochs.WatchSecurityEpoch(p.app.commandBridgeContext(), epoch)
	if err != nil {
		return epoch, nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || watch.Err() != nil {
		release()
		return epoch, nil, commandexecution.ErrStale
	}
	if p.manualAuthorityEpoch == epoch && p.manualAuthorityWatch != nil && p.manualAuthorityWatch.Err() == nil {
		release()
		watch = p.manualAuthorityWatch
	} else {
		if p.manualAuthorityRelease != nil {
			p.manualAuthorityRelease()
		}
		p.manualAuthorityEpoch, p.manualAuthorityWatch, p.manualAuthorityRelease = epoch, watch, release
	}
	return epoch, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if watch.Err() != nil {
			return commandexecution.ErrStale
		}
		return nil
	}, nil
}

func commandCurrentManualLayerIDs(snapshot commandconfig.Snapshot, principal auth.LocalSessionPrincipal, now time.Time, epoch commandsecurity.EpochSnapshot) []string {
	return commandLifecycleActiveUserLayerIDs(commandClaimsAtEpoch(snapshot, epoch), principal, now)
}

func commandClaimsAtEpoch(snapshot commandconfig.Snapshot, epoch commandsecurity.EpochSnapshot) commandconfig.Snapshot {
	current := snapshot
	current.ActivationClaims = nil
	for _, claim := range snapshot.ActivationClaims {
		if claim.AuthContextType == "local_session" && claim.AuthContextID == epoch.SessionID && claim.UserID == epoch.UserID && claim.AuthGeneration == epoch.AuthGeneration && claim.SecurityGeneration == epoch.SecurityGeneration {
			current.ActivationClaims = append(current.ActivationClaims, claim)
		}
	}
	return current
}

func commandManualLifecycleSupported(lifecycle commandactivation.Lifecycle) bool {
	return lifecycle == commandactivation.LifecyclePersistent || lifecycle == commandactivation.LifecycleSession || lifecycle == commandactivation.LifecycleTemporary
}

func commandManualClaimsDue(snapshot commandconfig.Snapshot, now time.Time) bool {
	for _, claim := range snapshot.ActivationClaims {
		if claim.SourceType == "manual" && claim.State == commandactivation.StateActive && claim.ExpiresAt != nil && !claim.ExpiresAt.After(now) {
			return true
		}
	}
	return false
}

// The deadline is conservative: even a disabled layer's claim must expire so
// that enabling it later cannot revive an elapsed activation.
func commandManualClaimsDeadline(snapshot commandconfig.Snapshot, principal auth.LocalSessionPrincipal, now time.Time) time.Time {
	var deadline time.Time
	for _, claim := range snapshot.ActivationClaims {
		if claim.UserID != principal.UserID || claim.AuthContextID != principal.SessionID || !commandLifecycleInScope(claim.WorkspaceID, snapshot.Scope.WorkspaceID) || claim.SourceType != "manual" || claim.State != commandactivation.StateActive || claim.ExpiresAt == nil || !claim.ExpiresAt.After(now) {
			continue
		}
		if deadline.IsZero() || claim.ExpiresAt.Before(deadline) {
			deadline = *claim.ExpiresAt
		}
	}
	return deadline
}

// One worker belongs to the mounted runtime, not to a screen. A publication
// wakes it to read the current immutable snapshot; out-of-order notifications
// cannot replace a newer deadline with an older one.
func (p *commandProductRuntime) scheduleCommandManualExpiry() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.manualExpiryWake == nil {
		ctx, cancel := context.WithCancel(p.app.commandBridgeContext())
		p.manualExpiryCancel = cancel
		p.manualExpiryWake = make(chan struct{}, 1)
		p.workers.Add(1)
		go p.runCommandManualExpiry(ctx, p.manualExpiryWake)
	}
	select {
	case p.manualExpiryWake <- struct{}{}:
	default:
	}
}

func (p *commandProductRuntime) runCommandManualExpiry(ctx context.Context, wake <-chan struct{}) {
	defer p.workers.Done()
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	var tick <-chan time.Time
	var pendingDeadline time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-tick:
			if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
				return
			}
			// Guard already denies the expired snapshot, even if this rebuild
			// fails. Never replay an invocation after a publication failure.
			_ = p.app.rebuildCommandLifecycleProjection(ctx, false)
		}
		if ctx.Err() != nil {
			return
		}
		if timer != nil {
			timer.Stop()
		}
		tick = nil
		// Scheduling may inspect an expired snapshot without admitting it.
		// ResolutionSnapshot would reject its guard before we could discover
		// the deadline if the worker's first wake was delayed past expiry.
		configuration, _, err := p.host.UserConfiguration(ctx, p.principal.UserID)
		if err == nil {
			pendingDeadline = configuration.ValidUntil()
		}
		// Unrelated readiness/security failures are not an invitation for an
		// expiry worker to republish configuration that had no expiry pending.
		if pendingDeadline.IsZero() {
			continue
		}
		delay := time.Second // bounded retry for a failed expiry publication
		if pendingDeadline.After(time.Now()) {
			delay = max(time.Millisecond, time.Until(pendingDeadline))
		}
		timer = time.NewTimer(delay)
		tick = timer.C
	}
}
