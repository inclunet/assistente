import { afterEach, describe, expect, it, vi } from 'vitest';
import i18n from 'i18next';
import { captureEditorMarkdown } from './commandEditorMarkdown';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';

type Listener = () => void;

function createMonacoFixture(value = 'alpha\nbeta') {
  let text = value;
  let version = 1;
  let selection = { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: value.split('\n')[0].length + 1 };
  const contentListeners = new Set<Listener>();
  const selectionListeners = new Set<Listener>();
  const modelListeners = new Set<Listener>();
  const compositionListeners = new Set<Listener>();
  const dom = document.createElement('div');
  dom.className = 'monaco-editor';
  document.body.append(dom);
  const offset = (position: { lineNumber: number; column: number }) => text.split('\n').slice(0, position.lineNumber - 1).reduce((sum, line) => sum + line.length + 1, 0) + position.column - 1;
  const model = {
    getVersionId: () => version,
    getValue: () => text,
    getOffsetAt: offset,
    getPositionAt: (index: number) => { const lines = text.slice(0, index).split('\n'); return { lineNumber: lines.length, column: lines[lines.length - 1].length + 1 }; },
    isDisposed: () => false,
    getLineContent: (line: number) => text.split('\n')[line - 1] ?? '',
    getLineMaxColumn: (line: number) => (text.split('\n')[line - 1] ?? '').length + 1,
    getValueInRange: (range: typeof selection) => {
      return text.slice(offset({ lineNumber: range.startLineNumber, column: range.startColumn }), offset({ lineNumber: range.endLineNumber, column: range.endColumn }));
    },
    isReadonly: () => false,
    onDidChangeContent: (listener: Listener) => { contentListeners.add(listener); return { dispose: () => contentListeners.delete(listener) }; },
  };
  const editor = {
    getModel: () => model,
    getSelection: () => ({ ...selection }),
    getDomNode: () => dom,
    getOption: vi.fn(() => false),
    onDidDispose: (listener: Listener) => { modelListeners.add(listener); return { dispose: () => modelListeners.delete(listener) }; },
    onDidChangeConfiguration: () => ({ dispose() {} }),
    focus: vi.fn(), setPosition: vi.fn(), revealPositionInCenter: vi.fn(),
    onDidChangeCursorSelection: (listener: Listener) => { selectionListeners.add(listener); return { dispose: () => selectionListeners.delete(listener) }; },
    onDidChangeModel: (listener: Listener) => { modelListeners.add(listener); return { dispose: () => modelListeners.delete(listener) }; },
    onDidCompositionStart: (listener: Listener) => { compositionListeners.add(listener); return { dispose: () => compositionListeners.delete(listener) }; },
    pushUndoStop: vi.fn(() => true),
    executeEdits: vi.fn((_source: string, edits: Array<{ text: string; range?: typeof selection }>) => {
      const range = edits[0].range;
      text = range ? text.slice(0, offset({ lineNumber: range.startLineNumber, column: range.startColumn })) + edits[0].text +
        text.slice(offset({ lineNumber: range.endLineNumber, column: range.endColumn })) : edits[0].text;
      version += 1;
      contentListeners.forEach(listener => listener());
      return true;
    }),
    changeSelection(next: Partial<typeof selection>) { selection = { ...selection, ...next }; selectionListeners.forEach(listener => listener()); },
    changeModel() { modelListeners.forEach(listener => listener()); },
    startComposition() { compositionListeners.forEach(listener => listener()); },
    getText: () => text,
  };
  const monaco = { editor: { EditorOption: { readOnly: 'readOnly' } } };
  const target = captureEditorMarkdown(editor as never, monaco as never, dom, () => true);
  return { editor, model, monaco, dom, target };
}

afterEach(() => {
  useQuestionnaireUIStore.getState().cancel();
  document.body.replaceChildren();
});

describe('commandEditorMarkdown', () => {
  it.each([
    ['editor.format.code_block.insert', '```\nalpha\n```'],
    ['editor.format.mermaid.insert', `\`\`\`mermaid\nflowchart TD\n  A[${i18n.t('editor.presentation.insert.diagramStart')}] --> B[${i18n.t('editor.presentation.insert.diagramEnd')}]\n\`\`\``],
    ['editor.format.list.bullet', '- alpha'],
    ['editor.format.list.ordered', '1. alpha'],
    ['editor.format.blockquote', '> alpha'],
  ] as const)('insere %s na seleção capturada', async (commandID, expected) => {
    const fixture = createMonacoFixture();
    expect(fixture.target?.canExecute(commandID)).toBe(true);
    expect(await fixture.target?.prepare(commandID)).toBe(true);
    expect(fixture.target?.execute(commandID)).toBe(true);
    expect(fixture.editor.getText()).toBe(expected + '\n\nbeta');
    expect(fixture.editor.pushUndoStop).toHaveBeenCalledTimes(2);
  });

  it('gera tabela Markdown sempre com linha de cabeçalho', async () => {
    const fixture = createMonacoFixture();
    expect(await fixture.target?.prepare('editor.format.table.insert', { kind: 'table', rows: 2, cols: 2, withHeaderRow: true })).toBe(true);
    expect(fixture.target?.execute('editor.format.table.insert')).toBe(true);
    expect(fixture.editor.getText()).toBe('| C1 | C2 |\n| --- | --- |\n|  |  |\n\nbeta');
  });

  it('preserva seleção e evita fence aninhada', async () => {
    const fixture = createMonacoFixture('alpha ``` nested');
    expect(await fixture.target?.prepare('editor.format.code_block.insert')).toBe(true);
    expect(fixture.target?.execute('editor.format.code_block.insert')).toBe(true);
    expect(fixture.editor.getText()).toBe('````\nalpha ``` nested\n````');
  });

  it('recusa tabela sem cabeçalho e não altera o modelo', async () => {
    const fixture = createMonacoFixture();
    expect(await fixture.target?.prepare('editor.format.table.insert', { kind: 'table', rows: 2, cols: 2, withHeaderRow: false })).toBe(false);
    expect(fixture.target?.execute('editor.format.table.insert')).toBe(false);
    expect(fixture.editor.getText()).toBe('alpha\nbeta');
  });

  it.each(['selection', 'content', 'model', 'composition'] as const)('invalida imediatamente após %s', (kind) => {
    const fixture = createMonacoFixture();
    if (kind === 'selection') fixture.editor.changeSelection({ startColumn: 2 });
    if (kind === 'content') fixture.editor.executeEdits('external', [{ text: 'changed' }]);
    if (kind === 'model') fixture.editor.changeModel();
    if (kind === 'composition') fixture.editor.startComposition();
    expect(fixture.target?.isCurrent()).toBe(false);
    expect(fixture.target?.execute('editor.format.blockquote')).toBe(false);
  });

  it('descarta e não retargeteia editor destruído, DOM desconectado ou readonly', () => {
    const fixture = createMonacoFixture();
    fixture.dom.remove();
    expect(fixture.target?.isCurrent()).toBe(false);
    const readonly = createMonacoFixture();
    readonly.editor.getOption.mockReturnValue(true);
    expect(captureEditorMarkdown(readonly.editor as never, readonly.monaco as never, readonly.dom, () => true)).toBeUndefined();
  });
  it('preserva bytes fora da seleção no meio de uma linha', async () => {
    const f = createMonacoFixture('before SELECT after');
    f.target?.dispose();
    f.editor.changeSelection({ startColumn: 8, endColumn: 14 });
    const target = captureEditorMarkdown(f.editor as never, f.monaco as never, f.dom, () => true);
    expect(await target?.prepare('editor.format.code_block.insert')).toBe(true);
    expect(target?.execute('editor.format.code_block.insert')).toBe(true);
    expect(f.editor.getText()).toBe('before \n\n```\nSELECT\n```\n\n after');
    target?.dispose();
  });
  it('dimensiona fence maior que qualquer sequência selecionada', async () => {
    const f = createMonacoFixture('```` nested');
    expect(await f.target?.prepare('editor.format.code_block.insert')).toBe(true);
    expect(f.target?.execute('editor.format.code_block.insert')).toBe(true);
    expect(f.editor.getText()).toBe('`````\n```` nested\n`````');
  });
  it.each(['confirm', 'cancel'] as const)('questionnaire de tabela %s restaura foco e só confirmação pode escrever', async outcome => {
    const f = createMonacoFixture();
    const preparing = f.target!.prepare('editor.format.table.insert');
    expect(useQuestionnaireUIStore.getState().active?.questions.map(question => question.id)).toEqual(['rows', 'cols']);
    if (outcome === 'confirm') useQuestionnaireUIStore.getState().submit({ rows: '3', cols: '2' });
    else useQuestionnaireUIStore.getState().cancel();
    expect(await preparing).toBe(outcome === 'confirm');
    expect(f.editor.focus).toHaveBeenCalled();
    expect(f.editor.executeEdits).not.toHaveBeenCalled();
    expect(f.target!.execute('editor.format.table.insert')).toBe(outcome === 'confirm');
    f.target?.dispose();
  });
  it('mudança de seleção aborta formulário sem roubar foco', async () => {
    const f = createMonacoFixture();
    const preparing = f.target!.prepare('editor.format.table.insert');
    f.editor.changeSelection({ startColumn: 2 });
    expect(await preparing).toBe(false);
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
    expect(f.editor.focus).not.toHaveBeenCalled();
    f.target?.dispose();
  });
  it('copia payload do menu antes de devolvê-lo ao chamador', async () => {
    const f = createMonacoFixture();
    const input = { kind: 'table' as const, rows: 2, cols: 2, withHeaderRow: true };
    const pending = f.target!.prepare('editor.format.table.insert', input);
    input.cols = 6;
    expect(await pending).toBe(true);
    expect(f.target!.execute('editor.format.table.insert')).toBe(true);
    expect(f.editor.getText()).not.toContain('C3');
  });
});
