import type { TFunction } from 'i18next';

export function mcpOAuthErrorKey(error: unknown): string | undefined {
  const message = error instanceof Error ? error.message : String(error ?? '');
  if (message.includes('oauth_migration_required')) return 'mcp.error.migrationRequired';
  if (message.includes('oauth_authentication_selection_required')) return 'mcp.error.authenticationSelectionRequired';
  if (message.includes('oauth_resource_destination_blocked')) return 'mcp.error.resourceDestinationBlocked';
  if (message.includes('oauth_discovery_destination_blocked')) return 'mcp.error.networkAuthorizationFailed';
  if (message.includes('oauth_authorization_changed')) return 'mcp.error.authorizationChanged';
  if (message.includes('oauth_legacy_persistence_failed')) return 'mcp.error.legacyPersistenceFailed';
  if (message.includes('oauth_request_not_replayable')) return 'mcp.error.requestNotReplayable';
  if (message.includes('oauth_client_configuration_required')) return 'mcp.error.clientConfigurationRequired';
  if (message.includes('oauth_public_client_secret_not_allowed')) return 'mcp.error.publicClientSecret';
  if (message.includes('oauth_permission_missing')) return 'mcp.error.authorizationPermissions';
  if (message.includes('oauth_reauthorization_required')) return 'mcp.error.authorizationRequired';
  if (message.includes('oauth_temporarily_unavailable')) return 'mcp.error.authorizationTemporary';
  if (message.includes('oauth_resource_not_authorized')) return 'mcp.error.authorizationMismatch';
  if (message.includes('oauth_registration_failed')) return 'mcp.error.registrationFailed';
  if (message.includes('oauth_callback_port_unavailable')) return 'mcp.error.callbackPortUnavailable';
  if (message.includes('oauth_consent_declined') || message.includes('oauth_device_grant_failed: access_denied')) return 'mcp.error.consentDeclined';
  if (message.includes('oauth_device_grant_failed: expired_token')) return 'mcp.error.deviceExpired';
  if (message.includes('oauth_device_grant_failed')) return 'mcp.error.deviceFailed';
  if (message.includes('oauth_code_exchange_failed')) return 'mcp.error.codeExchangeFailed';
  return undefined;
}

export function mcpOAuthErrorMessage(error: unknown, t: TFunction, fallback?: string): string {
  const key = mcpOAuthErrorKey(error);
  return key ? t(key) : fallback ?? (error instanceof Error ? error.message : String(error ?? ''));
}
