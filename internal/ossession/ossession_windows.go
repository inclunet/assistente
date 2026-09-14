//go:build windows

package ossession

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	wmDestroy            = 0x0002
	wmWtsSessionChange   = 0x02B1
	wmQuit               = 0x0012
	pmNoRemove           = 0x0000
	pmRemove             = 0x0001
	hwndMessage          = ^uintptr(2) // ((HWND)-3), HWND_MESSAGE
	notifyForThisSession = 0
	wtsCurrentServer     = 0
	wtsSessionInfoEx     = 25
	waitFailed           = ^uint32(0)
	waitTimeout          = 0x00000102
	qsAllInput           = 0x04FF
	mwmoInputAvailable   = 0x0004

	// WTSINFOEXW.Data é uma união que contém LARGE_INTEGER. O ABI usado pelo
	// wtsapi32 mantém o alinhamento de 8 bytes também em Windows 32-bit
	// (/Zp8); portanto o DWORD Level ocupa os bytes 0..3 e a união começa em
	// 8 em amd64, arm64 e 386. O decoder usa esse offset fixo e o valida em
	// testes.
	wtsInfoExDataOffset = 8

	minimumWindowsMajor = 6
	minimumWindowsMinor = 2

	wtsConsoleConnect       = 0x1
	wtsConsoleDisconnect    = 0x2
	wtsRemoteConnect        = 0x3
	wtsRemoteDisconnect     = 0x4
	wtsSessionLogon         = 0x5
	wtsSessionLogoff        = 0x6
	wtsSessionLock          = 0x7
	wtsSessionUnlock        = 0x8
	wtsSessionRemoteControl = 0x9
	wtsSessionCreate        = 0xa
	wtsSessionTerminate     = 0xb
	wtsSessionDesktopReady  = 0xf

	maxWTSQueryBuffer = 1 << 20
	cleanupTimeout    = 2 * time.Second
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	wtsapi32 = syscall.NewLazyDLL("wtsapi32.dll")

	procCreateWindowExW                  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                   = user32.NewProc("DefWindowProcW")
	procDestroyWindow                    = user32.NewProc("DestroyWindow")
	procDispatchMessageW                 = user32.NewProc("DispatchMessageW")
	procPeekMessageW                     = user32.NewProc("PeekMessageW")
	procMsgWaitForMultipleObjectsEx      = user32.NewProc("MsgWaitForMultipleObjectsEx")
	procRegisterClassExW                 = user32.NewProc("RegisterClassExW")
	procUnregisterClassW                 = user32.NewProc("UnregisterClassW")
	procGetCurrentProcessId              = kernel32.NewProc("GetCurrentProcessId")
	procGetCurrentThreadId               = kernel32.NewProc("GetCurrentThreadId")
	procGetModuleHandleW                 = kernel32.NewProc("GetModuleHandleW")
	procProcessIdToSessionId             = kernel32.NewProc("ProcessIdToSessionId")
	procRtlGetVersion                    = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
	procWTSFreeMemory                    = wtsapi32.NewProc("WTSFreeMemory")
	procWTSQuerySessionInformationW      = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnregisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
)

var windowStates sync.Map // map[HWND]*nativeState; a single process callback slot

// Uma única callback nativa por processo evita um slot NewCallback por
// Watch. O estado é removido antes de DestroyWindow, evitando que WM_DESTROY
// publique uma mensagem numa thread que já está sendo liberada.
var sessionWindowProc = syscall.NewCallback(func(hwnd, message, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if value, ok := windowStates.Load(hwnd); ok {
				value.(*nativeState).fail(fmt.Errorf("ossession: panic in native callback: %v", recovered))
			}
			result = 0
		}
	}()
	if value, ok := windowStates.Load(hwnd); ok {
		return value.(*nativeState).windowProc(hwnd, message, wParam, lParam)
	}
	result, _, _ = procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return result
})

type winPoint struct {
	x int32
	y int32
}

type winMSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   winPoint
	private uint32
}

type wndClassExW struct {
	cbSize      uint32
	style       uint32
	windowProc  uintptr
	classExtra  int32
	windowExtra int32
	instance    uintptr
	icon        uintptr
	cursor      uintptr
	background  uintptr
	menuName    *uint16
	className   *uint16
	smallIcon   uintptr
}

type rtlOSVersionInfoW struct {
	size         uint32
	major        uint32
	minor        uint32
	build        uint32
	platform     uint32
	servicePack  [128]uint16
	serviceMajor uint16
	serviceMinor uint16
	suiteMask    uint16
	productType  byte
	reserved     byte
}

type nativeState struct {
	queue           *eventQueue
	currentSession  uint32
	threadID        uint32
	hwnd            uintptr
	registered      bool
	className       *uint16
	instance        uintptr
	classRegistered bool
	fatalOnce       sync.Once
	fatalErr        error
}

func watchPlatform(ctx context.Context, observe func(State) error) error {
	queue := newEventQueue()
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopCh) }) }

	done := make(chan error, 1)
	go runNative(ctx, queue, stopCh, done)

	pumpErr := pump(ctx, queue.pop, observe, stop)
	if err := waitCleanup(done); err != nil {
		if pumpErr == nil {
			return err
		}
		return errors.Join(pumpErr, err)
	}
	return pumpErr
}

func runNative(ctx context.Context, queue *eventQueue, stopCh <-chan struct{}, done chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state := &nativeState{queue: queue}
	var runErr error

	defer func() {
		addErr := func(err error) {
			if err == nil {
				return
			}
			if runErr == nil {
				runErr = err
			} else {
				runErr = errors.Join(runErr, err)
			}
		}
		if recovered := recover(); recovered != nil {
			addErr(fmt.Errorf("ossession: panic in native session watcher: %v", recovered))
		}
		addErr(state.fatalErr)
		if runErr == nil && ctx.Err() == nil && !stopped(stopCh) {
			runErr = errPumpStopped
		}
		if state.registered {
			if err := unregisterSessionNotification(state.hwnd); err != nil {
				addErr(err)
			}
			state.registered = false
		}
		if state.hwnd != 0 {
			// Remova o estado antes de DestroyWindow: WM_DESTROY não deve
			// repostar WM_QUIT numa thread que já está saindo.
			hwnd := state.hwnd
			windowStates.Delete(hwnd)
			if result, _, callErr := procDestroyWindow.Call(hwnd); result == 0 {
				addErr(callError("DestroyWindow", callErr))
			}
			state.hwnd = 0
		}
		if state.classRegistered && state.className != nil {
			if result, _, callErr := procUnregisterClassW.Call(uintptr(unsafe.Pointer(state.className)), state.instance); result == 0 {
				addErr(callError("UnregisterClassW", callErr))
			}
		}
		queue.close(runErr)
		done <- runErr
	}()

	if err := initializeNativeState(state); err != nil {
		queue.push(pumpEvent{state: unknownState()})
		runErr = err
		return
	}

	if err := registerSessionNotification(state); err != nil {
		queue.push(pumpEvent{state: unknownState()})
		runErr = err
		return
	}

	initial, err := querySessionState(state.currentSession)
	if err != nil {
		queue.push(pumpEvent{state: unknownState()})
		runErr = err
		return
	}
	queue.push(pumpEvent{state: initial})

	for {
		if ctx.Err() != nil || stopped(stopCh) || state.fatalErr != nil {
			return
		}
		var message winMSG
		for result, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, pmRemove); result != 0; result, _, _ = procPeekMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, pmRemove) {
			if ctx.Err() != nil || stopped(stopCh) || state.fatalErr != nil {
				return
			}
			if message.message == wmQuit {
				return
			}
			_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
		}
		result, _, callErr := procMsgWaitForMultipleObjectsEx.Call(0, 0, 100, qsAllInput, mwmoInputAvailable)
		if uint32(result) == waitFailed {
			state.fail(callError("MsgWaitForMultipleObjectsEx", callErr))
			return
		}
		if result != waitTimeout && result != 0 {
			continue
		}
	}
}

func initializeNativeState(state *nativeState) error {
	if err := ensureSupportedWindowsVersion(); err != nil {
		return err
	}
	var err error
	state.currentSession, err = currentSessionID()
	if err != nil {
		return err
	}
	threadID, _, _ := procGetCurrentThreadId.Call()
	state.threadID = uint32(threadID)
	var message winMSG
	_, _, _ = procPeekMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, pmNoRemove)

	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return callError("GetModuleHandleW", callErr)
	}
	state.instance = instance
	name, err := syscall.UTF16PtrFromString(fmt.Sprintf("AssistenteOSSession-%d", state.threadID))
	if err != nil {
		return fmt.Errorf("ossession: class name: %w", err)
	}
	state.className = name
	class := wndClassExW{
		cbSize:     uint32(unsafe.Sizeof(wndClassExW{})),
		windowProc: sessionWindowProc,
		instance:   state.instance,
		className:  state.className,
	}
	if result, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return callError("RegisterClassExW", callErr)
	}
	state.classRegistered = true

	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(state.className)),
		0,
		0,
		0, 0, 0, 0,
		hwndMessage,
		0,
		state.instance,
		0,
	)
	if hwnd == 0 {
		return callError("CreateWindowExW", callErr)
	}
	state.hwnd = hwnd
	windowStates.Store(hwnd, state)
	return nil
}

func registerSessionNotification(state *nativeState) error {
	if result, _, callErr := procWTSRegisterSessionNotification.Call(state.hwnd, notifyForThisSession); result == 0 {
		return callError("WTSRegisterSessionNotification", callErr)
	}
	state.registered = true
	return nil
}

func unregisterSessionNotification(hwnd uintptr) error {
	if result, _, callErr := procWTSUnregisterSessionNotification.Call(hwnd); result == 0 {
		return callError("WTSUnRegisterSessionNotification", callErr)
	}
	return nil
}

func (state *nativeState) windowProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if message == wmWtsSessionChange {
		state.handleSessionChange(uint32(lParam), uint32(wParam))
		return 0
	}
	if message == wmDestroy {
		state.fail(errPumpStopped)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return result
}

func (state *nativeState) handleSessionChange(sessionID, reason uint32) {
	state.handleSessionChangeWithQuery(sessionID, reason, querySessionState)
}

func (state *nativeState) handleSessionChangeWithQuery(sessionID, reason uint32, query func(uint32) (State, error)) {
	if sessionID != state.currentSession {
		state.fail(fmt.Errorf("ossession: session notification mismatch: got %d, want %d", sessionID, state.currentSession))
		return
	}
	switch reason {
	case wtsConsoleDisconnect, wtsRemoteDisconnect, wtsSessionLogoff, wtsSessionTerminate:
		state.queue.push(pumpEvent{state: unknownState()})
	case wtsSessionLock:
		// Um lock invalida imediatamente o snapshot anterior. Não o
		// substitua por uma consulta que atravesse a transição e retorne
		// UNLOCK; o estado publicado para este evento é sempre fechado.
		state.queue.push(pumpEvent{state: State{Known: true, Locked: true}})
	case wtsConsoleConnect, wtsRemoteConnect, wtsSessionLogon, wtsSessionUnlock, wtsSessionRemoteControl, wtsSessionCreate, wtsSessionDesktopReady:
		current, err := query(state.currentSession)
		if err != nil {
			state.fail(err)
			return
		}
		state.queue.push(pumpEvent{state: current})
	default:
		state.fail(fmt.Errorf("ossession: unknown WTS session notification: %d", reason))
	}
}

func (state *nativeState) fail(err error) {
	if err == nil {
		return
	}
	state.fatalOnce.Do(func() {
		state.fatalErr = err
		state.queue.push(pumpEvent{state: unknownState()})
	})
}

func currentSessionID() (uint32, error) {
	processID, _, _ := procGetCurrentProcessId.Call()
	var sessionID uint32
	result, _, callErr := procProcessIdToSessionId.Call(processID, uintptr(unsafe.Pointer(&sessionID)))
	if result == 0 {
		return 0, callError("ProcessIdToSessionId", callErr)
	}
	return sessionID, nil
}

func querySessionState(sessionID uint32) (State, error) {
	var buffer unsafe.Pointer
	var returned uint32
	result, _, callErr := procWTSQuerySessionInformationW.Call(
		wtsCurrentServer,
		uintptr(sessionID),
		wtsSessionInfoEx,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if result == 0 {
		return unknownState(), callError("WTSQuerySessionInformationW", callErr)
	}
	if buffer == nil {
		return unknownState(), fmt.Errorf("ossession: WTSQuerySessionInformationW returned a nil buffer")
	}
	defer procWTSFreeMemory.Call(uintptr(buffer))
	if returned > maxWTSQueryBuffer {
		return unknownState(), fmt.Errorf("ossession: WTSINFOEX buffer is unreasonably large: %d", returned)
	}
	if returned < wtsInfoExDataOffset+wtsInfoExLevel1PrefixSize {
		return unknownState(), fmt.Errorf("ossession: WTSINFOEX returned only %d bytes", returned)
	}
	raw := unsafe.Slice((*byte)(buffer), int(returned))
	return decodeWTSInfoEx(raw, wtsInfoExDataOffset, sessionID)
}

func stopped(stopCh <-chan struct{}) bool {
	select {
	case <-stopCh:
		return true
	default:
		return false
	}
}

func ensureSupportedWindowsVersion() error {
	version := rtlOSVersionInfoW{size: uint32(unsafe.Sizeof(rtlOSVersionInfoW{}))}
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&version)))
	if status != 0 {
		return fmt.Errorf("ossession: RtlGetVersion: NTSTATUS 0x%x", status)
	}
	if version.major < minimumWindowsMajor ||
		(version.major == minimumWindowsMajor && version.minor < minimumWindowsMinor) {
		return fmt.Errorf("ossession: Windows %d.%d is unsupported; Windows 8 or later is required", version.major, version.minor)
	}
	return nil
}

func waitCleanup(done <-chan error) error {
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("ossession: cleanup timeout after %s", cleanupTimeout)
	}
}

func callError(name string, callErr error) error {
	if callErr == nil || callErr == syscall.Errno(0) {
		callErr = syscall.GetLastError()
	}
	if callErr == nil || callErr == syscall.Errno(0) {
		callErr = syscall.EINVAL
	}
	return fmt.Errorf("ossession: %s: %w", name, callErr)
}
