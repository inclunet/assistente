import { useEffect } from 'react';
import { useTaskListStore } from '../../store/taskListStore';
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

function clearActiveTaskListWhenNoTabs(workspace: WorkspaceData | null) {
  const hasTaskListTab = workspace?.tabs.some((tab) => tab.type === 'tasklist') ?? false;
  if (hasTaskListTab) return;

  const taskListStore = useTaskListStore.getState();
  if (taskListStore.activeTaskListId !== undefined) {
    taskListStore.setActiveTaskList(undefined);
  }
}

export function useWorkspacePanelLifecycleCleanup() {
  useEffect(() => {
    clearActiveTaskListWhenNoTabs(useWorkspaceStore.getState().workspace);

    return useWorkspaceStore.subscribe((state, previousState) => {
      closeRemovedTerminalSessions(previousState.workspace, state.workspace);
      clearActiveTaskListWhenNoTabs(state.workspace);
    });
  }, []);
}
