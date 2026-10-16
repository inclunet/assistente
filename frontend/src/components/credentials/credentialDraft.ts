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

export interface CredentialValidation {
  message: string;
  fields: Array<keyof CredentialDraft>;
}

export function credentialValidation(item: CredentialDraft, t: (key: string) => string): CredentialValidation | null {
  const error = (key: string, ...fields: Array<keyof CredentialDraft>) => ({ message: t(key), fields });
  const required = (...fields: Array<keyof CredentialDraft>) => error('credentials.sourceFields.required', ...fields);
  if (!item.pattern?.trim() || !item.type) return required(!item.pattern?.trim() ? 'pattern' : 'type');
  if (item.source === 'oauth') return error('credentials.sourceFields.oauthUnavailable', 'source');
  if (item.source === 'command') {
    if (!item.command?.trim()) return required('command');
    try {
      const args: unknown = JSON.parse(item.argsText || '[]');
      if (!Array.isArray(args) || !args.every(a => typeof a === 'string'))
        return error('credentials.sourceFields.invalidArgs', 'argsText');
    } catch { return error('credentials.sourceFields.invalidArgs', 'argsText'); }
    if (!Number.isInteger(item.timeoutSeconds) || !item.timeoutSeconds || item.timeoutSeconds < 1 || item.timeoutSeconds > 300)
      return error('credentials.sourceFields.invalidTimeout', 'timeoutSeconds');
  }
  if (item.source === 'env' && !item.token?.trim()) return required('token');
  if (item.source === 'keyring') {
    const target = Boolean(item.token?.trim());
    const pair = Boolean(item.keyringService?.trim() && item.keyringUser?.trim());
    if ((!target && !pair) || (target && (item.keyringService?.trim() || item.keyringUser?.trim())))
      return error('credentials.sourceFields.keyringChoice', 'token', 'keyringService', 'keyringUser');
  }
  if (item.type === 'basic' && !item.username?.trim()) return required('username');
  if (item.type === 'custom' && !item.headerName?.trim()) return required('headerName');
  if (item.source === 'static') {
    const field = item.type === 'basic' ? 'password' : item.type === 'custom' ? 'headerValue' : 'token';
    if (!item[field]?.trim()) return required(field);
  }
  return null;
}

export function validateCredential(item: CredentialDraft, t: (key: string) => string): string | null {
  return credentialValidation(item, t)?.message ?? null;
}
