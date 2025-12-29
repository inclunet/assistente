package filemanager

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/extrame/xls"
)

// LegacyOfficeHandler manipula arquivos Office legados (.doc, .xls, .ppt)
// Esses formatos usam estrutura OLE2 binária
type LegacyOfficeHandler struct{}

// NewLegacyOfficeHandler cria um novo handler para arquivos Office legados
func NewLegacyOfficeHandler() *LegacyOfficeHandler {
	return &LegacyOfficeHandler{}
}

// Name retorna o nome do handler
func (h *LegacyOfficeHandler) Name() string {
	return "legacy_office"
}

// Extensions retorna as extensões suportadas
func (h *LegacyOfficeHandler) Extensions() []string {
	return []string{".xls"} // Apenas .xls tem suporte confiável em Go puro
	// .doc e .ppt requerem ferramentas externas como LibreOffice/Antiword
}

// MimeTypes retorna os MIME types suportados
func (h *LegacyOfficeHandler) MimeTypes() []string {
	return []string{
		"application/vnd.ms-excel",
	}
}

// Capabilities retorna as capacidades do handler
func (h *LegacyOfficeHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   false,
		CanEdit:    false,
		CanSearch:  true,
		CanExtract: true,
	}
}

// ReadContent lê o conteúdo de um arquivo Office legado
func (h *LegacyOfficeHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".xls":
		return h.readXLS(path, opts)
	default:
		return nil, fmt.Errorf("formato %s não suportado diretamente (use LibreOffice para converter)", ext)
	}
}

// readXLS lê um arquivo .xls (Excel 97-2003)
func (h *LegacyOfficeHandler) readXLS(path string, opts ReadOptions) (*Content, error) {
	workbook, err := xls.Open(path, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo XLS: %w", err)
	}

	content := &Content{
		Sheets:   []Sheet{},
		Tables:   []Table{},
		Metadata: make(map[string]interface{}),
	}

	numSheets := workbook.NumSheets()
	content.Metadata["sheet_count"] = numSheets

	var sheetNames []string
	var textParts []string

	for i := 0; i < numSheets; i++ {
		sheet := workbook.GetSheet(i)
		if sheet == nil {
			continue
		}

		sheetName := sheet.Name
		sheetNames = append(sheetNames, sheetName)

		// Se especificou uma planilha específica
		if opts.SheetName != "" && !strings.EqualFold(sheetName, opts.SheetName) {
			continue
		}

		sheetData := Sheet{
			Name:     sheetName,
			Rows:     [][]string{},
			RowCount: int(sheet.MaxRow) + 1,
		}

		// Lê todas as linhas
		for rowIdx := 0; rowIdx <= int(sheet.MaxRow); rowIdx++ {
			row := sheet.Row(rowIdx)
			if row == nil {
				continue
			}

			var cells []string
			lastCol := row.LastCol()

			for colIdx := 0; colIdx <= lastCol; colIdx++ {
				cellValue := row.Col(colIdx)
				cells = append(cells, cellValue)
			}

			// Primeira linha como headers
			if rowIdx == 0 {
				sheetData.Headers = cells
				sheetData.ColCount = len(cells)
			} else {
				sheetData.Rows = append(sheetData.Rows, cells)
			}
		}

		content.Sheets = append(content.Sheets, sheetData)

		// Gera texto
		textParts = append(textParts, fmt.Sprintf("=== %s ===", sheetName))
		if len(sheetData.Headers) > 0 {
			textParts = append(textParts, strings.Join(sheetData.Headers, "\t"))
		}
		for _, row := range sheetData.Rows {
			textParts = append(textParts, strings.Join(row, "\t"))
		}
	}

	content.Text = strings.Join(textParts, "\n")
	content.Metadata["sheet_names"] = sheetNames

	// Extrai tabelas se solicitado
	if opts.ExtractTables {
		for _, sheet := range content.Sheets {
			if len(sheet.Headers) > 0 {
				content.Tables = append(content.Tables, Table{
					Name:    sheet.Name,
					Headers: sheet.Headers,
					Rows:    sheet.Rows,
					Source:  path,
				})
			}
		}
	}

	return content, nil
}

// WriteContent não é suportado para Office legado
func (h *LegacyOfficeHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita de arquivos Office legados não é suportada")
}

// SearchContent busca conteúdo dentro do arquivo
func (h *LegacyOfficeHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	content, err := h.ReadContent(path, ReadOptions{})
	if err != nil {
		return nil, err
	}

	var matches []SearchMatch

	// Prepara a busca
	var searchFunc func(string) bool
	if opts.UseRegex {
		flags := ""
		if !opts.CaseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + query)
		if err != nil {
			return nil, err
		}
		searchFunc = func(text string) bool {
			return re.MatchString(text)
		}
	} else {
		searchQuery := query
		if !opts.CaseSensitive {
			searchQuery = strings.ToLower(query)
		}
		searchFunc = func(text string) bool {
			searchText := text
			if !opts.CaseSensitive {
				searchText = strings.ToLower(text)
			}
			return strings.Contains(searchText, searchQuery)
		}
	}

	// Busca em todas as planilhas
	for _, sheet := range content.Sheets {
		// Busca nos headers
		for colIdx, header := range sheet.Headers {
			if searchFunc(header) {
				matches = append(matches, SearchMatch{
					Text:        query,
					Context:     header,
					SheetName:   sheet.Name,
					LineNumber:  1,
					CellAddress: fmt.Sprintf("%c1", 'A'+colIdx),
				})

				if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
					return matches, nil
				}
			}
		}

		// Busca nas células
		for rowIdx, row := range sheet.Rows {
			for colIdx, cell := range row {
				if searchFunc(cell) {
					cellAddr := fmt.Sprintf("%c%d", 'A'+colIdx, rowIdx+2)
					if colIdx >= 26 {
						cellAddr = fmt.Sprintf("Col%d Row%d", colIdx+1, rowIdx+2)
					}

					matches = append(matches, SearchMatch{
						Text:        query,
						Context:     cell,
						SheetName:   sheet.Name,
						LineNumber:  rowIdx + 2,
						CellAddress: cellAddr,
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

// GetMetadata obtém metadados do arquivo
func (h *LegacyOfficeHandler) GetMetadata(path string) (map[string]interface{}, error) {
	ext := strings.ToLower(filepath.Ext(path))

	stat, _ := os.Stat(path)
	fileSize := int64(0)
	if stat != nil {
		fileSize = stat.Size()
	}

	metadata := map[string]interface{}{
		"extension":    ext,
		"file_size":    fileSize,
		"is_text":      false,
		"handler":      "legacy_office",
		"can_write":    false,
		"can_search":   true,
		"content_type": "spreadsheet",
		"format":       "Excel 97-2003",
	}

	// Tenta obter mais metadados
	if ext == ".xls" {
		content, err := h.ReadContent(path, ReadOptions{})
		if err == nil {
			for k, v := range content.Metadata {
				metadata[k] = v
			}

			// Conta células
			totalCells := 0
			for _, sheet := range content.Sheets {
				totalCells += len(sheet.Headers)
				for _, row := range sheet.Rows {
					totalCells += len(row)
				}
			}
			metadata["total_cells"] = totalCells
		}
	}

	return metadata, nil
}

