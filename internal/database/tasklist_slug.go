package database

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var taskListSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// NormalizeTaskListSlug aplica trim e minúsculas para armazenamento e comparação.
func NormalizeTaskListSlug(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidateTaskListSlugFormat valida formato quando o slug não é vazio.
func ValidateTaskListSlugFormat(normalizedSlug string) error {
	if normalizedSlug == "" {
		return nil
	}
	if !taskListSlugPattern.MatchString(normalizedSlug) {
		return errors.New("slug inválido: use apenas letras minúsculas, dígitos, hífen e underscore; deve começar com letra ou dígito; máx. 63 caracteres")
	}
	return nil
}

// FindTaskListBySlug retorna a lista pelo slug normalizado, ou nil se não existir.
func FindTaskListBySlug(slug string) (*TaskList, error) {
	return FindTaskListBySlugWithContext(context.Background(), slug)
}

func FindTaskListBySlugWithContext(ctx context.Context, slug string) (*TaskList, error) {
	s := NormalizeTaskListSlug(slug)
	if s == "" {
		return nil, nil
	}
	var tl TaskList
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Where("slug = ?", s).First(&tl).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tl, nil
}

func slugTakenByOtherThanWithContext(ctx context.Context, normalizedSlug string, excludeID string) (bool, error) {
	if normalizedSlug == "" {
		return false, nil
	}
	var n int64
	if err := ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("slug = ? AND id <> ?", normalizedSlug, excludeID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetTaskListSlug define ou limpa o slug de uma lista (normalizado). slug vazio remove.
func SetTaskListSlug(taskListID string, slug string) error {
	return SetTaskListSlugWithContext(context.Background(), taskListID, slug)
}

func SetTaskListSlugWithContext(ctx context.Context, taskListID string, slug string) error {
	s := NormalizeTaskListSlug(slug)
	if err := ValidateTaskListSlugFormat(s); err != nil {
		return err
	}
	if s != "" {
		taken, err := slugTakenByOtherThanWithContext(ctx, s, taskListID)
		if err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("slug %q já está em uso por outra lista", s)
		}
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("id = ?", taskListID).Update("slug", s).Error
}

// ResolveTaskListID resolve identificação por id e/ou slug.
// Regras: é obrigatório pelo menos um de id (não vazio) ou slug não vazio.
// Se ambos forem informados, devem referir-se à mesma lista.
func ResolveTaskListID(taskListID *string, taskListSlug string) (string, error) {
	return ResolveTaskListIDWithContext(context.Background(), taskListID, taskListSlug)
}

func ResolveTaskListIDWithContext(ctx context.Context, taskListID *string, taskListSlug string) (string, error) {
	var idVal string
	if taskListID != nil {
		idVal = *taskListID
	}
	s := NormalizeTaskListSlug(taskListSlug)
	hasID := idVal != ""
	hasSlug := s != ""

	if !hasID && !hasSlug {
		return "", fmt.Errorf("informe task_list_id ou task_list_slug")
	}
	if hasID && !hasSlug {
		var tl TaskList
		if err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&tl, "id = ?", idVal).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", fmt.Errorf("task_list_id %s não encontrado", idVal)
			}
			return "", err
		}
		return tl.ID, nil
	}
	if !hasID && hasSlug {
		tl, err := FindTaskListBySlugWithContext(ctx, s)
		if err != nil {
			return "", err
		}
		if tl == nil {
			return "", fmt.Errorf("task_list_slug %q não encontrado", taskListSlug)
		}
		return tl.ID, nil
	}

	var byID TaskList
	if err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&byID, "id = ?", idVal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("task_list_id %s não encontrado", idVal)
		}
		return "", err
	}
	bySlug, err := FindTaskListBySlugWithContext(ctx, s)
	if err != nil {
		return "", err
	}
	if bySlug == nil {
		return "", fmt.Errorf("task_list_slug %q não encontrado", taskListSlug)
	}
	if byID.ID != bySlug.ID {
		return "", fmt.Errorf("task_list_id %s e task_list_slug %q referem listas diferentes", idVal, strings.TrimSpace(taskListSlug))
	}
	return byID.ID, nil
}

func ensureTaskListSlugUniqueIndex() {
	if db == nil {
		return
	}
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_task_lists_slug ON task_lists (slug) WHERE slug <> ''`)
}
