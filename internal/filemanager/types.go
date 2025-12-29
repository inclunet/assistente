// Package filemanager fornece funcionalidades completas de gerenciamento de arquivos,
// incluindo leitura, escrita, busca, detecção de tipos e validação de segurança.
// Este pacote pode ser usado diretamente pelo frontend ou por agentes LLM.
package filemanager

import (
	"errors"
	"time"
)

// ==================== Erros ====================

var (
	ErrProtectedPath      = errors.New("caminho protegido: acesso negado")
	ErrProtectedExtension = errors.New("extensão protegida: acesso negado")
	ErrProtectedFile      = errors.New("arquivo de sistema: acesso negado")
	ErrDeleteNotAllowed   = errors.New("exclusão não permitida: pasta não autorizada")
	ErrPathTraversal      = errors.New("tentativa de path traversal detectada")
	ErrSymlinkToProtected = errors.New("symlink aponta para caminho protegido")
)

// ==================== Categorias de Arquivo ====================

// FileCategory representa a categoria de um arquivo
type FileCategory string

const (
	CategoryText       FileCategory = "text"
	CategoryCode       FileCategory = "code"
	CategoryWeb        FileCategory = "web"
	CategoryDocument   FileCategory = "document"
	CategoryImage      FileCategory = "image"
	CategoryAudio      FileCategory = "audio"
	CategoryVideo      FileCategory = "video"
	CategoryArchive    FileCategory = "archive"
	CategoryExecutable FileCategory = "executable"
	CategoryData       FileCategory = "data"
	CategoryConfig     FileCategory = "config"
	CategoryUnknown    FileCategory = "unknown"
)

// ==================== Operações ====================

// Operation representa o tipo de operação de arquivo
type Operation string

const (
	OpRead   Operation = "read"
	OpWrite  Operation = "write"
	OpDelete Operation = "delete"
	OpList   Operation = "list"
	OpInfo   Operation = "info"
)

// ==================== Informações de Arquivo ====================

// FileInfo representa informações detalhadas sobre um arquivo
type FileInfo struct {
	Path        string       `json:"path"`
	Name        string       `json:"name"`
	Extension   string       `json:"extension"`
	Category    FileCategory `json:"category"`
	MimeType    string       `json:"mime_type"`
	Size        int64        `json:"size"`
	SizeHuman   string       `json:"size_human"`
	IsText      bool         `json:"is_text"`
	IsBinary    bool         `json:"is_binary"`
	IsDir       bool         `json:"is_dir"`
	IsHidden    bool         `json:"is_hidden"`
	IsReadOnly  bool         `json:"is_readonly"`
	ModifiedAt  time.Time    `json:"modified_at"`
	CreatedAt   time.Time    `json:"created_at"`
	Permissions string       `json:"permissions"`
}

// DirEntry representa uma entrada em um diretório
type DirEntry struct {
	Name     string       `json:"name"`
	Path     string       `json:"path"`
	IsDir    bool         `json:"is_dir"`
	Size     int64        `json:"size"`
	Category FileCategory `json:"category"`
	Modified time.Time    `json:"modified"`
}

// ==================== Autorização ====================

// AuthorizedPath representa uma pasta autorizada para operações
type AuthorizedPath struct {
	ID          uint      `json:"id"`
	Path        string    `json:"path"`
	AllowDelete bool      `json:"allow_delete"`
	AllowWrite  bool      `json:"allow_write"`
	Recursive   bool      `json:"recursive"`
	CreatedAt   time.Time `json:"created_at"`
}

// ==================== Conteúdo de Arquivo ====================

// Capabilities indica o que o handler pode fazer
type Capabilities struct {
	CanRead    bool `json:"can_read"`
	CanWrite   bool `json:"can_write"`
	CanEdit    bool `json:"can_edit"`
	CanSearch  bool `json:"can_search"`
	CanExtract bool `json:"can_extract"`
	CanConvert bool `json:"can_convert"`
}

// ReadOptions opções para leitura de arquivo
type ReadOptions struct {
	ExtractTables bool   `json:"extract_tables"`
	ExtractImages bool   `json:"extract_images"`
	SheetName     string `json:"sheet_name"`
	PageRange     string `json:"page_range"`
	MaxBytes      int64  `json:"max_bytes"`
}

// WriteOptions opções para escrita de arquivo
type WriteOptions struct {
	CreateDirs bool   `json:"create_dirs"`
	Overwrite  bool   `json:"overwrite"`
	Encoding   string `json:"encoding"`
}

// SearchOptions opções para busca dentro do arquivo
type SearchOptions struct {
	CaseSensitive bool `json:"case_sensitive"`
	UseRegex      bool `json:"use_regex"`
	MaxResults    int  `json:"max_results"`
	ContextLines  int  `json:"context_lines"`
}

// Content representa o conteúdo extraído de um arquivo
type Content struct {
	Text      string                 `json:"text"`
	RawBytes  []byte                 `json:"raw_bytes,omitempty"`
	Sections  []Section              `json:"sections,omitempty"`
	Sheets    []Sheet                `json:"sheets,omitempty"`
	Slides    []Slide                `json:"slides,omitempty"`
	Tables    []Table                `json:"tables,omitempty"`
	Images    []EmbeddedImage        `json:"images,omitempty"`
	Links     []Link                 `json:"links,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Encoding  string                 `json:"encoding,omitempty"`
	PageCount int                    `json:"page_count,omitempty"`
	LineCount int                    `json:"line_count,omitempty"`
}

// Section representa uma seção/capítulo de um documento
type Section struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Page    int    `json:"page,omitempty"`
	Level   int    `json:"level,omitempty"`
}

// Sheet representa uma planilha de um arquivo Excel
type Sheet struct {
	Name     string     `json:"name"`
	Rows     [][]string `json:"rows"`
	Headers  []string   `json:"headers,omitempty"`
	RowCount int        `json:"row_count"`
	ColCount int        `json:"col_count"`
}

// Slide representa um slide de uma apresentação
type Slide struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Notes   string `json:"notes,omitempty"`
}

// Table representa uma tabela extraída
type Table struct {
	Name    string     `json:"name,omitempty"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
	Source  string     `json:"source,omitempty"`
}

// EmbeddedImage representa uma imagem embutida no documento
type EmbeddedImage struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Size        string `json:"size,omitempty"`
	Page        int    `json:"page,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
}

// Link representa um link encontrado no documento
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
	Page int    `json:"page,omitempty"`
}

// ==================== Busca ====================

// SearchMatch representa um match de busca
type SearchMatch struct {
	Text          string   `json:"text"`
	Context       string   `json:"context"`
	Location      string   `json:"location,omitempty"`
	LineNumber    int      `json:"line_number,omitempty"`
	PageNumber    int      `json:"page_number,omitempty"`
	SheetName     string   `json:"sheet_name,omitempty"`
	CellAddress   string   `json:"cell_address,omitempty"`
	ColumnStart   int      `json:"column_start,omitempty"`
	ColumnEnd     int      `json:"column_end,omitempty"`
	ContextBefore []string `json:"context_before,omitempty"`
	ContextAfter  []string `json:"context_after,omitempty"`
}

// SearchResult representa o resultado de uma busca em arquivo
type SearchResult struct {
	File    FileInfo      `json:"file"`
	Matches []SearchMatch `json:"matches"`
}

// GrepResult representa o resultado de uma busca grep estruturada
type GrepResult struct {
	Query        string         `json:"query"`
	TotalFiles   int            `json:"total_files"`
	TotalMatches int            `json:"total_matches"`
	Results      []SearchResult `json:"results"`
}

// ==================== Linhas ====================

// LineInfo representa uma linha com seu número
type LineInfo struct {
	Number int    `json:"line"`
	Text   string `json:"text"`
}

// LinesResult representa o resultado de leitura de linhas específicas
type LinesResult struct {
	Path       string     `json:"path"`
	StartLine  int        `json:"start_line"`
	EndLine    int        `json:"end_line"`
	TotalLines int        `json:"total_lines"`
	Content    []LineInfo `json:"content"`
	RawText    string     `json:"raw_text"`
}

// ==================== Format Handler ====================

// FormatHandler define a interface comum para handlers de formato
type FormatHandler interface {
	Name() string
	Extensions() []string
	MimeTypes() []string
	Capabilities() Capabilities
	ReadContent(path string, opts ReadOptions) (*Content, error)
	WriteContent(path string, content *Content, opts WriteOptions) error
	SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error)
	GetMetadata(path string) (map[string]interface{}, error)
}

