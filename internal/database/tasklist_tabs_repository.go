package database

import (
	"errors"

	"gorm.io/gorm"
)

const MaxTaskListTabs = 20

// GetAllTaskListTabs retorna todas as abas de task lists ordenadas por posição
func GetAllTaskListTabs() ([]TaskListTab, error) {
	var tabs []TaskListTab
	err := db.Preload("TaskList").
		Order("position ASC").
		Find(&tabs).Error
	return tabs, err
}

// CreateTaskListTab cria uma nova aba para uma task list.
// Se já existe aba para o mesmo taskListID, retorna a existente e a ativa.
func CreateTaskListTab(taskListID uint, title string, setActive bool) (*TaskListTab, error) {
	// Verifica se já existe aba para esta task list
	var existing TaskListTab
	err := db.Where("task_list_id = ?", taskListID).First(&existing).Error
	if err == nil {
		// Já existe — ativa se necessário e retorna
		if setActive {
			db.Model(&TaskListTab{}).Where("is_active = ?", true).Update("is_active", false)
			db.Model(&existing).Update("is_active", true)
		}
		return &existing, nil
	}

	// Verifica limite
	var count int64
	if err := db.Model(&TaskListTab{}).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= MaxTaskListTabs {
		return nil, errors.New("limite de abas de task list atingido")
	}

	// Se setActive=true, desativa outras abas
	if setActive {
		db.Model(&TaskListTab{}).Where("is_active = ?", true).Update("is_active", false)
	}

	// Calcula próxima posição
	var maxPos int
	db.Model(&TaskListTab{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)

	tab := &TaskListTab{
		TaskListID: taskListID,
		Title:      title,
		Position:   maxPos + 1,
		IsActive:   setActive,
	}

	if err := db.Create(tab).Error; err != nil {
		return nil, err
	}

	return tab, nil
}

// CloseTaskListTab fecha (deleta) uma aba pelo ID
func CloseTaskListTab(id uint) error {
	var tab TaskListTab
	err := db.First(&tab, id).Error
	if err != nil {
		return err
	}

	wasActive := tab.IsActive
	if err := db.Delete(&tab).Error; err != nil {
		return err
	}

	// Se era a aba ativa, ativa a próxima
	if wasActive {
		var nextTab TaskListTab
		if err := db.Order("position ASC").First(&nextTab).Error; err == nil {
			db.Model(&nextTab).Update("is_active", true)
		}
	}

	return nil
}

// SetActiveTaskListTab define qual aba está ativa
func SetActiveTaskListTab(id uint) error {
	// Verifica se existe
	var tab TaskListTab
	if err := db.First(&tab, id).Error; err != nil {
		return err
	}

	// Desativa todas
	db.Model(&TaskListTab{}).Where("is_active = ?", true).Update("is_active", false)

	// Ativa a selecionada
	return db.Model(&tab).Update("is_active", true).Error
}

// CloseTaskListTabByTaskListID fecha a aba associada a uma task list (usado quando a task list é deletada)
func CloseTaskListTabByTaskListID(taskListID uint) error {
	var tab TaskListTab
	err := db.Where("task_list_id = ?", taskListID).First(&tab).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // Não tem aba, nada a fazer
	}
	if err != nil {
		return err
	}
	return CloseTaskListTab(tab.ID)
}
