package wailsapi

import (
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/plugincatalog"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"context"
)

// withRepositorySession binds picker and store operations to the user who
// opened the manager, including the native dialog's return after logout.
func (api *Plugins) withRepositorySession(operation func(context.Context, *plugins.Inventory, func() (string, error), commandsecurity.Admission) error) error {
	session, inventory, choose, capture, err := api.wired()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		admission, err := capture(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if admission == nil {
			return struct{}{}, database.ErrUserScopeRequired
		}
		return struct{}{}, operation(ctx, inventory, choose, admission)
	})
	return err
}
func (api *Plugins) ListRepositories() ([]plugins.Repository, error) {
	var rows []plugins.Repository
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		var err error
		rows, err = i.Repositories(ctx)
		if err != nil {
			return err
		}
		return admit.Check()
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}
func (api *Plugins) AddRemoteRepository(origin string, authenticated bool) error {
	return api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		return i.SaveRemoteRepository(ctx, origin, authenticated, admit)
	})
}

func (api *Plugins) AddGitRepository(origin string, authenticated bool) error {
	return api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		return i.SaveGitRepository(ctx, origin, authenticated, admit)
	})
}

func (api *Plugins) AddLocalRepository() (bool, error) {
	var selected bool
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, choose func() (string, error), admit commandsecurity.Admission) error {
		dir, err := choose()
		if err != nil || dir == "" {
			return err
		}
		if err := admit.Check(); err != nil {
			return err
		}
		selected = true
		return i.SaveRepository(ctx, dir, "", "", admit)
	})
	return selected && err == nil, err
}
func (api *Plugins) AddIndexRepository() (bool, error) {
	selected := false
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		file, err := api.pickFile("marketplace.json", "marketplace.json")
		if err != nil || file == "" {
			return err
		}
		if err = admit.Check(); err != nil {
			return err
		}
		root, err := plugincatalog.IndexDirectory(file)
		if err != nil {
			return err
		}
		selected = true
		return i.SaveRepository(ctx, root, "", "", admit)
	})
	return selected && err == nil, err
}
func (api *Plugins) AddArchiveRepository() (bool, error) {
	selected := false
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		file, err := api.pickArchive()
		if err != nil || file == "" {
			return err
		}
		selected = true
		return i.AddRepositoryZIP(ctx, file, "", admit)
	})
	return selected && err == nil, err
}

func (api *Plugins) AddReviewedRepository(token string) error {
	return api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), _ commandsecurity.Admission) error {
		owner, err := database.RequireUserID(ctx)
		if err != nil {
			return err
		}
		api.mu.Lock()
		selected := api.selection
		if selected == nil || !selected.preview.Repository || selected.preview.Token != token || selected.owner != owner {
			api.mu.Unlock()
			return errPreviewSuperseded
		}
		api.selection = nil
		api.mu.Unlock()
		return i.AddRepositoryZIP(ctx, selected.preview.Origin, selected.preview.Digest, selected.validate)
	})
}
func (api *Plugins) RefreshRepository(id, revision string) error {
	return api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		return i.SaveRepository(ctx, "", id, revision, admit)
	})
}
func (api *Plugins) RemoveRepository(id, revision string) error {
	return api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		return i.RemoveRepository(ctx, id, revision, admit)
	})
}

func (api *Plugins) PreviewRepositoryEntry(id, revision, format, name string) (*PluginPreview, error) {
	generation := api.beginPreview()
	var preview *PluginPreview
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		owner, err := database.RequireUserID(ctx)
		if err != nil {
			return err
		}
		preview, err = api.preparePreview(ctx, i, owner, generation, admit, func(active context.Context) (*pluginpackage.LocalSnapshot, error) {
			return i.InspectRepositoryEntry(active, id, revision, plugincatalog.Format(format), name)
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return preview, nil
}
