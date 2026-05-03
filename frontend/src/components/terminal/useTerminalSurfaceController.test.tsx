import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const terminalMocks = vi.hoisted(() => ({
  sessions: [] as Array<{ id: string }>,
  activeSessionId: null as string | null,
  createSession: vi.fn(),
  closeSession: vi.fn(),
  loadSessions: vi.fn(),
  setActiveSession: vi.fn(),
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  isInitialized: true,
  tabs: [] as WorkspaceTab[],
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
        workspace: { tabs: workspaceMocks.tabs },
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
    terminalMocks.createSession.mockReset();
    terminalMocks.closeSession.mockReset();
    terminalMocks.loadSessions.mockReset();
    terminalMocks.loadSessions.mockResolvedValue(undefined);
    terminalMocks.setActiveSession.mockReset();
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.isInitialized = true;
    workspaceMocks.tabs = [terminalTab];
  });

  it('cria sessão para aba ativa sem sessionId', async () => {
    terminalMocks.createSession.mockImplementation(async () => {
      terminalMocks.activeSessionId = 'session-1';
    });

    renderHook(() => useTerminalSurfaceController(terminalTab, true));

    await waitFor(() => {
      expect(terminalMocks.createSession).toHaveBeenCalled();
      expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
        state: { sessionId: 'session-1' },
      });
    });
  });

  it('ativa sessão existente para aba ativa', async () => {
    terminalMocks.sessions = [{ id: 'session-2' }];
    renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-2' },
    }, true));

    await waitFor(() => {
      expect(terminalMocks.setActiveSession).toHaveBeenCalledWith('session-2');
    });
    expect(terminalMocks.createSession).not.toHaveBeenCalled();
  });

  it('preserva sessão quando o painel desmonta mas a aba continua aberta', () => {
    const { unmount } = renderHook(() => useTerminalSurfaceController({
      ...terminalTab,
      state: { sessionId: 'session-3' },
    }, true));

    unmount();

    expect(terminalMocks.closeSession).not.toHaveBeenCalled();
  });

  it('fecha a última sessão conhecida quando a aba sai do workspace', async () => {
    terminalMocks.createSession.mockImplementation(async () => {
      terminalMocks.activeSessionId = 'session-created';
    });
    const { unmount } = renderHook(() => useTerminalSurfaceController(terminalTab, true));

    await waitFor(() => {
      expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-tab', {
        state: { sessionId: 'session-created' },
      });
    });

    workspaceMocks.tabs = [];
    unmount();

    expect(terminalMocks.closeSession).toHaveBeenCalledWith('session-created');
  });
});
