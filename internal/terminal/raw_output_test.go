package terminal

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"unicode/utf8"
)

func TestReadLoopDeliversManualOutputAcrossReads(t *testing.T) {
	const raw = "início\r\x1b[31m\n\x1bPdados\x18visível\x1bPoutro\nbloco\x1b\\\x1b]0;título\x1apronto\rúltimo\r"
	const want = "início\nvisívelpronto\núltimo\n"
	for cut := 0; cut <= len(raw); cut++ {
		var out strings.Builder
		s := &Session{ptyReader: io.MultiReader(strings.NewReader(raw[:cut]), strings.NewReader(raw[cut:])),
			onRawOutput: func(_, chunk string) {
				if !utf8.ValidString(chunk) {
					t.Fatalf("corte=%d evento UTF-8 inválido %q", cut, chunk)
				}
				out.WriteString(chunk)
				if !strings.HasPrefix(want, out.String()) {
					t.Fatalf("corte=%d evento inesperado %q", cut, out.String())
				}
			},
		}
		s.readLoop(t.Context())
		if out.String() != want {
			t.Fatalf("corte=%d saída=%q", cut, out.String())
		}
	}
}

type manualTestWriter func([]byte) (int, error)

func (write manualTestWriter) Write(p []byte) (int, error) { return write(p) }

func TestManualOutputAcrossManagedCommand(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	manualStarted := make(chan struct{}, 1)
	var manual strings.Builder
	s := &Session{shell: "bash", readerCtx: t.Context(), ptyReader: reader,
		onRawOutput: func(_, chunk string) {
			manual.WriteString(chunk)
			if chunk == "antes" {
				manualStarted <- struct{}{}
			}
		},
	}
	s.ptyWriter = manualTestWriter(func(p []byte) (int, error) {
		_, tail, found := strings.Cut(string(p), "'"+markerStart)
		if !found {
			return 0, errors.New("falta marcador inicial na moldura")
		}
		suffix, _, _ := strings.Cut(tail, "'")
		_, err := io.WriteString(writer, "\a\n"+markerStart+suffix+"\nexecutado\n"+markerEnd+suffix+"0!\n\x1bPoculto")
		return len(p), err
	})
	s.Start()
	if _, err := io.WriteString(writer, "antes\x1b]0;título"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manualStarted:
	case <-time.After(time.Second):
		t.Fatal("saída manual inicial não chegou")
	}
	if err := s.beginCommandWithID("teste"); err != nil {
		t.Fatal(err)
	}
	entry, err := s.RunCommand(t.Context(), "echo executado", time.Second, "teste", "teste")
	if err != nil || entry == nil || entry.ExitCode != 0 || entry.Output != "executado" {
		t.Fatalf("comando=%+v erro=%v", entry, err)
	}
	if _, err := io.WriteString(writer, "ainda oculto\x18depois"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	<-s.readerDone
	if got := manual.String(); got != "antesdepois" {
		t.Fatalf("saída manual=%q", got)
	}
}

func TestReadLoopFlushesManualOutputBeforeExit(t *testing.T) {
	const pending = "visível\x1bPcontrole\r\nincompleto\r"
	const want = "visível\x1bPcontrole\nincompleto\n"
	for _, err := range []error{io.EOF, errors.New("leitor falhou")} {
		var out strings.Builder
		s := &Session{
			ptyReader:   io.MultiReader(strings.NewReader(pending), iotest.ErrReader(err)),
			onRawOutput: func(_, chunk string) { out.WriteString(chunk) },
			onExit: func(_ string, _ error) {
				if out.String() != want {
					t.Fatalf("encerramento=%v saída no evento de saída=%q", err, out.String())
				}
			},
		}
		s.readLoop(t.Context())
		if out.String() != want {
			t.Fatalf("encerramento=%v saída=%q", err, out.String())
		}
	}
}
