package usecases

import (
	"testing"

	"assistente/internal/chat"
	"assistente/internal/profiles"
	"assistente/internal/tools"
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

	perfil := &profiles.Profile{Chat: profiles.ChatConfig{LLMProvider: "cursor"}}
	cfgDoTurno := func(p *profiles.Profile) chat.ProfileToolConfig {
		return chat.ProfileToolConfig{
			EnabledTools:      p.Chat.EnabledTools,
			ToolPolicy:        p.Chat.ToolPolicy,
			ToolPolicyDefault: p.Chat.ToolPolicyDefault,
			DisableTools:      p.Chat.DisableTools,
			RuntimeTools:      []string{"regular_tool"},
		}
	}

	_, nativas, adapter := policy.PlanTurnToolDefs(nil, nil, cfgDoTurno(perfil))
	if len(nativas) == 0 && len(adapter) == 0 {
		t.Fatal("sem a guarda o perfil deveria produzir tools — o teste não provaria nada")
	}

	_, nativas, adapter = policy.PlanTurnToolDefs(nil, nil, cfgDoTurno(profileWithToolsDisabled(perfil)))
	if len(nativas) > 0 || len(adapter) > 0 {
		t.Errorf("turno de agente chegou ao roteamento com tools: %d nativas, %d adapter", len(nativas), len(adapter))
	}
}
