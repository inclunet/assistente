import { describe, expect, it } from 'vitest';
import i18next from 'i18next';
import ptBR from '../locales/pt-BR';
import en from '../locales/en';
import es from '../locales/es';
import { mcpOAuthErrorMessage } from './mcpOAuthErrors';

describe('OAuth registration errors', () => {
  it.each(['pt-BR', 'en', 'es'])('localizes wrapped errors in %s', async (lng) => {
    const i18n = i18next.createInstance();
    await i18n.init({ lng, resources: { 'pt-BR': ptBR, en, es } });
    const result = mcpOAuthErrorMessage(new Error('failed to connect: oauth_registration_failed: HTTP 400'), i18n.t);
    expect(result).toBe(i18n.t('mcp.error.registrationFailed'));
    expect(result).not.toContain('oauth_registration_failed');
    expect(result).not.toBe('mcp.error.registrationFailed');
    expect(mcpOAuthErrorMessage('oauth_registration_failed', i18n.t)).toBe(result);
    const resource = mcpOAuthErrorMessage('oauth_resource_destination_blocked: oauth_discovery_destination_blocked', i18n.t);
    expect(resource).toBe(i18n.t('mcp.error.resourceDestinationBlocked'));
    expect(resource).not.toBe('mcp.error.resourceDestinationBlocked');
    expect(resource).not.toContain('oauth_resource_destination_blocked');
    const denied = mcpOAuthErrorMessage('oauth_registration_failed: oauth_discovery_destination_blocked', i18n.t);
    expect(denied).toBe(i18n.t('mcp.error.networkAuthorizationFailed'));
    expect(denied).not.toContain('oauth_discovery_destination_blocked');
    expect(denied).not.toBe('mcp.error.networkAuthorizationFailed');
    for (const [code, key] of [
      ['oauth_authorization_changed', 'authorizationChanged'],
      ['oauth_migration_required', 'migrationRequired'],
      ['oauth_authentication_selection_required', 'authenticationSelectionRequired'],
      ['oauth_legacy_persistence_failed', 'legacyPersistenceFailed'],
      ['oauth_request_not_replayable', 'requestNotReplayable'],
      ['oauth_reauthorization_required', 'authorizationRequired'],
      ['oauth_permission_missing', 'authorizationPermissions'],
      ['oauth_public_client_secret_not_allowed', 'publicClientSecret'],
      ['oauth_client_configuration_required', 'clientConfigurationRequired'],
      ['oauth_temporarily_unavailable', 'authorizationTemporary'],
      ['oauth_resource_not_authorized', 'authorizationMismatch'],
      ['oauth_callback_port_unavailable', 'callbackPortUnavailable'],
      ['oauth_consent_declined', 'consentDeclined'],
      ['oauth_device_grant_failed: access_denied', 'consentDeclined'],
      ['oauth_device_grant_failed: expired_token', 'deviceExpired'],
      ['oauth_device_grant_failed', 'deviceFailed'],
      ['oauth_code_exchange_failed', 'codeExchangeFailed'],
    ]) {
      const localized = mcpOAuthErrorMessage(`connection failed: ${code}`, i18n.t);
      expect(localized).toBe(i18n.t(`mcp.error.${key}`));
      expect(localized).not.toBe(`mcp.error.${key}`);
      expect(localized).not.toContain('oauth_');
    }
  });
  it('preserves unrelated failures and empty values', () => {
    expect(mcpOAuthErrorMessage(new Error('existing failure'), i18next.t)).toBe('existing failure');
    expect(mcpOAuthErrorMessage(null, i18next.t)).toBe('');
  });
});
