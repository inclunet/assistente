package filemanager

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// OpenDocumentHandler manipula arquivos OpenDocument (.odt, .ods, .odp)
// ODF é um formato aberto baseado em XML compactado em ZIP
type OpenDocumentHandler struct{}

// NewOpenDocumentHandler cria um novo handler para arquivos OpenDocument
func NewOpenDocumentHandler() *OpenDocumentHandler {
	return &OpenDocumentHandler{}
}

// Name retorna o nome do handler
func (h *OpenDocumentHandler) Name() string {
	return "opendocument"
}

// Extensions retorna as extensões suportadas
func (h *OpenDocumentHandler) Extensions() []string {
	return []string{".odt", ".ods", ".odp", ".odg", ".odf"}
}

// MimeTypes retorna os MIME types suportados
func (h *OpenDocumentHandler) MimeTypes() []string {
	return []string{
		"application/vnd.oasis.opendocument.text",
		"application/vnd.oasis.opendocument.spreadsheet",
		"application/vnd.oasis.opendocument.presentation",
		"application/vnd.oasis.opendocument.graphics",
		"application/vnd.oasis.opendocument.formula",
	}
}

// Capabilities retorna as capacidades do handler
func (h *OpenDocumentHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   false,
		CanEdit:    false,
		CanSearch:  true,
		CanExtract: true,
	}
}

// Estruturas XML para ODF content.xml

// odfDocument representa o documento ODF
type odfDocument struct {
	XMLName xml.Name `xml:"document-content"`
	Body    odfBody  `xml:"body"`
}

// odfBody representa o corpo do documento
type odfBody struct {
	Text         *odfText         `xml:"text"`
	Spreadsheet  *odfSpreadsheet  `xml:"spreadsheet"`
	Presentation *odfPresentation `xml:"presentation"`
}

// odfText representa um documento de texto
type odfText struct {
	Paragraphs []odfParagraph `xml:"p"`
	Headings   []odfHeading   `xml:"h"`
	Tables     []odfTable     `xml:"table"`
	Lists      []odfList      `xml:"list"`
}

// odfHeading representa um heading
type odfHeading struct {
	Level   int       `xml:"outline-level,attr"`
	Spans   []odfSpan `xml:"span"`
	Content string    `xml:",chardata"`
}

// odfParagraph representa um parágrafo
type odfParagraph struct {
	Spans   []odfSpan `xml:"span"`
	Content string    `xml:",chardata"`
}

// odfSpan representa um span de texto
type odfSpan struct {
	Content string `xml:",chardata"`
}

// odfList representa uma lista
type odfList struct {
	Items []odfListItem `xml:"list-item"`
}

// odfListItem representa um item de lista
type odfListItem struct {
	Paragraphs []odfParagraph `xml:"p"`
}

// odfTable representa uma tabela
type odfTable struct {
	Name string        `xml:"name,attr"`
	Rows []odfTableRow `xml:"table-row"`
}

// odfTableRow representa uma linha de tabela
type odfTableRow struct {
	Cells []odfTableCell `xml:"table-cell"`
}

// odfTableCell representa uma célula
type odfTableCell struct {
	Paragraphs []odfParagraph `xml:"p"`
	Repeat     int            `xml:"number-columns-repeated,attr"`
}

// odfSpreadsheet representa uma planilha
type odfSpreadsheet struct {
	Tables []odfTable `xml:"table"`
}

// odfPresentation representa uma apresentação
type odfPresentation struct {
	Pages []odfPage `xml:"page"`
}

// odfPage representa uma página/slide
type odfPage struct {
	Name   string     `xml:"name,attr"`
	Frames []odfFrame `xml:"frame"`
}

// odfFrame representa um frame em um slide
type odfFrame struct {
	TextBox *odfTextBox `xml:"text-box"`
}

// odfTextBox representa uma caixa de texto
type odfTextBox struct {
	Paragraphs []odfParagraph `xml:"p"`
}

// ReadContent lê o conteúdo de um arquivo OpenDocument
func (h *OpenDocumentHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo OpenDocument: %w", err)
	}
	defer r.Close()

	content := &Content{
		Sections: []Section{},
		Tables:   []Table{},
		Sheets:   []Sheet{},
		Slides:   []Slide{},
		Metadata: make(map[string]interface{}),
	}

	// Procura content.xml
	var contentXML *zip.File
	for _, f := range r.File {
		if f.Name == "content.xml" {
			contentXML = f
			break
		}
	}

	if contentXML == nil {
		return nil, fmt.Errorf("arquivo OpenDocument inválido: content.xml não encontrado")
	}

	// Lê o XML
	xmlReader, err := contentXML.Open()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler content.xml: %w", err)
	}
	defer xmlReader.Close()

	xmlBytes, err := io.ReadAll(xmlReader)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler conteúdo XML: %w", err)
	}

	// Determina o tipo pelo extensão
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".odt":
		return h.parseTextDocument(xmlBytes, content)
	case ".ods":
		return h.parseSpreadsheet(xmlBytes, content)
	case ".odp":
		return h.parsePresentation(xmlBytes, content)
	default:
		// Tenta como texto
		return h.parseTextDocument(xmlBytes, content)
	}
}

// parseTextDocument processa um documento de texto ODF
func (h *OpenDocumentHandler) parseTextDocument(xmlBytes []byte, content *Content) (*Content, error) {
	var doc odfDocument
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		return nil, fmt.Errorf("erro ao parsear content.xml: %w", err)
	}

	if doc.Body.Text == nil {
		return content, nil
	}

	var paragraphs []string
	var currentSection *Section

	// Processa headings e parágrafos
	for _, heading := range doc.Body.Text.Headings {
		text := h.extractHeadingText(heading)
		if text == "" {
			continue
		}

		// Salva seção anterior
		if currentSection != nil && currentSection.Content != "" {
			content.Sections = append(content.Sections, *currentSection)
		}

		currentSection = &Section{
			Title: text,
			Level: heading.Level,
		}
		paragraphs = append(paragraphs, text)
	}

	for _, para := range doc.Body.Text.Paragraphs {
		text := h.extractParagraphText(para)
		if text == "" {
			continue
		}

		if currentSection != nil {
			if currentSection.Content != "" {
				currentSection.Content += "\n"
			}
			currentSection.Content += text
		}
		paragraphs = append(paragraphs, text)
	}

	// Salva última seção
	if currentSection != nil && (currentSection.Title != "" || currentSection.Content != "") {
		content.Sections = append(content.Sections, *currentSection)
	}

	// Processa tabelas
	for i, tbl := range doc.Body.Text.Tables {
		table := h.extractTable(tbl, i)
		if table != nil {
			content.Tables = append(content.Tables, *table)
		}
	}

	// Processa listas
	for _, list := range doc.Body.Text.Lists {
		for _, item := range list.Items {
			for _, para := range item.Paragraphs {
				text := "• " + h.extractParagraphText(para)
				paragraphs = append(paragraphs, text)
			}
		}
	}

	content.Text = strings.Join(paragraphs, "\n")
	content.LineCount = len(paragraphs)
	content.Metadata["type"] = "text"

	return content, nil
}

// parseSpreadsheet processa uma planilha ODF
func (h *OpenDocumentHandler) parseSpreadsheet(xmlBytes []byte, content *Content) (*Content, error) {
	var doc odfDocument
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		return nil, fmt.Errorf("erro ao parsear content.xml: %w", err)
	}

	if doc.Body.Spreadsheet == nil {
		return content, nil
	}

	var textParts []string

	for _, tbl := range doc.Body.Spreadsheet.Tables {
		sheet := Sheet{
			Name: tbl.Name,
			Rows: [][]string{},
		}

		for _, row := range tbl.Rows {
			var cells []string
			for _, cell := range row.Cells {
				cellText := ""
				for _, para := range cell.Paragraphs {
					cellText += h.extractParagraphText(para)
				}

				// Repete célula se necessário
				repeat := cell.Repeat
				if repeat < 1 {
					repeat = 1
				}
				for i := 0; i < repeat && i < 100; i++ { // Limita repetições
					cells = append(cells, cellText)
				}
			}
			if len(cells) > 0 {
				sheet.Rows = append(sheet.Rows, cells)
			}
		}

		// Define headers como primeira linha
		if len(sheet.Rows) > 0 {
			sheet.Headers = sheet.Rows[0]
			sheet.Rows = sheet.Rows[1:]
		}
		sheet.RowCount = len(sheet.Rows) + 1
		sheet.ColCount = len(sheet.Headers)

		content.Sheets = append(content.Sheets, sheet)
		textParts = append(textParts, fmt.Sprintf("=== %s ===", sheet.Name))
	}

	content.Text = strings.Join(textParts, "\n\n")
	content.Metadata["type"] = "spreadsheet"
	content.Metadata["sheet_count"] = len(content.Sheets)

	return content, nil
}

// parsePresentation processa uma apresentação ODF
func (h *OpenDocumentHandler) parsePresentation(xmlBytes []byte, content *Content) (*Content, error) {
	var doc odfDocument
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		return nil, fmt.Errorf("erro ao parsear content.xml: %w", err)
	}

	if doc.Body.Presentation == nil {
		return content, nil
	}

	var textParts []string

	for i, page := range doc.Body.Presentation.Pages {
		slide := Slide{
			Number: i + 1,
			Title:  page.Name,
		}

		var slideContent []string
		for _, frame := range page.Frames {
			if frame.TextBox != nil {
				for _, para := range frame.TextBox.Paragraphs {
					text := h.extractParagraphText(para)
					if text != "" {
						slideContent = append(slideContent, text)
					}
				}
			}
		}
		slide.Content = strings.Join(slideContent, "\n")

		content.Slides = append(content.Slides, slide)
		textParts = append(textParts, fmt.Sprintf("--- Slide %d: %s ---\n%s", slide.Number, slide.Title, slide.Content))
	}

	content.Text = strings.Join(textParts, "\n\n")
	content.Metadata["type"] = "presentation"
	content.Metadata["slide_count"] = len(content.Slides)

	return content, nil
}

// extractParagraphText extrai texto de um parágrafo
func (h *OpenDocumentHandler) extractParagraphText(para odfParagraph) string {
	var parts []string

	if para.Content != "" {
		parts = append(parts, strings.TrimSpace(para.Content))
	}

	for _, span := range para.Spans {
		if span.Content != "" {
			parts = append(parts, strings.TrimSpace(span.Content))
		}
	}

	return strings.Join(parts, "")
}

// extractHeadingText extrai texto de um heading
func (h *OpenDocumentHandler) extractHeadingText(heading odfHeading) string {
	var parts []string

	if heading.Content != "" {
		parts = append(parts, strings.TrimSpace(heading.Content))
	}

	for _, span := range heading.Spans {
		if span.Content != "" {
			parts = append(parts, strings.TrimSpace(span.Content))
		}
	}

	return strings.Join(parts, "")
}

// extractTable extrai uma tabela
func (h *OpenDocumentHandler) extractTable(tbl odfTable, index int) *Table {
	if len(tbl.Rows) == 0 {
		return nil
	}

	table := &Table{
		Name: tbl.Name,
		Rows: [][]string{},
	}

	if table.Name == "" {
		table.Name = fmt.Sprintf("Tabela %d", index+1)
	}

	for i, row := range tbl.Rows {
		var cells []string
		for _, cell := range row.Cells {
			var cellText []string
			for _, para := range cell.Paragraphs {
				text := h.extractParagraphText(para)
				if text != "" {
					cellText = append(cellText, text)
				}
			}
			cells = append(cells, strings.Join(cellText, " "))
		}

		if i == 0 {
			table.Headers = cells
		} else {
			table.Rows = append(table.Rows, cells)
		}
	}

	return table
}

// WriteContent não é suportado para OpenDocument
func (h *OpenDocumentHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita de arquivos OpenDocument não é suportada")
}

// SearchContent busca conteúdo dentro do arquivo OpenDocument
func (h *OpenDocumentHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	content, err := h.ReadContent(path, ReadOptions{})
	if err != nil {
		return nil, err
	}

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

	// Busca no texto
	lines := strings.Split(content.Text, "\n")
	for lineNum, line := range lines {
		loc := searchFunc(line)
		if loc != nil {
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
				LineNumber: lineNum + 1,
			})

			if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
				return matches, nil
			}
		}
	}

	return matches, nil
}

// GetMetadata obtém metadados do arquivo OpenDocument
func (h *OpenDocumentHandler) GetMetadata(path string) (map[string]interface{}, error) {
	content, err := h.ReadContent(path, ReadOptions{})
	if err != nil {
		return nil, err
	}

	stat, _ := os.Stat(path)
	fileSize := int64(0)
	if stat != nil {
		fileSize = stat.Size()
	}

	ext := strings.ToLower(filepath.Ext(path))

	metadata := map[string]interface{}{
		"extension":  ext,
		"file_size":  fileSize,
		"is_text":    false,
		"handler":    "opendocument",
		"can_write":  false,
		"can_search": true,
	}

	// Adiciona metadados específicos do tipo
	for k, v := range content.Metadata {
		metadata[k] = v
	}

	switch ext {
	case ".odt":
		metadata["content_type"] = "document"
		metadata["paragraph_count"] = content.LineCount
		metadata["word_count"] = len(strings.Fields(content.Text))
	case ".ods":
		metadata["content_type"] = "spreadsheet"
		metadata["sheet_count"] = len(content.Sheets)
	case ".odp":
		metadata["content_type"] = "presentation"
		metadata["slide_count"] = len(content.Slides)
	}

	return metadata, nil
}
