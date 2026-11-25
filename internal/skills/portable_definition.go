package skills

import (
	"errors"
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
