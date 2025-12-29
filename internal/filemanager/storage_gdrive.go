// Package filemanager - implementação do provider Google Drive
package filemanager

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GoogleDriveProvider implementa StorageProvider para Google Drive/Docs
type GoogleDriveProvider struct {
	handler    *GoogleDocsHandler
	getToken   func() (string, error)
	isEnabled  bool
}

// NewGoogleDriveProvider cria um novo provider do Google Drive
func NewGoogleDriveProvider(tokenProvider func() (string, error)) *GoogleDriveProvider {
	p := &GoogleDriveProvider{
		getToken:  tokenProvider,
		isEnabled: tokenProvider != nil,
	}
	
	if tokenProvider != nil {
		p.handler = NewGoogleDocsHandler(tokenProvider)
	}
	
	return p
}

// SetTokenProvider configura o provider de token
func (p *GoogleDriveProvider) SetTokenProvider(tokenProvider func() (string, error)) {
	p.getToken = tokenProvider
	p.isEnabled = tokenProvider != nil
	
	if tokenProvider != nil {
		p.handler = NewGoogleDocsHandler(tokenProvider)
	} else {
		p.handler = nil
	}
}

// Name retorna o nome do provider
func (p *GoogleDriveProvider) Name() string {
	return "gdrive"
}

// Scheme retorna o esquema de URL
func (p *GoogleDriveProvider) Scheme() string {
	return "gdrive://"
}

// IsAvailable verifica se o provider está configurado
func (p *GoogleDriveProvider) IsAvailable() bool {
	return p.isEnabled && p.handler != nil
}

// CanWrite - Google Drive via API pode escrever, mas não implementamos ainda
func (p *GoogleDriveProvider) CanWrite() bool {
	return false // TODO: implementar escrita via API
}

// CanDelete - Google Drive pode deletar, mas não implementamos ainda
func (p *GoogleDriveProvider) CanDelete() bool {
	return false // TODO: implementar exclusão via API
}

// ReadFile lê conteúdo de um documento do Google
func (p *GoogleDriveProvider) ReadFile(ctx context.Context, path string, opts ReadOptions) (*Content, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("Google Drive não está configurado. Conecte sua conta Google nas configurações.")
	}
	
	content, err := p.handler.ReadContent(path, opts)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler documento do Google: %w", err)
	}
	
	return content, nil
}

// GetFileInfo obtém informações de um arquivo do Google Drive
func (p *GoogleDriveProvider) GetFileInfo(ctx context.Context, path string) (*RemoteFileInfo, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("Google Drive não está configurado. Conecte sua conta Google nas configurações.")
	}
	
	metadata, err := p.handler.GetMetadata(path)
	if err != nil {
		return nil, err
	}
	
	// Converte metadata para RemoteFileInfo
	info := &RemoteFileInfo{
		Provider: "gdrive",
	}
	
	if id, ok := metadata["id"].(string); ok {
		info.ID = id
		info.Path = "gdrive://" + id
	}
	if name, ok := metadata["name"].(string); ok {
		info.Name = name
	}
	if mimeType, ok := metadata["mimeType"].(string); ok {
		info.MimeType = mimeType
		info.IsDir = mimeType == MimeGoogleFolder
		info.Category = categorizeGoogleMimeType(mimeType)
	}
	if webLink, ok := metadata["webViewLink"].(string); ok {
		info.WebLink = webLink
	}
	if modified, ok := metadata["modifiedTime"].(time.Time); ok {
		info.ModifiedAt = modified
	}
	if created, ok := metadata["createdTime"].(time.Time); ok {
		info.CreatedAt = created
	}
	if size, ok := metadata["size"].(string); ok {
		info.Metadata = map[string]interface{}{"size": size}
	}
	
	return info, nil
}

// ListDirectory lista arquivos de uma pasta do Google Drive
func (p *GoogleDriveProvider) ListDirectory(ctx context.Context, path string, opts ListOptions) ([]RemoteFileInfo, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("Google Drive não está configurado. Conecte sua conta Google nas configurações.")
	}
	
	token, err := p.getToken()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter token: %w", err)
	}
	
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	
	// path pode ser vazio (root) ou um folder ID
	folderID := path
	if folderID == "" || folderID == "/" || folderID == "root" {
		folderID = ""
	}
	
	files, err := p.handler.ListGoogleDriveFiles(folderID, "", token, maxResults)
	if err != nil {
		return nil, err
	}
	
	var results []RemoteFileInfo
	for _, f := range files {
		results = append(results, RemoteFileInfo{
			ID:         f.ID,
			Path:       "gdrive://" + f.ID,
			Name:       f.Name,
			IsDir:      f.MimeType == MimeGoogleFolder,
			MimeType:   f.MimeType,
			Category:   categorizeGoogleMimeType(f.MimeType),
			ModifiedAt: f.ModifiedTime,
			CreatedAt:  f.CreatedTime,
			Provider:   "gdrive",
			WebLink:    f.WebViewLink,
		})
	}
	
	return results, nil
}

// SearchByName busca arquivos por nome no Google Drive
func (p *GoogleDriveProvider) SearchByName(ctx context.Context, basePath string, pattern string, opts SearchOptions) ([]RemoteFileInfo, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("Google Drive não está configurado. Conecte sua conta Google nas configurações.")
	}
	
	token, err := p.getToken()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter token: %w", err)
	}
	
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	
	// Converte glob pattern para query de busca por nome
	// Remove wildcards para busca simples
	searchQuery := strings.Trim(pattern, "*")
	
	folderID := ""
	if basePath != "" && basePath != "/" {
		folderID = basePath
	}
	
	files, err := p.handler.ListGoogleDriveFiles(folderID, searchQuery, token, maxResults)
	if err != nil {
		return nil, err
	}
	
	var results []RemoteFileInfo
	for _, f := range files {
		results = append(results, RemoteFileInfo{
			ID:         f.ID,
			Path:       "gdrive://" + f.ID,
			Name:       f.Name,
			IsDir:      f.MimeType == MimeGoogleFolder,
			MimeType:   f.MimeType,
			Category:   categorizeGoogleMimeType(f.MimeType),
			ModifiedAt: f.ModifiedTime,
			Provider:   "gdrive",
			WebLink:    f.WebViewLink,
		})
	}
	
	return results, nil
}

// SearchByContent busca por conteúdo no Google Drive (full-text search)
func (p *GoogleDriveProvider) SearchByContent(ctx context.Context, basePath string, query string, opts SearchOptions) ([]SearchResult, error) {
	if !p.IsAvailable() {
		return nil, fmt.Errorf("Google Drive não está configurado. Conecte sua conta Google nas configurações.")
	}
	
	token, err := p.getToken()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter token: %w", err)
	}
	
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 20
	}
	
	files, err := p.handler.SearchGoogleDriveFiles(query, token, maxResults)
	if err != nil {
		return nil, err
	}
	
	var results []SearchResult
	for _, f := range files {
		results = append(results, SearchResult{
			File: FileInfo{
				Path:     "gdrive://" + f.ID,
				Name:     f.Name,
				Category: categorizeGoogleMimeType(f.MimeType),
				MimeType: f.MimeType,
			},
			Matches: []SearchMatch{
				{
					Text:     fmt.Sprintf("Documento contém: %q", query),
					Location: f.WebViewLink,
				},
			},
		})
	}
	
	return results, nil
}

// WriteFile - não implementado ainda
func (p *GoogleDriveProvider) WriteFile(ctx context.Context, path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita no Google Drive ainda não implementada")
}

// CreateDirectory - não implementado ainda
func (p *GoogleDriveProvider) CreateDirectory(ctx context.Context, path string) error {
	return fmt.Errorf("criação de pasta no Google Drive ainda não implementada")
}

// DeleteFile - não implementado ainda
func (p *GoogleDriveProvider) DeleteFile(ctx context.Context, path string) error {
	return fmt.Errorf("exclusão no Google Drive ainda não implementada")
}

// ==================== Helpers ====================

// categorizeGoogleMimeType retorna a categoria baseada no MIME type do Google
func categorizeGoogleMimeType(mimeType string) FileCategory {
	switch mimeType {
	case MimeGoogleDoc:
		return CategoryDocument
	case MimeGoogleSheet:
		return CategoryData
	case MimeGoogleSlides:
		return CategoryDocument
	case MimeGoogleDrawing:
		return CategoryImage
	case MimeGoogleFolder:
		return CategoryUnknown // Diretório
	default:
		// Tenta categorizar por prefixo
		if strings.HasPrefix(mimeType, "image/") {
			return CategoryImage
		}
		if strings.HasPrefix(mimeType, "video/") {
			return CategoryVideo
		}
		if strings.HasPrefix(mimeType, "audio/") {
			return CategoryAudio
		}
		if strings.HasPrefix(mimeType, "text/") {
			return CategoryText
		}
		return CategoryUnknown
	}
}

