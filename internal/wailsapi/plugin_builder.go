package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"assistente/internal/commandsecurity"
	"assistente/internal/core/ports"
	"assistente/internal/plugins"
)

var errBuilderDestination = errors.New("PLUGIN_BUILDER_DESTINATION")

type BuilderPreview struct {
	Contents   map[string]string   `json:"contents"`
	Name       string              `json:"name"`
	Digest     string              `json:"digest"`
	Files      []string            `json:"files"`
	Components []plugins.Component `json:"components"`
}

func AttachPluginSaveDialog(api *Plugins, dialog func(ports.SaveFileOptions) (string, error)) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.savePackage = dialog
}

func (api *Plugins) PreviewBuilder(request plugins.BuilderRequest) (*BuilderPreview, error) {
	var preview *BuilderPreview
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		_, snap, err := i.BuildPackage(ctx, request)
		if err != nil {
			return err
		}
		p := &BuilderPreview{Contents: make(map[string]string, len(snap.Files)), Name: snap.Manifest.Name, Digest: snap.Digest, Components: plugins.DiscoverComponents(snap)}
		for file, contents := range snap.Files {
			p.Contents[file] = string(contents)
			p.Files = append(p.Files, file)
		}
		sort.Strings(p.Files)
		if err := admit.Check(); err != nil {
			return err
		}
		preview = p
		return nil
	})
	return preview, err
}

func (api *Plugins) SaveBuilder(request plugins.BuilderRequest, expectedDigest string) (bool, error) {
	saved := false
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		data, snap, err := i.BuildPackage(ctx, request)
		if err != nil {
			return err
		}
		if expectedDigest == "" || snap.Digest != expectedDigest {
			return errors.Join(plugins.ErrPackageChanged, errors.New("PLUGIN_BUILDER_CHANGED"))
		}
		api.mu.Lock()
		saveDialog := api.savePackage
		api.mu.Unlock()
		if saveDialog == nil {
			return errors.New("save dialog unavailable")
		}
		if err := admit.Check(); err != nil {
			return err
		}
		file, err := saveDialog(ports.SaveFileOptions{DefaultFilename: snap.Manifest.Name + ".zip", Filters: []ports.FileFilter{{DisplayName: "ZIP", Pattern: "*.zip"}}})
		if err != nil {
			return errBuilderDestination
		}
		if file == "" {
			return nil
		}
		if err := admit.Check(); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(file), ".assistente-plugin-*.tmp")
		if err != nil {
			return errBuilderDestination
		}
		defer func() { _ = os.Remove(tmp.Name()) }()
		_, err = tmp.Write(data)
		if err == nil {
			err = tmp.Sync()
		}
		closeErr := tmp.Close()
		if err != nil || closeErr != nil {
			return errBuilderDestination
		}
		err = admit(nil, func() error {
			if err := os.Rename(tmp.Name(), file); err != nil {
				return errBuilderDestination
			}
			return nil
		})
		saved = err == nil
		return err
	})
	return saved, err
}

func (api *Plugins) BuilderSkillNames() (*plugins.BuilderResourceCatalog, error) {
	return api.builderCatalog((*plugins.Inventory).BuilderSkillNames)
}

func (api *Plugins) BuilderProfileNames() (*plugins.BuilderResourceCatalog, error) {
	return api.builderCatalog((*plugins.Inventory).BuilderProfileNames)
}

func (api *Plugins) builderCatalog(list func(*plugins.Inventory) plugins.BuilderResourceCatalog) (*plugins.BuilderResourceCatalog, error) {
	var catalog plugins.BuilderResourceCatalog
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		catalog = list(i)
		return admit.Check()
	})
	if err != nil {
		return nil, err
	}
	return &catalog, nil
}
func (api *Plugins) BuilderJobNames() ([]string, error) {
	var names []string
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		var err error
		names, err = i.BuilderJobNames(ctx)
		if err != nil {
			return err
		}
		return admit.Check()
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

func (api *Plugins) BuilderProviderSlugs() ([]string, error) {
	var names []string
	err := api.withRepositorySession(func(ctx context.Context, i *plugins.Inventory, _ func() (string, error), admit commandsecurity.Admission) error {
		var err error
		names, err = i.BuilderProviderSlugs(ctx)
		if err != nil {
			return err
		}
		return admit.Check()
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}
