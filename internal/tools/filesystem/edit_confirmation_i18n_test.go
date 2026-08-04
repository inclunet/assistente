package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/questionnaire"
)

// exigirChaveEFallback cobra de cada campo visível do diálogo as duas metades do
// contrato (AEP-0085 D3): sem chave o texto sai em português para quem lê em
// outro idioma, e sem o texto pronto ele sai em branco se a chave faltar num
// locale — e diálogo em branco não é decidível, muito menos por leitor de telas.
func exigirChaveEFallback(t *testing.T, campos map[string]questionnaire.Text) {
	t.Helper()
	for nome, texto := range campos {
		if texto.Key == "" {
			t.Errorf("%s = %+v, quer chave de tradução", nome, texto)
		}
		if texto.Fallback == "" {
			t.Errorf("%s = %+v, quer o texto pronto para quem não traduz", nome, texto)
		}
	}
}

// exigirTextoDeAntes cobra que o texto pronto seja o de sempre: quem não traduz
// continua lendo a confirmação exatamente como ela era.
func exigirTextoDeAntes(t *testing.T, campos map[string]questionnaire.Text, quer map[string]string) {
	t.Helper()
	for nome, esperado := range quer {
		if got := campos[nome].Fallback; got != esperado {
			t.Errorf("%s = %q, quer o texto de antes %q", nome, got, esperado)
		}
	}
}

// confirmacaoDeEdicao monta a confirmação Antes/Depois como ela chegaria à tela.
func confirmacaoDeEdicao(t *testing.T, title, description questionnaire.Text, before, after string) questionnaire.RequestPayload {
	t.Helper()
	quest := &fakeQuestionnaireRequester{}
	if ok, result := confirmEditWithDiff(context.Background(), quest, title, description, before, after); !ok {
		t.Fatalf("a confirmação devolveu rejeição sem que ninguém rejeitasse: %+v", result)
	}
	if !quest.called {
		t.Fatal("a confirmação de edição não chegou à tela")
	}
	return quest.lastPayload
}

// camposVisiveis reúne todo texto do payload que a pessoa lê, com o nome pelo
// qual o teste o cobra.
func camposVisiveis(t *testing.T, payload questionnaire.RequestPayload) map[string]questionnaire.Text {
	t.Helper()
	campos := map[string]questionnaire.Text{
		"title":       payload.Title,
		"description": payload.Description,
		"submitLabel": payload.SubmitLabel,
		"cancelLabel": payload.CancelLabel,
	}
	for _, pergunta := range payload.Questions {
		campos["rótulo de "+pergunta.ID] = pergunta.Prompt
	}
	if payload.RejectReason == nil {
		t.Fatal("a confirmação foi para a tela sem o campo de motivo da rejeição")
	}
	campos["rótulo do motivo"] = payload.RejectReason.Label
	campos["placeholder do motivo"] = payload.RejectReason.Placeholder
	return campos
}

func TestAConfirmacaoDeEdicaoVaiTraduzivelParaATela(t *testing.T) {
	payload := confirmacaoDeEdicao(t, editConfirmTitle(), confirmDescriptionForPath("doc.md"), "antes", "depois")

	campos := camposVisiveis(t, payload)
	exigirChaveEFallback(t, campos)
	exigirTextoDeAntes(t, campos, map[string]string{
		"title":                 "Confirmar edição",
		"submitLabel":           "Aplicar",
		"cancelLabel":           "Rejeitar",
		"rótulo de before":      "Antes",
		"rótulo de after":       "Depois",
		"rótulo do motivo":      "Motivo da rejeição (opcional)",
		"placeholder do motivo": "Explique o que deveria ser diferente para o assistente propor nova versão",
	})
}

// Nenhum parâmetro pode se chamar como os reservados do i18next: caindo na
// interpolação, count mudaria a pluralização, context escolheria outra variante e
// lng trocaria o idioma da frase (AEP-0085 D2).
func TestOsParametrosDaConfirmacaoFogemDosNomesReservados(t *testing.T) {
	payload := confirmacaoDeEdicao(t,
		editConfirmTitle(),
		editConfirmDescription("", "doc.md", "por clareza"),
		"antes", "depois",
	)

	for nome, texto := range camposVisiveis(t, payload) {
		for _, reservado := range []string{"count", "context", "lng"} {
			if _, usado := texto.Params[reservado]; usado {
				t.Errorf("%s interpola %q, que o i18next reserva", nome, reservado)
			}
		}
	}
}

// Alterar um trecho e substituir o arquivo inteiro são decisões diferentes, e o
// título é o que diz qual delas está na tela. Chaves distintas para que a
// tradução não conte uma pela outra.
func TestOsTitulosDaConfirmacaoDistinguemEdicaoDeSobrescrita(t *testing.T) {
	edicao := editConfirmTitle()
	sobrescrita := overwriteConfirmTitle()

	exigirChaveEFallback(t, map[string]questionnaire.Text{
		"título da edição":      edicao,
		"título da sobrescrita": sobrescrita,
	})
	if edicao.Key == sobrescrita.Key {
		t.Errorf("chave = %q nos dois, quer distinguir a sobrescrita da edição", edicao.Key)
	}
	exigirTextoDeAntes(t, map[string]questionnaire.Text{
		"título da edição":      edicao,
		"título da sobrescrita": sobrescrita,
	}, map[string]string{
		"título da edição":      "Confirmar edição",
		"título da sobrescrita": "Confirmar sobrescrita",
	})
}

// O caminho do arquivo vai interpolado, nunca na chave: não existe tradução para
// o nome de um arquivo, e uma chave por caminho não existiria em locale nenhum
// (AEP-0085 D6).
func TestOCaminhoDoArquivoVaiInterpoladoENaoNaChave(t *testing.T) {
	descricao := confirmDescriptionForPath("notas/doc.md")

	if strings.Contains(descricao.Key, "doc.md") {
		t.Errorf("chave = %q, carrega o caminho do arquivo", descricao.Key)
	}
	if got := descricao.Params["path"]; got != "notas/doc.md" {
		t.Errorf("caminho nos params = %v, quer o do pedido", got)
	}
	if !strings.Contains(descricao.Fallback, `"notas/doc.md"`) {
		t.Errorf("texto pronto = %q, quer o caminho já no lugar", descricao.Fallback)
	}
}

// A justificativa do modelo se soma à frase padrão, e as duas formas moram no
// mesmo campo: é a chave que diz de qual delas se trata. Com uma chave só, quem
// traduz deixaria a justificativa de fora, e ela é o motivo da alteração.
func TestAJustificativaTrocaAChaveDaDescricao(t *testing.T) {
	semNota := editConfirmDescription("", "doc.md", "")
	comNota := editConfirmDescription("", "doc.md", "por clareza")

	exigirChaveEFallback(t, map[string]questionnaire.Text{
		"descrição sem justificativa": semNota,
		"descrição com justificativa": comNota,
	})
	if semNota.Key == comNota.Key {
		t.Errorf("chave = %q nas duas, quer distinguir a que traz a justificativa", semNota.Key)
	}
	if got := comNota.Params["notes"]; got != "por clareza" {
		t.Errorf("justificativa nos params = %v, quer a que o modelo escreveu", got)
	}
	if got := comNota.Params["path"]; got != "doc.md" {
		t.Errorf("caminho nos params = %v, quer o do pedido", got)
	}
	if !strings.HasSuffix(comNota.Fallback, "\n\npor clareza") {
		t.Errorf("texto pronto = %q, quer a justificativa no fim, como antes", comNota.Fallback)
	}
	if strings.Contains(semNota.Fallback, "por clareza") {
		t.Errorf("texto pronto = %q, traz justificativa que o modelo não escreveu", semNota.Fallback)
	}
}

// Título e descrição que o modelo escreve são conteúdo, nunca chave: aceitar
// chave de fora faria o diálogo exibir texto de outro lugar do app — ou nada, se
// a chave não existisse no locale (AEP-0085 D6).
func TestOTituloEADescricaoDoModeloNaoViramChave(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(filePath, []byte("antes"), 0644); err != nil {
		t.Fatalf("erro ao preparar o arquivo: %v", err)
	}

	quest := &fakeQuestionnaireRequester{}
	tool := NewTextEdit(dir, quest)
	args := `{"original": "antes", "replacement": "depois", "title": "Revisar parágrafo", "description": "Trocar o parágrafo de abertura.", "notes": "ficou mais direto"}`
	if result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args)); err != nil {
		t.Fatalf("Execute devolveu erro: %v", err)
	} else if result.IsError {
		t.Fatalf("Execute devolveu erro: %s", result.Content)
	}

	if got := quest.lastPayload.Title; got.Key != "" {
		t.Errorf("título = %+v, ganhou chave de tradução: o texto é do modelo", got)
	} else if got.Fallback != "Revisar parágrafo" {
		t.Errorf("título = %q, quer o que o modelo escreveu", got.Fallback)
	}

	descricao := quest.lastPayload.Description
	if descricao.Key != "" {
		t.Errorf("descrição = %+v, ganhou chave de tradução: o texto é do modelo", descricao)
	}
	if descricao.Fallback != "Trocar o parágrafo de abertura.\n\nficou mais direto" {
		t.Errorf("descrição = %q, quer a do modelo com a justificativa, como antes", descricao.Fallback)
	}
}

// O diff é o texto do arquivo: continua indo cru como conteúdo de bloco, e não
// como texto traduzível. Um trecho que pareça chave de tradução não pode ser
// resolvido como uma — o que a pessoa revisa tem de ser o que está no arquivo.
func TestODiffContinuaSendoConteudoDeBloco(t *testing.T) {
	before := "app.questionnaire.editConfirmation.submit"
	after := "linha nova\ncom quebra"

	payload := confirmacaoDeEdicao(t, editConfirmTitle(), confirmDescriptionForPath("doc.md"), before, after)

	conteudo := make(map[string]string, len(payload.Questions))
	for _, pergunta := range payload.Questions {
		conteudo[pergunta.ID] = pergunta.Content
	}
	if conteudo["before"] != before {
		t.Errorf("bloco Antes = %q, quer o texto do arquivo %q", conteudo["before"], before)
	}
	if conteudo["after"] != after {
		t.Errorf("bloco Depois = %q, quer o texto do arquivo %q", conteudo["after"], after)
	}
}
