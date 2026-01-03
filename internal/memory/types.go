package memory

// Data representa os dados de uma memória
type Data struct {
	ID       uint   `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Category string `json:"category,omitempty"`
}

// Provider define a interface para operações de memória
type Provider interface {
	Create(title, content, category string) (*Data, error)
	GetAll() ([]Data, error)
	Search(query string) ([]Data, error)
	Delete(id uint) error
}







