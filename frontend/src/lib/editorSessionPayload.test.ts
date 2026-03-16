import { describe, expect, it } from 'vitest';
import { toEditorSessionPayload } from './editorSessionPayload';

describe('editorSessionPayload', () => {
  it('retorna objeto vazio para input invalido', () => {
    expect(toEditorSessionPayload(null)).toEqual({});
    expect(toEditorSessionPayload('x')).toEqual({});
  });

  it('retorna payload quando valido', () => {
    const payload = { version: 1, activeTabId: 'a' };
    expect(toEditorSessionPayload(payload)).toBe(payload);
  });
});
