import { describe, expect, it, vi } from 'vitest';
import { validateRichTextSelectionSnapshot } from './richTextSelectionValidation';

describe('validateRichTextSelectionSnapshot', () => {
  it('falha quando não consegue ler a seleção atual', () => {
    const res = validateRichTextSelectionSnapshot({
      currentSelection: null,
      expectedFrom: 1,
      expectedTo: 1,
      expectedEmpty: true,
    });

    expect(res).toEqual({ ok: false, reason: 'no_selection' });
  });

  it('falha quando o range mudou (from/to)', () => {
    const res = validateRichTextSelectionSnapshot({
      currentSelection: { from: 1, to: 2, empty: false },
      expectedFrom: 1,
      expectedTo: 3,
      expectedEmpty: false,
      expectedSelectedText: 'x',
      getCurrentSelectedText: () => 'x',
    });

    expect(res).toEqual({ ok: false, reason: 'range_changed' });
  });

  it('falha quando esperava seleção vazia mas não está vazia', () => {
    const res = validateRichTextSelectionSnapshot({
      currentSelection: { from: 5, to: 5, empty: false },
      expectedFrom: 5,
      expectedTo: 5,
      expectedEmpty: true,
    });

    expect(res).toEqual({ ok: false, reason: 'empty_mismatch' });
  });

  it('falha quando texto selecionado mudou (expectedEmpty=false)', () => {
    const res = validateRichTextSelectionSnapshot({
      currentSelection: { from: 1, to: 4, empty: false },
      expectedFrom: 1,
      expectedTo: 4,
      expectedEmpty: false,
      expectedSelectedText: 'abc',
      getCurrentSelectedText: () => 'abd',
    });

    expect(res).toEqual({ ok: false, reason: 'selected_text_mismatch' });
  });

  it('não chama getCurrentSelectedText quando expectedEmpty=true', () => {
    const getCurrentSelectedText = vi.fn(() => 'x');

    const res = validateRichTextSelectionSnapshot({
      currentSelection: { from: 2, to: 2, empty: true },
      expectedFrom: 2,
      expectedTo: 2,
      expectedEmpty: true,
      expectedSelectedText: 'nao-importa',
      getCurrentSelectedText,
    });

    expect(res).toEqual({ ok: true });
    expect(getCurrentSelectedText).not.toHaveBeenCalled();
  });

  it('retorna ok quando range/empty/texto batem', () => {
    const res = validateRichTextSelectionSnapshot({
      currentSelection: { from: 1, to: 4, empty: false },
      expectedFrom: 1,
      expectedTo: 4,
      expectedEmpty: false,
      expectedSelectedText: 'abc',
      getCurrentSelectedText: () => 'abc',
    });

    expect(res).toEqual({ ok: true });
  });

  it('falha com cannot_read_selected_text quando getCurrentSelectedText lança', () => {
    const res = validateRichTextSelectionSnapshot({
      currentSelection: { from: 1, to: 2, empty: false },
      expectedFrom: 1,
      expectedTo: 2,
      expectedEmpty: false,
      expectedSelectedText: 'x',
      getCurrentSelectedText: () => {
        throw new Error('boom');
      },
    });

    expect(res).toEqual({ ok: false, reason: 'cannot_read_selected_text' });
  });
});
