package commandautomation

// ValidateGrant verifica apenas a forma e os vínculos canônicos persistidos;
// não transforma um grant em autorização para executar uma ferramenta.
func ValidateGrant(grant Grant) error {
	if !validUUID7(grant.ID) || !validOwner(grant.Owner) || !validRef(grant.LayerRef) || !validRef(grant.RuleRef) ||
		grant.EventName != JobRunStateEvent || grant.RuleFingerprint == "" || grant.ProducerTypesFingerprint == "" ||
		grant.AutomationGrantFingerprint == "" || grant.AutomationGrantGeneration < 1 || !validUUID7(grant.AuthorizationDecisionID) ||
		grant.GrantedAt.IsZero() || grant.GrantedBy == "" {
		return ErrInvalid
	}
	if grant.RevokedAt == nil && (grant.RevokedBy != nil || grant.RevocationReason != nil) {
		return ErrInvalid
	}
	if grant.RevokedAt != nil && grant.RevokedAt.IsZero() {
		return ErrInvalid
	}
	if grant.RevokedBy != nil && *grant.RevokedBy == "" || grant.RevocationReason != nil && *grant.RevocationReason == "" {
		return ErrInvalid
	}
	return nil
}
