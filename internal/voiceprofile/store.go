package voiceprofile

import (
	"assistente/internal/database"
)

// Store implementa Provider usando o banco de dados
type Store struct{}

// NewStore cria um novo Store de VoiceProfile
func NewStore() *Store {
	return &Store{}
}

// toData converte database.VoiceProfile para voiceprofile.Data
func toData(p *database.VoiceProfile) *Data {
	if p == nil {
		return nil
	}
	return &Data{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Provider:    p.Provider,
		VoiceID:     p.VoiceID,
		Rate:        p.Rate,
		Pitch:       p.Pitch,
		Volume:      p.Volume,
		IsDefault:   p.IsDefault,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// toDataSlice converte slice de database.VoiceProfile para slice de Data
func toDataSlice(profiles []database.VoiceProfile) []Data {
	result := make([]Data, len(profiles))
	for i, p := range profiles {
		result[i] = Data{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Provider:    p.Provider,
			VoiceID:     p.VoiceID,
			Rate:        p.Rate,
			Pitch:       p.Pitch,
			Volume:      p.Volume,
			IsDefault:   p.IsDefault,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		}
	}
	return result
}

// Create cria um novo perfil de voz
func (s *Store) Create(name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*Data, error) {
	profile, err := database.CreateVoiceProfile(name, description, provider, voiceID, rate, pitch, volume, isDefault)
	if err != nil {
		return nil, err
	}
	return toData(profile), nil
}

// GetByID retorna um perfil por ID
func (s *Store) GetByID(id uint) (*Data, error) {
	profile, err := database.GetVoiceProfile(id)
	if err != nil {
		return nil, err
	}
	return toData(profile), nil
}

// GetByName retorna um perfil por nome
func (s *Store) GetByName(name string) (*Data, error) {
	profile, err := database.GetVoiceProfileByName(name)
	if err != nil {
		return nil, err
	}
	return toData(profile), nil
}

// GetAll retorna todos os perfis
func (s *Store) GetAll() ([]Data, error) {
	profiles, err := database.GetAllVoiceProfiles()
	if err != nil {
		return nil, err
	}
	return toDataSlice(profiles), nil
}

// Update atualiza um perfil
func (s *Store) Update(id uint, name, description, provider, voiceID string, rate, pitch, volume float64, isDefault bool) (*Data, error) {
	profile, err := database.UpdateVoiceProfile(id, name, description, provider, voiceID, rate, pitch, volume, isDefault)
	if err != nil {
		return nil, err
	}
	return toData(profile), nil
}

// Delete remove um perfil
func (s *Store) Delete(id uint) error {
	return database.DeleteVoiceProfile(id)
}

// GetDefault retorna o perfil padrão
func (s *Store) GetDefault() (*Data, error) {
	profile, err := database.GetDefaultVoiceProfile()
	if err != nil {
		return nil, err
	}
	return toData(profile), nil
}

// SetDefault define um perfil como padrão
func (s *Store) SetDefault(id uint) error {
	return database.SetDefaultVoiceProfile(id)
}

// Search busca perfis por texto
func (s *Store) Search(query string) ([]Data, error) {
	profiles, err := database.SearchVoiceProfiles(query)
	if err != nil {
		return nil, err
	}
	return toDataSlice(profiles), nil
}

// Verifica que Store implementa Provider
var _ Provider = (*Store)(nil)
