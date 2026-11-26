package profiles

import (
	"assistente/internal/pluginpackage"

	"encoding/json"
	"errors"

	"io/fs"

	"os"
	"path/filepath"

	"strings"
	"unicode/utf8"
)

// ReadPortableDefinition lê o mesmo modelo da configuração local. A camada
// de instalação resolve cada referência explícita na conta de destino.
func ReadPortableDefinition(raw []byte) (*Profile, error) {
	if len(raw) > 1<<20 || !utf8.Valid(raw) {
		return nil, errors.New("invalid portable profile encoding or size")
	}
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.Active || p.BuiltinVersion != "" || strings.TrimSpace(p.Name) == "" {
		return nil, errors.New("invalid portable profile")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// MapProviderReferences preserva papéis opcionais e a seleção explícita do padrão.
func (p *Profile) MapProviderReferences(resolve func(string) (string, error)) error {
	for _, ref := range []*string{&p.Chat.LLMProvider, &p.Voice.Assistant.LLMProviderID, &p.Voice.User.LLMProviderID, &p.Voice.System.LLMProviderID, &p.Input.LLMProviderID} {
		if *ref == "" || *ref == DefaultProviderSentinel {
			continue
		}
		value, err := resolve(*ref)
		if err != nil {
			return err
		}
		*ref = value
	}
	return nil
}

// PortableDefinitions indexes readable native profiles by their exact, portable filename.
type PortableDefinitions map[string]func() ([]byte, error)

func (m *Manager) readPortableFile(dir, filename string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := m.ensurePortableSnapshotLocked(); err != nil {
		return nil, err
	}
	root, err := openProfileDirectory(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	raw, err := pluginpackage.ReadFile(root, filename, 1<<20)
	if err != nil {
		return nil, err
	}
	return encodePortableProfile(raw)
}

// The native search path ends in .assistente/profiles. Open both directories
// relative to the actual workspace, home or executable directory.
func openProfileDirectory(dir string) (*os.Root, error) {
	configDir := filepath.Dir(dir)
	parent, err := os.OpenRoot(filepath.Dir(configDir))
	if err != nil {
		return nil, err
	}
	defer func() { _ = parent.Close() }()
	config, err := openConfigChild(parent, filepath.Base(configDir))
	if err != nil {
		return nil, err
	}
	defer func() { _ = config.Close() }()
	return openConfigChild(config, filepath.Base(dir))
}

func openConfigChild(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fs.ErrInvalid
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = child.Close()
		return nil, fs.ErrInvalid
	}
	return child, nil
}

func encodePortableProfile(data []byte) ([]byte, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("invalid local profile encoding")
	}
	var p Profile
	err := json.Unmarshal(data, &p)
	if err != nil {
		return nil, err
	}
	normalizeRoutingFields(&p)
	p.Active = false
	p.BuiltinVersion = ""
	raw, err := json.Marshal(&p)
	if err != nil {
		return nil, err
	}
	if _, err = ReadPortableDefinition(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// DiscoverPortableDefinitions resolves exact filenames in native search priority order.
func (m *Manager) DiscoverPortableDefinitions() (PortableDefinitions, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := m.ensurePortableSnapshotLocked(); err != nil {
		return nil, err
	}
	definitions := PortableDefinitions{}
	var partial error
	for _, dir := range m.resolver.GetSearchPaths() {
		root, err := openProfileDirectory(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			partial = errors.Join(partial, err)
			continue
		}
		entries, err := fs.ReadDir(root.FS(), ".")
		_ = root.Close()
		if err != nil {
			partial = errors.Join(partial, err)
			continue
		}
		for _, entry := range entries {
			name, ok := strings.CutSuffix(entry.Name(), ".json")
			if ok && entry.Type().IsRegular() && Slugify(name) == name && !IsPluginProfile(name) {
				filename := entry.Name()
				definitions[name] = func() ([]byte, error) { return m.readPortableFile(dir, filename) }
			}
		}
	}
	return definitions, partial
}

// Native profile mutations recover their journal through the manager. A builder
// preview must not serialize an intermediate definition after an interrupted write.
func (m *Manager) ensurePortableSnapshotLocked() error {
	root, err := openProfileDirectory(m.resolver.GetHomeDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	_, err = root.Lstat(".profile-mutation.journal")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("profile mutation pending recovery")
}
