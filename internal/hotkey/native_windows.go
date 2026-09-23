//go:build windows

package hotkey

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"assistente/internal/logging"
	"golang.design/x/hotkey"
	"golang.org/x/sys/windows"
)

const (
	windowsHotkeyID         int32  = 1 // Identidade local da thread exclusiva desta inscrição.
	windowsNoRepeat         uint32 = 0x4000
	windowsWMHotkey                = 0x0312
	windowsWMQuit                  = 0x0012
	windowsHotkeyWaitMillis        = 50
)

// Todas as operações desta porta pertencem à mesma thread nativa. Wait deve
// retornar em no máximo windowsHotkeyWaitMillis, sem depender do release.
type windowsHotkeyAPI interface {
	Register(id int32, modifiers, key uint32) error
	Unregister(id int32) error
	Held(key uint32) bool
	Next() (id int32, present bool, err error)
	Wait() error
}

type windowsHotkey struct {
	api       windowsHotkeyAPI
	modifiers uint32
	key       uint32
	mu        sync.Mutex
	started   bool
	stopOnce  sync.Once
	stop      chan struct{}
	done      chan struct{}
	down      chan hotkey.Event
	err       error // Publicado pelo fechamento de done.
}

func newNativeHotkey(modifiers []hotkey.Modifier, key hotkey.Key) nativeHotkey {
	return newWindowsHotkey(modifiers, key, win32HotkeyAPI{})
}

func newWindowsHotkey(modifiers []hotkey.Modifier, key hotkey.Key, api windowsHotkeyAPI) *windowsHotkey {
	bits := windowsNoRepeat
	for _, modifier := range modifiers {
		bits |= uint32(modifier)
	}
	return &windowsHotkey{api: api, modifiers: bits, key: uint32(key),
		stop: make(chan struct{}), done: make(chan struct{}), down: make(chan hotkey.Event)}
}

func (h *windowsHotkey) Keydown() <-chan hotkey.Event { return h.down }

func (h *windowsHotkey) Register() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started || h.api == nil {
		return errors.New("hotkey: invalid or already used Windows registration")
	}
	h.started = true
	ready := make(chan error, 1)
	go h.run(ready)
	return <-ready
}

func (h *windowsHotkey) Unregister() error {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return errors.New("hotkey: Windows registration was not started")
	}
	h.stopOnce.Do(func() { close(h.stop) })
	h.mu.Unlock()
	<-h.done
	return h.err
}

func (h *windowsHotkey) run(ready chan<- error) {
	runtime.LockOSThread()
	// Não devolvemos esta thread ao pool: sua fila pode conter mensagens antigas.
	// Ao sair ainda locked, o runtime encerra a thread e a fila da inscrição.
	registered := false
	defer func() {
		if registered {
			h.err = errors.Join(h.err, h.api.Unregister(windowsHotkeyID))
			if h.err != nil {
				logging.Warnf(context.Background(), "hotkey.windows", "Registro nativo encerrado com erro: %v", h.err)
			}
		}
		close(h.down)
		close(h.done)
	}()
	if h.key == 0 || h.key > 0xFE || h.modifiers & ^uint32(0x400F) != 0 {
		h.err = errors.New("hotkey: invalid Windows key or modifiers")
		ready <- h.err
		return
	}
	if h.err = h.api.Register(windowsHotkeyID, h.modifiers, h.key); h.err != nil {
		ready <- h.err
		return
	}
	registered = true
	// A key already held when RegisterHotKey succeeds must not synthesize an
	// occurrence. Registration is ready immediately; the pump drains any
	// queued WM_HOTKEY messages and arms only after the queue is empty and the
	// physical key is released. Once armed, every native delivery is preserved;
	// MOD_NOREPEAT remains the OS repeat policy for the registration.
	armed := !h.api.Held(h.key)
	ready <- nil
	for {
		select {
		case <-h.stop:
			return
		default:
		}
		id, present, err := h.api.Next()
		if err != nil {
			h.err = err
			return
		}
		if present {
			if id != windowsHotkeyID {
				continue
			}
			if !armed {
				continue
			}
			// A entrega também é cancelável: um job demorado não prende teardown.
			select {
			case <-h.stop:
				return
			case h.down <- hotkey.Event{}:
			}
			continue
		}
		if !armed && !h.api.Held(h.key) {
			armed = true
		}
		if err := h.api.Wait(); err != nil {
			h.err = err
			return
		}
	}
}

type win32HotkeyAPI struct{}

var (
	hotkeyUser32          = windows.NewLazySystemDLL("user32.dll")
	registerHotkeyProc    = hotkeyUser32.NewProc("RegisterHotKey")
	unregisterHotkeyProc  = hotkeyUser32.NewProc("UnregisterHotKey")
	getAsyncKeyStateProc  = hotkeyUser32.NewProc("GetAsyncKeyState")
	peekHotkeyMessageProc = hotkeyUser32.NewProc("PeekMessageW")
	waitHotkeyMessageProc = hotkeyUser32.NewProc("MsgWaitForMultipleObjectsEx")
)

type windowsHotkeyMessage struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	x, y    int32
	private uint32
}

func (win32HotkeyAPI) Register(id int32, modifiers, key uint32) error {
	version := windows.RtlGetVersion()
	if version.MajorVersion < 6 || version.MajorVersion == 6 && version.MinorVersion < 1 {
		return errors.New("hotkey: Windows version does not support MOD_NOREPEAT")
	}
	ok, _, err := registerHotkeyProc.Call(0, uintptr(id), uintptr(modifiers), uintptr(key))
	if ok == 0 {
		return fmt.Errorf("RegisterHotKey: %w", err)
	}
	return nil
}

func (win32HotkeyAPI) Unregister(id int32) error {
	ok, _, err := unregisterHotkeyProc.Call(0, uintptr(id))
	if ok == 0 {
		return fmt.Errorf("UnregisterHotKey: %w", err)
	}
	return nil
}

func (win32HotkeyAPI) Held(key uint32) bool {
	state, _, _ := getAsyncKeyStateProc.Call(uintptr(key))
	return state&0x8000 != 0
}

func (win32HotkeyAPI) Next() (int32, bool, error) {
	var message windowsHotkeyMessage
	// HWND(-1): somente mensagens da thread, sem executar callbacks de janelas.
	present, _, _ := peekHotkeyMessageProc.Call(uintptr(unsafe.Pointer(&message)), ^uintptr(0), windowsWMHotkey, windowsWMHotkey, 1)
	if present == 0 {
		return 0, false, nil
	}
	if message.message == windowsWMQuit {
		return 0, false, errors.New("hotkey: native thread quit")
	}
	if message.message != windowsWMHotkey {
		return 0, false, nil
	}
	return int32(message.wParam), true, nil
}

func (win32HotkeyAPI) Wait() error {
	// QS_HOTKEY e MWMO_INPUTAVAILABLE. A espera termina imediatamente com input;
	// 50ms é o teto para observar cancelamento ocioso, não latência da hotkey.
	result, _, err := waitHotkeyMessageProc.Call(0, 0, windowsHotkeyWaitMillis, 0x0080, 0x0004)
	if uint32(result) == 0xFFFFFFFF {
		return fmt.Errorf("MsgWaitForMultipleObjectsEx: %w", err)
	}
	if result != 0 && result != 0x102 {
		return fmt.Errorf("hotkey: unexpected wait result %d", result)
	}
	return nil
}
