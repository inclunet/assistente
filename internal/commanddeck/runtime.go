package commanddeck

import (
	"context"
	"errors"
)

var (
	ErrDriverUnavailable = errors.New("driver de deck indisponível")
	ErrRuntimeClosed     = errors.New("runtime de deck encerrado")
)

type PhysicalDevice struct {
	ID    DeviceID
	Model Model
}

type PhysicalKeyEvent struct {
	Index  int
	Down   bool
	Repeat bool
}

// Driver é o contrato mínimo para uma biblioteca HID futura. Implementações
// concretas ficam fora deste pacote até validação de licença/build/modelos.
type Driver interface {
	Enumerate(context.Context) ([]PhysicalDevice, error)
	Open(context.Context, PhysicalDevice) (Handle, error)
}

type Handle interface {
	Write(context.Context, RenderPlan) error
	Read(context.Context) (PhysicalKeyEvent, error)
	Close(context.Context) error
}

type Runtime struct {
	driver  Driver
	adapter *DeviceAdapter
	handles map[DeviceID]Handle
	closed  bool
}

func NewRuntime(driver Driver, adapter *DeviceAdapter) (*Runtime, error) {
	if driver == nil || adapter == nil {
		return nil, ErrDriverUnavailable
	}
	return &Runtime{driver: driver, adapter: adapter, handles: make(map[DeviceID]Handle)}, nil
}

// Discover enumera e tenta abrir cada dispositivo. Falhas individuais não
// impedem a inicialização do app; o chamador observa os erros retornados.
func (r *Runtime) Discover(ctx context.Context) []error {
	if r == nil || ctx == nil {
		return []error{ErrDriverUnavailable}
	}
	if r.closed {
		return []error{ErrRuntimeClosed}
	}
	devices, err := r.driver.Enumerate(ctx)
	if err != nil {
		return []error{err}
	}
	errs := make([]error, 0)
	for _, device := range devices {
		if err := r.openOne(ctx, device); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (r *Runtime) openOne(ctx context.Context, device PhysicalDevice) error {
	if _, exists := r.handles[device.ID]; exists {
		return ErrDeviceAlreadyOpen
	}
	handle, err := r.driver.Open(ctx, device)
	if err != nil {
		return err
	}
	plan, err := r.adapter.Open(device.ID, device.Model)
	if err != nil {
		_ = handle.Close(ctx)
		return err
	}
	if err := handle.Write(ctx, plan); err != nil {
		_ = handle.Close(ctx)
		return err
	}
	snapshot, err := r.adapter.manager.Snapshot(device.ID)
	if err != nil {
		_ = handle.Close(ctx)
		return err
	}
	if err := r.adapter.Activate(device.ID, snapshot.Generation); err != nil {
		_ = handle.Close(ctx)
		return err
	}
	r.handles[device.ID] = handle
	return nil
}

// PollOne lê um evento de um dispositivo já aberto e o encaminha ao adapter.
// Se o handle falha, o dispositivo é desconectado e removido do runtime.
func (r *Runtime) PollOne(ctx context.Context, device DeviceID) error {
	if r == nil || ctx == nil {
		return ErrDriverUnavailable
	}
	if r.closed {
		return ErrRuntimeClosed
	}
	handle, ok := r.handles[device]
	if !ok {
		return ErrInvalidDevice
	}
	event, err := handle.Read(ctx)
	if err != nil {
		_, _ = r.adapter.Disconnect(device)
		_ = handle.Close(ctx)
		delete(r.handles, device)
		return err
	}
	_, err = r.adapter.Key(ctx, device, event.Index, event.Down, event.Repeat)
	return err
}

func (r *Runtime) Render(ctx context.Context, frame Frame) error {
	if r == nil || ctx == nil {
		return ErrDriverUnavailable
	}
	if r.closed {
		return ErrRuntimeClosed
	}
	handle, ok := r.handles[frame.Device]
	if !ok {
		return ErrInvalidDevice
	}
	plan, err := r.adapter.Render(frame)
	if err != nil {
		return err
	}
	if len(plan.Updates) == 0 && !plan.FullFrame {
		return nil
	}
	return handle.Write(ctx, plan)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || ctx == nil {
		return ErrDriverUnavailable
	}
	if r.closed {
		return nil
	}
	r.closed = true
	var first error
	plansByDevice := make(map[DeviceID]RenderPlan, len(r.handles))
	if plans, err := r.adapter.Logout(ctx); err == nil {
		for _, plan := range plans {
			plansByDevice[plan.Device] = plan
		}
	} else if first == nil {
		first = err
	}
	for device, handle := range r.handles {
		if plan, ok := plansByDevice[device]; ok {
			if err := handle.Write(ctx, plan); err != nil && first == nil {
				first = err
			}
		}
		if err := handle.Close(ctx); err != nil && first == nil {
			first = err
		}
		delete(r.handles, device)
	}
	return first
}
