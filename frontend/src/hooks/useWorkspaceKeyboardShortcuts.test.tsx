import { describe, expect, it, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useWorkspaceKeyboardShortcuts } from './useWorkspaceKeyboardShortcuts';
import { dispatchKey, expectGlobalShortcutIgnoredWhileModalOpen } from '../test/a11yHelpers';

/*
 * Demonstração do helper `expectGlobalShortcutIgnoredWhileModalOpen`.
 *
 * Os atalhos de TROCA/NAVEGAÇÃO de aba (Ctrl+1..9, Ctrl+Tab) respeitam
 * `isModalOpen()`: com um modal aberto, eles previnem o default do browser
 * mas NÃO trocam de aba (não chamam `setActiveTab`). Validamos exatamente
 * esse contrato reusando a infraestrutura real de `Modal`.
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

  it('Ctrl+2 (ir para aba) é ignorado enquanto um Modal está aberto', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    expectGlobalShortcutIgnoredWhileModalOpen({
      backgroundAction: setActiveTab,
      expectPreventDefault: true,
      dispatch: () => dispatchKey({ key: '2', ctrlKey: true }),
    });
  });

  it('Ctrl+Tab (próxima aba) é ignorado enquanto um Modal está aberto', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    expectGlobalShortcutIgnoredWhileModalOpen({
      backgroundAction: setActiveTab,
      expectPreventDefault: true,
      dispatch: () => dispatchKey({ key: 'Tab', ctrlKey: true }),
    });
  });

  it('controle: sem modal aberto, Ctrl+2 troca de aba normalmente', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ key: '2', ctrlKey: true });

    expect(setActiveTab).toHaveBeenCalledWith('t2');
  });

  it('Ctrl+N, R cria uma aba de terminal conectada pelo fluxo de domínio', async () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ key: 'n', ctrlKey: true });
    dispatchKey({ key: 'r' });

    await waitFor(() => {
      expect(createWorkspaceTab).toHaveBeenCalledWith('terminal', expect.any(String));
    });
    expect(addTab).not.toHaveBeenCalled();
  });
});
