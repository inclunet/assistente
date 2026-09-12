package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const logFileFlag = "--log-file"

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
				return "", nil, fmt.Errorf("%s requer um caminho", logFileFlag)
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
			return "", nil, fmt.Errorf("%s requer um caminho", logFileFlag)
		}
		if path != "" {
			return "", nil, fmt.Errorf("%s foi informado mais de uma vez", logFileFlag)
		}
		path = value
	}

	return path, remaining, nil
}

// FileOutput duplica os logs globais em um arquivo sem retirar a saída atual.
type FileOutput struct {
	file              *os.File
	previousLogWriter io.Writer
}

// OpenFileOutput abre path para anexação e passa a duplicar slog e log padrão.
func OpenFileOutput(path string) (*FileOutput, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%s requer um caminho", logFileFlag)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir o arquivo de log %q: %w", path, err)
	}

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
