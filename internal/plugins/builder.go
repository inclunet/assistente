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
	Profiles  []string          `json:"profiles,omitempty"`
	Skills    []string          `json:"skills,omitempty"`
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

func (i *Inventory) WithProfileDefinition(read func(string) ([]byte, error), names func() ([]string, error)) *Inventory {
	copy := *i
	copy.profileDefinition = read
	copy.profileNames = names
	return &copy
}

func (i *Inventory) WithSkillDefinition(read func(string) ([]byte, error), names func() []string) *Inventory {
	copy := *i
	copy.skillDefinition = read
	copy.skillNames = names
	return &copy
}

func (i *Inventory) BuilderSkillNames() []string {
	if i.skillNames == nil {
		return nil
	}
	return i.skillNames()
}

func (i *Inventory) BuildPackage(ctx context.Context, request BuilderRequest) ([]byte, *pluginpackage.LocalSnapshot, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, nil, err
	}
	count := len(request.TaskLists) + len(request.Skills) + len(request.Profiles)
	if count == 0 || count > 100 || strings.TrimSpace(request.Name) == "" || reservedDevice(request.Name) {
		return nil, nil, errors.New("PLUGIN_BUILDER_SELECTION")
	}
	files := map[string][]byte{}
	extension := nativeResources{Version: 1}
	total := 0
	for _, key := range request.Profiles {
		if !portableSkillName.MatchString(key) || len(key) > 64 || reservedDevice(key) || i.profileDefinition == nil {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		file := path.Join(nativeExtension, "profiles", key+".json")
		if _, exists := files[file]; exists {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		raw, err := i.profileDefinition(key)
		if err != nil {
			return nil, nil, err
		}
		total += len(raw)
		if total > 31<<20 {
			return nil, nil, errors.New("PLUGIN_BUILDER_SIZE")
		}
		files[file] = raw
		extension.Profiles = append(extension.Profiles, key)
	}
	sort.Strings(extension.Profiles)
	for _, key := range request.Skills {
		if !portableSkillName.MatchString(key) || len(key) > 64 || reservedDevice(key) || i.skillDefinition == nil {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		file := path.Join("skills", key, "SKILL.md")
		if _, exists := files[file]; exists {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		raw, err := i.skillDefinition(key)
		if err != nil {
			return nil, nil, err
		}
		total += len(raw)
		if total > 31<<20 {
			return nil, nil, errors.New("PLUGIN_BUILDER_SIZE")
		}
		files[file] = raw
	}
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
	manifest := map[string]any{"$schema": pluginpackage.ManifestSchema, "name": request.Name}
	if len(extension.TaskLists)+len(extension.Profiles) > 0 {
		manifest["extensions"] = map[string]nativeResources{nativeExtension: extension}
	}
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
		if (c.Kind == "skill" && c.Reason != "") || c.Kind == "invalidSkill" {
			return nil, nil, errors.New("PLUGIN_BUILDER_SKILL_UNAVAILABLE")
		}
		if c.Reason != "" || (c.Kind != "tasklistTemplate" && c.Kind != "skill" && c.Kind != "profile") {
			return nil, nil, ErrComponentUnavailable
		}
	}
	return output.Bytes(), snapshot, nil
}

func (i *Inventory) BuilderProfileNames() ([]string, error) {
	if i.profileNames == nil {
		return nil, nil
	}
	names, err := i.profileNames()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(names))
	for _, key := range names {
		if portableSkillName.MatchString(key) && len(key) <= 64 && !reservedDevice(key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
