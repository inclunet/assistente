import { afterEach, describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import {
  captureEditorFormatting,
  EDITOR_FORMAT_COMMANDS,
  registerEditorFormatting,
  registerEditorFormatAdapter,
} from './commandEditorFormatting';
import { vi } from 'vitest';

const cleanups: Array<() => void> = [];

describe('adaptadores capturados de inserção', () => {
  it('recusa ambiguidade e não escolhe outra superfície depois da preparação', async () => {
    const make = () => ({ isCurrent: () => true, canExecute: () => true,
      prepare: vi.fn(async () => true), execute: vi.fn(() => true), dispose: vi.fn() });
    const a = make();
    const b = make();
    const offA = registerEditorFormatAdapter({ capture: () => a });
    const offB = registerEditorFormatAdapter({ capture: () => b });
    cleanups.push(offA, offB);
    const target = captureEditorFormatting();
    expect(target?.canExecute('editor.slide.insert.basic')).toBe(false);
    expect(await target?.prepare('editor.slide.insert.basic')).toBe(false);
    expect(target?.execute('editor.slide.insert.basic')).toBe(false);
    expect(a.execute).not.toHaveBeenCalled();
    expect(b.execute).not.toHaveBeenCalled();
    target?.dispose();
    expect(a.dispose).toHaveBeenCalledTimes(1);
    expect(b.dispose).toHaveBeenCalledTimes(1);
  });
  it('mantém adaptador escolhido fixo mesmo se outro ficar elegível depois', async () => {
    let firstCurrent = true;
    const a = { isCurrent: () => firstCurrent, canExecute: () => firstCurrent,
      prepare: vi.fn(async () => true), execute: vi.fn(() => firstCurrent), dispose: vi.fn() };
    const b = { isCurrent: () => !firstCurrent, canExecute: () => !firstCurrent,
      prepare: vi.fn(async () => true), execute: vi.fn(() => true), dispose: vi.fn() };
    cleanups.push(registerEditorFormatAdapter({ capture: () => a }), registerEditorFormatAdapter({ capture: () => b }));
    const target = captureEditorFormatting();
    expect(await target?.prepare('editor.slide.insert.basic')).toBe(true);
    firstCurrent = false;
    expect(target?.isCurrent()).toBe(false);
    expect(target?.canExecute('editor.slide.insert.basic')).toBe(false);
    expect(target?.execute('editor.slide.insert.basic')).toBe(false);
    expect(b.execute).not.toHaveBeenCalled();
    target?.dispose();
  });
});

function createEditor(content = 'alpha beta gamma') {
  const root = document.createElement('div');
  document.body.append(root);

  const editor = new Editor({
    element: root,
    extensions: buildRichTextExtensions({
      placeholder: 'x',
      imageFallbackLabel: 'Imagem sem descrição',
      imageLabelPrefix: 'Imagem',
    }),
    content,
  });

  const unregister = registerEditorFormatting({
    root,
    editor,
    isCurrent: () => true,
    subscribe: () => () => undefined,
  });

  cleanups.push(() => {
    unregister();
    editor.destroy();
    root.remove();
  });

  return { editor, root };
}

function selectBeta(editor: Editor) {
  editor.commands.setTextSelection({ from: 7, to: 11 });
}

function expectOnlySelectedRangeFormatted(editor: Editor, markName: 'bold' | 'italic' | 'strike') {
  const mark = editor.schema.marks[markName];
  expect(mark).toBeDefined();
  expect(editor.state.doc.rangeHasMark(7, 11, mark)).toBe(true);
  expect(editor.state.doc.rangeHasMark(1, 7, mark)).toBe(false);
  expect(editor.state.doc.rangeHasMark(11, 18, mark)).toBe(false);
  expect(editor.state.doc.textBetween(7, 11, ' ')).toBe('beta');
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
});

describe('commandEditorFormatting com Tiptap real', () => {
  it.each([
    ['editor.format.bold', 'bold'],
    ['editor.format.italic', 'italic'],
    ['editor.format.strike', 'strike'],
  ] as const)('aplica %s uma vez somente na seleção capturada', (commandID, markName) => {
    const { editor } = createEditor();
    selectBeta(editor);

    const target = captureEditorFormatting();
    expect(target?.canExecute(commandID)).toBe(true);
    expect(target?.execute(commandID)).toBe(true);
    expect(target?.execute(commandID)).toBe(false);
    expectOnlySelectedRangeFormatted(editor, markName);
    expect(editor.state.selection.from).toBe(7);
    expect(editor.state.selection.to).toBe(11);
  });

  it('recusa documento alterado antes do commit, sem retarget', () => {
    const { editor } = createEditor();
    selectBeta(editor);
    const target = captureEditorFormatting();

    editor.commands.insertContentAt(1, 'X');

    expect(target?.isCurrent()).toBe(false);
    expect(target?.execute('editor.format.bold')).toBe(false);
    expect(editor.getHTML()).not.toContain('<strong>beta</strong>');
  });

  it('recusa seleção alterada, inclusive quando volta ao mesmo range', () => {
    const { editor } = createEditor();
    selectBeta(editor);
    const target = captureEditorFormatting();

    editor.commands.setTextSelection({ from: 1, to: 6 });
    editor.commands.setTextSelection({ from: 7, to: 11 });

    expect(target?.isCurrent()).toBe(false);
    expect(target?.execute('editor.format.italic')).toBe(false);
    expect(editor.getHTML()).not.toContain('<em>beta</em>');
  });

  it('recusa captura readonly, superfície desmontada e instância substituída', () => {
    const readonly = createEditor();
    readonly.editor.setEditable(false);
    selectBeta(readonly.editor);
    expect(captureEditorFormatting()).toBeUndefined();

    const detached = createEditor();
    selectBeta(detached.editor);
    const detachedTarget = captureEditorFormatting();
    detached.root.remove();
    expect(detachedTarget?.isCurrent()).toBe(false);
    expect(detachedTarget?.execute('editor.format.strike')).toBe(false);

    const replaced = createEditor();
    selectBeta(replaced.editor);
    const replacedTarget = captureEditorFormatting();
    replaced.editor.destroy();
    const replacement = new Editor({
      element: replaced.root,
      extensions: buildRichTextExtensions({
        placeholder: 'x',
        imageFallbackLabel: 'Imagem sem descrição',
        imageLabelPrefix: 'Imagem',
      }),
      content: 'alpha beta gamma',
    });
    cleanups.push(() => replacement.destroy());

    expect(replacedTarget?.isCurrent()).toBe(false);
    expect(replacedTarget?.execute('editor.format.bold')).toBe(false);
    expect(replacement.getHTML()).not.toContain('<strong>beta</strong>');
  });

  it('dispose invalida o alvo e remove a superfície capturada', () => {
    const { editor } = createEditor();
    selectBeta(editor);
    const target = captureEditorFormatting();

    target?.dispose();

    expect(target?.isCurrent()).toBe(false);
    expect(target?.canExecute('editor.format.bold')).toBe(false);
    expect(target?.execute('editor.format.bold')).toBe(false);
    expect(editor.getHTML()).not.toContain('<strong>beta</strong>');
  });

  it('unregister libera a lease imediatamente e é idempotente', () => {
    const root = document.createElement('div');
    document.body.append(root);
    const editor = new Editor({
      element: root,
      extensions: buildRichTextExtensions({
        placeholder: 'x',
        imageFallbackLabel: 'Imagem sem descrição',
        imageLabelPrefix: 'Imagem',
      }),
      content: 'alpha beta gamma',
    });
    let unsubscribeCalls = 0;
    const unregister = registerEditorFormatting({
      root,
      editor,
      isCurrent: () => true,
      subscribe: () => () => { unsubscribeCalls += 1; },
    });
    const target = captureEditorFormatting();

    expect(target?.isCurrent()).toBe(true);
    unregister();
    expect(target?.isCurrent()).toBe(false);
    expect(target?.execute('editor.format.bold')).toBe(false);
    expect(unsubscribeCalls).toBe(1);

    unregister();
    target?.dispose();
    target?.dispose();
    expect(unsubscribeCalls).toBe(1);

    editor.destroy();
    root.remove();
  });

  it('keymaps nativos Ctrl+B/I/Shift+X não formatam, mas os commands continuam funcionando', () => {
    const { editor } = createEditor();
    const nativeShortcuts = [
      ['b', false, 'bold'],
      ['i', false, 'italic'],
      ['x', true, 'strike'],
    ] as const;

    for (const [key, shiftKey, markName] of nativeShortcuts) {
      selectBeta(editor);
      const before = editor.getHTML();
      editor.view.dom.dispatchEvent(new KeyboardEvent('keydown', {
        bubbles: true,
        cancelable: true,
        ctrlKey: true,
        key,
        code: `Key${key.toUpperCase()}`,
        shiftKey,
      }));

      expect(editor.getHTML()).toBe(before);
      expect(editor.state.doc.rangeHasMark(7, 11, editor.schema.marks[markName])).toBe(false);
    }

    selectBeta(editor);
    expect(editor.commands.toggleBold()).toBe(true);
    expectOnlySelectedRangeFormatted(editor, 'bold');
    editor.commands.unsetBold();

    selectBeta(editor);
    expect(editor.commands.toggleItalic()).toBe(true);
    expectOnlySelectedRangeFormatted(editor, 'italic');
    editor.commands.unsetItalic();

    selectBeta(editor);
    expect(editor.commands.toggleStrike()).toBe(true);
    expectOnlySelectedRangeFormatted(editor, 'strike');
  });

  it('preserva os três comandos de marcação na lista auditada ampliada', () => {
    expect(EDITOR_FORMAT_COMMANDS).toMatchObject({
      'editor.format.bold': 'toggleBold',
      'editor.format.italic': 'toggleItalic',
      'editor.format.strike': 'toggleStrike',
    });
    expect(Object.keys(EDITOR_FORMAT_COMMANDS)).toHaveLength(32);
  });
});
