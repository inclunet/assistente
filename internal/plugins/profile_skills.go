package plugins

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"gorm.io/gorm"
)

// Only structured references selected by this profile can create a dependency.
func bundledProfileSkills(p *profiles.Profile, revisions map[string]string) map[string]string {
	if p.Chat.DisableSkills {
		return nil
	}
	required := map[string]string{}
	for _, name := range p.Chat.EnabledSkills {
		if revision, ok := revisions["skills/"+name]; ok {
			required[name] = revision
		}
	}
	return required
}

func profileSkillRevisions(snap *pluginpackage.LocalSnapshot, components []Component, names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	selected := make(map[string]bool, len(names))
	for _, name := range names {
		selected["skills/"+name] = true
	}
	var keys []string
	for _, c := range components {
		if selected[c.Key] && (c.Kind == "skill" || c.Kind == "invalidSkill") {
			keys = append(keys, c.Key)
		}
	}
	return skillRevisions(snap, keys)
}

// The selector is transient. Packages store only the portable name, and the catalog
// continues to expose one public slash command for equivalent definitions.
func profileSkillSelector(row database.PluginSkillActivation) string {
	return skills.PluginGrantPrefix + row.Slug + ":" + row.Generation
}

type pendingProfileSkill string

func (e pendingProfileSkill) Error() string { return "PLUGIN_PROFILE_SKILL_PENDING: " + string(e) }
func (e pendingProfileSkill) Unwrap() error { return gorm.ErrRecordNotFound }

// Review local files before entering a SQLite writer transaction.
func (i *Inventory) withProfileSkills(ctx context.Context, id, digest string, required map[string]string, use func(map[string]bool) error) error {
	if len(required) == 0 {
		return use(nil)
	}
	if id == "" {
		return use(nil)
	}
	var localSlugs []string
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	err = i.db.WithContext(ctx).Table("plugin_skill_activations a").Joins("JOIN plugin_installations p ON p.id=a.installation_id AND p.digest=a.digest").Where("p.user_id=? AND p.id=? AND p.digest=? AND p.disabled=? AND a.shared_slug<>''", owner, id, digest, false).Pluck("a.shared_slug", &localSlugs).Error
	if err != nil {
		return err
	}
	needed := map[string]string{}
	for _, slug := range localSlugs {
		if r, ok := required[slug]; ok {
			needed[slug] = r
		}
	}
	if len(needed) == 0 {
		return use(nil)
	}
	return i.reviewLocalSkills(func(definitions skills.PortableDefinitions, discoveryErr error) error {
		available := map[string]bool{}
		for name, revision := range needed {
			format, _, ok := strings.Cut(revision, ":")
			if !ok {
				continue
			}
			local := localSkillMatch(definitions, discoveryErr, "skills/"+name, format, revision)
			available[name] = local.slug == name && !local.conflict
		}
		return use(available)
	})
}

// Resolve each supplied skill against this installation's own current grant.
func profileSkillBindings(ctx context.Context, db *gorm.DB, required map[string]string, native map[string]bool, id, digest string) (map[string]string, error) {
	if len(required) == 0 {
		return nil, nil
	}
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var grants []database.PluginSkillActivation
	err = db.WithContext(ctx).Model(&database.PluginSkillActivation{}).Select("component_key,generation,skill_revision,package_format,slug,shared_slug,CASE WHEN coalesce(skill_revision,'')='' THEN definition ELSE '' END AS definition").Joins("JOIN plugin_installations p ON p.id=plugin_skill_activations.installation_id AND p.digest=plugin_skill_activations.digest").Where("p.user_id=? AND p.id=? AND p.digest=? AND p.disabled=?", owner, id, digest, false).Find(&grants).Error
	if err != nil {
		return nil, err
	}
	resolved := map[string]string{}
	for _, grant := range grants {
		name := strings.TrimPrefix(grant.ComponentKey, "skills/")
		expected, ok := required[name]
		if !ok {
			continue
		}
		revision := grant.SkillRevision
		if revision == "" {
			revision = grant.PackageFormat + ":" + pluginpackage.FilesDigest(map[string][]byte{"SKILL.md": []byte(grant.Definition)})
		}
		if revision != expected {
			continue
		}
		if public := grant.SharedSlug; public != "" {
			if !strings.HasPrefix(public, skills.PluginPrefix) && (public != name || !native[name]) {
				continue
			}
		}
		resolved[name] = profileSkillSelector(grant)
	}
	return resolved, nil
}

func applyProfileSkillBindings(p *profiles.Profile, required, resolved map[string]string) error {
	for n, name := range p.Chat.EnabledSkills {
		if _, required := required[name]; required {
			if resolved[name] == "" {
				return pendingProfileSkill(name)
			}
			p.Chat.EnabledSkills[n] = resolved[name]
		}
	}
	return nil
}

func (i *Inventory) readProfileSkillBindings(ctx context.Context, id, digest string, required map[string]string) (map[string]string, error) {
	var bindings map[string]string
	err := i.withProfileSkills(ctx, id, digest, required, func(native map[string]bool) error {
		var err error
		bindings, err = profileSkillBindings(ctx, i.db, required, native, id, digest)
		return err
	})
	return bindings, err
}

type installationSkillBindings struct {
	required map[string]string
	resolved map[string]string
}

func (i *Inventory) profileSkillCatalog(ctx context.Context, rows []database.PluginProfileActivation) (map[string]installationSkillBindings, error) {
	groups := map[string]installationSkillBindings{}
	digests := map[string]string{}
	for _, row := range rows {
		if len(row.SkillRequirements) == 0 {
			continue
		}
		var selected map[string]string
		if err := json.Unmarshal(row.SkillRequirements, &selected); err != nil {
			return nil, err
		}
		group := groups[row.InstallationID]
		if group.required == nil {
			group.required = map[string]string{}
		}
		for name, revision := range selected {
			group.required[name] = revision
		}
		groups[row.InstallationID] = group
		digests[row.InstallationID] = row.Digest
	}
	for id, group := range groups {
		var err error
		group.resolved, err = i.readProfileSkillBindings(ctx, id, digests[id], group.required)
		if err != nil {
			return nil, err
		}
		groups[id] = group
	}
	return groups, nil
}
