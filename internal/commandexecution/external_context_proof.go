package commandexecution

import "context"

// AuthenticatedExternalPrincipalFromContext exposes the authenticated external
// principal without exposing the JWT or allowing callers to construct the
// private credential marker. It is intended for handlers that must bind an
// admitted envelope to the authenticated request context.
func AuthenticatedExternalPrincipalFromContext(ctx context.Context) (ExternalUIPrincipal, bool) {
	if ctx == nil {
		return ExternalUIPrincipal{}, false
	}
	credential, ok := ctx.Value(externalCredentialKey{}).(externalCredential)
	if !ok || credential.service == nil || credential.token == "" || credential.userID == "" ||
		credential.userID != credential.principal.UserID || credential.principal.Issuer == "" ||
		credential.principal.Subject == "" || credential.principal.AuthContextID == "" {
		return ExternalUIPrincipal{}, false
	}
	return credential.principal, true
}
