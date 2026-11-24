package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	var bound []database.PluginMCPBinding
	if err := i.db.WithContext(ctx).Model(&database.PluginMCPBinding{}).Select("plugin_mcp_bindings.component_key, plugin_mcp_bindings.slug").Joins("JOIN plugin_installations p ON p.id=plugin_mcp_bindings.installation_id AND p.digest=plugin_mcp_bindings.digest").Joins("JOIN mcp_servers s ON s.id=plugin_mcp_bindings.server_id AND s.user_id=p.user_id").Where("p.id=? AND p.user_id=? AND p.digest=?", id, owner, digest).Find(&bound).Error; err != nil {
		return nil, err
	}
	for _, row := range bound {
		slugs[row.ComponentKey] = row.Slug
	}
	for n := range components {
		slug, ok := slugs[components[n].Key]
		components[n].Active = ok
		components[n].Slug = slug
	}
	return components, nil
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
			hash := sha256.Sum256([]byte(id + "\x00" + key))
			prefix := component.Name
			if len(prefix) > 28 {
				prefix = prefix[:28]
			}
			activation = database.PluginSkillActivation{InstallationID: id, ComponentKey: key, Digest: digest, Slug: skills.PluginPrefix + strings.TrimRight(prefix, "-") + "-" + hex.EncodeToString(hash[:12]), Definition: string(snap.Files[component.Entry])}
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
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "installation_id"}, {Name: "component_key"}}, DoUpdates: clause.AssignmentColumns([]string{"digest", "slug", "definition"})}).Create(&activation).Error
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
		meta.DisplayName = meta.Name
		meta.Name = row.Slug
		result = append(result, skills.Skill{SkillMetadata: *meta, Slug: row.Slug, Source: "plugin", Content: body})
	}
	return result, nil
}
