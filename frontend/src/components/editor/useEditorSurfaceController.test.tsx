import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const editorMocks = vi.hoisted(() => ({
  createDocument: vi.fn(),
  removeDocument: vi.fn(),
  renameDocument: vi.fn(),
  subscribe: vi.fn(),
  documents: {} as Record<string, unknown>,
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  isInitialized: true,
  tabs: [] as WorkspaceTab[],
  activeTabId: 'editor-tab' as string | null,
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
        workspace: { tabs: workspaceMocks.tabs, activeTabId: workspaceMocks.activeTabId },
        updateTab: workspaceMocks.updateTab,
      }),
    },
  ),
}));

vi.mock('i18next', () => ({
  default: {
    t: vi.fn((key: string) => (key === 'editor.fallback.newDoc' ? 'Novo documento' : key)),
  },
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
    editorMocks.removeDocument.mockReset();
    editorMocks.renameDocument.mockReset();
    editorMocks.subscribe.mockReset();
    editorMocks.documents = {};
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.isInitialized = true;
    workspaceMocks.tabs = [editorTab];
    workspaceMocks.activeTabId = 'editor-tab';
  });

  it('cria documento para a aba ativa por tabId explícito', async () => {
    renderHook(() => useEditorSurfaceController(editorTab, true));

    await waitFor(() => {
      expect(editorMocks.createDocument).toHaveBeenCalledWith(expect.objectContaining({
        id: 'editor-tab',
        filePath: 'C:/tmp/doc.md',
        markdown: '# Arquivo',
      }));
    });
  });

  it('não cria documento quando o painel está inativo', async () => {
    renderHook(() => useEditorSurfaceController(editorTab, false));

    expect(editorMocks.createDocument).not.toHaveBeenCalled();
  });

  it('preserva documento existente quando painel volta a ficar ativo', () => {
    editorMocks.documents = {
      'editor-tab': { id: 'editor-tab', title: 'Editor', filePath: null },
    };
    const { rerender } = renderHook(({ active }) => useEditorSurfaceController(editorTab, active), {
      initialProps: { active: true },
    });

    rerender({ active: false });
    rerender({ active: true });

    expect(editorMocks.createDocument).not.toHaveBeenCalled();
  });

  it('usa fallback i18n para título de documento novo sem arquivo', async () => {
    renderHook(() => useEditorSurfaceController({
      ...editorTab,
      state: {},
    }, true));

    await waitFor(() => {
      expect(editorMocks.createDocument).toHaveBeenCalledWith(expect.objectContaining({
        title: 'Novo documento',
      }));
    });
  });

  it('não cria nem ativa documento se a aba deixa de estar ativa durante leitura assíncrona', async () => {
    let resolveRead: (value: string) => void = () => undefined;
    const { EditorReadFile } = await import('@wailsjs/go/app/App');
    vi.mocked(EditorReadFile).mockImplementationOnce(() => new Promise((resolve) => {
      resolveRead = resolve;
    }));

    renderHook(() => useEditorSurfaceController(editorTab, true));
    workspaceMocks.activeTabId = 'other-tab';
    resolveRead('# Depois');
    await Promise.resolve();

    await waitFor(() => {
      expect(EditorReadFile).toHaveBeenCalledWith('C:/tmp/doc.md');
    });
    expect(editorMocks.createDocument).not.toHaveBeenCalled();
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
