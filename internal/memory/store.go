package memory

import (
	"assistente/internal/database"
)

// Store implementa Provider usando o banco de dados
type Store struct{}

// NewStore cria um novo Store de Memory
func NewStore() *Store {
	return &Store{}
}

// Create cria uma nova memória
func (s *Store) Create(title, content, category string) (*Data, error) {
	memory, err := database.CreateMemory(title, content, category)
	if err != nil {
		return nil, err
	}
	return &Data{
		ID:       memory.ID,
		Title:    memory.Title,
		Content:  memory.Content,
		Category: memory.Category,
	}, nil
}

// GetAll retorna todas as memórias
func (s *Store) GetAll() ([]Data, error) {
	memories, err := database.GetAllMemories()
	if err != nil {
		return nil, err
	}
	result := make([]Data, len(memories))
	for i, m := range memories {
		result[i] = Data{
			ID:       m.ID,
			Title:    m.Title,
			Content:  m.Content,
			Category: m.Category,
		}
	}
	return result, nil
}

// Search busca memórias por texto
func (s *Store) Search(query string) ([]Data, error) {
	memories, err := database.SearchMemories(query)
	if err != nil {
		return nil, err
	}
	result := make([]Data, len(memories))
	for i, m := range memories {
		result[i] = Data{
			ID:       m.ID,
			Title:    m.Title,
			Content:  m.Content,
			Category: m.Category,
		}
	}
	return result, nil
}

// Delete remove uma memória
func (s *Store) Delete(id uint) error {
	return database.DeleteMemory(id)
}

// Verifica que Store implementa Provider
var _ Provider = (*Store)(nil)






