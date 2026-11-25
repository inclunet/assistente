package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
	"assistente/internal/portability"
	"assistente/internal/profiles"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const nativeExtension = "br.com.inclunet.assistente"

type nativeResources struct {
	Version   int      `json:"version"`
	Jobs      []string `json:"jobs,omitempty"`
	Profiles  []string `json:"profiles,omitempty"`
	TaskLists []string `json:"tasklists,omitempty"`
}

func nativeComponents(snap *pluginpackage.LocalSnapshot) []Component {
	raw, ok := snap.Manifest.Extensions[nativeExtension]
	if !ok {
		return nil
	}
	var def nativeResources
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&def) != nil || def.Version != 1 || len(def.TaskLists)+len(def.Profiles)+len(def.Jobs) > 2048 {
		return []Component{{Kind: "invalidNative", Key: nativeExtension, Name: nativeExtension, Reason: "manifestConfiguration"}}
	}
	var result []Component
	seen := map[string]bool{}
	for _, name := range def.TaskLists {
		if !portableSkillName.MatchString(name) || len(name) > 64 || seen[name] {
			return []Component{{Kind: "invalidNative", Key: nativeExtension, Name: nativeExtension, Reason: "manifestConfiguration"}}
		}
		seen[name] = true
		entry := path.Join(nativeExtension, "tasklists", name+".json")
		c := Component{Key: "tasklists/" + name, Kind: "tasklistTemplate", Name: name, Entry: entry}
		raw, found := snap.Files[entry]
		definition, err := portability.ReadTaskListTemplate(raw)
		if !found {
			c.Reason = "missingTemplate"
		} else if err != nil {
			c.Reason = "invalidTemplate"
		} else {
			c.Name = definition.Title
		}
		result = append(result, c)
	}
	for _, name := range def.Profiles {
		if !portableSkillName.MatchString(name) || len(name) > 64 || seen["profiles/"+name] {
			return []Component{{Kind: "invalidNative", Key: nativeExtension, Name: nativeExtension, Reason: "manifestConfiguration"}}
		}
		key := "profiles/" + name
		seen[key] = true
		entry := path.Join(nativeExtension, key+".json")
		c := Component{Key: key, Kind: "profile", Name: name, Entry: entry}
		definition, err := profiles.ReadPortableDefinition(snap.Files[entry])
		if err != nil {
			c.Reason = "invalidProfile"
		} else {
			c.Name = definition.Name
		}
		result = append(result, c)
	}
	for _, name := range def.Jobs {
		key := "jobs/" + name
		if !portableSkillName.MatchString(name) || len(name) > 64 || seen[key] {
			return []Component{{Kind: "invalidNative", Key: nativeExtension, Name: nativeExtension, Reason: "manifestConfiguration"}}
		}
		seen[key] = true
		entry := path.Join(nativeExtension, key+".json")
		c := Component{Kind: "job", Key: key, Name: name, Entry: entry}
		def, err := jobs.ReadPortableJob(name, snap.Files[entry])
		if err != nil {
			c.Reason = "invalidJob"
		} else {
			c.Name = def.Name
			c.Tool = def.Tool
			c.JobRequiredInputs = def.RequiredInputs
			for _, trigger := range def.Triggers {
				c.JobTriggers = append(c.JobTriggers, string(trigger.Type))
			}
		}
		result = append(result, c)
	}
	return result
}

func (i *Inventory) WithListCreated(emit func(context.Context, *database.TaskList, commandsecurity.Admission)) *Inventory {
	copy := *i
	copy.listCreated = emit
	return &copy
}

func (i *Inventory) CreateListFromTemplate(ctx context.Context, id, digest, key, request string, admit commandsecurity.Admission) (string, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	requestID, err := uuid.Parse(request)
	if err != nil || admit == nil {
		return "", ErrComponentUnavailable
	}
	snap, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return "", err
	}
	var definition *portability.TaskListTemplate
	for _, c := range DiscoverComponents(snap) {
		if c.Key == key && c.Kind == "tasklistTemplate" && c.Reason == "" {
			definition, err = portability.ReadTaskListTemplate(snap.Files[c.Entry])
			if err != nil {
				return "", err
			}
			break
		}
	}
	if definition == nil {
		return "", ErrComponentUnavailable
	}
	var listID string
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var created *database.TaskList
	err = database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.tasklist.create", func(tx *gorm.DB) error {
		created = nil
		var installed database.PluginInstallation
		if err := tx.Where("id=? AND user_id=? AND digest=? AND disabled=?", id, owner, digest, false).Take(&installed).Error; err != nil {
			return err
		}
		receipt := database.PluginListReceipt{InstallationID: id, Digest: digest, ComponentKey: key, RequestID: requestID.String()}
		err := tx.Where(&receipt).Take(&receipt).Error
		if err == nil {
			listID = receipt.ListID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := database.CheckTaskListCapacity(ctx, tx); err != nil {
			return err
		}
		fresh, err := uuid.NewV7()
		if err != nil {
			return err
		}
		listID = fresh.String()
		receipt.ListID = listID
		if err := tx.Create(&receipt).Error; err != nil {
			return err
		}
		if err := definition.Instantiate(ctx, tx, listID); err != nil {
			return err
		}
		var list database.TaskList
		if err := tx.Preload("Workflow").Where("id=? AND user_id=?", listID, owner).Take(&list).Error; err != nil {
			return err
		}
		list.TaskCount = int64(len(definition.Tasks))
		created = &list
		return nil
	}, admit)
	if err != nil {
		return "", err
	}
	if created != nil && i.listCreated != nil {
		i.listCreated(ctx, created, admit)
	}
	return listID, nil
}
