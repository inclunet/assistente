package usecases

import (
	"context"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// O interruptor vale para o turno, não para o perfil salvo: quem trocar o perfil
// de volta para um provedor comum tem as tools de novo, sem reconfigurar nada.
func TestTurnoDeAgenteDesligaAsToolsSemMexerNoPerfilSalvo(t *testing.T) {
	salvo := &profiles.Profile{Chat: profiles.ChatConfig{LLMProvider: "cursor"}}

	doTurno := profileWithToolsDisabled(salvo)

	if !doTurno.Chat.DisableTools {
		t.Error("o turno de agente deveria ser planejado com as tools desligadas")
	}
	if salvo.Chat.DisableTools {
		t.Error("o perfil salvo não podia ter sido alterado")
	}
	if doTurno == salvo {
		t.Error("esperava uma cópia, não o mesmo perfil")
	}
}

func TestPerfilJaSemToolsAtravessaInteiro(t *testing.T) {
	salvo := &profiles.Profile{Chat: profiles.ChatConfig{DisableTools: true}}

	if doTurno := profileWithToolsDisabled(salvo); doTurno != salvo {
		t.Error("perfil que já desliga tools não precisa de cópia")
	}
	if profileWithToolsDisabled(nil) != nil {
		t.Error("perfil ausente continua ausente")
	}
}

// O critério do D7 é o conjunto FINAL que chega ao roteamento: uma tool
// sobrevivente — inclusive as de runtime, que entram fora do perfil — mandaria o
// turno do agente para o loop agêntico do app.
func TestConjuntoFinalDoTurnoDeAgenteChegaVazioAoRoteamento(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(testTool{name: "regular_tool"})
	policy := chat.NewToolSelectionPolicy(registry)

	perfil := &profiles.Profile{Chat: profiles.ChatConfig{
		LLMProvider:  "cursor",
		EnabledTools: []string{"regular_tool"},
	}}
	cfgDoTurno := func(p *profiles.Profile) chat.ProfileToolConfig {
		return chat.ProfileToolConfig{
			EnabledTools:      p.Chat.EnabledTools,
			ToolPolicy:        p.Chat.ToolPolicy,
			ToolPolicyDefault: p.Chat.ToolPolicyDefault,
			DisableTools:      p.Chat.DisableTools,
			RuntimeTools:      []string{"regular_tool"},
		}
	}

	turnPlan1 := policy.PlanTurnToolDefs(context.Background(), nil, nil, cfgDoTurno(perfil))

	nativas, adapter := turnPlan1.Definitions, turnPlan1.AdapterDefinitions
	if len(nativas) == 0 && len(adapter) == 0 {
		t.Fatal("sem a guarda o perfil deveria produzir tools — o teste não provaria nada")
	}

	turnPlan0 := policy.PlanTurnToolDefs(context.Background(), nil, nil, cfgDoTurno(profileWithToolsDisabled(perfil)))

	nativas, adapter = turnPlan0.Definitions, turnPlan0.AdapterDefinitions
	if len(nativas) > 0 || len(adapter) > 0 {
		t.Errorf("turno de agente chegou ao roteamento com tools: %d nativas, %d adapter", len(nativas), len(adapter))
	}
}

func TestDelegatedRunRequiresControllableProvider(t *testing.T) {
	for _, tc := range []struct {
		provider           llm.APIFormat
		tools              []string
		wantAgent, wantErr bool
	}{
		{llm.APIFormatACP, nil, true, false},
		{llm.APIFormatACP, []string{}, false, true},
		{llm.APIFormatACP, []string{"read_file"}, false, true},
		{llm.APIFormatOpenAI, []string{"read_file"}, false, false},
	} {
		ctx := invocationctx.WithDelegatedTools(t.Context(), tc.tools)
		agent, err := agentTurnForDelegation(&llm.ProviderConfig{APIFormat: tc.provider}, invocationctx.DelegatedToolsFrom(ctx))
		if agent != tc.wantAgent || (err != nil) != tc.wantErr {
			t.Errorf("%v %v: agente=%v, erro=%v", tc.provider, tc.tools, agent, err)
		}
	}
}

func TestPluginDelegationRejectsIncompatibleModel(t *testing.T) {
	for _, tc := range []struct{ provider, model, requested string }{
		{"other", "aba", ""}, {"provider", "aba", "diferente"}, {"", "", ""},
	} {
		if _, err := delegatedModel(tc.provider, tc.model, "provider", tc.requested); err == nil {
			t.Fatal(tc)
		}
	}
}
