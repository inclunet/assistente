package plugins

import (
	"archive/zip"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (i *Inventory) WithProfileDefinitions(discover func() (profiles.PortableDefinitions, error)) *Inventory {
	copy := *i
	copy.profileDefinitions = discover
	return &copy
}

func (i *Inventory) WithSkillDefinitions(discover func() (skills.PortableDefinitions, error)) *Inventory {
	copy := *i
	copy.skillDefinitions = discover
	return &copy
}

type BuilderResourceCatalog struct {
	Names   []string `json:"names"`
	Partial bool     `json:"partial"`
}

func (i *Inventory) BuilderSkillNames() BuilderResourceCatalog {
	catalog := BuilderResourceCatalog{Names: []string{}}
	if i.skillDefinitions != nil {
		definitions, err := i.skillDefinitions()
		catalog.Partial = err != nil
		for key := range definitions {
			if validBuilderSkill(key) {
				catalog.Names = append(catalog.Names, key)
			}
		}
		sort.Strings(catalog.Names)
	}
	return catalog
}

func validBuilderSkill(key string) bool {
	return portableSkillName.MatchString(key) && len(key) <= 64 && !reservedDevice(key)
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
	var profileFiles profiles.PortableDefinitions
	if len(request.Profiles) > 0 && i.profileDefinitions != nil {
		profileFiles, _ = i.profileDefinitions()
	}
	for _, key := range request.Profiles {
		if !portableSkillName.MatchString(key) || len(key) > 64 || reservedDevice(key) || profileFiles[key] == nil {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		file := path.Join(nativeExtension, "profiles", key+".json")
		if _, exists := files[file]; exists {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		raw, err := profileFiles[key]()
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
	var definitions skills.PortableDefinitions
	if len(request.Skills) > 0 && i.skillDefinitions != nil {
		// An unrelated root can fail while the selected names are available.
		definitions, _ = i.skillDefinitions()
	}
	for _, key := range request.Skills {
		if !validBuilderSkill(key) {
			return nil, nil, errors.New("PLUGIN_BUILDER_SKILL_UNAVAILABLE")
		}
		file := path.Join("skills", key, "SKILL.md")
		if _, exists := files[file]; exists {
			return nil, nil, errors.New("PLUGIN_BUILDER_KEY")
		}
		read := definitions[key]
		if read == nil {
			return nil, nil, fmt.Errorf("PLUGIN_BUILDER_SKILL_UNAVAILABLE:%s", key)
		}
		raw, err := read()
		if err != nil {
			return nil, nil, fmt.Errorf("PLUGIN_BUILDER_SKILL_UNAVAILABLE:%s", key)
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
			return nil, nil, fmt.Errorf("PLUGIN_BUILDER_SKILL_UNAVAILABLE:%s", strings.TrimPrefix(c.Key, "skills/"))
		}
		if c.Reason != "" || (c.Kind != "tasklistTemplate" && c.Kind != "skill" && c.Kind != "profile") {
			return nil, nil, ErrComponentUnavailable
		}
	}
	return output.Bytes(), snapshot, nil
}

func (i *Inventory) BuilderProfileNames() BuilderResourceCatalog {
	catalog := BuilderResourceCatalog{Names: []string{}}
	if i.profileDefinitions != nil {
		files, err := i.profileDefinitions()
		catalog.Partial = err != nil
		for key := range files {
			if validBuilderSkill(key) {
				catalog.Names = append(catalog.Names, key)
			}
		}
		sort.Strings(catalog.Names)
	}
	return catalog
}
