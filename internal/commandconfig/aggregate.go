package commandconfig

import (
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

func cloneActivationRule(rule commandactivation.Rule) commandactivation.Rule {
	rule.WorkspaceID = cloneWorkspace(rule.WorkspaceID)
	rule.EventName = cloneWorkspace(rule.EventName)
	rule.AllowedInternalProducerTypes = cloneWorkspace(rule.AllowedInternalProducerTypes)
	rule.AuthorizationDecisionID = cloneWorkspace(rule.AuthorizationDecisionID)
	rule.AutomationGrantID = cloneWorkspace(rule.AutomationGrantID)
	rule.AutomationGrantFingerprint = cloneWorkspace(rule.AutomationGrantFingerprint)
	rule.ReplacesDefaultID = cloneWorkspace(rule.ReplacesDefaultID)
	rule.ReplacesDefaultVersion = cloneWorkspace(rule.ReplacesDefaultVersion)
	rule.ReplacesDefaultFingerprint = cloneWorkspace(rule.ReplacesDefaultFingerprint)
	if rule.AutomationGrantGeneration != nil {
		value := *rule.AutomationGrantGeneration
		rule.AutomationGrantGeneration = &value
	}
	return rule
}

func cloneActivationRules(rows []commandactivation.Rule) []commandactivation.Rule {
	if rows == nil {
		return nil
	}
	result := make([]commandactivation.Rule, len(rows))
	for i, row := range rows {
		result[i] = cloneActivationRule(row)
	}
	return result
}

func cloneActivationClaim(claim commandactivation.Claim) commandactivation.Claim {
	claim.WorkspaceID = cloneWorkspace(claim.WorkspaceID)
	claim.SourceInstanceID = cloneWorkspace(claim.SourceInstanceID)
	claim.SourceEventID = cloneWorkspace(claim.SourceEventID)
	claim.SourceCorrelationID = cloneWorkspace(claim.SourceCorrelationID)
	claim.SourceJobDatabaseID = cloneWorkspace(claim.SourceJobDatabaseID)
	claim.SourceJobSlug = cloneWorkspace(claim.SourceJobSlug)
	claim.EventFingerprint = cloneWorkspace(claim.EventFingerprint)
	claim.SourceReplayPolicyGeneration = cloneWorkspace(claim.SourceReplayPolicyGeneration)
	claim.TerminalReason = cloneWorkspace(claim.TerminalReason)
	claim.Provenance = cloneWorkspace(claim.Provenance)
	claim.ManualStackKey = cloneWorkspace(claim.ManualStackKey)
	if claim.Sequence != nil {
		value := *claim.Sequence
		claim.Sequence = &value
	}
	if claim.SourceReplayDeadline != nil {
		value := claim.SourceReplayDeadline.UTC()
		claim.SourceReplayDeadline = &value
	}
	if claim.ExpiresAt != nil {
		value := claim.ExpiresAt.UTC()
		claim.ExpiresAt = &value
	}
	return claim
}

func cloneActivationClaims(rows []commandactivation.Claim) []commandactivation.Claim {
	if rows == nil {
		return nil
	}
	result := make([]commandactivation.Claim, len(rows))
	for i, row := range rows {
		result[i] = cloneActivationClaim(row)
	}
	return result
}

func cloneDocumentClaims(rows []commandactivation.Claim) []commandactivation.Claim {
	result := cloneActivationClaims(rows)
	for i := range result {
		// Provenance is an opaque event payload and is never part of a
		// persisted mutation document. The live diff keeps it available to
		// the in-process reconciler, while the audit receives no raw payload.
		result[i].Provenance = nil
	}
	return result
}

func cloneAutomationGrant(grant commandautomation.Grant) commandautomation.Grant {
	grant.Owner.WorkspaceID = cloneWorkspace(grant.Owner.WorkspaceID)
	grant.RevokedAt = cloneTime(grant.RevokedAt)
	grant.RevokedBy = cloneWorkspace(grant.RevokedBy)
	grant.RevocationReason = cloneWorkspace(grant.RevocationReason)
	return grant
}

func cloneAutomationGrants(rows []commandautomation.Grant) []commandautomation.Grant {
	if rows == nil {
		return nil
	}
	result := make([]commandautomation.Grant, len(rows))
	for i, row := range rows {
		result[i] = cloneAutomationGrant(row)
	}
	return result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func sameAggregateSnapshot(left, right Snapshot) bool {
	return equalRows(left.ActivationRules, right.ActivationRules) &&
		equalRows(left.AutomationGrants, right.AutomationGrants) &&
		equalRows(left.ActivationClaims, right.ActivationClaims)
}
