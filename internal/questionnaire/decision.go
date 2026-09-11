package questionnaire

// KindDecision marca um payload de decisão de um clique (AEP-0091).
// Questionários multi-campo continuam com Kind vazio.
const KindDecision = "decision"

// AnswerActionID é a chave em Response.Answers com o id da ação escolhida.
const AnswerActionID = "actionId"

// DecisionPolarity descreve a intenção afirmativa/negativa da ação.
// O frontend nunca infere este valor por label, id, posição ou variante.
type DecisionPolarity string

const (
	DecisionPolarityAffirmative DecisionPolarity = "affirmative"
	DecisionPolarityNegative    DecisionPolarity = "negative"
)

// DecisionScope descreve o alcance semântico usado pelos atalhos universais.
type DecisionScope string

const (
	DecisionScopeCurrent      DecisionScope = "current"
	DecisionScopeConversation DecisionScope = "conversation"
	DecisionScopePersistent   DecisionScope = "persistent"
	DecisionScopeProfile      DecisionScope = "profile"
	DecisionScopeGlobal       DecisionScope = "global"
)

// DecisionSeverity descreve a intenção do pedido, sem inferência por cor.
type DecisionSeverity string

const (
	DecisionSeverityPermission  DecisionSeverity = "permission"
	DecisionSeverityDestructive DecisionSeverity = "destructive"
	DecisionSeverityInfo        DecisionSeverity = "info"
)

// DecisionAction é um botão do DecisionDialog (sem rádio + Confirmar).
type DecisionAction struct {
	ID       string           `json:"id"`
	Label    Text             `json:"label"`
	Variant  string           `json:"variant,omitempty"` // primary | secondary | danger | outline | ghost
	Shortcut Text             `json:"shortcut,omitzero"`
	Primary  bool             `json:"primary,omitempty"`
	Polarity DecisionPolarity `json:"polarity,omitempty"`
	Scope    DecisionScope    `json:"scope,omitempty"`
}

// DecisionActionID lê o id da ação escolhida numa resposta kind=decision.
func DecisionActionID(resp Response) (string, bool) {
	if resp.Cancelled {
		return "", false
	}
	id, ok := resp.Answers[AnswerActionID].(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}
