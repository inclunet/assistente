package terminal

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/KennethanCeyer/ptyx"
	"golang.org/x/sys/windows"
)

type processStatusPTY struct {
	fakePTYSession
	pid     int
	waitErr error
}

func (p *processStatusPTY) Pid() int    { return p.pid }
func (p *processStatusPTY) Wait() error { return p.waitErr }

func TestConPTYClosedReaderUsesSignaledProcessStatus(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/d", "/c", "exit 3")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Mantenha o processo coletável enquanto o leitor recebe os.ErrClosed,
	// assim como o ptyx faz entre o fechamento do ConPTY e Session.Wait.
	t.Cleanup(func() {
		if err := cmd.Wait(); cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 3 {
			t.Errorf("status do processo de teste: %v", err)
		}
	})
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	state, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("processo não sinalizado: estado=%v erro=%v", state, err)
	}

	for _, tc := range []struct {
		name    string
		pid     int
		code    int
		readErr error
		kills   int
	}{
		{"encerrado", cmd.Process.Pid, 3, nil, 0},
		{"vivo", os.Getpid(), -1, os.ErrClosed, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := newCommandMarkerForShell("powershell")
			pty := &processStatusPTY{pid: tc.pid, waitErr: &ptyx.ExitError{ExitCode: 3}}
			readErr := &os.PathError{Op: "read", Path: "conpty-stdout", Err: os.ErrClosed}
			reader := io.MultiReader(strings.NewReader(marker.StartTag()+"\r\nparcial"), iotest.ErrReader(readErr))
			session := &Session{ptySession: pty, readerCtx: t.Context(), ptyReader: reader}
			session.Start()
			<-session.readerDone
			output, code, err := session.waitForMarker(t.Context(), marker, "test")
			if !errors.Is(err, tc.readErr) || code != tc.code || output != "parcial" {
				t.Errorf("output=%q code=%d erro=%v", output, code, err)
			}
			if tc.kills > 0 {
				<-session.closeDone
			}
			if pty.killCalls != tc.kills {
				t.Errorf("Kill foi chamado %d vezes", pty.killCalls)
			}
		})
	}
}
