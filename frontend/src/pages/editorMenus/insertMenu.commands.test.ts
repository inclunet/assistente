import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Editor } from '@tiptap/core';
import type { MenuItem } from '../../components/menu';
import type { EditorDocument } from '../../store/editorStore';

const mocks = vi.hoisted(() => ({
  captureEditorFormatting: vi.fn(),
  requestEditorFormatCommand: vi.fn(),
}));

vi.mock('../../lib/commandEditorFormatting', () => mocks);

import { buildInsertMenuItemsForContextMenu } from './insertMenu';

const baseDocument: EditorDocument = {
  id: 'doc-insert-menu',
  title: 'Documento',
  markdown: '',
  mode: 'rich',
  readOnly: false,
  loadError: false,
};

function findItem(items: MenuItem[], id: string): MenuItem | undefined {
  for (const item of items) {
    if (item.id === id) return item;
    const nested = item.submenu && findItem(item.submenu, id);
    if (nested) return nested;
  }
  return undefined;
}

function makeContext(document: EditorDocument = baseDocument) {
  const richEditor = Object.create(Editor.prototype) as Editor;
  return {
    activeTab: document,
    isAsking: false,
    editorReadyNonce: 1,
    richEditorRef: { current: richEditor },
    applyInsertRequest: vi.fn(async () => true),
    appendMarkdownToDocument: vi.fn(),
    focusEditorSoon: vi.fn(),
    addToast: vi.fn(),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.captureEditorFormatting.mockReturnValue({ canExecute: () => true, dispose: vi.fn() });
});

describe('menu Inserir e comandos centrais', () => {
  it('consulta disponibilidade ao mostrar o menu, após registro dos effects', () => {
    mocks.captureEditorFormatting.mockReturnValue(undefined);
    const items = buildInsertMenuItemsForContextMenu({ ctx: makeContext() });
    const item = findItem(items, 'ins-codeblock');
    expect(item?.disabled).toBe(true);
    mocks.captureEditorFormatting.mockReturnValue({ canExecute: () => true, dispose: vi.fn() });
    expect(item?.disabled).toBe(false);
    mocks.captureEditorFormatting.mockReturnValue(undefined);
    item?.action?.();
    expect(mocks.requestEditorFormatCommand).not.toHaveBeenCalled();
  });
  it('encaminha inserções rich sem mutação direta', () => {
    const context = makeContext();
    const items = buildInsertMenuItemsForContextMenu({ ctx: context });

    findItem(items, 'ins-table-r2-c3-hdr-on')?.action?.();
    findItem(items, 'ins-mermaid')?.action?.();
    findItem(items, 'ins-codeblock')?.action?.();
    findItem(items, 'ins-bullets')?.action?.();
    findItem(items, 'ins-numbers')?.action?.();
    findItem(items, 'ins-blockquote')?.action?.();

    expect(mocks.requestEditorFormatCommand.mock.calls).toEqual([
      ['editor.format.table.insert', { kind: 'table', rows: 2, cols: 3, withHeaderRow: true }, context.richEditorRef.current],
      ['editor.format.mermaid.insert', undefined, context.richEditorRef.current],
      ['editor.format.code_block.insert', undefined, context.richEditorRef.current],
      ['editor.format.list.bullet', undefined, context.richEditorRef.current],
      ['editor.format.list.ordered', undefined, context.richEditorRef.current],
      ['editor.format.blockquote', undefined, context.richEditorRef.current],
    ]);
  });

  it('desabilita ações rich quando a capability capturada recusa', () => {
    mocks.captureEditorFormatting.mockReturnValue({ canExecute: () => false, dispose: vi.fn() });
    const items = buildInsertMenuItemsForContextMenu({ ctx: makeContext() });

    expect(findItem(items, 'ins-table-r2-c3-hdr-on')?.disabled).toBe(true);
    expect(findItem(items, 'ins-mermaid')?.disabled).toBe(true);
    expect(findItem(items, 'ins-codeblock')?.disabled).toBe(true);
    expect(findItem(items, 'ins-bullets')?.disabled).toBe(true);
    expect(findItem(items, 'ins-numbers')?.disabled).toBe(true);
    expect(findItem(items, 'ins-blockquote')?.disabled).toBe(true);
    expect(mocks.requestEditorFormatCommand).not.toHaveBeenCalled();
  });

  it.each([
    ['readonly', { readOnly: true }],
    ['loadError', { loadError: true }],
  ] as const)('bloqueia comandos rich quando o documento está %s', (_reason, flags) => {
    const context = makeContext({ ...baseDocument, ...flags });
    const items = buildInsertMenuItemsForContextMenu({ ctx: context });
    expect(findItem(items, 'ins-mermaid')?.disabled).toBe(true);
    expect(findItem(items, 'ins-codeblock')?.disabled).toBe(true);
    expect(findItem(items, 'ins-bullets')?.disabled).toBe(true);
    expect(findItem(items, 'ins-blockquote')?.disabled).toBe(true);
    expect(mocks.captureEditorFormatting).not.toHaveBeenCalled();
  });

  it('migra Markdown pelo mesmo comando, com documento original e sem fallback legado', () => {
    const context = makeContext({ ...baseDocument, mode: 'markdown' });
    const items = buildInsertMenuItemsForContextMenu({ ctx: context });
    findItem(items, 'ins-codeblock')?.action?.();
    expect(context.applyInsertRequest).not.toHaveBeenCalled();
    expect(mocks.requestEditorFormatCommand).toHaveBeenCalledWith('editor.format.code_block.insert', undefined, context.activeTab);
    findItem(items, 'ins-table-r2-c3-md')?.action?.();
    expect(mocks.requestEditorFormatCommand).toHaveBeenCalledWith('editor.format.table.insert', { kind: 'table', rows: 2, cols: 3, withHeaderRow: true }, context.activeTab);
  });
  it.each(['rich', 'markdown'] as const)('todos os onze templates usam comandos em %s', mode => {
    const context = makeContext({ ...baseDocument, mode });
    const items = buildInsertMenuItemsForContextMenu({ ctx: context });
    const templates = findItem(items, 'ins-slide')?.submenu ?? [];
    expect(templates).toHaveLength(11);
    templates.forEach(template => template.action?.());
    expect(mocks.requestEditorFormatCommand).toHaveBeenCalledTimes(11);
    expect(new Set(mocks.requestEditorFormatCommand.mock.calls.map(call => call[0])).size).toBe(11);
    expect(mocks.requestEditorFormatCommand.mock.calls.every(call => call[2] === context.activeTab)).toBe(true);
    expect(context.appendMarkdownToDocument).not.toHaveBeenCalled();
  });
});
