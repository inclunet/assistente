// Package filemanager - implementação do provider de armazenamento local
package filemanager

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LocalStorageProvider implementa StorageProvider para o sistema de arquivos local
type LocalStorageProvider struct {
	registry  *FormatRegistry
	security  *SecurityValidator
}

// NewLocalStorageProvider cria um novo provider local
func NewLocalStorageProvider() *LocalStorageProvider {
	return &LocalStorageProvider{
		registry: NewFormatRegistry(),
		security: NewSecurityValidator(nil),
	}
}

// SetSecurityValidator permite configurar o validador de segurança
func (p *LocalStorageProvider) SetSecurityValidator(sv *SecurityValidator) {
	p.security = sv
}

// Name retorna o nome do provider
func (p *LocalStorageProvider) Name() string {
	return "local"
}

// Scheme retorna o esquema de URL (vazio para local)
func (p *LocalStorageProvider) Scheme() string {
	return ""
}

// IsAvailable retorna true - sistema de arquivos local sempre disponível
func (p *LocalStorageProvider) IsAvailable() bool {
	return true
}

// CanWrite retorna true - suporta escrita
func (p *LocalStorageProvider) CanWrite() bool {
	return true
}

// CanDelete retorna true - suporta exclusão
func (p *LocalStorageProvider) CanDelete() bool {
	return true
}

// ReadFile lê conteúdo de um arquivo local
func (p *LocalStorageProvider) ReadFile(ctx context.Context, path string, opts ReadOptions) (*Content, error) {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpRead); err != nil {
		return nil, err
	}
	
	// Obtém handler apropriado
	handler := p.registry.GetHandler(path)
	if handler == nil {
		return nil, fmt.Errorf("nenhum handler disponível para: %s", path)
	}
	
	return handler.ReadContent(path, opts)
}

// GetFileInfo obtém informações de um arquivo local
func (p *LocalStorageProvider) GetFileInfo(ctx context.Context, path string) (*RemoteFileInfo, error) {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpInfo); err != nil {
		return nil, err
	}
	
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	
	absPath, _ := filepath.Abs(path)
	ext := strings.ToLower(filepath.Ext(path))
	category := GetCategoryByExtension(ext)
	mimeType := GetMimeTypeByExtension(ext)
	
	return &RemoteFileInfo{
		Path:       absPath,
		Name:       info.Name(),
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		SizeHuman:  FormatSize(info.Size()),
		MimeType:   mimeType,
		Category:   category,
		ModifiedAt: info.ModTime(),
		Provider:   "local",
	}, nil
}

// ListDirectory lista arquivos em um diretório local
func (p *LocalStorageProvider) ListDirectory(ctx context.Context, path string, opts ListOptions) ([]RemoteFileInfo, error) {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpList); err != nil {
		return nil, err
	}
	
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	
	var files []RemoteFileInfo
	absPath, _ := filepath.Abs(path)
	
	for _, entry := range entries {
		// Filtra arquivos ocultos se necessário
		if !opts.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		
		entryPath := filepath.Join(absPath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		category := GetCategoryByExtension(ext)
		
		files = append(files, RemoteFileInfo{
			Path:       entryPath,
			Name:       entry.Name(),
			IsDir:      entry.IsDir(),
			Size:       info.Size(),
			SizeHuman:  FormatSize(info.Size()),
			Category:   category,
			ModifiedAt: info.ModTime(),
			Provider:   "local",
		})
		
		// Limita resultados se especificado
		if opts.MaxResults > 0 && len(files) >= opts.MaxResults {
			break
		}
	}
	
	return files, nil
}

// SearchByName busca arquivos por nome usando glob pattern
func (p *LocalStorageProvider) SearchByName(ctx context.Context, basePath string, pattern string, opts SearchOptions) ([]RemoteFileInfo, error) {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(basePath, OpList); err != nil {
		return nil, err
	}
	
	var results []RemoteFileInfo
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	
	// Converte glob pattern para regex se necessário
	var matchFn func(name string) bool
	if opts.UseRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("regex inválido: %w", err)
		}
		matchFn = re.MatchString
	} else {
		matchFn = func(name string) bool {
			matched, _ := filepath.Match(pattern, name)
			return matched
		}
	}
	
	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Ignora erros de acesso
		}
		
		// Verifica cancelamento
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Verifica match
		if matchFn(d.Name()) {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			
			ext := strings.ToLower(filepath.Ext(d.Name()))
			category := GetCategoryByExtension(ext)
			
			results = append(results, RemoteFileInfo{
				Path:       path,
				Name:       d.Name(),
				IsDir:      d.IsDir(),
				Size:       info.Size(),
				SizeHuman:  FormatSize(info.Size()),
				Category:   category,
				ModifiedAt: info.ModTime(),
				Provider:   "local",
			})
			
			if len(results) >= maxResults {
				return fs.SkipAll
			}
		}
		
		return nil
	})
	
	return results, err
}

// SearchByContent busca arquivos por conteúdo
func (p *LocalStorageProvider) SearchByContent(ctx context.Context, basePath string, query string, opts SearchOptions) ([]SearchResult, error) {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(basePath, OpList); err != nil {
		return nil, err
	}
	
	var results []SearchResult
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	
	// Compila regex se necessário
	var searchPattern *regexp.Regexp
	var err error
	if opts.UseRegex {
		if opts.CaseSensitive {
			searchPattern, err = regexp.Compile(query)
		} else {
			searchPattern, err = regexp.Compile("(?i)" + query)
		}
		if err != nil {
			return nil, fmt.Errorf("regex inválido: %w", err)
		}
	}
	
	err = filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		
		// Verifica cancelamento
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Só busca em arquivos de texto
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !IsTextFile(ext) {
			return nil
		}
		
		// Lê conteúdo
		handler := p.registry.GetHandler(path)
		if handler == nil {
			return nil
		}
		
		content, err := handler.ReadContent(path, ReadOptions{})
		if err != nil {
			return nil
		}
		
		// Busca no conteúdo
		lines := strings.Split(content.Text, "\n")
		var matches []SearchMatch
		
		for lineNum, line := range lines {
			var found bool
			if searchPattern != nil {
				found = searchPattern.MatchString(line)
			} else if opts.CaseSensitive {
				found = strings.Contains(line, query)
			} else {
				found = strings.Contains(strings.ToLower(line), strings.ToLower(query))
			}
			
			if found {
				match := SearchMatch{
					Text:       line,
					LineNumber: lineNum + 1,
				}
				
				// Adiciona contexto
				if opts.ContextLines > 0 {
					start := lineNum - opts.ContextLines
					if start < 0 {
						start = 0
					}
					end := lineNum + opts.ContextLines + 1
					if end > len(lines) {
						end = len(lines)
					}
					if start < lineNum {
						match.ContextBefore = lines[start:lineNum]
					}
					if lineNum+1 < end {
						match.ContextAfter = lines[lineNum+1 : end]
					}
				}
				
				matches = append(matches, match)
			}
		}
		
		if len(matches) > 0 {
			info, _ := d.Info()
			results = append(results, SearchResult{
				File: FileInfo{
					Path:       path,
					Name:       d.Name(),
					Extension:  ext,
					Size:       info.Size(),
					ModifiedAt: info.ModTime(),
				},
				Matches: matches,
			})
			
			if len(results) >= maxResults {
				return fs.SkipAll
			}
		}
		
		return nil
	})
	
	return results, err
}

// WriteFile escreve conteúdo em um arquivo local
func (p *LocalStorageProvider) WriteFile(ctx context.Context, path string, content *Content, opts WriteOptions) error {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpWrite); err != nil {
		return err
	}
	
	// Cria diretórios se necessário
	if opts.CreateDirs {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("erro ao criar diretório: %w", err)
		}
	}
	
	// Verifica se arquivo existe e se pode sobrescrever
	if _, err := os.Stat(path); err == nil && !opts.Overwrite {
		return fmt.Errorf("arquivo já existe: %s", path)
	}
	
	// Escreve usando handler se disponível, ou texto direto
	handler := p.registry.GetHandler(path)
	if handler != nil {
		return handler.WriteContent(path, content, opts)
	}
	
	// Fallback: escreve texto direto
	return os.WriteFile(path, []byte(content.Text), 0644)
}

// CreateDirectory cria um diretório
func (p *LocalStorageProvider) CreateDirectory(ctx context.Context, path string) error {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpWrite); err != nil {
		return err
	}
	
	return os.MkdirAll(path, 0755)
}

// DeleteFile exclui um arquivo ou diretório
func (p *LocalStorageProvider) DeleteFile(ctx context.Context, path string) error {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpDelete); err != nil {
		return err
	}
	
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// ==================== Helpers ====================

// GetMimeTypeByExtension está definida em detector.go

// ResolvePath resolve um caminho relativo baseado em um diretório de trabalho
func ResolvePath(path string, workingDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workingDir, path))
}

// ReadLines lê linhas específicas de um arquivo
func (p *LocalStorageProvider) ReadLines(ctx context.Context, path string, startLine, endLine int) (*LinesResult, error) {
	// Valida segurança
	if err := p.security.ValidatePathForOperation(path, OpRead); err != nil {
		return nil, err
	}
	
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var lines []LineInfo
	var allLines []string
	lineNum := 0
	
	// Lê todas as linhas para contar total
	scanner := newScanner(file)
	for scanner.Scan() {
		lineNum++
		allLines = append(allLines, scanner.Text())
	}
	
	totalLines := len(allLines)
	
	// Ajusta range
	if startLine < 1 {
		startLine = 1
	}
	if endLine > totalLines {
		endLine = totalLines
	}
	if endLine < startLine {
		endLine = startLine
	}
	
	// Extrai linhas do range
	for i := startLine - 1; i < endLine && i < len(allLines); i++ {
		lines = append(lines, LineInfo{
			Number: i + 1,
			Text:   allLines[i],
		})
	}
	
	// Monta texto bruto
	var rawText strings.Builder
	for _, line := range lines {
		rawText.WriteString(line.Text + "\n")
	}
	
	return &LinesResult{
		Path:       path,
		StartLine:  startLine,
		EndLine:    endLine,
		TotalLines: totalLines,
		Content:    lines,
		RawText:    rawText.String(),
	}, nil
}

// newScanner cria um scanner com buffer grande para linhas longas
func newScanner(file *os.File) *bufScanner {
	return &bufScanner{file: file}
}

// bufScanner é um scanner simples que lê linha por linha
type bufScanner struct {
	file    *os.File
	text    string
	err     error
	started bool
}

func (s *bufScanner) Scan() bool {
	if !s.started {
		s.started = true
		s.file.Seek(0, 0)
	}
	
	var line strings.Builder
	buf := make([]byte, 1)
	
	for {
		n, err := s.file.Read(buf)
		if n == 0 || err != nil {
			if line.Len() > 0 {
				s.text = line.String()
				return true
			}
			return false
		}
		
		if buf[0] == '\n' {
			s.text = strings.TrimSuffix(line.String(), "\r")
			return true
		}
		
		line.WriteByte(buf[0])
	}
}

func (s *bufScanner) Text() string {
	return s.text
}

