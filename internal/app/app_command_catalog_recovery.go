package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/credentials"
	"assistente/internal/logging"
	"assistente/internal/workspace"
)

const commandCatalogRefreshTimeout = 30 * time.Second

type commandCatalogRecoveryTicket struct {
	principal   auth.LocalSessionPrincipal
	sessionSvc  *auth.SessionService
	credMgr     *credentials.Manager
	workspace   *workspace.Manager
	workspaceID string
}

func commandCatalogRecoveryBackoff(attempt int) time.Duration {
	switch {
	case attempt <= 0:
		return time.Second
	case attempt == 1:
		return 2 * time.Second
	case attempt == 2:
		return 5 * time.Second
	case attempt == 3:
		return 10 * time.Second
	default:
		return 30 * time.Second
	}
}

func (a *App) captureCommandCatalogRecoveryTicket() *commandCatalogRecoveryTicket {
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return nil
	}
	a.authMu.RLock()
	sessions, credentials, manager := a.sessionSvc, a.credMgr, a.workspaceMgr
	a.authMu.RUnlock()
	if sessions == nil || credentials == nil || manager == nil {
		return nil
	}
	workspaceID := manager.ActiveID()
	if workspaceID == "" {
		return nil
	}
	ticket := &commandCatalogRecoveryTicket{principal: principal, sessionSvc: sessions, credMgr: credentials, workspace: manager, workspaceID: workspaceID}
	if !ticket.current(a) {
		return nil
	}
	return ticket
}

func (ticket *commandCatalogRecoveryTicket) current(a *App) bool {
	if ticket == nil || a == nil {
		return false
	}
	a.commandLifecycleMount.Lock()
	closing := a.commandLifecycleClosing
	a.commandLifecycleMount.Unlock()
	if closing {
		return false
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal != ticket.principal {
		return false
	}
	a.authMu.RLock()
	sameDependencies := a.sessionSvc == ticket.sessionSvc && a.credMgr == ticket.credMgr && a.workspaceMgr == ticket.workspace
	a.authMu.RUnlock()
	return sameDependencies && ticket.workspace.ActiveID() == ticket.workspaceID
}

// requestCommandCatalogRecovery coalesces MCP change events into one retry
// worker. Events arriving during a retry update the eventual snapshot without
// creating another goroutine.
func (a *App) requestCommandCatalogRecovery() {
	if a == nil {
		return
	}
	// Apps sem contexto de startup não podem possuir este worker.
	parent := a.commandBridgeContext()
	if a.ctx == nil || parent.Err() != nil {
		return
	}
	a.commandLifecycleMount.Lock()
	lifecycleClosing := a.commandLifecycleClosing
	a.commandLifecycleMount.Unlock()
	if lifecycleClosing {
		return
	}
	a.commandCatalogRecoveryMu.Lock()
	if a.commandCatalogRecoveryClosing {
		a.commandCatalogRecoveryMu.Unlock()
		return
	}
	// Captura e publicação do ticket são serializadas: uma requisição antiga
	// não pode sobrescrever uma captura mais recente após trocar de sessão.
	// A captura só lê memória; não acessa banco, rede ou o próprio worker.
	ticket := a.captureCommandCatalogRecoveryTicket()
	if a.commandCatalogRecoveryRunning {
		a.commandCatalogRecoveryPendingTicket = ticket
		select {
		case a.commandCatalogRecoveryWake <- struct{}{}:
		default:
		}
		a.commandCatalogRecoveryMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	a.commandCatalogRecoveryRunning = true
	a.commandCatalogRecoveryWake = make(chan struct{}, 1)
	a.commandCatalogRecoveryCancel = cancel
	a.commandCatalogRecoveryDone = make(chan struct{})
	a.commandCatalogRecoveryPendingTicket = nil
	wake, done := a.commandCatalogRecoveryWake, a.commandCatalogRecoveryDone
	go func() {
		defer cancel()
		a.runCommandCatalogRecovery(ctx, ticket, wake, done)
	}()
	a.commandCatalogRecoveryMu.Unlock()
}

func (a *App) runCommandCatalogRecovery(ctx context.Context, ticket *commandCatalogRecoveryTicket, wake <-chan struct{}, done chan struct{}) {
	defer func() {
		a.commandCatalogRecoveryMu.Lock()
		if a.commandCatalogRecoveryDone == done {
			a.commandCatalogRecoveryRunning = false
			a.commandCatalogRecoveryWake = nil
			a.commandCatalogRecoveryCancel = nil
			a.commandCatalogRecoveryDone = nil
			a.commandCatalogRecoveryPendingTicket = nil
			close(done)
		}
		a.commandCatalogRecoveryMu.Unlock()
	}()

	attempt := 0
	refreshPending := true
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if refreshPending {
			if a.jobMgr != nil {
				if err := a.jobMgr.RegenerateCatalog(); err != nil {
					logging.Errorf(context.Background(), "app.app-mcp", "Catalog regeneration failed (error_class=%s)", commandLoadErrorClass(err))
				} else {
					logging.Infof(context.Background(), "app.app-mcp", "Catalog regenerated after MCP tools change")
				}
			}
			refreshPending = false
		}
		if ticket == nil {
			finished, nextTicket, hasEvent := a.finishCommandCatalogRecoveryIfIdle(ctx, wake, done)
			if finished {
				return
			}
			if ctx.Err() != nil || !hasEvent {
				return
			}
			ticket = nextTicket
			refreshPending = true
			continue
		}
		if !ticket.current(a) {
			select {
			case <-ctx.Done():
				return
			case <-wake:
				ticket = a.takeCommandCatalogRecoveryTicket()
				refreshPending = true
				continue
			default:
				finished, nextTicket, hasEvent := a.finishCommandCatalogRecoveryIfIdle(ctx, wake, done)
				if finished {
					return
				}
				if ctx.Err() != nil || !hasEvent {
					return
				}
				ticket = nextTicket
				refreshPending = true
				continue
			}
		}

		refreshCtx, cancel := context.WithTimeout(ctx, commandCatalogRefreshTimeout)
		loadCtx, trace := beginCommandLoad(refreshCtx, "command_catalog_hot_swap")
		commandLoadStage(loadCtx, "refresh")
		err := a.refreshCommandProductCatalogForRecovery(loadCtx, ticket)
		commandLoadCause(loadCtx, err)
		trace.finish(err)
		cancel()
		if err == nil {
			attempt = 0
			if ctx.Err() != nil {
				return
			}
			finished, nextTicket, hasEvent := a.finishCommandCatalogRecoveryIfIdle(ctx, wake, done)
			if finished {
				return
			}
			if ctx.Err() != nil || !hasEvent {
				return
			}
			ticket = nextTicket
			refreshPending = true
			continue
		}
		if !ticket.current(a) {
			finished, nextTicket, hasEvent := a.finishCommandCatalogRecoveryIfIdle(ctx, wake, done)
			if finished {
				return
			}
			if ctx.Err() != nil || !hasEvent {
				return
			}
			ticket = nextTicket
			refreshPending = true
			continue
		}
		logging.Warnf(context.Background(), "app.app-mcp", "Catalog refresh deferred (error_class=%s)", commandLoadErrorClass(err))
		wait := time.NewTimer(commandCatalogRecoveryBackoff(attempt))
		authCheck := time.NewTicker(time.Second)
		attempt++
		for {
			select {
			case <-ctx.Done():
				wait.Stop()
				authCheck.Stop()
				return
			case <-wake:
				refreshPending = true
				ticket = a.takeCommandCatalogRecoveryTicket()
			case <-authCheck.C:
				if ticket == nil || !ticket.current(a) {
					wait.Stop()
					authCheck.Stop()
					// A saída precisa compartilhar o mutex com eventos novos;
					// o defer sozinho poderia apagar um wake da sessão seguinte.
					goto retry
				}
			case <-wait.C:
				authCheck.Stop()
				goto retry
			}
		}
	retry:
	}
}

func (a *App) stopCommandCatalogRecovery(ctx context.Context) {
	if a == nil {
		return
	}
	a.commandCatalogRecoveryMu.Lock()
	a.commandCatalogRecoveryClosing = true
	cancel, done := a.commandCatalogRecoveryCancel, a.commandCatalogRecoveryDone
	if cancel != nil {
		cancel()
	}
	a.commandCatalogRecoveryMu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
		logging.Warnf(context.Background(), "app.app-mcp", "catalog recovery did not join before shutdown deadline")
	}
}

// finishCommandCatalogRecoveryIfIdle makes the idle decision atomic with event
// enqueue: a concurrent MCP notification either wakes this worker or observes
// running=false and starts the next one.
func (a *App) finishCommandCatalogRecoveryIfIdle(ctx context.Context, wake <-chan struct{}, done chan struct{}) (finished bool, ticket *commandCatalogRecoveryTicket, hasEvent bool) {
	a.commandCatalogRecoveryMu.Lock()
	defer a.commandCatalogRecoveryMu.Unlock()
	select {
	case <-wake:
		ticket = a.commandCatalogRecoveryPendingTicket
		a.commandCatalogRecoveryPendingTicket = nil
		return false, ticket, true
	default:
	}
	if ctx.Err() != nil || a.commandCatalogRecoveryDone != done {
		return false, nil, false
	}
	a.commandCatalogRecoveryRunning = false
	a.commandCatalogRecoveryWake = nil
	a.commandCatalogRecoveryCancel = nil
	a.commandCatalogRecoveryDone = nil
	a.commandCatalogRecoveryPendingTicket = nil
	close(done)
	return true, nil, false
}

func (a *App) takeCommandCatalogRecoveryTicket() *commandCatalogRecoveryTicket {
	a.commandCatalogRecoveryMu.Lock()
	wake := a.commandCatalogRecoveryWake
	ticket := a.commandCatalogRecoveryPendingTicket
	a.commandCatalogRecoveryPendingTicket = nil
	for wake != nil {
		select {
		case <-wake:
			// O ticket já é o mais recente sob o mutex. Wakes coalescidos
			// não podem substituí-lo pelo nil que acabamos de armazenar.
		default:
			wake = nil
		}
	}
	a.commandCatalogRecoveryMu.Unlock()
	return ticket
}
