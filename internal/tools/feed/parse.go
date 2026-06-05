package feed

import (
	"io"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

// parseOptions controla a normalização do feed para o JSON canônico.
type parseOptions struct {
	MaxItems       int        // 0 = sem limite
	IncludeContent bool       // inclui o corpo completo (content) de cada item
	StripHTML      bool       // converte summary/content de HTML para texto
	Since          *time.Time // se != nil, descarta itens publicados antes deste instante
}

// CanonicalImage é uma imagem associada ao feed.
type CanonicalImage struct {
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

// CanonicalPodcastFeed agrega metadados de podcast no nível do feed (iTunes).
type CanonicalPodcastFeed struct {
	Author     string   `json:"author,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Explicit   string   `json:"explicit,omitempty"`
	Type       string   `json:"type,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Image      string   `json:"image,omitempty"`
}

// CanonicalEnclosure é um anexo (áudio/vídeo/arquivo) de um item.
type CanonicalEnclosure struct {
	URL    string `json:"url,omitempty"`
	Type   string `json:"type,omitempty"`
	Length string `json:"length,omitempty"`
}

// CanonicalPodcastItem agrega metadados de podcast no nível do item (iTunes).
type CanonicalPodcastItem struct {
	Duration    string `json:"duration,omitempty"`
	Episode     string `json:"episode,omitempty"`
	Season      string `json:"season,omitempty"`
	EpisodeType string `json:"episode_type,omitempty"`
	Explicit    string `json:"explicit,omitempty"`
	Image       string `json:"image,omitempty"`
}

// CanonicalItem é a forma normalizada de uma entrada do feed.
type CanonicalItem struct {
	Title      string                `json:"title,omitempty"`
	Link       string                `json:"link,omitempty"`
	GUID       string                `json:"guid,omitempty"`
	Published  string                `json:"published,omitempty"`
	Updated    string                `json:"updated,omitempty"`
	Authors    []string              `json:"authors,omitempty"`
	Categories []string              `json:"categories,omitempty"`
	Summary    string                `json:"summary,omitempty"`
	Content    string                `json:"content,omitempty"`
	Image      string                `json:"image,omitempty"`
	Enclosures []CanonicalEnclosure  `json:"enclosures,omitempty"`
	Podcast    *CanonicalPodcastItem `json:"podcast,omitempty"`
}

// CanonicalFeed é a representação JSON estável independente do formato de origem
// (RSS/Atom/JSON Feed). Quando há extensão iTunes/áudio, IsPodcast é true e os
// blocos Podcast são preenchidos.
type CanonicalFeed struct {
	Title       string                `json:"title,omitempty"`
	Description string                `json:"description,omitempty"`
	Link        string                `json:"link,omitempty"`
	FeedLink    string                `json:"feed_link,omitempty"`
	FeedType    string                `json:"feed_type,omitempty"`
	FeedVersion string                `json:"feed_version,omitempty"`
	Language    string                `json:"language,omitempty"`
	Updated     string                `json:"updated,omitempty"`
	Published   string                `json:"published,omitempty"`
	Authors     []string              `json:"authors,omitempty"`
	Image       *CanonicalImage       `json:"image,omitempty"`
	Categories  []string              `json:"categories,omitempty"`
	IsPodcast   bool                  `json:"is_podcast"`
	Podcast     *CanonicalPodcastFeed `json:"podcast,omitempty"`
	ItemCount   int                   `json:"item_count"`
	Items       []CanonicalItem       `json:"items"`
}

// parseFeed lê bytes de um feed (qualquer formato suportado pelo gofeed) e
// devolve a forma canônica, aplicando os filtros de opts. É puro (sem rede),
// para ser testável com fixtures.
func parseFeed(r io.Reader, opts parseOptions) (CanonicalFeed, error) {
	fp := gofeed.NewParser()
	src, err := fp.Parse(r)
	if err != nil {
		return CanonicalFeed{}, err
	}

	out := CanonicalFeed{
		Title:       strings.TrimSpace(src.Title),
		Description: cleanText(src.Description, opts.StripHTML),
		Link:        src.Link,
		FeedLink:    src.FeedLink,
		FeedType:    src.FeedType,
		FeedVersion: src.FeedVersion,
		Language:    src.Language,
		Updated:     normalizeTime(src.Updated, src.UpdatedParsed),
		Published:   normalizeTime(src.Published, src.PublishedParsed),
		Authors:     personNames(src.Authors),
		Categories:  src.Categories,
		// Inicializa não-nulo para que um feed sem itens serialize como "items": []
		// (JSON canônico estável), nunca "items": null.
		Items: []CanonicalItem{},
	}
	if src.Image != nil {
		out.Image = &CanonicalImage{URL: src.Image.URL, Title: src.Image.Title}
	}

	if src.ITunesExt != nil {
		out.IsPodcast = true
		pf := &CanonicalPodcastFeed{
			Author:   src.ITunesExt.Author,
			Explicit: src.ITunesExt.Explicit,
			Type:     src.ITunesExt.Type,
			Image:    src.ITunesExt.Image,
		}
		if src.ITunesExt.Owner != nil {
			pf.Owner = firstNonEmpty(src.ITunesExt.Owner.Name, src.ITunesExt.Owner.Email)
		}
		for _, c := range src.ITunesExt.Categories {
			if c != nil && strings.TrimSpace(c.Text) != "" {
				pf.Categories = append(pf.Categories, c.Text)
			}
		}
		out.Podcast = pf
	}

	for _, it := range src.Items {
		if it == nil {
			continue
		}
		// Filtro since: prefere a data de publicação, mas cai para a de atualização
		// (comum em Atom, que muitas vezes só tem <updated>). Itens sem nenhuma data
		// parseável são preservados.
		if opts.Since != nil {
			itemDate := it.PublishedParsed
			if itemDate == nil {
				itemDate = it.UpdatedParsed
			}
			if itemDate != nil && itemDate.Before(*opts.Since) {
				continue
			}
		}
		ci := CanonicalItem{
			Title:      strings.TrimSpace(it.Title),
			Link:       it.Link,
			GUID:       it.GUID,
			Published:  normalizeTime(it.Published, it.PublishedParsed),
			Updated:    normalizeTime(it.Updated, it.UpdatedParsed),
			Authors:    personNames(it.Authors),
			Categories: it.Categories,
			Summary:    cleanText(it.Description, opts.StripHTML),
		}
		if it.Image != nil {
			ci.Image = it.Image.URL
		}
		if opts.IncludeContent {
			ci.Content = cleanText(it.Content, opts.StripHTML)
		}
		for _, enc := range it.Enclosures {
			if enc == nil {
				continue
			}
			ci.Enclosures = append(ci.Enclosures, CanonicalEnclosure{
				URL:    enc.URL,
				Type:   enc.Type,
				Length: enc.Length,
			})
			// Podcast RSS "simples" pode não ter namespace iTunes; um enclosure de
			// áudio também sinaliza que é um podcast.
			if strings.HasPrefix(strings.ToLower(enc.Type), "audio/") {
				out.IsPodcast = true
			}
		}
		if it.ITunesExt != nil {
			out.IsPodcast = true
			ci.Podcast = &CanonicalPodcastItem{
				Duration:    it.ITunesExt.Duration,
				Episode:     it.ITunesExt.Episode,
				Season:      it.ITunesExt.Season,
				EpisodeType: it.ITunesExt.EpisodeType,
				Explicit:    it.ITunesExt.Explicit,
				Image:       it.ITunesExt.Image,
			}
		}
		out.Items = append(out.Items, ci)
		if opts.MaxItems > 0 && len(out.Items) >= opts.MaxItems {
			break
		}
	}

	out.ItemCount = len(out.Items)
	return out, nil
}

// normalizeTime prefere o timestamp já parseado (em RFC3339); cai para a string
// crua do feed quando o parse falhou.
func normalizeTime(raw string, parsed *time.Time) string {
	if parsed != nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return strings.TrimSpace(raw)
}

// personNames extrai os nomes (ou e-mails) de uma lista de autores do gofeed.
func personNames(people []*gofeed.Person) []string {
	if len(people) == 0 {
		return nil
	}
	names := make([]string, 0, len(people))
	for _, p := range people {
		if p == nil {
			continue
		}
		if n := firstNonEmpty(p.Name, p.Email); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// cleanText opcionalmente remove HTML do texto; sempre normaliza espaços nas
// bordas.
func cleanText(s string, stripHTML bool) string {
	if stripHTML {
		s = htmlToText(s)
	}
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
