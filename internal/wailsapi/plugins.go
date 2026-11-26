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
	"assistente/internal/userctx"
	"github.com/google/uuid"
)

type PluginPreview struct {
	Repository         bool                `json:"repository,omitempty"`
	Authentication     string              `json:"authentication,omitempty"`
	EditedMCPServers   []string            `json:"editedMcpServers,omitempty"`
	AffectedMCPServers []string            `json:"affectedMcpServers,omitempty"`
	Components         []plugins.Component `json:"components"`
	Token              string              `json:"token"`
	Name               string              `json:"name"`
	Version            string              `json:"version"`
	Origin             string              `json:"origin"`
	Digest             string              `json:"digest"`
	PreviousDigest     string              `json:"previousDigest"`
	Files              []string            `json:"files"`
	Diagnostics        []string            `json:"diagnostics"`
}

type pluginSelection struct {
	inspect  func(context.Context) (*pluginpackage.LocalSnapshot, error)
	affected []plugins.BoundMCPServer
	preview  PluginPreview
	owner    string
	validate commandsecurity.Admission
}

type Plugins struct {
	chooseArchive     func(ports.OpenFileOptions) (string, error)
	savePackage       func(ports.SaveFileOptions) (string, error)
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

func (api *Plugins) beginPreview() uint64 {
	api.mu.Lock()
	api.previewGeneration++
	defer api.mu.Unlock()
	return api.previewGeneration
}

func (api *Plugins) PreviewLocal() (*PluginPreview, error) {
	return api.previewLocal(false)
}

func (api *Plugins) PreviewLocalZIP() (*PluginPreview, error) {
	return api.previewLocal(true)
}

func (api *Plugins) previewLocal(zip bool) (*PluginPreview, error) {
	generation := api.beginPreview()
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
		inspect := pluginpackage.InspectLocal
		if zip {
			choose = api.pickArchive
			inspect = pluginpackage.InspectLocalZIP
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
		p, err := api.preparePreview(ctx, inventory, owner, generation, validate, func(context.Context) (*pluginpackage.LocalSnapshot, error) { return inspect(dir) })
		if !zip || !errors.Is(err, pluginpackage.ErrMissingManifest) {
			return p, err
		}
		indices, digest, indexErr := plugins.InspectRepositoryZIP(dir)
		if indexErr != nil {
			return nil, err
		}
		return api.rememberPreview(generation, &pluginSelection{owner: owner, validate: validate, preview: PluginPreview{Repository: true, Name: indices[0].Name, Origin: dir, Digest: digest}})

	})
}

func (api *Plugins) preparePreview(ctx context.Context, inventory *plugins.Inventory, owner string, generation uint64, validate commandsecurity.Admission, inspect func(context.Context) (*pluginpackage.LocalSnapshot, error)) (*PluginPreview, error) {
	snap, err := inspect(ctx)
	if err != nil {
		return nil, err
	}
	list, err := inventory.List(ctx)
	if err != nil {
		return nil, err
	}
	p := PluginPreview{Authentication: snap.DiscoveryAuthentication, Name: snap.Manifest.Name, Version: snap.Manifest.Version, Origin: snap.Origin, Digest: snap.Digest, Diagnostics: snap.Warnings, Components: plugins.DiscoverComponents(snap)}
	var affected []plugins.BoundMCPServer
	for _, row := range list {
		if row.Origin == p.Origin && row.Name == p.Name {
			p.PreviousDigest = row.Digest
			if row.Digest != p.Digest {
				affected, err = inventory.BoundMCPServers(ctx, row.ID, row.Digest)
				if err != nil {
					return nil, err
				}
				for _, server := range affected {
					if server.Edited {
						label := strings.TrimPrefix(server.ComponentKey, "mcp/")
						if label != server.Name {
							label += " (" + server.Name + ")"
						}
						p.EditedMCPServers = append(p.EditedMCPServers, label)
					} else {
						p.AffectedMCPServers = append(p.AffectedMCPServers, server.Name)
					}
				}
			}
			break
		}
	}
	for path := range snap.Files {
		p.Files = append(p.Files, path)
	}
	sort.Strings(p.Files)
	return api.rememberPreview(generation, &pluginSelection{inspect: inspect, affected: affected, preview: p, owner: owner, validate: validate})
}

func (api *Plugins) pickArchive() (string, error) {
	api.mu.Lock()
	picker := api.chooseArchive
	api.mu.Unlock()
	if picker == nil {
		return "", errors.New("archive picker unavailable")
	}
	return picker(ports.OpenFileOptions{Filters: []ports.FileFilter{{DisplayName: "ZIP", Pattern: "*.zip"}}})
}
func (api *Plugins) rememberPreview(generation uint64, selection *pluginSelection) (*PluginPreview, error) {
	token, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	selection.preview.Token = token.String()
	api.mu.Lock()
	defer api.mu.Unlock()
	if err := selection.validate.Check(); err != nil {
		return nil, err
	}
	if generation != api.previewGeneration {
		return nil, errPreviewSuperseded
	}
	api.selection = selection
	return &selection.preview, nil
}

// Runtime synchronization can fail after the inventory transaction has committed.
func pluginChangeCommitted(err error) bool {
	return err == nil || errors.Is(err, plugins.ErrMCPSyncPending) || errors.Is(err, plugins.ErrJobSyncPending)
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
		if selection == nil || selection.preview.Repository || selection.owner != owner || selection.preview.Token != token || strings.TrimSpace(token) == "" {
			api.mu.Unlock()
			return database.PluginInstallation{}, errors.New("plugin preview expired")
		}
		api.selection = nil
		api.mu.Unlock()
		if err := selection.validate.Check(); err != nil {
			return database.PluginInstallation{}, err
		}
		p := selection.preview
		row, err := inventory.InstallSource(ctx, p.Origin, p.Digest, p.PreviousDigest, selection.affected, selection.validate, func() (*pluginpackage.LocalSnapshot, error) { return selection.inspect(ctx) })
		if pluginChangeCommitted(err) && p.PreviousDigest != "" && p.PreviousDigest != p.Digest {
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

func (api *Plugins) ProviderRemovalPreview(id, digest string) (map[string]string, error) {
	session, inventory, _, capture, err := api.wired()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]string, error) {
		admit, err := capture(ctx)
		if err != nil {
			return nil, err
		}
		rows, err := inventory.ProviderRemovalPreview(ctx, id, digest)
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

func (api *Plugins) SetProfileActive(id, digest, key string, active bool) error {
	return api.withRepositorySession(func(ctx context.Context, inventory *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		return inventory.SetProfileActive(ctx, id, digest, key, active, admit)
	})
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

func (api *Plugins) RegisterProvider(id, digest, key string) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.RegisterProvider(ctx, id, digest, key, admit)
	})
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

func (api *Plugins) SyncPendingJobs(expectedSession string) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		if session, ok := userctx.SessionIDFromContext(ctx); !ok || session != expectedSession {
			return database.ErrUserScopeRequired
		}
		return i.SyncPendingJobs(ctx, admit)
	})
}

func (api *Plugins) SyncMCP(expectedSession string) error {
	return api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		if session, ok := userctx.SessionIDFromContext(ctx); !ok || session != expectedSession {
			return database.ErrUserScopeRequired
		}
		return i.SyncMCP(ctx, admit)
	})
}

func (api *Plugins) UninstallReviewed(id, digest, choice string, reviewed map[string]string) error {
	err := api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.UninstallReviewed(ctx, id, digest, choice, reviewed, admit)
	})
	if pluginChangeCommitted(err) || errors.Is(err, plugins.ErrProviderSyncPending) {
		api.notifySkillsChanged()
	}
	return err
}

func (api *Plugins) ChangeInstallation(id, digest, action string) error {
	err := api.mutateComponent(func(ctx context.Context, i *plugins.Inventory, admit commandsecurity.Admission) error {
		return i.ChangeInstallation(ctx, id, digest, action, admit)
	})
	if (action == "disable" || action == "uninstall") && pluginChangeCommitted(err) {
		api.notifySkillsChanged()
	}
	return err
}

func (api *Plugins) CreateListFromTemplate(id, digest, key, request string) (string, error) {
	var listID string
	err := api.mutateComponent(func(ctx context.Context, inventory *plugins.Inventory, admit commandsecurity.Admission) error {
		var err error
		listID, err = inventory.CreateListFromTemplate(ctx, id, digest, key, request, admit)
		return err
	})
	if errors.Is(err, database.ErrTaskListCapacity) {
		return "", errors.New("PLUGIN_LIST_CAPACITY")
	}
	return listID, err
}

// CopyJob creates an independent, disabled job from the saved managed definition.
func (api *Plugins) CopyJob(installationID, sourceSlug, request, name string) (string, error) {
	var slug string
	err := api.withRepositorySession(func(ctx context.Context, inventory *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		var err error
		slug, err = inventory.CopyJob(ctx, installationID, sourceSlug, request, name, admit)
		return err
	})
	return slug, err
}

func (api *Plugins) PrepareJob(id, digest, key string) (string, error) {
	var slug string
	err := api.withRepositorySession(func(ctx context.Context, inventory *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		var err error
		slug, err = inventory.PrepareJob(ctx, id, digest, key, admit)
		return err
	})
	return slug, err
}
