import { describe, expect, it } from 'vitest';
import { Schema } from '@tiptap/pm/model';
import { EditorState } from '@tiptap/pm/state';
import { history, undoDepth } from '@tiptap/pm/history';

import { clearRichEditorHistory } from './richEditorHistory';
import type { TipTapEditor } from '../../pages/editorTypes';

const schema = new Schema({
  nodes: {
    doc: { content: 'paragraph+' },
    paragraph: { content: 'text*', toDOM: () => ['p', 0] },
    text: {},
  },
});

function createStateWithUndoHistory(): EditorState {
  let state = EditorState.create({ schema, plugins: [history()] });
  state = state.apply(state.tr.insertText('slide 1', 1));
  state = state.apply(state.tr.insertText(' editado', state.doc.content.size - 1));
  return state;
}

describe('clearRichEditorHistory', () => {
  it('zera o undo mantendo documento e plugins', () => {
    const state = createStateWithUndoHistory();
    expect(undoDepth(state)).toBeGreaterThan(0);

    let nextState: EditorState | null = null;
    const editor = {
      view: {
        state,
        updateState: (s: EditorState) => {
          nextState = s;
        },
      },
    } as unknown as TipTapEditor;

    clearRichEditorHistory(editor);

    expect(nextState).not.toBeNull();
    expect(undoDepth(nextState!)).toBe(0);
    expect(nextState!.doc.eq(state.doc)).toBe(true);
    expect(nextState!.plugins).toHaveLength(state.plugins.length);
  });

  it('é no-op sem editor ou sem view', () => {
    expect(() => clearRichEditorHistory(null)).not.toThrow();
    expect(() => clearRichEditorHistory({} as unknown as TipTapEditor)).not.toThrow();
  });
});
