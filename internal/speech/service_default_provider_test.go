package speech

import (
	"context"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// recordingRegistry registra os IDs consultados em Get, permitindo verificar
// que o serviço nunca pede o literal "$default" ao registro de provedores.
type recordingRegistry struct {
	requested []string
	configs   map[string]*llm.ProviderConfig
}

func (r *recordingRegistry) Get(id string) *llm.ProviderConfig {
	r.requested = append(r.requested, id)
	return r.configs[id]
}

func (r *recordingRegistry) List() []*llm.ProviderConfig { return nil }

// stubProfileProvider resolve o sentinela "$default" para um ID fixo, imitando
// providers.Service.ResolveProfileDefaults.
type stubProfileProvider struct {
	resolveTo string // ID do provider default; vazio => não resolve
}

func (s *stubProfileProvider) GetActive() (*profiles.Profile, error) {
	return &profiles.Profile{}, nil
}

func (s *stubProfileProvider) ResolveDefaults(_ context.Context, p *profiles.Profile) *profiles.Profile {
	if p == nil {
		return nil
	}
	resolved := *p
	if resolved.Chat.LLMProvider == profiles.DefaultProviderSentinel && s.resolveTo != "" {
		resolved.Chat.LLMProvider = s.resolveTo
	}
	return &resolved
}

func newServiceForResolution(reg ProviderRegistry, pp ProfileProvider) *Service {
	return NewService(ServiceConfig{Registry: reg, ProfileProvider: pp})
}

// TestGetTTSModels_ResolveDefaultSentinel garante que "$default" é resolvido
// para o provider concreto ANTES de consultar o registro (regressão do
// "não foi possível criar client para provider $default" no assistente.log).
func TestGetTTSModels_ResolveDefaultSentinel(t *testing.T) {
	reg := &recordingRegistry{configs: map[string]*llm.ProviderConfig{}}
	svc := newServiceForResolution(reg, &stubProfileProvider{resolveTo: "openai-real"})

	_ = svc.GetTTSModels(context.Background(), profiles.DefaultProviderSentinel)

	if len(reg.requested) != 1 || reg.requested[0] != "openai-real" {
		t.Fatalf("registro consultado com %v; esperado apenas [openai-real]", reg.requested)
	}
	for _, id := range reg.requested {
		if id == profiles.DefaultProviderSentinel {
			t.Fatalf("registro consultado com o sentinela %q (não resolvido)", id)
		}
	}
}

// TestGetTTSVoices_ResolveDefaultSentinel cobre o mesmo para vozes.
func TestGetTTSVoices_ResolveDefaultSentinel(t *testing.T) {
	reg := &recordingRegistry{configs: map[string]*llm.ProviderConfig{}}
	svc := newServiceForResolution(reg, &stubProfileProvider{resolveTo: "openai-real"})

	_ = svc.GetTTSVoices(context.Background(), profiles.DefaultProviderSentinel, "gpt-4o-mini-tts")

	if len(reg.requested) != 1 || reg.requested[0] != "openai-real" {
		t.Fatalf("registro consultado com %v; esperado apenas [openai-real]", reg.requested)
	}
}

// TestGetTTSModels_DefaultNaoResolvidoNaoConsultaRegistro garante que, quando o
// sentinela não pode ser resolvido, o serviço degrada sem pedir "$default" ao
// registro.
func TestGetTTSModels_DefaultNaoResolvidoNaoConsultaRegistro(t *testing.T) {
	reg := &recordingRegistry{configs: map[string]*llm.ProviderConfig{}}
	svc := newServiceForResolution(reg, &stubProfileProvider{resolveTo: ""})

	models := svc.GetTTSModels(context.Background(), profiles.DefaultProviderSentinel)

	if len(models) != 0 {
		t.Fatalf("esperado nenhum modelo, got %d", len(models))
	}
	if len(reg.requested) != 0 {
		t.Fatalf("registro não deveria ser consultado; got %v", reg.requested)
	}
}

// TestGetTTSModels_IDConcretoPassaIntacto garante que IDs concretos não são
// afetados pela resolução do sentinela.
func TestGetTTSModels_IDConcretoPassaIntacto(t *testing.T) {
	reg := &recordingRegistry{configs: map[string]*llm.ProviderConfig{}}
	svc := newServiceForResolution(reg, &stubProfileProvider{resolveTo: "openai-real"})

	_ = svc.GetTTSModels(context.Background(), "anthropic-custom")

	if len(reg.requested) != 1 || reg.requested[0] != "anthropic-custom" {
		t.Fatalf("registro consultado com %v; esperado [anthropic-custom]", reg.requested)
	}
}
