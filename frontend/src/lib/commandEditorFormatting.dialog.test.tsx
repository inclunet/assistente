import { afterEach, describe, expect, it } from 'vitest';
import { act, cleanup, render, waitFor } from '@testing-library/react';
import { Editor } from '@tiptap/core';
import { QuestionnaireDialog } from '../components/ui/QuestionnaireDialog';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { captureEditorFormatting, registerEditorFormatting } from './commandEditorFormatting';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { isModalOpen } from './modalRegistry';

function Host() {
  const active = useQuestionnaireUIStore(s => s.active);
  return <QuestionnaireDialog isOpen={active !== null} data={active}
    onSubmit={useQuestionnaireUIStore.getState().submit}
    onCancel={useQuestionnaireUIStore.getState().cancel} />;
}

afterEach(cleanup);

describe('preparação com QuestionnaireDialog e Modal reais', () => {
  it.each(['confirmar', 'cancelar'])('restaura foco no editor após %s o formulário, sem foco tardio no modal', async outcome => {
    const root = document.createElement('div');
    const fallback = document.createElement('button');
    document.body.append(root, fallback);
    fallback.focus();
    const editor = new Editor({ element: root, content: 'alpha', extensions: buildRichTextExtensions({
      placeholder: '', imageFallbackLabel: 'image', imageLabelPrefix: 'image',
    }) });
    const unregister = registerEditorFormatting({ root, editor, isCurrent: () => true, subscribe: () => () => {} });
    render(<Host />);
    let pending!: Promise<boolean>;
    const target = captureEditorFormatting()!;
    try {
      act(() => { pending = target.prepare('editor.format.table.insert'); });
      await waitFor(() => expect(isModalOpen()).toBe(true));
      act(() => {
        if (outcome === 'confirmar') useQuestionnaireUIStore.getState().submit({ rows: '2', cols: '3', withHeaderRow: true });
        else useQuestionnaireUIStore.getState().cancel();
      });
      expect(await pending).toBe(outcome === 'confirmar');
      expect(isModalOpen()).toBe(false);
      expect(document.activeElement).toBe(editor.view.dom);
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
      expect(document.activeElement).toBe(editor.view.dom);
      expect(target.execute('editor.format.table.insert')).toBe(outcome === 'confirmar');
      if (outcome === 'cancelar') expect(editor.state.doc.textContent).toBe('alpha');
    } finally {
      target.dispose(); unregister(); cleanup(); editor.destroy(); root.remove(); fallback.remove();
    }
  });
});
