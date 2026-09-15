package commanddeck

import (
	"context"
	"image"
	"image/color"
	"sync"

	streamdeck "rafaelmartins.com/p/streamdeck"
)

type StreamDeckDriver struct{}

func NewStreamDeckDriver() *StreamDeckDriver {
	return &StreamDeckDriver{}
}

func (d *StreamDeckDriver) Enumerate(context.Context) ([]PhysicalDevice, error) {
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
	dev, err := streamdeck.GetDevice(string(device.ID))
	if err != nil {
		return nil, err
	}
	if err := dev.Open(); err != nil {
		return nil, err
	}
	handle := &streamDeckHandle{device: dev, events: make(chan PhysicalKeyEvent, int(device.Model.KeyCount())*2)}
	if err := dev.ForEachKey(func(key streamdeck.KeyID) error {
		return dev.AddKeyHandler(key, func(_ *streamdeck.Device, pressed *streamdeck.Key) error {
			index := int(pressed.GetID()) - 1
			handle.emit(PhysicalKeyEvent{Index: index, Down: true})
			go func() {
				pressed.WaitForRelease()
				handle.emit(PhysicalKeyEvent{Index: index, Down: false})
			}()
			return nil
		})
	}); err != nil {
		_ = dev.Close()
		return nil, err
	}
	go func() {
		errCh := make(chan error, 1)
		if err := dev.Listen(errCh); err != nil {
			handle.fail(err)
			return
		}
		if err := <-errCh; err != nil {
			handle.fail(err)
		}
	}()
	return handle, nil
}

type streamDeckHandle struct {
	mu     sync.Mutex
	device *streamdeck.Device
	events chan PhysicalKeyEvent
	err    error
	closed bool
}

func (h *streamDeckHandle) Write(ctx context.Context, plan RenderPlan) error {
	if ctx == nil {
		return ErrDriverUnavailable
	}
	for _, update := range plan.Updates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
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
	select {
	case event, ok := <-h.events:
		if !ok {
			if h.err != nil {
				return PhysicalKeyEvent{}, h.err
			}
			return PhysicalKeyEvent{}, ErrInvalidDevice
		}
		return event, nil
	case <-ctx.Done():
		return PhysicalKeyEvent{}, ctx.Err()
	}
}

func (h *streamDeckHandle) Close(context.Context) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	close(h.events)
	device := h.device
	h.mu.Unlock()
	return device.Close()
}

func (h *streamDeckHandle) emit(event PhysicalKeyEvent) {
	h.mu.Lock()
	closed := h.closed
	h.mu.Unlock()
	if closed {
		return
	}
	select {
	case h.events <- event:
	default:
	}
}

func (h *streamDeckHandle) fail(err error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.err = err
	h.closed = true
	close(h.events)
	h.mu.Unlock()
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
