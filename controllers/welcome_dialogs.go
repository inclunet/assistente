package controllers

import (
	"fmt"

	"assistente/internal/questionnaire"
)

// Diálogos do wizard de boas-vindas: as etapas que a pessoa percorre antes de o
// app existir para ela — senha mestre, código de recuperação, provedor, servidor,
// chave de API e modelo.
//
// Os textos vão com chave de tradução e com o texto pronto em pt-BR (AEP-0085).
// Aqui a tradução importa desde o primeiro segundo: é o primeiro diálogo que
// alguém vê, e quem instala o app em outro idioma ainda não tem onde trocar o
// idioma da interface. O que é dado do ambiente (URL de exemplo, nome de modelo,
// detalhe do erro do servidor) vai como parâmetro ou como texto puro, porque não
// existe tradução para o nome de um modelo nem para a mensagem de um servidor.

// welcomeTextKey é o assunto do wizard nas chaves de tradução (AEP-0085 D7).
func welcomeTextKey(field string) string {
	return "app.questionnaire.welcome." + field
}

// Rótulos das escolhas do wizard que não nomeiam um provedor: são texto do app,
// e o wizard decide o caminho por eles. Traduzidos na tela, continuam voltando em
// answers como o texto pronto (AEP-0085 D5) — é por ele que estas comparações
// funcionam com o app em qualquer idioma.
const (
	wizardProviderOther   = "Outro (URL personalizada)"
	wizardProviderOllama  = "Ollama (Local)"
	wizardProviderAzure   = "Azure OpenAI"
	wizardProviderLiteLLM = "LiteLLM"
)

// wizardProviderNames são os provedores que o wizard oferece. Nome de provedor é
// conteúdo, não rótulo: não se traduz (AEP-0085 D6).
var wizardProviderNames = []string{
	"OpenAI",
	"Anthropic (Claude)",
	"Google (Gemini)",
	"DeepSeek",
	"xAI (Grok)",
	"OpenRouter",
	"Mistral AI",
	"Groq",
	"Together AI",
	"Fireworks AI",
	"Perplexity",
	wizardProviderAzure,
	wizardProviderOllama,
	wizardProviderLiteLLM,
}

// wizardNeedsCustomURL diz se a escolha exige informar a URL do servidor: os
// provedores sem endereço canônico e o "outro".
func wizardNeedsCustomURL(provider string) bool {
	switch provider {
	case wizardProviderOther, wizardProviderAzure, wizardProviderLiteLLM:
		return true
	default:
		return false
	}
}

// Rótulos de botão que se repetem entre as etapas. A mesma palavra na tela é a
// mesma chave: duas chaves para "Continuar" fariam a mesma etapa ser traduzida de
// dois jeitos.
func welcomeSubmitContinue() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("submitContinue"), "Continuar")
}

func welcomeSubmitNext() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("submitNext"), "Próximo")
}

func welcomeSubmitFinish() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("submitFinish"), "Finalizar")
}

func welcomeCancel() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("cancel"), "Cancelar")
}

// welcomeBack é o cancelar das etapas do meio, onde desistir significa voltar
// para a etapa anterior — e não sair do wizard.
func welcomeBack() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("back"), "Voltar")
}

// welcomeDescription escolhe entre a frase da etapa e o erro que a etapa anterior
// deixou: o erro ocupa o lugar da descrição, porque é ele que diz o que fazer
// agora.
func welcomeDescription(erro, padrao questionnaire.Text) questionnaire.Text {
	if !erro.IsZero() {
		return erro
	}
	return padrao
}

// welcomeMasterPasswordPayload é a etapa da senha mestre.
func welcomeMasterPasswordPayload(erro questionnaire.Text) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(welcomeTextKey("passwordTitle"), "Segurança: senha mestre"),
		Description: welcomeDescription(erro, questionnaire.Keyed(
			welcomeTextKey("passwordDescription"),
			"Defina uma senha mestre para criptografar credenciais locais. Guarde com cuidado.",
		)),
		Questions: []questionnaire.Question{
			{
				ID:          "masterPassword",
				Type:        "password",
				Prompt:      questionnaire.Keyed(welcomeTextKey("passwordPrompt"), "Senha mestre"),
				Required:    true,
				Placeholder: questionnaire.Keyed(welcomeTextKey("passwordPlaceholder"), "Digite uma senha forte"),
			},
			{
				ID:          "confirmPassword",
				Type:        "password",
				Prompt:      questionnaire.Keyed(welcomeTextKey("passwordConfirmPrompt"), "Confirmar senha mestre"),
				Required:    true,
				Placeholder: questionnaire.Keyed(welcomeTextKey("passwordConfirmPlaceholder"), "Repita a senha"),
			},
		},
		AllowCancel: true,
		SubmitLabel: welcomeSubmitContinue(),
		CancelLabel: welcomeCancel(),
	}
}

// welcomePasswordMismatch é o aviso de que a confirmação não bateu.
func welcomePasswordMismatch() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("passwordMismatch"), "As senhas não conferem. Tente novamente.")
}

// welcomeRecoveryCodePayload mostra o código de recuperação. O código é segredo
// gerado agora: vai como conteúdo de bloco, cru (AEP-0085 D6).
func welcomeRecoveryCodePayload(recoveryKey string) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(welcomeTextKey("recoveryTitle"), "Código de recuperação"),
		Description: questionnaire.Keyed(
			welcomeTextKey("recoveryDescription"),
			"Guarde este código em local seguro. Ele permite recuperar suas credenciais se você esquecer a senha mestre.",
		),
		Questions: []questionnaire.Question{
			{
				ID:      "recoveryCode",
				Type:    "readonly_code",
				Prompt:  questionnaire.Keyed(welcomeTextKey("recoveryPrompt"), "Código de recuperação"),
				Content: recoveryKey,
			},
			{
				ID:   "confirmed",
				Type: "boolean",
				Prompt: questionnaire.Keyed(
					welcomeTextKey("recoveryConfirmPrompt"),
					"Eu salvei o código de recuperação em local seguro",
				),
				Required: true,
			},
		},
		AllowCancel: false,
		SubmitLabel: welcomeSubmitContinue(),
	}
}

// welcomeProviderPayload é a escolha do provedor de IA. Os nomes dos provedores
// não se traduzem; a escolha "outro" é texto do app e ganha chave, mas segue
// valendo pelo texto pronto — é por ele que o wizard sabe que precisa pedir a URL
// (AEP-0085 D5).
func welcomeProviderPayload(provider string) questionnaire.RequestPayload {
	opcoes := append(
		questionnaire.PlainTexts(wizardProviderNames),
		questionnaire.Keyed(welcomeTextKey("providerOptionOther"), wizardProviderOther),
	)

	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(welcomeTextKey("providerTitle"), "Bem-vindo ao Assistente!"),
		Description: questionnaire.Keyed(
			welcomeTextKey("providerDescription"),
			"Vamos configurar seu assistente em alguns passos simples.",
		),
		Questions: []questionnaire.Question{
			{
				ID:       "provider",
				Type:     "single_choice",
				Prompt:   questionnaire.Keyed(welcomeTextKey("providerPrompt"), "Qual provedor de IA você deseja usar?"),
				Required: true,
				Options:  opcoes,
				Default:  provider,
			},
		},
		AllowCancel: true,
		SubmitLabel: welcomeSubmitNext(),
		CancelLabel: welcomeCancel(),
	}
}

// welcomeServerURLPayload pede a URL do servidor. A URL de exemplo é endereço,
// não rótulo: vai como texto puro.
func welcomeServerURLPayload(placeholderURL, baseURL string, erro questionnaire.Text) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(welcomeTextKey("urlTitle"), "Configuração do Servidor"),
		Description: welcomeDescription(erro, questionnaire.Keyed(
			welcomeTextKey("urlDescription"),
			"Informe a URL do servidor OpenAI-compatible.",
		)),
		Questions: []questionnaire.Question{
			{
				ID:          "baseURL",
				Type:        "text",
				Prompt:      questionnaire.Keyed(welcomeTextKey("urlPrompt"), "URL do servidor"),
				Required:    true,
				Placeholder: questionnaire.Plain(placeholderURL),
				Default:     baseURL,
			},
		},
		AllowCancel: true,
		SubmitLabel: welcomeSubmitNext(),
		CancelLabel: welcomeBack(),
	}
}

// welcomeInvalidURL é o aviso de URL que não passou na validação. A mensagem do
// erro vem de fora e vai interpolada.
func welcomeInvalidURL(err error) questionnaire.Text {
	return questionnaire.KeyedWith(
		welcomeTextKey("urlInvalid"),
		map[string]any{"detail": err.Error()},
		fmt.Sprintf("⚠️ %v\n\nCorrija a URL e tente novamente.", err),
	)
}

// welcomeURLUnreachable repassa o detalhe do servidor sem envolvê-lo em frase
// nenhuma: é o texto que o próprio servidor devolveu.
func welcomeURLUnreachable(detail string) questionnaire.Text {
	return questionnaire.KeyedWith(
		welcomeTextKey("urlUnreachable"),
		map[string]any{"detail": detail},
		fmt.Sprintf("⚠️ %s", detail),
	)
}

// welcomeAPIKeyPayload pede a chave de API. O prefixo de exemplo é formato de
// chave, não rótulo: vai como texto puro.
func welcomeAPIKeyPayload(provider, apiKey string, erro questionnaire.Text) questionnaire.RequestPayload {
	padrao := questionnaire.Keyed(
		welcomeTextKey("apiKeyDescription"),
		"Informe sua chave de API. Deixe em branco se o servidor não requer autenticação.",
	)
	if provider == wizardProviderOllama {
		// O Ollama roda na máquina e costuma não pedir chave. A frase é outra, e
		// não a mesma com um acréscimo: por isso a chave é outra.
		padrao = questionnaire.Keyed(
			welcomeTextKey("apiKeyDescriptionLocal"),
			"Ollama local geralmente não precisa de chave. Você pode deixar em branco.",
		)
	}

	return questionnaire.RequestPayload{
		Title:       questionnaire.Keyed(welcomeTextKey("apiKeyTitle"), "Chave de API"),
		Description: welcomeDescription(erro, padrao),
		Questions: []questionnaire.Question{
			{
				ID:          "apiKey",
				Type:        "text",
				Prompt:      questionnaire.Keyed(welcomeTextKey("apiKeyPrompt"), "Chave de API (opcional)"),
				Required:    false,
				Placeholder: questionnaire.Plain("sk-..."),
				Default:     apiKey,
			},
		},
		AllowCancel: true,
		SubmitLabel: welcomeSubmitNext(),
		CancelLabel: welcomeBack(),
	}
}

// Avisos da etapa da chave de API. Cada um diz o que fazer em seguida, e é isso
// que os distingue: uma chave só faria a tradução dizer "verifique sua chave"
// onde o problema era o servidor.
func welcomeConnectionFailed(provider, baseURL, detail string) questionnaire.Text {
	return questionnaire.KeyedWith(
		welcomeTextKey("connectionFailed"),
		map[string]any{"provider": provider, "url": baseURL, "detail": detail},
		fmt.Sprintf("⚠️ Não foi possível conectar ao servidor do %s (%s).\n\n%s\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para escolher outro provedor.", provider, baseURL, detail),
	)
}

func welcomeAuthRequired(detail string) questionnaire.Text {
	return questionnaire.KeyedWith(
		welcomeTextKey("authRequired"),
		map[string]any{"detail": detail},
		fmt.Sprintf("⚠️ %s\n\nInforme uma API Key válida para continuar.", detail),
	)
}

func welcomeAuthInvalid(detail string) questionnaire.Text {
	return questionnaire.KeyedWith(
		welcomeTextKey("authInvalid"),
		map[string]any{"detail": detail},
		fmt.Sprintf("⚠️ %s\n\nVerifique sua chave e tente novamente.", detail),
	)
}

func welcomeServerError(detail string) questionnaire.Text {
	return questionnaire.KeyedWith(
		welcomeTextKey("serverError"),
		map[string]any{"detail": detail},
		fmt.Sprintf("⚠️ %s\n\nClique \"Próximo\" para tentar novamente ou \"Voltar\" para alterar configurações.", detail),
	)
}

// welcomeModelChoicePayload é a escolha do modelo padrão entre os que o servidor
// listou. Nome de modelo é identificador do provedor: não se traduz. A contagem
// vai interpolada, e o parâmetro não se chama count — o nome é reservado do
// i18next e mudaria a pluralização da frase (AEP-0085 D2).
func welcomeModelChoicePayload(models []string, modelDefault string) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(welcomeTextKey("modelTitle"), "Escolha o Modelo Padrão"),
		Description: questionnaire.KeyedWith(
			welcomeTextKey("modelDescription"),
			map[string]any{"models": len(models)},
			fmt.Sprintf("Conexão validada com sucesso! %d modelo(s) disponível(is).\n\nSelecione o modelo padrão. Você pode alterar depois nas configurações.", len(models)),
		),
		Questions: []questionnaire.Question{
			{
				ID:       "model",
				Type:     "single_choice",
				Prompt:   questionnaire.Keyed(welcomeTextKey("modelPrompt"), "Modelo padrão:"),
				Required: true,
				Options:  questionnaire.PlainTexts(models),
				Default:  modelDefault,
			},
		},
		AllowCancel: true,
		SubmitLabel: welcomeSubmitFinish(),
		CancelLabel: welcomeBack(),
	}
}

// welcomeManualModelPayload é a saída para servidor que não lista modelos: a
// pessoa escreve o nome.
func welcomeManualModelPayload(defaultModel string) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(welcomeTextKey("modelManualTitle"), "Configurar Modelo"),
		Description: questionnaire.Keyed(
			welcomeTextKey("modelManualDescription"),
			"Conexão validada! O servidor não suporta listagem automática de modelos.\n\nInforme o nome do modelo que deseja usar.",
		),
		Questions: []questionnaire.Question{
			{
				ID:          "defaultModel",
				Type:        "text",
				Prompt:      questionnaire.Keyed(welcomeTextKey("modelManualPrompt"), "Nome do modelo"),
				Required:    true,
				Placeholder: questionnaire.Plain("gpt-4o-mini"),
				Default:     defaultModel,
			},
		},
		AllowCancel: true,
		SubmitLabel: welcomeSubmitFinish(),
		CancelLabel: welcomeBack(),
	}
}
