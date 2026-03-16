import { describe, expect, it } from 'vitest';
import { stripMarkdown } from './stripMarkdown';

describe('stripMarkdown', () => {
  it('remove markup basico', () => {
    const text = '# Titulo\n\n**negrito** e *italico* com [link](http://x).';
    expect(stripMarkdown(text)).toBe('Titulo\n\nnegrito e italico com link.');
  });

  it('remove bloco de codigo', () => {
    const text = '```js\nconsole.log(1)\n```';
    expect(stripMarkdown(text)).toBe('bloco de código');
  });
});
