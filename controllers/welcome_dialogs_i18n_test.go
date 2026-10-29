package controllers

import (
	"testing"

	"assistente/internal/questionnaire"
)

// textosDoDialogo reúne todo texto visível de um payload, com o nome pelo qual o
// teste o cobra. Campo vazio fica de fora: o diálogo não o exibe.
func textosDoDialogo(payload questionnaire.RequestPayload) map[string]questionnaire.Text {
	campos := make(map[string]questionnaire.Text)
	incluir := func(nome string, texto questionnaire.Text) {
		if !texto.IsZero() {
			campos[nome] = texto
		}
	}

	incluir("title", payload.Title)
	incluir("description", payload.Description)
	incluir("hint", payload.Hint)
	incluir("submitLabel", payload.SubmitLabel)
	incluir("cancelLabel", payload.CancelLabel)
	for _, action := range payload.Actions {
		incluir("action:"+action.ID, action.Label)
	}
	for _, pergunta := range payload.Questions {
		incluir("prompt:"+pergunta.ID, pergunta.Prompt)
		incluir("placeholder:"+pergunta.ID, pergunta.Placeholder)
	}
	if payload.RejectReason != nil {
		incluir("rejectReason.label", payload.RejectReason.Label)
		incluir("rejectReason.placeholder", payload.RejectReason.Placeholder)
	}
	return campos
}

// dialogoEsperado descreve o contrato de um diálogo: o que se traduz e o que é
// conteúdo. Todo campo visível tem de estar num dos dois — um campo fora das
// listas é um texto que chegou à tela sem ninguém decidir se ele se traduz.
type dialogoEsperado struct {
	// traduzido: campo → texto pronto de antes. Exige chave (AEP-0085 D3).
	traduzido map[string]string
	// conteudo: campo → texto puro. Exige a ausência de chave (AEP-0085 D6).
	conteudo map[string]string
}

// exigirContratoDoDialogo cobra as duas metades do contrato de cada campo e a
// cobertura de todos eles.
func exigirContratoDoDialogo(t *testing.T, nome string, payload questionnaire.RequestPayload, quer dialogoEsperado) {
	t.Helper()
	campos := textosDoDialogo(payload)

	for campo, esperado := range quer.traduzido {
		texto, presente := campos[campo]
		if !presente {
			t.Errorf("%s: o campo %q não chegou à tela", nome, campo)
			continue
		}
		if texto.Key == "" {
			t.Errorf("%s: %s = %+v, quer chave de tradução", nome, campo, texto)
		}
		if texto.Fallback != esperado {
			t.Errorf("%s: %s = %q, quer o texto de antes %q", nome, campo, texto.Fallback, esperado)
		}
	}

	for campo, esperado := range quer.conteudo {
		texto, presente := campos[campo]
		if !presente {
			t.Errorf("%s: o campo %q não chegou à tela", nome, campo)
			continue
		}
		if texto.Key != "" {
			t.Errorf("%s: %s = %+v, ganhou chave de tradução: o texto é conteúdo", nome, campo, texto)
		}
		if texto.Fallback != esperado {
			t.Errorf("%s: %s = %q, quer %q", nome, campo, texto.Fallback, esperado)
		}
	}

	for campo := range campos {
		_, traduz := quer.traduzido[campo]
		_, conteudo := quer.conteudo[campo]
		if !traduz && !conteudo {
			t.Errorf("%s: o campo %q chegou à tela fora do contrato do diálogo", nome, campo)
		}
	}
}

// exigirParametrosSemNomesReservados cobra que nenhum parâmetro se chame como os
// reservados do i18next: caindo na interpolação, count mudaria a pluralização,
// context escolheria outra variante e lng trocaria o idioma da frase (D2).
func exigirParametrosSemNomesReservados(t *testing.T, nome string, payload questionnaire.RequestPayload) {
	t.Helper()
	for campo, texto := range textosDoDialogo(payload) {
		for _, reservado := range []string{"count", "context", "lng"} {
			if _, usado := texto.Params[reservado]; usado {
				t.Errorf("%s: %s interpola %q, que o i18next reserva", nome, campo, reservado)
			}
		}
	}
}

// dialogosDoWizard é o contrato de cada etapa do wizard, com o texto que ela
// exibia antes da migração.
func dialogosDoWizard() map[string]struct {
	payload questionnaire.RequestPayload
	quer    dialogoEsperado
} {
	return map[string]struct {
		payload questionnaire.RequestPayload
		quer    dialogoEsperado
	}{
		"senha mestre": {
			payload: welcomeMasterPasswordPayload(questionnaire.Text{}),
			quer: dialogoEsperado{traduzido: map[string]string{
				"title":                       "Segurança: senha mestre",
				"description":                 "Defina uma senha mestre para criptografar credenciais locais. Guarde com cuidado.",
				"submitLabel":                 "Continuar",
				"cancelLabel":                 "Cancelar",
				"prompt:masterPassword":       "Senha mestre",
				"placeholder:masterPassword":  "Digite uma senha forte",
				"prompt:confirmPassword":      "Confirmar senha mestre",
				"placeholder:confirmPassword": "Repita a senha",
			}},
		},
		"código de recuperação": {
			payload: welcomeRecoveryCodePayload("ABCD-1234"),
			quer: dialogoEsperado{traduzido: map[string]string{
				"title":               "Código de recuperação",
				"description":         "Guarde este código em local seguro. Ele permite recuperar suas credenciais se você esquecer a senha mestre.",
				"submitLabel":         "Continuar",
				"prompt:recoveryCode": "Código de recuperação",
				"prompt:confirmed":    "Eu salvei o código de recuperação em local seguro",
			}},
		},
	}
}

func TestOWizardVaiTraduzivelParaATela(t *testing.T) {
	for nome, caso := range dialogosDoWizard() {
		t.Run(nome, func(t *testing.T) {
			exigirContratoDoDialogo(t, nome, caso.payload, caso.quer)
			exigirParametrosSemNomesReservados(t, nome, caso.payload)
		})
	}
}

// O nome do provedor é conteúdo: não se traduz (AEP-0085 D6). A escolha "outro"
// não nomeia provedor nenhum — é texto do app e ganha chave —, mas o valor que
// volta em answers continua sendo o texto pronto, e é por ele que o wizard sabe
// que precisa pedir a URL do servidor (D5). Traduzir esse valor levaria quem usa
// o app em inglês direto para a etapa da chave de API, sem servidor nenhum.
func TestOCodigoDeRecuperacaoContinuaSendoConteudoDeBloco(t *testing.T) {
	payload := welcomeRecoveryCodePayload("ABCD-1234-EFGH")

	for _, pergunta := range payload.Questions {
		if pergunta.ID != "recoveryCode" {
			continue
		}
		if pergunta.Content != "ABCD-1234-EFGH" {
			t.Errorf("bloco do código = %q, quer o código gerado", pergunta.Content)
		}
		return
	}
	t.Fatal("o diálogo não trouxe o bloco do código de recuperação")
}

// O Ollama roda na máquina e costuma não pedir chave: a frase da etapa é outra, e
// não a mesma com um acréscimo. Chave própria, senão a tradução diria a uma
// pessoa com servidor local que ela precisa de uma chave de API.
func TestWelcomePasswordErrorReplacesDescription(t *testing.T) {
	errorText := welcomePasswordMismatch()
	if errorText.Key == "" {
		t.Fatal("untranslated mismatch")
	}
	if got := welcomeMasterPasswordPayload(errorText).Description; got.Key != errorText.Key {
		t.Fatal("mismatch not presented")
	}
	if got := welcomeMasterPasswordPayload(questionnaire.Text{}).Description; got.Key != welcomeTextKey("passwordDescription") {
		t.Fatal("normal description not restored")
	}
}
