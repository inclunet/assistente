package skills

import "strings"

// Validação de qualidade de descrição para descoberta (AEP-0072 D4).
//
// As descrições de skill alimentam o roteamento pelo LLM no Nível 1 (catálogo
// compacto). Descrições em 3ª pessoa com frases-gatilho ("use when ...")
// melhoram a precisão do roteamento (padrão Claude/Codex). Estas verificações
// são WARNINGS não-fatais: orientam autores no import sem rejeitar skills
// existentes (backward-compat). A coleta/telemetria fica na Fase 5 (#123).

// descriptionTriggerPhrases lista marcadores de frase-gatilho em pt/en/es. A
// presença de pelo menos um sinaliza uma descrição orientada a "quando usar".
var descriptionTriggerPhrases = []string{
	// inglês
	"use when", "used when", "use this when", "should be used", "when you",
	"use to", "helps you", "for when",
	// português
	"use quando", "usar quando", "quando ", "use para", "útil para", "para quando",
	// espanhol
	"usar cuando", "use cuando", "cuando ", "útil para", "para cuando",
}

// descriptionFirstPersonMarkers lista marcadores de 1ª pessoa (desencorajados)
// em pt/en/es. Usa espaços de fronteira para evitar falsos positivos.
var descriptionFirstPersonMarkers = []string{
	" i ", "i am ", "i'm ", "i will ", " my ", " me ", " we ", " our ",
	" eu ", " meu ", " minha ", " nós ", " nosso ", " nossa ",
	" yo ", " mi ", " nosotros ", " nuestro ", " nuestra ",
}

// DescriptionWarningTooShort, etc. são códigos estáveis para telemetria.
const (
	DescriptionWarnTooShort    = "description_too_short"
	DescriptionWarnNoTrigger   = "description_no_trigger_phrase"
	DescriptionWarnFirstPerson = "description_first_person"
)

// DescriptionWarning é um aviso não-fatal sobre a qualidade da descrição.
type DescriptionWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidateDescriptionQuality retorna warnings não-fatais sobre a descrição de um
// skill, voltados à qualidade de descoberta (Nível 1). Retorna nil quando a
// descrição está adequada. Descrição vazia não gera warning aqui (a obrigação
// de presença é tratada por validateSpec em modo estrito).
func ValidateDescriptionQuality(description string) []DescriptionWarning {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return nil
	}

	var warnings []DescriptionWarning

	// Recomendação de comprimento mínimo útil para roteamento.
	if len(desc) < 20 {
		warnings = append(warnings, DescriptionWarning{
			Code:    DescriptionWarnTooShort,
			Message: "description is very short; add specifics so the model can route to this skill",
		})
	}

	lower := " " + strings.ToLower(desc) + " "

	if !containsAny(lower, descriptionTriggerPhrases) {
		warnings = append(warnings, DescriptionWarning{
			Code:    DescriptionWarnNoTrigger,
			Message: "description has no trigger phrase; prefer 3rd-person with 'use when ...' (e.g. 'Use when the user asks to ...')",
		})
	}

	if containsAny(lower, descriptionFirstPersonMarkers) {
		warnings = append(warnings, DescriptionWarning{
			Code:    DescriptionWarnFirstPerson,
			Message: "description uses first person; prefer 3rd-person phrasing for skill descriptions",
		})
	}

	return warnings
}

// containsAny retorna true se haystack contém qualquer um dos needles.
func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
