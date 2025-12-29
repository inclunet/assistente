# File Manager Agent - Plano de Implementação

## Visão Geral

O **File Manager Agent** é um agente especializado em gerenciamento de arquivos no sistema local. Ele fornece capacidades avançadas de leitura, escrita, edição, busca e exclusão de arquivos, com sistema de segurança integrado para proteger arquivos e pastas sensíveis.

```
┌─────────────────────────────────────────────────────────────┐
│                    FILE MANAGER AGENT                        │
│                                                              │
│  Capacidades:                                                │
│  ├─ Leitura de arquivos                                     │
│  ├─ Escrita de arquivos                                     │
│  ├─ Edição (replace simples e múltiplo)                     │
│  ├─ Exclusão (com autorização)                              │
│  ├─ Busca por nome                                          │
│  ├─ Busca por conteúdo (texto e regex)                      │
│  ├─ Detecção de tipo de arquivo                             │
│  └─ Listagem de diretórios                                  │
│                                                              │
│  Segurança:                                                  │
│  ├─ Pastas autorizadas para deleção                         │
│  ├─ Confirmação obrigatória para exclusões                  │
│  └─ Lista de extensões/pastas protegidas                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Funcionalidades Detalhadas

### 1. Operações de Leitura

| Operação | Descrição |
|----------|-----------|
| `file_read` | Lê o conteúdo de um arquivo de texto |
| `file_info` | Obtém metadados do arquivo (tamanho, data, tipo) |
| `file_read_lines` | Lê linhas específicas de um arquivo (range) |
| `file_head` | Lê as primeiras N linhas de um arquivo |
| `file_tail` | Lê as últimas N linhas de um arquivo |

### 2. Operações de Escrita

| Operação | Descrição |
|----------|-----------|
| `file_write` | Cria ou sobrescreve um arquivo |
| `file_append` | Adiciona conteúdo ao final de um arquivo |
| `file_create` | Cria um novo arquivo (falha se existir) |

### 3. Operações de Edição

| Operação | Descrição |
|----------|-----------|
| `file_replace` | Substitui texto em um arquivo |
| `file_replace_regex` | Substitui usando expressão regular |
| `file_replace_all` | Substitui todas as ocorrências |
| `file_replace_multiple` | Replace em múltiplos arquivos |
| `file_insert_line` | Insere linha em posição específica |

### 4. Operações de Exclusão

| Operação | Descrição |
|----------|-----------|
| `file_delete` | Exclui um arquivo (requer autorização) |
| `file_delete_multiple` | Exclui múltiplos arquivos |
| `folder_delete` | Exclui uma pasta e seu conteúdo |

### 5. Operações de Busca

| Operação | Descrição |
|----------|-----------|
| `file_search_name` | Busca arquivos por nome (glob pattern) |
| `file_search_content` | Busca arquivos por conteúdo (texto) |
| `file_search_regex` | Busca usando expressão regular |
| `file_grep` | **Busca estruturada em múltiplos arquivos** - retorna path, linha, coluna e contexto para iterações |
| `file_read_lines` | Lê range específico de linhas - ideal para detalhar resultados de busca |

### 6. Operações de Navegação

| Operação | Descrição |
|----------|-----------|
| `folder_list` | Lista arquivos e pastas em um diretório |
| `folder_tree` | Exibe árvore de diretórios |
| `folder_create` | Cria um novo diretório |
| `folder_exists` | Verifica se pasta existe |

### 7. Operações de Diretório de Trabalho

| Operação | Descrição |
|----------|-----------|
| `get_working_directory` | Retorna o diretório de trabalho atual |
| `set_working_directory` | Define um novo diretório de trabalho |
| `clear_working_directory` | Limpa o diretório de trabalho (volta ao padrão) |

O **diretório de trabalho** é essencial para o agente interpretar caminhos relativos corretamente. Quando o usuário diz "leia o arquivo config.json", o agente precisa saber em qual pasta procurar.

```
┌─────────────────────────────────────────────────────────────┐
│                 DIRETÓRIO DE TRABALHO                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Padrão: Diretório home do usuário ou pasta do aplicativo   │
│                                                              │
│  Exemplo de fluxo:                                           │
│                                                              │
│  Usuário: "Mude para a pasta C:\projetos\meu-app"           │
│  → set_working_directory("C:\projetos\meu-app")             │
│  → "Diretório de trabalho alterado para C:\projetos\meu-app"│
│                                                              │
│  Usuário: "Liste os arquivos aqui"                          │
│  → folder_list(".")  (usa CWD = C:\projetos\meu-app)        │
│                                                              │
│  Usuário: "Leia o package.json"                             │
│  → file_read("package.json")                                │
│  → Resolve para: C:\projetos\meu-app\package.json           │
│                                                              │
│  Usuário: "Limpe o diretório de trabalho"                   │
│  → clear_working_directory()                                │
│  → "Diretório de trabalho resetado para o padrão"           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## Detecção de Tipos de Arquivo

### Categorias Suportadas

| Categoria | Extensões | MIME Types |
|-----------|-----------|------------|
| **Texto** | `.txt`, `.md`, `.json`, `.xml`, `.yaml`, `.yml`, `.csv`, `.log`, `.ini`, `.cfg`, `.conf` | `text/*` |
| **Código** | `.go`, `.py`, `.js`, `.ts`, `.java`, `.c`, `.cpp`, `.h`, `.cs`, `.rb`, `.php`, `.rs`, `.swift`, `.kt` | `text/x-*` |
| **Web** | `.html`, `.htm`, `.css`, `.scss`, `.less`, `.jsx`, `.tsx`, `.vue`, `.svelte` | `text/html`, `text/css` |
| **Documento** | `.pdf`, `.doc`, `.docx`, `.xls`, `.xlsx`, `.ppt`, `.pptx`, `.odt`, `.ods` | `application/*` |
| **Imagem** | `.jpg`, `.jpeg`, `.png`, `.gif`, `.bmp`, `.webp`, `.svg`, `.ico`, `.tiff` | `image/*` |
| **Áudio** | `.mp3`, `.wav`, `.ogg`, `.flac`, `.aac`, `.m4a`, `.wma` | `audio/*` |
| **Vídeo** | `.mp4`, `.avi`, `.mkv`, `.mov`, `.wmv`, `.webm`, `.flv` | `video/*` |
| **Arquivo** | `.zip`, `.rar`, `.7z`, `.tar`, `.gz`, `.bz2` | `application/zip`, etc. |
| **Executável** | `.exe`, `.dll`, `.so`, `.dylib`, `.app` | `application/x-executable` |
| **Dados** | `.db`, `.sqlite`, `.sql`, `.bak` | `application/x-sqlite3` |

### Estrutura de Retorno de Arquivo

```json
{
  "path": "C:\\Users\\user\\docs\\readme.md",
  "name": "readme.md",
  "extension": ".md",
  "category": "text",
  "mime_type": "text/markdown",
  "size": 2048,
  "size_human": "2 KB",
  "is_text": true,
  "is_binary": false,
  "modified_at": "2024-12-26T10:30:00Z",
  "created_at": "2024-12-20T08:00:00Z",
  "is_readonly": false,
  "is_hidden": false
}
```

---

## Sistema de Formatos de Arquivo (Extensível)

### Visão Geral

O sistema de formatos de arquivo é um **pacote independente** que abstrai completamente o tipo de arquivo, fornecendo uma interface unificada para o agente interagir com qualquer formato suportado.

```
┌─────────────────────────────────────────────────────────────────┐
│                     FILE MANAGER AGENT                           │
│                                                                  │
│  Usa interface unificada - não sabe detalhes do formato         │
│                                                                  │
│  agent.ReadContent(path)  → retorna texto/dados estruturados    │
│  agent.WriteContent(path, data) → grava no formato correto      │
│  agent.SearchContent(path, query) → busca dentro do arquivo     │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    PACOTE: fileformats                           │
│                                                                  │
│  Registry de Handlers + Interface Comum                          │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    FormatRegistry                           │ │
│  │                                                             │ │
│  │  GetHandler(ext) → FormatHandler                            │ │
│  │  RegisterHandler(exts, handler)                             │ │
│  │  SupportedFormats() → []string                              │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │ PlainText│ │  Office  │ │   PDF    │ │  Google  │  ...      │
│  │ Handler  │ │ Handler  │ │ Handler  │ │ Handler  │           │
│  │          │ │          │ │          │ │          │           │
│  │.txt .md  │ │.docx .xlsx│ │.pdf      │ │.gdoc .gsheet│        │
│  │.json .csv│ │.pptx .odt│ │          │ │          │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

### Princípios de Design

| Princípio | Descrição |
|-----------|-----------|
| **Interface Comum** | Todos os handlers implementam a mesma interface |
| **Extensibilidade** | Novos formatos adicionados sem modificar código existente |
| **Separação** | Pacote `fileformats` é independente do agente |
| **Graceful Degradation** | Se handler não existe, tenta leitura como texto |
| **Metadados** | Cada handler pode expor metadados específicos do formato |

### Interface FormatHandler

```go
package fileformats

// FormatHandler define a interface comum para todos os handlers de formato
type FormatHandler interface {
    // Identificação
    Name() string                     // "office", "pdf", "plaintext"
    Extensions() []string             // [".docx", ".xlsx", ".pptx"]
    MimeTypes() []string              // ["application/vnd.openxmlformats..."]
    
    // Capacidades (o que este handler pode fazer)
    Capabilities() Capabilities
    
    // Leitura
    ReadContent(path string, opts ReadOptions) (*Content, error)
    
    // Escrita (opcional - nem todos os formatos suportam)
    WriteContent(path string, content *Content, opts WriteOptions) error
    
    // Busca dentro do arquivo (opcional)
    SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error)
    
    // Metadados específicos do formato
    GetMetadata(path string) (map[string]interface{}, error)
}

// Capabilities indica o que o handler pode fazer
type Capabilities struct {
    CanRead       bool   // Pode ler conteúdo
    CanWrite      bool   // Pode escrever/criar
    CanEdit       bool   // Pode editar (modificar existente)
    CanSearch     bool   // Pode buscar dentro do arquivo
    CanExtract    bool   // Pode extrair elementos (imagens, tabelas)
    CanConvert    bool   // Pode converter para outros formatos
}

// Content representa o conteúdo extraído de um arquivo
type Content struct {
    // Conteúdo principal
    Text       string                 // Texto extraído (sempre disponível)
    RawBytes   []byte                 // Bytes brutos (para binários)
    
    // Estrutura (para formatos estruturados)
    Sections   []Section              // Seções/capítulos (Word, PDF)
    Sheets     []Sheet                // Planilhas (Excel)
    Slides     []Slide                // Slides (PowerPoint)
    Tables     []Table                // Tabelas extraídas
    
    // Elementos embedded
    Images     []EmbeddedImage        // Imagens incorporadas
    Links      []Link                 // Links/URLs encontrados
    
    // Metadados
    Metadata   map[string]interface{} // Autor, data criação, etc.
    Encoding   string                 // Encoding detectado
    PageCount  int                    // Número de páginas (PDF, Word)
}

// SearchMatch representa um match de busca dentro do arquivo
type SearchMatch struct {
    Text         string  // Texto encontrado
    Context      string  // Contexto ao redor
    Location     string  // Localização (página, célula, slide)
    PageNumber   int     // Número da página (se aplicável)
    SheetName    string  // Nome da planilha (se aplicável)
    CellAddress  string  // Endereço da célula (se aplicável)
}
```

### FormatRegistry (Gerenciador de Handlers)

```go
package fileformats

import (
    "path/filepath"
    "strings"
    "sync"
)

// FormatRegistry gerencia todos os handlers de formato
type FormatRegistry struct {
    handlers    map[string]FormatHandler  // extensão → handler
    mimeTypes   map[string]FormatHandler  // mime type → handler
    allHandlers []FormatHandler
    mu          sync.RWMutex
}

// NewRegistry cria um novo registry com handlers padrão
func NewRegistry() *FormatRegistry {
    r := &FormatRegistry{
        handlers:  make(map[string]FormatHandler),
        mimeTypes: make(map[string]FormatHandler),
    }
    
    // Registra handlers padrão
    r.Register(NewPlainTextHandler())
    r.Register(NewOfficeHandler())
    r.Register(NewPDFHandler())
    r.Register(NewRTFHandler())
    r.Register(NewCSVHandler())
    // ... outros handlers
    
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
    
    // Fallback para plain text
    return r.handlers[".txt"]
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

// CanHandle verifica se um formato é suportado
func (r *FormatRegistry) CanHandle(path string) bool {
    return r.GetHandler(path) != nil
}
```

### Handlers Implementados

#### 1. PlainTextHandler (Texto Simples)

```go
// Suporta: .txt, .md, .json, .xml, .yaml, .yml, .ini, .cfg, .log, .csv
type PlainTextHandler struct{}

func (h *PlainTextHandler) Name() string { return "plaintext" }

func (h *PlainTextHandler) Extensions() []string {
    return []string{".txt", ".md", ".json", ".xml", ".yaml", ".yml", 
                    ".ini", ".cfg", ".conf", ".log", ".env"}
}

func (h *PlainTextHandler) Capabilities() Capabilities {
    return Capabilities{
        CanRead:   true,
        CanWrite:  true,
        CanEdit:   true,
        CanSearch: true,
    }
}

func (h *PlainTextHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    // Detecta encoding
    encoding := detectEncoding(data)
    text := decodeToUTF8(data, encoding)
    
    return &Content{
        Text:     text,
        Encoding: encoding,
    }, nil
}
```

#### 2. OfficeHandler (Microsoft Office / OpenDocument)

```go
// Suporta: .docx, .xlsx, .pptx, .odt, .ods, .odp
// Usa bibliotecas: unioffice, excelize
type OfficeHandler struct{}

func (h *OfficeHandler) Extensions() []string {
    return []string{
        // Microsoft Office
        ".docx", ".xlsx", ".pptx",
        // Legacy Office (precisa conversão)
        ".doc", ".xls", ".ppt",
        // OpenDocument
        ".odt", ".ods", ".odp",
    }
}

func (h *OfficeHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
    ext := strings.ToLower(filepath.Ext(path))
    
    switch ext {
    case ".docx", ".doc", ".odt":
        return h.readWordDocument(path, opts)
    case ".xlsx", ".xls", ".ods":
        return h.readSpreadsheet(path, opts)
    case ".pptx", ".ppt", ".odp":
        return h.readPresentation(path, opts)
    default:
        return nil, fmt.Errorf("formato não suportado: %s", ext)
    }
}

func (h *OfficeHandler) readSpreadsheet(path string, opts ReadOptions) (*Content, error) {
    // Exemplo com excelize para .xlsx
    f, err := excelize.OpenFile(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    
    content := &Content{
        Sheets: make([]Sheet, 0),
    }
    
    var textBuilder strings.Builder
    
    for _, sheetName := range f.GetSheetList() {
        rows, _ := f.GetRows(sheetName)
        
        sheet := Sheet{
            Name: sheetName,
            Rows: make([][]string, len(rows)),
        }
        
        for i, row := range rows {
            sheet.Rows[i] = row
            textBuilder.WriteString(strings.Join(row, "\t") + "\n")
        }
        
        content.Sheets = append(content.Sheets, sheet)
    }
    
    content.Text = textBuilder.String()
    return content, nil
}
```

#### 3. PDFHandler

```go
// Suporta: .pdf
// Usa biblioteca: pdfcpu, unipdf, ou pdftotext externo
type PDFHandler struct {
    useExternalTool bool  // Usar pdftotext se disponível (melhor qualidade)
}

func (h *PDFHandler) Extensions() []string {
    return []string{".pdf"}
}

func (h *PDFHandler) Capabilities() Capabilities {
    return Capabilities{
        CanRead:   true,
        CanWrite:  false,  // PDF é complexo para criar
        CanSearch: true,
        CanExtract: true,  // Pode extrair imagens
    }
}

func (h *PDFHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
    // Tenta usar pdftotext externo (melhor qualidade)
    if h.useExternalTool && commandExists("pdftotext") {
        return h.readWithPdftotext(path, opts)
    }
    
    // Fallback para biblioteca Go
    return h.readWithGoLibrary(path, opts)
}

func (h *PDFHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
    content, err := h.ReadContent(path, ReadOptions{})
    if err != nil {
        return nil, err
    }
    
    var matches []SearchMatch
    lines := strings.Split(content.Text, "\n")
    
    for i, line := range lines {
        if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
            matches = append(matches, SearchMatch{
                Text:       query,
                Context:    line,
                Location:   fmt.Sprintf("Página %d, linha %d", i/50+1, i%50+1),
                PageNumber: i/50 + 1,
            })
        }
    }
    
    return matches, nil
}
```

#### 4. RTFHandler

```go
// Suporta: .rtf
type RTFHandler struct{}

func (h *RTFHandler) Extensions() []string {
    return []string{".rtf"}
}

func (h *RTFHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    // Strip RTF formatting codes
    text := stripRTF(string(data))
    
    return &Content{
        Text: text,
    }, nil
}

// stripRTF remove códigos de formatação RTF
func stripRTF(rtf string) string {
    // Implementação simplificada - usar biblioteca real em produção
    // Remove grupos RTF, códigos de controle, etc.
    re := regexp.MustCompile(`\\[a-z]+\d*\s?|[{}]`)
    return re.ReplaceAllString(rtf, "")
}
```

#### 5. GoogleDocsHandler (via API)

```go
// Suporta: Google Docs, Sheets, Slides (requer OAuth)
type GoogleDocsHandler struct {
    client *drive.Service
}

func (h *GoogleDocsHandler) Extensions() []string {
    // Extensões virtuais para identificar links do Google
    return []string{".gdoc", ".gsheet", ".gslide"}
}

func (h *GoogleDocsHandler) Capabilities() Capabilities {
    return Capabilities{
        CanRead:   true,
        CanWrite:  true,  // Via API
        CanEdit:   true,
        CanSearch: true,
    }
}

func (h *GoogleDocsHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
    // path pode ser URL ou ID do documento
    docID := extractGoogleDocID(path)
    
    // Exporta como texto/markdown via API
    resp, err := h.client.Files.Export(docID, "text/plain").Download()
    if err != nil {
        return nil, fmt.Errorf("erro ao exportar Google Doc: %w", err)
    }
    defer resp.Body.Close()
    
    data, _ := io.ReadAll(resp.Body)
    
    return &Content{
        Text: string(data),
        Metadata: map[string]interface{}{
            "google_doc_id": docID,
            "source":        "google_drive",
        },
    }, nil
}
```

### Integração com o FileAgent

```go
// FileAgent usa o FormatRegistry para operações de arquivo
type FileAgent struct {
    BaseAgent
    formats *fileformats.FormatRegistry  // ← Pacote de formatos
    // ... outros campos
}

func NewFileAgent(db *gorm.DB, llm LLMClient, model string) *FileAgent {
    return &FileAgent{
        formats: fileformats.NewRegistry(),
        // ...
    }
}

// ReadContent lê conteúdo de qualquer formato suportado
func (a *FileAgent) ReadContent(path string) (*fileformats.Content, error) {
    // Valida segurança primeiro
    if err := ValidatePathForOperation(path, "read"); err != nil {
        return nil, err
    }
    
    // Obtém handler apropriado
    handler := a.formats.GetHandler(path)
    if handler == nil {
        return nil, fmt.Errorf("formato não suportado: %s", filepath.Ext(path))
    }
    
    // Delega para o handler
    return handler.ReadContent(path, fileformats.ReadOptions{})
}

// SearchInFile busca conteúdo dentro de um arquivo
func (a *FileAgent) SearchInFile(path, query string) ([]fileformats.SearchMatch, error) {
    handler := a.formats.GetHandler(path)
    
    caps := handler.Capabilities()
    if !caps.CanSearch {
        // Fallback: lê conteúdo e busca manualmente
        content, err := handler.ReadContent(path, fileformats.ReadOptions{})
        if err != nil {
            return nil, err
        }
        return searchInText(content.Text, query), nil
    }
    
    return handler.SearchContent(path, query, fileformats.SearchOptions{})
}
```

### Adicionando Novos Formatos (Extensibilidade)

Para adicionar suporte a um novo formato:

```go
// 1. Implemente a interface FormatHandler
type EbookHandler struct{}

func (h *EbookHandler) Name() string { return "ebook" }

func (h *EbookHandler) Extensions() []string {
    return []string{".epub", ".mobi", ".azw3"}
}

func (h *EbookHandler) Capabilities() Capabilities {
    return Capabilities{
        CanRead:   true,
        CanSearch: true,
    }
}

func (h *EbookHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
    // Implementação específica para ebooks
    // Usa biblioteca como github.com/meskio/epubgo
}

// 2. Registre o handler
func init() {
    // Auto-registro via init()
    fileformats.DefaultRegistry.Register(&EbookHandler{})
}

// OU registro manual
registry.Register(&EbookHandler{})
```

### Estrutura de Arquivos do Pacote

```
internal/
├── fileformats/
│   ├── registry.go         # FormatRegistry
│   ├── types.go             # Content, Capabilities, interfaces
│   ├── handlers/
│   │   ├── plaintext.go     # .txt, .md, .json, etc.
│   │   ├── office.go        # .docx, .xlsx, .pptx
│   │   ├── pdf.go           # .pdf
│   │   ├── rtf.go           # .rtf
│   │   ├── csv.go           # .csv (especializado)
│   │   ├── google.go        # Google Docs (via API)
│   │   └── ebook.go         # .epub, .mobi (futuro)
│   ├── encoding.go          # Detecção de encoding
│   └── utils.go             # Funções auxiliares
```

### Formatos Planejados

| Fase | Formatos | Bibliotecas Go |
|------|----------|----------------|
| **Fase 1** | `.txt`, `.md`, `.json`, `.xml`, `.yaml`, `.csv` | stdlib |
| **Fase 2** | `.docx`, `.xlsx`, `.pptx` | `unioffice`, `excelize` |
| **Fase 3** | `.pdf` | `pdfcpu`, `unipdf`, ou `pdftotext` externo |
| **Fase 4** | `.doc`, `.xls`, `.ppt` (legacy) | Conversão via LibreOffice CLI |
| **Fase 5** | `.rtf`, `.odt`, `.ods` | Parsing custom / LibreOffice |
| **Fase 6** | Google Docs, Sheets, Slides | Google Drive API |
| **Futuro** | `.epub`, `.mobi`, `.fb2` | `epubgo` |

### Diagrama de Responsabilidades

```
┌─────────────────────────────────────────────────────────────────┐
│                        FILE AGENT                                │
│                                                                  │
│  Responsabilidades:                                              │
│  • Recebe comandos em linguagem natural                          │
│  • Valida segurança (pastas protegidas)                          │
│  • Gerencia diretório de trabalho                                │
│  • Formata respostas para o usuário                              │
│  • Orquestra múltiplas operações                                 │
│  • Pede confirmação quando necessário                            │
└───────────────────────────────┬─────────────────────────────────┘
                                │ usa
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    PACOTE fileformats                            │
│                                                                  │
│  Responsabilidades:                                              │
│  • Detecta formato do arquivo                                    │
│  • Extrai conteúdo (texto, tabelas, imagens)                     │
│  • Converte entre formatos                                       │
│  • Busca dentro de arquivos                                      │
│  • Grava em formatos específicos                                 │
│  • Expõe metadados do arquivo                                    │
│                                                                  │
│  NÃO sabe sobre:                                                 │
│  • Segurança, pastas protegidas                                  │
│  • Linguagem natural                                             │
│  • Confirmações do usuário                                       │
│  • Diretório de trabalho                                         │
└─────────────────────────────────────────────────────────────────┘
```

---

## Sistema de Segurança

### Níveis de Proteção

```
┌─────────────────────────────────────────────────────────────┐
│                    NÍVEIS DE SEGURANÇA                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  🔴 NÍVEL 1 - INTOCÁVEL (NENHUMA operação permitida)        │
│  ├─ Pastas do sistema: Windows, System32, Program Files     │
│  ├─ Pastas sensíveis: AppData, .ssh, .gnupg, .aws, .azure   │
│  ├─ Arquivos críticos: *.dll, *.sys, *.exe, boot.ini        │
│  │                                                          │
│  │  ⛔ Operações BLOQUEADAS:                                │
│  │  - Leitura (file_read)                                   │
│  │  - Escrita (file_write)                                  │
│  │  - Edição (file_replace)                                 │
│  │  - Exclusão (file_delete)                                │
│  │  - Qualquer modificação                                  │
│  │                                                          │
│  │  ✅ Operações PERMITIDAS:                                │
│  │  - Listar diretório (folder_list) - apenas nomes         │
│  │  - Verificar existência (folder_exists)                  │
│  │  - Obter info básica (file_info) - apenas metadados      │
│  │                                                          │
│  🟡 NÍVEL 2 - REQUER CONFIRMAÇÃO (padrão para deleção)      │
│  ├─ Qualquer arquivo fora de pastas autorizadas             │
│  ├─ Leitura e edição permitidas                             │
│  └─ Exclusão requer confirmação explícita do usuário        │
│                                                              │
│  🟢 NÍVEL 3 - AUTORIZADO (pode deletar livremente)          │
│  ├─ Pastas configuradas pelo usuário                        │
│  ├─ Ex: C:\temp, C:\Users\user\Downloads                    │
│  └─ Configurável via UI ou banco de dados                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Regra Fundamental de Segurança

> **Arquivos de sistema e pastas protegidas NÃO PODEM ser lidos, escritos, editados ou deletados de NENHUMA forma.**
> 
> Esta é uma regra inviolável. O agente deve recusar qualquer operação que tente acessar conteúdo de arquivos em pastas protegidas.

### Configuração de Pastas Autorizadas

```sql
CREATE TABLE file_agent_authorized_paths (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL,              -- Caminho da pasta
    allow_delete INTEGER DEFAULT 1,  -- Pode deletar
    allow_write INTEGER DEFAULT 1,   -- Pode escrever
    recursive INTEGER DEFAULT 1,     -- Aplica a subpastas
    created_at DATETIME,
    updated_at DATETIME
);
```

### Pastas Protegidas (Hardcoded - INTOCÁVEIS)

```go
// Pastas que NÃO PODEM ser acessadas de nenhuma forma
var protectedPaths = []string{
    // ===== SISTEMA WINDOWS =====
    "C:\\Windows",
    "C:\\Windows\\System32",
    "C:\\Windows\\SysWOW64",
    "C:\\Program Files",
    "C:\\Program Files (x86)",
    "C:\\ProgramData",
    "C:\\Recovery",
    "C:\\$Recycle.Bin",
    
    // ===== DADOS SENSÍVEIS DO USUÁRIO =====
    "AppData\\Local\\Microsoft",
    "AppData\\Roaming\\Microsoft",
    "AppData\\Local\\Google",
    "AppData\\Local\\Mozilla",
    
    // ===== CREDENCIAIS E CHAVES =====
    ".ssh",              // Chaves SSH
    ".gnupg",            // Chaves GPG
    ".aws",              // Credenciais AWS
    ".azure",            // Credenciais Azure
    ".kube",             // Config Kubernetes
    ".docker",           // Config Docker
    ".npmrc",            // Tokens NPM
    ".netrc",            // Credenciais de rede
    ".env",              // Variáveis de ambiente (arquivo)
    
    // ===== BROWSERS E SENHAS =====
    "AppData\\Local\\Google\\Chrome\\User Data",
    "AppData\\Local\\Microsoft\\Edge\\User Data",
    "AppData\\Roaming\\Mozilla\\Firefox\\Profiles",
}

// Extensões que NÃO PODEM ser lidas, escritas ou deletadas
var protectedExtensions = []string{
    // Executáveis e bibliotecas
    ".dll", ".sys", ".exe", ".com", ".scr",
    ".drv", ".ocx", ".cpl", ".msc",
    
    // Scripts de sistema
    ".bat", ".cmd", ".ps1", ".psm1", ".psd1",
    ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh",
    
    // Instaladores e registro
    ".msi", ".msp", ".msu", ".reg", ".inf",
    
    // Arquivos de boot
    ".efi", ".bin",
}

// Arquivos específicos protegidos (qualquer lugar)
var protectedFiles = []string{
    "boot.ini",
    "ntldr",
    "bootmgr",
    "pagefile.sys",
    "hiberfil.sys",
    "swapfile.sys",
    "desktop.ini",
    "thumbs.db",
    "NTUSER.DAT",
}
```

### Fluxo de Verificação (TODAS as Operações)

```
┌─────────────────────────────────────────────────────────────┐
│          FLUXO DE VERIFICAÇÃO DE SEGURANÇA                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. QUALQUER operação (ler, escrever, deletar, editar)      │
│                         │                                    │
│                         ▼                                    │
│  2. Normaliza o caminho (resolve .., ., links simbólicos)    │
│                         │                                    │
│                         ▼                                    │
│  3. Verifica se está em PASTA PROTEGIDA                     │
│     ├─ SIM → ⛔ BLOQUEIA: "Acesso negado a pasta protegida" │
│     └─ NÃO → Continua                                       │
│                         │                                    │
│                         ▼                                    │
│  4. Verifica se tem EXTENSÃO PROTEGIDA                      │
│     ├─ SIM → ⛔ BLOQUEIA: "Tipo de arquivo protegido"       │
│     └─ NÃO → Continua                                       │
│                         │                                    │
│                         ▼                                    │
│  5. Verifica se é ARQUIVO PROTEGIDO específico              │
│     ├─ SIM → ⛔ BLOQUEIA: "Arquivo de sistema protegido"    │
│     └─ NÃO → Continua                                       │
│                         │                                    │
│                         ▼                                    │
│  6. Se for operação de EXCLUSÃO:                            │
│     ├─ Em pasta AUTORIZADA → ✅ Executa                     │
│     └─ Em pasta NORMAL → 🟡 Pede confirmação                │
│                         │                                    │
│                         ▼                                    │
│  7. Outras operações → ✅ Executa                           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Fluxo de Exclusão (Detalhado)

```
1. Usuário pede: "Delete o arquivo teste.txt"
                  │
                  ▼
2. Verifica se caminho está em NÍVEL 1 (intocável)
   ├─ SIM → Retorna erro: "Não é possível acessar arquivos protegidos do sistema"
   │
   └─ NÃO → Continua
                  │
                  ▼
3. Verifica se caminho está em NÍVEL 3 (autorizado)
   ├─ SIM → Executa exclusão e confirma
   │
   └─ NÃO → Retorna: "Para excluir 'C:\docs\teste.txt', confirme dizendo: 
                       'confirmo a exclusão de teste.txt'"
                  │
                  ▼
4. Usuário confirma: "confirmo a exclusão de teste.txt"
                  │
                  ▼
5. Agente executa exclusão com confirmação única
```

---

## Arquitetura de Implementação

### Estrutura de Arquivos

```
internal/
├── agents/
│   ├── file_agent.go           # Implementação principal do agente
│   ├── file_agent_test.go      # Testes unitários
│   └── file_operations.go      # Funções de baixo nível para operações de arquivo
├── fileops/
│   ├── detector.go             # Detecção de tipo de arquivo
│   ├── detector_test.go        # Testes do detector
│   ├── search.go               # Motor de busca de arquivos
│   ├── search_test.go          # Testes de busca
│   ├── security.go             # Validação de segurança
│   ├── security_test.go        # Testes de segurança
│   └── types.go                # Tipos e constantes
│
database.go                      # Adicionar tabela file_agent_authorized_paths
app.go / tools.go               # APIs para frontend (autorizar pastas)

frontend/src/components/
├── FileAgentConfig.svelte      # UI para configurar pastas autorizadas
└── AgentManager.svelte         # Adicionar suporte ao File Agent
```

### Interface do Agente

```go
// FileAgent é um agente inteligente para gerenciamento de arquivos
type FileAgent struct {
    BaseAgent
    authorizedPaths   []AuthorizedPath       // Pastas autorizadas do banco
    pendingDeletes    map[string]time.Time   // Confirmações pendentes
    workingDirectory  string                 // Diretório de trabalho atual
    defaultDirectory  string                 // Diretório padrão (home ou app)
    mu                sync.RWMutex
}

// AuthorizedPath representa uma pasta autorizada para operações
type AuthorizedPath struct {
    ID          uint
    Path        string
    AllowDelete bool
    AllowWrite  bool
    Recursive   bool
}

// NewFileAgent cria um novo FileAgent
func NewFileAgent(db *gorm.DB, llmClient LLMClient, model string) *FileAgent {
    homeDir, _ := os.UserHomeDir()
    
    return &FileAgent{
        BaseAgent: BaseAgent{
            Name:        "file_manager",
            DisplayName: "File Manager",
            Description: "Gerencia arquivos no sistema local. Use para ler, escrever, buscar, editar e organizar arquivos e pastas.",
            // ...
        },
        workingDirectory: homeDir,
        defaultDirectory: homeDir,
        pendingDeletes:   make(map[string]time.Time),
    }
}

// GetWorkingDirectory retorna o diretório de trabalho atual
func (a *FileAgent) GetWorkingDirectory() string {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.workingDirectory
}

// SetWorkingDirectory define um novo diretório de trabalho
func (a *FileAgent) SetWorkingDirectory(path string) error {
    // Normaliza o caminho
    absPath, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("caminho inválido: %w", err)
    }
    
    // Verifica se existe e é um diretório
    info, err := os.Stat(absPath)
    if err != nil {
        return fmt.Errorf("diretório não encontrado: %w", err)
    }
    if !info.IsDir() {
        return fmt.Errorf("o caminho não é um diretório: %s", absPath)
    }
    
    // Verifica se não é uma pasta protegida
    if isProtectedPath(absPath) {
        return fmt.Errorf("não é possível usar pasta protegida como diretório de trabalho")
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    a.workingDirectory = absPath
    return nil
}

// ClearWorkingDirectory reseta para o diretório padrão
func (a *FileAgent) ClearWorkingDirectory() {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.workingDirectory = a.defaultDirectory
}

// ResolvePath resolve um caminho relativo usando o diretório de trabalho
func (a *FileAgent) ResolvePath(path string) string {
    if filepath.IsAbs(path) {
        return path
    }
    a.mu.RLock()
    defer a.mu.RUnlock()
    return filepath.Join(a.workingDirectory, path)
}
```

---

## Tools do Agente

### 1. file_read

```json
{
  "name": "file_read",
  "description": "Lê o conteúdo completo de um arquivo de texto. Retorna erro se for arquivo binário.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho completo ou relativo do arquivo"
      },
      "encoding": {
        "type": "string",
        "description": "Encoding do arquivo (utf-8, latin1, etc). Default: utf-8"
      }
    },
    "required": ["path"]
  }
}
```

### 2. file_read_content (Leitura Universal de Formatos)

Esta tool usa o pacote `fileformats` para ler conteúdo de **qualquer formato suportado**, abstraindo completamente o tipo de arquivo.

```json
{
  "name": "file_read_content",
  "description": "Lê conteúdo de qualquer formato suportado (PDF, DOCX, XLSX, RTF, etc). Extrai texto, tabelas, metadados automaticamente. Retorna conteúdo estruturado quando disponível.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo (qualquer formato suportado)"
      },
      "extract_tables": {
        "type": "boolean",
        "description": "Extrair tabelas separadamente (Excel, Word). Default: true"
      },
      "extract_images": {
        "type": "boolean",
        "description": "Extrair referências a imagens embedded. Default: false"
      },
      "sheet_name": {
        "type": "string",
        "description": "Nome da planilha específica (para Excel). Default: todas"
      },
      "page_range": {
        "type": "string",
        "description": "Range de páginas (para PDF). Ex: '1-5', '1,3,5'. Default: todas"
      }
    },
    "required": ["path"]
  }
}
```

**Exemplo de retorno para arquivo Excel:**

```json
{
  "path": "C:\\Users\\user\\relatorios\\vendas.xlsx",
  "format": "xlsx",
  "handler": "office",
  "capabilities": {
    "can_read": true,
    "can_write": true,
    "can_search": true
  },
  "content": {
    "text": "Produto\tQuantidade\tValor\nProduto A\t100\tR$ 1.000\n...",
    "sheets": [
      {
        "name": "Vendas 2024",
        "rows": 150,
        "cols": 5,
        "data": [
          ["Produto", "Quantidade", "Valor", "Data", "Vendedor"],
          ["Produto A", 100, 1000.00, "2024-01-15", "João"],
          ["Produto B", 50, 750.00, "2024-01-16", "Maria"]
        ],
        "headers": ["Produto", "Quantidade", "Valor", "Data", "Vendedor"]
      },
      {
        "name": "Resumo",
        "rows": 10,
        "cols": 3,
        "data": [...]
      }
    ],
    "tables": [
      {
        "sheet": "Vendas 2024",
        "range": "A1:E150",
        "headers": ["Produto", "Quantidade", "Valor", "Data", "Vendedor"]
      }
    ]
  },
  "metadata": {
    "author": "João Silva",
    "created": "2024-01-10T10:00:00Z",
    "modified": "2024-12-26T15:30:00Z",
    "application": "Microsoft Excel"
  }
}
```

**Exemplo de retorno para PDF:**

```json
{
  "path": "C:\\Users\\user\\docs\\contrato.pdf",
  "format": "pdf",
  "handler": "pdf",
  "capabilities": {
    "can_read": true,
    "can_write": false,
    "can_search": true,
    "can_extract": true
  },
  "content": {
    "text": "CONTRATO DE PRESTAÇÃO DE SERVIÇOS\n\nPelo presente instrumento...",
    "pages": 12,
    "sections": [
      { "page": 1, "title": "CONTRATO DE PRESTAÇÃO DE SERVIÇOS" },
      { "page": 3, "title": "CLÁUSULA PRIMEIRA - DO OBJETO" },
      { "page": 5, "title": "CLÁUSULA SEGUNDA - DO PRAZO" }
    ],
    "images": [
      { "page": 1, "description": "Logo da empresa", "size": "150x50" },
      { "page": 12, "description": "Assinaturas", "size": "400x200" }
    ]
  },
  "metadata": {
    "author": "Departamento Jurídico",
    "created": "2024-06-15T14:00:00Z",
    "title": "Contrato de Serviços - Cliente XYZ",
    "producer": "Adobe Acrobat Pro"
  }
}
```

**Formatos Suportados:**

| Formato | Extensões | Handler | Leitura | Escrita | Busca |
|---------|-----------|---------|---------|---------|-------|
| Texto | .txt, .md, .json, .xml, .yaml | plaintext | ✅ | ✅ | ✅ |
| Word | .docx, .doc, .odt | office | ✅ | ✅ | ✅ |
| Excel | .xlsx, .xls, .ods | office | ✅ | ✅ | ✅ |
| PowerPoint | .pptx, .ppt, .odp | office | ✅ | ❌ | ✅ |
| PDF | .pdf | pdf | ✅ | ❌ | ✅ |
| RTF | .rtf | rtf | ✅ | ✅ | ✅ |
| Google Docs | .gdoc, URL | google | ✅ | ✅ | ✅ |

### 3. file_write_content (Escrita Universal de Formatos)

Esta tool escreve conteúdo em formatos que suportam escrita (texto, Excel, Word).

```json
{
  "name": "file_write_content",
  "description": "Escreve conteúdo em formatos que suportam escrita. Para Excel, pode criar planilhas com dados estruturados. Para Word, pode criar documentos formatados.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo a criar/sobrescrever"
      },
      "content": {
        "type": "object",
        "description": "Conteúdo estruturado: {text: string} ou {sheets: [{name, data: [[...]]}]} para Excel"
      },
      "format": {
        "type": "string",
        "description": "Formato de saída. Default: detectado pela extensão"
      }
    },
    "required": ["path", "content"]
  }
}
```

### 4. file_write

```json
{
  "name": "file_write",
  "description": "Cria ou sobrescreve um arquivo com o conteúdo fornecido",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho completo ou relativo do arquivo"
      },
      "content": {
        "type": "string",
        "description": "Conteúdo a ser escrito no arquivo"
      },
      "create_dirs": {
        "type": "boolean",
        "description": "Criar diretórios intermediários se não existirem. Default: true"
      }
    },
    "required": ["path", "content"]
  }
}
```

### 5. file_replace

```json
{
  "name": "file_replace",
  "description": "Substitui texto em um arquivo. Pode substituir uma ou todas as ocorrências.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo"
      },
      "old_text": {
        "type": "string",
        "description": "Texto a ser substituído"
      },
      "new_text": {
        "type": "string",
        "description": "Novo texto"
      },
      "replace_all": {
        "type": "boolean",
        "description": "Substituir todas as ocorrências. Default: false (apenas primeira)"
      }
    },
    "required": ["path", "old_text", "new_text"]
  }
}
```

### 6. file_replace_regex

```json
{
  "name": "file_replace_regex",
  "description": "Substitui texto usando expressão regular",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo"
      },
      "pattern": {
        "type": "string",
        "description": "Expressão regular para busca"
      },
      "replacement": {
        "type": "string",
        "description": "Texto de substituição (suporta grupos: $1, $2, etc)"
      },
      "replace_all": {
        "type": "boolean",
        "description": "Substituir todas as ocorrências. Default: true"
      }
    },
    "required": ["path", "pattern", "replacement"]
  }
}
```

### 7. file_replace_multiple

```json
{
  "name": "file_replace_multiple",
  "description": "Executa replace em múltiplos arquivos. Ideal para refatorações.",
  "parameters": {
    "type": "object",
    "properties": {
      "paths": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Lista de caminhos de arquivos"
      },
      "old_text": {
        "type": "string",
        "description": "Texto a ser substituído"
      },
      "new_text": {
        "type": "string",
        "description": "Novo texto"
      },
      "use_regex": {
        "type": "boolean",
        "description": "Tratar old_text como regex. Default: false"
      }
    },
    "required": ["paths", "old_text", "new_text"]
  }
}
```

### 8. file_delete

```json
{
  "name": "file_delete",
  "description": "Exclui um arquivo. Requer confirmação se a pasta não estiver autorizada.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo a excluir"
      },
      "confirm": {
        "type": "boolean",
        "description": "Confirmação explícita da exclusão. Necessário para pastas não autorizadas."
      }
    },
    "required": ["path"]
  }
}
```

### 9. file_search_name

```json
{
  "name": "file_search_name",
  "description": "Busca arquivos por nome usando padrões glob",
  "parameters": {
    "type": "object",
    "properties": {
      "directory": {
        "type": "string",
        "description": "Diretório base para busca"
      },
      "pattern": {
        "type": "string",
        "description": "Padrão de busca (ex: *.txt, report*.pdf, **/*.go)"
      },
      "recursive": {
        "type": "boolean",
        "description": "Buscar em subdiretórios. Default: true"
      },
      "max_results": {
        "type": "integer",
        "description": "Limite de resultados. Default: 100"
      }
    },
    "required": ["directory", "pattern"]
  }
}
```

### 10. file_search_content

```json
{
  "name": "file_search_content",
  "description": "Busca arquivos que contêm determinado texto. Retorna caminhos completos e trechos encontrados.",
  "parameters": {
    "type": "object",
    "properties": {
      "directory": {
        "type": "string",
        "description": "Diretório base para busca"
      },
      "query": {
        "type": "string",
        "description": "Texto a buscar"
      },
      "file_pattern": {
        "type": "string",
        "description": "Filtrar por tipo de arquivo (ex: *.go, *.txt). Default: todos"
      },
      "case_sensitive": {
        "type": "boolean",
        "description": "Busca sensível a maiúsculas. Default: false"
      },
      "recursive": {
        "type": "boolean",
        "description": "Buscar em subdiretórios. Default: true"
      },
      "max_results": {
        "type": "integer",
        "description": "Limite de resultados. Default: 50"
      }
    },
    "required": ["directory", "query"]
  }
}
```

### 11. file_search_regex

```json
{
  "name": "file_search_regex",
  "description": "Busca arquivos usando expressão regular. Retorna caminhos completos e matches.",
  "parameters": {
    "type": "object",
    "properties": {
      "directory": {
        "type": "string",
        "description": "Diretório base para busca"
      },
      "pattern": {
        "type": "string",
        "description": "Expressão regular"
      },
      "file_pattern": {
        "type": "string",
        "description": "Filtrar por tipo de arquivo. Default: todos"
      },
      "recursive": {
        "type": "boolean",
        "description": "Buscar em subdiretórios. Default: true"
      },
      "max_results": {
        "type": "integer",
        "description": "Limite de resultados. Default: 50"
      }
    },
    "required": ["directory", "pattern"]
  }
}
```

### 12. file_grep (Busca Estruturada para Iterações)

Esta é a tool principal para busca em múltiplos arquivos com retorno estruturado que permite iterações subsequentes.

```json
{
  "name": "file_grep",
  "description": "Busca termo em múltiplos arquivos retornando dados estruturados (path, linha, coluna, contexto) para permitir iterações de detalhamento. Ideal para encontrar e depois editar ou ler trechos específicos.",
  "parameters": {
    "type": "object",
    "properties": {
      "directory": {
        "type": "string",
        "description": "Diretório base para busca"
      },
      "query": {
        "type": "string",
        "description": "Termo ou expressão a buscar"
      },
      "is_regex": {
        "type": "boolean",
        "description": "Tratar query como expressão regular. Default: false"
      },
      "file_pattern": {
        "type": "string",
        "description": "Filtrar por tipo de arquivo (ex: *.go, *.js). Default: todos"
      },
      "case_sensitive": {
        "type": "boolean",
        "description": "Busca sensível a maiúsculas. Default: false"
      },
      "context_lines": {
        "type": "integer",
        "description": "Número de linhas de contexto antes e depois do match. Default: 2"
      },
      "recursive": {
        "type": "boolean",
        "description": "Buscar em subdiretórios. Default: true"
      },
      "max_files": {
        "type": "integer",
        "description": "Limite de arquivos a retornar. Default: 50"
      },
      "max_matches_per_file": {
        "type": "integer",
        "description": "Limite de matches por arquivo. Default: 10"
      }
    },
    "required": ["directory", "query"]
  }
}
```

**Estrutura de Retorno (para iterações):**

```json
{
  "query": "handleRequest",
  "total_files": 5,
  "total_matches": 12,
  "results": [
    {
      "file": {
        "path": "C:\\projetos\\app\\internal\\handlers\\user.go",
        "relative_path": "internal/handlers/user.go",
        "type": "code",
        "size": 2048
      },
      "matches": [
        {
          "line_number": 45,
          "column_start": 5,
          "column_end": 18,
          "match_text": "handleRequest",
          "line_content": "func handleRequest(ctx context.Context, req *Request) error {",
          "context_before": [
            "// handleRequest processes incoming API requests",
            "// Returns error if validation fails"
          ],
          "context_after": [
            "    if req == nil {",
            "        return ErrNilRequest"
          ]
        },
        {
          "line_number": 78,
          "column_start": 12,
          "column_end": 25,
          "match_text": "handleRequest",
          "line_content": "    return handleRequest(ctx, wrappedReq)",
          "context_before": [
            "    wrappedReq := wrapRequest(req)",
            "    // Delegate to main handler"
          ],
          "context_after": [
            "}",
            ""
          ]
        }
      ]
    },
    {
      "file": {
        "path": "C:\\projetos\\app\\internal\\handlers\\order.go",
        "relative_path": "internal/handlers/order.go",
        "type": "code",
        "size": 3500
      },
      "matches": [
        {
          "line_number": 23,
          "column_start": 8,
          "column_end": 21,
          "match_text": "handleRequest",
          "line_content": "        handleRequest(ctx, orderReq)",
          "context_before": [
            "func ProcessOrder(ctx context.Context) {",
            "    orderReq := buildOrderRequest()"
          ],
          "context_after": [
            "    log.Info(\"Order processed\")",
            "}"
          ]
        }
      ]
    }
  ],
  "summary": {
    "files_with_matches": ["internal/handlers/user.go", "internal/handlers/order.go", "..."],
    "lines_with_matches": [45, 78, 23, "..."]
  }
}
```

**Fluxo de Iterações:**

```
┌─────────────────────────────────────────────────────────────┐
│                    FLUXO DE ITERAÇÕES                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1️⃣ PRIMEIRA ITERAÇÃO - Busca Ampla                          │
│  ────────────────────────────────────────                    │
│  Usuário: "Encontre todas as chamadas a handleRequest"       │
│                                                              │
│  → file_grep(directory=".", query="handleRequest")           │
│                                                              │
│  Retorno: Lista com 12 matches em 5 arquivos                 │
│  - path, linha, contexto curto                               │
│                                                              │
│  2️⃣ SEGUNDA ITERAÇÃO - Detalhamento                          │
│  ────────────────────────────────────────                    │
│  Usuário: "Me mostre mais detalhes do user.go linha 45"      │
│                                                              │
│  → file_read_lines(                                          │
│        path="internal/handlers/user.go",                     │
│        start_line=40,                                        │
│        end_line=60                                           │
│     )                                                        │
│                                                              │
│  Retorno: 20 linhas de contexto com a função completa        │
│                                                              │
│  3️⃣ TERCEIRA ITERAÇÃO - Ação                                 │
│  ────────────────────────────────────────                    │
│  Usuário: "Renomeie handleRequest para processRequest        │
│           em todos os arquivos"                              │
│                                                              │
│  → file_replace_multiple(                                    │
│        paths=["internal/handlers/user.go",                   │
│               "internal/handlers/order.go", ...],            │
│        old_text="handleRequest",                             │
│        new_text="processRequest"                             │
│     )                                                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 13. file_read_lines (Leitura por Range)

Complementa `file_grep` permitindo ler um range específico de linhas para detalhamento.

```json
{
  "name": "file_read_lines",
  "description": "Lê um range específico de linhas de um arquivo. Útil para detalhar resultados de busca.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo"
      },
      "start_line": {
        "type": "integer",
        "description": "Linha inicial (1-indexed)"
      },
      "end_line": {
        "type": "integer",
        "description": "Linha final (1-indexed). Default: start_line + 20"
      },
      "include_line_numbers": {
        "type": "boolean",
        "description": "Incluir números de linha no retorno. Default: true"
      }
    },
    "required": ["path", "start_line"]
  }
}
```

**Exemplo de retorno:**

```json
{
  "path": "C:\\projetos\\app\\internal\\handlers\\user.go",
  "start_line": 40,
  "end_line": 60,
  "total_lines_in_file": 150,
  "content": [
    { "line": 40, "text": "" },
    { "line": 41, "text": "// handleRequest processes incoming API requests" },
    { "line": 42, "text": "// Returns error if validation fails" },
    { "line": 43, "text": "func handleRequest(ctx context.Context, req *Request) error {" },
    { "line": 44, "text": "    if req == nil {" },
    { "line": 45, "text": "        return ErrNilRequest" },
    { "line": 46, "text": "    }" },
    { "line": 47, "text": "" },
    { "line": 48, "text": "    // Validate request fields" },
    { "line": 49, "text": "    if err := req.Validate(); err != nil {" },
    { "line": 50, "text": "        return fmt.Errorf(\"validation failed: %w\", err)" }
  ],
  "raw_text": "// handleRequest processes incoming API requests\n..."
}
```

### 14. file_info

```json
{
  "name": "file_info",
  "description": "Obtém informações detalhadas sobre um arquivo ou pasta",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do arquivo ou pasta"
      }
    },
    "required": ["path"]
  }
}
```

### 15. folder_list

```json
{
  "name": "folder_list",
  "description": "Lista arquivos e pastas em um diretório com informações de tipo",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do diretório"
      },
      "show_hidden": {
        "type": "boolean",
        "description": "Incluir arquivos ocultos. Default: false"
      },
      "filter_type": {
        "type": "string",
        "description": "Filtrar por tipo: text, image, video, audio, code, all. Default: all"
      }
    },
    "required": ["path"]
  }
}
```

### 16. folder_create

```json
{
  "name": "folder_create",
  "description": "Cria um novo diretório, incluindo diretórios intermediários",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do diretório a criar"
      }
    },
    "required": ["path"]
  }
}
```

### 17. get_working_directory

```json
{
  "name": "get_working_directory",
  "description": "Retorna o diretório de trabalho atual. Use para saber onde o agente está operando.",
  "parameters": {
    "type": "object",
    "properties": {}
  }
}
```

**Exemplo de retorno:**
```json
{
  "working_directory": "C:\\Users\\user\\projetos\\meu-app",
  "is_default": false,
  "set_at": "2024-12-26T10:30:00Z"
}
```

### 18. set_working_directory

```json
{
  "name": "set_working_directory",
  "description": "Define um novo diretório de trabalho. Caminhos relativos serão resolvidos a partir deste diretório.",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "Caminho do novo diretório de trabalho. Deve existir e ser acessível."
      }
    },
    "required": ["path"]
  }
}
```

**Validações:**
- O diretório deve existir
- O diretório não pode estar em uma pasta protegida
- O caminho é normalizado (resolve `.` e `..`)

**Exemplo de retorno:**
```json
{
  "success": true,
  "previous_directory": "C:\\Users\\user",
  "new_directory": "C:\\Users\\user\\projetos\\meu-app",
  "message": "Diretório de trabalho alterado com sucesso"
}
```

### 19. clear_working_directory

```json
{
  "name": "clear_working_directory",
  "description": "Reseta o diretório de trabalho para o padrão (diretório home do usuário ou pasta do aplicativo)",
  "parameters": {
    "type": "object",
    "properties": {}
  }
}
```

**Exemplo de retorno:**
```json
{
  "success": true,
  "previous_directory": "C:\\Users\\user\\projetos\\meu-app",
  "new_directory": "C:\\Users\\user",
  "message": "Diretório de trabalho resetado para o padrão"
}
```

---

## System Prompt do Agente

```
Você é um especialista em gerenciamento de arquivos no sistema local.

## Suas capacidades:

### Navegação e Diretório de Trabalho:
- **get_working_directory**: Descobre o diretório de trabalho atual
- **set_working_directory**: Define um novo diretório de trabalho
- **clear_working_directory**: Reseta para o diretório padrão
- **folder_list**: Lista arquivos e pastas
- **folder_create**: Cria diretórios

### Leitura:
- **file_read**: Lê conteúdo de arquivos de texto
- **file_info**: Obtém metadados (tamanho, tipo, datas)

### Escrita e Edição:
- **file_write**: Cria ou sobrescreve arquivos
- **file_replace**: Substitui texto em arquivo
- **file_replace_regex**: Substitui usando expressão regular
- **file_replace_multiple**: Replace em múltiplos arquivos

### Busca:
- **file_search_name**: Busca por nome (glob pattern)
- **file_search_content**: Busca por conteúdo
- **file_search_regex**: Busca com expressão regular
- **file_grep**: Busca estruturada em múltiplos arquivos (retorna path, linha, contexto)
- **file_read_lines**: Lê range específico de linhas (para detalhamento)

### Exclusão:
- **file_delete**: Exclui arquivo (requer confirmação)

## REGRAS DE SEGURANÇA (OBRIGATÓRIAS):

### 🔴 Arquivos e Pastas de SISTEMA - TOTALMENTE PROIBIDOS:
**NÃO É POSSÍVEL de NENHUMA forma:**
- Ler, escrever, editar ou excluir arquivos em:
  - C:\Windows, C:\Program Files, C:\ProgramData
  - AppData\Local\Microsoft, AppData\Roaming\Microsoft
  - .ssh, .gnupg, .aws, .azure, .kube, .docker
- Arquivos com extensões: .dll, .sys, .exe, .bat, .cmd, .ps1, .reg, .msi
- Arquivos de sistema: boot.ini, NTUSER.DAT, pagefile.sys

Se o usuário pedir para acessar esses arquivos, **RECUSE** educadamente e explique que são arquivos protegidos do sistema.

### 🟡 Exclusão de Arquivos:
1. Em pastas NÃO autorizadas: **SEMPRE** peça confirmação explícita
2. Explique claramente O QUE será excluído ANTES de pedir confirmação
3. Em pastas autorizadas: pode excluir sem confirmação adicional

## Diretório de Trabalho:

Quando o usuário mencionar caminhos relativos (ex: "leia config.json"), você precisa saber o diretório de trabalho atual para resolver o caminho completo.

1. No início, use **get_working_directory** para saber onde está
2. Se o usuário pedir para "ir para" ou "mudar para" uma pasta, use **set_working_directory**
3. Caminhos relativos são resolvidos a partir do diretório de trabalho atual

## Regras de Resposta:

### Busca de arquivos:
1. **SEMPRE** retorne o **caminho completo** dos arquivos encontrados
2. Ao buscar por conteúdo, mostre um trecho do contexto onde foi encontrado
3. Identifique e informe o tipo de arquivo (texto, imagem, vídeo, áudio, código, etc.)

### Edição de arquivos:
1. Para replace em múltiplos arquivos, primeiro LISTE os arquivos afetados
2. Confirme a quantidade de ocorrências ANTES de executar

### Formato de resposta:
- Liste resultados de busca em formato organizado
- Inclua informações relevantes: tamanho, data de modificação, tipo
- Para erros, explique claramente o que aconteceu e como resolver

## Exemplos de uso:

**Mudar diretório de trabalho:**
"Vá para a pasta C:\projetos\meu-app"
→ Use set_working_directory

**Descobrir onde está:**
"Qual é o diretório atual?"
→ Use get_working_directory

**Buscar arquivos:**
"Encontre todos os arquivos .go no projeto"
→ Use file_search_name com pattern "*.go"

**Buscar por conteúdo:**
"Onde está definida a função HandleRequest?"
→ Use file_search_content com query "func HandleRequest"

**Busca estruturada para iterações:**
"Encontre todas as chamadas a processOrder no projeto"
→ Use file_grep para obter lista estruturada (path, linha, contexto)
→ Na segunda iteração, use file_read_lines para detalhar um arquivo específico
→ Na terceira iteração, use file_replace_multiple para fazer alterações

**Detalhar resultado de busca:**
"Me mostre mais contexto da linha 45 do user.go"
→ Use file_read_lines com start_line=40, end_line=55

**Replace em múltiplos arquivos:**
"Renomeie 'oldFunc' para 'newFunc' em todos os arquivos .go"
→ Use file_replace_multiple

**Exclusão:**
"Delete o arquivo temp.txt"
→ Verifique autorização e peça confirmação se necessário

**Recusar acesso a sistema:**
"Leia o arquivo C:\Windows\System32\config.sys"
→ Recuse: "Não posso acessar arquivos de sistema protegidos"
```

---

## Fases de Implementação

### Fase 1: Pacote fileformats - Estrutura Base (3-4 dias)

**Objetivo:** Criar pacote independente e extensível para manipulação de formatos de arquivo.

**Tarefas:**
- [ ] Criar interface `FormatHandler` e tipos (`Content`, `Capabilities`, etc.)
- [ ] Implementar `FormatRegistry` com auto-descoberta de handlers
- [ ] Implementar `PlainTextHandler` (.txt, .md, .json, .xml, .yaml, .csv)
- [ ] Implementar detecção de encoding (UTF-8, Latin1, etc.)
- [ ] Testes unitários para cada handler

**Arquivos:**
```
internal/fileformats/
├── types.go             # Content, Capabilities, interfaces
├── registry.go          # FormatRegistry
├── encoding.go          # Detecção de encoding
├── handlers/
│   ├── plaintext.go     # Handler para texto simples
│   └── plaintext_test.go
└── utils.go             # Funções auxiliares
```

### Fase 2: Handlers de Documentos (4-5 dias)

**Objetivo:** Adicionar suporte a formatos complexos de documentos.

**Tarefas:**
- [ ] Implementar `OfficeHandler` para .docx, .xlsx, .pptx (usando `unioffice`, `excelize`)
- [ ] Implementar `PDFHandler` (usando `pdfcpu` ou `pdftotext` externo)
- [ ] Implementar `RTFHandler` para .rtf
- [ ] Suporte a extração de tabelas e imagens embedded
- [ ] Testes com arquivos reais de diferentes versões

**Dependências Go:**
```
github.com/unidoc/unioffice     # Word/PowerPoint
github.com/xuri/excelize/v2     # Excel
github.com/pdfcpu/pdfcpu        # PDF
```

**Arquivos:**
```
internal/fileformats/handlers/
├── office.go            # .docx, .xlsx, .pptx
├── office_test.go
├── pdf.go               # .pdf
├── pdf_test.go
├── rtf.go               # .rtf
└── rtf_test.go
```

### Fase 3: FileAgent - Estrutura Base (2-3 dias)

**Objetivo:** Criar agente usando o pacote fileformats.

**Tarefas:**
- [ ] Criar pacote `internal/fileops` para segurança e busca
- [ ] Implementar `security.go` com validações de pastas protegidas
- [ ] Criar `file_agent.go` integrado com `fileformats.FormatRegistry`
- [ ] Implementar tools básicas: `file_read`, `file_info`, `folder_list`
- [ ] Implementar `get_working_directory`, `set_working_directory`
- [ ] Testes unitários

**Arquivos:**
```
internal/fileops/
├── types.go
├── security.go          # Validação de caminhos protegidos
├── security_test.go
└── detector.go          # Detecção de tipo de arquivo

internal/agents/
├── file_agent.go
└── file_agent_test.go
```

### Fase 4: Operações de Escrita e Edição (2-3 dias)

**Objetivo:** Implementar escrita e replace.

**Tarefas:**
- [ ] Implementar `file_write`, `file_append`, `folder_create`
- [ ] Implementar `file_replace` e `file_replace_regex`
- [ ] Implementar `file_replace_multiple`
- [ ] Validações de permissão de escrita por handler
- [ ] Testes

### Fase 5: Motor de Busca Avançado (3-4 dias)

**Objetivo:** Implementar busca estruturada com iterações.

**Tarefas:**
- [ ] Implementar `file_grep` com retorno estruturado
- [ ] Implementar `file_read_lines` para detalhamento
- [ ] Busca por nome com glob patterns (usando `doublestar`)
- [ ] Busca dentro de arquivos usando handlers do `fileformats`
- [ ] Busca com expressões regulares
- [ ] Otimização para grandes diretórios (concurrent, streaming)
- [ ] Testes de performance

**Arquivos:**
```
internal/fileops/
├── search.go            # Motor de busca
├── search_test.go
├── grep.go              # file_grep estruturado
└── grep_test.go
```

### Fase 6: Sistema de Segurança e Autorização (2 dias)

**Objetivo:** Implementar sistema de autorização para exclusões.

**Tarefas:**
- [ ] Criar tabela `file_agent_authorized_paths` no banco
- [ ] CRUD de pastas autorizadas
- [ ] Lógica de confirmação pendente com timeout
- [ ] Integração com `file_delete`
- [ ] Testes de segurança (path traversal, symlinks, etc.)

### Fase 7: Handlers Avançados (3-4 dias)

**Objetivo:** Adicionar suporte a formatos legacy e cloud.

**Tarefas:**
- [ ] Suporte a .doc, .xls, .ppt (via LibreOffice CLI ou Aspose)
- [ ] Implementar `GoogleDocsHandler` (requer OAuth já existente)
- [ ] Implementar `OpenDocumentHandler` (.odt, .ods, .odp)
- [ ] Testes de integração

**Arquivos:**
```
internal/fileformats/handlers/
├── legacy_office.go     # .doc, .xls, .ppt
├── google.go            # Google Docs API
└── opendocument.go      # .odt, .ods
```

### Fase 8: Integração e UI (2 dias)

**Objetivo:** Integrar ao sistema e criar UI de configuração.

**Tarefas:**
- [ ] Registrar agente no Registry
- [ ] Expor APIs para frontend (autorizar pastas, formatos suportados)
- [ ] Criar `FileAgentConfig.svelte`
- [ ] Atualizar `AgentManager.svelte`
- [ ] Playground para testar leitura de diferentes formatos
- [ ] Testes de integração end-to-end

### Resumo do Timeline

| Fase | Descrição | Duração | Dependências |
|------|-----------|---------|--------------|
| 1 | fileformats - Base | 3-4 dias | - |
| 2 | Handlers de Documentos | 4-5 dias | Fase 1 |
| 3 | FileAgent - Base | 2-3 dias | Fase 1 |
| 4 | Escrita e Edição | 2-3 dias | Fase 3 |
| 5 | Motor de Busca | 3-4 dias | Fase 3 |
| 6 | Segurança | 2 dias | Fase 3, 4 |
| 7 | Handlers Avançados | 3-4 dias | Fase 2 |
| 8 | UI e Integração | 2 dias | Todas |
| **Total** | | **~22-28 dias** | |

---

## Exemplos de Uso

### Exemplo 1: Gerenciando o Diretório de Trabalho

```
Usuário: "Onde você está?"

Agente usa: get_working_directory()

Resposta:
"Estou no diretório: C:\Users\user

Este é o diretório padrão. Caminhos relativos serão resolvidos a partir daqui."
```

```
Usuário: "Vá para a pasta C:\projetos\meu-app"

Agente usa: set_working_directory(path="C:\\projetos\\meu-app")

Resposta:
"✅ Diretório de trabalho alterado para: C:\projetos\meu-app

Agora posso acessar arquivos usando caminhos relativos a partir dessa pasta.
Por exemplo, 'src/main.go' será resolvido como 'C:\projetos\meu-app\src\main.go'"
```

```
Usuário: "Leia o package.json"

Agente usa: 
  1. get_working_directory() → "C:\projetos\meu-app"
  2. file_read(path="package.json") → resolve para C:\projetos\meu-app\package.json

Resposta:
"📄 C:\projetos\meu-app\package.json

{
  "name": "meu-app",
  "version": "1.0.0",
  ...
}"
```

```
Usuário: "Volte para o diretório padrão"

Agente usa: clear_working_directory()

Resposta:
"✅ Diretório de trabalho resetado para: C:\Users\user"
```

### Exemplo 2: Tentativa de Acesso a Arquivo de Sistema (BLOQUEADO)

```
Usuário: "Leia o arquivo C:\Windows\System32\config.sys"

Agente usa: file_read(path="C:\\Windows\\System32\\config.sys")
→ Erro: ErrProtectedPath

Resposta:
"⛔ Não é possível acessar esse arquivo.

O caminho 'C:\Windows\System32\' está em uma pasta protegida do sistema.
Por segurança, não posso ler, escrever ou modificar arquivos nessas pastas.

Se você precisa ver informações sobre arquivos de sistema, posso apenas 
listar o conteúdo do diretório (nomes e tamanhos), mas não ler o conteúdo."
```

```
Usuário: "Edite o arquivo hosts"

Agente usa: file_info(path="C:\\Windows\\System32\\drivers\\etc\\hosts")
→ Erro: ErrProtectedPath

Resposta:
"⛔ O arquivo 'hosts' está em uma pasta protegida do sistema (C:\Windows\System32).

Não tenho permissão para ler ou editar arquivos nessa localização.
Para editar o arquivo hosts, você precisará:
1. Abrir o Bloco de Notas como Administrador
2. Navegar até C:\Windows\System32\drivers\etc\hosts
3. Fazer as alterações manualmente"
```

### Exemplo 3: Buscar e Listar Arquivos

```
Usuário: "Liste todos os arquivos markdown no projeto"

Agente usa: file_search_name(directory=".", pattern="*.md", recursive=true)

Resposta:
"Encontrei 5 arquivos Markdown no projeto:

📄 README.md (2.3 KB) - modificado em 26/12/2024
   C:\Users\user\projeto\README.md

📄 docs/AGENTS_ARCHITECTURE.md (45.2 KB) - modificado em 25/12/2024
   C:\Users\user\projeto\docs\AGENTS_ARCHITECTURE.md

📄 docs/FILE_MANAGER_AGENT.md (12.1 KB) - modificado em 26/12/2024
   C:\Users\user\projeto\docs\FILE_MANAGER_AGENT.md

📄 docs/SPEECH_ARCHITECTURE.md (8.4 KB) - modificado em 20/12/2024
   C:\Users\user\projeto\docs\SPEECH_ARCHITECTURE.md

📄 frontend/README.md (1.1 KB) - modificado em 15/12/2024
   C:\Users\user\projeto\frontend\README.md"
```

### Exemplo 4: Buscar por Conteúdo

```
Usuário: "Onde está definida a interface Agent?"

Agente usa: file_search_content(directory="internal", query="type Agent interface", file_pattern="*.go")

Resposta:
"Encontrei a definição em 1 arquivo:

📁 internal/agents/agent.go (linha 49)
   C:\Users\user\projeto\internal\agents\agent.go

   Trecho:
   ```go
   // Agent define a interface para agentes inteligentes
   type Agent interface {
       // Identificação
       GetName() string        // Identificador único
       GetDisplayName() string // Nome para exibição
       ...
   }
   ```"
```

### Exemplo 5: Replace em Múltiplos Arquivos

```
Usuário: "Renomeie 'GetTools' para 'ListTools' em todos os arquivos Go"

Agente usa: file_search_content(directory=".", query="GetTools", file_pattern="*.go")

Resposta:
"Encontrei 8 ocorrências de 'GetTools' em 4 arquivos:
- internal/agents/agent.go (2 ocorrências)
- internal/agents/faq_agent.go (2 ocorrências)
- internal/agents/memory_agent.go (2 ocorrências)
- internal/agents/registry.go (2 ocorrências)

Deseja que eu substitua 'GetTools' por 'ListTools' em todos esses arquivos?"

Usuário: "Sim, pode fazer"

Agente usa: file_replace_multiple(paths=[...], old_text="GetTools", new_text="ListTools")

Resposta:
"Substituição concluída com sucesso!

Modificados 4 arquivos:
✅ internal/agents/agent.go - 2 substituições
✅ internal/agents/faq_agent.go - 2 substituições
✅ internal/agents/memory_agent.go - 2 substituições
✅ internal/agents/registry.go - 2 substituições

Total: 8 ocorrências substituídas."
```

### Exemplo 6: Busca Estruturada com Iterações (file_grep)

Este exemplo mostra o fluxo completo de busca → detalhamento → ação.

```
=== ITERAÇÃO 1: Busca Ampla ===

Usuário: "Encontre todas as funções que usam 'context.Context' no projeto"

Agente usa: file_grep(
    directory="internal",
    query="func.*context\\.Context",
    is_regex=true,
    file_pattern="*.go",
    context_lines=1,
    max_files=20
)

Resposta:
"🔍 Encontrei 15 matches em 6 arquivos:

📁 internal/handlers/user.go
   ├─ Linha 23: func GetUser(ctx context.Context, id string) (*User, error)
   ├─ Linha 45: func CreateUser(ctx context.Context, req CreateUserReq) error
   └─ Linha 78: func DeleteUser(ctx context.Context, id string) error

📁 internal/handlers/order.go
   ├─ Linha 12: func GetOrders(ctx context.Context) ([]Order, error)
   └─ Linha 34: func ProcessOrder(ctx context.Context, order Order) error

📁 internal/services/auth.go
   ├─ Linha 56: func ValidateToken(ctx context.Context, token string) bool
   └─ Linha 89: func RefreshToken(ctx context.Context, refresh string) (string, error)

... e mais 8 matches em 3 arquivos.

Quer ver mais detalhes de algum arquivo específico?"
```

```
=== ITERAÇÃO 2: Detalhamento ===

Usuário: "Me mostre mais detalhes da função CreateUser no user.go"

Agente usa: file_read_lines(
    path="internal/handlers/user.go",
    start_line=40,
    end_line=65,
    include_line_numbers=true
)

Resposta:
"📄 internal/handlers/user.go (linhas 40-65)

40 |
41 | // CreateUser creates a new user in the database
42 | // Returns error if validation fails or user already exists
43 | func CreateUser(ctx context.Context, req CreateUserReq) error {
44 |     // Validate request
45 |     if err := req.Validate(); err != nil {
46 |         return fmt.Errorf("validation failed: %w", err)
47 |     }
48 |
49 |     // Check if user exists
50 |     existing, _ := GetUserByEmail(ctx, req.Email)
51 |     if existing != nil {
52 |         return ErrUserAlreadyExists
53 |     }
54 |
55 |     // Hash password
56 |     hashedPassword, err := hashPassword(req.Password)
57 |     if err != nil {
58 |         return fmt.Errorf("failed to hash password: %w", err)
59 |     }
60 |
61 |     // Create user
62 |     user := &User{
63 |         Email:    req.Email,
64 |         Password: hashedPassword,
65 |         Name:     req.Name,

Quer que eu continue lendo ou faça alguma modificação?"
```

```
=== ITERAÇÃO 3: Ação ===

Usuário: "Adicione logging no início de cada função que usa context"

Agente usa: 
1. file_grep para encontrar início de cada função
2. file_replace para adicionar logging após cada declaração de função

Resposta:
"✅ Logging adicionado em 15 funções:

Modificados 6 arquivos:
├─ internal/handlers/user.go (3 funções)
├─ internal/handlers/order.go (2 funções)  
├─ internal/services/auth.go (2 funções)
├─ internal/services/notification.go (4 funções)
├─ internal/repository/user_repo.go (2 funções)
└─ internal/repository/order_repo.go (2 funções)

Código adicionado em cada função:
    log.WithContext(ctx).Info(\"[FunctionName] called\")

Quer revisar alguma das modificações?"
```

### Exemplo 7: Exclusão com Confirmação

```
Usuário: "Delete os arquivos .log na pasta logs"

Agente usa: file_search_name(directory="logs", pattern="*.log")

Resposta:
"Encontrei 3 arquivos .log para exclusão:
- logs/app.log (45 KB)
- logs/error.log (12 KB)
- logs/debug.log (128 KB)

⚠️ A pasta 'logs' não está na lista de pastas autorizadas para exclusão.
Para confirmar, diga: 'confirmo a exclusão dos arquivos .log em logs'"

Usuário: "confirmo a exclusão dos arquivos .log em logs"

Agente usa: file_delete_multiple(paths=[...], confirm=true)

Resposta:
"Exclusão concluída!

✅ logs/app.log - excluído
✅ logs/error.log - excluído
✅ logs/debug.log - excluído

3 arquivos excluídos, 185 KB liberados."
```

---

## Considerações de Segurança

### Riscos e Mitigações

| Risco | Mitigação |
|-------|-----------|
| Acesso a arquivos do sistema | **BLOQUEIO TOTAL** - Pastas/extensões de sistema são intocáveis |
| Leitura de credenciais | .ssh, .aws, .azure, .gnupg são bloqueados para qualquer operação |
| Exclusão acidental de arquivos importantes | Sistema de confirmação + pastas protegidas |
| Acesso a arquivos sensíveis (.ssh, .env) | Lista de pastas bloqueadas hardcoded (nenhuma operação permitida) |
| Path traversal (../../) | Normalização e validação de caminhos antes de qualquer operação |
| Execução de arquivos maliciosos | Agente apenas lê/escreve, **NUNCA** executa |
| Sobrecarga do sistema em buscas | Limite de resultados + timeout |
| Links simbólicos para pastas protegidas | Resolve symlinks antes de validar caminho |

### Validações Obrigatórias

```go
var (
    ErrProtectedPath      = errors.New("caminho protegido: acesso negado")
    ErrProtectedExtension = errors.New("extensão protegida: acesso negado")
    ErrProtectedFile      = errors.New("arquivo de sistema: acesso negado")
)

// ValidatePathForOperation verifica se um caminho é seguro para a operação
// operation pode ser: "read", "write", "delete", "list", "info"
func ValidatePathForOperation(path string, operation string) error {
    // 1. Normalizar caminho (resolver .., ., links simbólicos)
    absPath, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("caminho inválido: %w", err)
    }
    
    // Resolver links simbólicos para evitar bypass
    realPath, err := filepath.EvalSymlinks(absPath)
    if err == nil {
        absPath = realPath
    }
    
    // 2. Verificar se está em pasta protegida
    // BLOQUEIA: read, write, delete (permite: list, info básico)
    if isProtectedPath(absPath) {
        if operation != "list" && operation != "info" {
            return ErrProtectedPath
        }
    }
    
    // 3. Verificar extensão protegida
    // BLOQUEIA TODAS as operações para extensões de sistema
    ext := strings.ToLower(filepath.Ext(absPath))
    if isProtectedExtension(ext) {
        return ErrProtectedExtension
    }
    
    // 4. Verificar se é arquivo específico protegido
    fileName := strings.ToLower(filepath.Base(absPath))
    if isProtectedFile(fileName) {
        return ErrProtectedFile
    }
    
    return nil
}

// isProtectedPath verifica se o caminho está em uma pasta protegida
func isProtectedPath(absPath string) bool {
    lowerPath := strings.ToLower(absPath)
    
    for _, protected := range protectedPaths {
        protectedLower := strings.ToLower(protected)
        
        // Verifica se é a pasta ou está dentro dela
        if strings.HasPrefix(lowerPath, protectedLower) {
            // Garante que é a pasta exata ou subpasta (não apenas prefixo de nome)
            if len(lowerPath) == len(protectedLower) || 
               lowerPath[len(protectedLower)] == os.PathSeparator {
                return true
            }
        }
    }
    return false
}

// isProtectedExtension verifica se a extensão é protegida
func isProtectedExtension(ext string) bool {
    for _, protected := range protectedExtensions {
        if ext == protected {
            return true
        }
    }
    return false
}

// isProtectedFile verifica se é um arquivo específico protegido
func isProtectedFile(fileName string) bool {
    for _, protected := range protectedFiles {
        if strings.ToLower(fileName) == strings.ToLower(protected) {
            return true
        }
    }
    return false
}
```

---

## UI de Configuração

### Tela de Pastas Autorizadas

```
┌─────────────────────────────────────────────────────────────────┐
│  📁 Pastas Autorizadas para Exclusão                    [Salvar]│
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Pastas onde o agente pode excluir arquivos sem pedir           │
│  confirmação adicional:                                         │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Pasta                           │ Recursivo │ Ações       │  │
│  ├──────────────────────────────────┼───────────┼─────────────┤  │
│  │ C:\Users\user\Downloads          │ ☑         │ [Remover]   │  │
│  │ C:\temp                          │ ☑         │ [Remover]   │  │
│  │ C:\projetos\sandbox              │ ☐         │ [Remover]   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  [+ Adicionar Pasta]                                            │
│                                                                 │
│  ⚠️ Atenção: Arquivos em pastas autorizadas podem ser           │
│     excluídos sem confirmação adicional.                        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Próximos Passos

1. **Revisar este documento** - Validar funcionalidades e arquitetura
2. **Fase 1** - Implementar estrutura base e operações de leitura
3. **Fase 2** - Operações de escrita e edição
4. **Fase 3** - Motor de busca avançado
5. **Fase 4** - Sistema de segurança e autorização
6. **Fase 5** - Integração e UI

---

## Referências

- [Arquitetura de Agentes](AGENTS_ARCHITECTURE.md)
- [Go filepath package](https://pkg.go.dev/path/filepath)
- [Go regexp package](https://pkg.go.dev/regexp)
- [doublestar - Glob patterns](https://github.com/bmatcuk/doublestar)

