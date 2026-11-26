package providers

import (
	"assistente/internal/commandsecurity"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"io"
	"net/url"
	"strings"

	"assistente/internal/database"
	"assistente/internal/llm"
)

var ErrPackageDefinition = errors.New("configuração de provedor indisponível para o pacote")

// MarshalPackageDefinition parte da configuração nativa e retira o estado da conta.
func MarshalPackageDefinition(row *database.LLMProvider) ([]byte, error) {
	p, err := fromDBModel(row)
	if err != nil {
		return nil, err
	}
	if len(p.ACPEnv) > 0 || len(p.ACPCredentialEnv) > 0 {
		return nil, ErrPackageDefinition
	}
	p.AuthMode = p.EffectiveAuthMode()
	p.ID = ""
	p.IsDefault = false
	p.CredentialPattern = ""
	p.InsecureHTTPAuthorizedURL = ""
	p.Headers = nil
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	decoded, err := ReadPackageDefinition(raw, p.Slug)
	if err != nil {
		return nil, err
	}
	if p.ErrorFamily != "" && decoded.ErrorFamily != p.ErrorFamily {
		return nil, ErrPackageDefinition
	}
	return raw, nil
}

// ReadPackageDefinition valida o recurso sem contato com a rede ou com o processo.
func ReadPackageDefinition(raw []byte, slug string) (*llm.ProviderConfig, error) {
	if len(raw) > 64<<10 || slug == "" {
		return nil, ErrPackageDefinition
	}
	var p llm.ProviderConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&p) != nil || decoder.Decode(new(any)) != io.EOF || p.Slug != slug || p.ID != "" || p.IsDefault || p.CredentialPattern != "" || p.InsecureHTTPAuthorizedURL != "" || len(p.Headers) > 0 || len(p.ACPEnv) > 0 || len(p.ACPCredentialEnv) > 0 {
		return nil, ErrPackageDefinition
	}
	if validateCredentialMode(&p, nil) != nil || !p.IsACP() && p.ACPAgentID != "" {
		return nil, ErrPackageDefinition
	}
	if p.Type == "" || p.Type == llm.ProviderChatGPT || (p.Type == llm.ProviderACP) != p.IsACP() {
		return nil, ErrPackageDefinition
	}
	if p.IsACP() {
		if p.Type != llm.ProviderACP || p.BaseURL != "" {
			return nil, ErrPackageDefinition
		}
	} else {
		if _, err := catalogAPIFormat(string(p.GetAPIFormat())); err != nil {
			return nil, ErrPackageDefinition
		}
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(p.BaseURL, "#") {
			return nil, ErrPackageDefinition
		}
	}
	p.ErrorFamily = llm.ExplicitProviderErrorFamilyFromType(p.Type)
	if p.ServerProfileID != "" {
		profile, ok := catalogRuntimeProfiles[p.ServerProfileID]
		origin, _, originErr := catalogCredentialOrigin(p.BaseURL)
		if !ok || originErr != nil || origin != profile.credentialOrigin || p.IsACP() || p.Type != profile.providerType || string(p.GetAPIFormat()) != profile.apiFormat {
			return nil, ErrPackageDefinition
		}
		p.ErrorFamily = catalogErrorFamily(profile.providerFamily)
	}
	// O validador local exige uma identidade. O destino atribui o ID definitivo.
	p.ID = slug
	err := p.Validate()
	p.ID = ""
	if err != nil && !errors.Is(err, llm.ErrInsecureHTTPAuthorizationRequired) {
		return nil, ErrPackageDefinition
	}
	return &p, nil
}

// InsertPackageDefinition grava a configuração já revisada na transação da instalação.
func InsertPackageDefinition(ctx context.Context, tx *gorm.DB, p llm.ProviderConfig, id string) error {
	p.ID = id
	p.RequiresExplicitDefault = true
	if !p.IsACP() && p.EffectiveAuthMode() != llm.AuthModeNone {
		p.CredentialPattern = "provider:" + id
	}
	return database.SaveLLMProviderTx(ctx, tx, toDBModel(&p))
}

// RefreshPackagedProvider publica o cadastro no registro da sessão que o instalou.
func (s *Service) RefreshPackagedProvider(ctx context.Context, id string, admit commandsecurity.Admission) error {
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	generation := s.registry.Generation()
	s.publicationMu.Lock()
	defer s.publicationMu.Unlock()
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return gorm.ErrRecordNotFound
	}
	return admit(nil, func() error {
		return s.registry.WithGeneration(generation, func() error {
			err := s.registry.RegisterGeneration(p, generation)
			if errors.Is(err, llm.ErrInsecureHTTPAuthorizationRequired) {
				return s.registry.RegisterPendingAuthorization(p)
			}
			return err
		})
	})
}

// RemovePackagedProviders serializa a gravação e a publicação com as edições
// nativas. Depois do commit basta remover as instâncias da geração capturada.
func (s *Service) RemovePackagedProviders(commit func() ([]string, error)) error {
	s.publicationMu.Lock()
	defer s.publicationMu.Unlock()
	generation := s.registry.Generation()
	ids, err := commit()
	if err != nil {
		return err
	}
	s.registry.ForgetGeneration(ids, generation)
	return nil
}
