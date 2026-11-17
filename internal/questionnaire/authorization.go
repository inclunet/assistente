package questionnaire

import (
	"errors"
	"slices"
	"strings"

	"assistente/internal/authorization"
)

// AuthorizationActions descreve escolhas de alcance; a autorização e a
// validação da resposta continuam no backend do domínio.
type AuthorizationActions struct {
	Scopes             []authorization.Scope
	Directory          bool
	RememberDenial     bool
	PersistentShortcut authorization.Scope
}

var authorizationScopes = []struct {
	scope authorization.Scope
	label string
}{
	{authorization.ScopeOnce, "Somente esta operação"},
	{authorization.ScopeSession, "Durante esta conversa"},
	{authorization.ScopeWorkspace, "Neste workspace"},
	{authorization.ScopeProfile, "Neste perfil"},
	{authorization.ScopeGlobal, "Todos os meus workspaces e perfis"},
}

func (o AuthorizationActions) Actions() []DecisionAction {
	var actions []DecisionAction
	appendAction := func(scope authorization.Scope, label string, prefix string) {
		id := prefix + string(scope)
		a := DecisionAction{ID: id, Variant: "secondary", Label: Keyed("app.questionnaire.authorization."+id, label)}
		if prefix == "" || prefix == "deny-" {
			a.Polarity = DecisionPolarityAffirmative
			if prefix == "deny-" {
				a.Polarity = DecisionPolarityNegative
				a.Variant = "danger"
			}
			switch scope {
			case authorization.ScopeOnce:
				a.Scope = DecisionScopeCurrent
			case authorization.ScopeSession:
				a.Scope = DecisionScopeConversation
			case o.PersistentShortcut:
				if scope == authorization.ScopeGlobal {
					a.Scope = DecisionScopeGlobal
				} else {
					a.Scope = DecisionScopePersistent
				}
			}
		}
		actions = append(actions, a)
	}
	for _, option := range authorizationScopes {
		if slices.Contains(o.Scopes, option.scope) {
			appendAction(option.scope, option.label, "")
		}
	}
	if len(actions) > 0 {
		actions[0].Primary = true
		actions[0].Variant = "primary"
	}
	if o.Directory {
		for _, option := range authorizationScopes {
			if slices.Contains(o.Scopes, option.scope) {
				appendAction(option.scope, "Pasta pai — "+option.label, "dir-")
			}
		}
	}
	if o.RememberDenial {
		for _, option := range authorizationScopes {
			if option.scope != authorization.ScopeOnce && slices.Contains(o.Scopes, option.scope) {
				appendAction(option.scope, "Negar — "+option.label, "deny-")
			}
		}
	}
	return append(actions, DecisionAction{ID: "deny", Label: Keyed("app.questionnaire.authorization.deny", "Negar"), Variant: "outline", Polarity: DecisionPolarityNegative, Scope: DecisionScopeCurrent})
}

type AuthorizationChoice struct {
	Effect    string
	Scope     authorization.Scope
	Directory bool
}

// Parse só aceita IDs efetivamente oferecidos. Não infere permissões a partir
// de rótulos, nem aceita um escopo escondido pelo contexto da operação.
func (o AuthorizationActions) Parse(resp Response) (AuthorizationChoice, error) {
	deny := AuthorizationChoice{Effect: "deny", Scope: authorization.ScopeOnce}
	if resp.Cancelled {
		return deny, nil
	}
	id, ok := DecisionActionID(resp)
	if !ok {
		return deny, errors.New("resposta de autorização sem ação")
	}
	id = strings.TrimSpace(id)
	found := false
	for _, action := range o.Actions() {
		if action.ID == id {
			found = true
			break
		}
	}
	if !found {
		return deny, errors.New("ação de autorização indisponível")
	}
	if id == "deny" {
		return deny, nil
	}
	choice := AuthorizationChoice{Effect: "allow"}
	if strings.HasPrefix(id, "deny-") {
		choice.Effect = "deny"
		id = strings.TrimPrefix(id, "deny-")
	}
	if strings.HasPrefix(id, "dir-") {
		choice.Directory = true
		id = strings.TrimPrefix(id, "dir-")
	}
	choice.Scope = authorization.Scope(id)
	return choice, nil
}
