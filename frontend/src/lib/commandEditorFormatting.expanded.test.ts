import { afterEach, describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import { CellSelection } from '@tiptap/pm/tables';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import {
  captureEditorFormatting,
  EDITOR_FORMAT_COMMANDS,
  registerEditorFormatting,
} from './commandEditorFormatting';

type JsonNode = {
  type: string;
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: Array<{ type: string; attrs?: Record<string, unknown> }>;
  content?: JsonNode[];
};

const cleanups: Array<() => void> = [];

const extensions = () => buildRichTextExtensions({
  placeholder: 'x',
  imageFallbackLabel: 'Imagem sem descrição',
  imageLabelPrefix: 'Imagem',
});

function createEditor(content: JsonNode | string = 'alpha beta gamma') {
  const root = document.createElement('div');
  document.body.append(root);
  const editor = new Editor({ element: root, extensions: extensions(), content });
  const unregister = registerEditorFormatting({
    root,
    editor,
    isCurrent: () => true,
    subscribe: () => () => undefined,
  });
  cleanups.push(() => {
    unregister();
    if (!editor.isDestroyed) editor.destroy();
    root.remove();
  });
  return { editor, root, close: cleanups[cleanups.length - 1] };
}

function textDoc(text: string, marks?: JsonNode['marks']): JsonNode {
  return {
    type: 'doc',
    content: [{ type: 'paragraph', content: [{ type: 'text', text, ...(marks ? { marks } : {}) }] }],
  };
}

function tableDoc(options: { merged?: boolean; headers?: boolean } = {}): JsonNode {
  const cell = (text: string, type = options.headers ? 'tableHeader' : 'tableCell', attrs?: Record<string, unknown>): JsonNode => ({
    type,
    ...(attrs ? { attrs } : {}),
    content: [{ type: 'paragraph', content: [{ type: 'text', text }] }],
  });
  const row = (prefix: string): JsonNode => ({
    type: 'tableRow',
    content: options.merged
      ? [cell(`${prefix} merged`, 'tableCell', { colspan: 2, rowspan: 1 })]
      : [cell(`${prefix} one`), cell(`${prefix} two`)],
  });
  return {
    type: 'doc',
    content: [{ type: 'table', content: [row('a'), row('b')] }],
  };
}

function cellPositions(editor: Editor): number[] {
  const positions: number[] = [];
  editor.state.doc.descendants((node, position) => {
    if (node.type.name === 'tableCell' || node.type.name === 'tableHeader') positions.push(position);
  });
  return positions;
}

function selectCell(editor: Editor, index = 0) {
  const position = cellPositions(editor)[index];
  expect(position).toBeDefined();
  editor.commands.setTextSelection(position + 2);
}

function selectCells(editor: Editor, fromIndex: number, toIndex: number) {
  const positions = cellPositions(editor);
  const selection = new CellSelection(
    editor.state.doc.resolve(positions[fromIndex]),
    editor.state.doc.resolve(positions[toIndex]),
  );
  editor.view.dispatch(editor.state.tr.setSelection(selection));
}

function tableShape(editor: Editor) {
  const table = editor.state.doc.firstChild;
  const rows = table?.content.content ?? [];
  return {
    tables: editor.state.doc.content.content.filter(node => node.type.name === 'table').length,
    rows: rows.length,
    columns: rows[0]?.content.content.length ?? 0,
    firstRowHeaders: rows[0]?.content.content.every(cell => cell.type.name === 'tableHeader') ?? false,
    firstColumnHeaders: rows.length > 0 && rows.every(row => row.content.firstChild?.type.name === 'tableHeader'),
    firstCellHeader: rows[0]?.firstChild?.type.name === 'tableHeader',
  };
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
});

describe('commandEditorFormatting expandido com Tiptap e JSON reais', () => {
  it('mantém exatamente os 32 IDs auditados, incluindo heading.h1..h6 e inserções', () => {
    expect(Object.keys(EDITOR_FORMAT_COMMANDS)).toEqual([
      'editor.format.bold', 'editor.format.italic', 'editor.format.strike',
      'editor.format.paragraph',
      'editor.format.heading.h1', 'editor.format.heading.h2', 'editor.format.heading.h3',
      'editor.format.heading.h4', 'editor.format.heading.h5', 'editor.format.heading.h6',
      'editor.format.blockquote', 'editor.format.code_block',
      'editor.format.code_block.insert', 'editor.format.mermaid.insert',
      'editor.format.list.bullet', 'editor.format.list.ordered', 'editor.format.clear_marks',
      'editor.format.link.remove',
      'editor.format.table.row.before', 'editor.format.table.row.after', 'editor.format.table.row.delete',
      'editor.format.table.column.before', 'editor.format.table.column.after', 'editor.format.table.column.delete',
      'editor.format.table.header.row', 'editor.format.table.header.column', 'editor.format.table.header.cell',
      'editor.format.table.merge', 'editor.format.table.split', 'editor.format.table.delete',
      'editor.format.link.set', 'editor.format.table.insert',
    ]);
  });

  it.each([
    ['editor.format.bold', 'bold'],
    ['editor.format.italic', 'italic'],
    ['editor.format.strike', 'strike'],
  ] as const)('aplica %s uma vez na seleção original', (id, markName) => {
    const { editor } = createEditor();
    editor.commands.setTextSelection({ from: 7, to: 11 });
    const target = captureEditorFormatting();
    expect(target?.canExecute(id)).toBe(true);
    expect(target?.execute(id)).toBe(true);
    expect(target?.execute(id)).toBe(false);
    expect(editor.state.doc.rangeHasMark(7, 11, editor.schema.marks[markName])).toBe(true);
    expect(editor.state.doc.textBetween(7, 11)).toBe('beta');
    expect(editor.state.selection.from).toBe(7);
    expect(editor.state.selection.to).toBe(11);
  });

  it.each([
    ['editor.format.heading.h1', 1], ['editor.format.heading.h2', 2], ['editor.format.heading.h3', 3],
    ['editor.format.heading.h4', 4], ['editor.format.heading.h5', 5], ['editor.format.heading.h6', 6],
  ] as const)('usa setHeading para %s, sem toggle', (id, level) => {
    const { editor } = createEditor();
    editor.commands.setTextSelection({ from: 2, to: 6 });
    const target = captureEditorFormatting();
    expect(target?.canExecute(id)).toBe(true);
    expect(target?.execute(id)).toBe(true);
    expect(editor.state.doc.firstChild?.type.name).toBe('heading');
    expect(editor.state.doc.firstChild?.attrs.level).toBe(level);
  });

  it.each([
    ['editor.format.blockquote'], ['editor.format.list.bullet'],
    ['editor.format.list.ordered'], ['editor.format.code_block'],
  ] as const)('usa setParagraph e executa %s no alvo real', (id) => {
    const { editor } = createEditor({ type: 'doc', content: [{ type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'title' }] }] });
    editor.commands.setTextSelection({ from: 2, to: 7 });
    const paragraph = captureEditorFormatting();
    expect(paragraph?.canExecute('editor.format.paragraph')).toBe(true);
    expect(paragraph?.execute('editor.format.paragraph')).toBe(true);
    expect(editor.state.doc.firstChild?.type.name).toBe('paragraph');

    editor.commands.setTextSelection({ from: 2, to: 6 });
    const target = captureEditorFormatting();
    expect(target?.canExecute(id)).toBe(true);
    expect(target?.execute(id)).toBe(true);
  });

  it('limpa somente marcas, sem alterar texto', () => {
    const { editor } = createEditor(textDoc('marked text', [
      { type: 'bold' }, { type: 'italic' }, { type: 'strike' },
    ]));
    editor.commands.setTextSelection({ from: 1, to: 12 });
    const before = editor.state.doc.textContent;
    const target = captureEditorFormatting();
    expect(target?.canExecute('editor.format.clear_marks')).toBe(true);
    expect(target?.execute('editor.format.clear_marks')).toBe(true);
    expect(editor.state.doc.textContent).toBe(before);
    expect(editor.state.doc.rangeHasMark(1, 12, editor.schema.marks.bold)).toBe(false);
    expect(editor.state.doc.rangeHasMark(1, 12, editor.schema.marks.italic)).toBe(false);
    expect(editor.state.doc.rangeHasMark(1, 12, editor.schema.marks.strike)).toBe(false);
  });

  it('remove link somente quando o cursor está dentro do link e expande seu range', () => {
    const { editor } = createEditor({
      type: 'doc',
      content: [{ type: 'paragraph', content: [
        { type: 'text', text: 'before ' },
        { type: 'text', text: 'linked', marks: [{ type: 'link', attrs: { href: 'https://example.com' } }] },
        { type: 'text', text: ' after' },
      ] }],
    });
    editor.commands.setTextSelection(9);
    const inside = captureEditorFormatting();
    expect(inside?.canExecute('editor.format.link.remove')).toBe(true);
    expect(inside?.execute('editor.format.link.remove')).toBe(true);
    expect(editor.state.doc.textContent).toBe('before linked after');
    expect(editor.state.doc.rangeHasMark(8, 14, editor.schema.marks.link)).toBe(false);

    editor.commands.setTextSelection(1);
    const outside = captureEditorFormatting();
    expect(outside?.canExecute('editor.format.link.remove')).toBe(false);
    expect(outside?.execute('editor.format.link.remove')).toBe(false);
  });

  const tableCommands = [
    ['editor.format.table.row.before', 'row.before'], ['editor.format.table.row.after', 'row.after'], ['editor.format.table.row.delete', 'row.delete'],
    ['editor.format.table.column.before', 'column.before'], ['editor.format.table.column.after', 'column.after'], ['editor.format.table.column.delete', 'column.delete'],
    ['editor.format.table.header.row', 'header.row'], ['editor.format.table.header.column', 'header.column'], ['editor.format.table.header.cell', 'header.cell'],
    ['editor.format.table.delete', 'delete'],
  ] as const;

  it.each(tableCommands)('executa %s em tabela JSON no alvo original', (id, kind) => {
    const { editor } = createEditor(tableDoc());
    selectCell(editor);
    const target = captureEditorFormatting();
    const before = tableShape(editor);
    expect(target?.canExecute(id)).toBe(true);
    expect(target?.execute(id)).toBe(true);
    const after = tableShape(editor);
    if (kind === 'row.before' || kind === 'row.after') expect(after.rows).toBe(before.rows + 1);
    if (kind === 'row.delete') expect(after.rows).toBe(before.rows - 1);
    if (kind === 'column.before' || kind === 'column.after') expect(after.columns).toBe(before.columns + 1);
    if (kind === 'column.delete') expect(after.columns).toBe(before.columns - 1);
    if (kind === 'header.row') expect(after.firstRowHeaders).toBe(true);
    if (kind === 'header.column') expect(after.firstColumnHeaders).toBe(true);
    if (kind === 'header.cell') expect(after.firstCellHeader).toBe(true);
    if (kind === 'delete') expect(after.tables).toBe(0);
    expect(target?.execute(id)).toBe(false);
  });

  it.each([
    ['editor.format.table.row.before'], ['editor.format.table.row.after'], ['editor.format.table.row.delete'],
    ['editor.format.table.column.before'], ['editor.format.table.column.after'], ['editor.format.table.column.delete'],
    ['editor.format.table.header.row'], ['editor.format.table.header.column'], ['editor.format.table.header.cell'],
    ['editor.format.table.merge'], ['editor.format.table.split'], ['editor.format.table.delete'],
  ] as const)('recusa %s sem tabela', (id) => {
    const { editor } = createEditor();
    editor.commands.setTextSelection({ from: 2, to: 6 });
    const target = captureEditorFormatting();
    expect(target?.canExecute(id)).toBe(false);
    expect(target?.execute(id)).toBe(false);
  });

  it('preserva CellSelection real em merge e split', () => {
    const merged = createEditor(tableDoc());
    selectCells(merged.editor, 0, 1);
    const merge = captureEditorFormatting();
    expect(merge?.canExecute('editor.format.table.merge')).toBe(true);
    expect(merge?.execute('editor.format.table.merge')).toBe(true);
    expect(merged.editor.state.selection).toBeInstanceOf(CellSelection);
    merged.close();

    const split = createEditor(tableDoc({ merged: true }));
    selectCells(split.editor, 0, 0);
    const splitTarget = captureEditorFormatting();
    expect(splitTarget?.canExecute('editor.format.table.split')).toBe(true);
    expect(splitTarget?.execute('editor.format.table.split')).toBe(true);
    expect(split.editor.state.selection).toBeInstanceOf(CellSelection);
  });

  it('invalida documento e seleção obsoletos antes de qualquer ação de tabela', () => {
    const { editor, close } = createEditor(tableDoc());
    selectCell(editor);
    const target = captureEditorFormatting();
    editor.commands.insertContentAt(1, 'stale');
    expect(target?.canExecute('editor.format.table.row.after')).toBe(false);
    expect(target?.execute('editor.format.table.row.after')).toBe(false);

    close();
    const second = createEditor(tableDoc());
    selectCell(second.editor);
    const staleSelection = captureEditorFormatting();
    selectCell(second.editor, 1);
    expect(staleSelection?.canExecute('editor.format.table.column.after')).toBe(false);
    expect(staleSelection?.execute('editor.format.table.column.after')).toBe(false);
  });

  it('remove exatamente os 11 atalhos nativos auditados e preserva Backspace/Enter de code block', () => {
    const shortcuts = [
      ...['0', '1', '2', '3', '4', '5', '6'].map(key => [key, `Digit${key}`, true, false] as const),
      ['c', 'KeyC', true, false],
      ['b', 'KeyB', false, true],
      ['7', 'Digit7', false, true], ['8', 'Digit8', false, true],
    ] as const;
    for (const [key, code, altKey, shiftKey] of shortcuts) {
      const initial = key === '0'
        ? { type: 'doc', content: [{ type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'heading' }] }] }
        : 'alpha beta gamma';
      const shortcutEditor = createEditor(initial);
      shortcutEditor.editor.commands.setTextSelection({ from: 2, to: 6 });
      const before = shortcutEditor.editor.getJSON();
      shortcutEditor.editor.view.dom.dispatchEvent(new KeyboardEvent('keydown', {
        bubbles: true, cancelable: true, ctrlKey: true, altKey, shiftKey,
        key, code,
      }));
      expect(shortcutEditor.editor.getJSON()).toEqual(before);
    }

    const code = createEditor({ type: 'doc', content: [{ type: 'codeBlock', content: [{ type: 'text', text: 'abc' }] }] });
    code.editor.commands.setTextSelection(1);
    code.editor.view.focus();
    code.editor.view.someProp('handleKeyDown', handler => handler(code.editor.view, new KeyboardEvent('keydown', {
      bubbles: true, cancelable: true, key: 'Backspace', code: 'Backspace',
    })));
    expect(code.editor.state.doc.firstChild?.type.name).toBe('paragraph');

    const tripleEnter = createEditor({ type: 'doc', content: [{ type: 'codeBlock', content: [{ type: 'text', text: 'abc\n\n' }] }] });
    tripleEnter.editor.commands.setTextSelection(6);
    tripleEnter.editor.view.focus();
    expect(tripleEnter.editor.commands.keyboardShortcut('Enter')).toBe(true);
    expect(tripleEnter.editor.state.doc.firstChild?.type.name).toBe('codeBlock');
    expect(tripleEnter.editor.state.doc.childCount).toBe(2);
    expect(tripleEnter.editor.state.doc.lastChild?.type.name).toBe('paragraph');
  });
});
