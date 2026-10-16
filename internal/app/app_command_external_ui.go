package app

import (
	"context"

	"assistente/internal/commandbridge"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
)

type ExternalUICommandRequest struct {
	Owner            commandbridge.Owner `json:"owner"`
	ConnectionID     string              `json:"connectionId"`
	Generation       string              `json:"generation"`
	InvocationID     string              `json:"invocationId"`
	TargetSnapshotID string              `json:"targetSnapshotId"`
	ContextVersion   string              `json:"contextVersion"`
}

type ExternalUICommandCompletion struct {
	Owner            commandbridge.Owner `json:"owner"`
	ConnectionID     string              `json:"connectionId"`
	Generation       string              `json:"generation"`
	InvocationID     string              `json:"invocationId"`
	ReceiptID        string              `json:"receiptId"`
	TargetSnapshotID string              `json:"targetSnapshotId"`
	ContextVersion   string              `json:"contextVersion"`
	Outcome          string              `json:"outcome"`
}

type ExternalUICommandAck struct {
	Accepted bool `json:"accepted"`
}

// O manager contém apenas convites, vínculos e entregas efêmeras. Nunca recebe
// um JWT da interface nem transforma a sessão de destino em autoridade externa.
func (a *App) ensureExternalUIConnections() *commandui.ExternalUIConnections {
	if manager := a.commandExternalUI.Load(); manager != nil {
		return manager
	}
	manager := commandui.NewExternalUIConnections()
	if a.commandExternalUI.CompareAndSwap(nil, manager) {
		return manager
	}
	manager.Close()
	return a.commandExternalUI.Load()
}

func (a *App) clearExternalUIConnections() {
	if a != nil {
		if manager := a.commandExternalUI.Load(); manager != nil {
			manager.Clear()
		}
	}
}

// A operação curta fica no mesmo gate das transições de autenticação. O owner
// informado pela UI é apenas uma expectativa, comparada com a sessão real.
func (a *App) withExternalUIOwner(expected *commandbridge.Owner, operation func(*commandui.ExternalUIConnections, commandbridge.Owner) error) error {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return err
	}
	owner := commandbridge.Owner{UserID: p.principal.UserID, SessionID: p.principal.SessionID, WorkspaceID: p.workspaceID}
	if expected != nil && *expected != owner {
		return commandexecution.ErrDenied
	}
	ctx := a.commandBridgeContext()
	validate := func(ctx context.Context) error {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		state, err := p.host.Snapshot(ctx, p.principal)
		if err != nil || !state.Unlocked {
			return commandexecution.ErrDenied
		}
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return commandexecution.ErrDenied
		}
		return nil
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		if err := validate(ctx); err != nil {
			return "", "", err
		}
		return owner.UserID, owner.SessionID, nil
	})
	if err != nil {
		return err
	}
	return p.epochs.Admit(ctx, epoch, validate, func() error {
		// Shutdown marca closed sob este mutex antes de limpar os vínculos.
		// Manter a checagem e a operação juntas impede recriar um convite
		// depois da limpeza, mesmo sem mudança do epoch de autenticação.
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed || a.commandProduct.Load() != p {
			return commandexecution.ErrDenied
		}
		return operation(a.ensureExternalUIConnections(), owner)
	})
}

func (a *App) ReadExternalUIConnection() (commandui.ExternalUIConnectionStatus, error) {
	var status commandui.ExternalUIConnectionStatus
	err := a.withExternalUIOwner(nil, func(manager *commandui.ExternalUIConnections, owner commandbridge.Owner) error {
		var err error
		status, err = manager.ReadForOwner(owner)
		return err
	})
	return status, err
}

func (a *App) BeginExternalUIConnection(target commandui.ExternalUIDestination) (commandui.ExternalUIInvitation, error) {
	var invitation commandui.ExternalUIInvitation
	err := a.withExternalUIOwner(nil, func(manager *commandui.ExternalUIConnections, owner commandbridge.Owner) error {
		if target.WorkspaceID != owner.WorkspaceID {
			return commandexecution.ErrDenied
		}
		var err error
		invitation, err = manager.Begin(owner, target)
		return err
	})
	return invitation, err
}

func (a *App) PublishExternalUIContext(publication commandui.ExternalUIContextPublication) (commandui.ExternalUIConnectionStatus, error) {
	var status commandui.ExternalUIConnectionStatus
	err := a.withExternalUIOwner(&publication.Owner, func(manager *commandui.ExternalUIConnections, owner commandbridge.Owner) error {
		if publication.Target.WorkspaceID != owner.WorkspaceID {
			return commandexecution.ErrDenied
		}
		var err error
		status, err = manager.PublishContext(publication)
		return err
	})
	return status, err
}

func (a *App) HeartbeatExternalUIConnection(lease commandui.ExternalUILease) (commandui.ExternalUIConnectionStatus, error) {
	var status commandui.ExternalUIConnectionStatus
	err := a.withExternalUIOwner(&lease.Owner, func(manager *commandui.ExternalUIConnections, _ commandbridge.Owner) error {
		var err error
		status, err = manager.Heartbeat(lease)
		return err
	})
	return status, err
}

func (a *App) DisconnectExternalUIConnection(lease commandui.ExternalUILease) error {
	return a.withExternalUIOwner(&lease.Owner, func(manager *commandui.ExternalUIConnections, _ commandbridge.Owner) error {
		return manager.Disconnect(lease)
	})
}

func (a *App) TakeExternalUICommand(request ExternalUICommandRequest) (commandui.ExternalUIHandoff, error) {
	var handoff commandui.ExternalUIHandoff
	err := a.withExternalUIOwner(&request.Owner, func(manager *commandui.ExternalUIConnections, owner commandbridge.Owner) error {
		var err error
		handoff, err = manager.TakeExternal(owner, request.ConnectionID, request.Generation, request.InvocationID, request.TargetSnapshotID, request.ContextVersion)
		return err
	})
	return handoff, err
}

func (a *App) CompleteExternalUICommand(request ExternalUICommandCompletion) (ExternalUICommandAck, error) {
	outcome := request.Outcome
	switch outcome {
	case "succeeded", "failed", "cancelled":
	case "unknown":
		outcome = "outcome_unknown"
	default:
		return ExternalUICommandAck{}, commandexecution.ErrInvalidRequest
	}
	err := a.withExternalUIOwner(&request.Owner, func(manager *commandui.ExternalUIConnections, owner commandbridge.Owner) error {
		return manager.CompleteExternal(owner, request.ConnectionID, request.Generation, request.InvocationID, request.ReceiptID, request.TargetSnapshotID, request.ContextVersion, outcome)
	})
	return ExternalUICommandAck{Accepted: err == nil}, err
}
