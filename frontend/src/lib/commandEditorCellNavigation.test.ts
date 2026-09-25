import { afterEach, describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import { CellSelection } from '@tiptap/pm/tables';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import {
  canNavigateCell,
  navigateEditorCell,
} from './commandEditorCellNavigation';

type JsonNode = {
  type: string;
  text?: string;
  content?: JsonNode[];
};

const editors: Editor[] = [];
const roots: HTMLElement[] = [];

function createEditor(readOnly = false) {
  const root = document.createElement('div');
  document.body.append(root);
  const editor = new Editor({
    element: root,
    extensions: buildRichTextExtensions({
      placeholder: 'x',
      imageFallbackLabel: 'Imagem',
      imageLabelPrefix: 'Imagem',
    }),
    editable: !readOnly,
    content: tableDocument(),
  });
  roots.push(root);
  editors.push(editor);
  return editor;
}

function tableDocument(): JsonNode {
  const cell = (text: string): JsonNode => ({
    type: 'tableCell',
    content: [{ type: 'paragraph', content: [{ type: 'text', text }] }],
  });
  const row = (prefix: string): JsonNode => ({
    type: 'tableRow',
    content: [cell(`${prefix} one`), cell(`${prefix} two`)],
  });
  return {
    type: 'doc',
    content: [
      { type: 'paragraph', content: [{ type: 'text', text: 'outside' }] },
      { type: 'table', content: [row('a'), row('b')] },
    ],
  };
}

function cellPositions(editor: Editor): number[] {
  const positions: number[] = [];
  editor.state.doc.descendants((node, position) => {
    if (node.type.name === 'tableCell' || node.type.name === 'tableHeader') positions.push(position);
  });
  return positions;
}

function selectTextCell(editor: Editor, index: number): void {
  const position = cellPositions(editor)[index];
  editor.commands.setTextSelection(position + 2);
}

afterEach(() => {
  while (editors.length) editors.pop()?.destroy();
  while (roots.length) roots.pop()?.remove();
});

describe('commandEditorCellNavigation', () => {
  it('navega em uma tabela 2x2 sem alterar o documento e sem acrescentar linha', () => {
    const editor = createEditor();
    selectTextCell(editor, 0);
    const before = editor.getJSON();

    expect(canNavigateCell(editor, 'editor.table.cell.next')).toBe(true);
    expect(navigateEditorCell(editor, 'editor.table.cell.next')).toBe(true);
    expect(editor.state.selection.from).toBe(cellPositions(editor)[1] + 2);
    expect(editor.getJSON()).toEqual(before);

    selectTextCell(editor, 3);
    expect(canNavigateCell(editor, 'editor.table.cell.next')).toBe(false);
    expect(navigateEditorCell(editor, 'editor.table.cell.next')).toBe(false);
    expect(editor.getJSON()).toEqual(before);
  });

  it('navega para trás e rejeita as bordas sem mutação', () => {
    const editor = createEditor();
    selectTextCell(editor, 3);
    const before = editor.getJSON();

    expect(navigateEditorCell(editor, 'editor.table.cell.previous')).toBe(true);
    expect(editor.state.selection.from).toBe(cellPositions(editor)[2] + 2);
    expect(editor.getJSON()).toEqual(before);

    selectTextCell(editor, 0);
    expect(canNavigateCell(editor, 'editor.table.cell.previous')).toBe(false);
    expect(editor.getJSON()).toEqual(before);
  });

  it('converte CellSelection em seleção de texto na célula seguinte', () => {
    const editor = createEditor();
    const positions = cellPositions(editor);
    editor.view.dispatch(editor.state.tr.setSelection(CellSelection.create(editor.state.doc, positions[0])));
    const before = editor.getJSON();

    expect(navigateEditorCell(editor, 'editor.table.cell.next')).toBe(true);
    expect(editor.state.selection).not.toBeInstanceOf(CellSelection);
    expect(editor.state.selection.from).toBe(positions[1] + 2);
    expect(editor.getJSON()).toEqual(before);
  });

  it('rejeita seleção fora de tabela, comando desconhecido e editor somente leitura', () => {
    const editor = createEditor();
    editor.commands.setTextSelection(1);
    expect(canNavigateCell(editor, 'editor.table.cell.next')).toBe(false);
    expect(navigateEditorCell(editor, 'editor.table.cell.next')).toBe(false);
    expect(canNavigateCell(editor, 'editor.table.cell.unknown')).toBe(false);

    const readOnlyEditor = createEditor(true);
    selectTextCell(readOnlyEditor, 0);
    expect(canNavigateCell(readOnlyEditor, 'editor.table.cell.next')).toBe(false);
    expect(navigateEditorCell(readOnlyEditor, 'editor.table.cell.next')).toBe(false);
  });
});
