import { EditorState } from '@tiptap/pm/state';

import type { TipTapEditor } from '../../pages/editorTypes';

/**
 * Limpa o histórico de undo/redo do TipTap recriando o EditorState com o
 * documento e a seleção atuais (plugin states são reinicializados, incluindo
 * o prosemirror-history).
 *
 * Necessário porque, com a key estável por aba, a troca de slide Reveal não
 * remonta o editor: sem esta limpeza, um Ctrl+Z após a troca restauraria o
 * conteúdo do slide anterior e `handleRichMarkdownChange` persistiria esse
 * conteúdo no slide atual, corrompendo o deck.
 */
export function clearRichEditorHistory(editor: TipTapEditor | null): void {
  try {
    const view = editor?.view;
    const state = view?.state;
    if (!view || !state) return;
    view.updateState(
      EditorState.create({
        doc: state.doc,
        selection: state.selection,
        plugins: state.plugins,
      })
    );
  } catch {
    // best-effort: editor destruído ou instância fake em testes
  }
}
