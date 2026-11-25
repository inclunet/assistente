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
func (m *Manager) BuilderSlugs() ([]string, error) {
	discovered, err := m.discoverNative()
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(discovered))
	for _, item := range discovered {
		slugs = append(slugs, item.slug)
	}
	sort.Strings(slugs)
	return slugs, nil
}

// PortableDefinition reads a selected native skill from its configured root.
// A definition with additional files cannot be published as a single SKILL.md.
func (m *Manager) PortableDefinition(slug string) ([]byte, error) {
	if !filepath.IsLocal(slug) || filepath.Base(slug) != slug || slug == "." {
		return nil, os.ErrInvalid
	}
	paths := m.resolver.GetSearchPaths()
	for n := len(paths) - 1; n >= 0; n-- {
		info, err := os.Lstat(filepath.Join(paths[n], slug))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(paths[n], slug, skillFile)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		base, err := os.OpenRoot(paths[n])
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
