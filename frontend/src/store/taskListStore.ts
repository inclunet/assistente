/**
 * Task List Store
 * Zustand store para gerenciar estado de TaskLists abertas, workflows e tasks
 */

import { create } from 'zustand';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  GetTaskList,
  GetAllTaskLists,
  GetTaskListsByConversation,
  CreateTaskList,
  UpdateTaskList,
  DeleteTaskList,
  CloneTaskList,
  SetTaskListViewMode,
  LinkTaskListToConversation,
  UnlinkTaskListFromConversation,
  CreateTask,
  UpdateTask,
  DeleteTask,
  UpdateTaskStatus,
  PromoteTask,
  DemoteTask,
  ReorderTasks,
} from '@wailsjs/go/main/App';
import type {
  Task,
  TaskListWithWorkflow,
  ViewMode,
  TaskListWorkflow,
  TaskListWorkflowStatus,
  WorkflowTransitions,
} from '../types/tasklist';
import type { database } from '@wailsjs/go/models';

/**
 * Mapeia uma Task do backend (snake_case do Wails/JSON) para o formato camelCase do frontend.
 */
function normalizeTask(raw: unknown): Task {
  const r = raw as Record<string, unknown>;
  return {
    id: r.id as number,
    taskListId: (r.taskListId ?? r.task_list_id) as number,
    title: (r.title ?? '') as string,
    description: (r.description ?? '') as string,
    statusId: (r.statusId ?? r.status_id) as number,
    parentId: (r.parentId ?? r.parent_id) as number | undefined,
    order: (r.order ?? 0) as number,
    dueDate: (r.dueDate ?? r.due_date) as string | undefined,
    createdAt: (r.createdAt ?? r.created_at ?? '') as string,
    updatedAt: (r.updatedAt ?? r.updated_at ?? '') as string,
    completedAt: (r.completedAt ?? r.completed_at) as string | undefined,
    subtasks: Array.isArray(r.subtasks)
      ? r.subtasks.map(normalizeTask)
      : undefined,
  };
}

/**
 * Normaliza uma TaskList vinda do backend.
 * - Mapeia snake_case → camelCase para TaskList, Workflow e Tasks
 * - Parseia `statuses` e `allowed_transitions` de strings JSON para objetos JS
 */
function normalizeTaskList(raw: TaskListWithWorkflow): TaskListWithWorkflow {
  const r = raw as unknown as Record<string, unknown>;

  // Normalize workflow
  const rawWf = (r.workflow ?? {}) as Record<string, unknown>;
  let statuses: TaskListWorkflowStatus[] | unknown = rawWf.statuses;
  let allowedTransitions: WorkflowTransitions | unknown = rawWf.allowedTransitions ?? rawWf.allowed_transitions;

  if (typeof statuses === 'string') {
    try { statuses = JSON.parse(statuses); } catch { statuses = []; }
  }
  if (!Array.isArray(statuses)) statuses = [];

  if (typeof allowedTransitions === 'string') {
    try { allowedTransitions = JSON.parse(allowedTransitions); } catch { allowedTransitions = {}; }
  }
  if (!allowedTransitions || typeof allowedTransitions !== 'object') allowedTransitions = {};

  const normalizedWorkflow: TaskListWorkflow = {
    id: rawWf.id as number,
    taskListId: (rawWf.taskListId ?? rawWf.task_list_id) as number,
    statuses: statuses as TaskListWorkflowStatus[],
    allowedTransitions: allowedTransitions as WorkflowTransitions,
    initialStatusId: (rawWf.initialStatusId ?? rawWf.initial_status_id) as number,
    createdAt: (rawWf.createdAt ?? rawWf.created_at ?? '') as string,
    updatedAt: (rawWf.updatedAt ?? rawWf.updated_at ?? '') as string,
  };

  // Normalize tasks
  const rawTasks = r.tasks;
  const normalizedTasks = Array.isArray(rawTasks) ? rawTasks.map(normalizeTask) : [];

  return {
    id: r.id as number,
    title: (r.title ?? '') as string,
    description: (r.description ?? '') as string,
    conversationId: (r.conversationId ?? r.conversation_id) as number | undefined,
    linkedMessageId: (r.linkedMessageId ?? r.linked_message_id) as number | undefined,
    preferredViewMode: ((r.preferredViewMode ?? r.preferred_view_mode) || 'list') as ViewMode,
    createdAt: (r.createdAt ?? r.created_at ?? '') as string,
    updatedAt: (r.updatedAt ?? r.updated_at ?? '') as string,
    workflow: normalizedWorkflow,
    tasks: normalizedTasks,
  };
}

/**
 * TaskListStore - Cache de conteúdo de tasklists (workspace-driven)
 * O workspace é o dono das tabs; aqui apenas gerenciamos o cache e o conteúdo ativo.
 */
interface TaskListStoreState {
  activeTaskListId?: number;
  taskLists: Map<number, TaskListWithWorkflow>;
  workflows: Map<number, TaskListWorkflow>;
  expandedTasks: Set<number>;
  isLoading: boolean;
  errors: Map<string, string>;

  // Active tasklist (driven by workspace bridge)
  setActiveTaskList: (id: number | undefined) => void;
  getActiveTaskList: () => TaskListWithWorkflow | undefined;

  // TaskList management
  loadTaskList: (taskListId: number) => Promise<TaskListWithWorkflow | null>;
  createTaskList: (title: string, description?: string, conversationId?: number) => Promise<TaskListWithWorkflow | null>;
  updateTaskList: (taskListId: number, title: string, description?: string) => Promise<void>;
  deleteTaskList: (taskListId: number) => Promise<void>;
  cloneTaskList: (taskListId: number, newTitle: string) => Promise<TaskListWithWorkflow | null>;
  linkToConversation: (taskListId: number, conversationId: number) => Promise<void>;
  unlinkFromConversation: (taskListId: number) => Promise<void>;
  fetchAllTaskLists: () => Promise<database.TaskList[]>;
  getTaskListsByConversation: (conversationId: number) => Promise<database.TaskList[]>;

  // View mode
  setViewMode: (taskListId: number, viewMode: ViewMode) => Promise<void>;

  // Workflow management
  loadWorkflow: (taskListId: number) => Promise<TaskListWorkflow | null>;
  updateWorkflow: (taskListId: number, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>) => Promise<void>;
  reorderWorkflowStatuses: (taskListId: number, statusOrder: number[]) => Promise<void>;

  // Task management
  createTask: (taskListId: number, title: string, description?: string, parentId?: number) => Promise<Task | null>;
  updateTask: (taskId: number, title: string, description?: string) => Promise<void>;
  deleteTask: (taskId: number) => Promise<void>;
  updateTaskStatus: (taskId: number, statusId: number) => Promise<void>;
  reorderTasks: (taskListId: number, statusId: number, orderedIds: number[]) => Promise<void>;
  promoteTask: (taskId: number) => Promise<void>;
  demoteTask: (taskId: number, parentId: number) => Promise<void>;
  getTaskListTasks: (taskListId: number) => Task[];

  // UI helpers
  toggleTaskExpanded: (taskId: number) => void;
  setError: (key: string, message: string) => void;
  clearError: (key: string) => void;
  clearAllErrors: () => void;
  setLoading: (isLoading: boolean) => void;

  // Cache operations
  invalidateTaskList: (taskListId: number) => void;
  cacheTaskList: (taskList: TaskListWithWorkflow) => void;
  getCachedTaskList: (taskListId: number) => TaskListWithWorkflow | undefined;
}

/**
 * Cria o store usando Zustand
 * Implementa CRUD completo + event listeners para sincronização em tempo real
 */
export const useTaskListStore = create<TaskListStoreState>((set, get) => {
  // Inicializa listeners para eventos de atualização em tempo real
  if (typeof window !== 'undefined' && (window as unknown as Record<string, unknown>).runtime) {
    // Eventos vêm do backend via Wails EventsEmit
    EventsOn('taskList:created', (taskList: TaskListWithWorkflow) => {
      const normalized = normalizeTaskList(taskList);
      set((state) => {
        const newCache = new Map(state.taskLists);
        newCache.set(normalized.id, normalized);
        return { taskLists: newCache };
      });
    });

    EventsOn('taskList:updated', (data: unknown) => {
      if (typeof data === 'number') {
        // ReorderTasks envia apenas o ID — recarrega
        get().loadTaskList(data);
      } else if (data && typeof data === 'object') {
        get().cacheTaskList(data as unknown as TaskListWithWorkflow);
      }
    });

    EventsOn('taskList:deleted', (taskListId: number) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        newCache.delete(taskListId);
        return {
          taskLists: newCache,
          activeTaskListId: state.activeTaskListId === taskListId ? undefined : state.activeTaskListId,
        };
      });
    });

    EventsOn('task:created', (rawTask: unknown) => {
      const task = normalizeTask(rawTask);
      set((state) => {
        const taskList = state.taskLists.get(task.taskListId);
        if (taskList) {
          const exists = taskList.tasks?.some((t) => t.id === task.id);
          if (!exists) {
            const newCache = new Map(state.taskLists);
            newCache.set(task.taskListId, {
              ...taskList,
              tasks: [...(taskList.tasks || []), task],
            });
            return { taskLists: newCache };
          }
        }
        return {};
      });
    });

    EventsOn('task:updated', (rawTask: unknown) => {
      const task = normalizeTask(rawTask);
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const idx = taskList.tasks?.findIndex((t) => t.id === task.id);
          if (idx !== undefined && idx >= 0) {
            const updatedTasks = [...taskList.tasks!];
            updatedTasks[idx] = task;
            newCache.set(tlId, { ...taskList, tasks: updatedTasks });
            return { taskLists: newCache };
          }
        }
        return {};
      });
    });

    EventsOn('task:deleted', (taskId: number) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [id, taskList] of newCache.entries()) {
          if (taskList.tasks?.some((t) => t.id === taskId)) {
            newCache.set(id, {
              ...taskList,
              tasks: taskList.tasks.filter((t) => t.id !== taskId),
            });
            return { taskLists: newCache };
          }
        }
        return {};
      });
    });

    EventsOn('workflow:updated', (workflow: TaskListWorkflow) => {
      set((state) => {
        const newWorkflows = new Map(state.workflows);
        newWorkflows.set(workflow.id, workflow);
        return { workflows: newWorkflows };
      });
    });
  }

  return {
    activeTaskListId: undefined,
    taskLists: new Map(),
    workflows: new Map(),
    expandedTasks: new Set(),
    isLoading: false,
    errors: new Map(),

    setActiveTaskList: (id: number | undefined) => {
      set({ activeTaskListId: id });
    },

    getActiveTaskList: () => {
      const { activeTaskListId, taskLists } = get();
      if (activeTaskListId === undefined) return undefined;
      return taskLists.get(activeTaskListId);
    },

    // TaskList management
    loadTaskList: async (taskListId: number) => {
      set({ isLoading: true });
      try {
        const taskList = await GetTaskList(taskListId);
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          set({ isLoading: false });
          return taskList as unknown as TaskListWithWorkflow;
        }
        set({ isLoading: false });
        return null;
      } catch (error) {
        get().setError('loadTaskList', String(error));
        set({ isLoading: false });
        return null;
      }
    },

    createTaskList: async (title: string, description?: string, conversationId?: number) => {
      set({ isLoading: true });
      try {
        const taskList = await CreateTaskList(title, description || '', false, conversationId);
        
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          set({ isLoading: false });
          return taskList as unknown as TaskListWithWorkflow;
        }
        
        console.error('[Store] CreateTaskList retornou null');
        set({ isLoading: false });
        return null;
      } catch (error) {
        console.error('[Store] Erro em createTaskList:', error);
        get().setError('createTaskList', String(error));
        set({ isLoading: false });
        return null;
      }
    },

    updateTaskList: async (taskListId: number, title: string, description?: string) => {
      try {
        await UpdateTaskList(taskListId, title, description || '');
        get().invalidateTaskList(taskListId);
      } catch (error) {
        get().setError('updateTaskList', String(error));
      }
    },

    deleteTaskList: async (taskListId: number) => {
      try {
        await DeleteTaskList(taskListId);
        set((state) => {
          const newCache = new Map(state.taskLists);
          newCache.delete(taskListId);
          return { taskLists: newCache };
        });
      } catch (error) {
        get().setError('deleteTaskList', String(error));
      }
    },

    cloneTaskList: async (taskListId: number, newTitle: string) => {
      try {
        const cloned = await CloneTaskList(taskListId, newTitle);
        if (cloned) {
          get().cacheTaskList(cloned as unknown as TaskListWithWorkflow);
          return cloned as unknown as TaskListWithWorkflow;
        }
        return null;
      } catch (error) {
        get().setError('cloneTaskList', String(error));
        return null;
      }
    },

    linkToConversation: async (taskListId: number, conversationId: number) => {
      try {
        await LinkTaskListToConversation(taskListId, conversationId);
        get().invalidateTaskList(taskListId);
      } catch (error) {
        get().setError('linkToConversation', String(error));
      }
    },

    unlinkFromConversation: async (taskListId: number) => {
      try {
        await UnlinkTaskListFromConversation(taskListId);
        get().invalidateTaskList(taskListId);
      } catch (error) {
        get().setError('unlinkFromConversation', String(error));
      }
    },

    fetchAllTaskLists: async () => {
      try {
        const lists = await GetAllTaskLists();
        return lists || [];
      } catch {
        return [];
      }
    },

    getTaskListsByConversation: async (conversationId: number) => {
      try {
        const lists = await GetTaskListsByConversation(conversationId);
        return lists || [];
      } catch {
        return [];
      }
    },

    // View mode
    setViewMode: async (taskListId: number, viewMode: ViewMode) => {
      try {
        await SetTaskListViewMode(taskListId, viewMode);
        set((state) => {
          const taskList = state.taskLists.get(taskListId);
          if (taskList) {
            const updated = { ...taskList, preferredViewMode: viewMode };
            const newCache = new Map(state.taskLists);
            newCache.set(taskListId, updated);
            return { taskLists: newCache };
          }
          return {};
        });
      } catch (error) {
        get().setError('setViewMode', String(error));
      }
    },

    // Workflow management
    loadWorkflow: async (_taskListId: number) => {
      try {
        // TODO: Chamar GetWorkflow via Wails
        // const workflow = await GetWorkflow(_taskListId);
        // set((state) => {
        //   const newWorkflows = new Map(state.workflows);
        //   newWorkflows.set(taskListId, workflow);
        //   return { workflows: newWorkflows };
        // });
        // return workflow;
        return null;
      } catch (error) {
        get().setError('loadWorkflow', String(error));
        return null;
      }
    },

    updateWorkflow: async (taskListId: number, _statuses: TaskListWorkflowStatus[], _transitions: Record<number, number[]>) => {
      try {
        // TODO: Chamar UpdateWorkflow via Wails
        // await UpdateWorkflow(taskListId, _statuses, _transitions);
        get().invalidateTaskList(taskListId);
      } catch (error) {
        get().setError('updateWorkflow', String(error));
      }
    },

    reorderWorkflowStatuses: async (taskListId: number, _statusOrder: number[]) => {
      try {
        // TODO: Chamar ReorderWorkflowStatuses via Wails
        // await ReorderWorkflowStatuses(taskListId, _statusOrder);
        get().invalidateTaskList(taskListId);
      } catch (error) {
        get().setError('reorderWorkflowStatuses', String(error));
      }
    },

    // Task management
    createTask: async (taskListId: number, title: string, description?: string, parentId?: number) => {
      try {
        const rawTask = await CreateTask(taskListId, title, description || '', parentId);
        if (rawTask) {
          const task = normalizeTask(rawTask);
          // Adiciona ao cache in-place (evita invalidação + reload)
          set((state) => {
            const taskList = state.taskLists.get(taskListId);
            if (taskList) {
              const exists = taskList.tasks?.some((t) => t.id === task.id);
              if (!exists) {
                const newCache = new Map(state.taskLists);
                newCache.set(taskListId, {
                  ...taskList,
                  tasks: [...(taskList.tasks || []), task],
                });
                return { taskLists: newCache };
              }
            }
            return {};
          });
          return task;
        }
        return null;
      } catch (error) {
        get().setError('createTask', String(error));
        return null;
      }
    },

    updateTask: async (taskId: number, title: string, description?: string) => {
      // Optimistic update — altera localmente antes de confirmar no backend
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const tasks = taskList.tasks;
          if (tasks) {
            const idx = tasks.findIndex((t) => t.id === taskId);
            if (idx >= 0) {
              const updatedTasks = [...tasks];
              updatedTasks[idx] = { ...updatedTasks[idx], title, description: description || '' };
              newCache.set(tlId, { ...taskList, tasks: updatedTasks });
              return { taskLists: newCache };
            }
          }
        }
        return {};
      });
      try {
        await UpdateTask(taskId, title, description || '');
      } catch (error) {
        get().setError('updateTask', String(error));
      }
    },

    deleteTask: async (taskId: number) => {
      // Optimistic delete
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [id, taskList] of newCache.entries()) {
          if (taskList.tasks?.some((t) => t.id === taskId)) {
            newCache.set(id, {
              ...taskList,
              tasks: taskList.tasks.filter((t) => t.id !== taskId),
            });
            return { taskLists: newCache };
          }
        }
        return {};
      });
      try {
        await DeleteTask(taskId);
      } catch (error) {
        get().setError('deleteTask', String(error));
      }
    },

    updateTaskStatus: async (taskId: number, statusId: number) => {
      // Optimistic update — move visualmente antes de confirmar
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const tasks = taskList.tasks;
          if (tasks) {
            const idx = tasks.findIndex((t) => t.id === taskId);
            if (idx >= 0) {
              const updatedTasks = [...tasks];
              updatedTasks[idx] = { ...updatedTasks[idx], statusId };
              newCache.set(tlId, { ...taskList, tasks: updatedTasks });
              return { taskLists: newCache };
            }
          }
        }
        return {};
      });
      try {
        await UpdateTaskStatus(taskId, statusId);
      } catch (error) {
        get().setError('updateTaskStatus', String(error));
      }
    },

    reorderTasks: async (taskListId: number, statusId: number, orderedIds: number[]) => {
      // Optimistic reorder
      set((state) => {
        const taskList = state.taskLists.get(taskListId);
        if (taskList && taskList.tasks) {
          const updatedTasks = taskList.tasks.map((t) => {
            if (t.statusId === statusId) {
              const newOrder = orderedIds.indexOf(t.id);
              return newOrder >= 0 ? { ...t, order: newOrder } : t;
            }
            return t;
          });
          const newCache = new Map(state.taskLists);
          newCache.set(taskListId, { ...taskList, tasks: updatedTasks });
          return { taskLists: newCache };
        }
        return {};
      });
      try {
        await ReorderTasks(taskListId, statusId, orderedIds);
      } catch (error) {
        get().setError('reorderTasks', String(error));
      }
    },

    promoteTask: async (taskId: number) => {
      try {
        await PromoteTask(taskId);
      } catch (error) {
        get().setError('promoteTask', String(error));
      }
    },

    demoteTask: async (taskId: number, parentId: number) => {
      try {
        await DemoteTask(taskId, parentId);
      } catch (error) {
        get().setError('demoteTask', String(error));
      }
    },

    getTaskListTasks: (taskListId: number) => {
      const taskList = get().taskLists.get(taskListId);
      return taskList?.tasks || [];
    },

    // UI helpers
    toggleTaskExpanded: (taskId: number) => {
      set((state) => {
        const expanded = new Set(state.expandedTasks);
        if (expanded.has(taskId)) {
          expanded.delete(taskId);
        } else {
          expanded.add(taskId);
        }
        return { expandedTasks: expanded };
      });
    },

    setError: (key: string, message: string) => {
      set((state) => {
        const newErrors = new Map(state.errors);
        newErrors.set(key, message);
        return { errors: newErrors };
      });
    },

    clearError: (key: string) => {
      set((state) => {
        const newErrors = new Map(state.errors);
        newErrors.delete(key);
        return { errors: newErrors };
      });
    },

    clearAllErrors: () => {
      set({ errors: new Map() });
    },

    setLoading: (isLoading: boolean) => {
      set({ isLoading });
    },

    // Cache operations
    invalidateTaskList: (taskListId: number) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        newCache.delete(taskListId);
        return { taskLists: newCache };
      });
    },

    cacheTaskList: (taskList: TaskListWithWorkflow) => {
      const normalized = normalizeTaskList(taskList);
      set((state) => {
        const newCache = new Map(state.taskLists);
        newCache.set(normalized.id, normalized);
        return { taskLists: newCache };
      });
    },

    getCachedTaskList: (taskListId: number) => {
      return get().taskLists.get(taskListId);
    },
  };
});
