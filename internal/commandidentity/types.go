// Package commandidentity contém as portas de confiança do executor de
// comandos. Identidades retornadas por este pacote são projeções: a política
// sempre relê a origem autoritativa antes de permitir o despacho.
package commandidentity

import (
	"context"
	"errors"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
)

var (
	ErrInvalidIdentityRequest = errors.New("solicitação de identidade de comando inválida")
	ErrCommandNotAuthorized   = errors.New("comando não autorizado pela política")
	ErrUnsupportedSource      = errors.New("origem não disponível para este contexto")
	ErrSystemCapability       = errors.New("capability system inválida")
	ErrJobExecutionDenied     = errors.New("job não autorizado para execução de comando")
	ErrExternalNotReady       = errors.New("identidade externa ainda não está pronta")
	ErrEpochUnavailable       = errors.New("epoch do serviço de execução indisponível")
)

// ContextPrincipal é a prova curta que o host entrega ao EpochPort. O host
// deve derivá-la do serviço autenticador/runtime, nunca de IDs do payload.
type ContextPrincipal struct{ UserID, Type, ID string }

// EpochPort é implementado pelo adaptador do mesmo
// commandsecurity.EpochService/DispatchGate do executor. O adaptador traduz
// ContextPrincipal/Epoch para commandsecurity e não mantém estado próprio.
// CaptureContextAuthenticated deve ser usado para preparar a prova;
// MutateContext é a porta de invalidação. Este pacote não cria contador, mutex
// ou gate paralelo. Nenhuma dessas operações deve ser chamada dentro de
// Admit, pois o gate não é reentrante.
type EpochPort interface {
	CaptureContextAuthenticated(context.Context, func() (ContextPrincipal, error)) (Epoch, error)
	MutateContext(context.Context, ContextPrincipal, func() error) error
}

type Epoch struct {
	AuthGeneration     string
	SecurityGeneration string
}

// TrustedIdentity é uma projeção exportada para transporte entre o resolvedor
// e o host. Seus campos podem ser forjados por código externo e, por isso,
// nunca são usados como prova no Authorize.
type TrustedIdentity struct {
	AuthContextType    commandcontract.AuthContextType
	AuthContextID      string
	AuthGeneration     string
	SecurityGeneration string
	UserID             string
	SessionID          string
	ActorType          commandcontract.ActorType
	ActorID            string
	Source             commandcatalog.Source
	Role               string
	Scopes             []string
	Roles              []string
	Job                *TrustedJob
}

type TrustedJob struct {
	DatabaseID               string
	Slug                     string
	OwnerUserID              string
	TargetProfileSlug        string
	JobDefinitionFingerprint string
	DelegationFingerprint    string
	GrantGeneration          uint64
	RunID                    string
}

// Requests não incluem owner ou actor. IDs do job são apenas referências e
// nunca autenticam a origem sem a capability e o JobRuntime.
type LocalSessionRequest struct {
	AccessToken string
	Source      commandcatalog.Source
}

type ExternalTokenRequest struct {
	AccessToken string
	Source      commandcatalog.Source
}

type JobServiceRequest struct {
	Capability        JobServiceCapability
	JobDatabaseID     string
	TargetProfileSlug string
	RunID             string
	Source            commandcatalog.Source
}

type SystemRequest struct{ Capability SystemCapability }

// SystemCapability é uma marca opaca emitida pelo bootstrap. Ela não contém
// owner, binding, comando ou permissões.
type SystemCapability interface{ commandSystemCapability() }

// JobServiceCapability deve ser emitida pelo runtime real de jobs e associada
// ao ciclo de vida desse runtime. IDs fornecidos na request não substituem a
// prova de origem.
type JobServiceCapability interface{ commandJobServiceCapability() }

type AgentSource interface {
	ResolveAgent(context.Context, auth.LocalSessionPrincipal) (string, error)
}

type ResolverPort interface {
	ResolveLocalSession(context.Context, LocalSessionRequest) (TrustedIdentity, error)
	ResolveExternalToken(context.Context, ExternalTokenRequest) (TrustedIdentity, error)
	ResolveJobService(context.Context, JobServiceRequest) (TrustedIdentity, error)
	ResolveSystem(context.Context, SystemRequest) (TrustedIdentity, error)
	ResolveAgent(context.Context, LocalSessionRequest, AgentSource) (TrustedIdentity, error)
}

type AuthorizationRule struct {
	CommandID      string
	Actors         []commandcontract.ActorType
	RequiredRoles  []string
	RequiredScopes []string
	AllowSystem    bool
}

// AuthorizationRequest leva novamente a credencial da origem. Identity é
// apenas a projeção observada e será comparada com uma nova resolução.
type AuthorizationRequest struct {
	Identity      TrustedIdentity
	Definition    commandcatalog.Definition
	LocalSession  *LocalSessionRequest
	ExternalToken *ExternalTokenRequest
	JobService    *JobServiceRequest
	System        *SystemRequest
	AgentSource   AgentSource
}

type AuthorizerPort interface {
	Authorize(context.Context, AuthorizationRequest) error
}

type ExecutionPorts struct {
	Resolver   ResolverPort
	Authorizer AuthorizerPort
}

func SystemCapabilityForBootstrap() SystemCapability { return systemCapability{} }
func JobServiceCapabilityForRuntime() JobServiceCapability {
	return &jobServiceCapability{nonce: 1}
}

type systemCapability struct{}

func (systemCapability) commandSystemCapability() {}

// Tamanho não zero garante identidade de ponteiro distinta entre emissões.
type jobServiceCapability struct{ nonce byte }

func (*jobServiceCapability) commandJobServiceCapability() {}

// JobRuntime é o adapter tipado do runtime já existente. Ele deve consultar
// owner, definição, profile e grants no seu contexto autoritativo, não por um
// ID global desacompanhado.
type JobRuntime interface {
	ResolveCommandJob(context.Context, JobServiceCapability, JobServiceRequest) (TrustedJob, error)
	RevalidateCommandJob(context.Context, JobServiceCapability, TrustedJob) error
}

// JobGrantStore é satisfeito pelo store real da AEP-0101. O contexto recebe
// somente o owner derivado pelo runtime; nunca um user_id do candidato.
type JobGrantStore interface {
	HasValidGeneration(context.Context, string, string, string, uint64) (bool, error)
}

var _ ResolverPort = (*Service)(nil)
var _ AuthorizerPort = (*Service)(nil)
