import type { TFunction } from 'i18next';

export function mcpOAuthErrorMessage(error: unknown, t: TFunction): string {
  const message = error instanceof Error ? error.message : String(error ?? '');
  if (message.includes('oauth_resource_destination_blocked')) return t('mcp.error.resourceDestinationBlocked');
  if (message.includes('oauth_discovery_destination_blocked')) return t('mcp.error.networkAuthorizationFailed');
  if (message.includes('oauth_authorization_changed')) return t('mcp.error.authorizationChanged');
  if (message.includes('oauth_legacy_persistence_failed')) return t('mcp.error.legacyPersistenceFailed');
  if (message.includes('oauth_client_configuration_required')) return t('mcp.error.clientConfigurationRequired');
  if (message.includes('oauth_public_client_secret_not_allowed')) return t('mcp.error.publicClientSecret');
  if (message.includes('oauth_permission_missing')) return t('mcp.error.authorizationPermissions');
  if (message.includes('oauth_reauthorization_required')) return t('mcp.error.authorizationRequired');
  if (message.includes('oauth_temporarily_unavailable')) return t('mcp.error.authorizationTemporary');
  if (message.includes('oauth_resource_not_authorized')) return t('mcp.error.authorizationMismatch');
  if (message.includes('oauth_registration_failed')) return t('mcp.error.registrationFailed');
  if (message.includes('oauth_callback_port_unavailable')) return t('mcp.error.callbackPortUnavailable');
  if (message.includes('oauth_consent_declined') || message.includes('oauth_device_grant_failed: access_denied')) return t('mcp.error.consentDeclined');
  if (message.includes('oauth_device_grant_failed: expired_token')) return t('mcp.error.deviceExpired');
  if (message.includes('oauth_device_grant_failed')) return t('mcp.error.deviceFailed');
  if (message.includes('oauth_code_exchange_failed')) return t('mcp.error.codeExchangeFailed');
  return message;
}
