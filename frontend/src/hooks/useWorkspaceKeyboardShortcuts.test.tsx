import { describe, expect, it, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useWorkspaceKeyboardShortcuts } from './useWorkspaceKeyboardShortcuts';
import { dispatchKey, expectGlobalShortcutIgnoredWhileModalOpen } from '../test/a11yHelpers';

/*
 * Demonstração do helper `expectGlobalShortcutIgnoredWhileModalOpen`.
 *
 * Os atalhos de troca/navegação de aba pertencem ao Topbar local_ui.
 * Este hook legado não os intercepta nem altera a aba ativa.
 */

const setActiveTab = vi.fn();
const addTab = vi.fn();
const removeTab = vi.fn(() => Promise.resolve());
const createWorkspace = vi.fn();
const createWorkspaceTab = vi.fn();
const announce = vi.fn();

const workspaceState = {
  workspace: {
    id: 'ws-1',
    name: 'Workspace de teste',
    profile: '',
    tabs: [
      { id: 't1', type: 'chat' as const, title: 'Aba 1', position: 0, conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001' },
      { id: 't2', type: 'chat' as const, title: 'Aba 2', position: 1, conversationId: '01926b90-7a5a-7c4e-8d3f-000000000002' },
    ],
    activeTabId: 't1',
  },
  addTab,
  removeTab,
  setActiveTab,
  createWorkspace,
};

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (state: typeof workspaceState) => unknown) =>
      selector ? selector(workspaceState) : workspaceState,
    { getState: () => workspaceState },
  ),
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: { getState: () => ({ requestOpen: vi.fn() }) },
}));

vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce }),
}));

vi.mock('../lib/createWorkspaceTab', () => ({
  createWorkspaceTab: (...args: unknown[]) => createWorkspaceTab(...args),
}));

describe('useWorkspaceKeyboardShortcuts — atalhos globais respeitam o modal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    createWorkspaceTab.mockResolvedValue('terminal-tab');
  });

  it('Ctrl+2 não é interceptado pelo hook legado enquanto um Modal está aberto', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    expectGlobalShortcutIgnoredWhileModalOpen({
      backgroundAction: setActiveTab,
      expectPreventDefault: false,
      dispatch: () => dispatchKey({ key: '2', ctrlKey: true }),
    });
  });

  it('Ctrl+Tab não é interceptado pelo hook legado enquanto um Modal está aberto', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    expectGlobalShortcutIgnoredWhileModalOpen({
      backgroundAction: setActiveTab,
      expectPreventDefault: false,
      dispatch: () => dispatchKey({ key: 'Tab', ctrlKey: true }),
    });
  });

  it('controle: sem modal aberto, Ctrl+2 fica para o Topbar local_ui', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ key: '2', ctrlKey: true });

    expect(event.defaultPrevented).toBe(false);
    expect(setActiveTab).not.toHaveBeenCalled();
  });

  it('Ctrl+N não executa mais o chord legado nem chama createWorkspaceTab', async () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const prefix = dispatchKey({ key: 'n', ctrlKey: true });
    dispatchKey({ key: 'r' });

    await waitFor(() => expect(createWorkspaceTab).not.toHaveBeenCalled());
    expect(prefix.defaultPrevented).toBe(false);
    expect(addTab).not.toHaveBeenCalled();
  });

  it('Ctrl+Shift+N não cria workspace pelo hook legado', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ key: 'N', code: 'KeyN', ctrlKey: true, shiftKey: true });

    expect(event.defaultPrevented).toBe(false);
    expect(createWorkspace).not.toHaveBeenCalled();
    expect(addTab).not.toHaveBeenCalled();
  });
});
