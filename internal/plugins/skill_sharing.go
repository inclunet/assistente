package plugins

import (
	"context"
	"slices"
	"sort"
	"strings"

	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"gorm.io/gorm"
)

// Prefixing the interpreter format keeps identical source with different execution semantics separate.
func skillRevisions(snap *pluginpackage.LocalSnapshot, keys []string) map[string]string {
	files := make(map[string]map[string][]byte, len(keys))
	revisions := make(map[string]string, len(keys))
	for _, key := range keys {
		files[key] = map[string][]byte{}
	}
	if len(files) == 0 {
		return revisions
	}
	for path, data := range snap.Files {
		relative, ok := strings.CutPrefix(path, "skills/")
		if !ok {
			continue
		}
		name, file, ok := strings.Cut(relative, "/")
		if group := files["skills/"+name]; ok && group != nil {
			group[file] = data
		}
	}
	for key, group := range files {
		revisions[key] = snap.Manifest.Format + ":" + pluginpackage.FilesDigest(group)
	}
	return revisions
}

type skillPeer struct {
	database.PluginSkillActivation
	Name, Origin string
}

func activeSkillCandidates(tx *gorm.DB, owner, installation string, keys []string) (map[string][]skillPeer, error) {
	groups := map[string][]skillPeer{}
	for len(keys) > 0 {
		n := min(300, len(keys))
		var rows []skillPeer
		err := tx.Table("plugin_skill_activations").Select("plugin_skill_activations.installation_id, component_key, plugin_skill_activations.digest, generation,package_format,slug,shared_slug,skill_revision,CASE WHEN coalesce(skill_revision,'')='' THEN definition ELSE '' END AS definition,p.name,p.origin").Joins("JOIN plugin_installations p ON p.id=plugin_skill_activations.installation_id AND p.digest=plugin_skill_activations.digest").Where("p.user_id=? AND p.id<>? AND p.disabled=? AND component_key IN ?", owner, installation, false, keys[:n]).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			groups[row.ComponentKey] = append(groups[row.ComponentKey], row)
		}
		keys = keys[n:]
	}
	return groups, nil
}

type skillCandidate struct {
	slug, token string
	conflict    bool
	consumers   []string
}

func skillMatch(rows []skillPeer, revision string) skillCandidate {
	var candidate skillCandidate
	generations := map[string][]byte{"revision": []byte(revision)}
	for _, row := range rows {
		generations["peer:"+row.InstallationID+"\x00"+row.ComponentKey] = []byte(row.Generation + "\x00" + row.Digest)
		candidate.consumers = append(candidate.consumers, row.Name+" ("+row.Origin+")")
		slug := row.SharedSlug
		if slug == "" {
			slug = row.Slug
		}
		current := row.SkillRevision
		if current == "" {
			// All previously activatable skills contained a single SKILL.md.
			current = row.PackageFormat + ":" + pluginpackage.FilesDigest(map[string][]byte{"SKILL.md": []byte(row.Definition)})
		}
		if current != revision || candidate.slug != "" && candidate.slug != slug {
			candidate.conflict = true
		}
		candidate.slug = slug
	}
	if candidate.slug != "" {
		generations["slug"] = []byte(candidate.slug)
		candidate.token = pluginpackage.FilesDigest(generations)
	}
	sort.Strings(candidate.consumers)
	candidate.consumers = slices.Compact(candidate.consumers)
	return candidate
}

func (i *Inventory) DescribeExistingSkills(ctx context.Context, components []Component, snap *pluginpackage.LocalSnapshot, id string) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	var keys []string
	for _, c := range components {
		if c.Kind == "skill" && c.Reason == "" && !c.Active {
			keys = append(keys, c.Key)
		}
	}
	revisions := skillRevisions(snap, keys)
	groups, err := activeSkillCandidates(i.db.WithContext(ctx), owner, id, keys)
	if err != nil {
		return err
	}
	for n := range components {
		c := &components[n]
		if c.Kind != "skill" || c.Active || c.Reason != "" || len(groups[c.Key]) == 0 {
			continue
		}
		candidate := skillMatch(groups[c.Key], revisions[c.Key])
		c.SkillConsumers = candidate.consumers
		if candidate.conflict {
			c.Reason = "skillConflict"
		} else {
			c.Reason = "skillAlreadyAvailable"
			c.SkillReuseToken = candidate.token
		}
	}
	return nil
}
