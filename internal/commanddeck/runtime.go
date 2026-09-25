package commanddeck

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrDriverUnavailable = errors.New("driver de deck indisponível")
	ErrRuntimeClosed     = errors.New("runtime de deck encerrado")
	ErrReconnectBackoff  = errors.New("dispositivo de deck aguardando reconexão")
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

// Driver possui os handles físicos. Read/Write/Open devem observar o contexto;
// Close precisa liberar o recurso mesmo se uma leitura anterior falhou.
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
	mu          sync.Mutex
	discover    sync.Mutex
	driver      Driver
	adapter     *DeviceAdapter
	handles     map[DeviceID]*runtimeDevice
	closed      bool
	lifetime    context.Context
	cancel      context.CancelFunc
	workers     sync.WaitGroup
	done        chan struct{}
	shutdownErr error
}

type runtimeDevice struct {
	mu       sync.Mutex // escrita, entrega e aposentadoria; nunca inclui Read
	readMu   sync.Mutex
	handle   Handle
	lifetime context.Context
	cancel   context.CancelFunc
	retired  bool
}

type DiscoverResult struct {
	Device DeviceID
	Model  Model
	Opened bool
	Err    error
}

func NewRuntime(driver Driver, adapter *DeviceAdapter) (*Runtime, error) {
	if driver == nil || adapter == nil {
		return nil, ErrDriverUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runtime{driver: driver, adapter: adapter, handles: make(map[DeviceID]*runtimeDevice), lifetime: ctx, cancel: cancel, done: make(chan struct{})}, nil
}

func (r *Runtime) begin(ctx context.Context) (context.Context, func(), error) {
	if r == nil || ctx == nil {
		return nil, nil, ErrDriverUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, nil, ErrRuntimeClosed
	}
	r.workers.Add(1)
	operation, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.lifetime, cancel)
	return operation, func() { stop(); cancel(); r.workers.Done() }, nil
}

func (r *Runtime) device(id DeviceID) (*runtimeDevice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrRuntimeClosed
	}
	entry := r.handles[id]
	if entry == nil {
		return nil, ErrInvalidDevice
	}
	return entry, nil
}

// Discover enumera e tenta abrir cada dispositivo. Falhas individuais não
// impedem a inicialização do app; o chamador observa os erros retornados.
func (r *Runtime) Discover(ctx context.Context) []error {
	results := r.DiscoverDetailed(ctx)
	errs := make([]error, 0, len(results))
	for _, result := range results {
		if result.Err != nil {
			errs = append(errs, result.Err)
		}
	}
	return errs
}

func (r *Runtime) DiscoverDetailed(ctx context.Context) []DiscoverResult {
	ctx, finish, err := r.begin(ctx)
	if err != nil {
		return []DiscoverResult{{Err: err}}
	}
	defer finish()
	r.discover.Lock()
	defer r.discover.Unlock()
	if err := ctx.Err(); err != nil {
		return []DiscoverResult{{Err: err}}
	}
	devices, err := r.driver.Enumerate(ctx)
	if err != nil {
		return []DiscoverResult{{Err: err}}
	}
	results := make([]DiscoverResult, 0, len(devices))
	for _, device := range devices {
		if err := r.openOne(ctx, device); err != nil {
			results = append(results, DiscoverResult{Device: device.ID, Model: device.Model, Err: err})
			continue
		}
		results = append(results, DiscoverResult{Device: device.ID, Model: device.Model, Opened: true})
	}
	return results
}

func (r *Runtime) openOne(ctx context.Context, device PhysicalDevice) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	_, exists := r.handles[device.ID]
	r.mu.Unlock()
	if exists {
		return ErrDeviceAlreadyOpen
	}
	if snapshot, err := r.adapter.manager.Snapshot(device.ID); err == nil && snapshot.Status == DeviceDisconnected && !r.adapter.CanReconnect(device.ID) {
		return ErrReconnectBackoff
	}
	handle, err := r.driver.Open(ctx, device)
	if err != nil {
		return err
	}
	if handle == nil {
		return ErrDriverUnavailable
	}
	owned := false
	published := false
	defer func() {
		if published {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if owned {
			_, err := r.adapter.Disconnect(device.ID)
			result = errors.Join(result, err)
		}
		result = errors.Join(result, handle.Close(cleanup))
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	plan, err := r.adapter.Open(device.ID, device.Model)
	if err != nil {
		return err
	}
	owned = true
	snapshot, err := r.adapter.manager.Snapshot(device.ID)
	if err != nil {
		return err
	}
	// Capturar ANTES do I/O: lock/logout durante a escrita não autoriza ativar
	// a geração nova com o frame preparado para a geração antiga.
	if err := handle.Write(ctx, plan); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.adapter.Activate(device.ID, snapshot.Generation); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lifetime, cancel := context.WithCancel(r.lifetime)
	r.handles[device.ID] = &runtimeDevice{handle: handle, lifetime: lifetime, cancel: cancel}
	published = true
	return nil
}

// PollOne lê um evento de um dispositivo já aberto e o encaminha ao adapter.
// Se o handle falha, o dispositivo é desconectado e removido do runtime.
func (r *Runtime) PollOne(ctx context.Context, device DeviceID) error {
	ctx, finish, err := r.begin(ctx)
	if err != nil {
		return err
	}
	defer finish()
	entry, err := r.device(device)
	if err != nil {
		return err
	}
	readCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(entry.lifetime, cancel)
	defer func() { stop(); cancel() }()
	entry.readMu.Lock()
	defer entry.readMu.Unlock()
	if err := readCtx.Err(); err != nil {
		return err
	}
	before, err := r.adapter.manager.Snapshot(device)
	if err != nil {
		return err
	}
	event, err := entry.handle.Read(readCtx)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if entry.retired {
		return ErrInvalidDevice
	}
	if err != nil {
		return errors.Join(err, r.retireLocked(device, entry))
	}
	if _, err := r.device(device); err != nil {
		return err
	}
	after, err := r.adapter.manager.Snapshot(device)
	if err != nil {
		return err
	}
	if before.Generation != after.Generation || before.Status != DeviceConnected || after.Status != DeviceConnected {
		return ErrDeviceSafe
	}
	_, err = r.adapter.Key(ctx, device, event.Index, event.Down, event.Repeat)
	return err
}

// entry.mu está adquirido. A identidade do handle impede callbacks antigos
// de desconectar uma abertura posterior do mesmo serial.
func (r *Runtime) retireLocked(device DeviceID, entry *runtimeDevice) error {
	if entry.retired {
		return nil
	}
	entry.retired = true
	entry.cancel()
	_, disconnectErr := r.adapter.Disconnect(device)
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closeErr := entry.handle.Close(cleanup)
	r.mu.Lock()
	if r.handles[device] == entry {
		delete(r.handles, device)
	}
	r.mu.Unlock()
	return errors.Join(disconnectErr, closeErr)
}

func (r *Runtime) Render(ctx context.Context, frame Frame) error {
	ctx, finish, err := r.begin(ctx)
	if err != nil {
		return err
	}
	defer finish()
	entry, err := r.device(frame.Device)
	if err != nil {
		return err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.retired {
		return ErrInvalidDevice
	}
	plan, err := r.adapter.Render(frame)
	if err != nil {
		return err
	}
	if len(plan.Updates) == 0 && !plan.FullFrame {
		return nil
	}
	if err := entry.handle.Write(ctx, plan); err != nil {
		// O diff foi calculado, mas o frame pode estar parcialmente escrito.
		// Desconectar força frame completo e nova geração antes de reutilizar.
		return errors.Join(err, r.retireLocked(frame.Device, entry))
	}
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || ctx == nil {
		return ErrDriverUnavailable
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.cancel()
		go r.closeDevices()
	}
	r.mu.Unlock()
	select {
	case <-r.done:
		return r.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) closeDevices() {
	r.workers.Wait()
	// Admissão fechada e todos os produtores encerrados: o mapa é exclusivo.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var failures []error
	plansByDevice := make(map[DeviceID]RenderPlan, len(r.handles))
	if plans, err := r.adapter.Logout(ctx); err == nil {
		for _, plan := range plans {
			plansByDevice[plan.Device] = plan
		}
	} else {
		failures = append(failures, err)
	}
	for device, entry := range r.handles {
		if plan, ok := plansByDevice[device]; ok {
			if err := entry.handle.Write(ctx, plan); err != nil {
				failures = append(failures, err)
			}
		}
		entry.cancel()
		failures = append(failures, entry.handle.Close(ctx))
		_, err := r.adapter.Disconnect(device)
		failures = append(failures, err)
		delete(r.handles, device)
	}
	r.shutdownErr = errors.Join(failures...)
	close(r.done)
}
