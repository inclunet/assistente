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

export interface Task {
  id: number;
  taskListId: number;
  title: string;
  description: string;
  statusId: number; // ID do status (não label)
  parentId?: number; // ID da task pai (para subtasks)
  order: number;
  dueDate?: string; // ISO date string
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  
  // Relações
  subtasks?: Task[];
}

// ==================== TaskList Types ====================

export type ViewMode = 'list' | 'kanban';

export interface TaskList {
  id: number;
  title: string;
  description: string;
  preferredViewMode: ViewMode;
  createdAt: string;
  updatedAt: string;
  
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

export interface BulkUpsertTasksParams {
  taskListId: number;
  tasks: UpsertTaskParams[];
}

export interface GetTaskListParams {
  taskListId: number;
  includeSubtasks?: boolean;
}

export interface ListTaskListsParams {
  includeStats?: boolean;
}
