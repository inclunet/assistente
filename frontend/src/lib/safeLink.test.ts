import { describe, expect, it } from 'vitest';
import { isSafeLinkHref } from './safeLink';

describe('isSafeLinkHref', () => {
  it('aceita http/https/mailto', () => {
    expect(isSafeLinkHref('https://example.com')).toBe(true);
    expect(isSafeLinkHref('http://example.com')).toBe(true);
    expect(isSafeLinkHref('mailto:test@example.com')).toBe(true);
  });

  it('aceita relativo e âncora', () => {
    expect(isSafeLinkHref('/docs')).toBe(true);
    expect(isSafeLinkHref('./rel')).toBe(true);
    expect(isSafeLinkHref('../up')).toBe(true);
    expect(isSafeLinkHref('algum-caminho')).toBe(true);
    expect(isSafeLinkHref('#secao')).toBe(true);
  });

  it('rejeita javascript/data e vazio', () => {
    expect(isSafeLinkHref('')).toBe(false);
    expect(isSafeLinkHref('   ')).toBe(false);
    expect(isSafeLinkHref('javascript:alert(1)')).toBe(false);
    expect(isSafeLinkHref('data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==')).toBe(false);
  });

  it('rejeita caracteres de controle', () => {
    expect(isSafeLinkHref('https://example.com\u0000')).toBe(false);
  });
});
