package filemanager

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFHandler manipula arquivos PDF
type PDFHandler struct{}

// NewPDFHandler cria um novo handler para arquivos PDF
func NewPDFHandler() *PDFHandler {
	return &PDFHandler{}
}

// Name retorna o nome do handler
func (h *PDFHandler) Name() string {
	return "pdf"
}

// Extensions retorna as extensões suportadas
func (h *PDFHandler) Extensions() []string {
	return []string{".pdf"}
}

// MimeTypes retorna os MIME types suportados
func (h *PDFHandler) MimeTypes() []string {
	return []string{"application/pdf"}
}

// Capabilities retorna as capacidades do handler
func (h *PDFHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   false, // Escrita de PDF é complexa
		CanEdit:    false,
		CanSearch:  true,
		CanExtract: true,
	}
}

// ReadContent lê o conteúdo de um arquivo PDF
func (h *PDFHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir PDF: %w", err)
	}
	defer f.Close()

	content := &Content{
		Sections: []Section{},
		Metadata: make(map[string]interface{}),
	}

	totalPages := r.NumPage()
	content.PageCount = totalPages
	content.Metadata["page_count"] = totalPages

	// Determina range de páginas
	startPage := 1
	endPage := totalPages

	if opts.PageRange != "" {
		// Parse page range (ex: "1-5", "3", "1,3,5")
		parsed := h.parsePageRange(opts.PageRange, totalPages)
		if len(parsed) > 0 {
			startPage = parsed[0]
			endPage = parsed[len(parsed)-1]
		}
	}

	// Extrai texto de cada página
	var textParts []string
	for pageNum := startPage; pageNum <= endPage; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		// Limpa o texto
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		textParts = append(textParts, text)

		content.Sections = append(content.Sections, Section{
			Title:   fmt.Sprintf("Página %d", pageNum),
			Content: text,
			Page:    pageNum,
		})
	}

	content.Text = strings.Join(textParts, "\n\n--- Página ---\n\n")

	// Conta linhas aproximadas
	content.LineCount = strings.Count(content.Text, "\n") + 1

	return content, nil
}

// parsePageRange converte uma string de range em lista de páginas
func (h *PDFHandler) parsePageRange(rangeStr string, maxPages int) []int {
	var pages []int

	// Remove espaços
	rangeStr = strings.ReplaceAll(rangeStr, " ", "")

	// Divide por vírgulas
	parts := strings.Split(rangeStr, ",")

	for _, part := range parts {
		// Verifica se é um range (ex: "1-5")
		if strings.Contains(part, "-") {
			bounds := strings.Split(part, "-")
			if len(bounds) == 2 {
				var start, end int
				fmt.Sscanf(bounds[0], "%d", &start)
				fmt.Sscanf(bounds[1], "%d", &end)

				if start < 1 {
					start = 1
				}
				if end > maxPages {
					end = maxPages
				}

				for i := start; i <= end; i++ {
					pages = append(pages, i)
				}
			}
		} else {
			// Página única
			var page int
			fmt.Sscanf(part, "%d", &page)
			if page >= 1 && page <= maxPages {
				pages = append(pages, page)
			}
		}
	}

	return pages
}

// WriteContent não é suportado para PDF
func (h *PDFHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita de PDF não é suportada")
}

// SearchContent busca conteúdo dentro do arquivo PDF
func (h *PDFHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir PDF: %w", err)
	}
	defer f.Close()

	var matches []SearchMatch

	// Prepara a busca
	var searchFunc func(string) []int
	if opts.UseRegex {
		flags := ""
		if !opts.CaseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + query)
		if err != nil {
			return nil, err
		}
		searchFunc = func(text string) []int {
			loc := re.FindStringIndex(text)
			return loc
		}
	} else {
		searchQuery := query
		if !opts.CaseSensitive {
			searchQuery = strings.ToLower(query)
		}
		searchFunc = func(text string) []int {
			searchText := text
			if !opts.CaseSensitive {
				searchText = strings.ToLower(text)
			}
			idx := strings.Index(searchText, searchQuery)
			if idx >= 0 {
				return []int{idx, idx + len(query)}
			}
			return nil
		}
	}

	totalPages := r.NumPage()

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}

		// Divide em linhas para melhor contexto
		lines := strings.Split(text, "\n")
		for lineNum, line := range lines {
			loc := searchFunc(line)
			if loc != nil {
				// Extrai contexto
				context := line
				if len(context) > 200 {
					start := loc[0] - 50
					if start < 0 {
						start = 0
					}
					end := loc[1] + 50
					if end > len(context) {
						end = len(context)
					}
					context = "..." + context[start:end] + "..."
				}

				matches = append(matches, SearchMatch{
					Text:       query,
					Context:    strings.TrimSpace(context),
					PageNumber: pageNum,
					LineNumber: lineNum + 1,
					Location:   fmt.Sprintf("Página %d, Linha %d", pageNum, lineNum+1),
				})

				if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
					return matches, nil
				}
			}
		}
	}

	return matches, nil
}

// GetMetadata obtém metadados do arquivo PDF
func (h *PDFHandler) GetMetadata(path string) (map[string]interface{}, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir PDF: %w", err)
	}
	defer f.Close()

	// Obtém tamanho do arquivo
	stat, _ := os.Stat(path)
	fileSize := int64(0)
	if stat != nil {
		fileSize = stat.Size()
	}

	totalPages := r.NumPage()

	// Conta caracteres aproximados
	var buf bytes.Buffer
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}
		text, _ := page.GetPlainText(nil)
		buf.WriteString(text)
	}

	textContent := buf.String()
	charCount := len(textContent)
	wordCount := len(strings.Fields(textContent))

	metadata := map[string]interface{}{
		"extension":    filepath.Ext(path),
		"page_count":   totalPages,
		"char_count":   charCount,
		"word_count":   wordCount,
		"file_size":    fileSize,
		"is_text":      false,
		"handler":      "pdf",
		"can_write":    false,
		"can_search":   true,
		"can_extract":  true,
		"content_type": "document",
	}

	return metadata, nil
}

