import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';

const shortcutHints = vi.hoisted(() => ({
  current: {} as Record<string, string | undefined>,
}));

vi.mock('../lib/commandShortcutHints', () => ({
  useCommandShortcutHints: () => (commandID: string) => shortcutHints.current[commandID],
}));

import { useEditorMenus } from './useEditorMenus';

function makeArgs(): Parameters<typeof useEditorMenus>[0] {
  return {
    activeTab: {
      id: 'doc-1',
      title: 'Documento',
      markdown: '',
      mode: 'markdown',
      filePath: 'C:/documento.md',
      readOnly: false,
    },
    workspaceTab: { id: 'tab-1', type: 'editor' },
    isAsking: false,
    editorReadyNonce: 0,
    richEditorRef: { current: null },
    richEditorHandleRef: { current: null },
    mergeStateRevision: 0,
    getMergeSession: () => null,
    createDocument: () => 'doc-2',
    addWorkspaceTab: vi.fn().mockResolvedValue('tab-2'),
    abortMerge: vi.fn().mockResolvedValue(undefined),
    rememberCurrentExplicitSelection: vi.fn(),
    focusEditorSoon: vi.fn(),
    addToast: vi.fn(),
  } as unknown as Parameters<typeof useEditorMenus>[0];
}

describe('useEditorMenus — rótulos de atalhos', () => {
  it('usa somente a projeção efetiva e não inventa Ctrl+N para Novo', () => {
    shortcutHints.current = {
      'editor.file.open': 'Ctrl+Alt+O',
      'editor.file.save': undefined,
      'editor.file.save_copy': 'Ctrl+Shift+Y',
      'workspace.chat.open': 'Ctrl+Shift+I',
    };

    const { result } = renderHook(() => useEditorMenus(makeArgs()));
    const items = result.current.fileMenuItemsForContextMenu;

    expect(items.find(item => item.id === 'editor-toolbar-file-new')?.shortcut).toBeUndefined();
    expect(items.find(item => item.id === 'editor-toolbar-file-open')).toMatchObject({ shortcut: 'Ctrl+Alt+O' });
    expect(items.find(item => item.id === 'editor-toolbar-file-save')?.shortcut).toBeUndefined();
    expect(items.find(item => item.id === 'editor-toolbar-file-saveas')).toMatchObject({ shortcut: 'Ctrl+Shift+Y' });
    expect(result.current.actions[0]).toMatchObject({ shortcut: 'Ctrl+Shift+I' });
  });

  it('reage a remapeamento e à supressão sem manter o valor anterior', () => {
    shortcutHints.current = {
      'editor.file.open': 'Ctrl+Alt+O',
      'editor.file.save': 'Ctrl+S',
      'editor.file.save_copy': 'Ctrl+Shift+S',
    };
    const { result, rerender } = renderHook(() => useEditorMenus(makeArgs()));
    expect(result.current.fileMenuItemsForContextMenu.find(item => item.id === 'editor-toolbar-file-save')).toMatchObject({ shortcut: 'Ctrl+S' });

    shortcutHints.current = {
      'editor.file.open': 'Ctrl+Alt+P',
      'editor.file.save': undefined,
      'editor.file.save_copy': undefined,
    };
    rerender();

    expect(result.current.fileMenuItemsForContextMenu.find(item => item.id === 'editor-toolbar-file-open')).toMatchObject({ shortcut: 'Ctrl+Alt+P' });
    expect(result.current.fileMenuItemsForContextMenu.find(item => item.id === 'editor-toolbar-file-save')?.shortcut).toBeUndefined();
    expect(result.current.fileMenuItemsForContextMenu.find(item => item.id === 'editor-toolbar-file-saveas')?.shortcut).toBeUndefined();
  });
});
