/**
 * Task List Store
 * Zustand store para gerenciar estado de TaskLists abertas, workflows e tasks
 */

import { create } from 'zustand';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  GetTaskList,
  GetAllTaskLists,
  CreateTaskList,
  UpdateTaskList,
  DeleteTaskList,
  ClearTaskList,
  CloneTaskList,
  SetTaskListViewMode,
  CreateTask,
  UpdateTask,
  UpdateTaskFull,
  UpdateTaskAssignee,
  DeleteTask,
  UpdateTaskStatus,
  PromoteTask,
  DemoteTask,
  ReorderTasks,
  ReorderWorkflowStatuses,
  UpdateWorkflowFull,
  GetTaskCountsByStatus,
  CreateTaskNote,
  GetTaskNotes,
  UpdateTaskNote,
  DeleteTaskNote,
} from '@wailsjs/go/main/App';
import type {
  Task,
  TaskNote,
  TaskNoteType,
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
    code: (r.code ?? '') as string || undefined,
    link: (r.link ?? '') as string || undefined,
    statusId: (r.statusId ?? r.status_id) as number,
    parentId: (r.parentId ?? r.parent_id) as number | undefined,
    order: (r.order ?? 0) as number,
    assigneeName: (r.assigneeName ?? r.assignee_name ?? '') as string || undefined,
    assigneeId: (r.assigneeId ?? r.assignee_id ?? '') as string || undefined,
    creatorName: (r.creatorName ?? r.creator_name ?? '') as string || undefined,
    creatorId: (r.creatorId ?? r.creator_id ?? '') as string || undefined,
    dueDate: (r.dueDate ?? r.due_date) as string | undefined,
    createdAt: (r.createdAt ?? r.created_at ?? '') as string,
    updatedAt: (r.updatedAt ?? r.updated_at ?? '') as string,
    completedAt: (r.completedAt ?? r.completed_at) as string | undefined,
    subtasks: Array.isArray(r.subtasks)
      ? r.subtasks.map(normalizeTask)
      : undefined,
  };
}

function normalizeTaskNote(raw: unknown): TaskNote {
  const r = raw as Record<string, unknown>;
  return {
    id: r.id as number,
    taskId: (r.taskId ?? r.task_id) as number,
    type: (r.type ?? 1) as TaskNoteType,
    content: (r.content ?? '') as string,
    authorName: (r.authorName ?? r.author_name ?? '') as string || undefined,
    authorId: (r.authorId ?? r.author_id ?? '') as string || undefined,
    createdAt: (r.createdAt ?? r.created_at ?? '') as string,
    updatedAt: (r.updatedAt ?? r.updated_at ?? '') as string,
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
  createTaskList: (title: string, description?: string) => Promise<TaskListWithWorkflow | null>;
  updateTaskList: (taskListId: number, title: string, description?: string) => Promise<void>;
  deleteTaskList: (taskListId: number) => Promise<void>;
  clearTaskList: (taskListId: number) => Promise<void>;
  cloneTaskList: (taskListId: number, newTitle: string) => Promise<TaskListWithWorkflow | null>;
  fetchAllTaskLists: () => Promise<database.TaskList[]>;

  // View mode
  setViewMode: (taskListId: number, viewMode: ViewMode) => Promise<void>;

  // Workflow management
  loadWorkflow: (taskListId: number) => Promise<TaskListWorkflow | null>;
  updateWorkflow: (taskListId: number, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>) => Promise<void>;
  updateWorkflowFull: (taskListId: number, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>, initialStatusId: number, statusMigration?: Record<number, number>) => Promise<void>;
  getTaskCountsByStatus: (taskListId: number) => Promise<Record<number, number>>;
  reorderWorkflowStatuses: (taskListId: number, statusOrder: number[]) => Promise<void>;

  // Task management
  createTask: (taskListId: number, title: string, description?: string, code?: string, link?: string, parentId?: number) => Promise<Task | null>;
  updateTask: (taskId: number, title: string, description?: string, code?: string, link?: string) => Promise<void>;
  updateTaskFull: (taskId: number, title: string, description?: string, code?: string, link?: string, assigneeName?: string, assigneeId?: string, creatorName?: string, creatorId?: string) => Promise<void>;
  updateTaskAssignee: (taskId: number, assigneeName: string, assigneeId?: string) => Promise<void>;
  deleteTask: (taskId: number) => Promise<void>;
  updateTaskStatus: (taskId: number, statusId: number) => Promise<void>;
  reorderTasks: (taskListId: number, statusId: number, orderedIds: number[]) => Promise<void>;
  promoteTask: (taskId: number) => Promise<void>;
  demoteTask: (taskId: number, parentId: number) => Promise<void>;
  getTaskListTasks: (taskListId: number) => Task[];

  // TaskNote management
  loadTaskNotes: (taskId: number) => Promise<TaskNote[]>;
  createTaskNote: (taskId: number, type: TaskNoteType, content: string, authorName?: string, authorId?: string) => Promise<TaskNote | null>;
  updateTaskNote: (noteId: number, content: string) => Promise<void>;
  deleteTaskNote: (noteId: number) => Promise<void>;

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

    createTaskList: async (title: string, description?: string) => {
      set({ isLoading: true });
      try {
        const taskList = await CreateTaskList(title, description || '');
        
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          set({ isLoading: false });
          return taskList as unknown as TaskListWithWorkflow;
        }
        
        set({ isLoading: false });
        return null;
      } catch (error) {
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

    clearTaskList: async (taskListId: number) => {
      try {
        await ClearTaskList(taskListId);
        set((state) => {
          const newCache = new Map(state.taskLists);
          const existing = newCache.get(taskListId);
          if (existing) {
            newCache.set(taskListId, { ...existing, tasks: [] });
          }
          return { taskLists: newCache };
        });
      } catch (error) {
        get().setError('clearTaskList', String(error));
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

    fetchAllTaskLists: async () => {
      try {
        const lists = await GetAllTaskLists();
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
    loadWorkflow: async (taskListId: number) => {
      try {
        const taskList = await GetTaskList(taskListId);
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          const cached = get().taskLists.get(taskListId);
          return cached?.workflow ?? null;
        }
        return null;
      } catch (error) {
        get().setError('loadWorkflow', String(error));
        return null;
      }
    },

    updateWorkflow: async (taskListId: number, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>) => {
      try {
        const currentWorkflow = get().taskLists.get(taskListId)?.workflow;
        const initialStatusId = currentWorkflow?.initialStatusId ?? statuses[0]?.id ?? 1;
        await UpdateWorkflowFull(taskListId, statuses as any, transitions, initialStatusId, {});
        await get().loadTaskList(taskListId);
      } catch (error) {
        get().setError('updateWorkflow', String(error));
      }
    },

    updateWorkflowFull: async (taskListId: number, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>, initialStatusId: number, statusMigration?: Record<number, number>) => {
      try {
        await UpdateWorkflowFull(taskListId, statuses as any, transitions, initialStatusId, statusMigration ?? {});
        await get().loadTaskList(taskListId);
      } catch (error) {
        get().setError('updateWorkflowFull', String(error));
        throw error;
      }
    },

    getTaskCountsByStatus: async (taskListId: number) => {
      try {
        const counts = await GetTaskCountsByStatus(taskListId);
        return counts ?? {};
      } catch (error) {
        get().setError('getTaskCountsByStatus', String(error));
        return {};
      }
    },

    reorderWorkflowStatuses: async (taskListId: number, statusOrder: number[]) => {
      try {
        await ReorderWorkflowStatuses(taskListId, statusOrder);
        await get().loadTaskList(taskListId);
      } catch (error) {
        get().setError('reorderWorkflowStatuses', String(error));
      }
    },

    // Task management
    createTask: async (taskListId: number, title: string, description?: string, code?: string, link?: string, parentId?: number) => {
      try {
        const rawTask = await CreateTask(taskListId, title, description || '', code || '', link || '', parentId);
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

    updateTask: async (taskId: number, title: string, description?: string, code?: string, link?: string) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const tasks = taskList.tasks;
          if (tasks) {
            const idx = tasks.findIndex((t) => t.id === taskId);
            if (idx >= 0) {
              const updatedTasks = [...tasks];
              updatedTasks[idx] = { ...updatedTasks[idx], title, description: description || '', code: code || undefined, link: link || undefined };
              newCache.set(tlId, { ...taskList, tasks: updatedTasks });
              return { taskLists: newCache };
            }
          }
        }
        return {};
      });
      try {
        await UpdateTask(taskId, title, description || '', code || '', link || '');
      } catch (error) {
        get().setError('updateTask', String(error));
      }
    },

    updateTaskFull: async (taskId: number, title: string, description?: string, code?: string, link?: string, assigneeName?: string, assigneeId?: string, creatorName?: string, creatorId?: string) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const tasks = taskList.tasks;
          if (tasks) {
            const idx = tasks.findIndex((t) => t.id === taskId);
            if (idx >= 0) {
              const updatedTasks = [...tasks];
              updatedTasks[idx] = {
                ...updatedTasks[idx],
                title,
                description: description || '',
                code: code || undefined,
                link: link || undefined,
                assigneeName: assigneeName || undefined,
                assigneeId: assigneeId || undefined,
                creatorName: creatorName || undefined,
                creatorId: creatorId || undefined,
              };
              newCache.set(tlId, { ...taskList, tasks: updatedTasks });
              return { taskLists: newCache };
            }
          }
        }
        return {};
      });
      try {
        await UpdateTaskFull(taskId, title, description || '', code || '', link || '', assigneeName || '', assigneeId || '', creatorName || '', creatorId || '');
      } catch (error) {
        get().setError('updateTaskFull', String(error));
      }
    },

    updateTaskAssignee: async (taskId: number, assigneeName: string, assigneeId?: string) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const tasks = taskList.tasks;
          if (tasks) {
            const idx = tasks.findIndex((t) => t.id === taskId);
            if (idx >= 0) {
              const updatedTasks = [...tasks];
              updatedTasks[idx] = {
                ...updatedTasks[idx],
                assigneeName: assigneeName || undefined,
                assigneeId: assigneeId || undefined,
              };
              newCache.set(tlId, { ...taskList, tasks: updatedTasks });
              return { taskLists: newCache };
            }
          }
        }
        return {};
      });
      try {
        await UpdateTaskAssignee(taskId, assigneeName, assigneeId || '');
      } catch (error) {
        get().setError('updateTaskAssignee', String(error));
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

    // TaskNote management
    loadTaskNotes: async (taskId: number) => {
      try {
        const rawNotes = await GetTaskNotes(taskId);
        return (rawNotes || []).map(normalizeTaskNote);
      } catch (error) {
        get().setError('loadTaskNotes', String(error));
        return [];
      }
    },

    createTaskNote: async (taskId: number, type: TaskNoteType, content: string, authorName?: string, authorId?: string) => {
      try {
        const rawNote = await CreateTaskNote(taskId, type, content, authorName || '', authorId || '');
        if (rawNote) {
          return normalizeTaskNote(rawNote);
        }
        return null;
      } catch (error) {
        get().setError('createTaskNote', String(error));
        return null;
      }
    },

    updateTaskNote: async (noteId: number, content: string) => {
      try {
        await UpdateTaskNote(noteId, content);
      } catch (error) {
        get().setError('updateTaskNote', String(error));
      }
    },

    deleteTaskNote: async (noteId: number) => {
      try {
        await DeleteTaskNote(noteId);
      } catch (error) {
        get().setError('deleteTaskNote', String(error));
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
