import { describe, expect, it, vi } from 'vitest';
import MarkdownIt from 'markdown-it';
import { markdownItDeepLink } from './markdownItDeepLink';

vi.mock('./i18n', () => ({
  default: { t: (key: string) => key },
}));

function render(markdown: string): string {
  const md = new MarkdownIt({ html: false, linkify: true });
  md.use(markdownItDeepLink);
  return md.render(markdown);
}

describe('markdownItDeepLink plugin', () => {
  it('adiciona classe deep-link em links assistente://', () => {
    const html = render('[Abrir conversa](assistente://conversation/42)');
    expect(html).toContain('class="deep-link deep-link--conversation"');
  });

  it('adiciona data-deep-link com a URI original', () => {
    const html = render('[Abrir](assistente://conversation/42)');
    expect(html).toContain('data-deep-link="assistente://conversation/42"');
  });

  it('adiciona role="link"', () => {
    const html = render('[Abrir](assistente://conversation/42)');
    expect(html).toContain('role="link"');
  });

  it('adiciona aria-label descritivo', () => {
    const html = render('[Abrir](assistente://conversation/42)');
    expect(html).toContain('aria-label="');
  });

  it('não altera links http normais', () => {
    const html = render('[Google](https://google.com)');
    expect(html).not.toContain('deep-link');
    expect(html).not.toContain('data-deep-link');
    expect(html).toContain('href="https://google.com"');
  });

  it('aplica classe correta para conversation:new', () => {
    const html = render('[Nova](assistente://conversation/new?message=oi)');
    expect(html).toContain('deep-link--new-conversation');
  });

  it('aplica classe correta para conversation:send', () => {
    const html = render('[Enviar](assistente://conversation/5/send?message=teste)');
    expect(html).toContain('deep-link--send');
  });

  it('aplica classe correta para navigate', () => {
    const html = render('[Histórico](assistente://navigate/history)');
    expect(html).toContain('deep-link--navigate');
  });

  it('preserva o texto do link', () => {
    const html = render('[Meu texto personalizado](assistente://conversation/42)');
    expect(html).toContain('Meu texto personalizado');
  });

  it('funciona com múltiplos links na mesma mensagem', () => {
    const md = [
      'Veja a [conversa 1](assistente://conversation/1) e',
      'a [conversa 2](assistente://conversation/2) ou',
      'vá para o [histórico](assistente://navigate/history).',
    ].join(' ');
    const html = render(md);

    expect(html).toContain('data-deep-link="assistente://conversation/1"');
    expect(html).toContain('data-deep-link="assistente://conversation/2"');
    expect(html).toContain('data-deep-link="assistente://navigate/history"');
  });

  it('funciona com mix de links normais e deep links', () => {
    const md = 'Veja o [site](https://example.com) e a [conversa](assistente://conversation/1).';
    const html = render(md);

    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('class="deep-link');
    // O link normal não deve ter deep-link
    const normalLinkMatch = html.match(/<a href="https:\/\/example\.com"[^>]*>/);
    expect(normalLinkMatch?.[0]).not.toContain('deep-link');
  });

  it('lida com URI inválido sem quebrar (fallback sem typeClass)', () => {
    const html = render('[Broken](assistente://invalid/resource)');
    expect(html).toContain('class="deep-link"');
    expect(html).toContain('data-deep-link="assistente://invalid/resource"');
  });
});
