package commandjobactivation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandjson"
	"gorm.io/gorm"
)

const projectionMaxClaims = 1024
const maxProjectionRevision = ^uint64(0)

// ProjectionClaim é uma ativação revalidada para o owner capturado. Runtime
// é a prova em memória usada na mesma leitura; RunID mantém a correlação do
// ciclo sem transformar o payload do evento em autoridade.
type ProjectionClaim struct {
	Claim   commandactivation.Claim
	Runtime RuntimeIdentity
	RunID   string
	// RootOrigin identifica a origem operacional do fato verificado. Não é
	// derivada da claim nem da provenance pública.
	RootOriginType string
	RootOriginID   string
	// Provenance vem exclusivamente do fato verificado na outbox. Nil impede
	// que o App trate uma claim sem cadeia revalidada como reativa.
	Provenance json.RawMessage
}

// ProjectionSnapshot é efêmero. Revision, identidade e validade devem ser
// conferidos pelo consumidor antes de publicar ou reutilizar este resultado.
type ProjectionSnapshot struct {
	Owner      commandactivation.Owner
	Revision   uint64
	ValidUntil time.Time
	Claims     []ProjectionClaim
}

// Projection retorna somente claims/leases autorizadas para o owner exato.
// A leitura usa o mesmo gate, WithContext e transação de lifecycle; não
// renova leases, não altera claims e propaga falhas não classificadas.
func (c *Consumer) Projection(ctx context.Context, owner commandactivation.Owner) (ProjectionSnapshot, error) {
	result := ProjectionSnapshot{Owner: detachedOwner(owner)}
	if c == nil || c.db == nil || c.gate == nil || c.outbox == nil || ctx == nil || !validProjectionOwner(owner) {
		return result, ErrUnavailable
	}
	err := c.gate.WithMutation(ctx, func() error {
		return c.withContext(ctx, func(txCtx context.Context) error {
			if c.revision.Load() == maxProjectionRevision {
				return ErrUnavailable
			}
			result.Revision = c.revision.Load()
			return c.db.WithContext(txCtx).Transaction(func(tx *gorm.DB) error {
				now := c.now().UTC()
				var leases []Lease
				leaseQuery := tx.Where("command_job_activation_leases.user_id = ?", owner.UserID).
					Joins("JOIN command_layer_activation_state AS projection_claim ON projection_claim.activation_id = command_job_activation_leases.activation_id AND projection_claim.user_id = command_job_activation_leases.user_id AND projection_claim.source_type = ?", "job")
				if owner.WorkspaceID == nil {
					leaseQuery = leaseQuery.Where("projection_claim.workspace_id IS NULL")
				} else {
					leaseQuery = leaseQuery.Where("projection_claim.workspace_id IS NULL OR projection_claim.workspace_id = ?", *owner.WorkspaceID)
				}
				if err := leaseQuery.Order("command_job_activation_leases.activation_id").Limit(projectionMaxClaims + 1).Find(&leases).Error; err != nil {
					return err
				}
				if len(leases) > projectionMaxClaims {
					return ErrUnavailable
				}
				for _, lease := range leases {
					var claim commandactivation.Claim
					if err := tx.Where("activation_id = ? AND user_id = ? AND source_type = ?", lease.ActivationID, owner.UserID, "job").Take(&claim).Error; err != nil {
						if errors.Is(err, gorm.ErrRecordNotFound) {
							continue
						}
						return err
					}
					if !projectionOwnerMatchesClaim(owner, claim) {
						continue
					}
					if _, _, err := c.liveClaim(txCtx, tx, lease, now); err != nil {
						if projectionRejection(err) {
							continue
						}
						return err
					}
					if claim.SourceEventID == nil {
						continue
					}
					fact, err := c.outbox.VerifiedRuntimeFactTx(txCtx, tx, *claim.SourceEventID, now)
					if err != nil {
						if projectionRejection(err) {
							continue
						}
						return err
					}
					runtime, err := c.ports.Runtime(txCtx, tx, detachedFact(fact))
					if err != nil {
						if projectionRejection(err) {
							continue
						}
						return err
					}
					if !runtime.matches(owner) || runtime.Generation != lease.RuntimeGeneration {
						continue
					}
					provenance, err := projectionProvenance(fact.Provenance)
					if err != nil {
						return err
					}
					result.Claims = append(result.Claims, ProjectionClaim{Claim: cloneProjectionClaim(claim), Runtime: runtime, RunID: lease.RunID, RootOriginType: fact.RootOriginType, RootOriginID: fact.RootOriginID, Provenance: provenance})
					projectionDeadline(&result.ValidUntil, lease.ExpiresAt)
					if claim.ExpiresAt != nil {
						projectionDeadline(&result.ValidUntil, *claim.ExpiresAt)
					}
				}
				return txCtx.Err()
			})
		})
	})
	if err != nil {
		return ProjectionSnapshot{Owner: detachedOwner(owner)}, err
	}
	return result, nil
}

func projectionProvenance(value map[string]any) (json.RawMessage, error) {
	if len(value) == 0 {
		return nil, nil
	}
	canonical, err := commandjson.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), canonical...), nil
}

// ProjectionRevision é lock-free para que o host possa comparar um snapshot
// antes da publicação. A revisão é conservadora: pode avançar mesmo quando a
// transação mutável posterior sofre rollback.
func (c *Consumer) ProjectionRevision() uint64 {
	if c == nil {
		return 0
	}
	return c.revision.Load()
}

func (c *Consumer) advanceRevision() error {
	for {
		current := c.revision.Load()
		if current == maxProjectionRevision {
			return ErrUnavailable
		}
		if c.revision.CompareAndSwap(current, current+1) {
			return nil
		}
	}
}

func validProjectionOwner(owner commandactivation.Owner) bool {
	return owner.UserID != "" && owner.AuthContextType != "" && owner.AuthContextID != "" && owner.AuthGeneration != "" && owner.SecurityGeneration != ""
}

func projectionOwnerMatchesClaim(owner commandactivation.Owner, claim commandactivation.Claim) bool {
	workspaceMatch := claim.WorkspaceID == nil || sameScope(claim.WorkspaceID, owner.WorkspaceID)
	return claim.UserID == owner.UserID && workspaceMatch && claim.AuthContextType == owner.AuthContextType && claim.AuthContextID == owner.AuthContextID && claim.AuthGeneration == owner.AuthGeneration && claim.SecurityGeneration == owner.SecurityGeneration
}

func projectionDeadline(current *time.Time, candidate time.Time) {
	if current.IsZero() || candidate.Before(*current) {
		*current = candidate
	}
}

func projectionRejection(err error) bool {
	return errors.Is(err, ErrUnavailable) || errors.Is(err, gorm.ErrRecordNotFound) ||
		errors.Is(err, commandjobevents.ErrInvalidFact) || errors.Is(err, commandjobevents.ErrFingerprintConflict) ||
		errors.Is(err, commandautomation.ErrInvalid) || errors.Is(err, commandautomation.ErrStale) ||
		errors.Is(err, commandautomation.ErrNotFound) || errors.Is(err, commandautomation.ErrForeignScope) ||
		errors.Is(err, commandautomation.ErrFingerprint)
}

func cloneProjectionClaim(claim commandactivation.Claim) commandactivation.Claim {
	claim.WorkspaceID = clone(claim.WorkspaceID)
	claim.SourceInstanceID = clone(claim.SourceInstanceID)
	claim.SourceEventID = clone(claim.SourceEventID)
	claim.SourceCorrelationID = clone(claim.SourceCorrelationID)
	claim.SourceJobDatabaseID = clone(claim.SourceJobDatabaseID)
	claim.SourceJobSlug = clone(claim.SourceJobSlug)
	claim.EventFingerprint = clone(claim.EventFingerprint)
	claim.SourceReplayPolicyGeneration = clone(claim.SourceReplayPolicyGeneration)
	claim.SourceReplayDeadline = cloneTime(claim.SourceReplayDeadline)
	claim.TerminalReason = clone(claim.TerminalReason)
	claim.Provenance = clone(claim.Provenance)
	claim.ManualStackKey = clone(claim.ManualStackKey)
	claim.ExpiresAt = cloneTime(claim.ExpiresAt)
	return claim
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
