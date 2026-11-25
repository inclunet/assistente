package profiles

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const PluginPrefix = "plugin--"

func IsPluginProfile(slug string) bool {
	suffix, ok := strings.CutPrefix(slug, PluginPrefix)
	if !ok {
		return false
	}
	id, err := uuid.Parse(suffix)
	return err == nil && id.Version() == 4 && id.Variant() == uuid.RFC4122 && id.String() == suffix
}

type PluginSource interface {
	GetPluginProfile(context.Context, string) (*Profile, error)
	ListPluginProfiles(context.Context) ([]ProfileInfo, error)
}

func (m *Manager) SetPluginSource(source PluginSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pluginSource = source
}

func (m *Manager) source() PluginSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pluginSource
}

func (m *Manager) GetForContext(ctx context.Context, slug string) (*Profile, error) {
	if !IsPluginProfile(slug) {
		return m.Get(slug)
	}
	source := m.source()
	if source == nil {
		return nil, errors.New("plugin profile source unavailable")
	}
	return source.GetPluginProfile(ctx, slug)
}

func (m *Manager) ListForContext(ctx context.Context) ([]ProfileInfo, error) {
	local, err := m.List()
	if err != nil {
		return nil, err
	}
	source := m.source()
	if source == nil {
		return local, nil
	}
	installed, err := source.ListPluginProfiles(ctx)
	if err != nil {
		return nil, err
	}
	items := append(local, installed...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Slug < items[j].Slug
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}
