package docextract

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var htmlTagRe = regexp.MustCompile(`(?is)<[^>]+>`)
var htmlEntityRe = regexp.MustCompile(`&(#?\w+);`)
var htmlScriptRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
var htmlStyleRe = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
var htmlBrRe = regexp.MustCompile(`(?is)<br\s*/?>`)
var htmlPEndRe = regexp.MustCompile(`(?is)</p>`)
var htmlDivEndRe = regexp.MustCompile(`(?is)</div>`)
var htmlHeadingEndRe = regexp.MustCompile(`(?is)</h[1-6]>`)

func extractEPUB(data []byte, filename string) (*Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	lim := &zipLimits{}

	container := findZipName(zr, "META-INF/container.xml")
	if container == nil {
		return nil, fmt.Errorf("EPUB sem META-INF/container.xml")
	}
	cbody, err := readZipFile(container, lim)
	if err != nil {
		return nil, err
	}
	opfPath, err := findRootfile(cbody)
	if err != nil {
		return nil, err
	}
	if opfPath == "" {
		return nil, fmt.Errorf("EPUB sem rootfile no container")
	}
	opfPath = strings.ReplaceAll(opfPath, "\\", "/")
	opfFile := findZipName(zr, opfPath)
	if opfFile == nil {
		return nil, fmt.Errorf("EPUB rootfile ausente: %s", opfPath)
	}
	opfBody, err := readZipFile(opfFile, lim)
	if err != nil {
		return nil, err
	}
	base := path.Dir(opfPath)
	items, spine, err := parseOPF(opfBody)
	if err != nil {
		return nil, err
	}

	var parts []string
	for i, id := range spine {
		href, ok := items[id]
		if !ok {
			continue
		}
		full := path.Join(base, href)
		full = strings.ReplaceAll(full, "\\", "/")
		f := findZipName(zr, full)
		if f == nil {
			continue
		}
		body, err := readZipFile(f, lim)
		if err != nil {
			return nil, err
		}
		text := htmlToText(string(body))
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("## Capítulo %d\n\n%s\n", i+1, text))
	}

	return &Result{
		Kind:     KindEPUB,
		Source:   filename,
		Pages:    len(parts),
		Markdown: strings.Join(parts, "\n") + "\n",
	}, nil
}

func findRootfile(containerXML []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(containerXML))
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return "", err
		}
		if done {
			return "", nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok || local(se.Name) != "rootfile" {
			continue
		}
		for _, a := range se.Attr {
			if local(a.Name) == "full-path" {
				return a.Value, nil
			}
		}
	}
}

func parseOPF(data []byte) (map[string]string, []string, error) {
	items := map[string]string{}
	var spine []string
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, done, err := nextToken(dec)
		if err != nil {
			return nil, nil, err
		}
		if done {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch local(se.Name) {
		case "item":
			var id, href, mt string
			for _, a := range se.Attr {
				switch local(a.Name) {
				case "id":
					id = a.Value
				case "href":
					href = a.Value
				case "media-type":
					mt = a.Value
				}
			}
			if id != "" && href != "" && (strings.Contains(mt, "html") || mt == "" || strings.HasSuffix(href, ".xhtml") || strings.HasSuffix(href, ".html")) {
				items[id] = href
			}
		case "itemref":
			for _, a := range se.Attr {
				if local(a.Name) == "idref" {
					spine = append(spine, a.Value)
				}
			}
		}
	}
	return items, spine, nil
}

func htmlToText(s string) string {
	s = htmlScriptRe.ReplaceAllString(s, "")
	s = htmlStyleRe.ReplaceAllString(s, "")
	s = htmlBrRe.ReplaceAllString(s, "\n")
	s = htmlPEndRe.ReplaceAllString(s, "\n\n")
	s = htmlDivEndRe.ReplaceAllString(s, "\n")
	s = htmlHeadingEndRe.ReplaceAllString(s, "\n\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = decodeEntities(s)
	s = xmlSpaceCollapse.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	var out []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n\n")
}

func decodeEntities(s string) string {
	repl := map[string]string{
		"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'", "nbsp": " ",
	}
	return htmlEntityRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[1 : len(m)-1]
		if strings.HasPrefix(inner, "#x") || strings.HasPrefix(inner, "#X") {
			var v int
			if _, err := fmt.Sscanf(inner[2:], "%x", &v); err == nil && v > 0 {
				return string(rune(v))
			}
			return m
		}
		if strings.HasPrefix(inner, "#") {
			var v int
			if _, err := fmt.Sscanf(inner[1:], "%d", &v); err == nil && v > 0 {
				return string(rune(v))
			}
			return m
		}
		if r, ok := repl[inner]; ok {
			return r
		}
		return m
	})
}
