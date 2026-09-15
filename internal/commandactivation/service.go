package commandactivation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	store *Store
	gate  *commandsecurity.DispatchGate
	ports Ports
	now   func() time.Time
}

func New(db *gorm.DB, gate *commandsecurity.DispatchGate, ports Ports, now func() time.Time) (*Service, error) {
	if db == nil || gate == nil || ports.Owner == nil || ports.Layer == nil || ports.Origin == nil || now == nil {
		return nil, ErrInvalid
	}
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	if ports.Rule == nil {
		ports.Rule = store
	}
	return &Service{store: store, gate: gate, ports: ports, now: now}, nil
}

func (s *Service) Store() *Store {
	if s == nil {
		return nil
	}
	return s.store
}

// ManualStackKey é determinística e não aceita uma chave fornecida pelo
// chamador. O framing evita colisões entre concatenações de campos.
func ManualStackKey(origin Origin) (string, error) {
	origin, err := canonicalOrigin(origin)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte("assistente.commandactivation.manual-stack.v1"))
	for _, value := range []string{origin.Type, origin.SessionID, origin.DeviceID} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(value))
	}
	return "manual:" + hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Service) Pin(ctx context.Context, owner Owner, layer, rule Ref, origin Origin, expiresAt *time.Time) (Mutation, error) {
	return s.manual(ctx, owner, layer, rule, origin, expiresAt, false)
}

func (s *Service) Toggle(ctx context.Context, owner Owner, layer, rule Ref, origin Origin, expiresAt *time.Time) (Mutation, error) {
	return s.manual(ctx, owner, layer, rule, origin, expiresAt, true)
}

func (s *Service) manual(ctx context.Context, asserted Owner, layer, rule Ref, origin Origin, expiresAt *time.Time, toggle bool) (Mutation, error) {
	if s == nil || ctx == nil || s.store == nil || s.gate == nil {
		return Mutation{}, ErrInvalid
	}
	if err := validateRef(layer); err != nil {
		return Mutation{}, err
	}
	if err := validateRef(rule); err != nil {
		return Mutation{}, err
	}
	var mutation Mutation
	err := s.gate.WithMutation(ctx, func() error {
		canonical, err := s.authorize(ctx, asserted)
		if err != nil {
			return err
		}
		canonicalOriginValue, err := s.origin(ctx, canonical, origin)
		if err != nil {
			return err
		}
		stackKey, err := ManualStackKey(canonicalOriginValue)
		if err != nil {
			return err
		}
		resolvedLayer, resolvedRule, err := s.resolvePair(ctx, canonical, layer, rule)
		if err != nil {
			return err
		}
		if !resolvedLayer.Enabled {
			return ErrLayerDisabled
		}
		if resolvedRule.ReviewStatus != "active" || !resolvedRule.Enabled {
			return ErrReviewRequired
		}
		if resolvedRule.Mode != ModeManual && resolvedRule.Mode != ModeToggle {
			return ErrInvalid
		}
		now := s.currentTime()
		if now.IsZero() {
			return ErrInvalid
		}
		if err := validateExpiry(resolvedRule.Lifecycle, expiresAt, now); err != nil {
			return err
		}
		return s.store.WithTx(ctx, func(tx *Tx) error {
			generationOwner := ownerAtWorkspace(canonical, resolvedLayer.WorkspaceID)
			active, findErr := tx.latestManualClaimAtScope(ctx, canonical, layer, rule, stackKey, resolvedLayer.WorkspaceID)
			if findErr != nil && !errors.Is(findErr, ErrNotFound) {
				return findErr
			}
			if findErr == nil {
				if !toggle {
					mutation = Mutation{Claim: active, EffectiveClaims: 1}
					return nil
				}
				changed, err := tx.deactivateClaimAt(ctx, canonical, active.ActivationID, "manual_toggle", now)
				if err != nil {
					return err
				}
				if !changed {
					return ErrStale
				}
				mutation = Mutation{Changed: true, ActiveLayersChanged: true}
				snapshot, err := s.bumpGeneration(ctx, tx.db, generationOwner)
				if err == nil {
					mutation.Generations = append(mutation.Generations, snapshot)
				}
				return err
			}
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			device := canonicalOriginValue.DeviceID
			claim := Claim{ActivationID: id.String(), LayerRefKind: layer.Kind, LayerRef: layer.ID, RuleRefKind: rule.Kind, RuleRef: rule.ID,
				UserID: canonical.UserID, WorkspaceID: cloneString(resolvedLayer.WorkspaceID), AuthContextType: canonical.AuthContextType, AuthContextID: canonical.AuthContextID,
				AuthGeneration: canonical.AuthGeneration, SecurityGeneration: canonical.SecurityGeneration, SourceType: "manual", SourceInstanceID: &device,
				State: StateActive, ManualStackKey: &stackKey, ActivatedAt: now, ExpiresAt: cloneTime(expiresAt), UpdatedAt: now}
			if err := tx.InsertClaim(ctx, claim); err != nil {
				return err
			}
			mutation = Mutation{Changed: true, ActiveLayersChanged: true, EffectiveClaims: 1, Claim: claim}
			snapshot, err := s.bumpGeneration(ctx, tx.db, generationOwner)
			if err == nil {
				mutation.Generations = append(mutation.Generations, snapshot)
			}
			return err
		})
	})
	return mutation, err
}

func (s *Service) Back(ctx context.Context, owner Owner, layer, rule Ref, origin Origin) (Mutation, error) {
	if s == nil || ctx == nil || s.store == nil {
		return Mutation{}, ErrInvalid
	}
	var mutation Mutation
	err := s.gate.WithMutation(ctx, func() error {
		canonical, err := s.authorize(ctx, owner)
		if err != nil {
			return err
		}
		canonicalOriginValue, err := s.origin(ctx, canonical, origin)
		if err != nil {
			return err
		}
		stackKey, err := ManualStackKey(canonicalOriginValue)
		if err != nil {
			return err
		}
		if _, _, err := s.resolvePair(ctx, canonical, layer, rule); err != nil {
			return err
		}
		now := s.currentTime()
		return s.store.WithTx(ctx, func(tx *Tx) error {
			resolvedLayer, _, err := s.resolvePair(ctx, canonical, layer, rule)
			if err != nil {
				return err
			}
			claim, err := tx.latestManualClaimAtScope(ctx, canonical, layer, rule, stackKey, resolvedLayer.WorkspaceID)
			if err != nil {
				return err
			}
			changed, err := tx.deactivateClaimAt(ctx, canonical, claim.ActivationID, "manual_back", now)
			if err != nil {
				return err
			}
			if !changed {
				return ErrStale
			}
			mutation = Mutation{Changed: true, ActiveLayersChanged: true, Claim: claim}
			snapshot, err := s.bumpGeneration(ctx, tx.db, ownerAtWorkspace(canonical, resolvedLayer.WorkspaceID))
			if err == nil {
				mutation.Generations = append(mutation.Generations, snapshot)
			}
			return err
		})
	})
	return mutation, err
}

func (s *Service) Expire(ctx context.Context, owner Owner) (Mutation, error) {
	if s == nil || ctx == nil || s.store == nil {
		return Mutation{}, ErrInvalid
	}
	var mutation Mutation
	err := s.gate.WithMutation(ctx, func() error {
		canonical, err := s.authorize(ctx, owner)
		if err != nil {
			return err
		}
		now := s.currentTime()
		if now.IsZero() {
			return ErrInvalid
		}
		return s.store.WithTx(ctx, func(tx *Tx) error {
			claims, err := tx.ListClaims(ctx, canonical)
			if err != nil {
				return err
			}
			count, effective := 0, 0
			for _, claim := range claims {
				if claim.State != StateActive || claim.ExpiresAt == nil || claim.ExpiresAt.After(now) {
					continue
				}
				changed, err := tx.expireClaimAt(ctx, canonical, claim.ActivationID, now)
				if err != nil {
					return err
				}
				if !changed {
					continue
				}
				count++
				layer, err := s.ports.Layer.ResolveLayer(ctx, canonical, Ref{Kind: claim.LayerRefKind, ID: claim.LayerRef})
				if err == nil && layer.Enabled {
					effective++
				}
			}
			mutation = Mutation{Changed: count > 0, EffectiveClaims: 0, ActiveLayersChanged: effective > 0}
			if effective > 0 {
				snapshot, err := s.bumpGeneration(ctx, tx.db, canonical)
				if err == nil {
					mutation.Generations = append(mutation.Generations, snapshot)
				}
				return err
			}
			return nil
		})
	})
	return mutation, err
}

func (s *Service) ReconcileContext(ctx context.Context, owner Owner) (Mutation, error) {
	if s == nil || ctx == nil || s.ports.Context == nil {
		return Mutation{}, ErrNoContextPort
	}
	var mutation Mutation
	err := s.gate.WithMutation(ctx, func() error {
		canonical, err := s.authorize(ctx, owner)
		if err != nil {
			return err
		}
		now := s.currentTime()
		if now.IsZero() {
			return ErrInvalid
		}
		return s.store.WithTx(ctx, func(tx *Tx) error {
			var rules []Rule
			if ruleStore, ok := s.ports.Rule.(*Store); ok && ruleStore == s.store {
				rules, err = tx.ListRules(ctx, canonical)
			} else {
				rules, err = s.ports.Rule.ListRules(ctx, canonical)
			}
			if err != nil {
				return err
			}
			claims, err := tx.ListClaims(ctx, canonical)
			if err != nil {
				return err
			}
			active := make(map[string]Claim)
			for _, claim := range claims {
				if claim.SourceType == "context" && claim.State == StateActive {
					active[claimKey(claim.WorkspaceID, claim.LayerRefKind, claim.LayerRef, claim.RuleRefKind, claim.RuleRef)] = claim
				}
			}
			changed, effectiveChanged := false, false
			for _, rule := range rules {
				if rule.Mode != ModeContext && rule.Mode != ModeCondition {
					continue
				}
				layerRef := Ref{Kind: rule.LayerRefKind, ID: rule.LayerRef}
				ruleRef := Ref{Kind: rule.RuleRefKind, ID: rule.RuleRef}
				layer, layerErr := s.ports.Layer.ResolveLayer(ctx, canonical, layerRef)
				if layerErr != nil {
					return layerErr
				}
				key := claimKey(rule.WorkspaceID, layerRef.Kind, layerRef.ID, ruleRef.Kind, ruleRef.ID)
				current, hasCurrent := active[key]
				desired := false
				contextVersion := ""
				if rule.Enabled && rule.ReviewStatus == "active" && layer.Enabled {
					result, evalErr := s.ports.Context.Evaluate(ctx, canonical, rule)
					if evalErr != nil {
						return evalErr
					}
					if strings.TrimSpace(result.Version) == "" {
						return ErrInvalid
					}
					desired = result.Active
					contextVersion = result.Version
					if hasCurrent && desired {
						if current.SourceInstanceID == nil || *current.SourceInstanceID != result.Version {
							version := result.Version
							if err := tx.updateClaim(ctx, canonical, current.ActivationID, map[string]any{"source_instance_id": version, "updated_at": now}); err != nil {
								return err
							}
						}
					}
				}
				if desired && !hasCurrent {
					id, err := uuid.NewV7()
					if err != nil {
						return err
					}
					version := contextVersion
					claim := Claim{ActivationID: id.String(), LayerRefKind: rule.LayerRefKind, LayerRef: rule.LayerRef, RuleRefKind: rule.RuleRefKind, RuleRef: rule.RuleRef, UserID: canonical.UserID, WorkspaceID: cloneString(rule.WorkspaceID), AuthContextType: canonical.AuthContextType, AuthContextID: canonical.AuthContextID, AuthGeneration: canonical.AuthGeneration, SecurityGeneration: canonical.SecurityGeneration, SourceType: "context", SourceInstanceID: &version, State: StateActive, ActivatedAt: now, UpdatedAt: now}
					if err := tx.InsertClaim(ctx, claim); err != nil {
						return err
					}
					changed, effectiveChanged = true, true
					continue
				}
				if !desired && hasCurrent {
					if err := tx.updateClaim(ctx, canonical, current.ActivationID, map[string]any{"state": StateDeactivated, "terminal_reason": "context_false", "updated_at": now}); err != nil {
						return err
					}
					changed = true
					if layer.Enabled {
						effectiveChanged = true
					}
				}
			}
			mutation = Mutation{Changed: changed, ActiveLayersChanged: effectiveChanged}
			if effectiveChanged {
				snapshot, err := s.bumpGeneration(ctx, tx.db, canonical)
				if err == nil {
					mutation.Generations = append(mutation.Generations, snapshot)
				}
				return err
			}
			return nil
		})
	})
	return mutation, err
}

// ReconcileLayersTx é chamado pelo MutationTxHook depois que commandconfig já
// aplicou o CRUD e as gerações no TX. Ele não adquire DispatchGate, não abre
// transação e não chama uma API que possa fazê-lo. A responsabilidade do
// chamador é manter a seção exclusiva e incrementar a geração efetiva quando
// o retorno indicar ActiveLayersChanged.
func (s *Service) ReconcileLayersTx(ctx context.Context, db *gorm.DB, owner Owner, changes []LayerChange) (Mutation, error) {
	if s == nil || db == nil || ctx == nil || len(changes) == 0 {
		return Mutation{}, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return Mutation{}, ErrInvalid
	}
	bound, err := s.store.BindTx(db)
	if err != nil {
		return Mutation{}, err
	}
	now := s.currentTime()
	if now.IsZero() {
		return Mutation{}, ErrInvalid
	}
	mutation := Mutation{}
	seen := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if err := validateRef(change.Ref); err != nil || !validChangeWorkspace(change.WorkspaceID, owner.WorkspaceID) {
			return Mutation{}, ErrInvalid
		}
		key := fmt.Sprintf("%s:%s:%v", change.Ref.Kind, change.Ref.ID, change.WorkspaceID != nil)
		if change.WorkspaceID != nil {
			key += ":" + *change.WorkspaceID
		}
		if _, ok := seen[key]; ok {
			return Mutation{}, ErrInvalid
		}
		seen[key] = struct{}{}
		var claims []Claim
		q := db.WithContext(ctx).Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ? AND state = ?", owner.UserID, change.Ref.Kind, change.Ref.ID, StateActive)
		if change.WorkspaceID == nil {
			q = q.Where("workspace_id IS NULL")
		} else {
			q = q.Where("workspace_id = ?", *change.WorkspaceID)
		}
		if err := q.Order("activated_at, activation_id").Find(&claims).Error; err != nil {
			return Mutation{}, err
		}
		beforeEffective := change.BeforePresent && change.BeforeEnabled && len(claims) > 0
		afterEffective := false
		if !change.AfterPresent {
			for _, claim := range claims {
				if err := bound.updateClaim(ctx, owner, claim.ActivationID, map[string]any{"state": StateInactive, "terminal_reason": "layer_deleted", "updated_at": now}); err != nil {
					return Mutation{}, err
				}
			}
			if len(claims) > 0 {
				mutation.Changed = true
			}
			if beforeEffective {
				mutation.ActiveLayersChanged = true
			}
			continue
		}
		for _, claim := range claims {
			if !change.AfterEnabled {
				continue
			}
			if claim.ExpiresAt != nil && !claim.ExpiresAt.After(now) {
				if err := bound.updateClaim(ctx, owner, claim.ActivationID, map[string]any{"state": StateExpired, "terminal_reason": "expiry", "updated_at": now}); err != nil {
					return Mutation{}, err
				}
				mutation.Changed = true
				continue
			}
			if claim.AuthGeneration != owner.AuthGeneration || claim.SecurityGeneration != owner.SecurityGeneration || claim.AuthContextType != owner.AuthContextType || claim.AuthContextID != owner.AuthContextID {
				if err := bound.updateClaim(ctx, owner, claim.ActivationID, map[string]any{"state": StateStale, "terminal_reason": "epoch_changed", "updated_at": now}); err != nil {
					return Mutation{}, err
				}
				mutation.Changed = true
				continue
			}
			if claim.SourceType == "context" {
				rule, err := s.resolveRuleTx(ctx, bound, owner, Ref{Kind: claim.RuleRefKind, ID: claim.RuleRef})
				if err != nil {
					return Mutation{}, err
				}
				if !rule.Enabled || rule.ReviewStatus != "active" {
					if err := bound.updateClaim(ctx, owner, claim.ActivationID, map[string]any{"state": StateStale, "terminal_reason": "rule_disabled", "updated_at": now}); err != nil {
						return Mutation{}, err
					}
					mutation.Changed = true
					continue
				}
				if s.ports.Context == nil {
					return Mutation{}, ErrNoContextPort
				}
				result, err := s.ports.Context.Evaluate(ctx, owner, rule)
				if err != nil {
					return Mutation{}, err
				}
				if strings.TrimSpace(result.Version) == "" {
					return Mutation{}, ErrInvalid
				}
				if !result.Active {
					if err := bound.updateClaim(ctx, owner, claim.ActivationID, map[string]any{"state": StateDeactivated, "terminal_reason": "context_false", "updated_at": now}); err != nil {
						return Mutation{}, err
					}
					mutation.Changed = true
					continue
				}
			}
			afterEffective = true
		}
		if beforeEffective != afterEffective {
			mutation.ActiveLayersChanged = true
		}
		if afterEffective {
			mutation.EffectiveClaims++
		}
		if change.BeforeEnabled != change.AfterEnabled {
			mutation.Changed = true
		}
	}
	if mutation.ActiveLayersChanged {
		snapshot, err := s.bumpGeneration(ctx, db, owner)
		if err != nil {
			return Mutation{}, err
		}
		mutation.Generations = append(mutation.Generations, snapshot)
	}
	return mutation, nil
}

func validChangeWorkspace(change, owner *string) bool {
	if change == nil {
		return true
	}
	if !validOpaque(*change) {
		return false
	}
	return owner != nil && *owner == *change
}

// RestorePersistent rebinds only persistent manual claims. Session and
// temporary claims are ended; a missing origin/layer makes the persistent
// claim inactive for review instead of silently reviving it.
func (s *Service) RestorePersistent(ctx context.Context, owner Owner, origin Origin) (Mutation, error) {
	if s == nil || ctx == nil || s.ports.Origin == nil {
		return Mutation{}, ErrNoOriginPort
	}
	var mutation Mutation
	err := s.gate.WithMutation(ctx, func() error {
		canonical, err := s.authorize(ctx, owner)
		if err != nil {
			return err
		}
		reboundOrigin, originErr := s.ports.Origin.NormalizeOrigin(ctx, canonical, origin)
		stackKey, stackErr := ManualStackKey(reboundOrigin)
		now := s.currentTime()
		if now.IsZero() {
			return ErrInvalid
		}
		return s.store.WithTx(ctx, func(tx *Tx) error {
			claims, err := tx.ListClaims(ctx, canonical)
			if err != nil {
				return err
			}
			if originErr != nil || stackErr != nil {
				return s.restoreUnavailableOrigin(ctx, tx, canonical, claims, now, &mutation)
			}
			changed, effectiveChanged := false, false
			for _, claim := range claims {
				if claim.SourceType != "manual" || claim.State != StateActive {
					continue
				}
				rule, ruleErr := s.resolveRuleTx(ctx, tx, canonical, Ref{Kind: claim.RuleRefKind, ID: claim.RuleRef})
				layer, layerErr := s.ports.Layer.ResolveLayer(ctx, canonical, Ref{Kind: claim.LayerRefKind, ID: claim.LayerRef})
				if ruleErr != nil || layerErr != nil || rule.Lifecycle != LifecyclePersistent {
					if err := tx.updateClaim(ctx, canonical, claim.ActivationID, map[string]any{"state": StateInactive, "terminal_reason": "restore_review", "updated_at": now}); err != nil {
						return err
					}
					changed = true
					if layerErr == nil && layer.Enabled {
						effectiveChanged = true
					}
					continue
				}
				if claim.AuthContextID == canonical.AuthContextID || claim.AuthGeneration == canonical.AuthGeneration || claim.SecurityGeneration == canonical.SecurityGeneration || claim.ManualStackKey == nil || *claim.ManualStackKey == stackKey {
					return ErrStale
				}
				device := reboundOrigin.DeviceID
				if err := tx.updateClaim(ctx, canonical, claim.ActivationID, map[string]any{"auth_context_type": canonical.AuthContextType, "auth_context_id": canonical.AuthContextID, "auth_generation": canonical.AuthGeneration, "security_generation": canonical.SecurityGeneration, "manual_stack_key": stackKey, "source_instance_id": device, "updated_at": now}); err != nil {
					return err
				}
				changed = true
			}
			mutation = Mutation{Changed: changed, ActiveLayersChanged: effectiveChanged}
			if effectiveChanged {
				snapshot, err := s.bumpGeneration(ctx, tx.db, canonical)
				if err == nil {
					mutation.Generations = append(mutation.Generations, snapshot)
				}
				return err
			}
			return nil
		})
	})
	return mutation, err
}

func (s *Service) restoreUnavailableOrigin(ctx context.Context, tx *Tx, owner Owner, claims []Claim, now time.Time, mutation *Mutation) error {
	changed, effective := false, false
	for _, claim := range claims {
		if claim.SourceType != "manual" || claim.State != StateActive {
			continue
		}
		layer, layerErr := s.ports.Layer.ResolveLayer(ctx, owner, Ref{Kind: claim.LayerRefKind, ID: claim.LayerRef})
		if err := tx.updateClaim(ctx, owner, claim.ActivationID, map[string]any{"state": StateInactive, "terminal_reason": "origin_unavailable", "updated_at": now}); err != nil {
			return err
		}
		changed = true
		if layerErr == nil && layer.Enabled {
			effective = true
		}
	}
	*mutation = Mutation{Changed: changed, ActiveLayersChanged: effective}
	if effective {
		snapshot, err := s.bumpGeneration(ctx, tx.db, owner)
		if err == nil {
			(*mutation).Generations = append((*mutation).Generations, snapshot)
		}
		return err
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, asserted Owner) (Owner, error) {
	canonical, err := s.ports.Owner.Authorize(ctx, asserted)
	if err != nil {
		return Owner{}, err
	}
	if err := validateOwner(canonical); err != nil {
		return Owner{}, ErrForeignOwner
	}
	return canonical, nil
}

func (s *Service) origin(ctx context.Context, owner Owner, asserted Origin) (Origin, error) {
	canonical, err := s.ports.Origin.NormalizeOrigin(ctx, owner, asserted)
	if err != nil {
		return Origin{}, err
	}
	return canonicalOrigin(canonical)
}

func (s *Service) resolvePair(ctx context.Context, owner Owner, layerRef, ruleRef Ref) (Layer, Rule, error) {
	layer, err := s.ports.Layer.ResolveLayer(ctx, owner, layerRef)
	if err != nil {
		return Layer{}, Rule{}, err
	}
	if layer.UserID != owner.UserID || !sameWorkspace(layer.WorkspaceID, owner.WorkspaceID) && layer.WorkspaceID != nil {
		return Layer{}, Rule{}, ErrForeignOwner
	}
	rule, err := s.ports.Rule.ResolveRule(ctx, owner, ruleRef)
	if err != nil {
		return Layer{}, Rule{}, err
	}
	if rule.UserID != owner.UserID || !sameWorkspace(rule.WorkspaceID, layer.WorkspaceID) || rule.LayerRefKind != layerRef.Kind || rule.LayerRef != layerRef.ID {
		return Layer{}, Rule{}, ErrForeignOwner
	}
	return layer, rule, nil
}

func (s *Service) resolveRuleTx(ctx context.Context, tx *Tx, owner Owner, ref Ref) (Rule, error) {
	if ruleStore, ok := s.ports.Rule.(*Store); ok && ruleStore == s.store {
		return tx.ResolveRule(ctx, owner, ref)
	}
	return s.ports.Rule.ResolveRule(ctx, owner, ref)
}

func (s *Service) bumpGeneration(ctx context.Context, db *gorm.DB, owner Owner) (GenerationSnapshot, error) {
	if s.ports.GenerationTx != nil {
		return s.ports.GenerationTx.BumpActiveLayersTx(ctx, db, owner)
	}
	return s.store.BumpActiveLayersTx(ctx, db, owner)
}

func (s *Service) currentTime() time.Time { return s.now().UTC() }

func validateExpiry(lifecycle Lifecycle, expiresAt *time.Time, now time.Time) error {
	if lifecycle == LifecycleTemporary {
		if expiresAt == nil || !expiresAt.After(now) {
			return ErrInvalid
		}
		return nil
	}
	if expiresAt != nil {
		return ErrInvalid
	}
	return nil
}

func canonicalOrigin(origin Origin) (Origin, error) {
	origin.Type = strings.ToLower(strings.TrimSpace(origin.Type))
	origin.SessionID = strings.TrimSpace(origin.SessionID)
	origin.DeviceID = strings.TrimSpace(origin.DeviceID)
	if !validText(origin.Type) || !validOpaque(origin.SessionID) || !validOpaque(origin.DeviceID) {
		return Origin{}, ErrInvalid
	}
	return origin, nil
}

func claimKey(workspace *string, layerKind RefKind, layer string, ruleKind RefKind, rule string) string {
	scope := "global"
	if workspace != nil {
		scope = "workspace:" + *workspace
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s", scope, layerKind, layer, ruleKind, rule)
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func ownerAtWorkspace(owner Owner, workspace *string) Owner {
	owner.WorkspaceID = cloneString(workspace)
	return owner
}
func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
