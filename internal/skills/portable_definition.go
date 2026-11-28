package skills

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"assistente/internal/pluginpackage"
)

// PortableDefinitions indexes the exact directory entries discovered for one
// operation. Only selected definitions are opened.
type PortableDefinitions map[string]func() ([]byte, error)

func (m *Manager) DiscoverPortableDefinitions() (PortableDefinitions, error) {
	items, err := m.discoverNative()
	definitions := make(PortableDefinitions, len(items))
	for _, item := range items {
		definitions[item.slug] = func() ([]byte, error) { return readPortableDefinition(item) }
	}
	return definitions, err
}

func readPortableDefinition(item discoveredSkill) ([]byte, error) {
	base, err := os.OpenRoot(filepath.Dir(filepath.Dir(item.path)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = base.Close() }()
	root, err := base.OpenRoot(item.slug)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	folder, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = folder.Close() }()
	entries, err := folder.ReadDir(2)
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 || entries[0].Name() != skillFile {
		return nil, errors.New("SKILL_BUILDER_SUPPORTING_FILES")
	}
	data, err := pluginpackage.ReadFile(root, skillFile, 1<<20)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errors.New("PLUGIN_BUILDER_SKILL_UNAVAILABLE")
	}
	return data, nil
}

// WithLinkableDefinitions keeps application-managed edits coordinated with the
// acceptance of a plugin link. Display-name aliases cannot supply a slug.
func (m *Manager) WithLinkableDefinitions(use func(PortableDefinitions, error) error) error {
	m.definitionsMu.RLock()
	defer m.definitionsMu.RUnlock()
	items, discoveryErr := m.discoverNative()
	definitions := make(PortableDefinitions, len(items))
	aliases := map[string]bool{}
	for _, item := range items {
		definitions[item.slug] = func() ([]byte, error) { return readPortableDefinition(item) }
		alias, err := nativeSelectionName(item)
		if err != nil {
			discoveryErr = err
			continue
		}
		if alias != "" && alias != item.slug {
			aliases[alias] = true
		}
	}
	for name := range aliases {
		definitions[name] = func() ([]byte, error) { return nil, errors.New("skill name is a display alias") }
	}
	return use(definitions, discoveryErr)
}

func nativeSelectionName(item discoveredSkill) (string, error) {
	root, err := os.OpenRoot(filepath.Dir(item.path))
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	f, err := pluginpackage.OpenRegularFile(root, skillFile)
	if errors.Is(err, os.ErrInvalid) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	// Parse accepts leading whitespace and a closing delimiter at the start of a line.
	r := bufio.NewReader(f)
	for {
		ch, err := r.ReadByte()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if ch != ' ' && ch != '\t' && ch != '\r' && ch != '\n' {
			if err := r.UnreadByte(); err != nil {
				return "", err
			}
			break
		}
	}
	opening, err := r.Peek(len(frontmatterDelimiter))
	if err != nil && err != io.EOF {
		return "", err
	}
	if string(opening) != frontmatterDelimiter {
		return "", nil
	}
	var header bytes.Buffer
	for {
		chunk, err := r.ReadSlice('\n')
		header.Write(chunk)
		if header.Len() > 1<<20 {
			return "", errors.New("skill frontmatter exceeds review limit")
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		next, err := r.Peek(len(frontmatterDelimiter))
		if err != nil && err != io.EOF {
			return "", err
		}
		if string(next) == frontmatterDelimiter {
			header.WriteString(frontmatterDelimiter)
			break
		}
	}
	meta, _, err := Parse(header.String())
	if err != nil {
		return "", nil
	} // The native catalog also skips invalid definitions.
	return meta.Name, nil
}
