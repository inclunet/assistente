import { describe, expect, it } from 'vitest';
import { markdownToHtml } from './markdownToHtml';

describe('markdownToHtml', () => {
  it('renderiza links markdown em <a href=...>', () => {
    const html = markdownToHtml('[site](https://example.com)');
    expect(html).toContain('<a href="https://example.com">site</a>');
  });

  it('renderiza tabelas (pipe tables) em <table>', () => {
    const md = [
      '| A | B |',
      '| --- | --- |',
      '| 1 | 2 |',
    ].join('\n');

    const html = markdownToHtml(md);
    expect(html).toContain('<table>');
    expect(html).toContain('<thead>');
    expect(html).toContain('<tbody>');
  });
});
