import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const terminalMocks = vi.hoisted(() => ({
  isLoadingSessions: false,
  sessionListFailed: false,
  sessions: [] as Array<{ id: string }>,
  historyBySession: {} as Record<string, unknown[]>,
  partialHistoryBySession: {} as Record<string, boolean>,
  createSession: vi.fn(),
  closeSession: vi.fn(),
  loadSessions: vi.fn(),
  loadHistory: vi.fn(),
  listeners: new Set<() => void>(),
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
    subscribe: (listener: (next: typeof terminalMocks, previous: typeof terminalMocks) => void) => {
      let previous = { ...terminalMocks };
      const notify = () => {
        const next = { ...terminalMocks };
        listener(next, previous);
        previous = next;
      };
      terminalMocks.listeners.add(notify);
      return () => terminalMocks.listeners.delete(notify);
    },
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
    terminalMocks.partialHistoryBySession = {};
    terminalMocks.createSession.mockReset();
    terminalMocks.closeSession.mockReset();
    terminalMocks.loadSessions.mockReset();
    terminalMocks.loadSessions.mockResolvedValue(undefined);
    terminalMocks.loadHistory.mockReset();
    terminalMocks.listeners.clear();
    terminalMocks.sessionListFailed = false;
    terminalMocks.isLoadingSessions = false;
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

  it('reconcilia sessão criada pelo backend antes do listener e carrega seu histórico sem criar outra', async () => {
    terminalMocks.loadSessions.mockImplementationOnce(async () => {
      terminalMocks.sessions = [{ id: 'session-backend' }];
    });
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-backend' },
    }, true));

    await waitFor(() => {
      expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-backend');
    });
    expect(terminalMocks.loadSessions).toHaveBeenCalledTimes(1);
    expect(terminalMocks.createSession).not.toHaveBeenCalled();
    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();
  });

  it('carrega o histórico quando a listagem substituta encontra a sessão ativa', async () => {
    terminalMocks.loadSessions.mockResolvedValueOnce(false);
    const { unmount } = renderHook(() => useTerminalSurfaceController({
      ...terminalTab, state: { sessionId: 'session-b' },
    }, true));
    await waitFor(() => expect(terminalMocks.loadSessions).toHaveBeenCalledTimes(1));
    expect(terminalMocks.loadHistory).not.toHaveBeenCalled();
    for (const loading of [true, false, true]) {
      terminalMocks.isLoadingSessions = loading;
      terminalMocks.listeners.forEach(notify => notify());
    }
    await Promise.resolve();
    terminalMocks.sessions = [{ id: 'session-a' }, { id: 'session-b' }];
    terminalMocks.listeners.forEach((listener) => listener());
    expect(terminalMocks.loadHistory).toHaveBeenCalledTimes(1);
    expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-b');
    expect(terminalMocks.listeners.size).toBe(0);
    terminalMocks.historyBySession['session-b'] = [{ command: 'anterior' }];
    terminalMocks.listeners.forEach((listener) => listener());
    expect(terminalMocks.loadHistory).toHaveBeenCalledTimes(1);
    unmount();
  });

  it.each([true, false])('distingue ausência confirmada de falha com nova tentativa (publicada: %s)', async published => {
    terminalMocks.sessions = [{ id: 'outra' }];
    terminalMocks.loadSessions.mockImplementationOnce(async () => {
      terminalMocks.sessionListFailed = !published;
      return published;
    });
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab, state: { sessionId: 'encerrada' },
    }, true));
    await waitFor(() => expect(terminalMocks.loadSessions).toHaveBeenCalledTimes(1));
    await Promise.resolve();
    expect(terminalMocks.listeners.size).toBe(published ? 0 : 1);
    expect(terminalMocks.loadHistory).not.toHaveBeenCalled();
    if (!published) {
      terminalMocks.sessionListFailed = false;
      terminalMocks.sessions = [{ id: 'outra' }, { id: 'encerrada' }];
      terminalMocks.listeners.forEach(notify => notify());
      expect(terminalMocks.loadHistory).toHaveBeenCalledWith('encerrada');
      expect(terminalMocks.listeners.size).toBe(0);
    }
  });

  it('encerra a observação quando a consulta de outra superfície termina sem a sessão', async () => {
    terminalMocks.loadSessions.mockResolvedValueOnce(false);
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab, state: { sessionId: 'encerrada' },
    }, true));
    terminalMocks.isLoadingSessions = true;
    terminalMocks.listeners.forEach(notify => notify());
    await Promise.resolve();
    expect(terminalMocks.listeners.size).toBe(1);
    terminalMocks.isLoadingSessions = false;
    terminalMocks.listeners.forEach(notify => notify());
    await Promise.resolve();
    expect(terminalMocks.listeners.size).toBe(0);
    expect(terminalMocks.loadHistory).not.toHaveBeenCalled();
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

  it.each([true, false])('só reconcilia o cache existente quando parcial: %s', partial => {
    terminalMocks.sessions = [{ id: 's' }];
    terminalMocks.historyBySession.s = [{}];
    terminalMocks.partialHistoryBySession.s = partial;
    renderHook(() => useTerminalSurfaceController({ ...terminalTab, state: { sessionId: 's' } }, true));
    expect(terminalMocks.loadHistory).toHaveBeenCalledTimes(partial ? 1 : 0);
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
