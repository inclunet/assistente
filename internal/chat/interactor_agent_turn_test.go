package chat

import (
	"context"
	"testing"

	"assistente/internal/contextprovider"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/slashskill"
)

// interactorDeAgente monta o preparo do turno com tudo o que costuma entrar no
// prompt — construtor, provedores de contexto e skills —, para o teste provar
// que nada disso é usado quando quem conduz o turno é um agente de código.
func interactorDeAgente(t *testing.T, em *spyEmitter) (*Interactor, *capturingPromptBuilder) {
	t.Helper()
	builder := &capturingPromptBuilder{}
	helper := &skills.Skill{
		SkillMetadata: skills.SkillMetadata{Name: "helper", DisplayName: "Helper", Description: "Ajuda"},
		Slug:          "helper",
		Content:       "instruções da skill",
	}
	interactor := NewInteractor(InteractorConfig{
		Emitter:          em,
		PromptBuilder:    builder,
		ContextProviders: contextprovider.NewRegistry(slashskill.NewContextProvider()),
		SkillMgr: staticSkillRuntimeManager{
			skills: map[string]*skills.Skill{"helper": helper},
		},
	})
	return interactor, builder
}

func perfilComSkill() *profiles.Profile {
	profile := &profiles.Profile{}
	profile.Chat.EnabledSkills = []string{"helper"}
	return profile
}

// O turno do agente sai daqui como entrou: sem mensagem de sistema e sem bloco
// de contexto nenhum (AEP-0084, Fase 8).
func TestTurnoDeAgenteNaoMontaPromptDoApp(t *testing.T) {
	em := &spyEmitter{}
	interactor, builder := interactorDeAgente(t, em)

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:            []llm.Message{{Role: "user", Content: "arruma o CSS"}},
		UserContent:         "arruma o CSS",
		ConversationSummary: "até aqui falamos de acessibilidade",
		ConversationID:      "conv-1",
		TurnID:              "turn-1",
		ActiveProfile:       perfilComSkill(),
		AgentTurn:           true,
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages devolveu erro: %v", result.Err)
	}
	if len(builder.messages) != 0 || len(builder.contextBlocks) != 0 {
		t.Errorf("o construtor de prompt foi usado num turno de agente: %+v", builder.contextBlocks)
	}
	if len(result.Messages) != 1 || result.Messages[0].Role != "user" {
		t.Fatalf("mensagens = %+v, esperava só a da pessoa", result.Messages)
	}
	if result.Messages[0].GetContentAsString() != "arruma o CSS" {
		t.Errorf("a mensagem chegou alterada: %q", result.Messages[0].GetContentAsString())
	}
}

// Num perfil de agente o menu da barra é dele: `/algo` vai como texto, sem o
// app carregar skill nem anunciar carregamento de skill.
func TestTurnoDeAgenteNaoProcessaSkillDeBarra(t *testing.T) {
	em := &spyEmitter{}
	interactor, builder := interactorDeAgente(t, em)

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "/helper agora"}},
		UserContent:    "/helper agora",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
		ActiveProfile:  perfilComSkill(),
		AgentTurn:      true,
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages devolveu erro: %v", result.Err)
	}
	if result.InvokedSkillSlug != "" {
		t.Errorf("skill %q foi invocada num turno de agente", result.InvokedSkillSlug)
	}
	if em.findSkillLoaded() != nil {
		t.Error("o app anunciou carregamento de skill num turno de agente")
	}
	if builder.slashSkillContent() != "" {
		t.Errorf("o conteúdo da skill foi injetado: %q", builder.slashSkillContent())
	}
	if result.Messages[0].GetContentAsString() != "/helper agora" {
		t.Errorf("o comando chegou alterado ao agente: %q", result.Messages[0].GetContentAsString())
	}
}

// A mesma mensagem num perfil comum continua carregando a skill: a exceção é do
// turno de agente, e não do preparo inteiro.
func TestTurnoComumContinuaProcessandoSkillDeBarra(t *testing.T) {
	em := &spyEmitter{}
	interactor, builder := interactorDeAgente(t, em)

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "/helper agora"}},
		UserContent:    "/helper agora",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
		ActiveProfile:  perfilComSkill(),
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages devolveu erro: %v", result.Err)
	}
	if result.InvokedSkillSlug != "helper" {
		t.Errorf("skill invocada = %q, esperava helper", result.InvokedSkillSlug)
	}
	if builder.slashSkillContent() == "" {
		t.Error("o conteúdo da skill não foi injetado no turno comum")
	}
}
