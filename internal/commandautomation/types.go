// Package commandautomation persiste os grants exclusivos das regras de
// ativação event-driven de camadas. O pacote não recebe candidatos de eventos
// e não executa ativação: essa integração pertence ao host autenticado.
package commandautomation

import (
	"context"
	"errors"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

const (
	JobRunStateEvent = "command-context.job-run-state.v1"
	JobsRuntime      = "jobs.runtime"
)

var (
	ErrInvalid      = errors.New("grant de automação inválido")
	ErrStale        = errors.New("grant de automação obsoleto")
	ErrNotFound     = errors.New("grant de automação não encontrado")
	ErrForeignScope = errors.New("grant de automação fora do escopo")
	ErrFingerprint  = errors.New("fingerprint de grant de automação indisponível")
)

// Owner é derivado pelo host autenticado. WorkspaceID nil significa o escopo
// global; um valor preenchido nunca inclui outros workspaces.
type Owner struct {
	UserID      string
	WorkspaceID *string
}

// Scope é mantido como alias para as portas que usam essa nomenclatura.
type Scope = Owner

type RuleRef struct {
	Kind string
	Ref  string
}

// NaturalKey é a chave canônica de um grant. Ela nunca é aceita do payload de
// um job; o host deve derivá-la da regra carregada no banco.
type NaturalKey struct {
	Owner    Owner
	LayerRef RuleRef
	RuleRef  RuleRef
}

type GrantReference struct {
	ID          string
	Generation  int64
	Fingerprint string
}

// Rule é uma cópia de leitura da linha command_layer_activation_rules. Os
// campos de grant são retornados para revalidação, mas não são aceitos como
// autoridade durante Prepare/Commit.
type Rule struct {
	ID                           string
	Owner                        Owner
	LayerRef                     RuleRef
	RuleRef                      RuleRef
	Mode                         string
	Condition                    string
	Lifecycle                    string
	EventName                    string
	AllowedInternalProducerTypes []string
	AuthorizationDecisionID      *string
	AutomationGrantID            *string
	AutomationGrantGeneration    *int64
	AutomationGrantFingerprint   *string
	Enabled                      bool
	Source                       string
	ReviewStatus                 string
}

type Grant struct {
	ID                         string
	Owner                      Owner
	LayerRef                   RuleRef
	RuleRef                    RuleRef
	RuleFingerprint            string
	EventName                  string
	ProducerTypesFingerprint   string
	AutomationGrantGeneration  int64
	AutomationGrantFingerprint string
	AuthorizationDecisionID    string
	GrantedAt                  time.Time
	GrantedBy                  string
	RevokedAt                  *time.Time
	RevokedBy                  *string
	RevocationReason           *string
}

// FingerprintKeyProvider é a porta do secret manager. A chave é sempre
// solicitada pelo host através de command-request-hmac:<versão>; não há
// fallback, provisionamento ou segredo no payload.
type FingerprintKeyProvider = commandledger.FingerprintKeyProvider

// PolicyValidator é executado dentro da transação de commit, depois da
// releitura autoritativa da regra e antes do CAS. O callback é interno ao
// host confiável: deve apenas validar estado local curto e não reabrir tx/gate.
type PolicyValidator func(context.Context, *gorm.DB, Owner, Rule) error

// GrantChange é deliberadamente opaco. Só Store.Prepare pode produzir uma
// proposta e os campos privados vinculam store, owner, regra e geração lidos.
type GrantChange struct {
	store         *Store
	owner         Owner
	rule          Rule
	maxGeneration int64
}

// ConfirmedGrantChange é a proposta após receipt accepted; não é autorização
// independente e só pode ser consumida pelo mesmo Store/epoch.
type ConfirmedGrantChange struct {
	store    *Store
	change   *GrantChange
	receipts *commanddecision.Store
	epoch    commandsecurity.EpochSnapshot
	request  commanddecision.Request
	grant    Grant
}
