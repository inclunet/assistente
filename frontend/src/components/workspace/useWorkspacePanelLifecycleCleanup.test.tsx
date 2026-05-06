import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceData } from '../../store/workspaceStore';

type WorkspaceListener = (
  state: { workspace: WorkspaceData | null },
  previousState: { workspace: WorkspaceData | null },
) => void;

const workspaceMocks = vi.hoisted(() => ({
  workspace: null as WorkspaceData | null,
  listeners: [] as WorkspaceListener[],
}));

const terminalMocks = vi.hoisted(() => ({
  closeSession: vi.fn(),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({ workspace: workspaceMocks.workspace }),
    subscribe: (listener: WorkspaceListener) => {
      workspaceMocks.listeners.push(listener);
      return () => {
        workspaceMocks.listeners = workspaceMocks.listeners.filter((candidate) => candidate !== listener);
      };
    },
  },
}));

vi.mock('../../store/terminalStore', () => ({
  useTerminalStore: {
    getState: () => terminalMocks,
  },
}));

import { useWorkspacePanelLifecycleCleanup } from './useWorkspacePanelLifecycleCleanup';

const workspaceWithTabs = (id: string, tabs: WorkspaceData['tabs']): WorkspaceData => ({
  id,
  name: id,
  tabs,
  activeTabId: tabs[0]?.id ?? null,
});

function emitWorkspaceChange(nextWorkspace: WorkspaceData | null) {
  const previousState = { workspace: workspaceMocks.workspace };
  workspaceMocks.workspace = nextWorkspace;
  const state = { workspace: workspaceMocks.workspace };
  workspaceMocks.listeners.forEach((listener) => listener(state, previousState));
}

describe('useWorkspacePanelLifecycleCleanup', () => {
  beforeEach(() => {
    workspaceMocks.workspace = null;
    workspaceMocks.listeners = [];
    terminalMocks.closeSession.mockReset();
  });

  it('fecha sessões de terminal quando abas são removidas mesmo sem painel montado', () => {
    workspaceMocks.workspace = workspaceWithTabs('workspace-1', [
      { id: 'terminal-tab', type: 'terminal', title: 'Terminal', position: 0, state: { sessionId: 'session-1' } },
      { id: 'chat-tab', type: 'chat', title: 'Chat', position: 1 },
    ]);

    renderHook(() => useWorkspacePanelLifecycleCleanup());
    emitWorkspaceChange(workspaceWithTabs('workspace-1', [
      { id: 'chat-tab', type: 'chat', title: 'Chat', position: 0 },
    ]));

    expect(terminalMocks.closeSession).toHaveBeenCalledWith('session-1');
  });

  it('não fecha sessões ao trocar de workspace ativo', () => {
    workspaceMocks.workspace = workspaceWithTabs('workspace-1', [
      { id: 'terminal-tab', type: 'terminal', title: 'Terminal', position: 0, state: { sessionId: 'session-1' } },
    ]);

    renderHook(() => useWorkspacePanelLifecycleCleanup());
    emitWorkspaceChange(workspaceWithTabs('workspace-2', []));

    expect(terminalMocks.closeSession).not.toHaveBeenCalled();
  });

  it('fecha sessões de terminal quando o workspace ativo é limpo', () => {
    workspaceMocks.workspace = workspaceWithTabs('workspace-1', [
      { id: 'terminal-tab', type: 'terminal', title: 'Terminal', position: 0, state: { sessionId: 'session-1' } },
    ]);

    renderHook(() => useWorkspacePanelLifecycleCleanup());
    emitWorkspaceChange(null);

    expect(terminalMocks.closeSession).toHaveBeenCalledWith('session-1');
  });
});
