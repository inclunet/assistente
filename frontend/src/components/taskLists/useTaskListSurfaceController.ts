import { useEffect, useRef } from 'react';
import i18next from 'i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';

export function useTaskListSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const updateWorkspaceTab = useWorkspaceStore((state) => state.updateTab);
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);
  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  const taskListId = (tab.state?.tasklistId as string) || '';

  useEffect(() => {
    if (!isWsInitialized || !isActive || tab.type !== 'tasklist') return;

    const syncKey = `${tab.id}:${taskListId}`;
    if (lastSyncedRef.current === syncKey) return;

    if (taskListId) {
      void syncExistingTaskList(taskListId);
      lastSyncedRef.current = syncKey;
      return;
    }

    if (!creatingRef.current) {
      void createTaskListForTab(tab);
    }
  }, [isActive, isWsInitialized, tab, tab.id, tab.type, taskListId]);

  useEffect(() => {
    const syncTitle = (state: ReturnType<typeof useTaskListStore.getState>) => {
      const ws = useWorkspaceStore.getState();
      const wsTab = ws.workspace?.tabs.find((candidate) => candidate.id === tab.id);
      if (!wsTab || wsTab.type !== 'tasklist') return;

      const id = wsTab.state?.tasklistId as string | undefined;
      if (!id) return;

      const taskList = state.taskLists.get(id);
      if (taskList && taskList.title !== wsTab.title) {
        void ws.updateTab(wsTab.id, { title: taskList.title });
      }
    };

    syncTitle(useTaskListStore.getState());
    const unsub = useTaskListStore.subscribe(syncTitle);
    return unsub;
  }, [tab.id]);

  useEffect(() => () => {
    const wsTabs = useWorkspaceStore.getState().workspace?.tabs || [];
    const tabStillOpen = wsTabs.some((candidate) => candidate.id === tab.id);
    if (tabStillOpen) return;

    const hasTaskListTab = wsTabs.some((candidate) => candidate.type === 'tasklist');
    if (!hasTaskListTab) {
      const taskListStore = useTaskListStore.getState();
      if (taskListStore.activeTaskListId !== undefined) {
        taskListStore.setActiveTaskList(undefined);
      }
    }
  }, [tab.id]);

  async function syncExistingTaskList(id: string) {
    const store = useTaskListStore.getState();
    store.setActiveTaskList(id);

    if (!store.taskLists.has(id)) {
      await store.loadTaskList(id);
    }
  }

  async function createTaskListForTab(workspaceTab: WorkspaceTab) {
    creatingRef.current = true;
    try {
      const taskList = await useTaskListStore.getState().createTaskList(i18next.t('tasklist.newList'));
      if (taskList) {
        await updateWorkspaceTab(workspaceTab.id, {
          state: { ...(workspaceTab.state ?? {}), tasklistId: String(taskList.id) },
          title: taskList.title,
        });
        lastSyncedRef.current = `${workspaceTab.id}:${taskList.id}`;
      }
    } catch (error) {
      console.error('[TaskListSurfaceController] Erro ao criar tasklist:', error);
    } finally {
      creatingRef.current = false;
    }
  }
}
