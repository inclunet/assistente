package terminal

import (
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"
	"unicode/utf8"
)

func TestReadLoopDeliversManualOutputAcrossReads(t *testing.T) {
	const raw = "início\r\x1b[31m\n\x1bPdados\x18visível\x1bPoutro\nbloco\x1b\\\x1b]0;título\x1apronto\rúltimo\r"
	const want = "início\nvisívelpronto\núltimo\n"
	for cut := 0; cut <= len(raw); cut++ {
		var out strings.Builder
		s := &Session{ptyReader: io.MultiReader(strings.NewReader(raw[:cut]), strings.NewReader(raw[cut:])),
			onRawOutput: func(_, _, chunk string) {
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

func TestCommandStartWaitsForPreviousManualDelivery(t *testing.T) {
	for _, managed := range []bool{true, false} {
		name := "manual"
		if managed {
			name = "gerenciado"
		}
		for _, tail := range []string{"", "\x18depois\x1b]nova"} {
			testName := name + "/EOF"
			if tail != "" {
				testName = name + "/dados_e_EOF"
			}
			t.Run(testName, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					reader, writer := io.Pipe()
					defer func() { _ = writer.Close() }()
					defer func() { _ = reader.Close() }()
					reading := make(chan struct{})
					exited := make(chan struct{})
					release := make(chan struct{})
					unblock := sync.OnceFunc(func() { close(release) })
					defer unblock()
					var mu sync.Mutex
					var events []string
					s := &Session{readerCtx: t.Context(), ptyReader: reader, ptyWriter: io.Discard}
					s.rawCommandID = "anterior"
					s.onRawOutput = func(_, owner, chunk string) {
						if chunk == "antes" {
							close(reading)
							<-release
						}
						mu.Lock()
						events = append(events, owner+"/"+chunk)
						mu.Unlock()
					}
					s.onExit = func(_ string, _ error) {
						mu.Lock()
						events = append(events, "fim")
						mu.Unlock()
						close(exited)
					}
					s.onCommandStart = func(_, id, _, _ string) {
						mu.Lock()
						events = append(events, "início")
						mu.Unlock()
						// O leitor deve conseguir drenar inclusive enquanto este callback
						// aguarda o encerramento. A saída pode pertencer ao novo dono.
						if tail != "" {
							_, _ = io.WriteString(writer, tail)
						}
						_ = writer.Close()
						<-s.readerDone
					}
					s.Start()
					if _, err := io.WriteString(writer, "antes\xe2\x1bPpendente"); err != nil {
						t.Fatal(err)
					}
					select {
					case <-reading:
					case <-time.After(time.Second):
						t.Fatal("leitura anterior não iniciou")
					}
					done := make(chan struct{})
					go func() {
						defer close(done)
						if managed {
							_, _ = s.RunCommand(t.Context(), "comando", time.Second, "teste", "novo")
						} else {
							_, _ = s.SendInput("comando", "novo")
						}
					}()
					synctest.Wait()
					if s.State() != StateRunning || len(events) != 0 {
						t.Fatalf("antes de liberar a entrega: estado=%v eventos=%v", s.State(), events)
					}
					unblock()
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Fatal("command_start bloqueou a drenagem do leitor")
					}
					select {
					case <-exited:
					case <-time.After(time.Second):
						t.Fatal("encerramento não foi entregue após a última saída")
					}
					mu.Lock()
					defer mu.Unlock()
					want := []string{"anterior/antes", "anterior/\uFFFD", "início", "fim"}
					if managed {
						// Sem despacho, inclusive o trecho incompleto continua manual até o EOF.
						want = []string{"anterior/antes", "início", "anterior/\uFFFD\x1bPpendente", "fim"}
						if tail != "" {
							want = []string{"anterior/antes", "início", "anterior/\uFFFDdepois", "anterior/\x1b]nova", "fim"}
						}
					} else if tail != "" {
						want = []string{"anterior/antes", "anterior/\uFFFD", "início", "novo/depois", "novo/\x1b]nova", "fim"}
					}
					if !slices.Equal(events, want) {
						t.Fatalf("eventos=%q esperados=%q", events, want)
					}
				})
			})
		}
	}
}

type manualTestWriter func([]byte) (int, error)

func (write manualTestWriter) Write(p []byte) (int, error) { return write(p) }

func TestSendInputPreservesReportedCRAcrossOwners(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reads []string
		want  string
	}{
		{"LF completa o CR de A", []string{"\nb"}, "b"},
		{"texto antes de LF", []string{"b\nc"}, "b\nc"},
		{"runa dividida antes de LF", []string{"\xc3", "\xa9\nb"}, "é\nb"},
		{"novo CRLF", []string{"\r\nb"}, "\nb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			var owner string
			s := &Session{ptyWriter: io.Discard,
				onCommandStart: func(_, id, _, _ string) {
					owner = id
					events = append(events, "início/"+id)
				},
				onRawOutput: func(_, _, text string) { events = append(events, owner+"/"+text) },
			}
			if _, err := s.SendInput("primeiro", "A"); err != nil {
				t.Fatal(err)
			}
			s.recordOutput("a\r")
			if _, err := s.SendInput("segundo", "B"); err != nil {
				t.Fatal(err)
			}
			for _, chunk := range tc.reads {
				s.recordOutput(chunk)
			}
			s.finishRawOutput()
			want := []string{"início/A", "A/a\n", "início/B", "B/" + tc.want}
			if !slices.Equal(events, want) {
				t.Fatalf("eventos=%q esperados=%q", events, want)
			}
		})
	}
}

func TestManagedAnnouncementKeepsManualUTF8UntilDispatch(t *testing.T) {
	var manual strings.Builder
	s := &Session{onRawOutput: func(_, id, text string) {
		if id != "A" {
			t.Errorf("destino da saída manual=%q", id)
		}
		manual.WriteString(text)
	}}
	s.rawCommandID = "A"
	s.recordOutput("\xc3")
	s.onCommandStart = func(_, _, _, _ string) {
		s.recordOutput("\xa9\xe2")
	}
	s.startManagedOutput(t.Context(), &HistoryEntry{ID: "B"})
	s.beginMarkerCapture(newCommandMarkerForShell("bash"))
	s.recordOutput("controle")
	if manual.String() != "é\uFFFD" {
		t.Errorf("saída anterior ao despacho=%q", manual.String())
	}
}

func TestSessionOutputAfterFooterAcrossReads(t *testing.T) {
	m := newCommandMarkerForShell("bash")
	body := m.startTag + "\ntexto!\n" + m.endTag + "7!"
	tail := "\r\n\r\n\x1b[31maviso😀!\x1b[0m"
	for cut := 0; cut <= len(tail); cut++ {
		s := &Session{shell: "bash"}
		var output strings.Builder
		s.onRawOutput = func(_, id, chunk string) {
			if id != "" {
				t.Errorf("corte %d: saída da sessão atribuída a %q", cut, id)
			}
			output.WriteString(chunk)
		}
		s.beginMarkerCapture(m)
		stream := newMarkerStream(m)
		_, result := stream.append(body + tail[:cut])
		if !result.Found || result.ExitCode != 7 || result.Output != "texto!" {
			t.Fatalf("corte %d: resultado=%+v", cut, result)
		}
		s.resumeRawAfterFooter(result)
		s.recordOutput(tail[cut:])
		s.finishRawOutput()
		s.rawOwner.wait()
		if got := output.String(); got != "\naviso😀!" {
			t.Fatalf("corte %d: saída da sessão=%q", cut, got)
		}
	}
}

func TestManualOutputAcrossManagedCommand(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	manualStarted := make(chan struct{}, 1)
	var manual strings.Builder
	var destinations []string
	s := &Session{shell: "bash", readerCtx: t.Context(), ptyReader: reader,
		onRawOutput: func(_, id, chunk string) {
			destinations = append(destinations, id)
			manual.WriteString(chunk)
			if chunk == "antes\n" {
				manualStarted <- struct{}{}
			}
		},
	}
	s.rawCommandID = "manual"
	// Complete a runa no intervalo entre a retirada do lote com o rodapé e a
	// liberação da captura gerenciada pelo ticker.
	s.onOutput = func(_, _, _ string) { s.recordOutput("\x82\xac\x1bPoculto") }
	s.ptyWriter = manualTestWriter(func(p []byte) (int, error) {
		_, tail, found := strings.Cut(string(p), "'"+markerStart)
		if !found {
			return 0, errors.New("falta marcador inicial na moldura")
		}
		suffix, _, _ := strings.Cut(tail, "'")
		_, err := io.WriteString(writer, "\a\n"+markerStart+suffix+"\nexecutado\n"+markerEnd+suffix+"0!\r\n\x1b[31maviso\x1b[0m\xe2")
		return len(p), err
	})
	s.Start()
	if _, err := io.WriteString(writer, "antes\r\x1b]0;título"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manualStarted:
	case <-time.After(time.Second):
		t.Fatal("saída manual inicial não chegou")
	}
	entry, err := s.RunCommand(t.Context(), "echo executado", time.Second, "teste", "teste")
	if err != nil || entry == nil || entry.ExitCode != 0 || entry.Output != "executado" {
		t.Fatalf("comando=%+v erro=%v", entry, err)
	}
	if _, err := io.WriteString(writer, "ainda oculto\x18\ndepois"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	<-s.readerDone
	s.rawOwner.wait()
	if !slices.Equal(destinations, []string{"manual", "", ""}) {
		t.Errorf("destinos da saída antes e depois do comando gerenciado=%v", destinations)
	}
	if got := manual.String(); got != "antes\naviso€\ndepois" {
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
			onRawOutput: func(_, _, chunk string) { out.WriteString(chunk) },
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
