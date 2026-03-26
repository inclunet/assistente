import { useState, useCallback, forwardRef, useImperativeHandle } from 'react';
import { LinkOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useTaskListStore } from '../../store/taskListStore';
import { openTaskLink } from '../../lib/deepLinks';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import { Modal } from '../ui/Modal';
import type { Task, TaskListWithWorkflow } from '../../types/tasklist';
import TaskForm from './TaskForm';
import TaskDetailModal from './TaskDetailModal';
import './TasksTable.css';

interface TasksTableProps {
  taskListId: number;
  tasks: Task[];
  taskList?: TaskListWithWorkflow;
  onTaskCreated?: (task: Task) => void;
  onTaskUpdated?: (task: Task) => void;
  onTaskDeleted?: (taskId: number) => void;
}

export interface TasksTableRef {
  openCreateModal: () => void;
}

const TasksTable = forwardRef<TasksTableRef, TasksTableProps>(function TasksTable({
  taskListId,
  tasks,
  taskList,
  onTaskCreated,
  onTaskUpdated,
  onTaskDeleted,
}, ref) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { deleteTask, promoteTask, demoteTask } = useTaskListStore();

  const handleLinkClick = useCallback((e: React.MouseEvent, link: string) => {
    e.stopPropagation();
    openTaskLink(link, { navigate });
  }, [navigate]);

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [isDetailModalOpen, setIsDetailModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [detailTask, setDetailTask] = useState<Task | null>(null);
  const [isDemoteModalOpen, setIsDemoteModalOpen] = useState(false);
  const [demotingTask, setDemotingTask] = useState<Task | null>(null);

  const handleOpenCreateModal = () => {
    setEditingTask(null);
    setIsCreateModalOpen(true);
  };

  const handleOpenEditModal = (task: Task) => {
    setEditingTask(task);
    setIsEditModalOpen(true);
  };

  const handleCloseModals = () => {
    setIsCreateModalOpen(false);
    setIsEditModalOpen(false);
    setIsDetailModalOpen(false);
    setEditingTask(null);
    setDetailTask(null);
    setIsDemoteModalOpen(false);
    setDemotingTask(null);
  };

  useImperativeHandle(ref, () => ({
    openCreateModal: handleOpenCreateModal,
  }));

  const handleTaskCreated = (task: Task) => {
    handleCloseModals();
    onTaskCreated?.(task);
  };

  const handleTaskUpdated = (task: Task) => {
    handleCloseModals();
    onTaskUpdated?.(task);
  };

  const handleDeleteTask = async (taskId: number) => {
    if (!confirm(t('tasklist.confirmDeleteTask', 'Tem certeza que deseja deletar esta tarefa?'))) {
      return;
    }

    try {
      await deleteTask(taskId);
      onTaskDeleted?.(taskId);
    } catch (error) {
      console.error('Erro ao deletar tarefa:', error);
    }
  };

  const handleOpenDemoteModal = (task: Task) => {
    setDemotingTask(task);
    setIsDemoteModalOpen(true);
  };

  const handleDemote = async (parentId: number) => {
    if (!demotingTask) return;
    try {
      await demoteTask(demotingTask.id, parentId);
      setIsDemoteModalOpen(false);
      setDemotingTask(null);
    } catch {
      // error handled by store
    }
  };

  const formatDate = (date: string | Date | undefined) => {
    if (!date) return '—';
    const d = new Date(date);
    return d.toLocaleDateString('pt-BR', { month: '2-digit', day: '2-digit' });
  };

  const columns: DataGridColumn<Task>[] = [
    {
      key: 'code',
      label: t('tasklist.code', 'Código'),
      width: '140px',
      format: (code) => {
        const val = code as string | undefined;
        if (!val) return <span className="task-code--empty">—</span>;
        return <span className="task-code">{val}</span>;
      },
    },
    {
      key: 'title',
      label: t('tasklist.taskTitle', 'Título'),
      width: '400px',
      format: (title, item) => {
        const tk = item as Task;
        return (
          <span className="task-title-cell">
            <span>{title as string}</span>
            {tk.link && (
              <span
                className="task-link-icon"
                title={tk.link}
                role="button"
                tabIndex={-1}
                aria-label={`Link: ${tk.link}`}
                onClick={(e) => handleLinkClick(e, tk.link!)}
              ><LinkOutlined /></span>
            )}
          </span>
        );
      },
    },
    {
      key: 'statusId',
      label: t('tasklist.status', 'Status'),
      width: '120px',
      format: (statusId) => {
        const status = taskList?.workflow?.statuses?.find(s => s.id === statusId);
        return (
          <span className={`task-status task-status-${status?.label?.toLowerCase()}`}>
            {status?.label || `Status ${statusId}`}
          </span>
        );
      },
    },
    {
      key: 'assigneeName',
      label: t('tasklist.assignee', 'Responsável'),
      width: '140px',
      format: (assigneeName) => {
        const name = assigneeName as string | undefined;
        if (!name) return <span className="task-assignee--empty">—</span>;
        return <span className="task-assignee">{name}</span>;
      },
    },
    {
      key: 'creatorName',
      label: t('tasklist.creator', 'Criador'),
      width: '140px',
      format: (creatorName) => {
        const name = creatorName as string | undefined;
        if (!name) return <span className="task-creator--empty">—</span>;
        return <span className="task-creator">{name}</span>;
      },
    },
    {
      key: 'dueDate',
      label: t('tasklist.dueDate', 'Data de vencimento'),
      width: '120px',
      format: (dueDate) => {
        const date = dueDate as string | undefined;
        const isOverdue = date && new Date(date) < new Date();
        return (
          <span className={isOverdue ? 'task-overdue' : ''}>
            {formatDate(date)}
          </span>
        );
      },
    },
    {
      key: 'createdAt',
      label: t('tasklist.created', 'Criado em'),
      width: '120px',
      format: (createdAt) => formatDate(createdAt as string),
    },
  ];

  const hasTasks = tasks && tasks.length > 0;

  const getRowActions = (task: Task) => {
    const actions = [
      {
        id: `details-${task.id}`,
        label: t('tasklist.details', 'Detalhes'),
        action: () => {
          setDetailTask(task);
          setIsDetailModalOpen(true);
        },
      },
      {
        id: `edit-${task.id}`,
        label: t('tasklist.edit', 'Editar'),
        action: () => handleOpenEditModal(task),
      },
    ];

    // Promote: só aparece se a task tem parentId (é subtask)
    if (task.parentId) {
      actions.push({
        id: `promote-${task.id}`,
        label: t('tasklist.promoteTask', 'Promover (remover pai)'),
        action: async () => {
          try {
            await promoteTask(task.id);
          } catch {
            // error handled by store
          }
        },
      });
    }

    // Demote: só aparece se há outras tasks na lista para servir de pai
    if (tasks.length > 1) {
      actions.push({
        id: `demote-${task.id}`,
        label: t('tasklist.demoteTask', 'Tornar subtarefa...'),
        action: () => handleOpenDemoteModal(task),
      });
    }

    actions.push({
      id: `delete-${task.id}`,
      label: t('tasklist.delete', 'Deletar'),
      action: () => handleDeleteTask(task.id),
      danger: true,
    } as typeof actions[number] & { danger: boolean });

    return actions;
  };

  return (
    <div className="tasks-table-container">
      {hasTasks ? (
        <>
          <div className="tasks-table-toolbar">
            <Button onClick={handleOpenCreateModal} variant="primary">
              <PlusOutlined /> {t('tasklist.createTask', 'Criar Tarefa')}
            </Button>
          </div>

          <DataGrid<Task>
            items={tasks}
            columns={columns}
            onActivate={(task) => {
              setDetailTask(task);
              setIsDetailModalOpen(true);
            }}
            getRowActions={getRowActions}
          />
        </>
      ) : (
        <div className="tasks-table-empty">
          <p>{t('tasklist.noTasks', 'Nenhuma tarefa nesta lista')}</p>
          <Button onClick={handleOpenCreateModal} variant="primary">
            <PlusOutlined /> {t('tasklist.createTask', 'Criar Tarefa')}
          </Button>
        </div>
      )}

      {/* Modal de Criar Tarefa */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={handleCloseModals}
        title={t('tasklist.createTask', 'Criar Tarefa')}
      >
        <TaskForm
          taskListId={taskListId}
          onSuccess={handleTaskCreated}
          onCancel={handleCloseModals}
        />
      </Modal>

      {/* Modal de Editar Tarefa */}
      <Modal
        isOpen={isEditModalOpen}
        onClose={handleCloseModals}
        title={t('tasklist.editTask', 'Editar Tarefa')}
      >
        {editingTask && (
          <TaskForm
            taskListId={taskListId}
            task={editingTask}
            onSuccess={handleTaskUpdated}
            onCancel={handleCloseModals}
          />
        )}
      </Modal>

      {/* Modal de Tornar Subtarefa (Demote) */}
      <Modal
        isOpen={isDemoteModalOpen}
        onClose={handleCloseModals}
        title={t('tasklist.demoteTaskTitle', 'Tornar subtarefa de...')}
      >
        {demotingTask && (
          <div className="demote-task-list">
            <p className="demote-task-hint">
              {t('tasklist.demoteTaskHint', 'Selecione a tarefa-pai para "{{name}}":', { name: demotingTask.title })}
            </p>
            {tasks
              .filter((t) => t.id !== demotingTask.id)
              .map((candidate) => (
                <button
                  key={candidate.id}
                  className="demote-task-option"
                  onClick={() => handleDemote(candidate.id)}
                >
                  {candidate.title}
                </button>
              ))}
          </div>
        )}
      </Modal>
      {/* Modal de Detalhes da Tarefa */}
      <TaskDetailModal
        isOpen={isDetailModalOpen}
        onClose={handleCloseModals}
        task={detailTask}
        statuses={taskList?.workflow?.statuses ?? []}
      />
    </div>
  );
});

TasksTable.displayName = 'TasksTable';

export default TasksTable;
