package profileadequacy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/tools"
)

type fakeProfileStore struct {
	infos    []profiles.ProfileInfo
	profiles map[string]*profiles.Profile
}

func (f *fakeProfileStore) List() ([]profiles.ProfileInfo, error) {
	return f.infos, nil
}

func (f *fakeProfileStore) Get(slug string) (*profiles.Profile, error) {
	profile, ok := f.profiles[slug]
	if !ok {
		return nil, errors.New("not found")
	}
	return profile, nil
}

type fakeProviderStore struct {
	provider llm.ChatProvider
}

func (f fakeProviderStore) GetChatProvider(context.Context, string) (llm.ChatProvider, error) {
	return f.provider, nil
}
func (f fakeProviderStore) ResolveProfileDefaults(_ context.Context, profile *profiles.Profile) *profiles.Profile {
	return profile
}
func (f fakeProviderStore) UsesAgentTurn(context.Context, *profiles.Profile) bool { return false }

type fakeProvider struct {
	response string
	err      error
}

func (f *fakeProvider) StreamChat(context.Context, []llm.Message, llm.ChatParams, llm.StreamHandler, ...llm.ToolDefinition) {
}
func (f *fakeProvider) SendChat(context.Context, []llm.Message, llm.ChatParams) (string, error) {
	return "", nil
}
func (f *fakeProvider) GetModels(context.Context) ([]string, error) { return nil, nil }
func (f *fakeProvider) SimpleChat(context.Context, string, string, string) (string, error) {
	return f.response, f.err
}
func (f *fakeProvider) NativeMCPCapable() bool { return false }
func (f *fakeProvider) WithMCPServers([]llm.MCPServerConfig) llm.ChatProvider {
	return f
}

type fakeTool struct {
	name string
}

func (f fakeTool) Name() string                { return f.name }
func (f fakeTool) Description() string         { return "tool " + f.name }
func (f fakeTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f fakeTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func TestAdvisorRecommendsUniqueProfileWithBetterPreloadedCoverage(t *testing.T) {
	advisor, current := testAdvisor(t, `{"required_tools":["run_command"],"confidence":0.95}`, map[string]map[string]string{
		"padrao":      {"run_command": "on_demand"},
		"programacao": {"run_command": "preloaded"},
		"restrito":    {"run_command": "disabled"},
	})

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "compile e rode os testes",
		CurrentSlug:    "padrao",
		CurrentProfile: current,
		Model:          "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation == nil {
		t.Fatal("esperava recomendação")
	}
	if recommendation.SuggestedSlug != "programacao" {
		t.Fatalf("SuggestedSlug = %q", recommendation.SuggestedSlug)
	}
	if recommendation.CurrentCoverage != 0 || recommendation.SuggestedCoverage != 1 {
		t.Fatalf("cobertura inesperada: %#v", recommendation)
	}
}

func TestAdvisorDoesNotChooseArbitrarilyOnTie(t *testing.T) {
	advisor, current := testAdvisor(t, `{"required_tools":["run_command"],"confidence":0.95}`, map[string]map[string]string{
		"atual": {"run_command": "on_demand"},
		"um":    {"run_command": "preloaded"},
		"dois":  {"run_command": "preloaded"},
	})

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode os testes",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation != nil {
		t.Fatalf("empate não deveria recomendar: %#v", recommendation)
	}
}

func TestAdvisorKeepsAlreadyAdequateProfile(t *testing.T) {
	advisor, current := testAdvisor(t, `{"required_tools":["run_command"],"confidence":0.99}`, map[string]map[string]string{
		"atual": {"run_command": "preloaded"},
		"outro": {"run_command": "preloaded"},
	})

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode os testes",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation != nil {
		t.Fatalf("profile adequado não deveria trocar: %#v", recommendation)
	}
}

func TestAdvisorIgnoresUnknownClassifierTool(t *testing.T) {
	advisor, current := testAdvisor(t, `{"required_tools":["invented_tool"],"confidence":1}`, map[string]map[string]string{
		"atual": {"run_command": "on_demand"},
		"outro": {"run_command": "preloaded"},
	})

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "faça algo",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if err != nil || recommendation != nil {
		t.Fatalf("tool desconhecida deveria resultar em sem recomendação: recommendation=%#v err=%v", recommendation, err)
	}
}

func TestAdvisorIgnoresLowConfidence(t *testing.T) {
	advisor, current := testAdvisor(t, `{"required_tools":["run_command"],"confidence":0.79}`, map[string]map[string]string{
		"atual": {"run_command": "on_demand"},
		"outro": {"run_command": "preloaded"},
	})

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode talvez",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation != nil {
		t.Fatalf("baixa confiança não deveria recomendar: %#v", recommendation)
	}
}

func TestAdvisorUsesIndependentRateLimitBucketWithBurstOne(t *testing.T) {
	advisor, _ := testAdvisor(t, `{"required_tools":["run_command"],"confidence":1}`, map[string]map[string]string{
		"rate-atual": {"run_command": "on_demand"},
		"rate-outro": {"run_command": "preloaded"},
	})
	current := advisor.profiles.(*fakeProfileStore).profiles["rate-atual"]
	one := 1
	current.Chat.RateLimitBurst = one

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode os testes",
		CurrentSlug:    "rate-atual",
		CurrentProfile: current,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation == nil || recommendation.SuggestedSlug != "rate-outro" {
		t.Fatalf("bucket auxiliar não deveria consumir o slot do turno principal: %#v", recommendation)
	}
}

func TestAdvisorDoesNotCountToolDroppedBySchemaBudget(t *testing.T) {
	advisor, current := testAdvisor(t, `{"required_tools":["run_command"],"confidence":1}`, map[string]map[string]string{
		"atual": {"run_command": "on_demand"},
		"outro": {"run_command": "preloaded"},
	})
	store := advisor.profiles.(*fakeProfileStore)
	store.profiles["outro"].Chat.ToolSchemaBudgetBytes = 1

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode os testes",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recommendation != nil {
		t.Fatalf("tool removida pelo orçamento não pode contar como cobertura: %#v", recommendation)
	}
}

func TestAdvisorIgnoresMalformedOrExtendedClassifierOutput(t *testing.T) {
	for _, response := range []string{
		"```json\n{\"required_tools\":[],\"confidence\":1}\n```",
		`{"required_tools":[],"confidence":1,"profile":"programacao"}`,
		`{"required_tools":[],"confidence":1} {"required_tools":[],"confidence":1}`,
	} {
		advisor, current := testAdvisor(t, response, map[string]map[string]string{
			"atual": {"run_command": "on_demand"},
			"outro": {"run_command": "preloaded"},
		})
		recommendation, err := advisor.Recommend(t.Context(), Request{
			UserContent:    "rode os testes",
			CurrentSlug:    "atual",
			CurrentProfile: current,
		})
		if err != nil || recommendation != nil {
			t.Fatalf("resposta inválida deveria resultar em sem recomendação: %q recommendation=%#v err=%v", response, recommendation, err)
		}
	}
}

func TestAdvisorReportsUnsupportedAuxiliaryRole(t *testing.T) {
	advisor, current := testAdvisor(t, "", map[string]map[string]string{
		"atual": {"run_command": "on_demand"},
		"outro": {"run_command": "preloaded"},
	})
	advisor.providers = fakeProviderStore{provider: &fakeProvider{err: llm.ErrACPAuxiliaryRole}}

	_, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode os testes",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if !errors.Is(err, ErrAuxiliaryClassificationUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestAdvisorIgnoresClassifierTimeout(t *testing.T) {
	advisor, current := testAdvisor(t, "", map[string]map[string]string{
		"atual": {"run_command": "on_demand"},
		"outro": {"run_command": "preloaded"},
	})
	advisor.providers = fakeProviderStore{provider: &fakeProvider{err: context.DeadlineExceeded}}

	recommendation, err := advisor.Recommend(t.Context(), Request{
		UserContent:    "rode os testes",
		CurrentSlug:    "atual",
		CurrentProfile: current,
	})
	if err != nil || recommendation != nil {
		t.Fatalf("timeout deveria resultar em sem recomendação: recommendation=%#v err=%v", recommendation, err)
	}
}

func testAdvisor(
	t *testing.T,
	classifierResponse string,
	policies map[string]map[string]string,
) (*Advisor, *profiles.Profile) {
	t.Helper()
	registry := tools.NewRegistry()
	registry.MustRegister(fakeTool{name: "run_command"})
	registry.MustRegister(fakeTool{name: tools.ToolCatalogName})

	store := &fakeProfileStore{profiles: make(map[string]*profiles.Profile)}
	for slug, policy := range policies {
		profile := &profiles.Profile{
			Name: slug,
			Chat: profiles.ChatConfig{
				LLMProvider:       "provider",
				ToolPolicy:        policy,
				ToolPolicyDefault: "disabled",
			},
		}
		store.profiles[slug] = profile
		store.infos = append(store.infos, profiles.ProfileInfo{Name: slug, Slug: slug})
	}
	current := store.profiles[firstSlug(policies)]
	advisor := NewAdvisor(store, fakeProviderStore{provider: &fakeProvider{response: classifierResponse}}, registry)
	return advisor, current
}

func firstSlug(policies map[string]map[string]string) string {
	for _, preferred := range []string{"padrao", "atual"} {
		if _, ok := policies[preferred]; ok {
			return preferred
		}
	}
	for slug := range policies {
		return slug
	}
	return ""
}
