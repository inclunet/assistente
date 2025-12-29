package filemanager

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PlainTextHandler manipula arquivos de texto simples
type PlainTextHandler struct{}

// NewPlainTextHandler cria um novo handler para arquivos de texto
func NewPlainTextHandler() *PlainTextHandler {
	return &PlainTextHandler{}
}

// Name retorna o nome do handler
func (h *PlainTextHandler) Name() string {
	return "plaintext"
}

// Extensions retorna as extensões suportadas
func (h *PlainTextHandler) Extensions() []string {
	return []string{
		".txt", ".md", ".markdown",
		".json", ".xml", ".yaml", ".yml",
		".ini", ".cfg", ".conf", ".config",
		".log", ".env", ".properties",
		".csv", ".tsv",
		".html", ".htm", ".css", ".scss", ".less",
		".js", ".ts", ".jsx", ".tsx", ".vue", ".svelte",
		".go", ".py", ".rb", ".php", ".java", ".c", ".cpp", ".h", ".hpp",
		".cs", ".rs", ".swift", ".kt", ".scala",
		".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat", ".cmd",
		".sql", ".graphql", ".gql",
		".toml", ".dockerfile", ".makefile",
		".gitignore", ".gitattributes", ".editorconfig",
	}
}

// MimeTypes retorna os MIME types suportados
func (h *PlainTextHandler) MimeTypes() []string {
	return []string{
		"text/plain",
		"text/markdown",
		"text/html",
		"text/css",
		"text/csv",
		"text/xml",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-yaml",
		"text/x-go",
		"text/x-python",
		"text/x-java",
		"text/x-c",
		"text/x-cpp",
	}
}

// Capabilities retorna as capacidades do handler
func (h *PlainTextHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:   true,
		CanWrite:  true,
		CanEdit:   true,
		CanSearch: true,
	}
}

// ReadContent lê o conteúdo de um arquivo de texto
func (h *PlainTextHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Aplica limite de bytes se especificado
	if opts.MaxBytes > 0 && int64(len(data)) > opts.MaxBytes {
		data = data[:opts.MaxBytes]
	}

	// Detecta encoding
	encoding := DetectEncoding(data)
	text := DecodeToUTF8(data, encoding)

	// Conta linhas
	lineCount := strings.Count(text, "\n")
	if len(text) > 0 && !strings.HasSuffix(text, "\n") {
		lineCount++
	}

	return &Content{
		Text:      text,
		Encoding:  encoding,
		LineCount: lineCount,
	}, nil
}

// WriteContent escreve conteúdo em um arquivo de texto
func (h *PlainTextHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	// Cria diretórios se necessário
	if opts.CreateDirs {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Verifica se deve sobrescrever
	if !opts.Overwrite {
		if _, err := os.Stat(path); err == nil {
			return os.ErrExist
		}
	}

	// Codifica para o encoding desejado
	data := []byte(content.Text)
	if opts.Encoding != "" && opts.Encoding != EncodingUTF8 {
		encoded, err := EncodeFromUTF8(content.Text, opts.Encoding)
		if err != nil {
			return err
		}
		data = encoded
	}

	return os.WriteFile(path, data, 0644)
}

// SearchContent busca conteúdo dentro do arquivo
func (h *PlainTextHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []SearchMatch
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	// Prepara a busca
	var searchFunc func(string) (bool, []int)
	if opts.UseRegex {
		flags := ""
		if !opts.CaseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + query)
		if err != nil {
			return nil, err
		}
		searchFunc = func(line string) (bool, []int) {
			loc := re.FindStringIndex(line)
			if loc != nil {
				return true, loc
			}
			return false, nil
		}
	} else {
		searchQuery := query
		if !opts.CaseSensitive {
			searchQuery = strings.ToLower(query)
		}
		searchFunc = func(line string) (bool, []int) {
			searchLine := line
			if !opts.CaseSensitive {
				searchLine = strings.ToLower(line)
			}
			idx := strings.Index(searchLine, searchQuery)
			if idx >= 0 {
				return true, []int{idx, idx + len(query)}
			}
			return false, nil
		}
	}

	// Buffer para linhas de contexto
	var contextBuffer []string
	maxContext := opts.ContextLines
	if maxContext <= 0 {
		maxContext = 2
	}

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Mantém buffer de contexto
		contextBuffer = append(contextBuffer, line)
		if len(contextBuffer) > maxContext*2+1 {
			contextBuffer = contextBuffer[1:]
		}

		found, loc := searchFunc(line)
		if found {
			match := SearchMatch{
				Text:        query,
				Context:     line,
				LineNumber:  lineNumber,
				ColumnStart: loc[0] + 1,
				ColumnEnd:   loc[1] + 1,
			}
			matches = append(matches, match)

			// Verifica limite de resultados
			if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

// GetMetadata obtém metadados do arquivo
func (h *PlainTextHandler) GetMetadata(path string) (map[string]interface{}, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Lê primeiros bytes para detectar encoding
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	header := make([]byte, 1024)
	n, _ := file.Read(header)
	encoding := DetectEncoding(header[:n])

	// Conta linhas
	file.Seek(0, 0)
	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}

	return map[string]interface{}{
		"size":       info.Size(),
		"modified":   info.ModTime(),
		"encoding":   encoding,
		"line_count": lineCount,
		"is_text":    true,
		"extension":  filepath.Ext(path),
	}, nil
}

