// Package commandjobactivation compõe outbox, grants e claims sob o gate
// compartilhado com o executor. Não instala listeners nem habilita o App.
package commandjobactivation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandjson"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrUnavailable = errors.New("command job activation unavailable")

// Ports são exclusivamente de bootstrap. Todas as consultas são locais e
// curtas, sem UI/cofre/reentrada no gate. Keys deve ser cache em memória.
// Authorize rederiva sessão/epochs atuais e acesso ao workspace da regra.
// Runtime confirma que este run continua vivo no runtime, não só no SQLite.
type Ports struct {
	Authorize  func(context.Context, *gorm.DB, commandjobevents.Fact, *string) (commandactivation.Owner, error)
	Layer      func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule) (bool, error)
	Condition  func(context.Context, *gorm.DB, commandactivation.Owner, commandactivation.Rule, commandjobevents.Fact) (bool, error)
	Runtime    func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error)
	Keys       commandautomation.FingerprintKeyProvider
	KeyVersion string
	// WithContext mantém a fonte externa travada durante a transação. É
	// opcional para consumidores que não dependem de uma fonte externa.
	WithContext func(context.Context, func(context.Context) error) error
}

// RuntimeIdentity vem do run vivo, incluindo as gerações em que foi admitido.
// Não tem workspace: esse escopo é derivado exclusivamente da regra/layer.
type RuntimeIdentity struct{ Generation, UserID, AuthContextType, AuthContextID, AuthGeneration, SecurityGeneration string }

func (r RuntimeIdentity) matches(owner commandactivation.Owner) bool {
	return r.Generation != "" && r.UserID == owner.UserID && r.AuthContextType == owner.AuthContextType && r.AuthContextID == owner.AuthContextID && r.AuthGeneration == owner.AuthGeneration && r.SecurityGeneration == owner.SecurityGeneration
}

type Consumer struct {
	db               *gorm.DB
	gate             *commandsecurity.DispatchGate
	outbox           *commandjobevents.Store
	ports            Ports
	now              func() time.Time
	lease, retention time.Duration
	revision         atomic.Uint64
}

func (c *Consumer) withContext(ctx context.Context, fn func(context.Context) error) error {
	if c.ports.WithContext == nil {
		return fn(ctx)
	}
	return c.ports.WithContext(ctx, fn)
}

func New(db *gorm.DB, gate *commandsecurity.DispatchGate, ports Ports, lease, retention time.Duration, now func() time.Time) (*Consumer, error) {
	if db == nil || gate == nil || ports.Authorize == nil || ports.Layer == nil || ports.Condition == nil || ports.Runtime == nil || ports.Keys == nil || ports.KeyVersion == "" || lease <= 0 || retention <= 0 {
		return nil, ErrUnavailable
	}
	if now == nil {
		now = time.Now
	}
	return &Consumer{db: db, gate: gate, outbox: commandjobevents.NewStore(db), ports: ports, now: now, lease: lease, retention: retention}, nil
}

type Result struct{ Applied, Replayed, Ignored, Conflicts int }

// PassResult é o resultado de uma única passagem bounded. Claimed conta as
// ocorrências que receberam a lease de entrega; Processed conta somente as
// que foram consumidas e confirmadas. Em caso de erro, os contadores refletem
// o que já foi confirmado e as ocorrências restantes permanecem protegidas
// pela lease até expirar/reencaminhar.
type PassResult struct {
	Claimed, Processed int
	DeadLettered       int
	Applied, Replayed  int
	Ignored, Conflicts int
	More               bool
}

// RunPass expõe uma passagem síncrona para o lifecycle da instância. Ele não
// cria goroutine, timer ou segunda cadência: reivindica um lote, consome cada
// ocorrência pelo Consumer comum e retorna. deliveryOwner é uma capacidade
// opaca de entrega, nunca a identidade do usuário ou do job.
func (c *Consumer) RunPass(ctx context.Context, deliveryOwner string, limit int) (PassResult, error) {
	var result PassResult
	if c == nil || c.db == nil || c.gate == nil || c.outbox == nil || ctx == nil || strings.TrimSpace(deliveryOwner) == "" || limit <= 0 || limit > 100 {
		return result, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	rows, more, err := c.outbox.ClaimBatch(ctx, deliveryOwner, limit)
	if err != nil {
		return result, err
	}
	result.Claimed = len(rows)
	result.More = more
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		outcome, err := c.Consume(ctx, row.SourceEventID, deliveryOwner)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, commandjobevents.ErrLeaseLost) {
				return result, err
			}
			errorCode, permanent := classifyDeliveryFailure(err)
			var transitionErr error
			if permanent {
				transitionErr = c.outbox.DeadLetter(ctx, row.SourceEventID, deliveryOwner, errorCode)
			} else {
				transitionErr = c.outbox.Retry(ctx, row.SourceEventID, deliveryOwner, errorCode)
			}
			if transitionErr != nil {
				return result, errors.Join(err, transitionErr)
			}
			if permanent {
				result.Processed++
				result.DeadLettered++
			} else {
				// Retry pode ter convertido a oitava tentativa em dead-letter.
				// Observe o estado durável para não sinalizar trabalho inexistente.
				retried, getErr := c.outbox.Get(ctx, row.SourceEventID)
				if getErr != nil {
					return result, errors.Join(err, getErr)
				}
				if retried.DeliveryState == commandjobevents.DeliveryDeadLetter {
					result.Processed++
					result.DeadLettered++
				} else {
					// Retry deixou uma ocorrência pendente. Não há segunda
					// tentativa nesta passagem; a próxima cadência é necessária.
					result.More = true
				}
			}
			// A falha já registrada não deve abortar os demais itens do lote.
			// O item permanece bounded por Retry/DeadLetter e o próximo ciclo
			// continua sendo controlado pelo coordinator.
			continue
		}
		result.Processed++
		result.Applied += outcome.Applied
		result.Replayed += outcome.Replayed
		result.Ignored += outcome.Ignored
		result.Conflicts += outcome.Conflicts
	}
	return result, nil
}

// classifyDeliveryFailure é deliberadamente conservadora: somente erros que
// provam que a ocorrência nunca será válida são dead-lettered. Falhas de
// banco, runtime, chaves ou portas desconhecidas seguem retry bounded.
func classifyDeliveryFailure(err error) (string, bool) {
	switch {
	case errors.Is(err, commandjobevents.ErrInvalidFact):
		return "invalid_fact", true
	case errors.Is(err, commandjobevents.ErrFingerprintConflict):
		return "fingerprint_conflict", true
	case errors.Is(err, commandautomation.ErrInvalid):
		return "invalid_authorization", true
	case errors.Is(err, commandautomation.ErrStale):
		return "stale_authorization", true
	case errors.Is(err, commandautomation.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return "source_not_found", true
	case errors.Is(err, commandautomation.ErrForeignScope):
		return "foreign_scope", true
	case errors.Is(err, commandautomation.ErrFingerprint):
		return "fingerprint_invalid", true
	default:
		return "transient_consume_failure", false
	}
}

// Consume recebe apenas a identidade de entrega. Nenhum owner, regra,
// sequência, fingerprint ou deadline do candidato vira autoridade.
// Ack e processamento de todas as regras compartilham a mesma transação.
func (c *Consumer) Consume(ctx context.Context, eventID, deliveryOwner string) (Result, error) {
	var result Result
	if c == nil || ctx == nil || strings.TrimSpace(deliveryOwner) == "" {
		return result, ErrUnavailable
	}
	err := c.gate.WithMutation(ctx, func() error {
		return c.withContext(ctx, func(txCtx context.Context) error {
			if err := c.advanceRevision(); err != nil {
				return err
			}
			return c.db.WithContext(txCtx).Transaction(func(tx *gorm.DB) error {
				now := c.now().UTC()
				var row commandjobevents.ActivationOutbox
				if err := tx.Where("source_event_id = ? AND delivery_state = ? AND lease_owner = ? AND lease_expires_at > ?", eventID, commandjobevents.DeliveryProcessing, deliveryOwner, now).Take(&row).Error; err != nil {
					return commandjobevents.ErrLeaseLost
				}
				fact, err := c.outbox.VerifiedFactTx(txCtx, tx, eventID, now)
				if err != nil {
					return err
				}
				if err := verifyJob(tx, fact); err != nil {
					return err
				}
				var rules []commandactivation.Rule
				if err := tx.Where("user_id = ? AND mode = ? AND enabled = ? AND review_status = ?", fact.UserID, "event", true, "active").Order("workspace_id, rule_ref_kind, rule_ref").Find(&rules).Error; err != nil {
					return err
				}
				for _, rule := range rules {
					owner, err := c.ports.Authorize(txCtx, tx, detachedFact(fact), clone(rule.WorkspaceID))
					if err != nil {
						return err
					}
					if !sameScope(owner.WorkspaceID, rule.WorkspaceID) || owner.UserID != fact.UserID || owner.AuthContextType == "" || owner.AuthContextID == "" || owner.AuthGeneration == "" || owner.SecurityGeneration == "" {
						return ErrUnavailable
					}
					enabled, err := c.ports.Layer(txCtx, tx, detachedOwner(owner), detachedRule(rule))
					if err != nil {
						return err
					}
					if !enabled {
						result.Ignored++
						continue
					}
					if err := c.validateGrant(txCtx, tx, rule); err != nil {
						// Persistir a desabilitação, sem conceder outra autoridade ou aceitar
						// a ocorrência. Uma nova decisão é necessária para reabilitar.
						if errors.Is(err, commandautomation.ErrNotFound) || errors.Is(err, commandautomation.ErrStale) || errors.Is(err, commandautomation.ErrInvalid) {
							activation, e := commandactivation.NewStore(tx)
							if e != nil {
								return e
							}
							change, e := activation.RevokeInvalidRuleTx(txCtx, tx, owner, commandactivation.Ref{Kind: rule.RuleRefKind, ID: rule.RuleRef}, "grant_invalid")
							if e != nil {
								return e
							}
							config, e := commandconfig.New(tx)
							if e != nil {
								return e
							}
							if _, e = config.BumpGenerationTx(txCtx, tx, commandconfig.Scope{UserID: owner.UserID, WorkspaceID: clone(owner.WorkspaceID)}); e != nil {
								return e
							}
							if change.EffectiveClaims > 0 {
								if _, e = activation.BumpActiveLayersTx(txCtx, tx, owner); e != nil {
									return e
								}
							}
							result.Ignored++
							continue
						}
						return err
					}
					matched, err := c.ports.Condition(txCtx, tx, detachedOwner(owner), detachedRule(rule), detachedFact(fact))
					if err != nil {
						return err
					}
					if !matched && !terminalFact(fact) {
						result.Ignored++
						continue
					}
					outcome, err := c.apply(txCtx, tx, owner, rule, fact, now)
					if err != nil {
						return err
					}
					switch outcome {
					case "applied":
						result.Applied++
					case "replay":
						result.Replayed++
					case "conflict":
						result.Conflicts++
					default:
						result.Ignored++
					}
				}
				ack := tx.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ? AND delivery_state = ? AND lease_owner = ? AND lease_expires_at > ?", eventID, commandjobevents.DeliveryProcessing, deliveryOwner, c.now().UTC()).Updates(map[string]any{"delivery_state": commandjobevents.DeliveryDelivered, "lease_owner": nil, "lease_expires_at": nil, "delivered_at": now})
				if ack.Error != nil {
					return ack.Error
				}
				if ack.RowsAffected != 1 {
					return commandjobevents.ErrLeaseLost
				}
				return nil
			})
		})
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func verifyJob(tx *gorm.DB, f commandjobevents.Fact) error {
	var job struct{ ID string }
	if err := tx.Table("jobs").Select("id").Where("user_id = ? AND slug = ?", f.UserID, f.JobSlug).Take(&job).Error; err != nil {
		return err
	}
	if job.ID != f.JobDatabaseID {
		return ErrUnavailable
	}
	var count int64
	if err := tx.Table("job_runs").Where("id = ? AND user_id = ? AND job_id = ?", f.RunID, f.UserID, job.ID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return commandautomation.ErrNotFound
	}
	if count != 1 {
		return ErrUnavailable
	}
	return nil
}

func (c *Consumer) validateGrant(ctx context.Context, tx *gorm.DB, r commandactivation.Rule) error {
	if r.EventName == nil || *r.EventName != commandautomation.JobRunStateEvent || r.AllowedInternalProducerTypes == nil || r.AutomationGrantID == nil || r.AutomationGrantGeneration == nil || r.AutomationGrantFingerprint == nil || r.AuthorizationDecisionID == nil {
		return commandautomation.ErrInvalid
	}
	producers, err := commandjson.Canonicalize([]byte(*r.AllowedInternalProducerTypes))
	if err != nil || string(producers) != `["jobs.runtime"]` {
		return commandautomation.ErrInvalid
	}
	owner := commandautomation.Owner{UserID: r.UserID, WorkspaceID: clone(r.WorkspaceID)}
	key := commandautomation.NaturalKey{Owner: owner, LayerRef: commandautomation.RuleRef{Kind: string(r.LayerRefKind), Ref: r.LayerRef}, RuleRef: commandautomation.RuleRef{Kind: string(r.RuleRefKind), Ref: r.RuleRef}}
	grant, err := commandautomation.ValidateTx(ctx, tx, owner, key, commandautomation.GrantReference{ID: *r.AutomationGrantID, Generation: *r.AutomationGrantGeneration, Fingerprint: *r.AutomationGrantFingerprint})
	if err != nil {
		return err
	}
	rule := commandautomation.Rule{ID: r.ID, Owner: owner, LayerRef: key.LayerRef, RuleRef: key.RuleRef, Mode: string(r.Mode), Condition: r.Condition, Lifecycle: string(r.Lifecycle), EventName: *r.EventName, AllowedInternalProducerTypes: []string{commandautomation.JobsRuntime}, Enabled: r.Enabled, Source: r.Source, ReviewStatus: r.ReviewStatus}
	rfp, err := commandautomation.FingerprintRule(ctx, rule, c.ports.KeyVersion, c.ports.Keys)
	if err != nil {
		return err
	}
	pfp, err := commandautomation.ProducerTypesFingerprint(ctx, c.ports.KeyVersion, c.ports.Keys)
	if err != nil {
		return err
	}
	gfp, err := commandautomation.FingerprintGrant(ctx, key, rfp, pfp, grant.AutomationGrantGeneration, c.ports.KeyVersion, c.ports.Keys)
	if err != nil {
		return err
	}
	if grant.RuleFingerprint != rfp || grant.ProducerTypesFingerprint != pfp || grant.AutomationGrantFingerprint != gfp || grant.AuthorizationDecisionID != *r.AuthorizationDecisionID {
		return commandautomation.ErrStale
	}
	return nil
}

func clone(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

func detachedOwner(o commandactivation.Owner) commandactivation.Owner {
	o.WorkspaceID = clone(o.WorkspaceID)
	return o
}
func terminalFact(f commandjobevents.Fact) bool {
	return f.State == commandjobevents.StateCompleted || f.State == commandjobevents.StateFailed || f.State == commandjobevents.StateSkipped
}
func sameScope(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func scope(q *gorm.DB, workspace *string) *gorm.DB {
	if workspace == nil {
		return q.Where("workspace_id IS NULL")
	}
	return q.Where("workspace_id = ?", *workspace)
}
func detachedRule(r commandactivation.Rule) commandactivation.Rule {
	raw, _ := json.Marshal(r)
	var v commandactivation.Rule
	_ = json.Unmarshal(raw, &v)
	return v
}
func detachedFact(f commandjobevents.Fact) commandjobevents.Fact {
	raw, _ := json.Marshal(f)
	var v commandjobevents.Fact
	_ = json.Unmarshal(raw, &v)
	return v
}
func freshID() (string, error) { id, err := uuid.NewV7(); return id.String(), err }

func eventKey(user string, workspace *string, kind commandactivation.RefKind, ref, event string) string {
	s := "global"
	if workspace != nil {
		s = "workspace:" + *workspace
	}
	return "activation:" + user + ":" + s + ":" + string(kind) + ":" + ref + ":" + event
}
