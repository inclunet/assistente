package memory

import (
	"testing"

	"gorm.io/gorm"
)

func TestStore_Interface(t *testing.T) {
	t.Run("Store implements Provider interface", func(t *testing.T) {
		var _ Provider = (*Store)(nil)
	})

	t.Run("NewStore returns non-nil", func(t *testing.T) {
		store := NewStore()
		if store == nil {
			t.Error("NewStore() returned nil")
		}
	})
}

func TestData_Fields(t *testing.T) {
	data := Data{
		ID:       1,
		Title:    "Preferências do usuário",
		Content:  "O usuário prefere respostas curtas",
		Category: "preferencias",
	}

	if data.ID != 1 {
		t.Errorf("ID = %d, want 1", data.ID)
	}
	if data.Title != "Preferências do usuário" {
		t.Errorf("Title = %s, want 'Preferências do usuário'", data.Title)
	}
	if data.Content != "O usuário prefere respostas curtas" {
		t.Errorf("Content = %s, want 'O usuário prefere respostas curtas'", data.Content)
	}
	if data.Category != "preferencias" {
		t.Errorf("Category = %s, want 'preferencias'", data.Category)
	}
}

// MockProvider é um mock do Provider para testes unitários
type MockProvider struct {
	memories      map[uint]*Data
	nextID        uint
	searchResults []Data
	searchErr     error
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		memories: make(map[uint]*Data),
		nextID:   1,
	}
}

func (m *MockProvider) Create(title, content, category string) (*Data, error) {
	data := &Data{
		ID:       m.nextID,
		Title:    title,
		Content:  content,
		Category: category,
	}
	m.memories[m.nextID] = data
	m.nextID++
	return data, nil
}

func (m *MockProvider) GetAll() ([]Data, error) {
	result := make([]Data, 0, len(m.memories))
	for _, mem := range m.memories {
		result = append(result, *mem)
	}
	return result, nil
}

func (m *MockProvider) Search(query string) ([]Data, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	if m.searchResults != nil {
		return m.searchResults, nil
	}
	// Busca simples
	var results []Data
	for _, mem := range m.memories {
		if contains(mem.Title, query) || contains(mem.Content, query) || contains(mem.Category, query) {
			results = append(results, *mem)
		}
	}
	return results, nil
}

func (m *MockProvider) Delete(id uint) error {
	if _, ok := m.memories[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.memories, id)
	return nil
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr))
}

func TestMockProvider_Create(t *testing.T) {
	mock := NewMockProvider()

	data, err := mock.Create("Título", "Conteúdo da memória", "geral")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if data.ID != 1 {
		t.Errorf("Create() ID = %d, want 1", data.ID)
	}
	if data.Title != "Título" {
		t.Errorf("Create() Title = %s, want 'Título'", data.Title)
	}
	if data.Content != "Conteúdo da memória" {
		t.Errorf("Create() Content = %s, want 'Conteúdo da memória'", data.Content)
	}
	if data.Category != "geral" {
		t.Errorf("Create() Category = %s, want 'geral'", data.Category)
	}
}

func TestMockProvider_GetAll(t *testing.T) {
	mock := NewMockProvider()

	// Vazio inicialmente
	all, _ := mock.GetAll()
	if len(all) != 0 {
		t.Errorf("GetAll() len = %d, want 0", len(all))
	}

	// Adiciona algumas memórias
	mock.Create("Preferência 1", "Conteúdo 1", "preferencias")
	mock.Create("Fato 1", "Conteúdo 2", "fatos")
	mock.Create("Core 1", "Conteúdo 3", "core")

	all, _ = mock.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll() len = %d, want 3", len(all))
	}
}

func TestMockProvider_Delete(t *testing.T) {
	mock := NewMockProvider()

	// Cria uma memória
	created, _ := mock.Create("Título", "Conteúdo", "geral")

	// Verifica que existe
	all, _ := mock.GetAll()
	if len(all) != 1 {
		t.Fatalf("GetAll() len = %d, want 1", len(all))
	}

	// Deleta
	err := mock.Delete(created.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verifica que não existe mais
	all, _ = mock.GetAll()
	if len(all) != 0 {
		t.Errorf("GetAll() after Delete len = %d, want 0", len(all))
	}

	// Deleta ID inexistente
	err = mock.Delete(999)
	if err == nil {
		t.Error("Delete() expected error for non-existent ID")
	}
}

func TestMockProvider_Search(t *testing.T) {
	mock := NewMockProvider()

	// Adiciona algumas memórias
	mock.Create("Preferências de comunicação", "O usuário prefere mensagens curtas", "preferencias")
	mock.Create("Horário de trabalho", "Trabalha das 9h às 18h", "fatos")
	mock.Create("Nome do pet", "O gato se chama Felix", "fatos")

	// Busca por termo
	results, err := mock.Search("usuário")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	// Deve encontrar pelo menos 1 resultado
	if len(results) < 1 {
		t.Errorf("Search('usuário') found %d results, want at least 1", len(results))
	}
}

func TestMockProvider_SearchWithError(t *testing.T) {
	mock := NewMockProvider()
	mock.searchErr = gorm.ErrInvalidDB

	_, err := mock.Search("teste")
	if err == nil {
		t.Error("Search() expected error when searchErr is set")
	}
}

func TestMockProvider_SearchWithPredefinedResults(t *testing.T) {
	mock := NewMockProvider()
	mock.searchResults = []Data{
		{ID: 1, Title: "Resultado 1", Content: "Conteúdo 1", Category: "cat1"},
		{ID: 2, Title: "Resultado 2", Content: "Conteúdo 2", Category: "cat2"},
	}

	results, err := mock.Search("qualquer")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() len = %d, want 2", len(results))
	}
}

func TestMockProvider_MultipleCreates(t *testing.T) {
	mock := NewMockProvider()

	// Cria várias memórias e verifica IDs únicos
	ids := make(map[uint]bool)
	for i := 0; i < 10; i++ {
		data, err := mock.Create("Título", "Conteúdo", "cat")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if ids[data.ID] {
			t.Errorf("Create() returned duplicate ID: %d", data.ID)
		}
		ids[data.ID] = true
	}

	if len(ids) != 10 {
		t.Errorf("Created %d unique IDs, want 10", len(ids))
	}
}

func TestMockProvider_Categories(t *testing.T) {
	mock := NewMockProvider()

	// Cria memórias em diferentes categorias
	mock.Create("Core 1", "Informação core", "core")
	mock.Create("Core 2", "Outra core", "core")
	mock.Create("Pref 1", "Preferência", "preferencias")
	mock.Create("Fato 1", "Um fato", "fatos")

	all, _ := mock.GetAll()
	if len(all) != 4 {
		t.Errorf("GetAll() len = %d, want 4", len(all))
	}

	// Conta por categoria
	categories := make(map[string]int)
	for _, mem := range all {
		categories[mem.Category]++
	}

	if categories["core"] != 2 {
		t.Errorf("core count = %d, want 2", categories["core"])
	}
	if categories["preferencias"] != 1 {
		t.Errorf("preferencias count = %d, want 1", categories["preferencias"])
	}
	if categories["fatos"] != 1 {
		t.Errorf("fatos count = %d, want 1", categories["fatos"])
	}
}








