import { useEffect, useRef } from 'react';
import i18next from 'i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';

export function useTaskListSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const updateWorkspaceTab = useWorkspaceStore((state) => state.updateTab);
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);
  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  const tabId = tab.id;
  const tabType = tab.type;
  const taskListId = (tab.state?.tasklistId as string) || '';

  useEffect(() => {
    if (!isWsInitialized || !isActive || tabType !== 'tasklist') return;

    if (taskListId) {
      void syncExistingTaskList(taskListId);
      return;
    }

    if (!creatingRef.current) {
      void createTaskListForTab(tabId);
    }
  }, [isActive, isWsInitialized, tabId, tabType, taskListId]);

  useEffect(() => {
    const syncTitle = (state: ReturnType<typeof useTaskListStore.getState>) => {
      const ws = useWorkspaceStore.getState();
      const wsTab = ws.workspace?.tabs.find((candidate) => candidate.id === tabId);
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
  }, [tabId]);

  async function syncExistingTaskList(id: string) {
    try {
      const store = useTaskListStore.getState();
      if (store.activeTaskListId !== id) {
        store.setActiveTaskList(id);
      }

      if (lastSyncedRef.current === `${tabId}:${id}`) return;

      if (!store.taskLists.has(id)) {
        await store.loadTaskList(id);
      }
      lastSyncedRef.current = `${tabId}:${id}`;
    } catch (error) {
      console.error('[TaskListSurfaceController] Erro ao sincronizar tasklist:', error);
    }
  }

  async function createTaskListForTab(tabId: string) {
    creatingRef.current = true;
    try {
      const taskList = await useTaskListStore.getState().createTaskList(i18next.t('tasklist.newList'));
      if (taskList) {
        const workspaceTab = useWorkspaceStore.getState().workspace?.tabs.find((candidate) => candidate.id === tabId);
        await updateWorkspaceTab(tabId, {
          state: { ...(workspaceTab?.state ?? {}), tasklistId: String(taskList.id) },
          title: taskList.title,
        });
        lastSyncedRef.current = `${tabId}:${taskList.id}`;
      }
    } catch (error) {
      console.error('[TaskListSurfaceController] Erro ao criar tasklist:', error);
    } finally {
      creatingRef.current = false;
    }
  }
}
