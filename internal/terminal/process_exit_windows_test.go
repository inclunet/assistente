package terminal

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"
	"time"

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
			reader := io.MultiReader(strings.NewReader(marker.startTag+"\r\nparcial"), iotest.ErrReader(readErr))
			session := &Session{ptySession: pty, readerCtx: t.Context(), ptyReader: reader}
			session.beginMarkerCapture(marker)
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

type ptyOutputWriter func([]byte) (int, error)

func (write ptyOutputWriter) Write(p []byte) (int, error) { return write(p) }

func TestConPTYResizeAcrossSessionCommands(t *testing.T) {
	s := &Session{shell: "powershell.exe", viewportColumns: 80}
	command := 0
	// O PTY simulado só precisa do identificador do protocolo para responder
	// com as tags. A sintaxe usada pelo shell para montá-las pode variar.
	markerID := regexp.MustCompile(`[0-9a-f]{8}` + regexp.QuoteMeta(markerSuffix))
	s.ptyWriter = ptyOutputWriter(func(wrapper []byte) (int, error) {
		suffix := string(markerID.Find(wrapper))
		if suffix == "" {
			return 0, errors.New("o PTY simulado não encontrou o identificador do comando")
		}
		command++
		switch command {
		case 2:
			s.recordOutput("3t\x1b\\") // CSI falso dentro de DCS
		case 3:
			s.recordOutput("3t\a") // CSI falso dentro de OSC
		case 4:
			s.recordOutput("3t") // CSI real partido na troca de comando
		}
		s.recordOutput(markerStart + suffix + "\r\nabc\r\n\x1b[2;3Hcde\r\n" + markerEnd + suffix + "0!\r\n")
		switch command {
		case 1:
			s.recordOutput("\x1bP" + strings.Repeat("x", 65536) + "\x1b[8;40;")
		case 2:
			s.recordOutput("\x1b]0;\x1b[8;40;")
		case 3:
			s.recordOutput("prompt \x1b[8;40;")
		}
		return len(wrapper), nil
	})
	for _, want := range []string{"abc\ncde", "abc\ncde", "abc\ncde", "abcde"} {
		entry, err := s.RunCommand(t.Context(), "echo", time.Second, "test", "test")
		if err != nil || entry == nil || entry.Output != want {
			t.Fatalf("comando %d: entrada=%+v erro=%v", command, entry, err)
		}
	}
}
