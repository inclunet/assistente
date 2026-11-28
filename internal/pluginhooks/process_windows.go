// Package pluginhooks executa handlers da revisão inspecionada do plugin.
package pluginhooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// Os atributos pertencem ao SDK do Windows (ProcThreadAttributeValue).
const (
	securityCapabilitiesAttribute         = 0x20009
	allApplicationPackagesPolicyAttribute = 0x2000f
	jobListAttribute                      = 0x2000d
	maxOutput                             = 16 << 10
)

type securityCapabilities struct {
	appContainerSID *windows.SID
	capabilities    *windows.SIDAndAttributes
	count, reserved uint32
}

var userenv = windows.NewLazySystemDLL("userenv.dll")
var createProfile = userenv.NewProc("CreateAppContainerProfile")
var deleteProfile = userenv.NewProc("DeleteAppContainerProfile")

func createContainer(name string) (*windows.SID, func() error, error) {
	if err := createProfile.Find(); err != nil {
		return nil, nil, err
	}
	if err := deleteProfile.Find(); err != nil {
		return nil, nil, err
	}
	encoded, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	var sid *windows.SID
	hr, _, _ := createProfile.Call(uintptr(unsafe.Pointer(encoded)), uintptr(unsafe.Pointer(encoded)), uintptr(unsafe.Pointer(encoded)), 0, 0, uintptr(unsafe.Pointer(&sid)))
	runtime.KeepAlive(encoded)
	if hr != 0 {
		return nil, nil, fmt.Errorf("CreateAppContainerProfile: HRESULT %#x", hr)
	}
	return sid, func() error {
		_ = windows.FreeSid(sid)
		var hr uintptr
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				time.Sleep(50 * time.Millisecond)
			}
			hr, _, _ = deleteProfile.Call(uintptr(unsafe.Pointer(encoded)))
			runtime.KeepAlive(encoded)
			if hr == 0 {
				return nil
			}
		}
		return fmt.Errorf("DeleteAppContainerProfile %s: HRESULT %#x", name, hr)
	}, nil
}

// Cada execução recebe um SID novo. Somente o usuário hospedeiro grava os arquivos
// do snapshot; o AppContainer recebe leitura e execução desde a criação da pasta.
func makePackageDir(parent, name string, sid *windows.SID) (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GRGX;;;" + sid.String() + ")")
	if err != nil {
		return "", err
	}
	path := filepath.Join(parent, name)
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	sa := windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(sa))
	return path, windows.CreateDirectory(ptr, &sa)
}

func createJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
	info.BasicLimitInformation.ActiveProcessLimit = 1
	info.ProcessMemoryLimit = 256 << 20
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// Run inicia Windows PowerShell 5.1 com um snapshot privado e um evento JSON
// em stdin. O chamador escolhe uma pasta privada da aplicação para a preparação.
// A saída é limitada e o status só é válido quando err é nil.
func Run(ctx context.Context, parent string, files map[string][]byte, entry string, event []byte) (output []byte, status uint32, err error) {
	if !filepath.IsAbs(parent) || len(event) > 64<<10 || !strings.EqualFold(filepath.Ext(entry), ".ps1") {
		return nil, 0, os.ErrInvalid
	}
	if _, ok := files[entry]; !ok {
		return nil, 0, os.ErrNotExist
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	name := "Assistente.Hook." + uuid.NewString()
	sid, cleanup, err := createContainer(name)
	if err != nil {
		return nil, 0, err
	}
	defer func() { err = errors.Join(err, cleanup()) }()
	dir, err := makePackageDir(parent, name, sid)
	if err != nil {
		return nil, 0, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	if err = writeSnapshot(ctx, dir, files); err != nil {
		return nil, 0, err
	}

	// O leitor herdado possui somente acesso de leitura ao evento. O escritor
	// da saída é um pipe; o processo nunca recebe um handle do workspace ou do cofre.
	in, err := os.CreateTemp(parent, "hook-event-")
	if err != nil {
		return nil, 0, err
	}
	defer func() { err = errors.Join(err, os.Remove(in.Name())) }()
	_, writeErr := in.Write(event)
	if err = errors.Join(writeErr, in.Close()); err != nil {
		return nil, 0, err
	}
	input, err := os.Open(in.Name())
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = input.Close() }()
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	job, err := createJob()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = windows.CloseHandle(job) }()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	process, err := start(ctx, dir, entry, sid, job, input, writer)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	_ = writer.Close()
	type capture struct {
		data []byte
		err  error
	}
	captured := make(chan capture, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(reader, maxOutput+1))
		captured <- capture{data, err}
	}()
	var finished bool
	defer func() {
		if finished {
			return
		}
		if killErr := windows.TerminateJobObject(job, 1); killErr != nil {
			err = errors.Join(err, killErr)
			return
		}
		state, waitErr := windows.WaitForSingleObject(process, 1000)
		if waitErr != nil || state != windows.WAIT_OBJECT_0 {
			err = errors.Join(err, fmt.Errorf("encerramento do hook: estado %d", state), waitErr)
			return
		}
		if captured != nil {
			select {
			case result := <-captured:
				if len(result.data) <= maxOutput {
					output = result.data
				}
			case <-time.After(time.Second):
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case result := <-captured:
		captured = nil
		if result.err != nil {
			return nil, 0, result.err
		}
		if len(result.data) > maxOutput {
			return nil, 0, errors.New("saída do hook excedeu o limite")
		}
		output = result.data
	}
	for !finished {
		if err := ctx.Err(); err != nil {
			return output, 0, err
		}
		state, err := windows.WaitForSingleObject(process, 25)
		if err != nil {
			return nil, 0, err
		}
		finished = state == windows.WAIT_OBJECT_0
	}
	err = windows.GetExitCodeProcess(process, &status)
	return
}

func start(ctx context.Context, dir, entry string, sid *windows.SID, job windows.Handle, input, output *os.File) (windows.Handle, error) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return 0, err
	}
	shell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	app, err := windows.UTF16PtrFromString(shell)
	if err != nil {
		return 0, err
	}
	args, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", filepath.Join(dir, entry)}))
	if err != nil {
		return 0, err
	}
	cwd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	winDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return 0, err
	}
	// CreateProcess resolves the AppContainer profile using LOCALAPPDATA.
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return 0, err
	}
	env, err := windows.UTF16FromString("LOCALAPPDATA=" + local)
	if err != nil {
		return 0, err
	}
	systemEnv, err := windows.UTF16FromString("SystemRoot=" + winDir)
	if err != nil {
		return 0, err
	}
	env = append(env, systemEnv...)
	env = append(env, 0)
	attrs, err := windows.NewProcThreadAttributeList(4)
	if err != nil {
		return 0, err
	}
	defer attrs.Delete()
	caps := securityCapabilities{appContainerSID: sid}
	optOut := uint32(1) // PROCESS_CREATION_ALL_APPLICATION_PACKAGES_OPT_OUT (LPAC).
	if err = attrs.Update(securityCapabilitiesAttribute, unsafe.Pointer(&caps), unsafe.Sizeof(caps)); err != nil {
		return 0, err
	}
	if err = attrs.Update(allApplicationPackagesPolicyAttribute, unsafe.Pointer(&optOut), unsafe.Sizeof(optOut)); err != nil {
		return 0, err
	}
	if err = attrs.Update(jobListAttribute, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return 0, err
	}
	// Como em os/exec no Windows, passe uma lista explícita e feche as
	// duplicatas herdáveis assim que CreateProcess devolver o controle.
	var handles [2]windows.Handle
	for i, file := range []*os.File{input, output} {
		err = windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(file.Fd()), windows.CurrentProcess(), &handles[i], 0, true, windows.DUPLICATE_SAME_ACCESS)
		if err != nil {
			return 0, err
		}
		defer func(h windows.Handle) { _ = windows.CloseHandle(h) }(handles[i])
	}
	if err = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), unsafe.Sizeof(handles)); err != nil {
		return 0, err
	}
	var startup windows.StartupInfoEx
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[1]
	startup.ProcThreadAttributeList = attrs.List()
	var process windows.ProcessInformation
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	err = windows.CreateProcess(app, args, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, &env[0], cwd, &startup.StartupInfo, &process)
	runtime.KeepAlive(sid)
	if err != nil {
		return 0, fmt.Errorf("CreateProcess de Windows PowerShell: %w", err)
	}
	_ = windows.CloseHandle(process.Thread)
	return process.Process, nil
}
