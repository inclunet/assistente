package filemanager

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/unidoc/unioffice/document"
)

// WordHandler manipula arquivos Word (.docx) usando unioffice
type WordHandler struct{}

// NewWordHandler cria um novo handler para arquivos Word
func NewWordHandler() *WordHandler {
	return &WordHandler{}
}

// Name retorna o nome do handler
func (h *WordHandler) Name() string {
	return "word"
}

// Extensions retorna as extensões suportadas
func (h *WordHandler) Extensions() []string {
	return []string{".docx"}
}

// MimeTypes retorna os MIME types suportados
func (h *WordHandler) MimeTypes() []string {
	return []string{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	}
}

// Capabilities retorna as capacidades do handler
func (h *WordHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   true, // unioffice suporta escrita
		CanEdit:    false,
		CanSearch:  true,
		CanExtract: true,
	}
}

// ReadContent lê o conteúdo de um arquivo Word
func (h *WordHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	doc, err := document.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Word: %w", err)
	}
	defer doc.Close()

	content := &Content{
		Sections: []Section{},
		Tables:   []Table{},
		Links:    []Link{},
		Images:   []EmbeddedImage{},
		Metadata: make(map[string]interface{}),
	}

	var paragraphs []string
	var currentSection *Section

	// Extrai parágrafos
	for _, para := range doc.Paragraphs() {
		text := h.extractParagraphText(para)

		// Detecta headings pelo estilo
		style := para.Style()
		if strings.HasPrefix(strings.ToLower(style), "heading") ||
			strings.HasPrefix(strings.ToLower(style), "título") ||
			strings.HasPrefix(strings.ToLower(style), "title") {

			// Salva seção anterior
			if currentSection != nil && currentSection.Content != "" {
				content.Sections = append(content.Sections, *currentSection)
			}

			// Determina nível do heading
			level := 1
			if len(style) > 7 {
				fmt.Sscanf(style[7:], "%d", &level)
			}

			currentSection = &Section{
				Title: text,
				Level: level,
			}
		} else if currentSection != nil {
			if currentSection.Content != "" {
				currentSection.Content += "\n"
			}
			currentSection.Content += text
		}

		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	}

	// Salva última seção
	if currentSection != nil && (currentSection.Title != "" || currentSection.Content != "") {
		content.Sections = append(content.Sections, *currentSection)
	}

	// Extrai tabelas
	if opts.ExtractTables {
		for i, tbl := range doc.Tables() {
			table := h.extractTable(tbl, i)
			if table != nil {
				content.Tables = append(content.Tables, *table)
			}
		}
	}

	// Extrai informações de imagens
	if opts.ExtractImages {
		for i, img := range doc.Images {
			content.Images = append(content.Images, EmbeddedImage{
				Name:     fmt.Sprintf("image_%d%s", i+1, filepath.Ext(img.Path())),
				MimeType: img.Format(),
			})
		}
	}

	content.Text = strings.Join(paragraphs, "\n")
	content.LineCount = len(paragraphs)
	content.Metadata["paragraph_count"] = len(paragraphs)
	content.Metadata["section_count"] = len(content.Sections)
	content.Metadata["table_count"] = len(content.Tables)
	content.Metadata["image_count"] = len(content.Images)
	content.Metadata["link_count"] = len(content.Links)

	return content, nil
}

// extractParagraphText extrai texto de um parágrafo
func (h *WordHandler) extractParagraphText(para document.Paragraph) string {
	var parts []string

	for _, run := range para.Runs() {
		parts = append(parts, run.Text())
	}

	return strings.TrimSpace(strings.Join(parts, ""))
}

// extractTable extrai uma tabela
func (h *WordHandler) extractTable(tbl document.Table, index int) *Table {
	rows := tbl.Rows()
	if len(rows) == 0 {
		return nil
	}

	table := &Table{
		Name: fmt.Sprintf("Tabela %d", index+1),
		Rows: [][]string{},
	}

	for i, row := range rows {
		var cells []string
		for _, cell := range row.Cells() {
			var cellText []string
			for _, para := range cell.Paragraphs() {
				text := h.extractParagraphText(para)
				if text != "" {
					cellText = append(cellText, text)
				}
			}
			cells = append(cells, strings.Join(cellText, " "))
		}

		// Primeira linha como headers
		if i == 0 {
			table.Headers = cells
		} else {
			table.Rows = append(table.Rows, cells)
		}
	}

	return table
}

// WriteContent escreve conteúdo em um arquivo Word
func (h *WordHandler) WriteContent(path string, contentData *Content, opts WriteOptions) error {
	doc := document.New()
	defer doc.Close()

	// Se tem seções, usa elas
	if len(contentData.Sections) > 0 {
		for _, section := range contentData.Sections {
			// Adiciona título como heading
			if section.Title != "" {
				para := doc.AddParagraph()
				para.SetStyle(fmt.Sprintf("Heading%d", section.Level))
				run := para.AddRun()
				run.AddText(section.Title)
			}

			// Adiciona conteúdo
			if section.Content != "" {
				lines := strings.Split(section.Content, "\n")
				for _, line := range lines {
					para := doc.AddParagraph()
					run := para.AddRun()
					run.AddText(line)
				}
			}
		}
	} else if contentData.Text != "" {
		// Adiciona texto como parágrafos
		lines := strings.Split(contentData.Text, "\n")
		for _, line := range lines {
			para := doc.AddParagraph()
			run := para.AddRun()
			run.AddText(line)
		}
	}

	// Adiciona tabelas
	for _, tbl := range contentData.Tables {
		table := doc.AddTable()

		// Adiciona headers
		if len(tbl.Headers) > 0 {
			row := table.AddRow()
			for _, header := range tbl.Headers {
				cell := row.AddCell()
				para := cell.AddParagraph()
				run := para.AddRun()
				run.AddText(header)
			}
		}

		// Adiciona dados
		for _, rowData := range tbl.Rows {
			row := table.AddRow()
			for _, cellData := range rowData {
				cell := row.AddCell()
				para := cell.AddParagraph()
				run := para.AddRun()
				run.AddText(cellData)
			}
		}
	}

	return doc.SaveToFile(path)
}

// SearchContent busca conteúdo dentro do arquivo Word
func (h *WordHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	doc, err := document.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Word: %w", err)
	}
	defer doc.Close()

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
			return re.FindStringIndex(text)
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

	// Busca nos parágrafos
	for i, para := range doc.Paragraphs() {
		text := h.extractParagraphText(para)
		loc := searchFunc(text)

		if loc != nil {
			context := text
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
				LineNumber: i + 1,
				Location:   fmt.Sprintf("Parágrafo %d", i+1),
			})

			if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
				return matches, nil
			}
		}
	}

	// Busca nas tabelas
	for tblIdx, tbl := range doc.Tables() {
		for rowIdx, row := range tbl.Rows() {
			for cellIdx, cell := range row.Cells() {
				var cellText []string
				for _, para := range cell.Paragraphs() {
					cellText = append(cellText, h.extractParagraphText(para))
				}
				text := strings.Join(cellText, " ")

				loc := searchFunc(text)
				if loc != nil {
					matches = append(matches, SearchMatch{
						Text:     query,
						Context:  text,
						Location: fmt.Sprintf("Tabela %d, Linha %d, Coluna %d", tblIdx+1, rowIdx+1, cellIdx+1),
					})

					if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
						return matches, nil
					}
				}
			}
		}
	}

	return matches, nil
}

// GetMetadata obtém metadados do arquivo Word
func (h *WordHandler) GetMetadata(path string) (map[string]interface{}, error) {
	doc, err := document.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Word: %w", err)
	}
	defer doc.Close()

	// Obtém tamanho do arquivo
	stat, _ := os.Stat(path)
	fileSize := int64(0)
	if stat != nil {
		fileSize = stat.Size()
	}

	// Conta parágrafos e extrai texto para estatísticas
	paragraphCount := 0
	var allText strings.Builder

	for _, para := range doc.Paragraphs() {
		paragraphCount++
		text := h.extractParagraphText(para)
		if text != "" {
			allText.WriteString(text)
			allText.WriteString(" ")
		}
	}

	fullText := allText.String()
	wordCount := len(strings.Fields(fullText))
	charCount := len(fullText)

	// Propriedades do documento
	props := doc.CoreProperties

	metadata := map[string]interface{}{
		"extension":       filepath.Ext(path),
		"paragraph_count": paragraphCount,
		"table_count":     len(doc.Tables()),
		"image_count":     len(doc.Images),
		"word_count":      wordCount,
		"char_count":      charCount,
		"file_size":       fileSize,
		"is_text":         false,
		"handler":         "word",
		"can_write":       true,
		"can_search":      true,
		"can_extract":     true,
		"content_type":    "document",
	}

	// Propriedades do core
	if props.Title() != "" {
		metadata["title"] = props.Title()
	}
	if props.Author() != "" {
		metadata["author"] = props.Author()
	}
	if props.Description() != "" {
		metadata["description"] = props.Description()
	}
	if !props.Created().IsZero() {
		metadata["created"] = props.Created()
	}
	if !props.Modified().IsZero() {
		metadata["modified"] = props.Modified()
	}

	return metadata, nil
}
