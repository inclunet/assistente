const REDACTED = '[redacted]';
const MAX_INPUT_BYTES = 256 * 1024;
const SENSITIVE_TOKENS = [
  'api_key', 'access_key', 'authorization', 'client_secret', 'cookie',
  'credential', 'jwt', 'password', 'private_key', 'refresh_token',
  'secret', 'session', 'session_id', 'token',
];

function sensitiveKey(key: string): boolean {
  const normalized = key.toLowerCase().replace(/-/g, '_');
  const compact = normalized.replace(/[_. ]/g, '');
  return SENSITIVE_TOKENS.some((token) => normalized.includes(token) || compact.includes(token.replace(/_/g, '')));
}

function sensitiveString(value: string): boolean {
  const trimmed = value.trim();
  return /^bearer\s/i.test(trimmed) || trimmed.startsWith('sk-') || trimmed.includes('-----BEGIN') ||
    (trimmed.length >= 40 && trimmed.split('.').length >= 3 && !/\s/.test(trimmed));
}

function sanitize(value: unknown, key = '', depth = 0): unknown {
  if (depth > 30 || sensitiveKey(key)) return REDACTED;
  if (typeof value === 'string') {
    if (sensitiveString(value)) return REDACTED;
    // Alguns registros legados guardam argumentos JSON como string dentro do
    // envelope da chamada. Sanitizá-los evita uma rota de bypass por nesting.
    try { return JSON.stringify(sanitize(JSON.parse(value), '', depth + 1)); } catch { return value; }
  }
  if (Array.isArray(value)) return value.map((item) => sanitize(item, '', depth + 1));
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value as Record<string, unknown>)
      .map(([childKey, childValue]) => [childKey, sanitize(childValue, childKey, depth + 1)]));
  }
  return value;
}

/** Defesa no cliente para detalhes legados; o ledger continua sendo a auditoria. */
export function sanitizeToolDetailArguments(raw: string): string {
  if (new TextEncoder().encode(raw).byteLength > MAX_INPUT_BYTES) return '{"_redacted":true,"_too_large":true}';
  try { return JSON.stringify(sanitize(JSON.parse(raw))); } catch { return '[redacted: unstructured arguments]'; }
}
