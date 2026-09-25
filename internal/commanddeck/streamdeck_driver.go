package commanddeck

import (
	"context"
	"errors"
	"image"
	"image/color"
	"sync"

	streamdeck "rafaelmartins.com/p/streamdeck"
)

var ErrStreamDeckEventOverflow = errors.New("buffer de eventos do stream deck cheio")

type StreamDeckDriver struct{}

func NewStreamDeckDriver() *StreamDeckDriver {
	return &StreamDeckDriver{}
}

func (d *StreamDeckDriver) Enumerate(ctx context.Context) ([]PhysicalDevice, error) {
	if ctx == nil {
		return nil, ErrDriverUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	devices, err := streamdeck.Enumerate()
	if err != nil {
		return nil, err
	}
	result := make([]PhysicalDevice, 0, len(devices))
	for _, device := range devices {
		model := modelFromStreamDeck(device)
		if err := model.validate(); err != nil {
			return nil, err
		}
		result = append(result, PhysicalDevice{ID: DeviceID(device.GetSerialNumber()), Model: model})
	}
	return result, nil
}

func (d *StreamDeckDriver) Open(ctx context.Context, device PhysicalDevice) (Handle, error) {
	if ctx == nil {
		return nil, ErrDriverUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dev, err := streamdeck.GetDevice(string(device.ID))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := dev.Open(); err != nil {
		return nil, err
	}
	eventCapacity := int(device.Model.KeyCount()) * 2
	if eventCapacity < 1 {
		eventCapacity = 1
	}
	handle := &streamDeckHandle{
		device:      dev,
		events:      make(chan PhysicalKeyEvent, eventCapacity),
		eventClosed: make(chan struct{}),
		listenDone:  make(chan struct{}),
	}
	if err := dev.ForEachKey(func(key streamdeck.KeyID) error {
		return dev.AddKeyHandler(key, func(_ *streamdeck.Device, pressed *streamdeck.Key) error {
			index := int(pressed.GetID()) - 1
			if err := handle.emit(PhysicalKeyEvent{Index: index, Down: true}); err != nil {
				return err
			}
			go func() {
				pressed.WaitForRelease()
				_ = handle.emit(PhysicalKeyEvent{Index: index, Down: false})
			}()
			return nil
		})
	}); err != nil {
		_ = dev.Close()
		return nil, err
	}
	go func() {
		defer close(handle.listenDone)
		errCh := make(chan error, 1)
		errWatchDone := make(chan struct{})
		go func() {
			defer close(errWatchDone)
			for {
				select {
				case err := <-errCh:
					if err != nil {
						handle.fail(err)
					}
				case <-handle.eventClosed:
					return
				}
			}
		}()
		if err := handle.terminalError(); err != nil {
			<-errWatchDone
			return
		}
		listenErr := dev.Listen(errCh)
		if listenErr != nil {
			handle.fail(listenErr)
		} else if handle.terminalError() == nil {
			handle.fail(ErrInvalidDevice)
		}
		<-errWatchDone
	}()
	return handle, nil
}

type streamDeckHandle struct {
	mu          sync.Mutex
	ioMu        sync.Mutex
	device      streamDeckDevice
	events      chan PhysicalKeyEvent
	eventClosed chan struct{}
	listenDone  chan struct{}
	releaseOnce sync.Once
	releaseErr  error
	err         error
	closed      bool
}

type streamDeckDevice interface {
	Close() error
	ClearKey(streamdeck.KeyID) error
	SetKeyColor(streamdeck.KeyID, color.Color) error
	SetKeyImage(streamdeck.KeyID, image.Image) error
	GetKeyImageRectangle() (image.Rectangle, error)
}

func (h *streamDeckHandle) Write(ctx context.Context, plan RenderPlan) error {
	if ctx == nil {
		return ErrDriverUnavailable
	}
	h.ioMu.Lock()
	defer h.ioMu.Unlock()
	if err := h.terminalError(); err != nil {
		return err
	}
	for _, update := range plan.Updates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := h.terminalError(); err != nil {
			return err
		}
		key := streamdeck.KeyID(update.Index + 1)
		if update.View.ImageID == "" || len(update.View.ImageRGBA) == 0 {
			if err := h.device.ClearKey(key); err != nil {
				return err
			}
			continue
		}
		img, ok := h.imageFromKeyView(update.View)
		if !ok {
			if err := h.device.SetKeyColor(key, color.RGBA{R: 0, G: 0, B: 0, A: 255}); err != nil {
				return err
			}
			continue
		}
		if err := h.device.SetKeyImage(key, img); err != nil {
			return err
		}
	}
	return nil
}

func (h *streamDeckHandle) Read(ctx context.Context) (PhysicalKeyEvent, error) {
	if ctx == nil {
		return PhysicalKeyEvent{}, ErrDriverUnavailable
	}
	for {
		if err := ctx.Err(); err != nil {
			return PhysicalKeyEvent{}, err
		}
		if err := h.terminalError(); err != nil {
			return PhysicalKeyEvent{}, err
		}
		select {
		case event := <-h.events:
			if err := ctx.Err(); err != nil {
				return PhysicalKeyEvent{}, err
			}
			if err := h.terminalError(); err != nil {
				return PhysicalKeyEvent{}, err
			}
			return event, nil
		case <-h.eventClosed:
			if err := ctx.Err(); err != nil {
				return PhysicalKeyEvent{}, err
			}
			if err := h.terminalError(); err != nil {
				return PhysicalKeyEvent{}, err
			}
			return PhysicalKeyEvent{}, ErrInvalidDevice
		case <-ctx.Done():
			return PhysicalKeyEvent{}, ctx.Err()
		}
	}
}

func (h *streamDeckHandle) Close(ctx context.Context) error {
	h.terminate(ErrInvalidDevice)
	err := h.release()
	if h.listenDone != nil {
		if ctx == nil {
			<-h.listenDone
		} else {
			select {
			case <-h.listenDone:
			case <-ctx.Done():
				return errors.Join(err, ctx.Err())
			}
		}
	}
	return err
}

func (h *streamDeckHandle) emit(event PhysicalKeyEvent) error {
	select {
	case <-h.eventClosed:
		return h.terminalError()
	case h.events <- event:
		return h.terminalError()
	default:
		h.fail(ErrStreamDeckEventOverflow)
		return h.terminalError()
	}
}

func (h *streamDeckHandle) fail(err error) {
	if err == nil {
		err = ErrInvalidDevice
	}
	h.terminate(err)
}

func (h *streamDeckHandle) terminate(err error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.err = err
	h.closed = true
	close(h.eventClosed)
	h.mu.Unlock()
}

func (h *streamDeckHandle) terminalError() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.err != nil {
		return h.err
	}
	if h.closed {
		return ErrInvalidDevice
	}
	return nil
}

func (h *streamDeckHandle) release() error {
	h.releaseOnce.Do(func() {
		h.ioMu.Lock()
		h.releaseErr = h.device.Close()
		h.ioMu.Unlock()
	})
	return h.releaseErr
}

func modelFromStreamDeck(device *streamdeck.Device) Model {
	keys := int(device.GetKeyCount())
	rows, columns := geometryForKeyCount(keys)
	width, height := 72, 72
	if rect, err := device.GetKeyImageRectangle(); err == nil && rect.Dx() > 0 && rect.Dy() > 0 {
		width, height = rect.Dx(), rect.Dy()
	}
	return Model{
		ID:          device.GetModelID(),
		Name:        device.GetModelName(),
		Rows:        rows,
		Columns:     columns,
		KeyImageW:   width,
		KeyImageH:   height,
		SupportsHID: true,
	}
}

func geometryForKeyCount(keys int) (int, int) {
	switch keys {
	case 6:
		return 2, 3
	case 8:
		return 2, 4
	case 15:
		return 3, 5
	case 32:
		return 4, 8
	default:
		if keys <= 0 {
			return 1, 1
		}
		return 1, keys
	}
}

func (h *streamDeckHandle) imageFromKeyView(view KeyView) (image.Image, bool) {
	if len(view.ImageRGBA) == 0 {
		return nil, false
	}
	rect, err := h.device.GetKeyImageRectangle()
	if err != nil || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return nil, false
	}
	if rect.Dx()*rect.Dy()*4 != len(view.ImageRGBA) {
		return nil, false
	}
	img := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	copy(img.Pix, view.ImageRGBA)
	return img, true
}
