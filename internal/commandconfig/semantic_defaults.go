package commandconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandjson"
)

// SemanticDefault contém exclusivamente defaults builtin confiáveis. Não é
// formato de importação nem aceita segredos de usuário. Versão e apresentação
// não fazem parte da identidade; alteração de qualquer contrato executável faz.
type SemanticDefault struct {
	Candidate           commandbindings.Candidate
	Version             string
	Invariant           bool
	TriggerType         commandcatalog.Source
	TriggerSpec         json.RawMessage
	Arguments           json.RawMessage
	AdapterRequirements []string
}

// BuildSemanticDefault materializa a projeção do resolvedor a partir do
// catálogo completo. O fingerprint não pode ser fornecido por configuração.
// SHA-256 aqui identifica conteúdo builtin público, não argumentos de usuário;
// argumentos de invocação usam HMAC com domínio separado no commandcontract.
func BuildSemanticDefault(registry *commandcatalog.Registry, input SemanticDefault) (commandbindings.Default, error) {
	if registry == nil || !registry.Complete() || strings.TrimSpace(input.Version) == "" {
		return commandbindings.Default{}, ErrInvalid
	}
	definition, ok := registry.Lookup(input.Candidate.CommandID)
	if !ok || !definition.AllowsSource(input.TriggerType) {
		return commandbindings.Default{}, ErrInvalid
	}
	arguments, err := registry.ValidateArguments(input.Candidate.CommandID, input.Arguments)
	if err != nil {
		return commandbindings.Default{}, ErrInvalid
	}
	trigger, err := commandjson.Canonicalize(input.TriggerSpec)
	if err != nil {
		return commandbindings.Default{}, ErrInvalid
	}
	triggerDocument, err := strictObject(string(input.TriggerSpec))
	if err != nil || !versionOne(triggerDocument) {
		return commandbindings.Default{}, ErrInvalid
	}
	requirements := slices.Clone(input.AdapterRequirements)
	slices.Sort(requirements)
	for i, item := range requirements {
		if item == "" || strings.TrimSpace(item) != item || (i > 0 && item == requirements[i-1]) {
			return commandbindings.Default{}, ErrInvalid
		}
	}
	if requirements == nil {
		requirements = []string{}
	}
	// Presentation is deliberately excluded, including localized text/icons.
	definition.Presentation = nil
	slices.Sort(definition.AllowedSources)
	slices.Sort(definition.Scopes)
	slices.Sort(definition.SensitivePaths.Input)
	slices.Sort(definition.SensitivePaths.Output)
	slices.SortFunc(definition.Context.Facts, func(a, b commandcatalog.ContextFact) int {
		if a.Provider != b.Provider {
			return strings.Compare(a.Provider, b.Provider)
		}
		return strings.Compare(a.Fact, b.Fact)
	})
	candidate := input.Candidate
	if candidate.Condition == nil {
		candidate.Condition = commandbindings.Facts{}
	}
	candidate.ArgumentsKey = string(arguments)
	if _, err := commandbindings.New([]commandbindings.Candidate{candidate}); err != nil {
		return commandbindings.Default{}, ErrInvalid
	}
	payload, err := commandjson.Marshal(map[string]any{
		"version": 1, "command": definition, "trigger_type": input.TriggerType,
		"trigger_spec": json.RawMessage(trigger), "trigger_identity": candidate.Trigger,
		"arguments": json.RawMessage(arguments), "condition": candidate.Condition,
		"effect": "execute", "scope": candidate.Scope, "execution_scope": candidate.ExecutionScopeKey,
		"dialog_id": candidate.DialogID, "layer_priority": candidate.LayerPriority,
		"binding_priority": candidate.BindingPriority, "enabled": candidate.Enabled,
		"invariant": input.Invariant, "adapter_requirements": requirements,
	})
	if err != nil {
		return commandbindings.Default{}, ErrInvalid
	}
	sum := sha256.Sum256(append([]byte("assistente.command.default.v1\x00"), payload...))
	return commandbindings.Default{Candidate: candidate, Version: input.Version, Invariant: input.Invariant, Fingerprint: hex.EncodeToString(sum[:])}, nil
}
