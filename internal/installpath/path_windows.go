package installpath

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var querySession = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSQuerySessionInformationW")

// CleanupWarning indica limpeza parcial; não impede remover o aplicativo.
// Os marcadores dos perfis não acessíveis ficam preservados.
type CleanupWarning struct{ Err error }

func (w *CleanupWarning) Error() string { return w.Err.Error() }
func (w *CleanupWarning) Unwrap() error { return w.Err }

// InteractiveUserSID identifica a conta da sessão, não a conta usada pelo UAC.
// Não há fallback para o administrador se a sessão não puder ser identificada.
func InteractiveUserSID() (string, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		return "", err
	}
	if session == 0 {
		return "", errors.New("installer PATH requires an interactive user session")
	}
	read := func(class uintptr) (string, error) {
		var buffer *uint16
		var size uint32
		ok, _, err := querySession.Call(0, uintptr(session), class, uintptr(unsafe.Pointer(&buffer)), uintptr(unsafe.Pointer(&size)))
		if ok == 0 {
			return "", fmt.Errorf("query installer user: %w", err)
		}
		defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
		if buffer == nil || size < 2 {
			return "", errors.New("empty session identity")
		}
		return windows.UTF16PtrToString(buffer), nil
	}
	user, err := read(5) // WTSUserName
	if err != nil || user == "" {
		return "", fmt.Errorf("session user unavailable: %v", err)
	}
	domain, err := read(7) // WTSDomainName
	if err != nil {
		return "", err
	}
	if domain != "" {
		user = domain + `\` + user
	}
	sid, _, kind, err := windows.LookupSID("", user)
	if err != nil {
		return "", err
	}
	if kind != windows.SidTypeUser {
		return "", errors.New("session identity is not a user")
	}
	return sid.String(), nil
}

func readPath(key registry.Key) (string, uint32, error) {
	value, kind, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", registry.EXPAND_SZ, nil
	}
	return value, kind, err
}

func writePath(key registry.Key, value string, kind uint32) error {
	switch kind {
	case registry.SZ:
		return key.SetStringValue("Path", value)
	case registry.EXPAND_SZ:
		return key.SetExpandStringValue("Path", value)
	default:
		return fmt.Errorf("unsupported PATH registry type: %d", kind)
	}
}

// update usa chaves explícitas para permitir testes em um registro descartável.
// O marcador só pertence a entradas que este instalador adicionou.
func update(environment, owners registry.Key, sid, directory string, remove bool) error {
	before, kind, err := readPath(environment)
	if err != nil {
		return err
	}
	owned, _, ownerErr := owners.GetStringValue(sid)
	if ownerErr != nil && !errors.Is(ownerErr, registry.ErrNotExist) {
		return ownerErr
	}
	if ownerErr == nil && !sameEntry(owned, directory) {
		return errors.New("installer PATH ownership mismatch")
	}
	if remove {
		if errors.Is(ownerErr, registry.ErrNotExist) {
			return nil
		}
		after := removeEntry(before, directory)
		if after != before {
			if err := writePath(environment, after, kind); err != nil {
				return err
			}
		}
		return owners.DeleteValue(sid)
	}
	after, added := addEntry(before, directory)
	if !added {
		return nil
	}
	// Registra a propriedade primeiro; uma interrupção nunca deixa uma entrada
	// adicionada sem informação para a desinstalação.
	if err := owners.SetStringValue(sid, directory); err != nil {
		return err
	}
	if err := writePath(environment, after, kind); err != nil {
		if errors.Is(ownerErr, registry.ErrNotExist) {
			_ = owners.DeleteValue(sid)
		}
		return err
	}
	return nil
}

// Run é exclusivo do instalador. A pasta vem do próprio asst.exe instalado,
// não de argumentos externos; nenhum serviço do Assistente é inicializado.
func Run(remove bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Serializa instalações/desinstalações do Assistente que atualizam o PATH.
	name, _ := windows.UTF16PtrFromString(`Global\InclunetAssistenteInstallerPath`)
	mutex, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	defer func() { _ = windows.CloseHandle(mutex) }()
	status, err := windows.WaitForSingleObject(mutex, 30000)
	if err != nil {
		return err
	}
	if status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		return errors.New("another installer is updating PATH")
	}
	defer func() { _ = windows.ReleaseMutex(mutex) }()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	directory := filepath.Dir(exe)
	if strings.Contains(directory, ";") {
		return errors.New("installation directory cannot contain a semicolon")
	}
	keyName := fmt.Sprintf(`Software\Inclunet\Assistente\InstallerPath\%x`, sha256.Sum256([]byte(strings.ToLower(directory))))
	var owners registry.Key
	if remove {
		owners, err = registry.OpenKey(registry.LOCAL_MACHINE, keyName, registry.ALL_ACCESS|registry.WOW64_64KEY)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
	} else {
		owners, _, err = registry.CreateKey(registry.LOCAL_MACHINE, keyName, registry.ALL_ACCESS|registry.WOW64_64KEY)
	}
	if err != nil {
		return err
	}
	defer func() { _ = owners.Close() }()
	var users []string
	if remove {
		users, err = owners.ReadValueNames(-1)
	} else {
		var sid string
		sid, err = InteractiveUserSID()
		users = []string{sid}
	}
	if err != nil {
		return err
	}
	updateErr := updateUsers(owners, users, directory, remove, func(sid string) (registry.Key, error) {
		return registry.OpenKey(registry.USERS, sid+`\Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	})
	// Novos processos do Explorer precisam receber o ambiente atualizado.
	message, _ := windows.UTF16PtrFromString("Environment")
	_, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(0xffff, 0x1a, 0, uintptr(unsafe.Pointer(message)), 2, 5000, 0)
	return updateErr
}

func updateUsers(owners registry.Key, users []string, directory string, remove bool, openEnvironment func(string) (registry.Key, error)) error {
	var warnings []error
	for _, sid := range users {
		if _, err := windows.StringToSid(sid); err != nil {
			return err
		}
		environment, err := openEnvironment(sid)
		if err != nil {
			if remove {
				warnings = append(warnings, fmt.Errorf("PATH %q for %s was not removed: %w", directory, sid, err))
				continue
			}
			return fmt.Errorf("open PATH for %s (user must be signed in): %w", sid, err)
		}
		err = update(environment, owners, sid, directory, remove)
		_ = environment.Close()
		if err != nil {
			if remove {
				warnings = append(warnings, fmt.Errorf("PATH %q for %s: %w", directory, sid, err))
				continue
			}
			return err
		}
	}
	if len(warnings) > 0 {
		return &CleanupWarning{Err: errors.Join(warnings...)}
	}
	return nil
}
