package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"

	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/portability"
	"gorm.io/gorm"
)

type BuilderTaskList struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	IncludeTasks bool   `json:"includeTasks"`
}

type BuilderRequest struct {
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	TaskLists []BuilderTaskList `json:"tasklists"`
}

// Windows reserves device names even when they are followed by an extension.
func reservedDevice(name string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (base[:3] == "COM" || base[:3] == "LPT") && base[3] >= '1' && base[3] <= '9'
}

func (i *Inventory) BuildPackage(ctx context.Context, request BuilderRequest) ([]byte, *pluginpackage.LocalSnapshot, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, nil, err
	}
	if len(request.TaskLists) == 0 || len(request.TaskLists) > 100 || strings.TrimSpace(request.Name) == "" || reservedDevice(request.Name) {
		return nil, nil, errors.New("PLUGIN_BUILDER_SELECTION")
	}
	files := map[string][]byte{}
	extension := nativeResources{Version: 1}
	total := 0
	err := i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range request.TaskLists {
			if !portableSkillName.MatchString(item.Key) || len(item.Key) > 64 || reservedDevice(item.Key) {
				return errors.New("PLUGIN_BUILDER_KEY")
			}
			file := path.Join(nativeExtension, "tasklists", item.Key+".json")
			if _, dup := files[file]; dup {
				return errors.New("PLUGIN_BUILDER_KEY")
			}
			template, err := portability.ExportTaskListTemplate(ctx, tx, item.ID, item.IncludeTasks)
			if err != nil {
				return err
			}
			raw, err := json.MarshalIndent(template, "", "  ")
			if err != nil {
				return err
			}
			if _, err = portability.ReadTaskListTemplate(raw); err != nil {
				return err
			}
			total += len(raw)
			if total > 31<<20 {
				return errors.New("PLUGIN_BUILDER_SIZE")
			}
			files[file] = raw
			extension.TaskLists = append(extension.TaskLists, item.Key)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(extension.TaskLists)
	manifest := map[string]any{"$schema": pluginpackage.ManifestSchema, "name": request.Name, "extensions": map[string]nativeResources{nativeExtension: extension}}
	if request.Version != "" {
		manifest["version"] = request.Version
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	files["plugin.json"] = raw
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, nil, err
		}
		if _, err := entry.Write(files[name]); err != nil {
			return nil, nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}
	archive, err := pluginpackage.OpenArchive(output.Bytes())
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := archive.Package(request.Name, ".")
	if err != nil {
		return nil, nil, err
	}
	for _, c := range DiscoverComponents(snapshot) {
		if c.Reason != "" {
			return nil, nil, ErrComponentUnavailable
		}
	}
	return output.Bytes(), snapshot, nil
}
