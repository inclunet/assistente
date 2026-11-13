/** @vitest-environment node */
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

describe('página de retorno OAuth MCP', () => {
  it.each([
    ['pt-BR', 'received', 'Retorno recebido', 'pt-BR'], ['pt-BR', 'error', 'Falha na autorização', 'pt-BR'],
    ['en-US', 'received', 'Return received', 'en'], ['en-US', 'error', 'Authorization failed', 'en'],
    ['es-ES', 'received', 'Retorno recibido', 'es'], ['es-ES', 'error', 'Error de autorización', 'es'],
    ['fr-FR', 'error', 'Authorization failed', 'en'],
  ])('informa %s / %s sem inferir conclusão', (language, outcome, heading, expectedLanguage) => {
    const source = readFileSync(new URL('../../../internal/mcp/oauth.go', import.meta.url), 'utf8');
    const script = source.match(/\(function\(\)\{[\s\S]*?\}\)\(\);/)?.[0];
    expect(script).toBeDefined();
    const title = { textContent: '' }, message = { textContent: '' };
    const document = { body: { dataset: { outcome } }, documentElement: { lang: '' }, title: '', getElementById: (id: string) => id === 'title' ? title : message };
    runInNewContext(script!, { navigator: { language }, document });
    expect(document.documentElement.lang).toBe(expectedLanguage);
    expect(title.textContent).toBe(heading);
    expect(message.textContent).not.toMatch(/concluída|completed|completada/i);
    expect(message.textContent).toMatch(/Assistente/);
  });
});
