package filemanager

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PowerPointHandler manipula arquivos PowerPoint (.pptx)
// Faz parsing direto do XML interno do .pptx
type PowerPointHandler struct{}

// NewPowerPointHandler cria um novo handler para arquivos PowerPoint
func NewPowerPointHandler() *PowerPointHandler {
	return &PowerPointHandler{}
}

// Name retorna o nome do handler
func (h *PowerPointHandler) Name() string {
	return "powerpoint"
}

// Extensions retorna as extensões suportadas
func (h *PowerPointHandler) Extensions() []string {
	return []string{".pptx"}
}

// MimeTypes retorna os MIME types suportados
func (h *PowerPointHandler) MimeTypes() []string {
	return []string{
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
}

// Capabilities retorna as capacidades do handler
func (h *PowerPointHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   false,
		CanEdit:    false,
		CanSearch:  true,
		CanExtract: true,
	}
}

// Estruturas XML para parsing do PPTX

type pptxSlide struct {
	XMLName xml.Name    `xml:"sld"`
	CSld    pptxCSld    `xml:"cSld"`
}

type pptxCSld struct {
	SpTree pptxSpTree `xml:"spTree"`
}

type pptxSpTree struct {
	Shapes []pptxShape `xml:"sp"`
}

type pptxShape struct {
	TxBody *pptxTxBody `xml:"txBody"`
}

type pptxTxBody struct {
	Paragraphs []pptxParagraph `xml:"p"`
}

type pptxParagraph struct {
	Runs []pptxRun `xml:"r"`
}

type pptxRun struct {
	Text string `xml:"t"`
}

// ReadContent lê o conteúdo de um arquivo PowerPoint
func (h *PowerPointHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo PowerPoint: %w", err)
	}
	defer r.Close()

	content := &Content{
		Slides:   []Slide{},
		Images:   []EmbeddedImage{},
		Metadata: make(map[string]interface{}),
	}

	// Encontra todos os slides (ppt/slides/slide1.xml, slide2.xml, etc)
	var slideFiles []string
	var imageCount int

	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			slideFiles = append(slideFiles, f.Name)
		}
		if strings.HasPrefix(f.Name, "ppt/media/") {
			imageCount++
		}
	}

	// Ordena os slides por número
	sort.Slice(slideFiles, func(i, j int) bool {
		return slideFiles[i] < slideFiles[j]
	})

	content.Metadata["slide_count"] = len(slideFiles)
	content.Metadata["image_count"] = imageCount

	var textParts []string

	for i, slidePath := range slideFiles {
		slideNum := i + 1

		// Encontra o arquivo do slide
		var slideFile *zip.File
		for _, f := range r.File {
			if f.Name == slidePath {
				slideFile = f
				break
			}
		}

		if slideFile == nil {
			continue
		}

		// Lê o XML do slide
		xmlReader, err := slideFile.Open()
		if err != nil {
			continue
		}

		xmlBytes, err := io.ReadAll(xmlReader)
		xmlReader.Close()
		if err != nil {
			continue
		}

		// Parse o XML
		var slide pptxSlide
		if err := xml.Unmarshal(xmlBytes, &slide); err != nil {
			continue
		}

		// Extrai texto
		var slideContent []string
		for _, shape := range slide.CSld.SpTree.Shapes {
			if shape.TxBody == nil {
				continue
			}
			for _, para := range shape.TxBody.Paragraphs {
				var paraText []string
				for _, run := range para.Runs {
					if run.Text != "" {
						paraText = append(paraText, run.Text)
					}
				}
				text := strings.TrimSpace(strings.Join(paraText, ""))
				if text != "" {
					slideContent = append(slideContent, text)
				}
			}
		}

		slideData := Slide{
			Number: slideNum,
		}

		if len(slideContent) > 0 {
			slideData.Title = slideContent[0]
			if len(slideContent) > 1 {
				slideData.Content = strings.Join(slideContent[1:], "\n")
			}
		}

		content.Slides = append(content.Slides, slideData)
		textParts = append(textParts, fmt.Sprintf("--- Slide %d ---\n%s", slideNum, strings.Join(slideContent, "\n")))
	}

	content.Text = strings.Join(textParts, "\n\n")

	return content, nil
}

// WriteContent não é suportado para PowerPoint
func (h *PowerPointHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita de arquivos PowerPoint não é suportada")
}

// SearchContent busca conteúdo dentro do arquivo PowerPoint
func (h *PowerPointHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
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

	// Busca em cada slide
	for _, slide := range content.Slides {
		// Busca no título
		if loc := searchFunc(slide.Title); loc != nil {
			matches = append(matches, SearchMatch{
				Text:       query,
				Context:    slide.Title,
				PageNumber: slide.Number,
				Location:   fmt.Sprintf("Slide %d (título)", slide.Number),
			})
		}

		// Busca no conteúdo
		lines := strings.Split(slide.Content, "\n")
		for _, line := range lines {
			if loc := searchFunc(line); loc != nil {
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
					PageNumber: slide.Number,
					Location:   fmt.Sprintf("Slide %d", slide.Number),
				})

				if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
					return matches, nil
				}
			}
		}
	}

	return matches, nil
}

// GetMetadata obtém metadados do arquivo PowerPoint
func (h *PowerPointHandler) GetMetadata(path string) (map[string]interface{}, error) {
	content, err := h.ReadContent(path, ReadOptions{})
	if err != nil {
		return nil, err
	}

	stat, _ := os.Stat(path)
	fileSize := int64(0)
	if stat != nil {
		fileSize = stat.Size()
	}

	wordCount := len(strings.Fields(content.Text))

	metadata := map[string]interface{}{
		"extension":    filepath.Ext(path),
		"slide_count":  len(content.Slides),
		"word_count":   wordCount,
		"file_size":    fileSize,
		"is_text":      false,
		"handler":      "powerpoint",
		"can_write":    false,
		"can_search":   true,
		"can_extract":  true,
		"content_type": "presentation",
	}

	for k, v := range content.Metadata {
		metadata[k] = v
	}

	return metadata, nil
}
