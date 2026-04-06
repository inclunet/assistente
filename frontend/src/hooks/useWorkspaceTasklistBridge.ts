import { useEffect, useRef } from 'react';
import { useWorkspaceStore, type WorkspaceTab, registerTabRenameHandler } from '../store/workspaceStore';
import { useTaskListStore } from '../store/taskListStore';

/**
 * Sincroniza abas de tasklist do workspace com o taskListStore (cache de conteúdo).
 *
 * Fluxo:
 * 1. Workspace ativa uma aba do tipo tasklist
 * 2. Se state.tasklistId vazio → cria tasklist via taskListStore.createTaskList()
 *    e salva o taskListId em state.tasklistId da aba do workspace
 * 3. Se state.tasklistId existente → carrega no store via loadTaskList + setActiveTaskList
 * 4. Título da tasklist no store é sincronizado de volta ao workspace
 */
export function useWorkspaceTasklistBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  const tasklistId = (activeTab?.state?.tasklistId as string) || '';

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab || activeTab.type !== 'tasklist') return;

    const syncKey = `${activeTab.id}:${tasklistId}`;
    if (lastSyncedRef.current === syncKey) return;

    const taskListId = tasklistId ? parseInt(tasklistId, 10) : 0;
    if (taskListId > 0) {
      syncExistingTaskList(taskListId);
      lastSyncedRef.current = syncKey;
    } else if (!creatingRef.current) {
      createTaskListForTab(activeTab);
    }
  }, [activeTab?.id, activeTab?.type, tasklistId, isWsInitialized]);

  async function syncExistingTaskList(id: number) {
    const store = useTaskListStore.getState();
    store.setActiveTaskList(id);

    if (!store.taskLists.has(id)) {
      await store.loadTaskList(id);
    }
  }

  async function createTaskListForTab(wsTab: WorkspaceTab) {
    creatingRef.current = true;
    try {
      const taskList = await useTaskListStore.getState().createTaskList('Nova lista');
      if (taskList) {
        await updateWsTab(wsTab.id, {
          state: { tasklistId: String(taskList.id) },
          title: taskList.title,
        });
        lastSyncedRef.current = `${wsTab.id}:${taskList.id}`;
      }
    } catch (error) {
      console.error('[WorkspaceTasklistBridge] Erro ao criar tasklist:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  // Sync título do taskListStore → workspace tab
  useEffect(() => {
    const unsub = useTaskListStore.subscribe((state) => {
      const ws = useWorkspaceStore.getState();
      const wsTabs = ws.workspace?.tabs || [];
      for (const wsTab of wsTabs) {
        if (wsTab.type !== 'tasklist') continue;
        const tlIdStr = wsTab.state?.tasklistId as string | undefined;
        if (!tlIdStr) continue;
        const tlId = parseInt(tlIdStr, 10);
        const tl = state.taskLists.get(tlId);
        if (tl && tl.title !== wsTab.title) {
          void ws.updateTab(wsTab.id, { title: tl.title });
        }
      }
    });
    return unsub;
  }, []);

  // F2 tab rename → rename tasklist in backend
  useEffect(() => {
    return registerTabRenameHandler('tasklist', (id, newTitle) => {
      const tlId = parseInt(id, 10);
      if (tlId) void useTaskListStore.getState().updateTaskList(tlId, newTitle);
    });
  }, []);

  // Limpa activeTaskListId quando não há mais abas de tasklist
  useEffect(() => {
    const unsub = useWorkspaceStore.subscribe((state) => {
      const wsTabs = state.workspace?.tabs || [];
      const hasTasklistTab = wsTabs.some((t) => t.type === 'tasklist');
      if (!hasTasklistTab) {
        const tlStore = useTaskListStore.getState();
        if (tlStore.activeTaskListId !== undefined) {
          tlStore.setActiveTaskList(undefined);
        }
      }
    });
    return unsub;
  }, []);
}
