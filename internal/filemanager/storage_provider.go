// Package filemanager - interface unificada para provedores de armazenamento
package filemanager

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Cache de regex compilados
var regexCache = make(map[string]*regexp.Regexp)

func mustCompile(pattern string) *regexp.Regexp {
	if re, ok := regexCache[pattern]; ok {
		return re
	}
	re := regexp.MustCompile(pattern)
	regexCache[pattern] = re
	return re
}

// StorageProvider define a interface para provedores de armazenamento (local, Google Drive, OneDrive, etc.)
type StorageProvider interface {
	// Identificação
	Name() string                    // Nome do provider (ex: "local", "gdrive", "onedrive")
	Scheme() string                  // Esquema de URL (ex: "", "gdrive://", "onedrive://")
	IsAvailable() bool               // Verifica se o provider está configurado e disponível

	// Operações de leitura
	ReadFile(ctx context.Context, path string, opts ReadOptions) (*Content, error)
	GetFileInfo(ctx context.Context, path string) (*RemoteFileInfo, error)
	
	// Operações de listagem
	ListDirectory(ctx context.Context, path string, opts ListOptions) ([]RemoteFileInfo, error)
	
	// Operações de busca
	SearchByName(ctx context.Context, basePath string, pattern string, opts SearchOptions) ([]RemoteFileInfo, error)
	SearchByContent(ctx context.Context, basePath string, query string, opts SearchOptions) ([]SearchResult, error)
	
	// Operações de escrita (opcionais - nem todos os providers suportam)
	WriteFile(ctx context.Context, path string, content *Content, opts WriteOptions) error
	CreateDirectory(ctx context.Context, path string) error
	DeleteFile(ctx context.Context, path string) error
	
	// Capacidades
	CanWrite() bool
	CanDelete() bool
}

// RemoteFileInfo representa informações de um arquivo em qualquer provider
type RemoteFileInfo struct {
	ID           string            `json:"id,omitempty"`      // ID único (para providers de nuvem)
	Path         string            `json:"path"`              // Caminho ou URL
	Name         string            `json:"name"`
	IsDir        bool              `json:"is_dir"`
	Size         int64             `json:"size"`
	SizeHuman    string            `json:"size_human,omitempty"`
	MimeType     string            `json:"mime_type,omitempty"`
	Category     FileCategory      `json:"category"`
	ModifiedAt   time.Time         `json:"modified_at"`
	CreatedAt    time.Time         `json:"created_at,omitempty"`
	Provider     string            `json:"provider"`          // "local", "gdrive", "onedrive", etc.
	WebLink      string            `json:"web_link,omitempty"` // URL para abrir no browser (para nuvem)
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ListOptions opções para listagem de diretórios
type ListOptions struct {
	ShowHidden bool   `json:"show_hidden"`
	MaxResults int    `json:"max_results"`
	PageToken  string `json:"page_token,omitempty"` // Para paginação em providers de nuvem
}

// ==================== Storage Manager ====================

// StorageManager gerencia múltiplos providers de armazenamento
type StorageManager struct {
	providers map[string]StorageProvider
	local     StorageProvider // Provider local sempre disponível
}

// NewStorageManager cria um novo gerenciador de armazenamento
func NewStorageManager() *StorageManager {
	sm := &StorageManager{
		providers: make(map[string]StorageProvider),
	}
	// Provider local é sempre registrado
	localProvider := NewLocalStorageProvider()
	sm.local = localProvider
	sm.providers["local"] = localProvider
	sm.providers[""] = localProvider // Path sem scheme = local
	return sm
}

// RegisterProvider registra um novo provider de armazenamento
func (sm *StorageManager) RegisterProvider(provider StorageProvider) {
	sm.providers[provider.Name()] = provider
	if scheme := provider.Scheme(); scheme != "" {
		sm.providers[scheme] = provider
	}
}

// GetProvider retorna o provider apropriado para um path
func (sm *StorageManager) GetProvider(path string) (StorageProvider, string, error) {
	providerName, cleanPath := sm.ParsePath(path)
	
	provider, ok := sm.providers[providerName]
	if !ok {
		return nil, "", fmt.Errorf("provider não encontrado: %s", providerName)
	}
	
	if !provider.IsAvailable() {
		return nil, "", fmt.Errorf("provider %s não está disponível (verifique configurações OAuth)", providerName)
	}
	
	return provider, cleanPath, nil
}

// ParsePath extrai o nome do provider e o caminho limpo de um path
// Exemplos:
//   "C:\docs\file.txt" → ("local", "C:\docs\file.txt")
//   "gdrive://folder-id/file" → ("gdrive", "folder-id/file")
//   "https://docs.google.com/document/d/..." → ("gdrive", "document-id")
//   "onedrive://..." → ("onedrive", "...")
func (sm *StorageManager) ParsePath(path string) (providerName string, cleanPath string) {
	path = strings.TrimSpace(path)
	
	// Detecta URLs do Google
	if strings.Contains(path, "docs.google.com") || strings.Contains(path, "drive.google.com") {
		docID := extractGoogleDocID(path)
		if docID != "" {
			return "gdrive", docID
		}
	}
	
	// Detecta pseudo-schemes
	schemes := []struct {
		prefix   string
		provider string
	}{
		{"gdrive://", "gdrive"},
		{"gdocs://", "gdrive"},
		{"gsheet://", "gdrive"},
		{"gslides://", "gdrive"},
		{"onedrive://", "onedrive"},
		{"dropbox://", "dropbox"},
		{"icloud://", "icloud"},
	}
	
	for _, s := range schemes {
		if strings.HasPrefix(strings.ToLower(path), s.prefix) {
			return s.provider, strings.TrimPrefix(path, s.prefix)
		}
	}
	
	// Se não tem scheme, é local
	return "local", path
}

// IsCloudPath verifica se um path é de um provider de nuvem
func (sm *StorageManager) IsCloudPath(path string) bool {
	providerName, _ := sm.ParsePath(path)
	return providerName != "local"
}

// GetAvailableProviders retorna lista de providers disponíveis
func (sm *StorageManager) GetAvailableProviders() []string {
	var available []string
	seen := make(map[string]bool)
	
	for _, p := range sm.providers {
		if !seen[p.Name()] && p.IsAvailable() {
			available = append(available, p.Name())
			seen[p.Name()] = true
		}
	}
	return available
}

// ==================== Operações Unificadas ====================

// ReadFile lê um arquivo de qualquer provider
func (sm *StorageManager) ReadFile(ctx context.Context, path string, opts ReadOptions) (*Content, error) {
	provider, cleanPath, err := sm.GetProvider(path)
	if err != nil {
		return nil, err
	}
	return provider.ReadFile(ctx, cleanPath, opts)
}

// GetFileInfo obtém informações de um arquivo de qualquer provider
func (sm *StorageManager) GetFileInfo(ctx context.Context, path string) (*RemoteFileInfo, error) {
	provider, cleanPath, err := sm.GetProvider(path)
	if err != nil {
		return nil, err
	}
	return provider.GetFileInfo(ctx, cleanPath)
}

// ListDirectory lista arquivos de qualquer provider
func (sm *StorageManager) ListDirectory(ctx context.Context, path string, opts ListOptions) ([]RemoteFileInfo, error) {
	provider, cleanPath, err := sm.GetProvider(path)
	if err != nil {
		return nil, err
	}
	return provider.ListDirectory(ctx, cleanPath, opts)
}

// SearchByName busca arquivos por nome em qualquer provider
func (sm *StorageManager) SearchByName(ctx context.Context, basePath string, pattern string, opts SearchOptions) ([]RemoteFileInfo, error) {
	provider, cleanPath, err := sm.GetProvider(basePath)
	if err != nil {
		return nil, err
	}
	return provider.SearchByName(ctx, cleanPath, pattern, opts)
}

// SearchByContent busca por conteúdo em qualquer provider
func (sm *StorageManager) SearchByContent(ctx context.Context, basePath string, query string, opts SearchOptions) ([]SearchResult, error) {
	provider, cleanPath, err := sm.GetProvider(basePath)
	if err != nil {
		return nil, err
	}
	return provider.SearchByContent(ctx, cleanPath, query, opts)
}

// WriteFile escreve um arquivo em qualquer provider
func (sm *StorageManager) WriteFile(ctx context.Context, path string, content *Content, opts WriteOptions) error {
	provider, cleanPath, err := sm.GetProvider(path)
	if err != nil {
		return err
	}
	if !provider.CanWrite() {
		return fmt.Errorf("provider %s não suporta escrita", provider.Name())
	}
	return provider.WriteFile(ctx, cleanPath, content, opts)
}

// CreateDirectory cria um diretório em qualquer provider
func (sm *StorageManager) CreateDirectory(ctx context.Context, path string) error {
	provider, cleanPath, err := sm.GetProvider(path)
	if err != nil {
		return err
	}
	if !provider.CanWrite() {
		return fmt.Errorf("provider %s não suporta criação de diretórios", provider.Name())
	}
	return provider.CreateDirectory(ctx, cleanPath)
}

// DeleteFile exclui um arquivo de qualquer provider
func (sm *StorageManager) DeleteFile(ctx context.Context, path string) error {
	provider, cleanPath, err := sm.GetProvider(path)
	if err != nil {
		return err
	}
	if !provider.CanDelete() {
		return fmt.Errorf("provider %s não suporta exclusão", provider.Name())
	}
	return provider.DeleteFile(ctx, cleanPath)
}

// ==================== Helper Functions ====================

// extractGoogleDocID extrai o ID de um documento de uma URL do Google
func extractGoogleDocID(url string) string {
	// Padrões: docs.google.com/document/d/{ID}/...
	//          docs.google.com/spreadsheets/d/{ID}/...
	//          docs.google.com/presentation/d/{ID}/...
	//          drive.google.com/file/d/{ID}/...
	patterns := []string{
		`docs\.google\.com/document/d/([a-zA-Z0-9_-]+)`,
		`docs\.google\.com/spreadsheets/d/([a-zA-Z0-9_-]+)`,
		`docs\.google\.com/presentation/d/([a-zA-Z0-9_-]+)`,
		`drive\.google\.com/file/d/([a-zA-Z0-9_-]+)`,
		`drive\.google\.com/open\?id=([a-zA-Z0-9_-]+)`,
	}
	
	for _, pattern := range patterns {
		re := mustCompile(pattern)
		if matches := re.FindStringSubmatch(url); len(matches) > 1 {
			return matches[1]
		}
	}
	
	return ""
}

// FormatSize está definida em detector.go

