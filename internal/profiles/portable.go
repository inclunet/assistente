package profiles

import (
	"assistente/internal/pluginpackage"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"

	"os"
	"path/filepath"

	"strings"
	"unicode/utf8"
)

// PortableDefinition carries conversational preferences across accounts.
// Provider credentials, command grants and local resource IDs have no representation here.
type PortableDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

func ReadPortableDefinition(raw []byte) (*Profile, error) {
	if len(raw) > 64<<10 || !utf8.Valid(raw) {
		return nil, errors.New("invalid portable profile encoding or size")
	}
	var def PortableDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(def.Name) == "" {
		return nil, errors.New("invalid portable profile")
	}
	p := DefaultProfile()
	p.Name = def.Name
	p.Description = def.Description
	p.Chat.Temperature = def.Temperature
	p.Chat.MaxTokens = def.MaxTokens
	p.Chat.TopP = def.TopP
	p.Chat.DisableTools = true
	p.Chat.DisableSkills = true
	p.Chat.ToolPolicy = nil
	p.Chat.ToolPolicyDefault = "disabled"
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
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
	defer root.Close()
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
	def := PortableDefinition{Name: p.Name, Description: p.Description, Temperature: p.Chat.Temperature, MaxTokens: p.Chat.MaxTokens, TopP: p.Chat.TopP}
	raw, err := json.MarshalIndent(def, "", "  ")
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
