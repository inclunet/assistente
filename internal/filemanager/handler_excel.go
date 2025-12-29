package filemanager

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ExcelHandler manipula arquivos Excel (.xlsx)
type ExcelHandler struct{}

// NewExcelHandler cria um novo handler para arquivos Excel
func NewExcelHandler() *ExcelHandler {
	return &ExcelHandler{}
}

// Name retorna o nome do handler
func (h *ExcelHandler) Name() string {
	return "excel"
}

// Extensions retorna as extensões suportadas
func (h *ExcelHandler) Extensions() []string {
	return []string{".xlsx", ".xlsm"}
}

// MimeTypes retorna os MIME types suportados
func (h *ExcelHandler) MimeTypes() []string {
	return []string{
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel.sheet.macroEnabled.12",
	}
}

// Capabilities retorna as capacidades do handler
func (h *ExcelHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   true,
		CanEdit:    false, // Edição parcial é complexa em Excel
		CanSearch:  true,
		CanExtract: true,
	}
}

// ReadContent lê o conteúdo de um arquivo Excel
func (h *ExcelHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Excel: %w", err)
	}
	defer f.Close()

	content := &Content{
		Sheets:   []Sheet{},
		Tables:   []Table{},
		Metadata: make(map[string]interface{}),
	}

	// Lista todas as planilhas
	sheetList := f.GetSheetList()
	content.Metadata["sheet_count"] = len(sheetList)
	content.Metadata["sheet_names"] = sheetList

	// Se especificou uma planilha específica
	if opts.SheetName != "" {
		sheet, err := h.readSheet(f, opts.SheetName)
		if err != nil {
			return nil, err
		}
		content.Sheets = append(content.Sheets, *sheet)
		content.Text = h.sheetToText(sheet)
	} else {
		// Lê todas as planilhas
		var textParts []string
		for _, sheetName := range sheetList {
			sheet, err := h.readSheet(f, sheetName)
			if err != nil {
				continue // Pula planilhas com erro
			}
			content.Sheets = append(content.Sheets, *sheet)
			textParts = append(textParts, fmt.Sprintf("=== %s ===\n%s", sheetName, h.sheetToText(sheet)))
		}
		content.Text = strings.Join(textParts, "\n\n")
	}

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

// readSheet lê uma planilha específica
func (h *ExcelHandler) readSheet(f *excelize.File, sheetName string) (*Sheet, error) {
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler planilha %s: %w", sheetName, err)
	}

	sheet := &Sheet{
		Name:     sheetName,
		Rows:     rows,
		RowCount: len(rows),
	}

	// Detecta headers (primeira linha)
	if len(rows) > 0 {
		sheet.Headers = rows[0]
		sheet.ColCount = len(rows[0])
		// Remove headers das rows para evitar duplicação
		if len(rows) > 1 {
			sheet.Rows = rows[1:]
		} else {
			sheet.Rows = [][]string{}
		}
	}

	// Encontra o maior número de colunas
	for _, row := range rows {
		if len(row) > sheet.ColCount {
			sheet.ColCount = len(row)
		}
	}

	return sheet, nil
}

// sheetToText converte uma planilha para texto
func (h *ExcelHandler) sheetToText(sheet *Sheet) string {
	var lines []string

	// Headers
	if len(sheet.Headers) > 0 {
		lines = append(lines, strings.Join(sheet.Headers, "\t"))
		lines = append(lines, strings.Repeat("-", 40))
	}

	// Rows
	for _, row := range sheet.Rows {
		lines = append(lines, strings.Join(row, "\t"))
	}

	return strings.Join(lines, "\n")
}

// WriteContent escreve conteúdo em um arquivo Excel
func (h *ExcelHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	f := excelize.NewFile()
	defer f.Close()

	// Se tem sheets definidas, usa elas
	if len(content.Sheets) > 0 {
		for i, sheet := range content.Sheets {
			sheetName := sheet.Name
			if sheetName == "" {
				sheetName = fmt.Sprintf("Sheet%d", i+1)
			}

			// Cria a planilha (exceto a primeira que já existe)
			if i == 0 {
				f.SetSheetName("Sheet1", sheetName)
			} else {
				f.NewSheet(sheetName)
			}

			// Escreve headers
			if len(sheet.Headers) > 0 {
				for col, header := range sheet.Headers {
					cell, _ := excelize.CoordinatesToCellName(col+1, 1)
					f.SetCellValue(sheetName, cell, header)
				}
			}

			// Escreve dados
			startRow := 1
			if len(sheet.Headers) > 0 {
				startRow = 2
			}

			for rowIdx, row := range sheet.Rows {
				for colIdx, value := range row {
					cell, _ := excelize.CoordinatesToCellName(colIdx+1, startRow+rowIdx)
					f.SetCellValue(sheetName, cell, value)
				}
			}
		}
	} else if len(content.Tables) > 0 {
		// Se tem tables, converte para sheets
		for i, table := range content.Tables {
			sheetName := table.Name
			if sheetName == "" {
				sheetName = fmt.Sprintf("Table%d", i+1)
			}

			if i == 0 {
				f.SetSheetName("Sheet1", sheetName)
			} else {
				f.NewSheet(sheetName)
			}

			// Escreve headers
			for col, header := range table.Headers {
				cell, _ := excelize.CoordinatesToCellName(col+1, 1)
				f.SetCellValue(sheetName, cell, header)
			}

			// Escreve dados
			for rowIdx, row := range table.Rows {
				for colIdx, value := range row {
					cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
					f.SetCellValue(sheetName, cell, value)
				}
			}
		}
	}

	return f.SaveAs(path)
}

// SearchContent busca conteúdo dentro do arquivo Excel
func (h *ExcelHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Excel: %w", err)
	}
	defer f.Close()

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
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			continue
		}

		for rowIdx, row := range rows {
			for colIdx, cell := range row {
				if searchFunc(cell) {
					cellAddr, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+1)
					matches = append(matches, SearchMatch{
						Text:        query,
						Context:     cell,
						SheetName:   sheetName,
						CellAddress: cellAddr,
						LineNumber:  rowIdx + 1,
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

// GetMetadata obtém metadados do arquivo Excel
func (h *ExcelHandler) GetMetadata(path string) (map[string]interface{}, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()

	// Conta células preenchidas
	totalCells := 0
	sheetInfo := make([]map[string]interface{}, 0)

	for _, sheetName := range sheets {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			continue
		}

		cellCount := 0
		for _, row := range rows {
			cellCount += len(row)
		}
		totalCells += cellCount

		sheetInfo = append(sheetInfo, map[string]interface{}{
			"name":       sheetName,
			"row_count":  len(rows),
			"cell_count": cellCount,
		})
	}

	// Tenta obter propriedades do documento
	props, _ := f.GetDocProps()

	metadata := map[string]interface{}{
		"extension":    filepath.Ext(path),
		"sheet_count":  len(sheets),
		"sheet_names":  sheets,
		"sheets":       sheetInfo,
		"total_cells":  totalCells,
		"is_text":      false,
		"handler":      "excel",
		"can_write":    true,
		"can_search":   true,
		"can_extract":  true,
		"content_type": "spreadsheet",
	}

	if props != nil {
		if props.Title != "" {
			metadata["title"] = props.Title
		}
		if props.Creator != "" {
			metadata["author"] = props.Creator
		}
		if props.Description != "" {
			metadata["description"] = props.Description
		}
	}

	return metadata, nil
}

