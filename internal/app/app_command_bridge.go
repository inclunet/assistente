package app

import (
	"context"
	"errors"

	"assistente/internal/commandbridge"
)

var errCommandBridgeAlreadyConfigured = errors.New("ponte de comandos já configurada")

// ConfigureCommandBridge instala a ponte UI/backend criada pelo bootstrap
// confiável. A função não abre sessão nem registra comandos de produto.
func ConfigureCommandBridge(a *App, bridge *commandbridge.Bridge) error {
	if a == nil || bridge == nil {
		return commandbridge.ErrInvalidConfiguration
	}
	a.commandLifecycleMount.Lock()
	defer a.commandLifecycleMount.Unlock()
	if a.commandLifecycleClosing {
		return commandbridge.ErrBridgeClosed
	}
	if a.commandBridge.Load() != nil {
		return errCommandBridgeAlreadyConfigured
	}
	a.commandBridge.Store(bridge)
	return nil
}

func loadCommandBridge(a *App) (*commandbridge.Bridge, bool) {
	if a == nil {
		return nil, false
	}
	bridge := a.commandBridge.Load()
	return bridge, bridge != nil
}

func (a *App) commandBridgeContext() context.Context {
	if a != nil && a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) validateCommandBridgeOwner(owner commandbridge.Owner) error {
	if a == nil {
		return commandbridge.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentUserID == "" || a.currentAuthUser == nil ||
		a.currentAuthUser.UserID != a.currentUserID ||
		owner.UserID != a.currentUserID ||
		owner.SessionID != a.currentAuthUser.SessionID {
		return commandbridge.ErrSessionUnavailable
	}
	return nil
}

// CommandBridgeInvoke é o ponto Wails/backend para dispatch tipado da UI. O
// owner ainda precisa casar com capability/sessão dentro da ponte; aqui o App
// apenas bloqueia payload que tente trocar usuário ou sessão autenticada.
func (a *App) CommandBridgeInvoke(invocation commandbridge.Invocation, owner commandbridge.Owner) (commandbridge.InvocationAck, error) {
	bridge, ok := loadCommandBridge(a)
	if !ok {
		return commandbridge.InvocationAck{}, commandbridge.ErrInvalidConfiguration
	}
	if err := a.validateCommandBridgeOwner(owner); err != nil {
		return commandbridge.InvocationAck{}, err
	}
	return bridge.InvokeIngress(a.commandBridgeContext(), invocation, owner)
}

func (a *App) CommandBridgeInput(input commandbridge.Input) (commandbridge.InvocationAck, error) {
	bridge, ok := loadCommandBridge(a)
	if !ok {
		return commandbridge.InvocationAck{}, commandbridge.ErrInvalidConfiguration
	}
	if err := a.validateCommandBridgeOwner(input.Owner); err != nil {
		return commandbridge.InvocationAck{}, err
	}
	return bridge.InputIngress(a.commandBridgeContext(), input)
}

func (a *App) CommandBridgeAcceptResult(result commandbridge.Result) (commandbridge.ResultAck, error) {
	bridge, ok := loadCommandBridge(a)
	if !ok {
		return commandbridge.ResultAck{}, commandbridge.ErrInvalidConfiguration
	}
	if err := a.validateCommandBridgeOwner(result.Owner); err != nil {
		return commandbridge.ResultAck{}, err
	}
	return bridge.AcceptResult(result)
}

func (a *App) CommandBridgeCancel(request commandbridge.CancelRequest) (commandbridge.CancelAck, error) {
	bridge, ok := loadCommandBridge(a)
	if !ok {
		return commandbridge.CancelAck{}, commandbridge.ErrInvalidConfiguration
	}
	if err := a.validateCommandBridgeOwner(request.Owner); err != nil {
		return commandbridge.CancelAck{}, err
	}
	return bridge.Cancel(a.commandBridgeContext(), request)
}

func (a *App) CommandBridgeLifecycle(event commandbridge.LifecycleEvent) error {
	bridge, ok := loadCommandBridge(a)
	if !ok {
		return commandbridge.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	current := a.currentAuthUser
	valid := current != nil && a.currentUserID != "" && current.UserID == a.currentUserID && event.SessionID == current.SessionID
	a.authMu.RUnlock()
	if !valid {
		return commandbridge.ErrSessionUnavailable
	}
	return bridge.LifecycleIngress(a.commandBridgeContext(), event)
}

func (a *App) shutdownCommandBridgeIfConfigured(ctx context.Context) error {
	if a == nil || ctx == nil {
		return commandbridge.ErrInvalidConfiguration
	}
	a.commandLifecycleMount.Lock()
	a.commandLifecycleClosing = true
	bridge := a.commandBridge.Load()
	a.commandLifecycleMount.Unlock()
	if bridge == nil {
		return nil
	}
	if err := bridge.Shutdown(ctx); err != nil {
		return err
	}
	if !a.commandBridge.CompareAndSwap(bridge, nil) {
		return errCommandBridgeAlreadyConfigured
	}
	return nil
}
