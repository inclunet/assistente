package voiceprofile

import "time"

// Data representa os dados de um perfil de voz
type Data struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Provider    string    `json:"provider"`
	VoiceID     string    `json:"voice_id"`
	Rate        float64   `json:"rate"`
	Pitch       float64   `json:"pitch"`
	Volume      float64   `json:"volume"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Provider define a interface para operações de perfil de voz
// Esta interface é usada pelos agentes para interagir com perfis de voz
type Provider interface {
	// CRUD básico
	Create(name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*Data, error)
	GetByID(id uint) (*Data, error)
	GetByName(name string) (*Data, error)
	GetAll() ([]Data, error)
	Update(id uint, name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*Data, error)
	Delete(id uint) error

	// Operações específicas
	GetDefault() (*Data, error)
	SetDefault(id uint) error
	Search(query string) ([]Data, error)
}
