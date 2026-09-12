package logging

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

const logFileFlag = "--log-file"

var (
	ErrLogFilePathRequired = errors.New("--log-file requer um caminho")
	ErrLogFileRepeated     = errors.New("--log-file foi informado mais de uma vez")
)

// FileOpenError preserva o caminho e o erro do sistema para apresentação
// localizada pelo entrypoint.
type FileOpenError struct {
	Path string
	Err  error
}

func (e *FileOpenError) Error() string {
	return fmt.Sprintf("não foi possível abrir o arquivo de log %q: %v", e.Path, e.Err)
}

func (e *FileOpenError) Unwrap() error {
	return e.Err
}

// ParseLogFileArgs extrai a opção exclusiva do executável gráfico e preserva
// os demais argumentos para o Wails.
func ParseLogFileArgs(args []string) (string, []string, error) {
	remaining := make([]string, 0, len(args))
	var path string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		var value string
		switch {
		case arg == logFileFlag:
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return "", nil, ErrLogFilePathRequired
			}
			index++
			value = args[index]
		case strings.HasPrefix(arg, logFileFlag+"="):
			value = strings.TrimPrefix(arg, logFileFlag+"=")
		default:
			remaining = append(remaining, arg)
			continue
		}

		if strings.TrimSpace(value) == "" {
			return "", nil, ErrLogFilePathRequired
		}
		if path != "" {
			return "", nil, ErrLogFileRepeated
		}
		path = value
	}

	return path, remaining, nil
}

// FileOutput duplica os logs globais em um arquivo sem retirar a saída atual.
type FileOutput struct {
	file              *fileSink
	previousLogWriter io.Writer
}

// OpenFileOutput abre path para anexação e passa a duplicar slog e log padrão.
func OpenFileOutput(path string) (*FileOutput, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrLogFilePathRequired
	}

	logFile, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, &FileOpenError{Path: path, Err: err}
	}
	file := &fileSink{file: logFile}

	output := &FileOutput{
		file:              file,
		previousLogWriter: log.Writer(),
	}
	// O handler padrão de slog, ativo no início do executável gráfico, escreve
	// pelo pacote log. Assim, este tee preserva formato e nível e captura ambos
	// sem instalar outro handler nem alterar a sanitização.
	log.SetOutput(DuplicateTo(output.previousLogWriter, file))
	return output, nil
}

// DuplicateTo preserva primary e sempre tenta gravar também em secondary.
// Diferentemente de io.MultiWriter, uma saída gráfica sem console não impede
// a escrita no arquivo quando stdout/stderr retorna erro no Windows.
func DuplicateTo(primary, secondary io.Writer) io.Writer {
	return duplicateWriter{primary: primary, secondary: secondary}
}

type duplicateWriter struct {
	primary   io.Writer
	secondary io.Writer
}

func (w duplicateWriter) Write(p []byte) (int, error) {
	if w.primary != nil {
		_, _ = w.primary.Write(p)
	}
	written, err := w.secondary.Write(p)
	if err != nil {
		return written, err
	}
	if written != len(p) {
		return written, io.ErrShortWrite
	}
	return len(p), nil
}

// fileSink serializa gravações e se torna um writer inofensivo após Close.
// Loggers com ciclo de vida próprio (como o GORM) podem manter esta referência
// sem tentar escrever em um descritor já fechado durante o shutdown.
type fileSink struct {
	mu   sync.Mutex
	file *os.File
}

func (s *fileSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return len(p), nil
	}
	return s.file.Write(p)
}

func (s *fileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// Writer retorna o destino do arquivo para loggers que mantêm writer próprio,
// como o logger padrão do GORM.
func (o *FileOutput) Writer() io.Writer {
	if o == nil {
		return nil
	}
	return o.file
}

// Close restaura os destinos globais anteriores e fecha o arquivo.
func (o *FileOutput) Close() error {
	if o == nil || o.file == nil {
		return nil
	}
	log.SetOutput(o.previousLogWriter)
	err := o.file.Close()
	o.file = nil
	return err
}
