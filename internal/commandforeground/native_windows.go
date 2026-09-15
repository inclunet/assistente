//go:build windows

package commandforeground

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

const (
	maxProcessImageUTF16 = 32768
	maxWindowClassUTF16  = 256
)

type nativeReader struct{}

// NewNative retorna o adapter Windows que consulta somente APIs de identidade
// da janela/processo. A implementação não toca no conteúdo da janela.
func NewNative() Reader {
	return nativeReader{}
}

func (nativeReader) Capture(ctx context.Context) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, errors.New("foreground: contexto nil")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return Snapshot{}, fmt.Errorf("foreground: GetForegroundWindow: %w", ErrUnknown)
	}

	pid, err := windowPID(hwnd)
	if err != nil {
		return Snapshot{}, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Snapshot{}, fmt.Errorf("foreground: OpenProcess: %w", err)
	}
	defer windows.CloseHandle(process)

	if err := checkHandlePID(process, pid); err != nil {
		return Snapshot{}, err
	}
	creationTime, err := queryProcessCreationTime(process)
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	imagePath, err := queryImagePath(process)
	if err != nil {
		return Snapshot{}, err
	}
	className, err := queryClassName(hwnd)
	if err != nil {
		return Snapshot{}, err
	}
	executable := normalizeExecutableBase(imagePath)
	windowClass := normalizeWindowClass(className)
	if executable == "" || windowClass == "" {
		return Snapshot{}, fmt.Errorf("foreground: %w", ErrUnknown)
	}

	// O HWND pode ser reutilizado durante a leitura. Confirme a PID observada
	// pela janela e a PID do handle antes de publicar a observação.
	if err := checkWindowPID(hwnd, pid); err != nil {
		return Snapshot{}, err
	}
	identity := Identity{window: uintptr(hwnd), process: pid, creationTime: creationTime}
	summary := Summary{
		Executable:      executable,
		WindowClass:     windowClass,
		ProviderVersion: ProviderVersion,
	}
	if err := checkHandlePID(process, pid); err != nil {
		return Snapshot{}, err
	}
	if current := windows.GetForegroundWindow(); current != hwnd {
		return Snapshot{}, fmt.Errorf("foreground: foco mudou durante captura: %w", ErrUnknown)
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	capturedAt := time.Now()
	return Snapshot{
		Identity:   identity,
		Version:    foregroundFactVersion(identity, summary),
		CapturedAt: capturedAt,
		Summary:    summary,
	}, nil
}

func windowPID(hwnd windows.HWND) (uint32, error) {
	var pid uint32
	tid, err := windows.GetWindowThreadProcessId(hwnd, &pid)
	if err != nil {
		return 0, fmt.Errorf("foreground: GetWindowThreadProcessId: %w", err)
	}
	if tid == 0 || pid == 0 {
		return 0, fmt.Errorf("foreground: %w", ErrUnknown)
	}
	return pid, nil
}

func checkWindowPID(hwnd windows.HWND, want uint32) error {
	got, err := windowPID(hwnd)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("foreground: janela mudou de PID (%d para %d): %w", want, got, ErrUnknown)
	}
	return nil
}

func checkHandlePID(process windows.Handle, want uint32) error {
	got, err := windows.GetProcessId(process)
	if err != nil {
		return fmt.Errorf("foreground: GetProcessId: %w", err)
	}
	if got == 0 || got != want {
		return fmt.Errorf("foreground: PID do handle divergente (%d para %d): %w", want, got, ErrUnknown)
	}
	return nil
}

func queryProcessCreationTime(process windows.Handle) (uint64, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		return 0, fmt.Errorf("foreground: GetProcessTimes: %w", err)
	}
	value := uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	if value == 0 {
		return 0, fmt.Errorf("foreground: tempo de criação do processo inválido: %w", ErrUnknown)
	}
	return value, nil
}

func queryImagePath(process windows.Handle) (string, error) {
	buffer := make([]uint16, maxProcessImageUTF16)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", fmt.Errorf("foreground: QueryFullProcessImageName: %w", err)
	}
	if size == 0 || size > uint32(len(buffer)) {
		return "", fmt.Errorf("foreground: caminho do processo inválido: %w", ErrUnknown)
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func queryClassName(hwnd windows.HWND) (string, error) {
	buffer := make([]uint16, maxWindowClassUTF16)
	copied, err := windows.GetClassName(hwnd, &buffer[0], int32(len(buffer)))
	if err != nil {
		return "", fmt.Errorf("foreground: GetClassName: %w", err)
	}
	if copied <= 0 || copied >= int32(len(buffer)) {
		return "", fmt.Errorf("foreground: classe da janela inválida: %w", ErrUnknown)
	}
	return windows.UTF16ToString(buffer[:copied]), nil
}
