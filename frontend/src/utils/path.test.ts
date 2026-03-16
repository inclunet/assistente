import { describe, expect, it } from 'vitest';
import { basenameFromPath, normalizePathKey } from './path';

describe('path utils', () => {
  it('normaliza caminho', () => {
    expect(normalizePathKey('a\\b\\c')).toBe('a/b/c');
  });

  it('retorna basename', () => {
    expect(basenameFromPath('a/b/c.txt')).toBe('c.txt');
  });
});
