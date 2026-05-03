import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const editorMocks = vi.hoisted(() => ({
  createDocument: vi.fn(),
  setActiveDocument: vi.fn(),
  removeDocument: vi.fn(),
  renameDocument: vi.fn(),
  subscribe: vi.fn(),
  documents: {} as Record<string, unknown>,
  activeDocumentId: null as string | null,
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  isInitialized: true,
  tabs: [] as WorkspaceTab[],
}));

vi.mock('@wailsjs/go/app/App', () => ({
  EditorReadFile: vi.fn().mockResolvedValue({ content: '# Arquivo' }),
}));

vi.mock('../../store/editorStore', () => ({
  DEFAULT_MD: '# Novo documento',
  useEditorStore: Object.assign(
    (selector: (state: typeof editorMocks) => unknown) => selector(editorMocks),
    {
      getState: () => editorMocks,
      subscribe: (listener: (state: typeof editorMocks) => void) => {
        editorMocks.subscribe(listener);
        return vi.fn();
      },
    },
  ),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector: (state: { isInitialized: boolean; updateTab: typeof workspaceMocks.updateTab }) => unknown) => selector({
      isInitialized: workspaceMocks.isInitialized,
      updateTab: workspaceMocks.updateTab,
    }),
    {
      getState: () => ({
        workspace: { tabs: workspaceMocks.tabs },
        updateTab: workspaceMocks.updateTab,
      }),
    },
  ),
}));

import { useEditorSurfaceController } from './useEditorSurfaceController';

const editorTab: WorkspaceTab = {
  id: 'editor-tab',
  type: 'editor',
  title: 'Editor',
  position: 0,
  state: { filePath: 'C:/tmp/doc.md' },
};

describe('useEditorSurfaceController', () => {
  beforeEach(() => {
    editorMocks.createDocument.mockReset();
    editorMocks.setActiveDocument.mockReset();
    editorMocks.removeDocument.mockReset();
    editorMocks.renameDocument.mockReset();
    editorMocks.subscribe.mockReset();
    editorMocks.documents = {};
    editorMocks.activeDocumentId = null;
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.isInitialized = true;
    workspaceMocks.tabs = [editorTab];
  });

  it('cria e ativa documento para a aba ativa', async () => {
    renderHook(() => useEditorSurfaceController(editorTab, true));

    await waitFor(() => {
      expect(editorMocks.createDocument).toHaveBeenCalledWith(expect.objectContaining({
        id: 'editor-tab',
        filePath: 'C:/tmp/doc.md',
        markdown: '# Arquivo',
      }));
      expect(editorMocks.setActiveDocument).toHaveBeenCalledWith('editor-tab');
    });
  });

  it('não cria documento quando o painel está inativo', async () => {
    renderHook(() => useEditorSurfaceController(editorTab, false));

    expect(editorMocks.createDocument).not.toHaveBeenCalled();
    expect(editorMocks.setActiveDocument).not.toHaveBeenCalled();
  });

  it('preserva documento quando o painel desmonta mas a aba continua aberta', () => {
    const { unmount } = renderHook(() => useEditorSurfaceController(editorTab, true));

    unmount();

    expect(editorMocks.removeDocument).not.toHaveBeenCalled();
  });

  it('remove documento quando o painel desmonta após a aba sair do workspace', () => {
    const { unmount } = renderHook(() => useEditorSurfaceController(editorTab, true));
    workspaceMocks.tabs = [];

    unmount();

    expect(editorMocks.removeDocument).toHaveBeenCalledWith('editor-tab');
  });
});
