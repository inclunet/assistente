import { describe, expect, it } from 'vitest';
import { buildMarkdownLinkFromSelection, normalizePastedLinkHref } from './linkPaste';

describe('normalizePastedLinkHref', () => {
  it('aceita http/https e remove whitespace ao redor', () => {
    expect(normalizePastedLinkHref('  https://example.com  ')).toBe('https://example.com');
    expect(normalizePastedLinkHref('http://example.com')).toBe('http://example.com');
  });

  it('aceita caminhos relativos e âncora', () => {
    expect(normalizePastedLinkHref('/docs')).toBe('/docs');
    expect(normalizePastedLinkHref('./rel')).toBe('./rel');
    expect(normalizePastedLinkHref('../up')).toBe('../up');
    expect(normalizePastedLinkHref('#secao')).toBe('#secao');
  });

  it('aceita www.* e normaliza para https://', () => {
    expect(normalizePastedLinkHref('www.example.com')).toBe('https://www.example.com');
  });

  it('não trata texto comum como link', () => {
    expect(normalizePastedLinkHref('abc')).toBe(null);
    expect(normalizePastedLinkHref('isso nao e')).toBe(null);
  });
});

describe('buildMarkdownLinkFromSelection', () => {
  it('gera um link markdown usando o texto selecionado como label', () => {
    const md = buildMarkdownLinkFromSelection({ selectedText: 'Clique aqui', href: 'https://example.com' });
    expect(md).toBe('[Clique aqui](<https://example.com>)');
  });

  it('escapa colchetes no label', () => {
    const md = buildMarkdownLinkFromSelection({ selectedText: 'a[b]c', href: 'https://example.com' });
    expect(md).toBe('[a\\[b\\]c](<https://example.com>)');
  });
});
