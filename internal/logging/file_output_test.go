package logging

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLogFileArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantPath  string
		wantArgs  []string
		wantError string
	}{
		{
			name:     "argumento separado",
			args:     []string{"--log-file", `C:\logs\assistente.log`, "--debug"},
			wantPath: `C:\logs\assistente.log`,
			wantArgs: []string{"--debug"},
		},
		{
			name:     "argumento com igual",
			args:     []string{"--debug", "--log-file=assistente.log"},
			wantPath: "assistente.log",
			wantArgs: []string{"--debug"},
		},
		{
			name:     "sem flag",
			args:     []string{"--debug"},
			wantArgs: []string{"--debug"},
		},
		{
			name:      "caminho ausente",
			args:      []string{"--log-file"},
			wantError: "--log-file requer um caminho",
		},
		{
			name:      "valor vazio",
			args:      []string{"--log-file="},
			wantError: "--log-file requer um caminho",
		},
		{
			name:      "flag repetida",
			args:      []string{"--log-file=um.log", "--log-file", "dois.log"},
			wantError: "--log-file foi informado mais de uma vez",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, args, err := ParseLogFileArgs(test.args)
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("erro = %v, esperado %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLogFileArgs() retornou erro: %v", err)
			}
			if path != test.wantPath {
				t.Fatalf("path = %q, esperado %q", path, test.wantPath)
			}
			if strings.Join(args, "\x00") != strings.Join(test.wantArgs, "\x00") {
				t.Fatalf("args = %#v, esperado %#v", args, test.wantArgs)
			}
		})
	}
}

func TestOpenFileOutputDuplicaLogsSemRemoverSaidaAtual(t *testing.T) {
	previousLogWriter := log.Writer()
	previousSlog := slog.Default()
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
		slog.SetDefault(previousSlog)
	})

	var terminalOutput bytes.Buffer
	log.SetOutput(&terminalOutput)

	path := filepath.Join(t.TempDir(), "assistente.log")
	if err := os.WriteFile(path, []byte("registro anterior\n"), 0o600); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}

	output, err := OpenFileOutput(path)
	if err != nil {
		t.Fatalf("OpenFileOutput() retornou erro: %v", err)
	}

	log.Print("log padrão capturado")
	Logger(context.Background(), "teste").Info("log estruturado capturado", "chave", "valor")
	Logger(context.Background(), "teste").Debug("debug não deve ser habilitado")

	if err := output.Close(); err != nil {
		t.Fatalf("Close() retornou erro: %v", err)
	}

	if !strings.Contains(terminalOutput.String(), "log padrão capturado") {
		t.Fatalf("saída padrão não foi preservada: %q", terminalOutput.String())
	}
	if !strings.Contains(terminalOutput.String(), "log estruturado capturado") {
		t.Fatalf("saída estruturada não foi preservada: %q", terminalOutput.String())
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler arquivo: %v", err)
	}
	got := string(content)
	for _, expected := range []string{
		"registro anterior",
		"log padrão capturado",
		"log estruturado capturado",
		"component=teste",
		"chave=valor",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("arquivo não contém %q:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "debug não deve ser habilitado") {
		t.Errorf("arquivo alterou o nível atual de slog:\n%s", got)
	}
}

func TestOpenFileOutputRejeitaArquivoInacessivel(t *testing.T) {
	_, err := OpenFileOutput(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "não foi possível abrir o arquivo de log") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestDuplicateToGravaArquivoMesmoSemConsole(t *testing.T) {
	var file bytes.Buffer
	writer := DuplicateTo(failingWriter{}, &file)

	written, err := writer.Write([]byte("registro sem console"))
	if err != nil {
		t.Fatalf("Write() retornou erro: %v", err)
	}
	if written != len("registro sem console") {
		t.Fatalf("Write() = %d bytes", written)
	}
	if file.String() != "registro sem console" {
		t.Fatalf("arquivo recebeu %q", file.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("console indisponível")
}
