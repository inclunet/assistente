import { describe, expect, it, vi } from 'vitest';
import { applyRichTextInsert, type RichTextChain, type RichTextEditorLike } from './richTextPatchApply';

function createRichMock(params?: { editable?: boolean; runThrows?: boolean }) {
  const calls: Array<unknown[]> = [];

  const chain: RichTextChain = {
    focus: () => {
      calls.push(['focus']);
      return chain;
    },
    setTextSelection: (range) => {
      calls.push(['setTextSelection', range]);
      return chain;
    },
    insertContent: (content) => {
      calls.push(['insertContent', content]);
      return chain;
    },
    run: () => {
      calls.push(['run']);
      if (params?.runThrows) throw new Error('boom');
      return true;
    },
  };

  const setEditable = vi.fn((editable: boolean) => {
    calls.push(['setEditable', editable]);
  });

  const rich: RichTextEditorLike = {
    isEditable: !!params?.editable,
    setEditable,
    chain: () => chain,
  };

  return { rich, calls, setEditable };
}

describe('applyRichTextInsert', () => {
  it('faz replace na seleção (from/to) chamando setTextSelection + insertContent + run', () => {
    const { rich, calls } = createRichMock({ editable: true });

    applyRichTextInsert({
      rich,
      from: 10,
      to: 15,
      contentToInsert: '<p>ok</p>',
    });

    expect(calls).toEqual([
      ['focus'],
      ['setTextSelection', { from: 10, to: 15 }],
      ['insertContent', '<p>ok</p>'],
      ['run'],
    ]);
  });

  it('insere no cursor quando from === to', () => {
    const { rich, calls } = createRichMock({ editable: true });

    applyRichTextInsert({
      rich,
      from: 7,
      to: 7,
      contentToInsert: 'X',
    });

    expect(calls).toEqual([
      ['focus'],
      ['setTextSelection', { from: 7, to: 7 }],
      ['insertContent', 'X'],
      ['run'],
    ]);
  });

  it('quando não está editável, faz setEditable(true) e depois reverte para false', () => {
    const { rich, calls, setEditable } = createRichMock({ editable: false });

    applyRichTextInsert({
      rich,
      from: 1,
      to: 2,
      contentToInsert: 'ok',
    });

    expect(setEditable).toHaveBeenCalledTimes(2);
    expect(calls[0]).toEqual(['setEditable', true]);
    expect(calls[calls.length - 1]).toEqual(['setEditable', false]);
  });

  it('reverte setEditable(false) mesmo se run lançar erro', () => {
    const { rich, calls } = createRichMock({ editable: false, runThrows: true });

    expect(() =>
      applyRichTextInsert({
        rich,
        from: 1,
        to: 1,
        contentToInsert: 'x',
      }),
    ).toThrow('boom');

    expect(calls[0]).toEqual(['setEditable', true]);
    expect(calls[calls.length - 1]).toEqual(['setEditable', false]);
  });
});
