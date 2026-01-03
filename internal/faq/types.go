package faq

// Data representa os dados de uma FAQ
type Data struct {
	ID       uint   `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Tags     string `json:"tags,omitempty"`
}

// Provider define a interface para operações de FAQ
type Provider interface {
	Create(question, answer, tags string) (*Data, error)
	Get(id uint) (*Data, error)
	GetAll() ([]Data, error)
	Update(id uint, question, answer, tags string) (*Data, error)
	Delete(id uint) error
	Search(query string) ([]Data, error)
}






