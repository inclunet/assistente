import { apidto } from '@wailsjs/go/models';

export interface CredentialDraft {
  id: string;
  pattern: string;
  type: string;
  source: string;
  command?: string;
  argsText?: string;
  timeoutSeconds?: number;
  keyringService?: string;
  keyringUser?: string;
  masked: string;
  managed: boolean;
  token?: string;
  username?: string;
  password?: string;
  headerName?: string;
  headerValue?: string;
  [key: string]: unknown;
}

export function newCredential(pattern = '', type = 'bearer'): CredentialDraft {
  return {
    id: pattern,
    pattern,
    type,
    source: 'static',
    argsText: '[]',
    timeoutSeconds: 30,
    masked: '',
    managed: false,
    token: '',
    username: '',
    password: '',
    headerName: '',
    headerValue: '',
  };
}

export function credentialFromSummary(found: apidto.CredentialSummary): CredentialDraft {
  return {
    ...newCredential(found.pattern, found.type),
    source: found.source,
    command: found.sourceConfig?.command || '',
    argsText: JSON.stringify(found.sourceConfig?.args || []),
    timeoutSeconds: found.sourceConfig?.timeoutSeconds || 30,
    keyringService: found.sourceConfig?.keyringService || '',
    keyringUser: found.sourceConfig?.keyringUser || '',
    masked: found.masked || '',
    managed: found.managed ?? false,
    token: found.sourceConfig?.env || found.sourceConfig?.keyringTarget || '',
    username: found.username || '',
    headerName: found.headerName || '',
  };
}

export function credentialInput(data: CredentialDraft): apidto.CredentialInput {
  return apidto.CredentialInput.createFrom({
    pattern: data.pattern,
    type: data.type,
    source: data.source,
    sourceConfig:
      data.source === 'static'
        ? undefined
        : {
            env: data.source === 'env' ? data.token : undefined,
            keyringTarget: data.source === 'keyring' ? data.token : undefined,
            keyringService: data.source === 'keyring' ? data.keyringService : undefined,
            keyringUser: data.source === 'keyring' ? data.keyringUser : undefined,
            command: data.source === 'command' ? data.command : undefined,
            args: data.source === 'command' ? JSON.parse(data.argsText || '[]') : undefined,
            timeoutSeconds: data.source === 'command' ? data.timeoutSeconds : undefined,
          },
    token:
      data.source === 'static' && ['bearer', 'secret'].includes(data.type) ? data.token : undefined,
    username: data.username,
    password: data.source === 'static' && data.type === 'basic' ? data.password : undefined,
    headerName: data.headerName,
    headerValue: data.source === 'static' && data.type === 'custom' ? data.headerValue : undefined,
  });
}

export function validateCredential(
  item: CredentialDraft,
  t: (key: string) => string
): string | null {
  if (!item.pattern?.trim() || !item.type) return t('credentials.sourceFields.required');
  if (item.source === 'oauth') return t('credentials.sourceFields.oauthUnavailable');
  if (item.source === 'command') {
    if (!item.command?.trim()) return t('credentials.sourceFields.required');
    try {
      const args: unknown = JSON.parse(item.argsText || '[]');
      if (!Array.isArray(args) || !args.every((a) => typeof a === 'string'))
        return t('credentials.sourceFields.invalidArgs');
    } catch {
      return t('credentials.sourceFields.invalidArgs');
    }
    if (
      !Number.isInteger(item.timeoutSeconds) ||
      !item.timeoutSeconds ||
      item.timeoutSeconds < 1 ||
      item.timeoutSeconds > 300
    )
      return t('credentials.sourceFields.invalidTimeout');
  }
  if (item.source === 'env' && !item.token?.trim()) return t('credentials.sourceFields.required');
  if (item.source === 'keyring') {
    const target = Boolean(item.token?.trim());
    const pair = Boolean(item.keyringService?.trim() && item.keyringUser?.trim());
    if ((!target && !pair) || (target && (item.keyringService?.trim() || item.keyringUser?.trim())))
      return t('credentials.sourceFields.keyringChoice');
  }
  if (item.type === 'basic' && !item.username?.trim())
    return t('credentials.sourceFields.required');
  if (item.type === 'custom' && !item.headerName?.trim())
    return t('credentials.sourceFields.required');
  if (item.source === 'static') {
    const value =
      item.type === 'basic'
        ? item.password
        : item.type === 'custom'
          ? item.headerValue
          : item.token;
    if (!value?.trim()) return t('credentials.sourceFields.required');
  }
  return null;
}
