package questionnaire

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTextoSemChaveViajaComoStringNoPayload(t *testing.T) {
	// Quem ainda não migrou continua mandando texto puro, e o payload precisa
	// continuar sendo o de sempre para quem o lê do outro lado.
	data, err := json.Marshal(Plain("Confirmar execução de comando"))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got, want := string(data), `"Confirmar execução de comando"`; got != want {
		t.Errorf("payload = %s, quer %s", got, want)
	}
}

func TestTextoComChaveLevaChaveParametrosEFallback(t *testing.T) {
	data, err := json.Marshal(KeyedWith(
		"app.questionnaire.network.title",
		map[string]any{"host": "interno.local"},
		`Autorizar acesso a "interno.local"`,
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("payload não é objeto: %v", err)
	}
	if payload["key"] != "app.questionnaire.network.title" {
		t.Errorf("chave = %v, quer a do locale", payload["key"])
	}
	if payload["fallback"] != `Autorizar acesso a "interno.local"` {
		t.Errorf("fallback = %v, quer o texto pronto", payload["fallback"])
	}
	params, _ := payload["params"].(map[string]any)
	if params["host"] != "interno.local" {
		t.Errorf("params = %v, quer o host para a tradução interpolar", payload["params"])
	}
}

func TestPayloadDeDialogoAceitaTextoENaoSoChave(t *testing.T) {
	// A tool collect_responses recebe as perguntas do modelo em JSON, com
	// prompt e opções como strings.
	var q Question
	entrada := `{"id":"q1","type":"single_choice","prompt":"Qual opção?","options":["Uma","Outra"],"placeholder":"digite"}`
	if err := json.Unmarshal([]byte(entrada), &q); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if q.Prompt.String() != "Qual opção?" {
		t.Errorf("prompt = %q, quer o texto que veio", q.Prompt.String())
	}
	if q.Prompt.Key != "" {
		t.Errorf("prompt ganhou chave %q sozinho", q.Prompt.Key)
	}
	if got := TextValues(q.Options); len(got) != 2 || got[0] != "Uma" || got[1] != "Outra" {
		t.Errorf("opções = %q, quer as duas que vieram", got)
	}
	if q.Placeholder.String() != "digite" {
		t.Errorf("placeholder = %q, quer o texto que veio", q.Placeholder.String())
	}
}

func TestPerguntaDeForaNaoTrazChaveDeTraducao(t *testing.T) {
	// Chave é decisão do app. Vinda de fora, ela exibiria texto de outro lugar
	// — ou nada, se não existisse no locale.
	perguntas := PlainQuestions([]Question{{
		ID:          "q1",
		Type:        "single_choice",
		Prompt:      Keyed("menu.settings", "Configurações"),
		Description: Keyed("qualquer.chave", "Descrição"),
		Placeholder: Keyed("outra.chave", "Exemplo"),
		Options:     []Text{Keyed("common.cancel", "Cancelar")},
	}})

	q := perguntas[0]
	if q.Prompt.Key != "" || q.Description.Key != "" || q.Placeholder.Key != "" {
		t.Errorf("a pergunta manteve chaves de tradução: %+v", q)
	}
	if q.Options[0].Key != "" {
		t.Errorf("a opção manteve a chave %q", q.Options[0].Key)
	}
	if q.Prompt.String() != "Configurações" || q.Options[0].String() != "Cancelar" {
		t.Errorf("o texto visível se perdeu: %+v", q)
	}
}

func TestTextoSemFallbackAindaDizAlgo(t *testing.T) {
	// Superfície sem tradução (canal) e valor de opção não podem ficar vazios:
	// um diálogo sem rótulo não é decidível, muito menos por leitor de telas.
	if got := Keyed("app.questionnaire.shell.submit", "").String(); got == "" {
		t.Error("texto sem fallback virou string vazia")
	}
}

func TestOEventoDoDialogoLevaOsTextosDeCadaCampo(t *testing.T) {
	var recebido map[string]any
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		if event != EventQuestionnaire {
			return
		}
		recebido, _ = data.(map[string]any)
		id, _ := recebido["id"].(string)
		go func() { _ = mgr.Respond(id, map[string]any{"q1": "ok"}, false) }()
	})

	if _, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Title:       Keyed("app.questionnaire.shell.title", "Confirmar execução de comando"),
		Description: Plain("O assistente quer executar: ls"),
		SubmitLabel: Keyed("app.questionnaire.shell.submit", "Permitir"),
		CancelLabel: Keyed("app.questionnaire.shell.cancel", "Negar"),
		Questions: []Question{{
			ID:     "q1",
			Type:   "text",
			Prompt: Keyed("app.questionnaire.shell.prompt", "Permitir a execução deste comando?"),
		}},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	titulo, ok := recebido["title"].(Text)
	if !ok {
		t.Fatalf("título veio como %T, quer questionnaire.Text", recebido["title"])
	}
	if titulo.Key != "app.questionnaire.shell.title" || titulo.Fallback == "" {
		t.Errorf("título = %+v, quer chave e fallback", titulo)
	}
	if rotulo, _ := recebido["submitLabel"].(Text); rotulo.Key == "" {
		t.Errorf("rótulo de confirmar = %+v, quer a chave de tradução", rotulo)
	}
	perguntas, _ := recebido["questions"].([]Question)
	if len(perguntas) != 1 || perguntas[0].Prompt.Key == "" {
		t.Errorf("perguntas = %+v, quer o prompt com chave", perguntas)
	}
}
