// Package commandactivation persiste e reconcilia claims de ativação de
// camadas. O pacote é uma borda interna: não recebe listas de claims da UI,
// não executa comandos e não cria grants ou outbox.
package commandactivation

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type RefKind string

const (
	BuiltinRef RefKind = "builtin"
	UserRef    RefKind = "user"
)

type Ref struct {
	Kind RefKind
	ID   string
}

type Mode string

const (
	ModeAlways    Mode = "always"
	ModeContext   Mode = "context"
	ModeCondition Mode = "condition"
	ModeManual    Mode = "manual"
	ModeToggle    Mode = "toggle"
	ModeEvent     Mode = "event"
)

type Lifecycle string

const (
	LifecyclePersistent Lifecycle = "persistent"
	LifecycleSession    Lifecycle = "session"
	LifecycleTemporary  Lifecycle = "temporary"
)

type State string

const (
	StateActive      State = "active"
	StateInactive    State = "inactive"
	StateDeactivated State = "deactivated"
	StateExpired     State = "expired"
	StateStale       State = "stale"
)

type Scope struct {
	UserID      string
	WorkspaceID *string
}

type Owner struct {
	Scope
	AuthContextType    string
	AuthContextID      string
	AuthGeneration     string
	SecurityGeneration string
}

// Origin é a origem normalizada por uma porta confiável. SessionID e DeviceID
// são deliberadamente opacos; nenhum deles é obtido de uma claim persistida.
type Origin struct {
	Type      string
	SessionID string
	DeviceID  string
}

type Layer struct {
	Ref         Ref
	UserID      string
	WorkspaceID *string
	Enabled     bool
}

// Rule usa as mesmas colunas de command_layer_activation_rules em D11. Os
// campos de grant permanecem no schema para integração posterior; este pacote
// não cria, valida ou consume grants.
type Rule struct {
	ID                           string
	UserID                       string
	WorkspaceID                  *string
	LayerRefKind                 RefKind
	LayerRef                     string
	RuleRefKind                  RefKind
	RuleRef                      string
	Mode                         Mode
	Condition                    string
	Lifecycle                    Lifecycle
	EventName                    *string
	AllowedInternalProducerTypes *string
	AuthorizationDecisionID      *string
	AutomationGrantID            *string
	AutomationGrantGeneration    *int64
	AutomationGrantFingerprint   *string
	Enabled                      bool
	Source                       string
	ReplacesDefaultID            *string
	ReplacesDefaultVersion       *string
	ReplacesDefaultFingerprint   *string
	ReviewStatus                 string
}

func (Rule) TableName() string { return "command_layer_activation_rules" }

type Claim struct {
	ActivationID                 string
	LayerRefKind                 RefKind
	LayerRef                     string
	RuleRefKind                  RefKind
	RuleRef                      string
	UserID                       string
	WorkspaceID                  *string
	AuthContextType              string
	AuthContextID                string
	AuthGeneration               string
	SecurityGeneration           string
	SourceType                   string
	SourceInstanceID             *string
	SourceEventID                *string
	SourceCorrelationID          *string
	Sequence                     *int64
	SourceJobDatabaseID          *string
	SourceJobSlug                *string
	EventFingerprint             *string
	SourceReplayPolicyGeneration *string
	SourceReplayDeadline         *time.Time
	State                        State
	TerminalReason               *string
	Provenance                   *string
	ManualStackKey               *string
	ActivatedAt                  time.Time
	ExpiresAt                    *time.Time
	UpdatedAt                    time.Time
}

func (Claim) TableName() string { return "command_layer_activation_state" }

type ContextResult struct {
	Active  bool
	Version string
}

// OwnerPort rederiva o owner autenticado. O Owner recebido por uma operação
// é uma asserção, nunca autoridade; implementações devem consultar a sessão.
type OwnerPort interface {
	Authorize(context.Context, Owner) (Owner, error)
}

type OwnerPortFunc func(context.Context, Owner) (Owner, error)

func (f OwnerPortFunc) Authorize(ctx context.Context, owner Owner) (Owner, error) {
	return f(ctx, owner)
}

type LayerPort interface {
	ResolveLayer(context.Context, Owner, Ref) (Layer, error)
}

type LayerPortFunc func(context.Context, Owner, Ref) (Layer, error)

func (f LayerPortFunc) ResolveLayer(ctx context.Context, owner Owner, ref Ref) (Layer, error) {
	return f(ctx, owner, ref)
}

type OriginPort interface {
	NormalizeOrigin(context.Context, Owner, Origin) (Origin, error)
}

type OriginPortFunc func(context.Context, Owner, Origin) (Origin, error)

func (f OriginPortFunc) NormalizeOrigin(ctx context.Context, owner Owner, origin Origin) (Origin, error) {
	return f(ctx, owner, origin)
}

type ContextPort interface {
	Evaluate(context.Context, Owner, Rule) (ContextResult, error)
}

type ContextPortFunc func(context.Context, Owner, Rule) (ContextResult, error)

func (f ContextPortFunc) Evaluate(ctx context.Context, owner Owner, rule Rule) (ContextResult, error) {
	return f(ctx, owner, rule)
}

type RulePort interface {
	ResolveRule(context.Context, Owner, Ref) (Rule, error)
	ListRules(context.Context, Owner) ([]Rule, error)
}

type RulePortFunc struct {
	Resolve func(context.Context, Owner, Ref) (Rule, error)
	List    func(context.Context, Owner) ([]Rule, error)
}

func (f RulePortFunc) ResolveRule(ctx context.Context, owner Owner, ref Ref) (Rule, error) {
	if f.Resolve == nil {
		return Rule{}, ErrInvalid
	}
	return f.Resolve(ctx, owner, ref)
}

func (f RulePortFunc) ListRules(ctx context.Context, owner Owner) ([]Rule, error) {
	if f.List == nil {
		return nil, ErrInvalid
	}
	return f.List(ctx, owner)
}

// GenerationTxPort só é chamado com o TX/gate do chamador. Nunca adquira
// DispatchGate aqui: isso causaria deadlock com Epochs.AdmitMutation.
type GenerationTxPort interface {
	BumpActiveLayersTx(context.Context, *gorm.DB, Owner) (GenerationSnapshot, error)
}

type GenerationTxPortFunc func(context.Context, *gorm.DB, Owner) (GenerationSnapshot, error)

func (f GenerationTxPortFunc) BumpActiveLayersTx(ctx context.Context, db *gorm.DB, owner Owner) (GenerationSnapshot, error) {
	return f(ctx, db, owner)
}

type Ports struct {
	Owner        OwnerPort
	Layer        LayerPort
	Origin       OriginPort
	Rule         RulePort
	Context      ContextPort
	GenerationTx GenerationTxPort
}

type Mutation struct {
	Changed             bool
	EffectiveClaims     int
	ActiveLayersChanged bool
	Claim               Claim
	Generations         []GenerationSnapshot
}

type GenerationSnapshot struct {
	ID          string
	UserID      string
	WorkspaceID *string
	Generation  int64
	UpdatedAt   time.Time
}

// LayerChange é o DTO deliberadamente independente de commandconfig. O
// chamador monta-o a partir do diff autoritativo já aplicado na mesma
// transação; não é uma lista de claims nem um snapshot vindo da UI.
type LayerChange struct {
	Ref           Ref
	WorkspaceID   *string
	BeforePresent bool
	AfterPresent  bool
	BeforeEnabled bool
	AfterEnabled  bool
}

var (
	ErrInvalid          = errors.New("ativação de comando inválida")
	ErrNotFound         = errors.New("ativação de comando não encontrada")
	ErrForeignOwner     = errors.New("ativação de comando pertence a outro owner")
	ErrStale            = errors.New("ativação de comando obsoleta")
	ErrTerminal         = errors.New("claim de ativação terminal")
	ErrLayerDisabled    = errors.New("camada de comando desabilitada")
	ErrNoContextPort    = errors.New("porta contextual não configurada")
	ErrNoOriginPort     = errors.New("porta de origem não configurada")
	ErrReviewRequired   = errors.New("regra requer revisão")
	ErrGrantUnavailable = errors.New("grant de ativação não disponível")
)
