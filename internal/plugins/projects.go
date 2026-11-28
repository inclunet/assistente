package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BuilderProject struct {
	ID        string         `json:"id"`
	Revision  int            `json:"revision"`
	Selection BuilderRequest `json:"selection"`
}

func (i *Inventory) Projects(ctx context.Context) ([]BuilderProject, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.PluginProject
	if err = i.db.WithContext(ctx).Where("user_id = ?", owner).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]BuilderProject, 0, len(rows))
	for _, row := range rows {
		project := BuilderProject{ID: row.ID, Revision: row.Revision}
		if err := json.Unmarshal(row.Selection, &project.Selection); err != nil {
			return nil, err
		}
		result = append(result, project)
	}
	return result, nil
}

func (i *Inventory) SaveProject(ctx context.Context, project BuilderProject, admit commandsecurity.Admission) (BuilderProject, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return BuilderProject{}, err
	}
	if admit == nil {
		return BuilderProject{}, database.ErrUserScopeRequired
	}
	if strings.TrimSpace(project.Selection.Name) == "" || project.Revision < 0 || (project.ID == "") != (project.Revision == 0) {
		return BuilderProject{}, errors.New("PLUGIN_PROJECT_INVALID")
	}
	body, err := json.Marshal(project.Selection)
	if err != nil {
		return BuilderProject{}, err
	}
	saved := project
	err = database.WithSQLiteAdmittedCommit(ctx, i.db, "plugins.project.save", func(tx *gorm.DB) error {
		if project.ID == "" {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			row := database.PluginProject{ID: id.String(), UserID: owner, Revision: 1, Selection: body}
			if err = tx.Create(&row).Error; err != nil {
				return err
			}
			saved.ID, saved.Revision = row.ID, row.Revision
			return nil
		}
		update := tx.Model(&database.PluginProject{}).Where("user_id=? AND id=? AND revision=?", owner, project.ID, project.Revision).Updates(map[string]any{"selection": body, "revision": project.Revision + 1})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("PLUGIN_PROJECT_CHANGED")
		}
		saved.Revision = project.Revision + 1
		return nil
	}, admit)
	if err != nil {
		return BuilderProject{}, err
	}
	return saved, nil
}
