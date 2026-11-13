/** @vitest-environment node */
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it, vi } from 'vitest';

describe('idioma do retorno OAuth MCP', () => {
  it.each([
    ['pt-BR', 'pt-BR', 'Retorno recebido'],
    ['en-US', 'en', 'Return received'],
    ['es-ES', 'es', 'Retorno recibido'],
    ['fr-FR', 'en', 'Return received'],
  ])('usa o idioma do navegador %s, com fallback inglês', (browserLanguage, expectedLanguage, heading) => {
    const source = readFileSync(new URL('../../../internal/mcp/oauth.go', import.meta.url), 'utf8');
    const script = source.match(/\(function\(\)\{[\s\S]*?\}\)\(\);/)?.[0];
    expect(script).toBeDefined();
    const title = { textContent: '' };
    const message = { textContent: '' };
    const document = { documentElement: { lang: 'en' }, title: '', getElementById: (id: string) => id === 'title' ? title : message };
    const close = vi.fn();
    runInNewContext(script!, { navigator: { language: browserLanguage }, document, window: { close }, setTimeout: (callback: () => void) => callback() });
    expect(document.documentElement.lang).toBe(expectedLanguage);
    expect(title.textContent).toBe(heading);
    expect(close).toHaveBeenCalledOnce();
  });
});
