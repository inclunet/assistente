package commandconfig

import (
	"maps"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
)

// layerActivationProjection mantém a prova de ativação separada da condição
// do binding. Cada entrada é um caminho OR; não altera a identidade nem a
// especificidade do binding que pertence à camada.
type layerActivationProjection struct {
	active     bool
	conditions []commandbindings.Facts
}

// projectLayerActivation materializa regras síncronas do snapshot, sem
// consultar armazenamento durante Resolve. Regras contextuais viram fatos
// OR; manual/toggle/evento só podem ser provadas por assertedActive, que é a
// asserção autenticada do host. Claims persistidas nunca são autoridade aqui.
// A lista vazia, quando não nil, é deliberadamente uma negação: impede
// fallback acidental quando uma camada possui regras mas nenhuma está
// elegível.
func projectLayerActivation(snapshot Snapshot, refKind commandactivation.RefKind, ref string, workspace *string, enabled, assertedActive bool) (layerActivationProjection, error) {
	projection := layerActivationProjection{active: enabled && assertedActive}
	matchedRule := false
	for _, rule := range snapshot.ActivationRules {
		if rule.LayerRefKind != refKind || rule.LayerRef != ref || !sameWorkspace(rule.WorkspaceID, workspace) {
			continue
		}
		matchedRule = true
		if err := commandactivation.ValidateRule(rule); err != nil {
			return layerActivationProjection{}, ErrInvalid
		}
		if !rule.Enabled || rule.ReviewStatus != "active" {
			continue
		}
		var condition commandbindings.Facts
		switch rule.Mode {
		case commandactivation.ModeAlways:
			condition = commandbindings.Facts{}
		case commandactivation.ModeContext, commandactivation.ModeCondition:
			var err error
			condition, err = decodeActivationCondition(rule.Condition)
			if err != nil {
				return layerActivationProjection{}, ErrInvalid
			}
		case commandactivation.ModeManual, commandactivation.ModeToggle, commandactivation.ModeEvent:
			continue
		default:
			return layerActivationProjection{}, ErrInvalid
		}
		appendUniqueLayerCondition(&projection.conditions, condition)
	}
	if !matchedRule {
		return projection, nil
	}
	if assertedActive {
		appendUniqueLayerCondition(&projection.conditions, commandbindings.Facts{})
	}
	if len(projection.conditions) > 0 {
		projection.active = enabled
	} else if !enabled {
		projection.active = false
	}
	return projection, nil
}

func decodeActivationCondition(raw string) (commandbindings.Facts, error) {
	if raw == "{}" {
		return commandbindings.Facts{}, nil
	}
	return decodeCondition(raw)
}

func appendUniqueLayerCondition(target *[]commandbindings.Facts, condition commandbindings.Facts) {
	// An unconditional path makes the union unconditional. Keeping redundant
	// restricted branches would make adapters demand facts they do not need.
	if len(condition) == 0 {
		*target = []commandbindings.Facts{{}}
		return
	}
	for _, existing := range *target {
		if len(existing) == 0 || maps.Equal(existing, condition) {
			return
		}
	}
	*target = append(*target, maps.Clone(condition))
}
