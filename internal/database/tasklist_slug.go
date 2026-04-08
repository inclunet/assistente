package database

import (
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
	s := NormalizeTaskListSlug(slug)
	if s == "" {
		return nil, nil
	}
	var tl TaskList
	err := db.Where("slug = ?", s).First(&tl).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tl, nil
}

// slugTakenByOtherThan retorna true se outra lista já usa esse slug.
func slugTakenByOtherThan(normalizedSlug string, excludeID uint) (bool, error) {
	if normalizedSlug == "" {
		return false, nil
	}
	var n int64
	if err := db.Model(&TaskList{}).Where("slug = ? AND id <> ?", normalizedSlug, excludeID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetTaskListSlug define ou limpa o slug de uma lista (normalizado). slug vazio remove.
func SetTaskListSlug(taskListID uint, slug string) error {
	s := NormalizeTaskListSlug(slug)
	if err := ValidateTaskListSlugFormat(s); err != nil {
		return err
	}
	if s != "" {
		taken, err := slugTakenByOtherThan(s, taskListID)
		if err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("slug %q já está em uso por outra lista", s)
		}
	}
	return db.Model(&TaskList{}).Where("id = ?", taskListID).Update("slug", s).Error
}

// ResolveTaskListID resolve identificação por id e/ou slug.
// Regras: é obrigatório pelo menos um de id (>0) ou slug não vazio.
// Se ambos forem informados, devem referir-se à mesma lista.
func ResolveTaskListID(taskListID *uint, taskListSlug string) (uint, error) {
	var idVal uint
	if taskListID != nil {
		idVal = *taskListID
	}
	s := NormalizeTaskListSlug(taskListSlug)
	hasID := idVal > 0
	hasSlug := s != ""

	if !hasID && !hasSlug {
		return 0, fmt.Errorf("informe task_list_id ou task_list_slug")
	}
	if hasID && !hasSlug {
		var tl TaskList
		if err := db.First(&tl, idVal).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, fmt.Errorf("task_list_id %d não encontrado", idVal)
			}
			return 0, err
		}
		return tl.ID, nil
	}
	if !hasID && hasSlug {
		tl, err := FindTaskListBySlug(s)
		if err != nil {
			return 0, err
		}
		if tl == nil {
			return 0, fmt.Errorf("task_list_slug %q não encontrado", taskListSlug)
		}
		return tl.ID, nil
	}

	var byID TaskList
	if err := db.First(&byID, idVal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("task_list_id %d não encontrado", idVal)
		}
		return 0, err
	}
	bySlug, err := FindTaskListBySlug(s)
	if err != nil {
		return 0, err
	}
	if bySlug == nil {
		return 0, fmt.Errorf("task_list_slug %q não encontrado", taskListSlug)
	}
	if byID.ID != bySlug.ID {
		return 0, fmt.Errorf("task_list_id %d e task_list_slug %q referem listas diferentes", idVal, strings.TrimSpace(taskListSlug))
	}
	return byID.ID, nil
}

func ensureTaskListSlugUniqueIndex() {
	if db == nil {
		return
	}
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_task_lists_slug ON task_lists (slug) WHERE slug <> ''`)
}
