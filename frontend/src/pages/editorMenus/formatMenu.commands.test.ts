import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { MenuItem } from '../../components/menu';
import type { RichTextEditorHandle } from '../../components/editor/RichTextEditor';
import type { EditorDocument } from '../../store/editorStore';
import { Editor } from '@tiptap/core';

const mocks = vi.hoisted(() => ({
  captureEditorFormatting: vi.fn(),
  requestEditorFormatCommand: vi.fn(),
}));

vi.mock('../../lib/commandEditorFormatting', () => mocks);

import { buildFormatMenuItemsForContextMenu } from './formatMenu';

const migratedCommandToMenuID = {
  'editor.format.bold': 'fmt-bold',
  'editor.format.italic': 'fmt-italic',
  'editor.format.strike': 'fmt-strike',
  'editor.format.paragraph': 'fmt-p',
  'editor.format.heading.h1': 'fmt-h1',
  'editor.format.heading.h2': 'fmt-h2',
  'editor.format.heading.h3': 'fmt-h3',
  'editor.format.heading.h4': 'fmt-h4',
  'editor.format.heading.h5': 'fmt-h5',
  'editor.format.heading.h6': 'fmt-h6',
  'editor.format.blockquote': 'fmt-bq',
  'editor.format.code_block': 'fmt-code',
  'editor.format.list.bullet': 'fmt-ul',
  'editor.format.list.ordered': 'fmt-ol',
  'editor.format.clear_marks': 'fmt-clear-marks',
  'editor.format.link.remove': 'fmt-link-unset',
  'editor.format.link.set': 'fmt-link-set',
  'editor.format.table.row.before': 'fmt-table-row-before',
  'editor.format.table.row.after': 'fmt-table-row-after',
  'editor.format.table.row.delete': 'fmt-table-del-row',
  'editor.format.table.column.before': 'fmt-table-col-before',
  'editor.format.table.column.after': 'fmt-table-col-after',
  'editor.format.table.column.delete': 'fmt-table-del-col',
  'editor.format.table.header.row': 'fmt-table-toggle-header-row',
  'editor.format.table.header.column': 'fmt-table-toggle-header-col',
  'editor.format.table.header.cell': 'fmt-table-toggle-header-cell',
  'editor.format.table.merge': 'fmt-table-merge',
  'editor.format.table.split': 'fmt-table-split',
  'editor.format.table.delete': 'fmt-table-delete',
} as const;

const baseDocument: EditorDocument = {
  id: 'doc-format-menu',
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
  const directChain = vi.fn();
  const capabilityChain = {
    chain: () => capabilityChain,
    focus: () => capabilityChain,
    goToPreviousCell: () => capabilityChain,
    goToNextCell: () => capabilityChain,
    run: () => true,
  };
  const rich = Object.assign(Object.create(Editor.prototype), {
    chain: directChain,
    can: () => capabilityChain,
    isActive: (name: string) => name === 'table' || name === 'link',
  });
  const richEditorHandle: RichTextEditorHandle = {
    getMarkdown: () => '',
    flushMarkdown: vi.fn(),
    openLinkDialog: vi.fn(async () => {}),
  };

  return {
    context: {
      activeTab: document,
      isAsking: false,
      editorReadyNonce: 1,
      richEditorRef: { current: rich },
      richEditorHandleRef: { current: richEditorHandle },
    },
    directChain,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.captureEditorFormatting.mockReturnValue({
    canExecute: () => true,
    dispose: vi.fn(),
  });
});

describe('menu de formatação e comandos centrais', () => {
  it('despacha o ID de cada item migrado sem mutação Tiptap direta', () => {
    const { context, directChain } = makeContext();
    const items = buildFormatMenuItemsForContextMenu({ ctx: context });

    for (const menuID of Object.values(migratedCommandToMenuID)) {
      const item = findItem(items, menuID);
      expect(item, `item ${menuID}`).toBeDefined();
      expect(item?.disabled, `item ${menuID}`).toBe(false);
      item?.action?.();
    }

    expect(mocks.requestEditorFormatCommand.mock.calls.map(([id]) => id)).toEqual(Object.keys(migratedCommandToMenuID));
    expect(directChain).not.toHaveBeenCalled();
  });

  it('mantém itens migrados desabilitados quando a admissão/capability recusa', () => {
    mocks.captureEditorFormatting.mockReturnValue({
      canExecute: () => false,
      dispose: vi.fn(),
    });
    const { context, directChain } = makeContext();
    const items = buildFormatMenuItemsForContextMenu({ ctx: context });

    expect(findItem(items, 'fmt-bold')?.disabled).toBe(true);
    expect(findItem(items, 'fmt-table-row-before')?.disabled).toBe(true);
    expect(mocks.requestEditorFormatCommand).not.toHaveBeenCalled();
    expect(directChain).not.toHaveBeenCalled();
  });

  it.each([
    ['readonly', { readOnly: true }],
    ['loadError', { loadError: true }],
  ] as const)('bloqueia o menu quando o documento está %s', (_reason, flags) => {
    const { context } = makeContext({ ...baseDocument, ...flags });
    const items = buildFormatMenuItemsForContextMenu({ ctx: context });

    for (const menuID of Object.values(migratedCommandToMenuID)) {
      expect(findItem(items, menuID)?.disabled, `item ${menuID}`).toBe(true);
    }
    expect(mocks.captureEditorFormatting).not.toHaveBeenCalled();
    expect(mocks.requestEditorFormatCommand).not.toHaveBeenCalled();
  });
});
