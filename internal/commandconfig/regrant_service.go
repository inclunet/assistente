package commandconfig

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"gorm.io/gorm"
)

// RegrantEventRule é o único fluxo de habilitação de regra event-driven. A
// decisão é assinada pelo fingerprint exato da regra/produtores pelo Store de
// automação; o consumo da receipt, regra, grant, geração, hook e auditoria
// ocorre no mesmo TX do writer comum.
func (s *CompleteMutationService) RegrantEventRule(ctx context.Context, token string, workspace *string, ruleID string) (MutationDiff, error) {
	if s == nil || s.service == nil || s.service.config.Automation == nil || ctx == nil || !validID(ruleID) {
		return MutationDiff{}, ErrInvalid
	}
	workspace = cloneWorkspace(workspace)
	var principal auth.LocalSessionPrincipal
	var scope Scope
	epoch, err := s.service.config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		p, err := s.service.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return "", "", err
		}
		principal = p
		scope = Scope{UserID: p.UserID, WorkspaceID: workspace}
		if !validScope(scope) {
			return "", "", ErrInvalid
		}
		if err := s.service.config.Authorize(ctx, p, cloneScope(scope), RuleEnable); err != nil {
			return "", "", err
		}
		return p.UserID, p.SessionID, nil
	})
	if err != nil {
		return MutationDiff{}, err
	}

	owner := commandautomation.Owner{UserID: scope.UserID, WorkspaceID: cloneWorkspace(scope.WorkspaceID)}
	var version string
	var preparedGrant *commandautomation.GrantChange
	err = s.service.config.Epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, err := s.service.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrStale
		}
		return s.service.config.Authorize(ctx, current, cloneScope(scope), RuleEnable)
	}, func() error {
		version, err = s.service.config.Version(ctx)
		if err != nil || version == "" {
			if err == nil {
				err = ErrInvalid
			}
			return err
		}
		preparedGrant, err = s.service.config.Automation.PrepareRule(ctx, owner, ruleID)
		return err
	})
	if err != nil {
		return MutationDiff{}, err
	}

	// Inscreve o contexto antes de abrir a decisão. Logout/invalidação deve
	// cancelar a espera do presenter, sem permitir que a decisão continue fora
	// do epoch capturado.
	watch, release, err := s.service.config.Epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return MutationDiff{}, err
	}
	defer release()
	grantRule, err := preparedGrant.Rule()
	if err != nil {
		return MutationDiff{}, err
	}
	confirmedGrant, err := s.service.config.Automation.ConfirmGrant(watch, preparedGrant, epoch, s.service.config.Receipts, s.service.config.KeyVersion, s.service.config.Keys, time.Now().Add(s.service.config.DecisionTTL), func(rule commandautomation.Rule) (string, error) {
		before := activationRuleFromAutomation(rule)
		after := before
		after.Enabled = true
		return s.service.config.Render(MutationDiff{Operation: RuleEnable, Scope: cloneScope(scope), BeforeActivationRules: []commandactivation.Rule{before}, AfterActivationRules: []commandactivation.Rule{after}})
	})
	if err != nil {
		return MutationDiff{}, err
	}
	request, err := confirmedGrant.DecisionRequest()
	if err != nil {
		return MutationDiff{}, err
	}
	grant, err := confirmedGrant.Grant()
	if err != nil {
		return MutationDiff{}, err
	}
	if grantRule.ID != ruleID {
		return MutationDiff{}, ErrStale
	}

	var prepared *PreparedMutation
	err = s.service.config.Epochs.AdmitMutation(ctx, epoch, func(ctx context.Context) error {
		current, err := s.service.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrStale
		}
		if err := s.service.config.Authorize(ctx, current, cloneScope(scope), RuleEnable); err != nil {
			return err
		}
		currentVersion, err := s.service.config.Version(ctx)
		if err != nil {
			return err
		}
		if currentVersion != version {
			return ErrStale
		}
		prepared, err = s.service.config.Store.PrepareRuleRegrant(ctx, scope, ruleID, grant, s.service.config.Validate)
		if err != nil {
			return err
		}
		prepared.diff.MutationID = request.MutationID
		prepared.diff.RequiresDecision = true
		prepared.diff.DecisionFingerprint = request.Fingerprint
		return s.service.config.Validate(ctx, cloneConfigSnapshot(prepared.after))
	}, func() error {
		if s.service.config.BeforeCommit != nil {
			if err := s.service.config.BeforeCommit(ctx, cloneScope(scope)); err != nil {
				return err
			}
		}
		return s.service.config.Store.commitConfirmedRequest(ctx, prepared, epoch, s.service.config.Receipts, request, s.service.config.OnMutationTx, func(ctx context.Context, tx *gorm.DB) error {
			return s.service.config.Automation.ApplyConfirmedGrantTx(ctx, tx, confirmedGrant, epoch)
		})
	})
	if err != nil {
		return MutationDiff{}, err
	}
	return prepared.Diff(), nil
}

// Preview delega à mesma preparação autoritativa usada por Apply. Regrant
// possui uma prévia própria porque RuleEnable puro continua proibido para
// regras de evento sem grant.
func (s *CompleteMutationService) Preview(ctx context.Context, token string, workspace *string, intent MutationIntent) (MutationDiff, error) {
	if s == nil || s.service == nil {
		return MutationDiff{}, ErrInvalid
	}
	return s.service.Preview(ctx, token, workspace, intent)
}

// PreviewRegrantEventRule apresenta somente a transição estrutural, sem
// executar decisão, gravar receipt ou habilitar a regra. Não é autorização nem
// um diff completo da autoridade: fingerprint e vínculos do grant só existem
// no fluxo RegrantEventRule.
func (s *CompleteMutationService) PreviewRegrantEventRule(ctx context.Context, token string, workspace *string, ruleID string) (MutationDiff, error) {
	if s == nil || s.service == nil || s.service.config.Automation == nil || ctx == nil || !validID(ruleID) {
		return MutationDiff{}, ErrInvalid
	}
	diagnostic, err := s.checkConflicts(ctx, token, workspace, RuleEnable)
	if err != nil {
		return MutationDiff{}, err
	}
	for _, rule := range diagnostic.snapshot.ActivationRules {
		if rule.ID != ruleID || !sameWorkspace(rule.WorkspaceID, diagnostic.Scope.WorkspaceID) {
			continue
		}
		if rule.Mode != commandactivation.ModeEvent || rule.Enabled {
			return MutationDiff{}, ErrInvalid
		}
		after := cloneActivationRule(rule)
		after.Enabled = true
		return MutationDiff{Operation: RuleEnable, Scope: cloneScope(diagnostic.Scope), RequiresDecision: true,
			BeforeActivationRules: []commandactivation.Rule{cloneActivationRule(rule)}, AfterActivationRules: []commandactivation.Rule{after}}, nil
	}
	return MutationDiff{}, ErrInvalid
}

func activationRuleFromAutomation(rule commandautomation.Rule) commandactivation.Rule {
	producers := stringSliceJSON(rule.AllowedInternalProducerTypes)
	return commandactivation.Rule{ID: rule.ID, UserID: rule.Owner.UserID, WorkspaceID: cloneWorkspace(rule.Owner.WorkspaceID),
		LayerRefKind: commandactivation.RefKind(rule.LayerRef.Kind), LayerRef: rule.LayerRef.Ref,
		RuleRefKind: commandactivation.RefKind(rule.RuleRef.Kind), RuleRef: rule.RuleRef.Ref,
		Mode: commandactivation.Mode(rule.Mode), Condition: rule.Condition, Lifecycle: commandactivation.Lifecycle(rule.Lifecycle),
		EventName: cloneWorkspace(&rule.EventName), AllowedInternalProducerTypes: cloneWorkspace(&producers),
		Enabled: rule.Enabled, Source: rule.Source, ReviewStatus: rule.ReviewStatus}
}

func stringSliceJSON(values []string) string {
	encoded, err := json.Marshal(values)
	if err == nil {
		return string(encoded)
	}
	return "[]"
}
