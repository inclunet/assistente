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
  SetTaskConversation,
  SetTaskListConversation,
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
  GetTaskListCustomActions,
  SetTaskListCustomActions,
  ListCardCustomActions,
  ListBoardCustomActions,
  TriggerCustomAction,
} from '@wailsjs/go/app/App';
import type {
  Task,
  TaskNote,
  TaskNoteType,
  TaskListWithWorkflow,
  TaskListValidationPolicy,
  ViewMode,
  TaskListWorkflow,
  TaskListWorkflowStatus,
  WorkflowTransitions,
  TaskListCustomActions,
  CustomActionView,
  CustomActionSurface,
} from '../types/tasklist';
import type { database } from '@wailsjs/go/models';

/**
 * Mapeia uma Task do backend (snake_case do Wails/JSON) para o formato camelCase do frontend.
 */
function normalizeTask(raw: unknown): Task {
  const r = raw as Record<string, unknown>;
  return {
    id: r.id as string,
    taskListId: (r.taskListId ?? r.task_list_id) as string,
    title: (r.title ?? '') as string,
    description: (r.description ?? '') as string,
    code: (r.code ?? '') as string || undefined,
    link: (r.link ?? '') as string || undefined,
    statusId: (r.statusId ?? r.status_id) as number,
    parentId: (r.parentId ?? r.parent_id) as string | undefined,
    order: (r.order ?? 0) as number,
    assigneeName: (r.assigneeName ?? r.assignee_name ?? '') as string || undefined,
    assigneeId: (r.assigneeId ?? r.assignee_id ?? '') as string || undefined,
    creatorName: (r.creatorName ?? r.creator_name ?? '') as string || undefined,
    creatorId: (r.creatorId ?? r.creator_id ?? '') as string || undefined,
    dueDate: (r.dueDate ?? r.due_date) as string | undefined,
    createdAt: (r.createdAt ?? r.created_at ?? '') as string,
    updatedAt: (r.updatedAt ?? r.updated_at ?? '') as string,
    completedAt: (r.completedAt ?? r.completed_at) as string | undefined,
    conversationId: (r.conversationId ?? r.conversation_id ?? '') as string || undefined,
    subtasks: Array.isArray(r.subtasks)
      ? r.subtasks.map(normalizeTask)
      : undefined,
  };
}

function normalizeTaskNote(raw: unknown): TaskNote {
  const r = raw as Record<string, unknown>;
  const extUpd = r.externalUpdatedAt ?? r.external_updated_at;
  return {
    id: r.id as string,
    taskId: (r.taskId ?? r.task_id) as string,
    type: (r.type ?? 1) as TaskNoteType,
    content: (r.content ?? '') as string,
    authorName: (r.authorName ?? r.author_name ?? '') as string || undefined,
    authorId: (r.authorId ?? r.author_id ?? '') as string || undefined,
    source: (r.source ?? r.external_source ?? '') as string || undefined,
    externalId: (r.externalId ?? r.external_id ?? '') as string || undefined,
    externalParentId: (r.externalParentId ?? r.external_parent_id ?? '') as string || undefined,
    externalUpdatedAt: typeof extUpd === 'string' ? extUpd : extUpd != null ? String(extUpd) : undefined,
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
    id: rawWf.id as string,
    taskListId: (rawWf.taskListId ?? rawWf.task_list_id) as string,
    statuses: statuses as TaskListWorkflowStatus[],
    allowedTransitions: allowedTransitions as WorkflowTransitions,
    initialStatusId: (rawWf.initialStatusId ?? rawWf.initial_status_id) as number,
    createdAt: (rawWf.createdAt ?? rawWf.created_at ?? '') as string,
    updatedAt: (rawWf.updatedAt ?? rawWf.updated_at ?? '') as string,
  };

  // Normalize tasks
  const rawTasks = r.tasks;
  const normalizedTasks = Array.isArray(rawTasks) ? rawTasks.map(normalizeTask) : [];

  let validationPolicy: TaskListValidationPolicy | undefined;
  const vpRaw = r.validationPolicy ?? r.validation_policy;
  if (typeof vpRaw === 'string' && vpRaw.trim()) {
    try {
      validationPolicy = JSON.parse(vpRaw) as TaskListValidationPolicy;
    } catch {
      validationPolicy = undefined;
    }
  } else if (vpRaw && typeof vpRaw === 'object') {
    validationPolicy = vpRaw as TaskListValidationPolicy;
  }

  return {
    id: r.id as string,
    title: (r.title ?? '') as string,
    slug: (() => {
      const s = r.slug ?? r.Slug;
      if (s == null || s === '') return undefined;
      return String(s);
    })(),
    description: (r.description ?? '') as string,
    preferredViewMode: ((r.preferredViewMode ?? r.preferred_view_mode) || 'list') as ViewMode,
    createdAt: (r.createdAt ?? r.created_at ?? '') as string,
    updatedAt: (r.updatedAt ?? r.updated_at ?? '') as string,
    validationPolicy,
    conversationId: (r.conversationId ?? r.conversation_id ?? '') as string || undefined,
    workflow: normalizedWorkflow,
    tasks: normalizedTasks,
  };
}

function taskListErrorKey(operation: string, taskListId: string): string {
  return `${operation}:${taskListId}`;
}

/**
 * TaskListStore - Cache de conteúdo de tasklists (workspace-driven)
 * O workspace é o dono das tabs; aqui apenas gerenciamos cache e operações por ID explícito.
 */
interface TaskListStoreState {
  taskLists: Map<string, TaskListWithWorkflow>;
  workflows: Map<string, TaskListWorkflow>;
  expandedTasks: Set<string>;
  loadingByTaskListId: Map<string, boolean>;
  errors: Map<string, string>;

  // TaskList management
  loadTaskList: (taskListId: string) => Promise<TaskListWithWorkflow | null>;
  createTaskList: (title: string, description?: string) => Promise<TaskListWithWorkflow | null>;
  updateTaskList: (taskListId: string, title: string, description?: string) => Promise<void>;
  deleteTaskList: (taskListId: string) => Promise<void>;
  clearTaskList: (taskListId: string) => Promise<void>;
  cloneTaskList: (taskListId: string, newTitle: string) => Promise<TaskListWithWorkflow | null>;
  fetchAllTaskLists: () => Promise<database.TaskList[]>;

  // Custom actions (AEP-0067)
  getTaskListCustomActions: (taskListId: string) => Promise<TaskListCustomActions>;
  setTaskListCustomActions: (taskListId: string, actionsJSON: string) => Promise<void>;
  listCardCustomActions: (taskId: string, surface: CustomActionSurface) => Promise<CustomActionView[]>;
  listBoardCustomActions: (taskListId: string) => Promise<CustomActionView[]>;
  triggerCustomAction: (taskListId: string, taskId: string, actionId: string) => Promise<string>;

  // View mode
  setViewMode: (taskListId: string, viewMode: ViewMode) => Promise<void>;

  // Workflow management
  loadWorkflow: (taskListId: string) => Promise<TaskListWorkflow | null>;
  updateWorkflow: (taskListId: string, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>) => Promise<void>;
  updateWorkflowFull: (taskListId: string, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>, initialStatusId: number, statusMigration?: Record<number, number>) => Promise<void>;
  getTaskCountsByStatus: (taskListId: string) => Promise<Record<number, number>>;
  reorderWorkflowStatuses: (taskListId: string, statusOrder: number[]) => Promise<void>;

  // Task management
  createTask: (taskListId: string, title: string, description?: string, code?: string, link?: string, parentId?: string) => Promise<Task | null>;
  updateTask: (taskId: string, title: string, description?: string, code?: string, link?: string) => Promise<void>;
  updateTaskFull: (taskId: string, title: string, description?: string, code?: string, link?: string, assigneeName?: string, assigneeId?: string, creatorName?: string, creatorId?: string) => Promise<void>;
  setTaskConversation: (taskId: string, conversationId: string | null) => Promise<void>;
  setTaskListConversation: (taskListId: string, conversationId: string | null) => Promise<void>;
  updateTaskAssignee: (taskId: string, assigneeName: string, assigneeId?: string) => Promise<void>;
  deleteTask: (taskId: string) => Promise<void>;
  updateTaskStatus: (taskId: string, statusId: number) => Promise<void>;
  reorderTasks: (taskListId: string, statusId: number, orderedIds: string[]) => Promise<void>;
  promoteTask: (taskId: string) => Promise<void>;
  demoteTask: (taskId: string, parentId: string) => Promise<void>;
  getTaskListTasks: (taskListId: string) => Task[];

  // TaskNote management
  loadTaskNotes: (taskId: string) => Promise<TaskNote[]>;
  createTaskNote: (taskId: string, type: TaskNoteType, content: string, authorName?: string, authorId?: string) => Promise<TaskNote | null>;
  updateTaskNote: (noteId: string, content: string) => Promise<void>;
  deleteTaskNote: (noteId: string) => Promise<void>;

  // UI helpers
  toggleTaskExpanded: (taskId: string) => void;
  setError: (key: string, message: string) => void;
  clearError: (key: string) => void;
  clearAllErrors: () => void;
  setTaskListLoading: (taskListId: string, isLoading: boolean) => void;

  // Cache operations
  invalidateTaskList: (taskListId: string) => void;
  cacheTaskList: (taskList: TaskListWithWorkflow) => void;
  getCachedTaskList: (taskListId: string) => TaskListWithWorkflow | undefined;
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
      if (typeof data === 'string') {
        // ReorderTasks envia apenas o ID — recarrega
        get().loadTaskList(data);
      } else if (data && typeof data === 'object') {
        get().cacheTaskList(data as unknown as TaskListWithWorkflow);
      }
    });

    EventsOn('taskList:deleted', (taskListId: string) => {
      set((state) => {
        const newCache = new Map(state.taskLists);
        newCache.delete(taskListId);
        return { taskLists: newCache };
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

    EventsOn('task:deleted', (taskId: string) => {
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
    taskLists: new Map(),
    workflows: new Map(),
    expandedTasks: new Set(),
    loadingByTaskListId: new Map(),
    errors: new Map(),

    // TaskList management
    loadTaskList: async (taskListId: string) => {
      get().setTaskListLoading(taskListId, true);
      try {
        const taskList = await GetTaskList(taskListId);
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          get().setTaskListLoading(taskListId, false);
          return taskList as unknown as TaskListWithWorkflow;
        }
        get().setTaskListLoading(taskListId, false);
        return null;
      } catch (error) {
        get().setError(taskListErrorKey('loadTaskList', taskListId), String(error));
        get().setTaskListLoading(taskListId, false);
        return null;
      }
    },

    createTaskList: async (title: string, description?: string) => {
      try {
        const taskList = await CreateTaskList(title, description || '', '');
        
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          return taskList as unknown as TaskListWithWorkflow;
        }
        
        return null;
      } catch (error) {
        get().setError('createTaskList', String(error));
        return null;
      }
    },

    updateTaskList: async (taskListId: string, title: string, description?: string) => {
      try {
        await UpdateTaskList(taskListId, title, description || '');
        get().invalidateTaskList(taskListId);
      } catch (error) {
        get().setError(taskListErrorKey('updateTaskList', taskListId), String(error));
      }
    },

    deleteTaskList: async (taskListId: string) => {
      try {
        await DeleteTaskList(taskListId);
        set((state) => {
          const newCache = new Map(state.taskLists);
          newCache.delete(taskListId);
          return { taskLists: newCache };
        });
      } catch (error) {
        get().setError(taskListErrorKey('deleteTaskList', taskListId), String(error));
      }
    },

    clearTaskList: async (taskListId: string) => {
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
        get().setError(taskListErrorKey('clearTaskList', taskListId), String(error));
      }
    },

    cloneTaskList: async (taskListId: string, newTitle: string) => {
      try {
        const cloned = await CloneTaskList(taskListId, newTitle);
        if (cloned) {
          get().cacheTaskList(cloned as unknown as TaskListWithWorkflow);
          return cloned as unknown as TaskListWithWorkflow;
        }
        return null;
      } catch (error) {
        get().setError(taskListErrorKey('cloneTaskList', taskListId), String(error));
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

    // ── Custom actions (AEP-0067) ──────────────────────────────────────────
    getTaskListCustomActions: async (taskListId: string) => {
      const res = await GetTaskListCustomActions(taskListId);
      return (res as unknown as TaskListCustomActions) || { actions: [] };
    },

    setTaskListCustomActions: async (taskListId: string, actionsJSON: string) => {
      await SetTaskListCustomActions(taskListId, actionsJSON);
    },

    listCardCustomActions: async (taskId: string, surface: CustomActionSurface) => {
      const res = await ListCardCustomActions(taskId, surface);
      return (res as unknown as CustomActionView[]) || [];
    },

    listBoardCustomActions: async (taskListId: string) => {
      const res = await ListBoardCustomActions(taskListId);
      return (res as unknown as CustomActionView[]) || [];
    },

    triggerCustomAction: async (taskListId: string, taskId: string, actionId: string) => {
      return (await TriggerCustomAction(taskListId, taskId, actionId)) || '';
    },

    // View mode
    setViewMode: async (taskListId: string, viewMode: ViewMode) => {
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
        get().setError(taskListErrorKey('setViewMode', taskListId), String(error));
      }
    },

    // Workflow management
    loadWorkflow: async (taskListId: string) => {
      try {
        const taskList = await GetTaskList(taskListId);
        if (taskList) {
          get().cacheTaskList(taskList as unknown as TaskListWithWorkflow);
          const cached = get().taskLists.get(taskListId);
          return cached?.workflow ?? null;
        }
        return null;
      } catch (error) {
        get().setError(taskListErrorKey('loadWorkflow', taskListId), String(error));
        return null;
      }
    },

    updateWorkflow: async (taskListId: string, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>) => {
      try {
        const currentWorkflow = get().taskLists.get(taskListId)?.workflow;
        const initialStatusId = currentWorkflow?.initialStatusId ?? statuses[0]?.id ?? 1;
        await UpdateWorkflowFull(taskListId, statuses as TaskListWorkflowStatus[], transitions, initialStatusId, {});
        await get().loadTaskList(taskListId);
      } catch (error) {
        get().setError(taskListErrorKey('updateWorkflow', taskListId), String(error));
      }
    },

    updateWorkflowFull: async (taskListId: string, statuses: TaskListWorkflowStatus[], transitions: Record<number, number[]>, initialStatusId: number, statusMigration?: Record<number, number>) => {
      try {
        await UpdateWorkflowFull(taskListId, statuses as TaskListWorkflowStatus[], transitions, initialStatusId, statusMigration ?? {});
        await get().loadTaskList(taskListId);
      } catch (error) {
        get().setError(taskListErrorKey('updateWorkflowFull', taskListId), String(error));
        throw error;
      }
    },

    getTaskCountsByStatus: async (taskListId: string) => {
      try {
        const counts = await GetTaskCountsByStatus(taskListId);
        return counts ?? {};
      } catch (error) {
        get().setError(taskListErrorKey('getTaskCountsByStatus', taskListId), String(error));
        return {};
      }
    },

    reorderWorkflowStatuses: async (taskListId: string, statusOrder: number[]) => {
      try {
        await ReorderWorkflowStatuses(taskListId, statusOrder);
        await get().loadTaskList(taskListId);
      } catch (error) {
        get().setError(taskListErrorKey('reorderWorkflowStatuses', taskListId), String(error));
      }
    },

    // Task management
    createTask: async (taskListId: string, title: string, description?: string, code?: string, link?: string, parentId?: string) => {
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

    updateTask: async (taskId: string, title: string, description?: string, code?: string, link?: string) => {
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

    updateTaskFull: async (taskId: string, title: string, description?: string, code?: string, link?: string, assigneeName?: string, assigneeId?: string, creatorName?: string, creatorId?: string) => {
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

    setTaskConversation: async (taskId: string, conversationId: string | null) => {
      const normalized = conversationId && conversationId.trim() ? conversationId.trim() : undefined;
      let affectedTaskListId: string | undefined;
      set((state) => {
        const newCache = new Map(state.taskLists);
        for (const [tlId, taskList] of newCache.entries()) {
          const tasks = taskList.tasks;
          if (tasks) {
            const idx = tasks.findIndex((t) => t.id === taskId);
            if (idx >= 0) {
              affectedTaskListId = tlId;
              const updatedTasks = [...tasks];
              updatedTasks[idx] = { ...updatedTasks[idx], conversationId: normalized };
              newCache.set(tlId, { ...taskList, tasks: updatedTasks });
              return { taskLists: newCache };
            }
          }
        }
        return {};
      });
      try {
        await SetTaskConversation(taskId, normalized ?? null);
      } catch (error) {
        get().setError('setTaskConversation', String(error));
        // Update otimista divergiu do backend: recarrega a lista para restaurar
        // o estado real e repropaga para o caller (TaskForm) poder dar feedback.
        if (affectedTaskListId) {
          await get().loadTaskList(affectedTaskListId);
        }
        throw error;
      }
    },

    setTaskListConversation: async (taskListId: string, conversationId: string | null) => {
      const normalized = conversationId && conversationId.trim() ? conversationId.trim() : undefined;
      set((state) => {
        const newCache = new Map(state.taskLists);
        const taskList = newCache.get(taskListId);
        if (taskList) {
          newCache.set(taskListId, { ...taskList, conversationId: normalized });
          return { taskLists: newCache };
        }
        return {};
      });
      try {
        await SetTaskListConversation(taskListId, normalized ?? null);
      } catch (error) {
        get().setError('setTaskListConversation', String(error));
        // Update otimista divergiu do backend: recarrega a lista para restaurar
        // o estado real e repropaga para o caller (TaskListView) dar feedback.
        await get().loadTaskList(taskListId);
        throw error;
      }
    },

    updateTaskAssignee: async (taskId: string, assigneeName: string, assigneeId?: string) => {
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

    deleteTask: async (taskId: string) => {
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

    updateTaskStatus: async (taskId: string, statusId: number) => {
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

    reorderTasks: async (taskListId: string, statusId: number, orderedIds: string[]) => {
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
        get().setError(taskListErrorKey('reorderTasks', taskListId), String(error));
      }
    },

    promoteTask: async (taskId: string) => {
      try {
        await PromoteTask(taskId);
      } catch (error) {
        get().setError('promoteTask', String(error));
      }
    },

    demoteTask: async (taskId: string, parentId: string) => {
      try {
        await DemoteTask(taskId, parentId);
      } catch (error) {
        get().setError('demoteTask', String(error));
      }
    },

    // TaskNote management
    loadTaskNotes: async (taskId: string) => {
      try {
        const rawNotes = await GetTaskNotes(taskId);
        return (rawNotes || []).map(normalizeTaskNote);
      } catch (error) {
        get().setError('loadTaskNotes', String(error));
        return [];
      }
    },

    createTaskNote: async (taskId: string, type: TaskNoteType, content: string, authorName?: string, authorId?: string) => {
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

    updateTaskNote: async (noteId: string, content: string) => {
      try {
        await UpdateTaskNote(noteId, content);
      } catch (error) {
        get().setError('updateTaskNote', String(error));
      }
    },

    deleteTaskNote: async (noteId: string) => {
      try {
        await DeleteTaskNote(noteId);
      } catch (error) {
        get().setError('deleteTaskNote', String(error));
      }
    },

    getTaskListTasks: (taskListId: string) => {
      const taskList = get().taskLists.get(taskListId);
      return taskList?.tasks || [];
    },

    // UI helpers
    toggleTaskExpanded: (taskId: string) => {
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

    setTaskListLoading: (taskListId: string, isLoading: boolean) => {
      set((state) => {
        const loadingByTaskListId = new Map(state.loadingByTaskListId);
        if (isLoading) {
          loadingByTaskListId.set(taskListId, true);
        } else {
          loadingByTaskListId.delete(taskListId);
        }
        return { loadingByTaskListId };
      });
    },

    // Cache operations
    invalidateTaskList: (taskListId: string) => {
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

    getCachedTaskList: (taskListId: string) => {
      return get().taskLists.get(taskListId);
    },
  };
});
