package filemanager

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// FormatRegistry gerencia todos os handlers de formato
type FormatRegistry struct {
	handlers       map[string]FormatHandler
	mimeTypes      map[string]FormatHandler
	allHandlers    []FormatHandler
	defaultHandler FormatHandler
	mu             sync.RWMutex
}

// NewFormatRegistry cria um novo registry com handlers padrão
func NewFormatRegistry() *FormatRegistry {
	r := &FormatRegistry{
		handlers:  make(map[string]FormatHandler),
		mimeTypes: make(map[string]FormatHandler),
	}

	// Registra handlers padrão
	plaintext := NewPlainTextHandler()
	r.Register(plaintext)

	// Registra handlers de documentos modernos
	r.Register(NewExcelHandler())      // .xlsx, .xlsm
	r.Register(NewWordHandler())       // .docx
	r.Register(NewPowerPointHandler()) // .pptx
	r.Register(NewPDFHandler())        // .pdf

	// Registra handlers OpenDocument
	r.Register(NewOpenDocumentHandler()) // .odt, .ods, .odp

	// Registra handlers Office legado
	r.Register(NewLegacyOfficeHandler()) // .xls

	// Registra handler EPUB
	r.Register(NewEPUBHandler()) // .epub (todas as versões)

	// Define plaintext como handler padrão (fallback)
	r.defaultHandler = plaintext

	return r
}

// Register adiciona um handler ao registry
func (r *FormatRegistry) Register(handler FormatHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ext := range handler.Extensions() {
		r.handlers[strings.ToLower(ext)] = handler
	}
	for _, mime := range handler.MimeTypes() {
		r.mimeTypes[strings.ToLower(mime)] = handler
	}
	r.allHandlers = append(r.allHandlers, handler)
}

// GetHandler retorna o handler para uma extensão ou mime type
func (r *FormatRegistry) GetHandler(pathOrMime string) FormatHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Tenta por extensão
	ext := strings.ToLower(filepath.Ext(pathOrMime))
	if h, ok := r.handlers[ext]; ok {
		return h
	}

	// Tenta por mime type
	if h, ok := r.mimeTypes[strings.ToLower(pathOrMime)]; ok {
		return h
	}

	// Fallback para handler padrão (plaintext)
	return r.defaultHandler
}

// GetHandlerByExtension retorna o handler para uma extensão específica
func (r *FormatRegistry) GetHandlerByExtension(ext string) FormatHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	if h, ok := r.handlers[ext]; ok {
		return h
	}
	return r.defaultHandler
}

// SupportedFormats retorna todas as extensões suportadas
func (r *FormatRegistry) SupportedFormats() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	formats := make([]string, 0, len(r.handlers))
	for ext := range r.handlers {
		formats = append(formats, ext)
	}
	return formats
}

// CanHandle verifica se um formato é suportado (além do fallback)
func (r *FormatRegistry) CanHandle(path string) bool {
	handler := r.GetHandler(path)
	return handler != nil && handler != r.defaultHandler
}

// ReadContent lê conteúdo de qualquer formato suportado
func (r *FormatRegistry) ReadContent(path string, opts ReadOptions) (*Content, error) {
	handler := r.GetHandler(path)
	if handler == nil {
		return nil, fmt.Errorf("formato não suportado: %s", filepath.Ext(path))
	}
	return handler.ReadContent(path, opts)
}

// WriteContent escreve conteúdo em qualquer formato suportado
func (r *FormatRegistry) WriteContent(path string, content *Content, opts WriteOptions) error {
	handler := r.GetHandler(path)
	if handler == nil {
		return fmt.Errorf("formato não suportado: %s", filepath.Ext(path))
	}

	caps := handler.Capabilities()
	if !caps.CanWrite {
		return fmt.Errorf("formato %s não suporta escrita", filepath.Ext(path))
	}

	return handler.WriteContent(path, content, opts)
}

// SearchContent busca conteúdo em qualquer formato suportado
func (r *FormatRegistry) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	handler := r.GetHandler(path)
	if handler == nil {
		return nil, fmt.Errorf("formato não suportado: %s", filepath.Ext(path))
	}

	caps := handler.Capabilities()
	if !caps.CanSearch {
		return nil, fmt.Errorf("formato %s não suporta busca", filepath.Ext(path))
	}

	return handler.SearchContent(path, query, opts)
}

// GetMetadata obtém metadados de qualquer formato suportado
func (r *FormatRegistry) GetMetadata(path string) (map[string]interface{}, error) {
	handler := r.GetHandler(path)
	if handler == nil {
		return nil, fmt.Errorf("formato não suportado: %s", filepath.Ext(path))
	}
	return handler.GetMetadata(path)
}

// GetAllHandlers retorna todos os handlers registrados
func (r *FormatRegistry) GetAllHandlers() []FormatHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FormatHandler, len(r.allHandlers))
	copy(result, r.allHandlers)
	return result
}
