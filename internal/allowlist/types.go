package allowlist

import (
	"errors"
	"fmt"
	"strings"
)

// Decision representa a decisão de controle de acesso para um comando.
type Decision int

const (
	// DecisionApprove indica que o comando pode ser executado sem confirmação.
	DecisionApprove Decision = iota

	// DecisionDeny indica que o comando é sempre bloqueado.
	DecisionDeny

	// DecisionConfirm indica que o comando requer confirmação do usuário.
	DecisionConfirm
)

// String retorna a representação textual da decisão.
func (d Decision) String() string {
	switch d {
	case DecisionApprove:
		return "approve"
	case DecisionDeny:
		return "deny"
	case DecisionConfirm:
		return "confirm"
	default:
		return "unknown"
	}
}

// Allowlist define um conjunto de regras para controle de acesso a comandos.
// Armazenada como arquivo JSON em .assistente/allowlists/<slug>.json.
type Allowlist struct {
	// Name é o nome amigável da allowlist (ex: "Desenvolvimento")
	Name string `json:"name"`

	// Description é uma descrição opcional do propósito desta allowlist
	Description string `json:"description,omitempty"`

	// AutoApprove contém patterns de comandos que podem ser executados sem confirmação.
	// Matching: prefix match — se o comando começa com o pattern (sem o * final).
	// Exemplos: "ls", "git status", "git diff *", "go test *", "npm *"
	AutoApprove []string `json:"auto_approve"`

	// AlwaysDeny contém patterns de comandos que são sempre bloqueados.
	// Mesma lógica de matching que AutoApprove.
	AlwaysDeny []string `json:"always_deny"`

	// CommandRules contém regras estruturadas por programa/subcomando.
	// Elas permitem diferenciar comandos como "kubectl get" e "kubectl delete".
	CommandRules []CommandRule `json:"command_rules,omitempty"`

	// DefaultAction define o que fazer quando nenhum pattern corresponde.
	// Valores válidos: "confirm" (padrão) ou "deny"
	DefaultAction string `json:"default_action"`
}

// CommandRule define uma regra estruturada para comandos atomicos.
type CommandRule struct {
	Program     string   `json:"program"`
	Subcommands []string `json:"subcommands,omitempty"`
	Args        []string `json:"args,omitempty"`
	Decision    string   `json:"decision"`
	Description string   `json:"description,omitempty"`
}

// AllowlistInfo contém informações resumidas de uma allowlist (para listagem).
type AllowlistInfo struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	RuleCount   int    `json:"ruleCount"`
}

// validRuleDecisions contém as decisões aceitas em CommandRule.Decision.
var validRuleDecisions = map[string]struct{}{
	"approve": {},
	"confirm": {},
	"deny":    {},
}

// validDefaultActions contém as ações aceitas em Allowlist.DefaultAction.
// Vazio também é aceito (o evaluator trata como "confirm" por padrão).
var validDefaultActions = map[string]struct{}{
	"confirm": {},
	"deny":    {},
	"":        {},
}

// Validate verifica a integridade semantica de uma allowlist antes da
// persistencia. Os erros sao agrupados para que o frontend possa exibir todos
// de uma vez. Em runtime mantemos comportamento fail-closed (decision
// desconhecido => confirm), mas aqui falhamos cedo para sinalizar problemas
// na origem (UI ou edicao manual do JSON).
func (a *Allowlist) Validate() error {
	if a == nil {
		return errors.New("allowlist nao pode ser nil")
	}

	var errs []string

	if strings.TrimSpace(a.Name) == "" {
		errs = append(errs, "name: obrigatorio")
	}

	if _, ok := validDefaultActions[strings.ToLower(strings.TrimSpace(a.DefaultAction))]; !ok {
		errs = append(errs, fmt.Sprintf("default_action: valor invalido %q (esperado: approve|confirm|deny)", a.DefaultAction))
	}

	for i, rule := range a.CommandRules {
		if err := rule.Validate(); err != nil {
			errs = append(errs, fmt.Sprintf("command_rules[%d]: %v", i, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("allowlist invalida: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Validate verifica a consistencia de uma regra estruturada.
//
// Regras obrigatorias:
//   - Program nao pode ser vazio.
//   - Decision deve ser approve|confirm|deny (case-insensitive).
//   - "*" so pode aparecer na ultima posicao de Subcommands ou Args (em outras
//     posicoes seria silenciosamente tratado como literal pelo evaluator,
//     enganando o autor da regra).
func (r CommandRule) Validate() error {
	var errs []string

	if strings.TrimSpace(r.Program) == "" {
		errs = append(errs, "program: obrigatorio")
	}

	decisionKey := strings.ToLower(strings.TrimSpace(r.Decision))
	if _, ok := validRuleDecisions[decisionKey]; !ok {
		errs = append(errs, fmt.Sprintf("decision: valor invalido %q (esperado: approve|confirm|deny)", r.Decision))
	}

	if pos, ok := wildcardOutOfTail(r.Subcommands); ok {
		errs = append(errs, fmt.Sprintf("subcommands[%d]: \"*\" so pode aparecer como ultimo elemento", pos))
	}
	if pos, ok := wildcardOutOfTail(r.Args); ok {
		errs = append(errs, fmt.Sprintf("args[%d]: \"*\" so pode aparecer como ultimo elemento", pos))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// wildcardOutOfTail devolve a posicao do primeiro "*" encontrado fora da
// ultima posicao da slice. Se nao houver violacao, ok=false.
func wildcardOutOfTail(values []string) (int, bool) {
	if len(values) == 0 {
		return 0, false
	}
	for i := 0; i < len(values)-1; i++ {
		if values[i] == "*" {
			return i, true
		}
	}
	return 0, false
}
