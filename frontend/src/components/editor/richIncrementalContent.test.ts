import { afterEach, describe, expect, it, vi } from 'vitest';
import { Editor } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { Markdown } from 'tiptap-markdown';
import type { Transaction } from '@tiptap/pm/state';

import {
  applyExternalMarkdownIncrementally,
  parseExternalMarkdownToDoc,
  type IncrementalEditorLike,
} from './richIncrementalContent';
import {
  createRichMarkdownSyncRefs,
  disposeRichMarkdownSync,
  getMarkdownNow,
  onUpdate,
  syncFromExternal,
  type EditorLike,
  type RichMarkdownSyncRefs,
} from './richMarkdownSync';

/**
 * Editor TipTap REAL (StarterKit + tiptap-markdown) em jsdom, sem node views
 * custom, para exercitar o parser e o diff de ProseMirror de verdade.
 */
function createRealEditor(markdown: string): Editor {
  return new Editor({
    extensions: [StarterKit, Markdown.configure({ html: false })],
    content: markdown,
  });
}

/**
 * Simula o primeiro syncFromExternal do mount (rebase do baseline para o
 * round-trip serializado), deixando os refs prontos para syncs subsequentes.
 */
function mountRefs(editor: Editor, initialMarkdown: string): RichMarkdownSyncRefs {
  const refs = createRichMarkdownSyncRefs(initialMarkdown);
  syncFromExternal({ refs, editor: editor as unknown as EditorLike, nextMarkdown: initialMarkdown });
  return refs;
}

/** Round-trip serializado do editor (o storage do tiptap-markdown não é tipado). */
function serialize(editor: Editor): string {
  return getMarkdownNow(editor as unknown as EditorLike);
}

function collectDocChanges(editor: Editor): Transaction[] {
  const docTransactions: Transaction[] = [];
  editor.on('transaction', ({ transaction }) => {
    if (transaction.docChanged) docTransactions.push(transaction);
  });
  return docTransactions;
}

const editors: Editor[] = [];

function track(editor: Editor): Editor {
  editors.push(editor);
  return editor;
}

afterEach(() => {
  while (editors.length) editors.pop()?.destroy();
  vi.useRealTimers();
});

describe('parseExternalMarkdownToDoc', () => {
  it('parseia markdown para doc ProseMirror com o pipeline do tiptap-markdown', () => {
    const editor = track(createRealEditor('# Título'));
    const doc = parseExternalMarkdownToDoc(editor as unknown as IncrementalEditorLike, '# Outro título');

    expect(doc).not.toBeNull();
    expect(editor.state.doc.eq(doc as never)).toBe(false);

    const same = parseExternalMarkdownToDoc(editor as unknown as IncrementalEditorLike, '# Título');
    expect(editor.state.doc.eq(same as never)).toBe(true);
  });

  it('retorna null quando o editor não expõe parser/schema (fakes de teste)', () => {
    expect(parseExternalMarkdownToDoc({}, 'x')).toBeNull();
    expect(
      parseExternalMarkdownToDoc({ storage: { markdown: { getMarkdown: () => '' } } }, 'x')
    ).toBeNull();
  });
});

describe('applyExternalMarkdownIncrementally (editor TipTap real)', () => {
  it('mudança pequena no meio do doc preserva seleção fora do range alterado', () => {
    const initial = 'Primeiro parágrafo\n\nSegundo parágrafo\n\nTerceiro parágrafo';
    const editor = track(createRealEditor(initial));

    // Cursor dentro de "Primeiro" (bem antes do range que vai mudar).
    editor.commands.setTextSelection({ from: 3, to: 8 });
    const selectionBefore = { from: editor.state.selection.from, to: editor.state.selection.to };

    const docChanges = collectDocChanges(editor);
    const ok = applyExternalMarkdownIncrementally(
      editor as unknown as IncrementalEditorLike,
      'Primeiro parágrafo\n\nSegundo parágrafo\n\nTerceiro parágrafo ALTERADO'
    );

    expect(ok).toBe(true);
    expect(docChanges).toHaveLength(1);
    expect(serialize(editor)).toContain('Terceiro parágrafo ALTERADO');

    // A seleção não foi destruída (setContent total a jogaria para o fim).
    expect(editor.state.selection.from).toBe(selectionBefore.from);
    expect(editor.state.selection.to).toBe(selectionBefore.to);

    // A transação substituiu apenas o range alterado, não o doc inteiro.
    const [tr] = docChanges;
    expect(tr.steps).toHaveLength(1);
    const stepJson = tr.steps[0].toJSON() as { from: number; to: number };
    expect(stepJson.from).toBeGreaterThan(selectionBefore.to);
  });

  it('mudança antes da seleção remapeia o cursor pelo mapping da transação', () => {
    const editor = track(createRealEditor('Primeiro\n\nSegundo'));

    // Cursor dentro de "Segundo".
    const docSize = editor.state.doc.content.size;
    editor.commands.setTextSelection(docSize - 2);
    const posBefore = editor.state.selection.from;

    const ok = applyExternalMarkdownIncrementally(
      editor as unknown as IncrementalEditorLike,
      'Primeiro MAIOR\n\nSegundo'
    );

    expect(ok).toBe(true);
    // " MAIOR" tem 6 caracteres: o cursor desloca junto com o conteúdo.
    expect(editor.state.selection.from).toBe(posBefore + 6);
  });

  it('doc idêntico não gera nenhuma transação', () => {
    const editor = track(createRealEditor('- item'));
    const docChanges = collectDocChanges(editor);

    // "* item" parseia para o MESMO doc de "- item" (só o marcador difere).
    const ok = applyExternalMarkdownIncrementally(editor as unknown as IncrementalEditorLike, '* item');

    expect(ok).toBe(true);
    expect(docChanges).toHaveLength(0);
  });

  it('undo não contém a aplicação externa (addToHistory: false)', () => {
    const editor = track(createRealEditor('Olá mundo'));

    const ok = applyExternalMarkdownIncrementally(
      editor as unknown as IncrementalEditorLike,
      'Olá mundo externo'
    );
    expect(ok).toBe(true);
    expect(serialize(editor)).toBe('Olá mundo externo');

    editor.commands.undo();
    expect(serialize(editor)).toBe('Olá mundo externo');
  });

  it('retorna false quando o parse falha ou o editor não expõe view/state', () => {
    expect(applyExternalMarkdownIncrementally({}, 'x')).toBe(false);

    const editor = track(createRealEditor('a'));
    const broken = {
      state: editor.state,
      view: editor.view,
      schema: editor.schema,
      storage: {
        markdown: {
          parser: {
            parse: () => {
              throw new Error('parse quebrado');
            },
          },
        },
      },
    } as unknown as IncrementalEditorLike;

    expect(applyExternalMarkdownIncrementally(broken, 'x')).toBe(false);
  });
});

describe('syncFromExternal com editor TipTap real (integração)', () => {
  it('aplica incrementalmente, preserva seleção e atualiza baselines', () => {
    const initial = 'Início\n\nMeio\n\nFim';
    const editor = track(createRealEditor(initial));
    const refs = mountRefs(editor, initial);

    editor.commands.setTextSelection(3);
    const posBefore = editor.state.selection.from;

    const next = 'Início\n\nMeio\n\nFim ALTERADO';
    syncFromExternal({ refs, editor: editor as unknown as EditorLike, nextMarkdown: next });

    expect(editor.state.selection.from).toBe(posBefore);
    // Baselines conforme o contrato do #381: round-trip serializado + forma bruta.
    expect(refs.lastMarkdownRef.current).toBe(serialize(editor));
    expect(refs.lastExternalMarkdownRef.current).toBe(next);

    disposeRichMarkdownSync(refs);
  });

  it('não emite onMarkdownChange durante a aplicação externa', () => {
    vi.useFakeTimers();

    const initial = 'Um\n\nDois';
    const editor = track(createRealEditor(initial));
    const refs = mountRefs(editor, initial);
    const onMarkdownChange = vi.fn();

    // Espelha o RichTextEditor real: onUpdate ligado ao evento update do editor.
    editor.on('update', () => {
      onUpdate({
        refs,
        ctx: { editor: editor as unknown as EditorLike },
        onMarkdownChange,
        debounceMs: 0,
      });
    });

    syncFromExternal({ refs, editor: editor as unknown as EditorLike, nextMarkdown: 'Um\n\nDois novo' });

    // Esgota o debounce do onUpdate e o release do guard de forma determinística.
    vi.runAllTimers();
    expect(onMarkdownChange).not.toHaveBeenCalled();
    expect(serialize(editor)).toContain('Dois novo');

    disposeRichMarkdownSync(refs);
  });

  it('doc idêntico (só normalização) não dispara transação nem emissão', () => {
    const editor = track(createRealEditor('- item'));
    const refs = mountRefs(editor, '- item');
    const docChanges = collectDocChanges(editor);

    syncFromExternal({ refs, editor: editor as unknown as EditorLike, nextMarkdown: '* item' });

    expect(docChanges).toHaveLength(0);
    expect(refs.lastExternalMarkdownRef.current).toBe('* item');
    expect(refs.lastMarkdownRef.current).toBe(serialize(editor));

    disposeRichMarkdownSync(refs);
  });

  it('fallback para setContent quando o parse falha, com baselines corretos', () => {
    vi.useFakeTimers();

    // Editor fake sem parser/state/view: força o caminho de fallback.
    const setContent = vi.fn();
    let current = 'inicial';
    const editor: EditorLike = {
      commands: {
        setContent: (md: string) => {
          setContent(md);
          current = md;
        },
      },
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    const refs = createRichMarkdownSyncRefs('inicial');
    refs.hasEditorBaselineRef.current = true;

    syncFromExternal({ refs, editor, nextMarkdown: 'externo' });

    expect(setContent).toHaveBeenCalledTimes(1);
    expect(setContent).toHaveBeenCalledWith('externo');
    expect(refs.lastMarkdownRef.current).toBe('externo');
    expect(refs.lastExternalMarkdownRef.current).toBe('externo');

    vi.runAllTimers();
    expect(refs.isApplyingExternalMarkdownRef.current).toBe(false);

    disposeRichMarkdownSync(refs);
  });

  it('undo após sync externo não restaura o conteúdo anterior', () => {
    const initial = 'Slide 1';
    const editor = track(createRealEditor(initial));
    const refs = mountRefs(editor, initial);

    syncFromExternal({ refs, editor: editor as unknown as EditorLike, nextMarkdown: 'Slide 2' });
    expect(serialize(editor)).toBe('Slide 2');

    editor.commands.undo();
    expect(serialize(editor)).toBe('Slide 2');

    // Round-trip continua sendo o baseline (nada a emitir num flush).
    expect(refs.lastMarkdownRef.current).toBe('Slide 2');
    expect(getMarkdownNow(editor as unknown as EditorLike)).toBe('Slide 2');

    disposeRichMarkdownSync(refs);
  });
});
