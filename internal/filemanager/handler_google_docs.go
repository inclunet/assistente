// Package filemanager - handler para Google Docs/Drive
package filemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// GoogleDocsHandler implementa FormatHandler para documentos do Google Docs/Drive
type GoogleDocsHandler struct {
	httpClient   *http.Client
	getToken     func() (string, error)
	capabilities Capabilities
}

// GoogleDriveFile representa um arquivo do Google Drive
type GoogleDriveFile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	MimeType     string    `json:"mimeType"`
	Size         string    `json:"size,omitempty"`
	CreatedTime  time.Time `json:"createdTime"`
	ModifiedTime time.Time `json:"modifiedTime"`
	WebViewLink  string    `json:"webViewLink"`
	Parents      []string  `json:"parents,omitempty"`
}

// GoogleDriveList representa a resposta de listagem do Drive
type GoogleDriveList struct {
	Files         []GoogleDriveFile `json:"files"`
	NextPageToken string            `json:"nextPageToken,omitempty"`
}

// Google MIME types
const (
	MimeGoogleDoc     = "application/vnd.google-apps.document"
	MimeGoogleSheet   = "application/vnd.google-apps.spreadsheet"
	MimeGoogleSlides  = "application/vnd.google-apps.presentation"
	MimeGoogleDrawing = "application/vnd.google-apps.drawing"
	MimeGoogleFolder  = "application/vnd.google-apps.folder"
)

// Google Drive API base URL
const googleDriveAPIBase = "https://www.googleapis.com/drive/v3"
const googleDocsAPIBase = "https://docs.googleapis.com/v1"

// NewGoogleDocsHandler cria um novo handler para Google Docs
// O parâmetro tokenProvider é uma função que retorna o access token OAuth do Google
func NewGoogleDocsHandler(tokenProvider func() (string, error)) *GoogleDocsHandler {
	return &GoogleDocsHandler{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		getToken:   tokenProvider,
		capabilities: Capabilities{
			CanRead:    true,
			CanWrite:   false, // TODO: implementar escrita futuramente
			CanEdit:    false,
			CanSearch:  true,
			CanExtract: true,
			CanConvert: false,
		},
	}
}

// Name retorna o nome do handler
func (h *GoogleDocsHandler) Name() string {
	return "Google Docs"
}

// Extensions retorna extensões suportadas (usa pseudo-extensões para Google)
func (h *GoogleDocsHandler) Extensions() []string {
	return []string{".gdoc", ".gsheet", ".gslides", ".gdraw"}
}

// MimeTypes retorna os MIME types suportados
func (h *GoogleDocsHandler) MimeTypes() []string {
	return []string{
		MimeGoogleDoc,
		MimeGoogleSheet,
		MimeGoogleSlides,
		MimeGoogleDrawing,
	}
}

// Capabilities retorna as capacidades do handler
func (h *GoogleDocsHandler) Capabilities() Capabilities {
	return h.capabilities
}

// ReadContent lê o conteúdo de um documento Google
// O path pode ser:
// - ID do documento: "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms"
// - URL do documento: "https://docs.google.com/document/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit"
// - Pseudo-path: "gdocs://1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms"
func (h *GoogleDocsHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	docID := h.extractDocumentID(path)
	if docID == "" {
		return nil, fmt.Errorf("não foi possível extrair ID do documento de: %s", path)
	}

	token, err := h.getToken()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter token OAuth: %w", err)
	}

	// Primeiro, obtém metadata do arquivo para saber o tipo
	fileInfo, err := h.getFileMetadata(docID, token)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter metadata: %w", err)
	}

	content := &Content{
		Metadata: make(map[string]interface{}),
	}

	// Adiciona metadata básica
	content.Metadata["id"] = fileInfo.ID
	content.Metadata["name"] = fileInfo.Name
	content.Metadata["mimeType"] = fileInfo.MimeType
	content.Metadata["webViewLink"] = fileInfo.WebViewLink
	content.Metadata["modifiedTime"] = fileInfo.ModifiedTime
	content.Metadata["createdTime"] = fileInfo.CreatedTime

	// Lê o conteúdo baseado no tipo
	switch fileInfo.MimeType {
	case MimeGoogleDoc:
		err = h.readGoogleDoc(docID, token, content)
	case MimeGoogleSheet:
		err = h.readGoogleSheet(docID, token, content, opts)
	case MimeGoogleSlides:
		err = h.readGoogleSlides(docID, token, content)
	default:
		// Para outros tipos, tenta exportar como texto
		err = h.exportAsText(docID, token, fileInfo.MimeType, content)
	}

	if err != nil {
		return nil, err
	}

	return content, nil
}

// WriteContent escreve conteúdo em um documento Google (não implementado ainda)
func (h *GoogleDocsHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita em Google Docs ainda não implementada")
}

// SearchContent busca conteúdo dentro de um documento Google
func (h *GoogleDocsHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
	// Lê o conteúdo primeiro
	content, err := h.ReadContent(path, ReadOptions{})
	if err != nil {
		return nil, err
	}

	// Busca no texto
	var matches []SearchMatch
	lines := strings.Split(content.Text, "\n")

	var searchPattern *regexp.Regexp
	if opts.UseRegex {
		if opts.CaseSensitive {
			searchPattern, err = regexp.Compile(query)
		} else {
			searchPattern, err = regexp.Compile("(?i)" + query)
		}
		if err != nil {
			return nil, fmt.Errorf("regex inválido: %w", err)
		}
	}

	for lineNum, line := range lines {
		var found bool
		if searchPattern != nil {
			found = searchPattern.MatchString(line)
		} else if opts.CaseSensitive {
			found = strings.Contains(line, query)
		} else {
			found = strings.Contains(strings.ToLower(line), strings.ToLower(query))
		}

		if found {
			match := SearchMatch{
				Text:       line,
				LineNumber: lineNum + 1,
			}

			// Adiciona contexto
			if opts.ContextLines > 0 {
				start := lineNum - opts.ContextLines
				if start < 0 {
					start = 0
				}
				end := lineNum + opts.ContextLines + 1
				if end > len(lines) {
					end = len(lines)
				}

				if start < lineNum {
					match.ContextBefore = lines[start:lineNum]
				}
				if lineNum+1 < end {
					match.ContextAfter = lines[lineNum+1 : end]
				}
			}

			matches = append(matches, match)

			if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
				break
			}
		}
	}

	return matches, nil
}

// GetMetadata retorna metadata do documento
func (h *GoogleDocsHandler) GetMetadata(path string) (map[string]interface{}, error) {
	docID := h.extractDocumentID(path)
	if docID == "" {
		return nil, fmt.Errorf("não foi possível extrair ID do documento de: %s", path)
	}

	token, err := h.getToken()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter token OAuth: %w", err)
	}

	fileInfo, err := h.getFileMetadata(docID, token)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":           fileInfo.ID,
		"name":         fileInfo.Name,
		"mimeType":     fileInfo.MimeType,
		"webViewLink":  fileInfo.WebViewLink,
		"modifiedTime": fileInfo.ModifiedTime,
		"createdTime":  fileInfo.CreatedTime,
		"size":         fileInfo.Size,
	}, nil
}

// ==================== Métodos internos ====================

// extractDocumentID extrai o ID do documento de várias formas de input
func (h *GoogleDocsHandler) extractDocumentID(path string) string {
	// Remove espaços
	path = strings.TrimSpace(path)

	// Se é um pseudo-path gdocs://
	if strings.HasPrefix(path, "gdocs://") {
		return strings.TrimPrefix(path, "gdocs://")
	}
	if strings.HasPrefix(path, "gsheet://") {
		return strings.TrimPrefix(path, "gsheet://")
	}
	if strings.HasPrefix(path, "gslides://") {
		return strings.TrimPrefix(path, "gslides://")
	}
	if strings.HasPrefix(path, "gdrive://") {
		return strings.TrimPrefix(path, "gdrive://")
	}

	// Se é uma URL do Google
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
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(path); len(matches) > 1 {
			return matches[1]
		}
	}

	// Se parece ser um ID diretamente (alfanumérico com hífens/underscores, tamanho típico)
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{25,60}$`, path); matched {
		return path
	}

	return ""
}

// getFileMetadata obtém metadata de um arquivo do Google Drive
func (h *GoogleDocsHandler) getFileMetadata(fileID, token string) (*GoogleDriveFile, error) {
	url := fmt.Sprintf("%s/files/%s?fields=id,name,mimeType,size,createdTime,modifiedTime,webViewLink,parents",
		googleDriveAPIBase, fileID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro da API do Google (%d): %s", resp.StatusCode, string(body))
	}

	var file GoogleDriveFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	return &file, nil
}

// readGoogleDoc lê conteúdo de um Google Document
func (h *GoogleDocsHandler) readGoogleDoc(docID, token string, content *Content) error {
	url := fmt.Sprintf("%s/documents/%s", googleDocsAPIBase, docID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Se não tem acesso ao Docs API, tenta exportar como texto
		return h.exportAsText(docID, token, MimeGoogleDoc, content)
	}

	var docData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&docData); err != nil {
		return fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	// Extrai o texto do documento
	text := h.extractTextFromGoogleDoc(docData)
	content.Text = text
	content.LineCount = len(strings.Split(text, "\n"))

	// Extrai links
	content.Links = h.extractLinksFromGoogleDoc(docData)

	return nil
}

// readGoogleSheet lê conteúdo de um Google Spreadsheet
func (h *GoogleDocsHandler) readGoogleSheet(docID, token string, content *Content, opts ReadOptions) error {
	// Usa a Sheets API para obter os dados
	url := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s?includeGridData=true", docID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Se não tem acesso à Sheets API, tenta exportar como CSV
		return h.exportAsText(docID, token, MimeGoogleSheet, content)
	}

	var sheetData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sheetData); err != nil {
		return fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	// Extrai as planilhas
	content.Sheets = h.extractSheetsFromSpreadsheet(sheetData, opts.SheetName)

	// Gera texto formatado
	var textBuilder strings.Builder
	for _, sheet := range content.Sheets {
		textBuilder.WriteString(fmt.Sprintf("=== Planilha: %s ===\n", sheet.Name))
		for _, row := range sheet.Rows {
			textBuilder.WriteString(strings.Join(row, "\t") + "\n")
		}
		textBuilder.WriteString("\n")
	}
	content.Text = textBuilder.String()

	return nil
}

// readGoogleSlides lê conteúdo de uma apresentação Google Slides
func (h *GoogleDocsHandler) readGoogleSlides(docID, token string, content *Content) error {
	url := fmt.Sprintf("https://slides.googleapis.com/v1/presentations/%s", docID)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Se não tem acesso, tenta exportar como texto
		return h.exportAsText(docID, token, MimeGoogleSlides, content)
	}

	var slideData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&slideData); err != nil {
		return fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	// Extrai os slides
	content.Slides = h.extractSlidesFromPresentation(slideData)
	content.PageCount = len(content.Slides)

	// Gera texto formatado
	var textBuilder strings.Builder
	for _, slide := range content.Slides {
		textBuilder.WriteString(fmt.Sprintf("=== Slide %d", slide.Number))
		if slide.Title != "" {
			textBuilder.WriteString(fmt.Sprintf(": %s", slide.Title))
		}
		textBuilder.WriteString(" ===\n")
		textBuilder.WriteString(slide.Content + "\n")
		if slide.Notes != "" {
			textBuilder.WriteString(fmt.Sprintf("[Notas: %s]\n", slide.Notes))
		}
		textBuilder.WriteString("\n")
	}
	content.Text = textBuilder.String()

	return nil
}

// exportAsText exporta um documento como texto simples usando a API de export
func (h *GoogleDocsHandler) exportAsText(docID, token, mimeType string, content *Content) error {
	// Define o formato de exportação baseado no tipo
	var exportMime string
	switch mimeType {
	case MimeGoogleDoc:
		exportMime = "text/plain"
	case MimeGoogleSheet:
		exportMime = "text/csv"
	case MimeGoogleSlides:
		exportMime = "text/plain"
	default:
		exportMime = "text/plain"
	}

	url := fmt.Sprintf("%s/files/%s/export?mimeType=%s", googleDriveAPIBase, docID, exportMime)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("erro ao exportar documento (%d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta: %w", err)
	}

	content.Text = string(body)
	content.LineCount = len(strings.Split(content.Text, "\n"))

	return nil
}

// extractTextFromGoogleDoc extrai texto de um documento Google Docs
func (h *GoogleDocsHandler) extractTextFromGoogleDoc(docData map[string]interface{}) string {
	var text strings.Builder

	body, ok := docData["body"].(map[string]interface{})
	if !ok {
		return ""
	}

	contentItems, ok := body["content"].([]interface{})
	if !ok {
		return ""
	}

	for _, item := range contentItems {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Processa parágrafos
		if para, ok := itemMap["paragraph"].(map[string]interface{}); ok {
			elements, ok := para["elements"].([]interface{})
			if !ok {
				continue
			}

			for _, elem := range elements {
				elemMap, ok := elem.(map[string]interface{})
				if !ok {
					continue
				}

				if textRun, ok := elemMap["textRun"].(map[string]interface{}); ok {
					if content, ok := textRun["content"].(string); ok {
						text.WriteString(content)
					}
				}
			}
		}

		// Processa tabelas
		if table, ok := itemMap["table"].(map[string]interface{}); ok {
			tableRows, ok := table["tableRows"].([]interface{})
			if !ok {
				continue
			}

			for _, row := range tableRows {
				rowMap, ok := row.(map[string]interface{})
				if !ok {
					continue
				}

				cells, ok := rowMap["tableCells"].([]interface{})
				if !ok {
					continue
				}

				var rowTexts []string
				for _, cell := range cells {
					cellMap, ok := cell.(map[string]interface{})
					if !ok {
						continue
					}

					cellContent, ok := cellMap["content"].([]interface{})
					if !ok {
						continue
					}

					var cellText strings.Builder
					for _, cc := range cellContent {
						ccMap, ok := cc.(map[string]interface{})
						if !ok {
							continue
						}

						if para, ok := ccMap["paragraph"].(map[string]interface{}); ok {
							elements, ok := para["elements"].([]interface{})
							if !ok {
								continue
							}

							for _, elem := range elements {
								elemMap, ok := elem.(map[string]interface{})
								if !ok {
									continue
								}

								if textRun, ok := elemMap["textRun"].(map[string]interface{}); ok {
									if content, ok := textRun["content"].(string); ok {
										cellText.WriteString(strings.TrimSpace(content))
									}
								}
							}
						}
					}
					rowTexts = append(rowTexts, cellText.String())
				}
				text.WriteString("| " + strings.Join(rowTexts, " | ") + " |\n")
			}
		}
	}

	return text.String()
}

// extractLinksFromGoogleDoc extrai links de um documento Google Docs
func (h *GoogleDocsHandler) extractLinksFromGoogleDoc(docData map[string]interface{}) []Link {
	var links []Link

	body, ok := docData["body"].(map[string]interface{})
	if !ok {
		return links
	}

	contentItems, ok := body["content"].([]interface{})
	if !ok {
		return links
	}

	for _, item := range contentItems {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if para, ok := itemMap["paragraph"].(map[string]interface{}); ok {
			elements, ok := para["elements"].([]interface{})
			if !ok {
				continue
			}

			for _, elem := range elements {
				elemMap, ok := elem.(map[string]interface{})
				if !ok {
					continue
				}

				if textRun, ok := elemMap["textRun"].(map[string]interface{}); ok {
					textStyle, ok := textRun["textStyle"].(map[string]interface{})
					if !ok {
						continue
					}

					if link, ok := textStyle["link"].(map[string]interface{}); ok {
						url, _ := link["url"].(string)
						content, _ := textRun["content"].(string)
						if url != "" {
							links = append(links, Link{
								Text: strings.TrimSpace(content),
								URL:  url,
							})
						}
					}
				}
			}
		}
	}

	return links
}

// extractSheetsFromSpreadsheet extrai planilhas de um spreadsheet
func (h *GoogleDocsHandler) extractSheetsFromSpreadsheet(sheetData map[string]interface{}, sheetName string) []Sheet {
	var sheets []Sheet

	sheetsArray, ok := sheetData["sheets"].([]interface{})
	if !ok {
		return sheets
	}

	for _, s := range sheetsArray {
		sheetMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		properties, ok := sheetMap["properties"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := properties["title"].(string)

		// Filtra por nome se especificado
		if sheetName != "" && !strings.EqualFold(name, sheetName) {
			continue
		}

		sheet := Sheet{
			Name: name,
		}

		// Extrai dados
		data, ok := sheetMap["data"].([]interface{})
		if !ok || len(data) == 0 {
			sheets = append(sheets, sheet)
			continue
		}

		gridData, ok := data[0].(map[string]interface{})
		if !ok {
			sheets = append(sheets, sheet)
			continue
		}

		rowData, ok := gridData["rowData"].([]interface{})
		if !ok {
			sheets = append(sheets, sheet)
			continue
		}

		for _, row := range rowData {
			rowMap, ok := row.(map[string]interface{})
			if !ok {
				continue
			}

			values, ok := rowMap["values"].([]interface{})
			if !ok {
				continue
			}

			var rowCells []string
			for _, cell := range values {
				cellMap, ok := cell.(map[string]interface{})
				if !ok {
					rowCells = append(rowCells, "")
					continue
				}

				// Tenta obter valor formatado
				if formattedValue, ok := cellMap["formattedValue"].(string); ok {
					rowCells = append(rowCells, formattedValue)
				} else {
					rowCells = append(rowCells, "")
				}
			}

			sheet.Rows = append(sheet.Rows, rowCells)
			if len(rowCells) > sheet.ColCount {
				sheet.ColCount = len(rowCells)
			}
		}

		sheet.RowCount = len(sheet.Rows)

		// Define headers como primeira linha
		if len(sheet.Rows) > 0 {
			sheet.Headers = sheet.Rows[0]
		}

		sheets = append(sheets, sheet)
	}

	return sheets
}

// extractSlidesFromPresentation extrai slides de uma apresentação
func (h *GoogleDocsHandler) extractSlidesFromPresentation(slideData map[string]interface{}) []Slide {
	var slides []Slide

	slidesArray, ok := slideData["slides"].([]interface{})
	if !ok {
		return slides
	}

	for i, s := range slidesArray {
		slideMap, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		slide := Slide{
			Number: i + 1,
		}

		// Extrai elementos do slide
		pageElements, ok := slideMap["pageElements"].([]interface{})
		if ok {
			var textContent []string

			for _, elem := range pageElements {
				elemMap, ok := elem.(map[string]interface{})
				if !ok {
					continue
				}

				// Processa shapes com texto
				if shape, ok := elemMap["shape"].(map[string]interface{}); ok {
					if text, ok := shape["text"].(map[string]interface{}); ok {
						textElements, ok := text["textElements"].([]interface{})
						if !ok {
							continue
						}

						for _, te := range textElements {
							teMap, ok := te.(map[string]interface{})
							if !ok {
								continue
							}

							if textRun, ok := teMap["textRun"].(map[string]interface{}); ok {
								if content, ok := textRun["content"].(string); ok {
									content = strings.TrimSpace(content)
									if content != "" {
										textContent = append(textContent, content)
									}
								}
							}
						}
					}

					// Verifica se é título
					if shapeType, ok := shape["shapeType"].(string); ok {
						if strings.Contains(shapeType, "TITLE") && len(textContent) > 0 {
							slide.Title = textContent[len(textContent)-1]
						}
					}
				}
			}

			slide.Content = strings.Join(textContent, "\n")
		}

		// Extrai notas do orador
		if notesPage, ok := slideMap["slideProperties"].(map[string]interface{}); ok {
			if notesId, ok := notesPage["notesPage"].(map[string]interface{}); ok {
				if pageElements, ok := notesId["pageElements"].([]interface{}); ok {
					var notes []string
					for _, elem := range pageElements {
						elemMap, ok := elem.(map[string]interface{})
						if !ok {
							continue
						}

						if shape, ok := elemMap["shape"].(map[string]interface{}); ok {
							if text, ok := shape["text"].(map[string]interface{}); ok {
								textElements, ok := text["textElements"].([]interface{})
								if !ok {
									continue
								}

								for _, te := range textElements {
									teMap, ok := te.(map[string]interface{})
									if !ok {
										continue
									}

									if textRun, ok := teMap["textRun"].(map[string]interface{}); ok {
										if content, ok := textRun["content"].(string); ok {
											content = strings.TrimSpace(content)
											if content != "" {
												notes = append(notes, content)
											}
										}
									}
								}
							}
						}
					}
					slide.Notes = strings.Join(notes, " ")
				}
			}
		}

		slides = append(slides, slide)
	}

	return slides
}

// ==================== Funções utilitárias para o FileAgent ====================

// ListGoogleDriveFiles lista arquivos do Google Drive
func (h *GoogleDocsHandler) ListGoogleDriveFiles(folderID, query, token string, maxResults int) ([]GoogleDriveFile, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	// Constrói a query
	var q []string
	if folderID != "" {
		q = append(q, fmt.Sprintf("'%s' in parents", folderID))
	}
	if query != "" {
		q = append(q, fmt.Sprintf("name contains '%s'", query))
	}
	q = append(q, "trashed = false")

	qStr := strings.Join(q, " and ")
	url := fmt.Sprintf("%s/files?q=%s&pageSize=%d&fields=files(id,name,mimeType,size,createdTime,modifiedTime,webViewLink,parents)",
		googleDriveAPIBase, qStr, maxResults)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro da API do Google (%d): %s", resp.StatusCode, string(body))
	}

	var list GoogleDriveList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	return list.Files, nil
}

// SearchGoogleDriveFiles busca arquivos no Google Drive por conteúdo
func (h *GoogleDocsHandler) SearchGoogleDriveFiles(fullTextQuery, token string, maxResults int) ([]GoogleDriveFile, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	q := fmt.Sprintf("fullText contains '%s' and trashed = false", fullTextQuery)
	url := fmt.Sprintf("%s/files?q=%s&pageSize=%d&fields=files(id,name,mimeType,size,createdTime,modifiedTime,webViewLink)",
		googleDriveAPIBase, q, maxResults)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro da API do Google (%d): %s", resp.StatusCode, string(body))
	}

	var list GoogleDriveList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	return list.Files, nil
}

// IsGoogleDocsPath verifica se um caminho é um identificador de Google Docs
func IsGoogleDocsPath(path string) bool {
	path = strings.TrimSpace(path)

	// Pseudo-protocolos
	if strings.HasPrefix(path, "gdocs://") ||
		strings.HasPrefix(path, "gsheet://") ||
		strings.HasPrefix(path, "gslides://") ||
		strings.HasPrefix(path, "gdrive://") {
		return true
	}

	// URLs do Google
	if strings.Contains(path, "docs.google.com") ||
		strings.Contains(path, "drive.google.com") {
		return true
	}

	return false
}



