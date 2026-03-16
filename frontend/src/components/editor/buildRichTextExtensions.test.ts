import { describe, expect, it } from 'vitest';
import { buildRichTextExtensions } from './buildRichTextExtensions';

describe('buildRichTextExtensions', () => {
  it('retorna lista de extensoes', () => {
    const result = buildRichTextExtensions({ placeholder: 'x' });
    expect(Array.isArray(result)).toBe(true);
    expect(result.length).toBeGreaterThan(0);
  });
});
