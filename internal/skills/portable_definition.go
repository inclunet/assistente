package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"assistente/internal/pluginpackage"
)

// BuilderSlugs enumerates names using native resolution, without loading file contents.
func (m *Manager) BuilderSlugs() []string {
	discovered := m.discoverAll()
	slugs := make([]string, 0, len(discovered))
	for _, item := range discovered {
		slugs = append(slugs, item.slug)
	}
	sort.Strings(slugs)
	return slugs
}

// PortableDefinition reads a selected native skill from its configured root.
// A definition with additional files cannot be published as a single SKILL.md.
func (m *Manager) PortableDefinition(slug string) ([]byte, error) {
	for _, item := range m.discoverAll() {
		if item.slug != slug {
			continue
		}
		base, err := os.OpenRoot(filepath.Dir(filepath.Dir(item.path)))
		if err != nil {
			return nil, err
		}
		defer func() { _ = base.Close() }()
		root, err := base.OpenRoot(slug)
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
	return nil, os.ErrNotExist
}
