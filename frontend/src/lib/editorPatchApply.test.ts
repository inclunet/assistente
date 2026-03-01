import { describe, expect, it } from 'vitest';
import { applyTextReplacementByOffset } from './editorPatchApply';

describe('applyTextReplacementByOffset', () => {
  it('substitui exatamente o trecho selecionado', () => {
    const res = applyTextReplacementByOffset({
      current: 'abcDEFghi',
      startOffset: 3,
      endOffset: 6,
      expectedSelectedText: 'DEF',
      replacement: 'XYZ',
    });

    expect(res).toEqual({ ok: true, nextText: 'abcXYZghi' });
  });

  it('insere no cursor quando seleção é vazia', () => {
    const res = applyTextReplacementByOffset({
      current: 'abc',
      startOffset: 1,
      endOffset: 1,
      expectedSelectedText: '',
      replacement: '_',
    });

    expect(res).toEqual({ ok: true, nextText: 'a_bc' });
  });

  it('falha se o texto selecionado não bate com o conteúdo atual', () => {
    const res = applyTextReplacementByOffset({
      current: 'abcDEFghi',
      startOffset: 3,
      endOffset: 6,
      expectedSelectedText: 'DEf',
      replacement: 'XYZ',
    });

    expect(res).toEqual({ ok: false, error: 'selected_text_mismatch' });
  });

  it('falha para offsets fora dos limites', () => {
    const res = applyTextReplacementByOffset({
      current: 'abc',
      startOffset: 0,
      endOffset: 99,
      expectedSelectedText: 'abc',
      replacement: 'x',
    });

    expect(res).toEqual({ ok: false, error: 'offset_out_of_bounds' });
  });
});
