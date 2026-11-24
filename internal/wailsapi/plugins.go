package wailsapi

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"github.com/google/uuid"
)

type PluginPreview struct {
	Components     []plugins.Component `json:"components"`
	Token          string              `json:"token"`
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	Origin         string              `json:"origin"`
	Digest         string              `json:"digest"`
	PreviousDigest string              `json:"previousDigest"`
	Files          []string            `json:"files"`
	Diagnostics    []string            `json:"diagnostics"`
}

type pluginSelection struct {
	preview  PluginPreview
	owner    string
	validate commandsecurity.Admission
}

type Plugins struct {
	mu                sync.Mutex
	session           Session
	inventory         *plugins.Inventory
	choose            func() (string, error)
	capture           func(context.Context) (commandsecurity.Admission, error)
	selection         *pluginSelection
	previewGeneration uint64
	skillsChanged     func()
}

func NewPlugins() *Plugins { return &Plugins{} }

func AttachPlugins(api *Plugins, session Session, inventory *plugins.Inventory, choose func() (string, error), capture func(context.Context) (commandsecurity.Admission, error), skillsChanged func()) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session, api.inventory, api.choose, api.capture = session, inventory, choose, capture
	api.skillsChanged = skillsChanged
	api.selection = nil
	api.previewGeneration++
}

func (api *Plugins) notifySkillsChanged() {
	api.mu.Lock()
	notify := api.skillsChanged
	api.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func (api *Plugins) wired() (Session, *plugins.Inventory, func() (string, error), func(context.Context) (commandsecurity.Admission, error), error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.session == nil || api.inventory == nil || api.capture == nil || api.choose == nil {
		return nil, nil, nil, nil, errors.New("plugins unavailable")
	}
	return api.session, api.inventory, api.choose, api.capture, nil
}

func (api *Plugins) ListInstalled() ([]database.PluginInstallation, error) {
	session, inventory, _, capture, err := api.wired()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.PluginInstallation, error) {
		validate, err := capture(ctx)
		if err == nil && validate == nil {
			return nil, errors.New("plugin session unavailable")
		}
		if err != nil {
			return nil, err
		}
		rows, err := inventory.List(ctx)
		if err != nil {
			return nil, err
		}
		if err := validate.Check(); err != nil {
			return nil, err
		}
		return rows, nil
	})
}

var errPreviewSuperseded = errors.New("plugin preview superseded")

func (api *Plugins) previewIsCurrent(generation uint64) bool {
	api.mu.Lock()
	defer api.mu.Unlock()
	return api.previewGeneration == generation
}

func (api *Plugins) PreviewLocal() (*PluginPreview, error) {
	api.mu.Lock()
	api.previewGeneration++
	generation := api.previewGeneration
	api.mu.Unlock()
	session, inventory, choose, capture, err := api.wired()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*PluginPreview, error) {
		validate, err := capture(ctx)
		if err == nil && validate == nil {
			return nil, errors.New("plugin session unavailable")
		}
		if err != nil {
			return nil, err
		}
		owner, err := database.RequireUserID(ctx)
		if err != nil {
			return nil, err
		}
		if !api.previewIsCurrent(generation) {
			return nil, errPreviewSuperseded
		}
		dir, err := choose()
		if !api.previewIsCurrent(generation) {
			return nil, errPreviewSuperseded
		}
		if err != nil || dir == "" {
			return nil, err
		}
		if err := validate.Check(); err != nil {
			return nil, err
		}
		snap, err := pluginpackage.InspectLocal(dir)
		if err != nil {
			return nil, err
		}
		list, err := inventory.List(ctx)
		if err != nil {
			return nil, err
		}
		p := PluginPreview{Name: snap.Manifest.Name, Version: snap.Manifest.Version, Origin: snap.Origin, Digest: snap.Digest, Diagnostics: snap.Warnings, Components: plugins.DiscoverComponents(snap)}
		for _, row := range list {
			if row.Origin == p.Origin && row.Name == p.Name {
				p.PreviousDigest = row.Digest
				break
			}
		}
		for path := range snap.Files {
			p.Files = append(p.Files, path)
		}
		sort.Strings(p.Files)
		token, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		p.Token = token.String()
		api.mu.Lock()
		defer api.mu.Unlock()
		if err := validate.Check(); err != nil {
			return nil, err
		}
		if generation != api.previewGeneration {
			return nil, errPreviewSuperseded
		}
		api.selection = &pluginSelection{preview: p, owner: owner, validate: validate}
		return &p, nil
	})
}

func (api *Plugins) InstallReviewed(token string) (database.PluginInstallation, error) {
	session, inventory, _, _, err := api.wired()
	if err != nil {
		return database.PluginInstallation{}, err
	}
	return WithUser(session, func(ctx context.Context) (database.PluginInstallation, error) {
		owner, err := database.RequireUserID(ctx)
		if err != nil {
			return database.PluginInstallation{}, err
		}
		api.mu.Lock()
		selection := api.selection
		if selection == nil || selection.owner != owner || selection.preview.Token != token || strings.TrimSpace(token) == "" {
			api.mu.Unlock()
			return database.PluginInstallation{}, errors.New("plugin preview expired")
		}
		api.selection = nil
		api.mu.Unlock()
		if err := selection.validate.Check(); err != nil {
			return database.PluginInstallation{}, err
		}
		p := selection.preview
		row, err := inventory.InstallReviewed(ctx, p.Origin, p.Digest, p.PreviousDigest, selection.validate)
		if (err == nil || errors.Is(err, plugins.ErrMCPSyncPending)) && p.PreviousDigest != "" && p.PreviousDigest != p.Digest {
			api.notifySkillsChanged()
		}
		return row, err
	})
}

// DiscardPreview revokes only the selection owned by the current account.
func (api *Plugins) DiscardPreview(token string) error {
	session, _, _, _, err := api.wired()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		owner, err := database.RequireUserID(ctx)
		if err != nil {
			return struct{}{}, err
		}
		api.mu.Lock()
		defer api.mu.Unlock()
		if api.selection != nil && api.selection.owner == owner && api.selection.preview.Token == token {
			api.selection = nil
		}
		return struct{}{}, nil
	})
	return err
}

func (api *Plugins) InstalledComponents(id, digest string) ([]plugins.Component, error) {
	session, inventory, _, capture, err := api.wired()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]plugins.Component, error) {
		admit, err := capture(ctx)
		if err != nil {
			return nil, err
		}
		rows, err := inventory.InstalledComponents(ctx, id, digest)
		if err != nil {
			return nil, err
		}
		if err := admit.Check(); err != nil {
			return nil, err
		}
		return rows, nil
	})
}

func (api *Plugins) mutateComponent(apply func(context.Context, *plugins.Inventory, commandsecurity.Admission) error) error {
	session, inventory, _, capture, err := api.wired()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		admit, err := capture(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if admit == nil {
			return struct{}{}, database.ErrUserScopeRequired
		}
		return struct{}{}, apply(ctx, inventory, admit)
	})
	return err
}

func (api *Plugins) SetSkillActive(id, digest, key string, active bool) error {
	err := api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.SetSkillActive(ctx, id, digest, key, active, admit)
	})
	if err == nil {
		api.notifySkillsChanged()
	}
	return err
}

func (api *Plugins) RegisterMCP(id, digest, key string) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.RegisterMCP(ctx, id, digest, key, admit)
	})
}
func (api *Plugins) RemoveMCP(id, digest, key string, preserve bool) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.RemoveMCP(ctx, id, digest, key, preserve, admit)
	})
}

func (api *Plugins) SyncMCP() error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.SyncMCP(ctx, admit)
	})
}
