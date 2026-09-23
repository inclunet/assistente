package commanddeck

import (
	"context"
	"errors"
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	streamdeck "rafaelmartins.com/p/streamdeck"
)

type streamDeckDeviceFake struct {
	closeCalls atomic.Int32
	closeEnter chan struct{}
	closeOnce  sync.Once
	clearEnter chan struct{}
	clearGate  chan struct{}
	clearOnce  sync.Once
}

func (f *streamDeckDeviceFake) Close() error {
	f.closeCalls.Add(1)
	if f.closeEnter != nil {
		f.closeOnce.Do(func() { close(f.closeEnter) })
	}
	return nil
}

func (f *streamDeckDeviceFake) ClearKey(streamdeck.KeyID) error {
	if f.clearEnter != nil {
		f.clearOnce.Do(func() { close(f.clearEnter) })
		<-f.clearGate
	}
	return nil
}

func (f *streamDeckDeviceFake) SetKeyColor(streamdeck.KeyID, color.Color) error { return nil }

func (f *streamDeckDeviceFake) SetKeyImage(streamdeck.KeyID, image.Image) error { return nil }

func (f *streamDeckDeviceFake) GetKeyImageRectangle() (image.Rectangle, error) {
	return image.Rect(0, 0, 1, 1), nil
}

func newStreamDeckHandleFake(device streamDeckDevice, capacity int) *streamDeckHandle {
	return &streamDeckHandle{
		device:      device,
		events:      make(chan PhysicalKeyEvent, capacity),
		eventClosed: make(chan struct{}),
	}
}

func TestStreamDeckHandleFalhaLiberaEReadPriorizaTerminalSobreEventosBufferizados(t *testing.T) {
	device := &streamDeckDeviceFake{}
	handle := newStreamDeckHandleFake(device, 1)
	want := errors.New("falha HID")
	if err := handle.emit(PhysicalKeyEvent{Index: 1, Down: true}); err != nil {
		t.Fatal(err)
	}

	handle.fail(want)
	if got, err := handle.Read(context.Background()); !errors.Is(err, want) || got != (PhysicalKeyEvent{}) {
		t.Fatalf("falha terminal não priorizada: event=%+v err=%v", got, err)
	}
	if got := device.closeCalls.Load(); got != 0 {
		t.Fatalf("falha liberou o dispositivo dentro da callback: %d", got)
	}
	if err := handle.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := device.closeCalls.Load(); got != 1 {
		t.Fatalf("Close repetido liberou o dispositivo novamente: %d", got)
	}
}

func TestStreamDeckHandleOverflowFalhaFechadoSemDescartarRelease(t *testing.T) {
	device := &streamDeckDeviceFake{}
	handle := newStreamDeckHandleFake(device, 1)
	if err := handle.emit(PhysicalKeyEvent{Index: 0, Down: true}); err != nil {
		t.Fatal(err)
	}

	if err := handle.emit(PhysicalKeyEvent{Index: 0, Down: false}); !errors.Is(err, ErrStreamDeckEventOverflow) {
		t.Fatalf("overflow deveria falhar fechado: %v", err)
	}
	if _, err := handle.Read(context.Background()); !errors.Is(err, ErrStreamDeckEventOverflow) {
		t.Fatalf("Read não devolveu falha de overflow: %v", err)
	}
	if got := device.closeCalls.Load(); got != 0 {
		t.Fatalf("overflow liberou o dispositivo dentro da callback: %d", got)
	}
	if err := handle.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := device.closeCalls.Load(); got != 1 {
		t.Fatalf("Close após overflow não liberou o dispositivo exatamente uma vez: %d", got)
	}
}

func TestStreamDeckHandleCloseConcorrenteEEmitNaoEnviamEmCanalFechado(t *testing.T) {
	device := &streamDeckDeviceFake{}
	handle := newStreamDeckHandleFake(device, 4)
	var emitters sync.WaitGroup
	for i := 0; i < 32; i++ {
		emitters.Add(1)
		go func(index int) {
			defer emitters.Done()
			for j := 0; j < 32; j++ {
				_ = handle.emit(PhysicalKeyEvent{Index: index, Down: true})
			}
		}(i)
	}
	var closers sync.WaitGroup
	for i := 0; i < 32; i++ {
		closers.Add(1)
		go func() {
			defer closers.Done()
			_ = handle.Close(context.Background())
		}()
	}
	emitters.Wait()
	closers.Wait()
	if got := device.closeCalls.Load(); got != 1 {
		t.Fatalf("Close concorrente liberou %d vezes", got)
	}
}

func TestStreamDeckHandleWriteECloseSaoSerializadosNoDispositivo(t *testing.T) {
	device := &streamDeckDeviceFake{clearEnter: make(chan struct{}), clearGate: make(chan struct{})}
	handle := newStreamDeckHandleFake(device, 1)
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- handle.Write(context.Background(), RenderPlan{Updates: []KeyUpdate{{Index: 0}}})
	}()
	select {
	case <-device.clearEnter:
	case <-time.After(time.Second):
		t.Fatal("Write não chegou ao dispositivo fake")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- handle.Close(context.Background()) }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close liberou durante Write: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(device.clearGate)
	if err := <-writeDone; err != nil {
		t.Fatalf("Write após concluir operação nativa = %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if got := device.closeCalls.Load(); got != 1 {
		t.Fatalf("liberação não foi única: %d", got)
	}
}

func TestStreamDeckHandleReadCanceladoNaoEntregaEventoBufferizado(t *testing.T) {
	device := &streamDeckDeviceFake{}
	handle := newStreamDeckHandleFake(device, 1)
	if err := handle.emit(PhysicalKeyEvent{Index: 2, Down: true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := handle.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read cancelado entregou evento bufferizado: %v", err)
	}
}

func TestStreamDeckHandleCloseRespeitaCancelamentoDoJoin(t *testing.T) {
	device := &streamDeckDeviceFake{closeEnter: make(chan struct{})}
	handle := newStreamDeckHandleFake(device, 1)
	handle.listenDone = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	closeDone := make(chan error, 1)
	go func() { closeDone <- handle.Close(ctx) }()
	select {
	case <-device.closeEnter:
	case <-time.After(time.Second):
		t.Fatal("Close não iniciou a liberação nativa")
	}
	cancel()
	select {
	case err := <-closeDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Close sem erro de cancelamento do join: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close não respeitou cancelamento do join")
	}
	if got := device.closeCalls.Load(); got != 1 {
		t.Fatalf("liberação nativa não foi iniciada exatamente uma vez: %d", got)
	}
}

func TestStreamDeckDriverValidaCancelamentoAntesDaAPIHID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	driver := NewStreamDeckDriver()
	if _, err := driver.Enumerate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Enumerate cancelado = %v", err)
	}
	if _, err := driver.Open(ctx, PhysicalDevice{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open cancelado = %v", err)
	}
}
