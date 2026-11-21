package wailsapi

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"assistente/internal/commandsecurity"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"github.com/google/uuid"
)

type PluginPreview struct {
	Authentication string              `json:"authentication,omitempty"`
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
	inspect  func(context.Context) (*pluginpackage.LocalSnapshot, error)
	preview  PluginPreview
	owner    string
	validate commandsecurity.Admission
}

type Plugins struct {
	savePackage func(ports.SaveFileOptions) (string, error)
	mu          sync.Mutex
	session     Session
	inventory   *plugins.Inventory
	choose      func() (string, error)
	capture     func(context.Context) (commandsecurity.Admission, error)
	selection   *pluginSelection
}

func NewPlugins() *Plugins { return &Plugins{} }

func AttachPlugins(api *Plugins, session Session, inventory *plugins.Inventory, choose func() (string, error), capture func(context.Context) (commandsecurity.Admission, error)) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session, api.inventory, api.choose, api.capture = session, inventory, choose, capture
	api.selection = nil
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

func (api *Plugins) PreviewLocal() (*PluginPreview, error) {
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
		dir, err := choose()
		if err != nil || dir == "" {
			return nil, err
		}
		if err := validate.Check(); err != nil {
			return nil, err
		}
		return api.preparePreview(ctx, inventory, owner, validate, func(context.Context) (*pluginpackage.LocalSnapshot, error) { return pluginpackage.InspectLocal(dir) })
	})
}

func (api *Plugins) preparePreview(ctx context.Context, inventory *plugins.Inventory, owner string, validate commandsecurity.Admission, inspect func(context.Context) (*pluginpackage.LocalSnapshot, error)) (*PluginPreview, error) {
	snap, err := inspect(ctx)
	if err != nil {
		return nil, err
	}
	list, err := inventory.List(ctx)
	if err != nil {
		return nil, err
	}
	p := PluginPreview{Authentication: snap.DiscoveryAuthentication, Name: snap.Manifest.Name, Version: snap.Manifest.Version, Origin: snap.Origin, Digest: snap.Digest, Diagnostics: snap.Warnings, Components: plugins.DiscoverComponents(snap)}
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
	api.selection = &pluginSelection{preview: p, owner: owner, validate: validate, inspect: inspect}
	return &p, nil
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
		return inventory.InstallSource(ctx, p.Origin, p.Digest, p.PreviousDigest, selection.validate, func() (*pluginpackage.LocalSnapshot, error) { return selection.inspect(ctx) })
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

func (api *Plugins) mutateComponent(apply func(context.Context, *plugins.Inventory) error) error {
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
		return struct{}{}, admit(func() error { return apply(ctx, inventory) })
	})
	return err
}

func (api *Plugins) SetProfileActive(id, digest, key string, active bool) error {
	return api.withRepositorySession(func(ctx context.Context, inventory *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		return inventory.SetProfileActive(ctx, id, digest, key, active, admit)
	})
}

func (api *Plugins) SetSkillActive(id, digest, key string, active bool) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory) error {
		return i.SetSkillActive(ctx, id, digest, key, active)
	})
}

func (api *Plugins) RegisterMCP(id, digest, key string) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory) error { return i.RegisterMCP(ctx, id, digest, key) })
}
func (api *Plugins) RemoveMCP(id, digest, key string, preserve bool) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory) error {
		return i.RemoveMCP(ctx, id, digest, key, preserve)
	})
}

func (api *Plugins) SyncMCP() error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory) error { return i.SyncMCP(ctx) })
}

func (api *Plugins) ChangeInstallation(id, digest, action string) error {
	session, inventory, _, capture, err := api.wired()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		admit, err := capture(ctx)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, inventory.ChangeInstallation(ctx, id, digest, action, admit)
	})
	return err
}

func (api *Plugins) CreateListFromTemplate(id, digest, key, request string) (string, error) {
	var listID string
	err := api.withRepositorySession(func(ctx context.Context, inventory *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		var err error
		listID, err = inventory.CreateListFromTemplate(ctx, id, digest, key, request, admit)
		return err
	})
	return listID, err
}
