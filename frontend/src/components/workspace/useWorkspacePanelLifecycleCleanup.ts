import { useEffect } from 'react';
import { useTerminalStore } from '../../store/terminalStore';
import { useWorkspaceStore, type WorkspaceData, type WorkspaceTab } from '../../store/workspaceStore';

function getTerminalSessionIds(tabs: WorkspaceTab[]): Set<string> {
  return new Set(tabs
    .filter((tab) => tab.type === 'terminal')
    .map((tab) => tab.state?.sessionId)
    .filter((sessionId): sessionId is string => typeof sessionId === 'string' && sessionId.length > 0));
}

function closeRemovedTerminalSessions(previous: WorkspaceData | null, current: WorkspaceData | null) {
  if (!previous || (current && previous.id !== current.id)) return;

  const currentSessionIds = current ? getTerminalSessionIds(current.tabs) : new Set<string>();
  for (const sessionId of getTerminalSessionIds(previous.tabs)) {
    if (!currentSessionIds.has(sessionId)) {
      void useTerminalStore.getState().closeSession(sessionId);
    }
  }
}

export function useWorkspacePanelLifecycleCleanup() {
  useEffect(() => {
    return useWorkspaceStore.subscribe((state, previousState) => {
      closeRemovedTerminalSessions(previousState.workspace, state.workspace);
    });
  }, []);
}
