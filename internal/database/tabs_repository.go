package database

import (
	"errors"

	"gorm.io/gorm"
)

const MaxTabs = 20

// GetAllTabs retorna todas as abas ordenadas por posição
func GetAllTabs() ([]ChatTab, error) {
	var tabs []ChatTab
	err := db.Preload("Conversation").
		Order("position ASC").
		Find(&tabs).Error
	return tabs, err
}

// GetActiveTab retorna a aba ativa (ou cria uma se não existir)
func GetActiveTab() (*ChatTab, error) {
	var tab ChatTab
	err := db.Where("is_active = ?", true).First(&tab).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Não há aba ativa - cria uma nova
		return CreateTab("Nova conversa", "💬", true)
	}

	return &tab, err
}

// CreateTab cria uma nova aba
func CreateTab(title, icon string, setActive bool) (*ChatTab, error) {
	// Verifica limite
	var count int64
	if err := db.Model(&ChatTab{}).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= MaxTabs {
		return nil, errors.New("limite de abas atingido")
	}

	// Se setActive=true, desativa outras abas
	if setActive {
		db.Model(&ChatTab{}).Where("is_active = ?", true).Update("is_active", false)
	}

	// Calcula próxima posição
	var maxPos int
	db.Model(&ChatTab{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)

	tab := &ChatTab{
		Title:    title,
		Icon:     icon,
		Position: maxPos + 1,
		IsActive: setActive,
	}

	if err := db.Create(tab).Error; err != nil {
		return nil, err
	}

	return tab, nil
}

// CloseTab fecha uma aba (remove do banco)
func CloseTab(id uint) error {
	var tab ChatTab
	if err := db.First(&tab, id).Error; err != nil {
		return err
	}

	wasActive := tab.IsActive

	// Deleta aba
	if err := db.Delete(&tab).Error; err != nil {
		return err
	}

	// Se era a aba ativa, ativa outra
	if wasActive {
		var nextTab ChatTab
		err := db.Where("id != ?", id).
			Order("position ASC").
			First(&nextTab).Error

		if err == nil {
			db.Model(&nextTab).Update("is_active", true)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	return nil
}

// SetActiveTab define a aba ativa
func SetActiveTab(id uint) error {
	// Desativa todas
	db.Model(&ChatTab{}).Update("is_active", false)

	// Ativa a selecionada
	return db.Model(&ChatTab{}).Where("id = ?", id).Update("is_active", true).Error
}

// UpdateTabTitle atualiza o título de uma aba
func UpdateTabTitle(id uint, title string) error {
	return db.Model(&ChatTab{}).Where("id = ?", id).Update("title", title).Error
}

// LoadConversationInTab carrega uma conversa em uma aba
func LoadConversationInTab(tabId, conversationId uint) error {
	// Valida que a conversa existe
	var conv Conversation
	if err := db.First(&conv, conversationId).Error; err != nil {
		return err
	}

	// Atualiza aba
	return db.Model(&ChatTab{}).Where("id = ?", tabId).Updates(map[string]interface{}{
		"conversation_id": conversationId,
		"title":           conv.Title,
	}).Error
}

// ClearTab limpa a conversa de uma aba (reseta para nova conversa)
func ClearTab(id uint) error {
	return db.Model(&ChatTab{}).Where("id = ?", id).Updates(map[string]interface{}{
		"conversation_id": nil,
		"title":           "Nova conversa",
	}).Error
}

// ReorderTabs reordena as abas
func ReorderTabs(orderedIds []uint) error {
	for i, id := range orderedIds {
		if err := db.Model(&ChatTab{}).Where("id = ?", id).Update("position", i).Error; err != nil {
			return err
		}
	}
	return nil
}

// InitializeDefaultTab cria uma aba padrão se não existir nenhuma
func InitializeDefaultTab() error {
	var count int64
	if err := db.Model(&ChatTab{}).Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		_, err := CreateTab("Nova conversa", "💬", true)
		return err
	}

	return nil
}
