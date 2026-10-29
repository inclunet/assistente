package controllers

import "assistente/internal/questionnaire"

// Preparação do cofre no onboarding: senha mestre e código de recuperação.
// A configuração do provedor usa o mesmo ProviderForm das configurações.
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

// Rótulos de botão que se repetem entre as etapas. A mesma palavra na tela é a
// mesma chave: duas chaves para "Continuar" fariam a mesma etapa ser traduzida de
// dois jeitos.
func welcomeSubmitContinue() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("submitContinue"), "Continuar")
}

func welcomeCancel() questionnaire.Text {
	return questionnaire.Keyed(welcomeTextKey("cancel"), "Cancelar")
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
