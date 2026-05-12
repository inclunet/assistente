/**
 * Task List Manager Types
 * TypeScript interfaces para gerenciamento de listas de tarefas
 */

// ==================== Workflow Types ====================

export interface TaskListWorkflowStatus {
  id: number;
  order: number;
  label: string;
  color: string;
  icon: string;
}

export type WorkflowTransitions = Record<number, number[]>;

export interface TaskListWorkflow {
  id: string;
  taskListId: string;
  statuses: TaskListWorkflowStatus[];
  allowedTransitions: WorkflowTransitions;
  initialStatusId: number;
  createdAt: string;
  updatedAt: string;
}

// ==================== Task Types ====================

export type TaskNoteType = 1 | 2 | 3 | 4;

export const TASK_NOTE_TYPES = {
  INTERNAL: 1 as TaskNoteType,
  CUSTOMER: 2 as TaskNoteType,
  AGENT: 3 as TaskNoteType,
  SYSTEM: 4 as TaskNoteType,
} as const;

export interface TaskNote {
  id: string;
  taskId: string;
  type: TaskNoteType;
  content: string;
  authorName?: string;
  authorId?: string;
  /** Sistema externo de origem (ex.: jira), quando a nota veio de sync */
  source?: string;
  externalId?: string;
  externalParentId?: string;
  externalUpdatedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Task {
  id: string;
  taskListId: string;
  title: string;
  description: string;
  code?: string;
  link?: string;
  statusId: number;
  parentId?: string;
  order: number;
  assigneeName?: string;
  assigneeId?: string;
  creatorName?: string;
  creatorId?: string;
  dueDate?: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  
  // Relações
  subtasks?: Task[];
  notes?: TaskNote[];
}

// ==================== TaskList Types ====================

export type ViewMode = 'list' | 'kanban';

/** Regras opcionais por lista (espelha TaskListValidationPolicy no backend). */
export interface TaskListValidationPolicy {
  task_code_regex?: string;
  allowed_note_sources?: string[];
  note_external_id_regex?: string;
  note_external_parent_id_regex?: string;
}

export interface TaskList {
  id: string;
  title: string;
  /** Slug estável (minúsculas), opcional */
  slug?: string;
  description: string;
  preferredViewMode: ViewMode;
  createdAt: string;
  updatedAt: string;
  /** Política opcional de validação (JSON); ausente = sem restrições extras */
  validationPolicy?: TaskListValidationPolicy;
  
  // Relações
  workflow?: TaskListWorkflow;
  tasks?: Task[];
}

export interface TaskListWithWorkflow extends TaskList {
  workflow: TaskListWorkflow;
  tasks: Task[];
}

// ==================== UI/Store Types ====================

export interface TaskListState {
  taskLists: Map<string, TaskListWithWorkflow>;
  expandedTasks: Set<string>;
  isLoading: boolean;
  error?: string;
}

// ==================== API Request/Response Types ====================

export interface CreateTaskListRequest {
  title: string;
  description?: string;
  conversationId?: string;
  templateWorkflowId?: string; // Para clonar workflow de outra tasklist
}

export interface CreateTaskRequest {
  taskListId: string;
  title: string;
  description?: string;
  parentId?: string;
}

export interface UpdateTaskRequest {
  id: string;
  title?: string;
  description?: string;
}

export interface UpdateTaskStatusRequest {
  id: string;
  statusId: number;
}

export interface CreateWorkflowRequest {
  taskListId: string;
  statuses: TaskListWorkflowStatus[];
  allowedTransitions: WorkflowTransitions;
  initialStatusId: number;
}

// ==================== Statistics ====================

export interface TaskListStats {
  total: number;
  byStatus: Record<number, number>;
  completedCount?: number;
}

// ==================== LLM Tool Parameters ====================

export interface UpsertTaskParams {
  taskListId: string;
  title: string;
  description?: string;
  statusId?: number;
  parentId?: string;
  taskId?: string; // Se presente, é UPDATE; se ausente, é CREATE
  dueDate?: string;
}

export interface GetTaskListParams {
  taskListId: string;
  includeSubtasks?: boolean;
}

export interface ListTaskListsParams {
  includeStats?: boolean;
}
