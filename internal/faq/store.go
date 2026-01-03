package faq

import (
	"assistente/internal/database"
)

// Store implementa Provider usando o banco de dados
type Store struct{}

// NewStore cria um novo Store de FAQ
func NewStore() *Store {
	return &Store{}
}

// Create cria uma nova FAQ
func (s *Store) Create(question, answer, tags string) (*Data, error) {
	faq, err := database.CreateFAQ(question, answer, tags)
	if err != nil {
		return nil, err
	}

	// Gera embedding em background
	go func() {
		database.GenerateFAQEmbedding(faq.ID)
	}()

	return &Data{
		ID:       faq.ID,
		Question: faq.Question,
		Answer:   faq.Answer,
		Tags:     faq.Tags,
	}, nil
}

// Get retorna uma FAQ por ID
func (s *Store) Get(id uint) (*Data, error) {
	faq, err := database.GetFAQ(id)
	if err != nil {
		return nil, err
	}
	return &Data{
		ID:       faq.ID,
		Question: faq.Question,
		Answer:   faq.Answer,
		Tags:     faq.Tags,
	}, nil
}

// GetAll retorna todas as FAQs
func (s *Store) GetAll() ([]Data, error) {
	faqs, err := database.GetAllFAQs()
	if err != nil {
		return nil, err
	}
	result := make([]Data, len(faqs))
	for i, faq := range faqs {
		result[i] = Data{
			ID:       faq.ID,
			Question: faq.Question,
			Answer:   faq.Answer,
			Tags:     faq.Tags,
		}
	}
	return result, nil
}

// Update atualiza uma FAQ
func (s *Store) Update(id uint, question, answer, tags string) (*Data, error) {
	faq, err := database.UpdateFAQ(id, question, answer, tags)
	if err != nil {
		return nil, err
	}

	// Regenera embedding em background
	go func() {
		database.GenerateFAQEmbedding(faq.ID)
	}()

	return &Data{
		ID:       faq.ID,
		Question: faq.Question,
		Answer:   faq.Answer,
		Tags:     faq.Tags,
	}, nil
}

// Delete remove uma FAQ
func (s *Store) Delete(id uint) error {
	return database.DeleteFAQ(id)
}

// Search busca FAQs usando busca semântica
func (s *Store) Search(query string) ([]Data, error) {
	faqs, err := database.SearchFAQSemantic(query, 5, 0.5)
	if err != nil {
		return nil, err
	}
	result := make([]Data, len(faqs))
	for i, faq := range faqs {
		result[i] = Data{
			ID:       faq.ID,
			Question: faq.Question,
			Answer:   faq.Answer,
			Tags:     faq.Tags,
		}
	}
	return result, nil
}

// Verifica que Store implementa Provider
var _ Provider = (*Store)(nil)






