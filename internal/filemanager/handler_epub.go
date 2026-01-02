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

	"golang.org/x/net/html"
)

// EPUBHandler manipula arquivos EPUB (todas as versões: 2.0, 3.0, 3.2)
// EPUB é um formato de ebook baseado em ZIP contendo XHTML/HTML
type EPUBHandler struct{}

// NewEPUBHandler cria um novo handler para arquivos EPUB
func NewEPUBHandler() *EPUBHandler {
	return &EPUBHandler{}
}

// Name retorna o nome do handler
func (h *EPUBHandler) Name() string {
	return "epub"
}

// Extensions retorna as extensões suportadas
func (h *EPUBHandler) Extensions() []string {
	return []string{".epub"}
}

// MimeTypes retorna os MIME types suportados
func (h *EPUBHandler) MimeTypes() []string {
	return []string{
		"application/epub+zip",
	}
}

// Capabilities retorna as capacidades do handler
func (h *EPUBHandler) Capabilities() Capabilities {
	return Capabilities{
		CanRead:    true,
		CanWrite:   false,
		CanEdit:    false,
		CanSearch:  true,
		CanExtract: true,
	}
}

// Estruturas XML para parsing do EPUB

// container.xml
type epubContainer struct {
	XMLName  xml.Name       `xml:"container"`
	RootFile epubRootFile   `xml:"rootfiles>rootfile"`
}

type epubRootFile struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

// content.opf (package)
type epubPackage struct {
	XMLName  xml.Name     `xml:"package"`
	Version  string       `xml:"version,attr"`
	Metadata epubMetadata `xml:"metadata"`
	Manifest epubManifest `xml:"manifest"`
	Spine    epubSpine    `xml:"spine"`
}

type epubMetadata struct {
	Title       []string `xml:"title"`
	Creator     []string `xml:"creator"`
	Language    []string `xml:"language"`
	Publisher   []string `xml:"publisher"`
	Description []string `xml:"description"`
	Subject     []string `xml:"subject"`
	Date        []string `xml:"date"`
	Identifier  []string `xml:"identifier"`
}

type epubManifest struct {
	Items []epubManifestItem `xml:"item"`
}

type epubManifestItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type epubSpine struct {
	Toc      string          `xml:"toc,attr"`
	ItemRefs []epubItemRef   `xml:"itemref"`
}

type epubItemRef struct {
	IDRef  string `xml:"idref,attr"`
	Linear string `xml:"linear,attr"`
}

// ReadContent lê o conteúdo de um arquivo EPUB
func (h *EPUBHandler) ReadContent(path string, opts ReadOptions) (*Content, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo EPUB: %w", err)
	}
	defer r.Close()

	content := &Content{
		Sections: []Section{},
		Images:   []EmbeddedImage{},
		Links:    []Link{},
		Metadata: make(map[string]interface{}),
	}

	// 1. Lê container.xml para encontrar o OPF
	container, err := h.readContainer(r)
	if err != nil {
		return nil, err
	}

	// 2. Lê o arquivo OPF (content.opf)
	opfPath := container.RootFile.FullPath
	opfDir := filepath.Dir(opfPath)

	pkg, err := h.readOPF(r, opfPath)
	if err != nil {
		return nil, err
	}

	// 3. Extrai metadados
	content.Metadata["version"] = pkg.Version
	if len(pkg.Metadata.Title) > 0 {
		content.Metadata["title"] = pkg.Metadata.Title[0]
	}
	if len(pkg.Metadata.Creator) > 0 {
		content.Metadata["author"] = strings.Join(pkg.Metadata.Creator, ", ")
	}
	if len(pkg.Metadata.Language) > 0 {
		content.Metadata["language"] = pkg.Metadata.Language[0]
	}
	if len(pkg.Metadata.Publisher) > 0 {
		content.Metadata["publisher"] = pkg.Metadata.Publisher[0]
	}
	if len(pkg.Metadata.Description) > 0 {
		content.Metadata["description"] = pkg.Metadata.Description[0]
	}

	// 4. Cria mapa de itens do manifest
	manifestMap := make(map[string]epubManifestItem)
	for _, item := range pkg.Manifest.Items {
		manifestMap[item.ID] = item
	}

	// 5. Conta imagens
	imageCount := 0
	for _, item := range pkg.Manifest.Items {
		if strings.HasPrefix(item.MediaType, "image/") {
			imageCount++
			content.Images = append(content.Images, EmbeddedImage{
				Name:     filepath.Base(item.Href),
				MimeType: item.MediaType,
			})
		}
	}
	content.Metadata["image_count"] = imageCount

	// 6. Lê conteúdo na ordem do spine
	var textParts []string
	chapterNum := 0

	for _, itemRef := range pkg.Spine.ItemRefs {
		item, exists := manifestMap[itemRef.IDRef]
		if !exists {
			continue
		}

		// Só processa arquivos XHTML/HTML
		if !strings.Contains(item.MediaType, "html") && !strings.Contains(item.MediaType, "xml") {
			continue
		}

		// Resolve o caminho relativo ao OPF
		itemPath := item.Href
		if opfDir != "." && opfDir != "" {
			itemPath = opfDir + "/" + item.Href
		}

		// Lê o conteúdo do capítulo
		chapterText, chapterTitle := h.readChapter(r, itemPath)
		if chapterText == "" {
			continue
		}

		chapterNum++

		section := Section{
			Title:   chapterTitle,
			Content: chapterText,
			Page:    chapterNum,
		}

		if section.Title == "" {
			section.Title = fmt.Sprintf("Capítulo %d", chapterNum)
		}

		content.Sections = append(content.Sections, section)
		textParts = append(textParts, fmt.Sprintf("=== %s ===\n%s", section.Title, chapterText))
	}

	content.Text = strings.Join(textParts, "\n\n")
	content.Metadata["chapter_count"] = chapterNum
	content.LineCount = strings.Count(content.Text, "\n") + 1

	return content, nil
}

// readContainer lê o container.xml
func (h *EPUBHandler) readContainer(r *zip.ReadCloser) (*epubContainer, error) {
	for _, f := range r.File {
		if f.Name == "META-INF/container.xml" {
			reader, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}

			var container epubContainer
			if err := xml.Unmarshal(data, &container); err != nil {
				return nil, fmt.Errorf("erro ao parsear container.xml: %w", err)
			}

			return &container, nil
		}
	}

	return nil, fmt.Errorf("container.xml não encontrado")
}

// readOPF lê o arquivo OPF
func (h *EPUBHandler) readOPF(r *zip.ReadCloser, opfPath string) (*epubPackage, error) {
	for _, f := range r.File {
		if f.Name == opfPath {
			reader, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer reader.Close()

			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}

			var pkg epubPackage
			if err := xml.Unmarshal(data, &pkg); err != nil {
				return nil, fmt.Errorf("erro ao parsear OPF: %w", err)
			}

			return &pkg, nil
		}
	}

	return nil, fmt.Errorf("arquivo OPF não encontrado: %s", opfPath)
}

// readChapter lê o conteúdo de um capítulo XHTML/HTML
func (h *EPUBHandler) readChapter(r *zip.ReadCloser, chapterPath string) (string, string) {
	// Normaliza o caminho
	chapterPath = strings.ReplaceAll(chapterPath, "\\", "/")

	for _, f := range r.File {
		if f.Name == chapterPath || strings.HasSuffix(f.Name, "/"+chapterPath) {
			reader, err := f.Open()
			if err != nil {
				return "", ""
			}
			defer reader.Close()

			// Parse HTML/XHTML
			doc, err := html.Parse(reader)
			if err != nil {
				return "", ""
			}

			var title string
			var textContent []string

			// Extrai texto recursivamente
			var extractText func(*html.Node)
			extractText = func(n *html.Node) {
				if n.Type == html.ElementNode {
					// Ignora scripts e styles
					if n.Data == "script" || n.Data == "style" {
						return
					}

					// Extrai título
					if n.Data == "title" && title == "" {
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							if c.Type == html.TextNode {
								title = strings.TrimSpace(c.Data)
							}
						}
					}

					// Extrai h1 como título alternativo
					if n.Data == "h1" && title == "" {
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							if c.Type == html.TextNode {
								title = strings.TrimSpace(c.Data)
							}
						}
					}
				}

				if n.Type == html.TextNode {
					text := strings.TrimSpace(n.Data)
					if text != "" {
						textContent = append(textContent, text)
					}
				}

				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extractText(c)
				}
			}

			extractText(doc)

			return strings.Join(textContent, " "), title
		}
	}

	return "", ""
}

// WriteContent não é suportado para EPUB
func (h *EPUBHandler) WriteContent(path string, content *Content, opts WriteOptions) error {
	return fmt.Errorf("escrita de arquivos EPUB não é suportada")
}

// SearchContent busca conteúdo dentro do arquivo EPUB
func (h *EPUBHandler) SearchContent(path string, query string, opts SearchOptions) ([]SearchMatch, error) {
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

	// Busca em cada seção/capítulo
	for _, section := range content.Sections {
		// Busca no título
		if loc := searchFunc(section.Title); loc != nil {
			matches = append(matches, SearchMatch{
				Text:       query,
				Context:    section.Title,
				PageNumber: section.Page,
				Location:   fmt.Sprintf("%s (título)", section.Title),
			})
		}

		// Busca no conteúdo
		// Divide em sentenças para melhor contexto
		sentences := splitIntoSentences(section.Content)
		for _, sentence := range sentences {
			if loc := searchFunc(sentence); loc != nil {
				context := sentence
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
					PageNumber: section.Page,
					Location:   section.Title,
				})

				if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
					return matches, nil
				}
			}
		}
	}

	return matches, nil
}

// splitIntoSentences divide texto em sentenças
func splitIntoSentences(text string) []string {
	// Divide por pontuação de fim de sentença
	re := regexp.MustCompile(`[.!?]+\s+`)
	parts := re.Split(text, -1)

	var sentences []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			sentences = append(sentences, p)
		}
	}

	return sentences
}

// GetMetadata obtém metadados do arquivo EPUB
func (h *EPUBHandler) GetMetadata(path string) (map[string]interface{}, error) {
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
	charCount := len(content.Text)

	metadata := map[string]interface{}{
		"extension":    ".epub",
		"file_size":    fileSize,
		"word_count":   wordCount,
		"char_count":   charCount,
		"is_text":      false,
		"handler":      "epub",
		"can_write":    false,
		"can_search":   true,
		"can_extract":  true,
		"content_type": "ebook",
	}

	// Adiciona metadados do EPUB
	for k, v := range content.Metadata {
		metadata[k] = v
	}

	return metadata, nil
}

// ListChapters retorna a lista de capítulos do EPUB
func (h *EPUBHandler) ListChapters(path string) ([]Section, error) {
	content, err := h.ReadContent(path, ReadOptions{})
	if err != nil {
		return nil, err
	}

	// Retorna apenas título e número, sem conteúdo completo
	chapters := make([]Section, len(content.Sections))
	for i, s := range content.Sections {
		chapters[i] = Section{
			Title: s.Title,
			Page:  s.Page,
		}
	}

	// Ordena por número de página
	sort.Slice(chapters, func(i, j int) bool {
		return chapters[i].Page < chapters[j].Page
	})

	return chapters, nil
}



