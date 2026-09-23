import { describe, expect, it } from 'vitest';
import { sanitizeToolDetailArguments } from './toolDetailSanitization';

describe('sanitizeToolDetailArguments', () => {
  it('redige chaves sensíveis inclusive dentro de JSON serializado', () => {
    const value = sanitizeToolDetailArguments(JSON.stringify({
      password: 'hunter2', nested: { api_key: 'sk-secret' },
      arguments: JSON.stringify({ authorization: 'Bearer secret-token' }),
      query: 'manter',
    }));
    const parsed = JSON.parse(value) as { password: string; nested: { api_key: string }; arguments: string; query: string };
    expect(parsed.password).toBe('[redacted]');
    expect(parsed.nested.api_key).toBe('[redacted]');
    expect(JSON.parse(parsed.arguments)).toEqual({ authorization: '[redacted]' });
    expect(parsed.query).toBe('manter');
    expect(value).not.toContain('hunter2');
    expect(value).not.toContain('secret-token');
  });

  it('falha de forma segura para argumentos sem JSON', () => {
    expect(sanitizeToolDetailArguments('password=exposto')).toBe('[redacted: unstructured arguments]');
  });
});
