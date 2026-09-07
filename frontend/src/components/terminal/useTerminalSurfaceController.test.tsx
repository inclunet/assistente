import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const terminalMocks = vi.hoisted(() => ({
  sessions: [] as Array<{ id: string }>,
  historyBySession: {} as Record<string, unknown[]>,
  createSession: vi.fn(),
  closeSession: vi.fn(),
  loadSessions: vi.fn(),
  loadHistory: vi.fn(),
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  isInitialized: true,
  tabs: [] as WorkspaceTab[],
  activeTabId: 'terminal-tab' as string | null,
}));

vi.mock('../../store/terminalStore', () => ({
  useTerminalStore: {
    getState: () => terminalMocks,
  },
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
      }),
    },
  ),
}));

import { useTerminalSurfaceController } from './useTerminalSurfaceController';

const terminalTab: WorkspaceTab = {
  id: 'terminal-tab',
  type: 'terminal',
  title: 'Terminal',
  position: 0,
  state: {},
};

describe('useTerminalSurfaceController', () => {
  beforeEach(() => {
    terminalMocks.sessions = [];
    terminalMocks.historyBySession = {};
    terminalMocks.createSession.mockReset();
    terminalMocks.closeSession.mockReset();
    terminalMocks.loadSessions.mockReset();
    terminalMocks.loadSessions.mockResolvedValue(undefined);
    terminalMocks.loadHistory.mockReset();
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.isInitialized = true;
    workspaceMocks.tabs = [terminalTab];
    workspaceMocks.activeTabId = 'terminal-tab';
  });

  it('não cria sessão implicitamente para aba ativa sem sessionId', () => {
    renderHook(() => useTerminalSurfaceController(terminalTab, true));

    expect(terminalMocks.createSession).not.toHaveBeenCalled();
    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();
  });

  it('recarrega a lista para resolver um sessionId ainda ausente do store', async () => {
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-1' },
    }, true));

    await waitFor(() => {
      expect(terminalMocks.loadSessions).toHaveBeenCalled();
    });
    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();
  });

  it('carrega histórico de sessão existente por sessionId explícito', async () => {
    terminalMocks.sessions = [{ id: 'session-2' }];
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-2' },
    }, true));

    await waitFor(() => {
      expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-2');
    });
    expect(terminalMocks.createSession).not.toHaveBeenCalled();
  });

  it('preserva sessão existente quando painel volta a ficar ativo', () => {
    terminalMocks.sessions = [{ id: 'session-2' }];
    const tabWithSession = {
      ...terminalTab,
      state: { sessionId: 'session-2' },
    };
    const { rerender } = renderHook(({ active }) => useTerminalSurfaceController(tabWithSession, active), {
      initialProps: { active: true },
    });

    expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-2');

    terminalMocks.loadHistory.mockClear();
    rerender({ active: false });
    rerender({ active: true });

    expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-2');
  });

  it('informa falha sem substituir a sessão referenciada', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    terminalMocks.loadSessions.mockRejectedValue(new Error('load failed'));
    terminalMocks.createSession.mockResolvedValue('session-recovered');

    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'stale-session' },
    }, true));

    await waitFor(() => {
      expect(errorSpy).toHaveBeenCalledWith(
        '[TerminalSurfaceController] Erro ao carregar sessões:',
        expect.any(Error),
      );
    });
    expect(terminalMocks.createSession).not.toHaveBeenCalled();
    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();

    errorSpy.mockRestore();
  });

  it('não carrega histórico se a aba deixa de estar ativa antes do load terminar', async () => {
    let resolveLoad: () => void = () => undefined;
    terminalMocks.loadSessions.mockImplementationOnce(() => new Promise<void>((resolve) => {
      resolveLoad = resolve;
    }));

    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-late' },
    }, true));
    workspaceMocks.activeTabId = 'other-tab';
    terminalMocks.sessions = [{ id: 'session-late' }];
    resolveLoad();
    await Promise.resolve();

    expect(terminalMocks.loadHistory).not.toHaveBeenCalledWith('session-late');
    expect(workspaceMocks.updateTab).not.toHaveBeenCalledWith('terminal-tab', {
      state: { sessionId: expect.any(String) },
    });
  });

  it('preserva sessão quando o painel desmonta mas a aba continua aberta', () => {
    const { unmount } = renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-3' },
    }, true));

    unmount();

    expect(terminalMocks.closeSession).not.toHaveBeenCalled();
  });

  it('não cria nem fecha sessão ao desmontar uma aba vazia', () => {
    const { unmount } = renderHook(() => useTerminalSurfaceController(terminalTab, true));

    workspaceMocks.tabs = [];
    unmount();

    expect(terminalMocks.createSession).not.toHaveBeenCalled();
    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();
    expect(terminalMocks.closeSession).not.toHaveBeenCalled();
  });
});
