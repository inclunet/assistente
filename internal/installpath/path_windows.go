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

var user32 = windows.NewLazySystemDLL("user32.dll")

// CleanupWarning indica limpeza parcial; não impede remover o aplicativo.
// Os marcadores dos perfis não acessíveis ficam preservados.
type CleanupWarning struct{ Err error }

func (w *CleanupWarning) Error() string { return w.Err.Error() }
func (w *CleanupWarning) Unwrap() error { return w.Err }

// InteractiveUserSID resolve a conta dona do shell da estação interativa.
// Não depende de RDS nem usa o token da conta elevada como fallback.
func InteractiveUserSID() (string, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		return "", err
	}
	if session == 0 {
		return "", errors.New("interactive session unavailable")
	}
	station, _, err := user32.NewProc("GetProcessWindowStation").Call()
	if station == 0 {
		return "", fmt.Errorf("window station: %w", err)
	}
	// O handle pertence ao sistema e não deve ser fechado.
	query := user32.NewProc("GetUserObjectInformationW")
	var flags struct{ inherit, reserved, flags uint32 }
	var needed uint32
	ok, _, err := query.Call(station, 1, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 {
		return "", fmt.Errorf("window station flags: %w", err)
	}
	if flags.flags&1 == 0 {
		return "", errors.New("window station is not interactive")
	}
	// SECURITY_MAX_SID_SIZE: SID revision/subauthority header + 15 subauthorities.
	var buffer [68]byte
	ok, _, err = query.Call(station, 4, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 {
		return "", fmt.Errorf("window station user: %w", err)
	}
	logonSID, err := stationUserSID(buffer[:], needed)
	if err != nil {
		return "", err
	}
	return shellAccountSID(logonSID, session)
}

func stationUserSID(buffer []byte, length uint32) (string, error) {
	if length < 8 || int(length) > len(buffer) {
		return "", errors.New("window station user unavailable")
	}
	// Valida comprimento antes de chamar IsValidSid/GetLengthSid.
	if expected := 8 + 4*int(buffer[1]); expected != int(length) || buffer[1] > 15 {
		return "", errors.New("invalid window station SID length")
	}
	sid := (*windows.SID)(unsafe.Pointer(&buffer[0]))
	if !sid.IsValid() {
		return "", errors.New("invalid window station user")
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

// UOI_USER_SID identifica o logon (S-1-5-5-X-Y), não a conta em HKEY_USERS.
// Consulta somente o token do shell desta sessão, sem usar o usuário elevado
// do instalador. O grupo de logon deve corresponder à estação interativa.
func shellAccountSID(logonSID string, session uint32) (string, error) {
	window, _, _ := user32.NewProc("GetShellWindow").Call()
	if window == 0 {
		return "", errors.New("interactive shell unavailable")
	}
	var pid uint32
	thread, _, err := user32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	if thread == 0 || pid == 0 {
		return "", fmt.Errorf("interactive shell process: %w", err)
	}
	token, err := shellQueryToken(pid)
	if err != nil {
		return "", fmt.Errorf("interactive shell token: %w", err)
	}
	defer func() { _ = token.Close() }()
	var tokenSession, returned uint32
	if err := windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&tokenSession)), uint32(unsafe.Sizeof(tokenSession)), &returned); err != nil {
		return "", fmt.Errorf("interactive shell session: %w", err)
	}
	if returned != uint32(unsafe.Sizeof(tokenSession)) || session == 0 || tokenSession != session {
		return "", errors.New("interactive shell belongs to another session")
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("interactive shell user: %w", err)
	}
	groups, err := token.GetTokenGroups()
	if err != nil {
		return "", fmt.Errorf("interactive shell groups: %w", err)
	}
	return accountSIDForStation(user.User.Sid, groups.AllGroups(), logonSID)
}

func openQueryToken(pid uint32) (windows.Token, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	var token windows.Token
	err = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token)
	return token, err
}

// O helper é um processo exclusivo do instalador, sem serviços do aplicativo.
// Se outra conta autorizou o UAC, habilita temporariamente SeDebugPrivilege
// apenas para abrir o handle de consulta e restaura o estado antes de usá-lo.
func shellQueryToken(pid uint32) (windows.Token, error) {
	token, err := openQueryToken(pid)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return token, err
	}
	var own windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES, &own); err != nil {
		return 0, err
	}
	defer func() { _ = own.Close() }()
	err = withTokenPrivilege(own, "SeDebugPrivilege", func() error {
		var openErr error
		token, openErr = openQueryToken(pid)
		return openErr
	})
	if err != nil && token != 0 {
		_ = token.Close()
		token = 0
	}
	return token, err
}

func withTokenPrivilege(token windows.Token, name string, action func() error) (err error) {
	privilege, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	state := windows.Tokenprivileges{PrivilegeCount: 1}
	if err := windows.LookupPrivilegeValue(nil, privilege, &state.Privileges[0].Luid); err != nil {
		return err
	}
	state.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
	var previous windows.Tokenprivileges
	var size uint32
	if err := windows.AdjustTokenPrivileges(token, false, &state, uint32(unsafe.Sizeof(previous)), &previous, &size); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, windows.AdjustTokenPrivileges(token, false, &previous, 0, nil, nil))
	}()
	// AdjustTokenPrivileges pode retornar sucesso sem atribuir um privilégio
	// ausente do token. Confira o estado efetivo antes de executar a consulta.
	var needed uint32
	queryErr := windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &needed)
	if !errors.Is(queryErr, windows.ERROR_INSUFFICIENT_BUFFER) {
		return fmt.Errorf("query token privileges: %w", queryErr)
	}
	buffer := make([]byte, needed)
	if err := windows.GetTokenInformation(token, windows.TokenPrivileges, &buffer[0], needed, &needed); err != nil {
		return err
	}
	actual := (*windows.Tokenprivileges)(unsafe.Pointer(&buffer[0]))
	for _, entry := range actual.AllPrivileges() {
		if entry.Luid == state.Privileges[0].Luid && entry.Attributes&windows.SE_PRIVILEGE_ENABLED != 0 {
			return action()
		}
	}
	return fmt.Errorf("%s unavailable: %w", name, windows.ERROR_NOT_ALL_ASSIGNED)
}

func accountSIDForStation(user *windows.SID, groups []windows.SIDAndAttributes, logonSID string) (string, error) {
	if user == nil || !user.IsValid() {
		return "", errors.New("invalid interactive account SID")
	}
	account := user.String()
	if strings.HasPrefix(account, "S-1-5-5-") {
		return "", errors.New("logon SID is not an account SID")
	}
	for _, group := range groups {
		if group.Attributes&windows.SE_GROUP_LOGON_ID == windows.SE_GROUP_LOGON_ID && group.Sid != nil && group.Sid.IsValid() && group.Sid.String() == logonSID {
			return account, nil
		}
	}
	return "", errors.New("interactive shell logon does not match window station")
}
