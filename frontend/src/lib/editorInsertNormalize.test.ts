import { describe, expect, test } from 'vitest';
import { normalizeEditorInsertContent, __private__ } from './editorInsertNormalize';

describe('editorInsertNormalize', () => {
  test('detecta HTML simples', () => {
    expect(__private__.looksLikeHtml('<p>oi</p>')).toBe(true);
    expect(__private__.looksLikeHtml('**markdown**')).toBe(false);
    expect(__private__.looksLikeHtml('```\n<div>code</div>\n```')).toBe(false);
  });

  test('markdown target: converte HTML para markdown legível', () => {
    const out = normalizeEditorInsertContent({
      targetMode: 'markdown',
      format: 'markdown',
      content: '<p><strong>Olá</strong><br>mundo</p><ul><li>A</li><li>B</li></ul>',
    });

    expect(out.format).toBe('markdown');
    expect(out.content).not.toMatch(/<\/?\w+/);
    expect(out.content).toContain('**Olá**');
    expect(out.content).toContain('mundo');
    expect(out.content).toContain('- A');
    expect(out.content).toContain('- B');
  });

  test('plain target: remove tags e preserva quebras', () => {
    const out = normalizeEditorInsertContent({
      targetMode: 'markdown',
      format: 'plain',
      content: '<p>linha1<br>linha2</p>',
    });

    expect(out.format).toBe('plain');
    expect(out.content).toBe('linha1\nlinha2');
  });

  test('rich target: se pediu markdown mas veio HTML, troca para html', () => {
    const out = normalizeEditorInsertContent({
      targetMode: 'rich',
      format: 'markdown',
      content: '<p>Oi <em>mundo</em></p>',
    });

    expect(out.format).toBe('html');
    expect(out.content).toBe('<p>Oi <em>mundo</em></p>');
  });

  test('rich target: se veio HTML escapado (&lt;...&gt;), decodifica e troca para html', () => {
    const out = normalizeEditorInsertContent({
      targetMode: 'rich',
      format: 'markdown',
      content: '&lt;p&gt;Oi &lt;em&gt;mundo&lt;/em&gt;&lt;/p&gt;',
    });

    expect(out.format).toBe('html');
    expect(out.content).toBe('<p>Oi <em>mundo</em></p>');
  });

  test('rich target: se veio <pre><code> contendo markdown, desembrulha e mantém markdown (não vira code fence)', () => {
    const out = normalizeEditorInsertContent({
      targetMode: 'rich',
      format: 'markdown',
      content: '<pre><code># Título\n\nTexto</code></pre>',
    });

    expect(out.format).toBe('markdown');
    expect(out.content).toBe('# Título\n\nTexto');
  });

  test('rich target: <pre><code> escapado como entidades também é desembrulhado para markdown', () => {
    const out = normalizeEditorInsertContent({
      targetMode: 'rich',
      format: 'markdown',
      content: '&lt;pre&gt;&lt;code&gt;# Título\n\nTexto&lt;/code&gt;&lt;/pre&gt;',
    });

    expect(out.format).toBe('markdown');
    expect(out.content).toBe('# Título\n\nTexto');
  });
});
