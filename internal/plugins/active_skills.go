package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strconv"
	"strings"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/skills"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrComponentUnavailable = errors.New("componente indisponível nesta revisão")

func (i *Inventory) installedSnapshot(ctx context.Context, id, digest string) (*pluginpackage.LocalSnapshot, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var revision database.PluginRevision
	lookup := i.db.WithContext(ctx).Model(&database.PluginRevision{}).Joins("JOIN plugin_installations p ON p.id = plugin_revisions.installation_id AND p.digest = plugin_revisions.digest").Where("p.user_id = ? AND p.id = ? AND p.digest = ?", owner, id, digest).Take(&revision)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	result := &pluginpackage.LocalSnapshot{Digest: digest}
	if err := json.Unmarshal(revision.Files, &result.Files); err != nil {
		return nil, err
	}
	_, manifest, warnings, err := pluginpackage.CapturedManifest(result.Files)
	if err != nil {
		return nil, err
	}
	result.Manifest, result.Warnings = manifest, warnings
	return result, nil
}

func (i *Inventory) InstalledComponents(ctx context.Context, id, digest string) ([]Component, error) {
	snapshot, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return nil, err
	}
	components := DiscoverComponents(snapshot)
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var active []database.PluginSkillActivation
	err = i.db.WithContext(ctx).Model(&database.PluginSkillActivation{}).Select("plugin_skill_activations.component_key, plugin_skill_activations.slug").Joins("JOIN plugin_installations p ON p.id=plugin_skill_activations.installation_id AND p.digest=plugin_skill_activations.digest").Where("p.id=? AND p.user_id=? AND p.digest=?", id, owner, digest).Find(&active).Error
	if err != nil {
		return nil, err
	}
	slugs := map[string]string{}
	for _, row := range active {
		slugs[row.ComponentKey] = row.Slug
	}
	var agents []database.PluginAgentActivation
	if err := i.db.WithContext(ctx).Where("installation_id=? AND digest=?", id, digest).Find(&agents).Error; err != nil {
		return nil, err
	}
	for _, grant := range agents {
		slugs[grant.ComponentKey] = grant.Slug
	}
	bound, err := i.BoundMCPServers(ctx, id, digest)
	if err != nil {
		return nil, err
	}
	edited := make(map[string]string)
	for _, server := range bound {
		slugs[server.ComponentKey] = server.Slug
		if server.Edited {
			edited[server.ComponentKey] = server.Name
		}
	}
	pq, err := i.profileQuery(ctx)
	if err != nil {
		return nil, err
	}
	var granted []database.PluginProfileActivation
	if err := pq.Select("plugin_profile_activations.component_key, plugin_profile_activations.slug").Where("p.id=? AND p.digest=?", id, digest).Find(&granted).Error; err != nil {
		return nil, err
	}
	for _, row := range granted {
		slugs[row.ComponentKey] = row.Slug
	}
	var prepared []database.Job
	if err := i.db.WithContext(ctx).Select("plugin_component_key,slug").Where("user_id=? AND plugin_installation_id=? AND plugin_digest=?", owner, id, digest).Find(&prepared).Error; err != nil {
		return nil, err
	}
	for _, job := range prepared {
		slugs[job.PluginComponentKey] = job.Slug
	}
	if err := i.DescribeProfileDependencies(ctx, components, snapshot, id); err != nil {
		return nil, err
	}
	for n := range components {
		if components[n].HookScript != "" {
			var row database.PluginInstallation
			if err := i.db.WithContext(ctx).Select("hook_generation").Where("id = ? AND user_id = ? AND digest = ?", id, owner, digest).Take(&row).Error; err != nil {
				return nil, err
			}
			components[n].HookGeneration = strconv.FormatUint(row.HookGeneration, 10)
		}
		components[n].EditedServer = edited[components[n].Key]
		slug, ok := slugs[components[n].Key]
		if components[n].Kind == "provider" {
			continue
		}
		components[n].Active = ok
		components[n].Slug = slug
	}
	if err := i.DescribeExistingProviders(ctx, components, snapshot, id); err != nil {
		return nil, err
	}
	return components, nil
}

func pluginComponentSlug(id, key, prefix string) string {
	hash := sha256.Sum256([]byte(id + "\x00" + key))
	if len(prefix) > 28 {
		prefix = prefix[:28]
	}
	return skills.PluginPrefix + strings.TrimRight(prefix, "-") + "-" + hex.EncodeToString(hash[:12])
}

func (i *Inventory) SetSkillActive(ctx context.Context, id, digest, key string, active bool, admit commandsecurity.Admission) error {
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var activation database.PluginSkillActivation
	if active {
		snap, err := i.installedSnapshot(ctx, id, digest)
		if err != nil {
			return err
		}
		for _, component := range DiscoverComponents(snap) {
			if component.Key != key || component.Kind != "skill" || component.Reason != "" {
				continue
			}
			activation = database.PluginSkillActivation{Generation: uuid.NewString(), PackageFormat: snap.Manifest.Format, InstallationID: id, ComponentKey: key, Digest: digest, Slug: pluginComponentSlug(id, key, component.Name), Definition: string(snap.Files[component.Entry])}
			break
		}
		if activation.Slug == "" {
			return ErrComponentUnavailable
		}
	}
	return database.WithSQLiteAdmittedCommit(ctx, i.db, "plugins.skill.activation", func(tx *gorm.DB) error {
		var installation database.PluginInstallation
		if err := tx.Where("user_id = ? AND id = ? AND digest = ?", owner, id, digest).Take(&installation).Error; err != nil {
			return err
		}
		if active && installation.Disabled {
			return ErrInstallationDisabled
		}
		if !active {
			return tx.Where("installation_id = ? AND component_key = ?", id, key).Delete(&database.PluginSkillActivation{}).Error
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "installation_id"}, {Name: "component_key"}}, DoNothing: true}).Create(&activation).Error
	}, admit)
}

func (i *Inventory) ActiveSkills(ctx context.Context) ([]skills.Skill, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.PluginSkillActivation
	err = i.db.WithContext(ctx).Model(&database.PluginSkillActivation{}).Joins("JOIN plugin_installations p ON p.id = plugin_skill_activations.installation_id AND p.digest = plugin_skill_activations.digest").Where("p.user_id = ? AND p.disabled = ?", owner, false).Order("plugin_skill_activations.slug").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]skills.Skill, 0, len(rows))
	for _, row := range rows {
		meta, body, err := skills.Parse(row.Definition)
		if err != nil {
			return nil, fmt.Errorf("plugin skill %s: %w", row.Slug, err)
		}
		if meta.Name == "" {
			meta.Name = strings.TrimPrefix(row.ComponentKey, "skills/")
		}
		meta.DisplayName = meta.Name
		meta.Name = row.Slug
		check := func(ctx context.Context) error {
			current, err := database.RequireUserID(ctx)
			if err != nil || current != owner {
				return ErrComponentUnavailable
			}
			var count int64
			err = i.db.WithContext(ctx).Model(&database.PluginSkillActivation{}).
				Joins("JOIN plugin_installations p ON p.id = plugin_skill_activations.installation_id AND p.digest = plugin_skill_activations.digest").
				Where("p.user_id = ? AND p.disabled = ? AND p.id = ? AND p.digest = ? AND plugin_skill_activations.component_key = ? AND plugin_skill_activations.generation = ?", owner, false, row.InstallationID, row.Digest, row.ComponentKey, row.Generation).
				Count(&count).Error
			if err != nil {
				return err
			}
			if count != 1 {
				return ErrComponentUnavailable
			}
			return nil
		}
		result = append(result, skills.Skill{CheckCommandActivation: check, SkillMetadata: *meta, Slug: row.Slug, Source: "plugin", PackageFormat: row.PackageFormat, Content: body})
	}
	return result, nil
}
