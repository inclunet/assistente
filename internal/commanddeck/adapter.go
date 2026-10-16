package commanddeck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

var ErrInvalidKey = errors.New("tecla de deck inválida")

type Controller interface {
	Input(context.Context, commandadapter.Event) (commandbridge.InvocationAck, error)
	Lock(context.Context) error
	Logout(context.Context) error
}

// DeviceAdapter é a borda entre um futuro driver HID e o controller físico
// genérico. Ele não abre USB/HID nem executa handlers finais.
type DeviceAdapter struct {
	manager    *Manager
	controller Controller
}

func NewDeviceAdapter(manager *Manager, controller Controller) (*DeviceAdapter, error) {
	if manager == nil || controller == nil {
		return nil, ErrInvalidDevice
	}
	return &DeviceAdapter{manager: manager, controller: controller}, nil
}

func (a *DeviceAdapter) Open(device DeviceID, model Model) (RenderPlan, error) {
	return a.manager.Open(device, model)
}

func (a *DeviceAdapter) Activate(device DeviceID, generation uint64) error {
	return a.manager.Activate(device, generation)
}

func (a *DeviceAdapter) Render(frame Frame) (RenderPlan, error) {
	return a.manager.Render(frame)
}

func (a *DeviceAdapter) Key(ctx context.Context, device DeviceID, index int, down bool, repeat bool) (commandbridge.InvocationAck, error) {
	if ctx == nil {
		return commandbridge.InvocationAck{}, ErrInvalidKey
	}
	snapshot, err := a.manager.Snapshot(device)
	if err != nil {
		return commandbridge.InvocationAck{}, err
	}
	if snapshot.Status != DeviceConnected {
		return commandbridge.InvocationAck{}, ErrDeviceSafe
	}
	if index < 0 || index >= snapshot.Model.KeyCount() || repeat && !down {
		return commandbridge.InvocationAck{}, ErrInvalidKey
	}
	kind := commandinput.KeyUp
	if down {
		kind = commandinput.KeyDown
	}
	return a.controller.Input(ctx, commandadapter.Event{
		SourceInstance: sourceInstance(device),
		Key:            keyName(index),
		Kind:           kind,
		Repeat:         repeat,
	})
}

func (a *DeviceAdapter) Lock(ctx context.Context) ([]RenderPlan, error) {
	if ctx == nil {
		return nil, ErrInvalidDevice
	}
	plans := a.manager.LockOrLogout()
	return plans, a.controller.Lock(ctx)
}

func (a *DeviceAdapter) Logout(ctx context.Context) ([]RenderPlan, error) {
	if ctx == nil {
		return nil, ErrInvalidDevice
	}
	plans := a.manager.LockOrLogout()
	return plans, a.controller.Logout(ctx)
}

func (a *DeviceAdapter) Disconnect(device DeviceID) (time.Duration, error) {
	return a.manager.Disconnect(device)
}

func (a *DeviceAdapter) CanReconnect(device DeviceID) bool {
	return a.manager.CanReconnect(device)
}

func sourceInstance(device DeviceID) string {
	return "streamdeck.key:" + strings.TrimSpace(string(device))
}

func keyName(index int) string {
	return fmt.Sprintf("key:%d", index)
}
