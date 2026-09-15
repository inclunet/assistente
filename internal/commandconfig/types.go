// Package commandconfig carrega, valida e oferece primitivas de persistência
// de configuração. Não autentica, autoriza nem ativa camadas por conta própria.
package commandconfig

import (
	"errors"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

var (
	ErrInvalid = errors.New("configuração persistida inválida")
	ErrStale   = errors.New("configuração persistida obsoleta")
)

// Scope vem do host autenticado. WorkspaceID nil significa apenas global;
// preenchido inclui o global e exatamente esse workspace, nunca todos.
type Scope struct {
	UserID      string
	WorkspaceID *string
}

type Layer struct {
	ID                 string
	UserID             string
	WorkspaceID        *string
	Name               string
	Description        string
	Enabled            bool
	Source             string
	ResolutionPriority int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Layer) TableName() string { return "command_layers" }

// JSONs são documentos opacos. O projetor confiável ainda precisa validar
// versões/schemas, catálogo, referências builtin e ausência de segredo bruto.
// Carregar estes dados não produz um Candidate nem concede execução.
type Binding struct {
	ID                         string
	UserID                     string
	WorkspaceID                *string
	LayerRefKind               string
	LayerRef                   string
	TriggerType                string
	TriggerSpec                string
	CommandID                  *string
	Arguments                  string
	Condition                  string
	Effect                     string
	Enabled                    bool
	Source                     string
	ResolutionPriority         int
	ReplacesDefaultID          *string
	ReplacesDefaultVersion     *string
	ReplacesDefaultFingerprint *string
	ReviewStatus               string
	Presentation               string
}

func (Binding) TableName() string { return "command_bindings" }

type Generation struct {
	ID          string
	UserID      string
	WorkspaceID *string
	Generation  int64
	UpdatedAt   time.Time
}

func (Generation) TableName() string { return "command_config_generations" }

// Snapshot inclui o estado persistente composto que participa do restore. As
// concessões e claims são histórico/estado derivado: o commit não os restaura
// por payload, mas o diff os carrega para que a reconciliação e a auditoria
// sejam atômicas e completas. Slices retornadas pertencem ao chamador.
type Snapshot struct {
	Scope            Scope
	Layers           []Layer
	Bindings         []Binding
	Generations      []Generation
	ActivationRules  []commandactivation.Rule
	AutomationGrants []commandautomation.Grant
	ActivationClaims []commandactivation.Claim
	stamp            *stamp
}
