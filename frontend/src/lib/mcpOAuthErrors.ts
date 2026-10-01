import type { TFunction } from 'i18next';

export function mcpOAuthErrorMessage(error: unknown, t: TFunction): string {
  const message = error instanceof Error ? error.message : String(error ?? '');
  if (message.includes('oauth_registration_failed')) return t('mcp.error.registrationFailed');
  return message;
}
