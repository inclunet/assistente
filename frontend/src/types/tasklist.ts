/**
 * Task List Manager Types
 * TypeScript interfaces para gerenciamento de listas de tarefas
 */

// ==================== Workflow Types ====================

export interface TaskListWorkflowStatus {
  id: number; // Identificador numérico do status (imutável)
  order: number; // Ordem de exibição
  label: string; // Nome do status
  color: string; // Cor CSS (ex: var(--accent), #ff0000)
  icon: string; // Ícone (emoji ou similar)
}

export type WorkflowTransitions = Record<number, number[]>;

export interface TaskListWorkflow {
  id: number;
  taskListId: number;
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
  id: number;
  taskId: number;
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
  id: number;
  taskListId: number;
  title: string;
  description: string;
  code?: string;
  link?: string;
  statusId: number;
  parentId?: number;
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
  id: number;
  title: string;
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
  activeTaskListId?: number;
  taskLists: Map<number, TaskListWithWorkflow>;
  expandedTasks: Set<number>;
  isLoading: boolean;
  error?: string;
}

// ==================== API Request/Response Types ====================

export interface CreateTaskListRequest {
  title: string;
  description?: string;
  conversationId?: number;
  templateWorkflowId?: number; // Para clonar workflow de outra tasklist
}

export interface CreateTaskRequest {
  taskListId: number;
  title: string;
  description?: string;
  parentId?: number;
}

export interface UpdateTaskRequest {
  id: number;
  title?: string;
  description?: string;
}

export interface UpdateTaskStatusRequest {
  id: number;
  statusId: number;
}

export interface CreateWorkflowRequest {
  taskListId: number;
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
  taskListId: number;
  title: string;
  description?: string;
  statusId?: number;
  parentId?: number;
  taskId?: number; // Se presente, é UPDATE; se ausente, é CREATE
  dueDate?: string;
}

export interface GetTaskListParams {
  taskListId: number;
  includeSubtasks?: boolean;
}

export interface ListTaskListsParams {
  includeStats?: boolean;
}
