package app

import (
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/questionnaire"
)

// dialogoDeExtensao apresenta um pedido bloqueante do agente e devolve o payload
// que chegaria à tela, com a primeira opção marcada.
func dialogoDeExtensao(t *testing.T, req acp.CustomRequest) map[string]any {
	t.Helper()
	tela := novaTelaDeExtensao(escolhendoAPrimeira())
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	mustHandle(t, h, req)
	return tela.ultimoDialogo(t)
}

// textoDaPergunta lê o rótulo de um item do diálogo pelo identificador.
func textoDaPergunta(t *testing.T, payload map[string]any, id string) questionnaire.Text {
	t.Helper()
	for _, pergunta := range perguntasDe(payload) {
		if pergunta.ID == id {
			return pergunta.Prompt
		}
	}
	t.Fatalf("o diálogo não trouxe o item %q", id)
	return questionnaire.Text{}
}

func TestAPerguntaDoAgenteVaiTraduzivelParaATela(t *testing.T) {
	payload := dialogoDeExtensao(t, pedidoDePergunta(t))

	exigirChaveEFallback(t, payload, "title", "description", "submitLabel", "cancelLabel")
	for campo, esperado := range map[string]string{
		"title":       "O agente tem uma pergunta",
		"submitLabel": "Responder",
		"cancelLabel": "Pular a pergunta",
	} {
		if got := textoDoDialogo(payload, campo); got != esperado {
			t.Errorf("%s = %q, quer o texto de antes %q", campo, got, esperado)
		}
	}
	exigirRotulosDasPerguntas(t, payload, map[string]string{
		askPromptPrefix + "0": "Pergunta do agente",
		askAnswerPrefix + "0": "Sua resposta",
	})
}

// O assunto é texto do agente: entra como valor interpolado, nunca na chave —
// chave é decisão do app (AEP-0085 D6). Sem assunto a frase é outra, e por isso
// tem chave própria: uma só deixaria "Assunto:" pendurado sem nada depois.
func TestOAssuntoDaPerguntaVaiInterpoladoENaoNaChave(t *testing.T) {
	comAssunto, _ := dialogoDeExtensao(t, pedidoDePergunta(t))["description"].(questionnaire.Text)

	if got := comAssunto.Params["subject"]; got != "Escolher a abordagem" {
		t.Errorf("assunto nos params = %v, quer o que o agente mandou", got)
	}
	if strings.Contains(comAssunto.Key, "Escolher") {
		t.Errorf("chave = %q, carrega texto do agente", comAssunto.Key)
	}
	if !strings.Contains(comAssunto.Fallback, `Assunto: "Escolher a abordagem".`) {
		t.Errorf("texto pronto = %q, quer o assunto já no lugar", comAssunto.Fallback)
	}

	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":      "q1",
				"prompt":  "Qual banco?",
				"options": []any{map[string]any{"id": "sqlite", "label": "SQLite"}},
			},
		},
	})
	semAssunto, _ := dialogoDeExtensao(t, pedido)["description"].(questionnaire.Text)

	if semAssunto.Key == comAssunto.Key {
		t.Errorf("chave = %q nas duas, quer distinguir a descrição que nomeia o assunto", semAssunto.Key)
	}
	if len(semAssunto.Params) != 0 {
		t.Errorf("params sem assunto = %v, quer nenhum", semAssunto.Params)
	}
	if strings.Contains(semAssunto.Fallback, "Assunto:") {
		t.Errorf("texto pronto = %q, anuncia um assunto que o agente não mandou", semAssunto.Fallback)
	}
}

// A posição do bloco é número, não assunto: vai em parâmetro, com o texto pronto
// já numerado. Os nomes fogem dos reservados do i18next (count, context, lng),
// que mudariam pluralização, contexto ou idioma da tradução (AEP-0085 D2).
func TestOsBlocosNumeradosLevamAPosicaoEmParametro(t *testing.T) {
	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":      "q1",
				"prompt":  "Primeira?",
				"options": []any{map[string]any{"id": "a", "label": "A"}},
			},
			map[string]any{
				"id":      "q2",
				"prompt":  "Segunda?",
				"options": []any{map[string]any{"id": "b", "label": "B"}},
			},
		},
	})
	payload := dialogoDeExtensao(t, pedido)

	primeiro := textoDaPergunta(t, payload, askPromptPrefix+"0")
	if primeiro.Key == "" {
		t.Fatalf("rótulo do primeiro bloco = %+v, quer chave de tradução", primeiro)
	}
	if primeiro.Params["position"] != 1 || primeiro.Params["total"] != 2 {
		t.Errorf("params = %v, quer a posição e o total do bloco", primeiro.Params)
	}
	if primeiro.Fallback != "Pergunta 1 de 2" {
		t.Errorf("texto pronto = %q, quer o de antes já numerado", primeiro.Fallback)
	}

	// Com uma pergunta só o rótulo não numera, e a chave é outra: numerar seria
	// ruído para quem ouve o diálogo.
	unico := textoDaPergunta(t, dialogoDeExtensao(t, pedidoDePergunta(t)), askPromptPrefix+"0")
	if unico.Key == primeiro.Key {
		t.Errorf("chave = %q nas duas, quer distinguir o bloco numerado", unico.Key)
	}
	if len(unico.Params) != 0 {
		t.Errorf("params do bloco único = %v, quer nenhum", unico.Params)
	}
}

// A múltipla escolha avisa no rótulo que dá para marcar mais de uma opção. É
// disso que depende quem só ouve o rótulo antes de responder, então cada forma
// da pergunta tem a sua chave.
func TestORotuloDaRespostaMudaDeChaveNaMultiplaEscolha(t *testing.T) {
	pedido := pedidoCom(t, methodAskQuestion, map[string]any{
		"toolCallId": "call-1",
		"questions": []any{
			map[string]any{
				"id":            "q1",
				"prompt":        "Quais bancos?",
				"allowMultiple": true,
				"options":       []any{map[string]any{"id": "a", "label": "A"}},
			},
		},
	})

	multipla := textoDaPergunta(t, dialogoDeExtensao(t, pedido), askAnswerPrefix+"0")
	unica := textoDaPergunta(t, dialogoDeExtensao(t, pedidoDePergunta(t)), askAnswerPrefix+"0")

	if multipla.Key == "" || unica.Key == "" {
		t.Fatalf("rótulos sem chave: múltipla %+v, única %+v", multipla, unica)
	}
	if multipla.Key == unica.Key {
		t.Errorf("chave = %q nas duas, quer distinguir a que aceita mais de uma", unica.Key)
	}
	if multipla.Fallback != "Sua resposta (pode marcar mais de uma)" {
		t.Errorf("texto pronto = %q, quer o de antes", multipla.Fallback)
	}
	if unica.Fallback != "Sua resposta" {
		t.Errorf("texto pronto = %q, quer o de antes", unica.Fallback)
	}
}

// O rótulo que o agente ofereceu é texto dele, já saneado, e nunca vira chave: a
// chave é decisão do app, e uma que viesse de fora exibiria o texto de outro
// lugar — ou nada, se não existisse no locale (AEP-0085 D6).
func TestOsRotulosDaPerguntaDoAgenteNaoViramChave(t *testing.T) {
	payload := dialogoDeExtensao(t, pedidoDePergunta(t))

	for _, pergunta := range perguntasDe(payload) {
		for _, opcao := range pergunta.Options {
			if opcao.Key != "" {
				t.Errorf("opção %+v ganhou chave de tradução: o texto é do agente", opcao)
			}
			if opcao.Fallback == "" {
				t.Error("opção sem texto: a pessoa escolheria um rótulo em branco")
			}
		}
	}
}

func TestOPlanoPropostoVaiTraduzivelParaATela(t *testing.T) {
	payload := dialogoDeExtensao(t, pedidoDePlano(t))

	exigirChaveEFallback(t, payload, "title", "description", "submitLabel", "cancelLabel")
	for campo, esperado := range map[string]string{
		"title":       "O agente propôs um plano",
		"submitLabel": "Confirmar",
		"cancelLabel": "Recusar",
	} {
		if got := textoDoDialogo(payload, campo); got != esperado {
			t.Errorf("%s = %q, quer o texto de antes %q", campo, got, esperado)
		}
	}
	exigirRotulosDasPerguntas(t, payload, map[string]string{
		planContentID: "Plano proposto",
		planAnswerID:  "O agente pode seguir este plano?",
	})
}

// As opções do plano são do app, e não do agente: ele manda o plano, não as
// escolhas. Ganham chave, mas o valor que volta em Answers continua sendo o
// texto pronto — é por ele que createPlan reencontra a decisão (AEP-0085 D5).
func TestAsOpcoesDoPlanoTemChaveESeguemValendoPeloTextoPronto(t *testing.T) {
	payload := dialogoDeExtensao(t, pedidoDePlano(t))

	for _, pergunta := range perguntasDe(payload) {
		if pergunta.ID != planAnswerID {
			continue
		}
		for _, opcao := range pergunta.Options {
			if opcao.Key == "" {
				t.Errorf("opção %+v sem chave: a decisão do plano ficaria só em português", opcao)
			}
		}
		valores := questionnaire.TextValues(pergunta.Options)
		quer := []string{planApproveLabel, planRejectLabel}
		if len(valores) != len(quer) {
			t.Fatalf("opções = %q, quer %q", valores, quer)
		}
		for i, valor := range valores {
			if valor != quer[i] {
				t.Errorf("valor da opção %d = %q, quer o que o backend reencontra (%q)", i, valor, quer[i])
			}
		}
	}
}

// A descrição do plano muda com o que o pedido traz — plano de projeto, contagem
// de passos. Como ela é um campo só, é a chave que diz qual das frases é; o
// número de passos vai interpolado, porque número não é assunto.
func TestADescricaoDoPlanoDistingueProjetoEContagemDePassos(t *testing.T) {
	descricao := func(campos map[string]any) questionnaire.Text {
		t.Helper()
		pedido := map[string]any{"toolCallId": "call-2", "name": "Plano", "plan": "Fazer algo"}
		for chave, valor := range campos {
			pedido[chave] = valor
		}
		texto, _ := dialogoDeExtensao(t, pedidoCom(t, methodCreatePlan, pedido))["description"].(questionnaire.Text)
		return texto
	}

	passos := []any{map[string]any{"id": "t1", "content": "Um passo", "status": "pending"}}
	simples := descricao(nil)
	comPassos := descricao(map[string]any{"todos": passos})
	deProjeto := descricao(map[string]any{"isProject": true})
	completo := descricao(map[string]any{"isProject": true, "todos": passos})

	chaves := map[string]questionnaire.Text{
		"simples": simples, "com passos": comPassos,
		"de projeto": deProjeto, "de projeto com passos": completo,
	}
	vistas := make(map[string]string, len(chaves))
	for nome, texto := range chaves {
		if texto.Key == "" {
			t.Errorf("descrição %s = %+v, quer chave de tradução", nome, texto)
			continue
		}
		if anterior, repetida := vistas[texto.Key]; repetida {
			t.Errorf("as descrições %q e %q dividem a chave %q: uma delas seria dita pela outra",
				anterior, nome, texto.Key)
		}
		vistas[texto.Key] = nome
	}

	if comPassos.Params["steps"] != 1 {
		t.Errorf("passos nos params = %v, quer quantos o plano tem", comPassos.Params["steps"])
	}
	if !strings.Contains(comPassos.Fallback, "1 passo(s)") {
		t.Errorf("texto pronto = %q, quer a contagem já no lugar", comPassos.Fallback)
	}
	if len(simples.Params) != 0 {
		t.Errorf("params do plano sem passos = %v, quer nenhum", simples.Params)
	}
	if !strings.Contains(deProjeto.Fallback, "de projeto") {
		t.Errorf("texto pronto = %q, quer dizer que o plano é de projeto", deProjeto.Fallback)
	}
}

// O desfecho não muda com a tradução: a pessoa aprova pelo valor estável, e o
// agente recebe o mesmo "accepted" de antes.
func TestOPlanoAprovadoContinuaAceitoDepoisDaMigracao(t *testing.T) {
	tela := novaTelaDeExtensao(respondendo(func([]string) any { return planApproveLabel }))
	h := handlerDeExtensao(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	resposta := respostaDoPlano(t, mustHandle(t, h, pedidoDePlano(t)))

	if resposta.Outcome.Outcome != planOutcomeAccepted {
		t.Errorf("desfecho = %q, quer %q", resposta.Outcome.Outcome, planOutcomeAccepted)
	}
}
