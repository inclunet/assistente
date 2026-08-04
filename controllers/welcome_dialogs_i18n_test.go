package controllers

import (
	"errors"
	"strings"
	"testing"

	"assistente/internal/questionnaire"
)

// errURLDeTeste é a recusa de validação de URL que o wizard repassa à tela.
var errURLDeTeste = errors.New("esquema não suportado")

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
	incluir("submitLabel", payload.SubmitLabel)
	incluir("cancelLabel", payload.CancelLabel)
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
		"provedor": {
			payload: welcomeProviderPayload(""),
			quer: dialogoEsperado{traduzido: map[string]string{
				"title":           "Bem-vindo ao Assistente!",
				"description":     "Vamos configurar seu assistente em alguns passos simples.",
				"submitLabel":     "Próximo",
				"cancelLabel":     "Cancelar",
				"prompt:provider": "Qual provedor de IA você deseja usar?",
			}},
		},
		"URL do servidor": {
			payload: welcomeServerURLPayload("http://localhost:4000", "", questionnaire.Text{}),
			quer: dialogoEsperado{
				traduzido: map[string]string{
					"title":          "Configuração do Servidor",
					"description":    "Informe a URL do servidor OpenAI-compatible.",
					"submitLabel":    "Próximo",
					"cancelLabel":    "Voltar",
					"prompt:baseURL": "URL do servidor",
				},
				// Endereço de exemplo é conteúdo: não existe tradução para uma URL.
				conteudo: map[string]string{"placeholder:baseURL": "http://localhost:4000"},
			},
		},
		"chave de API": {
			payload: welcomeAPIKeyPayload("OpenAI", "", questionnaire.Text{}),
			quer: dialogoEsperado{
				traduzido: map[string]string{
					"title":         "Chave de API",
					"description":   "Informe sua chave de API. Deixe em branco se o servidor não requer autenticação.",
					"submitLabel":   "Próximo",
					"cancelLabel":   "Voltar",
					"prompt:apiKey": "Chave de API (opcional)",
				},
				// Prefixo de chave é formato do provedor, não rótulo.
				conteudo: map[string]string{"placeholder:apiKey": "sk-..."},
			},
		},
		"escolha do modelo": {
			payload: welcomeModelChoicePayload([]string{"gpt-4o-mini", "gpt-4o"}, "gpt-4o-mini"),
			quer: dialogoEsperado{traduzido: map[string]string{
				"title":        "Escolha o Modelo Padrão",
				"description":  "Conexão validada com sucesso! 2 modelo(s) disponível(is).\n\nSelecione o modelo padrão. Você pode alterar depois nas configurações.",
				"submitLabel":  "Finalizar",
				"cancelLabel":  "Voltar",
				"prompt:model": "Modelo padrão:",
			}},
		},
		"modelo digitado": {
			payload: welcomeManualModelPayload(""),
			quer: dialogoEsperado{
				traduzido: map[string]string{
					"title":               "Configurar Modelo",
					"description":         "Conexão validada! O servidor não suporta listagem automática de modelos.\n\nInforme o nome do modelo que deseja usar.",
					"submitLabel":         "Finalizar",
					"cancelLabel":         "Voltar",
					"prompt:defaultModel": "Nome do modelo",
				},
				// Nome de modelo é identificador do provedor.
				conteudo: map[string]string{"placeholder:defaultModel": "gpt-4o-mini"},
			},
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
func TestAsOpcoesDeProvedorSeguemValendoPeloTextoPronto(t *testing.T) {
	opcoes := welcomeProviderPayload("").Questions[0].Options

	valores := questionnaire.TextValues(opcoes)
	quer := append(append([]string{}, wizardProviderNames...), wizardProviderOther)
	if len(valores) != len(quer) {
		t.Fatalf("opções = %q, quer %q", valores, quer)
	}
	for i, valor := range valores {
		if valor != quer[i] {
			t.Errorf("valor da opção %d = %q, quer o que o wizard reencontra (%q)", i, valor, quer[i])
		}
	}

	for _, opcao := range opcoes[:len(wizardProviderNames)] {
		if opcao.Key != "" {
			t.Errorf("opção %+v ganhou chave: nome de provedor não se traduz", opcao)
		}
	}
	outro := opcoes[len(opcoes)-1]
	if outro.Key == "" {
		t.Errorf("opção %+v sem chave: a escolha é texto do app", outro)
	}

	// O caminho do wizard continua saindo do valor da opção, e não do rótulo
	// traduzido.
	if !wizardNeedsCustomURL(outro.String()) {
		t.Errorf("a escolha %q deixou de pedir a URL do servidor", outro.String())
	}
	for _, valor := range valores {
		if info := GetWizardProviderInfo(valor); info.ID == "" {
			t.Errorf("o valor %q não volta a um provedor do wizard", valor)
		}
	}
	if wizardNeedsCustomURL("OpenAI") {
		t.Error("OpenAI passou a pedir URL de servidor")
	}
}

// Nome de modelo é identificador do provedor: vai como texto, e o valor que volta
// é o mesmo que o servidor listou.
func TestOsModelosListadosNaoViramChave(t *testing.T) {
	modelos := []string{"gpt-4o-mini", "llama-3.3-70b-versatile"}

	opcoes := welcomeModelChoicePayload(modelos, "").Questions[0].Options

	for _, opcao := range opcoes {
		if opcao.Key != "" {
			t.Errorf("opção %+v ganhou chave: nome de modelo não se traduz", opcao)
		}
	}
	valores := questionnaire.TextValues(opcoes)
	for i, valor := range valores {
		if valor != modelos[i] {
			t.Errorf("valor da opção %d = %q, quer o modelo que o servidor listou (%q)", i, valor, modelos[i])
		}
	}
}

// A contagem de modelos vai interpolada, e o parâmetro não pode se chamar count:
// o nome é reservado do i18next e mudaria a pluralização da frase (D2).
func TestAContagemDeModelosVaiEmParametroProprio(t *testing.T) {
	descricao := welcomeModelChoicePayload([]string{"a", "b", "c"}, "").Description

	if got := descricao.Params["models"]; got != 3 {
		t.Errorf("contagem nos params = %v, quer quantos o servidor listou", got)
	}
	if !strings.Contains(descricao.Fallback, "3 modelo(s)") {
		t.Errorf("texto pronto = %q, quer a contagem já no lugar", descricao.Fallback)
	}
}

// O código de recuperação é segredo gerado agora: continua indo como conteúdo de
// bloco, cru, e não como texto traduzível.
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
func TestADescricaoDaChaveDistingueOServidorLocal(t *testing.T) {
	remoto := welcomeAPIKeyPayload("OpenAI", "", questionnaire.Text{}).Description
	local := welcomeAPIKeyPayload(wizardProviderOllama, "", questionnaire.Text{}).Description

	if remoto.Key == "" || local.Key == "" {
		t.Fatalf("descrições sem chave: remoto %+v, local %+v", remoto, local)
	}
	if remoto.Key == local.Key {
		t.Errorf("chave = %q nas duas, quer distinguir o servidor local", local.Key)
	}
	if local.Fallback != "Ollama local geralmente não precisa de chave. Você pode deixar em branco." {
		t.Errorf("texto pronto = %q, quer o de antes", local.Fallback)
	}
}

// Cada aviso do wizard diz o que fazer em seguida, e é isso que os distingue: com
// uma chave só, a tradução mandaria conferir a chave de API onde o problema era o
// servidor. O detalhe vem do servidor e vai interpolado, nunca na chave (D6).
func TestOsAvisosDoWizardTemChaveEDetalheInterpolado(t *testing.T) {
	avisos := map[string]struct {
		texto     questionnaire.Text
		fallback  string
		temDetail bool
	}{
		"senhas diferentes": {
			texto:    welcomePasswordMismatch(),
			fallback: "As senhas não conferem. Tente novamente.",
		},
		"URL inválida": {
			texto:     welcomeInvalidURL(errURLDeTeste),
			fallback:  "⚠️ esquema não suportado\n\nCorreija a URL e tente novamente.",
			temDetail: true,
		},
		"servidor inalcançável": {
			texto:     welcomeURLUnreachable("host inalcançável"),
			fallback:  "⚠️ host inalcançável",
			temDetail: true,
		},
		"conexão falhou": {
			texto:     welcomeConnectionFailed("OpenAI", "https://api.openai.com", "timeout"),
			fallback:  "⚠️ Não foi possível conectar ao servidor do OpenAI (https://api.openai.com).\n\ntimeout\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para escolher outro provedor.",
			temDetail: true,
		},
		"autenticação exigida": {
			texto:     welcomeAuthRequired("401 unauthorized"),
			fallback:  "⚠️ 401 unauthorized\n\nInforme uma API Key válida para continuar.",
			temDetail: true,
		},
		"chave inválida": {
			texto:     welcomeAuthInvalid("403 forbidden"),
			fallback:  "⚠️ 403 forbidden\n\nVerifique sua chave e tente novamente.",
			temDetail: true,
		},
		"erro do servidor": {
			texto:     welcomeServerError("500 internal error"),
			fallback:  "⚠️ 500 internal error\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para alterar configurações.",
			temDetail: true,
		},
	}

	porChave := make(map[string]string, len(avisos))
	for nome, aviso := range avisos {
		if aviso.texto.Key == "" {
			t.Errorf("aviso %q = %+v, quer chave de tradução", nome, aviso.texto)
			continue
		}
		if anterior, repetida := porChave[aviso.texto.Key]; repetida {
			t.Errorf("os avisos %q e %q dividem a chave %q: um deles seria dito pelo outro",
				anterior, nome, aviso.texto.Key)
		}
		porChave[aviso.texto.Key] = nome

		if aviso.texto.Fallback != aviso.fallback {
			t.Errorf("aviso %q = %q, quer o texto de antes %q", nome, aviso.texto.Fallback, aviso.fallback)
		}
		if aviso.temDetail {
			detalhe, _ := aviso.texto.Params["detail"].(string)
			if detalhe == "" {
				t.Errorf("aviso %q = %+v, quer o detalhe do servidor interpolado", nome, aviso.texto)
			} else if !strings.Contains(aviso.texto.Fallback, detalhe) {
				t.Errorf("aviso %q: texto pronto %q não traz o detalhe %q", nome, aviso.texto.Fallback, detalhe)
			}
		}
	}
}

// O aviso ocupa o lugar da descrição da etapa: é ele que diz o que fazer agora.
func TestOAvisoPendenteSubstituiADescricaoDaEtapa(t *testing.T) {
	aviso := welcomeAuthInvalid("403 forbidden")

	comAviso := welcomeAPIKeyPayload("OpenAI", "", aviso).Description
	if comAviso.Key != aviso.Key {
		t.Errorf("descrição = %+v, quer o aviso da tentativa anterior", comAviso)
	}

	semAviso := welcomeAPIKeyPayload("OpenAI", "", questionnaire.Text{}).Description
	if semAviso.Key == aviso.Key {
		t.Error("a etapa abriu com um aviso que ninguém deixou pendente")
	}
}
