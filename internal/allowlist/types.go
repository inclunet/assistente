package allowlist

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
