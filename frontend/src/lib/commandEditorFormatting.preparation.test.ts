import { afterEach, describe, expect, it, vi } from 'vitest';
import { Editor } from '@tiptap/core';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { captureEditorFormatting, registerEditorFormatting } from './commandEditorFormatting';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';

const cleanups: Array<() => void> = [];
function setup(content = 'alpha beta') {
  const root = document.createElement('div');
  document.body.append(root);
  const editor = new Editor({ element: root, content, extensions: buildRichTextExtensions({
    placeholder: '', imageFallbackLabel: 'image', imageLabelPrefix: 'image',
  }) });
  let invalidate = () => {};
  const unregister = registerEditorFormatting({ root, editor, isCurrent: () => true,
    subscribe: callback => { invalidate = callback; return () => {}; },
  });
  cleanups.push(() => { unregister(); editor.destroy(); root.remove(); });
  return { editor, unregister, invalidate: () => invalidate() };
}
afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  const state = useQuestionnaireUIStore.getState();
  if (state.active) state.cancel();
  vi.useRealTimers();
});

describe('preparação efêmera de conteúdo rico', () => {
  it('não redireciona um menu antigo para outra instância do editor', () => {
    const first = setup();
    first.unregister();
    const second = setup();
    expect(captureEditorFormatting(first.editor)).toBeUndefined();
    const target = captureEditorFormatting(second.editor);
    expect(target?.isCurrent()).toBe(true);
    target?.dispose();
  });

  it('só aplica o link após preparar e executar, na seleção original', async () => {
    const { editor } = setup();
    editor.commands.setTextSelection({ from: 1, to: 6 });
    const target = captureEditorFormatting()!;
    const before = editor.getHTML();
    expect(target.execute('editor.format.link.set')).toBe(false);
    expect(await target.prepare('editor.format.link.set', { kind: 'link', href: ' https://example.com ' })).toBe(true);
    expect(editor.getHTML()).toBe(before);
    expect(target.execute('editor.format.link.set')).toBe(true);
    expect(editor.getHTML()).toContain('href="https://example.com"');
    expect(editor.state.doc.textContent).toBe('alpha beta');
    expect(editor.state.doc.rangeHasMark(1, 6, editor.schema.marks.link)).toBe(true);
    expect(editor.state.doc.rangeHasMark(6, 11, editor.schema.marks.link)).toBe(false);
    expect(target.execute('editor.format.link.set')).toBe(false);
  });

  it('edita o vínculo inteiro sob o cursor sem duplicar texto', async () => {
    const { editor } = setup('[alpha](https://old.example) beta');
    editor.commands.setTextSelection(3);
    const target = captureEditorFormatting()!;
    expect(await target.prepare('editor.format.link.set', { kind: 'link', href: 'https://new.example' })).toBe(true);
    expect(target.execute('editor.format.link.set')).toBe(true);
    expect(editor.state.doc.textContent).toBe('alpha beta');
    expect(editor.getHTML()).toContain('href="https://new.example"');
    expect(editor.getHTML()).not.toContain('old.example');
  });

  it('usa endereço quando texto é vazio, sem criar nó de texto vazio', async () => {
    const { editor } = setup('');
    const target = captureEditorFormatting()!;
    expect(await target.prepare('editor.format.link.set', { kind: 'link', href: 'https://example.com', text: '' })).toBe(true);
    expect(target.execute('editor.format.link.set')).toBe(true);
    expect(editor.state.doc.textContent).toBe('https://example.com');
  });

  it('copia dimensões fornecidas e cria uma única tabela no commit', async () => {
    const { editor } = setup();
    const target = captureEditorFormatting()!;
    const input = { kind: 'table', rows: 2, cols: 3, withHeaderRow: true };
    const preparing = target.prepare('editor.format.table.insert', input);
    input.rows = 100;
    expect(await preparing).toBe(true);
    expect(editor.getHTML()).not.toContain('<table');
    expect(target.execute('editor.format.table.insert')).toBe(true);
    const tables = editor.view.dom.querySelectorAll('table');
    expect(tables).toHaveLength(1);
    expect(tables[0].rows).toHaveLength(2);
    expect(tables[0].rows[0].cells).toHaveLength(3);
    expect(tables[0].rows[0].cells[0].tagName).toBe('TH');
  });

  it.each([
    ['editor.format.link.set', { kind: 'link', href: 'javascript:alert(1)' }],
    ['editor.format.table.insert', { kind: 'table', rows: 100, cols: 2, withHeaderRow: true }],
    ['editor.format.table.insert', { kind: 'table', rows: 2, cols: 2, withHeaderRow: true, extra: 1 }],
  ])('recusa payload inválido de %s sem mutação', async (id, input) => {
    const { editor } = setup();
    const before = editor.getHTML();
    const target = captureEditorFormatting()!;
    expect(await target.prepare(id, input)).toBe(false);
    expect(target.execute(id)).toBe(false);
    expect(editor.getHTML()).toBe(before);
  });

  it.each(['cancel', 'selection', 'context', 'unmount'] as const)('cancela formulário por %s', async reason => {
    const { editor, invalidate, unregister } = setup();
    const before = editor.getHTML();
    const target = captureEditorFormatting()!;
    const pending = target.prepare('editor.format.table.insert');
    expect(useQuestionnaireUIStore.getState().active).not.toBeNull();
    if (reason === 'cancel') useQuestionnaireUIStore.getState().cancel();
    if (reason === 'selection') editor.commands.setTextSelection(3);
    if (reason === 'context') invalidate();
    if (reason === 'unmount') unregister();
    expect(await pending).toBe(false);
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
    expect(target.execute('editor.format.table.insert')).toBe(false);
    expect(editor.getHTML()).toBe(before);
  });

  it('limita o tempo total do formulário e remove a solicitação', async () => {
    vi.useFakeTimers();
    setup();
    const target = captureEditorFormatting()!;
    const pending = target.prepare('editor.format.table.insert');
    expect(useQuestionnaireUIStore.getState().active).not.toBeNull();
    await vi.advanceTimersByTimeAsync(300_000);
    expect(await pending).toBe(false);
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
  });

  it('confirma formulário sem editar durante a preparação', async () => {
    const { editor } = setup();
    const before = editor.getHTML();
    const target = captureEditorFormatting()!;
    const pending = target.prepare('editor.format.table.insert');
    useQuestionnaireUIStore.getState().submit({ rows: '3', cols: '2', withHeaderRow: false });
    expect(await pending).toBe(true);
    expect(editor.getHTML()).toBe(before);
    expect(target.execute('editor.format.table.insert')).toBe(true);
    expect(editor.getJSON().content!.find(n => n.type === 'table')!.content).toHaveLength(3);
  });

  it.each(['editor.format.code_block.insert', 'editor.format.mermaid.insert'])('insere %s sem efeitos na consulta', async id => {
    const { editor } = setup('');
    const target = captureEditorFormatting()!;
    const before = editor.getHTML();
    expect(target.canExecute(id)).toBe(true);
    expect(await target.prepare(id)).toBe(true);
    expect(editor.getHTML()).toBe(before);
    expect(target.execute(id)).toBe(true);
    expect(editor.getJSON().content![0].type).toBe('codeBlock');
    if (id.endsWith('mermaid.insert')) {
      expect(editor.getJSON().content![0].attrs!.language).toBe('mermaid');
      expect(editor.state.doc.textContent).toContain('flowchart TD');
    } else expect(editor.getJSON().content![0].attrs!.language).toBe('');
  });
});
