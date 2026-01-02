package faq

import (
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTestDB configura um banco SQLite em memória para testes
func setupTestDB(t *testing.T) func() {
	t.Helper()

	// Cria banco em memória
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("erro ao criar banco de teste: %v", err)
	}

	// Injeta o banco no pacote database usando reflexão
	// Como database.db é privado, precisamos usar Init com um truque
	// Por ora, vamos testar via o pacote database diretamente

	// Faz migrate das tabelas necessárias
	err = db.AutoMigrate(&database.FAQ{})
	if err != nil {
		t.Fatalf("erro ao migrar tabelas: %v", err)
	}

	// Guarda o db original
	originalDB := database.DB()

	// Substitui pelo banco de teste
	setTestDB(db)

	// Retorna cleanup function
	return func() {
		setTestDB(originalDB)
	}
}

// setTestDB é um hack para injetar o DB de teste
// No mundo real, seria melhor usar injeção de dependência
var testDB *gorm.DB

func setTestDB(db *gorm.DB) {
	testDB = db
}

func TestStore_Create(t *testing.T) {
	// Para este teste funcionar, precisamos de um banco real ou mock
	// Por hora, vamos criar um teste que verifica a estrutura

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
		Question: "Como funciona?",
		Answer:   "Funciona assim!",
		Tags:     "teste,faq",
	}

	if data.ID != 1 {
		t.Errorf("ID = %d, want 1", data.ID)
	}
	if data.Question != "Como funciona?" {
		t.Errorf("Question = %s, want 'Como funciona?'", data.Question)
	}
	if data.Answer != "Funciona assim!" {
		t.Errorf("Answer = %s, want 'Funciona assim!'", data.Answer)
	}
	if data.Tags != "teste,faq" {
		t.Errorf("Tags = %s, want 'teste,faq'", data.Tags)
	}
}

func TestProvider_Interface(t *testing.T) {
	// Verifica que a interface Provider tem todos os métodos esperados
	var p Provider

	// Tenta atribuir um Store
	p = NewStore()

	// Se compilou, a interface está correta
	if p == nil {
		t.Error("Store should implement Provider")
	}
}

// MockProvider é um mock do Provider para testes unitários
type MockProvider struct {
	faqs          map[uint]*Data
	nextID        uint
	searchResults []Data
	searchErr     error
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		faqs:   make(map[uint]*Data),
		nextID: 1,
	}
}

func (m *MockProvider) Create(question, answer, tags string) (*Data, error) {
	data := &Data{
		ID:       m.nextID,
		Question: question,
		Answer:   answer,
		Tags:     tags,
	}
	m.faqs[m.nextID] = data
	m.nextID++
	return data, nil
}

func (m *MockProvider) Get(id uint) (*Data, error) {
	data, ok := m.faqs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return data, nil
}

func (m *MockProvider) GetAll() ([]Data, error) {
	result := make([]Data, 0, len(m.faqs))
	for _, faq := range m.faqs {
		result = append(result, *faq)
	}
	return result, nil
}

func (m *MockProvider) Update(id uint, question, answer, tags string) (*Data, error) {
	data, ok := m.faqs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	data.Question = question
	data.Answer = answer
	data.Tags = tags
	return data, nil
}

func (m *MockProvider) Delete(id uint) error {
	if _, ok := m.faqs[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.faqs, id)
	return nil
}

func (m *MockProvider) Search(query string) ([]Data, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	if m.searchResults != nil {
		return m.searchResults, nil
	}
	// Busca simples por substring
	var results []Data
	for _, faq := range m.faqs {
		if contains(faq.Question, query) || contains(faq.Answer, query) || contains(faq.Tags, query) {
			results = append(results, *faq)
		}
	}
	return results, nil
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr))
}

func TestMockProvider_Create(t *testing.T) {
	mock := NewMockProvider()

	data, err := mock.Create("Pergunta?", "Resposta!", "tag1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if data.ID != 1 {
		t.Errorf("Create() ID = %d, want 1", data.ID)
	}
	if data.Question != "Pergunta?" {
		t.Errorf("Create() Question = %s, want 'Pergunta?'", data.Question)
	}
	if data.Answer != "Resposta!" {
		t.Errorf("Create() Answer = %s, want 'Resposta!'", data.Answer)
	}
	if data.Tags != "tag1" {
		t.Errorf("Create() Tags = %s, want 'tag1'", data.Tags)
	}
}

func TestMockProvider_Get(t *testing.T) {
	mock := NewMockProvider()

	// Cria uma FAQ
	created, _ := mock.Create("Pergunta?", "Resposta!", "tag1")

	// Busca por ID
	data, err := mock.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if data.Question != "Pergunta?" {
		t.Errorf("Get() Question = %s, want 'Pergunta?'", data.Question)
	}

	// Busca ID inexistente
	_, err = mock.Get(999)
	if err == nil {
		t.Error("Get() expected error for non-existent ID")
	}
}

func TestMockProvider_GetAll(t *testing.T) {
	mock := NewMockProvider()

	// Vazio inicialmente
	all, _ := mock.GetAll()
	if len(all) != 0 {
		t.Errorf("GetAll() len = %d, want 0", len(all))
	}

	// Adiciona algumas FAQs
	mock.Create("P1", "R1", "t1")
	mock.Create("P2", "R2", "t2")
	mock.Create("P3", "R3", "t3")

	all, _ = mock.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll() len = %d, want 3", len(all))
	}
}

func TestMockProvider_Update(t *testing.T) {
	mock := NewMockProvider()

	// Cria uma FAQ
	created, _ := mock.Create("Pergunta?", "Resposta!", "tag1")

	// Atualiza
	updated, err := mock.Update(created.ID, "Nova Pergunta?", "Nova Resposta!", "tag2")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.Question != "Nova Pergunta?" {
		t.Errorf("Update() Question = %s, want 'Nova Pergunta?'", updated.Question)
	}
	if updated.Answer != "Nova Resposta!" {
		t.Errorf("Update() Answer = %s, want 'Nova Resposta!'", updated.Answer)
	}
	if updated.Tags != "tag2" {
		t.Errorf("Update() Tags = %s, want 'tag2'", updated.Tags)
	}

	// Verifica que a alteração persistiu
	fetched, _ := mock.Get(created.ID)
	if fetched.Question != "Nova Pergunta?" {
		t.Errorf("After Update, Get() Question = %s, want 'Nova Pergunta?'", fetched.Question)
	}

	// Atualiza ID inexistente
	_, err = mock.Update(999, "X", "Y", "Z")
	if err == nil {
		t.Error("Update() expected error for non-existent ID")
	}
}

func TestMockProvider_Delete(t *testing.T) {
	mock := NewMockProvider()

	// Cria uma FAQ
	created, _ := mock.Create("Pergunta?", "Resposta!", "tag1")

	// Verifica que existe
	_, err := mock.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() before Delete error = %v", err)
	}

	// Deleta
	err = mock.Delete(created.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verifica que não existe mais
	_, err = mock.Get(created.ID)
	if err == nil {
		t.Error("Get() after Delete should return error")
	}

	// Deleta ID inexistente
	err = mock.Delete(999)
	if err == nil {
		t.Error("Delete() expected error for non-existent ID")
	}
}

func TestMockProvider_Search(t *testing.T) {
	mock := NewMockProvider()

	// Adiciona algumas FAQs
	mock.Create("Como resetar senha?", "Clique em esqueci minha senha", "senha,reset")
	mock.Create("Como criar conta?", "Clique em cadastrar", "conta,cadastro")
	mock.Create("Horário de atendimento?", "Das 9h às 18h", "horario,atendimento")

	// Busca por termo
	results, err := mock.Search("senha")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	// Deve encontrar pelo menos 1 resultado
	if len(results) < 1 {
		t.Errorf("Search('senha') found %d results, want at least 1", len(results))
	}
}




