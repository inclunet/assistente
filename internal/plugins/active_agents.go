package plugins

import (
	"context"
	"errors"
	"path"
	"strings"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/slug"
	"assistente/internal/subagent"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SetAgentActive grants access only to the exact installed revision.
func (i *Inventory) SetAgentActive(ctx context.Context, id, digest, key string, active bool, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	var grant database.PluginAgentActivation
	if active {
		snap, err := i.installedSnapshot(ctx, id, digest)
		if err != nil {
			return err
		}
		for _, c := range DiscoverComponents(snap) {
			if c.Kind == "agent" && c.Key == key && c.Reason == "" {
				generation, err := uuid.NewV7()
				if err != nil {
					return err
				}
				name := slug.Slugify(strings.TrimSuffix(path.Base(key), ".md"), "agent")
				grant = database.PluginAgentActivation{Slug: pluginComponentSlug(id, key, name), InstallationID: id, ComponentKey: key, Digest: digest, Generation: generation.String()}
				break
			}
		}
		if grant.Generation == "" {
			return ErrComponentUnavailable
		}
	}
	return database.WithSQLiteAdmittedCommit(ctx, i.db, "plugins.agent.activation", func(tx *gorm.DB) error {
		var row database.PluginInstallation
		if err := tx.Where("user_id=? AND id=? AND digest=?", owner, id, digest).Take(&row).Error; err != nil {
			return err
		}
		if !active {
			return tx.Where("installation_id=? AND component_key=?", id, key).Delete(&database.PluginAgentActivation{}).Error
		}
		if row.Disabled {
			return ErrInstallationDisabled
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error
	}, admit)
}

func (i *Inventory) agentGrants(ctx context.Context) (*gorm.DB, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	return i.db.WithContext(ctx).Model(&database.PluginAgentActivation{}).Joins("JOIN plugin_installations p ON p.id=plugin_agent_activations.installation_id AND p.digest=plugin_agent_activations.digest").Where("p.user_id=? AND p.disabled=?", owner, false), nil
}

// AgentGeneration resolves a user-facing slug only within the active account.
func (i *Inventory) AgentGeneration(ctx context.Context, slug string) (string, error) {
	q, err := i.agentGrants(ctx)
	if err != nil {
		return "", err
	}
	var grant database.PluginAgentActivation
	if err = q.Where("plugin_agent_activations.slug=?", slug).Take(&grant).Error; err != nil {
		return "", err
	}
	return grant.Generation, nil
}

// CheckAgent verifies the current grant before a delegated operation.
func (i *Inventory) CheckAgent(ctx context.Context, generation string) error {
	q, err := i.agentGrants(ctx)
	if err != nil {
		return err
	}
	var grant database.PluginAgentActivation
	return q.Select("generation").Where("generation=?", generation).Take(&grant).Error
}

var ErrAgentInactive = errors.New("ativação do agente encerrada")

// ResolveAgent reads the authorized definition from the installation snapshot.
func (i *Inventory) ResolveAgent(ctx context.Context, generation string) (subagent.RunParams, error) {
	query, err := i.agentGrants(ctx)
	if err != nil {
		return subagent.RunParams{}, err
	}
	var grant database.PluginAgentActivation
	if err = query.Where("generation=?", generation).Take(&grant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return subagent.RunParams{}, ErrAgentInactive
		}
		return subagent.RunParams{}, err
	}
	snap, err := i.installedSnapshot(ctx, grant.InstallationID, grant.Digest)
	if err != nil {
		return subagent.RunParams{}, err
	}
	raw, exists := snap.Files[grant.ComponentKey]
	if !exists {
		return subagent.RunParams{}, ErrComponentUnavailable
	}
	run, _, err := readClaudeAgent(raw)
	if err != nil {
		return subagent.RunParams{}, errors.Join(ErrComponentUnavailable, err)
	}
	return run, nil
}
