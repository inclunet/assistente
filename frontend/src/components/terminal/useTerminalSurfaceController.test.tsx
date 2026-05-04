import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const terminalMocks = vi.hoisted(() => ({
  sessions: [] as Array<{ id: string }>,
  activeSessionId: null as string | null,
  historyBySession: {} as Record<string, unknown[]>,
  createSession: vi.fn(),
  closeSession: vi.fn(),
  loadSessions: vi.fn(),
  loadHistory: vi.fn(),
  setActiveSession: vi.fn(),
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
    terminalMocks.activeSessionId = null;
    terminalMocks.historyBySession = {};
    terminalMocks.createSession.mockReset();
    terminalMocks.closeSession.mockReset();
    terminalMocks.loadSessions.mockReset();
    terminalMocks.loadSessions.mockResolvedValue(undefined);
    terminalMocks.loadHistory.mockReset();
    terminalMocks.setActiveSession.mockReset();
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.isInitialized = true;
    workspaceMocks.tabs = [terminalTab];
    workspaceMocks.activeTabId = 'terminal-tab';
  });

  it('cria sessão para aba ativa sem sessionId', async () => {
    terminalMocks.createSession.mockResolvedValue('session-1');

    renderHook(() => useTerminalSurfaceController(terminalTab, true));

    await waitFor(() => {
      expect(terminalMocks.createSession).toHaveBeenCalled();
      expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
        state: { sessionId: 'session-1' },
      });
    });
  });

  it('carrega histórico de sessão existente sem ativar singleton global', async () => {
    terminalMocks.sessions = [{ id: 'session-2' }];
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-2' },
    }, true));

    await waitFor(() => {
      expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-2');
    });
    expect(terminalMocks.setActiveSession).not.toHaveBeenCalled();
    expect(terminalMocks.createSession).not.toHaveBeenCalled();
  });

  it('preserva sessão existente sem reativar singleton global quando painel volta a ficar ativo', () => {
    terminalMocks.sessions = [{ id: 'session-2' }];
    terminalMocks.activeSessionId = 'session-other';
    const tabWithSession = {
      ...terminalTab,
      state: { sessionId: 'session-2' },
    };
    const { rerender } = renderHook(({ active }) => useTerminalSurfaceController(tabWithSession, active), {
      initialProps: { active: true },
    });

    expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-2');
    expect(terminalMocks.setActiveSession).not.toHaveBeenCalled();

    terminalMocks.loadHistory.mockClear();
    terminalMocks.setActiveSession.mockClear();
    terminalMocks.activeSessionId = 'session-other';
    rerender({ active: false });
    rerender({ active: true });

    expect(terminalMocks.loadHistory).toHaveBeenCalledWith('session-2');
    expect(terminalMocks.setActiveSession).not.toHaveBeenCalled();
  });

  it('recupera sessão quando loadSessions falha', async () => {
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
      expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
        state: { sessionId: 'session-recovered' },
      });
    });

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
    expect(terminalMocks.setActiveSession).not.toHaveBeenCalledWith('session-late');
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

  it('não fecha sessão no unmount porque o cleanup é centralizado no workspace', async () => {
    terminalMocks.createSession.mockResolvedValue('session-created');
    const { unmount } = renderHook(() => useTerminalSurfaceController(terminalTab, true));

    await waitFor(() => {
      expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
        state: { sessionId: 'session-created' },
      });
    });

    workspaceMocks.tabs = [];
    unmount();

    expect(terminalMocks.closeSession).not.toHaveBeenCalled();
  });
});
