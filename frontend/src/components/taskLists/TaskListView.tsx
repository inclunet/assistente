import { useEffect, useRef, useCallback, useMemo, useState, lazy, Suspense } from 'react';
import { AppstoreOutlined, ClearOutlined, CopyOutlined, DeleteOutlined, MessageOutlined, PlusOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useMiniChatStore } from '../../store/miniChatStore';
import type { MiniChatAdapter } from '../../store/miniChatStore';
import { useRegisterMiniChatAdapter } from '../../hooks/useRegisterMiniChatAdapter';
import { useUIStore } from '../../store/uiStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { registerDefaultFocus, unregisterDefaultFocus } from '../../hooks/useDefaultFocus';
import { isModalOpen, Modal } from '../ui/Modal';
import { Toolbar } from '../ui/Toolbar';
import TasksTable, { type TasksTableRef } from './TasksTable';
import KanbanBoard, { type KanbanBoardRef } from './KanbanBoard';
import type { ViewMode, TaskListWorkflowStatus, WorkflowTransitions } from '../../types/tasklist';
import { createTextMediaFile } from '../../services/mediaService';

const WorkflowEditor = lazy(() => import('./WorkflowEditor'));

interface TaskListViewProps {
  taskListId: number;
}

/**
 * Renderiza o conteúdo de uma TaskList individual (toolbar + table/kanban).
 * Usado dentro de uma aba do workspace.
 */
export default function TaskListView({ taskListId }: TaskListViewProps) {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const requestConfirm = useConfirm();

  const wsActiveTab = useWorkspaceStore((s) => s.getActiveTab());
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = wsActiveTab?.type === 'tasklist'
    ? (wsActiveTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';

  const taskList = useTaskListStore((s) => s.taskLists.get(taskListId));
  const { loadTaskList, setViewMode, cloneTaskList, clearTaskList, deleteTaskList, updateWorkflowFull, getTaskCountsByStatus } = useTaskListStore();

  const tasksRef = useRef<TasksTableRef | KanbanBoardRef | null>(null);
  const [isWorkflowEditorOpen, setIsWorkflowEditorOpen] = useState(false);
  const [taskCountsByStatus, setTaskCountsByStatus] = useState<Record<number, number>>({});

  useEffect(() => {
    if (!taskList) {
      void loadTaskList(taskListId);
    }
  }, [taskListId, taskList, loadTaskList]);

  const contentAreaRef = useRef<HTMLDivElement>(null);

  const focusContentArea = useCallback((): boolean => {
    const area = contentAreaRef.current;
    if (!area) return false;
    // Kanban: focus the board container which manages card focus internally
    const board = area.querySelector<HTMLElement>('.kanban-board[tabindex="0"]');
    if (board) { board.focus(); return true; }
    // DataGrid: focus a cell with tabindex=0, or the grid container
    const cell = area.querySelector<HTMLElement>('[role="gridcell"][tabindex="0"]');
    if (cell) { cell.focus(); return true; }
    const grid = area.querySelector<HTMLElement>('[role="grid"]');
    if (grid) { grid.focus(); return true; }
    return false;
  }, []);

  useEffect(() => {
    registerDefaultFocus(focusContentArea);
    return () => unregisterDefaultFocus(focusContentArea);
  }, [focusContentArea]);

  const tasks = useMemo(() => taskList?.tasks || [], [taskList?.tasks]);
  const currentViewMode: ViewMode = taskList?.preferredViewMode || 'list';
  const hasTasks = tasks.length > 0;

  const handleOpenCreateTask = useCallback(() => {
    tasksRef.current?.openCreateModal();
  }, []);

  const handleToggleViewMode = useCallback(async () => {
    const newMode: ViewMode = currentViewMode === 'list' ? 'kanban' : 'list';
    try {
      await setViewMode(taskListId, newMode);
      announce(
        t('tasklist.viewModeChanged', `Alterado para visualização ${newMode === 'list' ? 'Lista' : 'Kanban'}`)
      );
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao alterar visualização'), 'error');
    }
  }, [currentViewMode, taskListId, setViewMode, announce, addToast, t]);

  const handleOpenWorkflowEditor = useCallback(async () => {
    try {
      const counts = await getTaskCountsByStatus(taskListId);
      setTaskCountsByStatus(counts);
      setIsWorkflowEditorOpen(true);
    } catch {
      addToast(t('common.error', 'Erro ao carregar dados'), 'error');
    }
  }, [taskListId, getTaskCountsByStatus, addToast, t]);

  const handleSaveWorkflow = useCallback(async (
    statuses: TaskListWorkflowStatus[],
    transitions: WorkflowTransitions,
    initialStatusId: number,
    statusMigration: Record<number, number>,
  ) => {
    try {
      await updateWorkflowFull(taskListId, statuses, transitions, initialStatusId, statusMigration);
      setIsWorkflowEditorOpen(false);
      addToast(t('tasklist.workflow.saved', 'Workflow atualizado com sucesso'), 'success');
      announce(t('tasklist.workflow.saved', 'Workflow atualizado com sucesso'));
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      throw new Error(msg || t('tasklist.workflow.saveFailed', 'Erro ao salvar workflow'));
    }
  }, [taskListId, updateWorkflowFull, addToast, announce, t]);

  const handleClone = useCallback(async () => {
    const newTitle = `${taskList?.title || 'Lista'} (Cópia)`;
    try {
      const cloned = await cloneTaskList(taskListId, newTitle);
      if (cloned) {
        addToast(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'), 'success');
        announce(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'));
      }
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao clonar'), 'error');
    }
  }, [taskList?.title, taskListId, cloneTaskList, addToast, announce, t]);

  const handleClear = useCallback(async () => {
    const confirmed = await requestConfirm({
      title: t('tasklist.clearConfirmTitle', 'Limpar Lista'),
      message: t(
        'tasklist.clearConfirmMessage',
        `Tem certeza que deseja remover todas as tarefas de "${taskList?.title}"? Esta ação não pode ser desfeita.`
      ),
    });
    if (!confirmed) return;

    try {
      await clearTaskList(taskListId);
      addToast(t('tasklist.clearedSuccess', 'Lista limpa com sucesso'), 'success');
      announce(t('tasklist.clearedSuccess', 'Lista limpa com sucesso'));
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao limpar'), 'error');
    }
  }, [taskList?.title, taskListId, requestConfirm, clearTaskList, addToast, announce, t]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (isModalOpen()) return;

      if (e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && (e.key === 'l' || e.key === 'L')) {
        e.preventDefault();
        void handleClear();
        return;
      }

      if (e.ctrlKey || e.altKey || e.metaKey || e.shiftKey) return;
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement)?.isContentEditable) return;

      if (e.key === 'n' || e.key === 'N') {
        e.preventDefault();
        handleOpenCreateTask();
        return;
      }

      if (e.key === 'd' || e.key === 'D') {
        e.preventDefault();
        void handleClone();
        return;
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handleOpenCreateTask, handleClear, handleClone]);

  const tasklistMiniChatAdapter = useMemo((): MiniChatAdapter | null => {
    if (!wsActiveTab || wsActiveTab.type !== 'tasklist' || !taskList) return null;

    return {
      prepare: async () => {
        const taskLabel = t('tasklist.miniChatContext.taskCount', { count: tasks.length });
        const header = `${taskList.title}\n${taskLabel}\n`;
        const body = tasks
          .slice(0, 40)
          .map((x) => `- ${String(x.title || '').trim()}`)
          .join('\n');
        const contextDisplay = `${header}${body || t('tasklist.miniChatContext.noTasks')}`;
        return { ok: true, contextDisplay, meta: null };
      },
      send: async (instruction, media) => {
        const taskLabel = t('tasklist.miniChatContext.taskCount', { count: tasks.length });
        const header = `${taskList.title}\n${taskLabel}\n`;
        const body = tasks
          .slice(0, 40)
          .map((x) => `- ${String(x.title || '').trim()}`)
          .join('\n');
        const contextAttachment = await createTextMediaFile(
          'tasklist-context.md',
          `${header}${body || t('tasklist.miniChatContext.noTasks')}`,
          'text/markdown',
        );
        return {
          content: instruction,
          mediaFiles: [...(media ?? []), contextAttachment],
          paramsOverride: {
            profileSlug: effectiveProfileSlug || undefined,
          },
        };
      },
    };
  }, [wsActiveTab, taskList, tasks, effectiveProfileSlug, t]);

  useRegisterMiniChatAdapter(wsActiveTab?.id, tasklistMiniChatAdapter);

  const handleDelete = useCallback(async () => {
    const confirmed = await requestConfirm({
      title: t('tasklist.deleteConfirmTitle', 'Deletar Lista'),
      message: t(
        'tasklist.deleteConfirmMessage',
        `Tem certeza que deseja deletar "${taskList?.title}"? Esta ação não pode ser desfeita.`
      ),
    });
    if (!confirmed) return;

    try {
      await deleteTaskList(taskListId);
      addToast(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'), 'success');
      announce(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'));
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao deletar'), 'error');
    }
  }, [taskList?.title, taskListId, requestConfirm, deleteTaskList, addToast, announce, t]);

  if (!taskList) {
    return <div className="tasklist-loading">{t('tasklist.loading', 'Carregando...')}</div>;
  }

  return (
    <div className="tasklist-detail">
      <div className="ws-content-toolbar">
        <Toolbar
          left={
            <h1 className="page-toolbar__title">{taskList.title}</h1>
          }
          actions={[
            {
              key: 'mini-chat',
              label: t('editor.inlineChat.title'),
              icon: <MessageOutlined />,
              shortcut: 'Ctrl+Shift+I',
              onClick: () => void useMiniChatStore.getState().requestOpen(),
            },
            {
              key: 'new-task',
              label: t('tasklist.createTask', 'Nova Tarefa'),
              icon: <PlusOutlined />,
              onClick: handleOpenCreateTask,
              shortcut: 'N',
              variant: 'primary',
            },
            ...(hasTasks
              ? [
                  {
                    key: 'toggle-view',
                    label: currentViewMode === 'list' ? 'Kanban' : 'Lista',
                    icon: currentViewMode === 'list' ? <AppstoreOutlined /> : <UnorderedListOutlined />,
                    onClick: handleToggleViewMode,
                    variant: 'secondary' as const,
                  },
                ]
              : []),
            {
              key: 'edit-workflow',
              label: t('tasklist.workflow.editWorkflow', 'Editar Workflow'),
              icon: '⚙️',
              onClick: handleOpenWorkflowEditor,
              variant: 'secondary' as const,
            },
            {
              key: 'clone-list',
              label: t('tasklist.duplicate', 'Duplicar'),
              icon: <CopyOutlined />,
              shortcut: 'D',
              onClick: handleClone,
              variant: 'secondary' as const,
            },
            {
              key: 'clear-list',
              label: t('tasklist.clear', 'Limpar'),
              icon: <ClearOutlined />,
              shortcut: 'Ctrl+L',
              onClick: () => void handleClear(),
              variant: 'danger' as const,
              disabled: !hasTasks,
            },
            {
              key: 'delete-list',
              label: t('tasklist.delete', 'Apagar'),
              icon: <DeleteOutlined />,
              onClick: handleDelete,
              variant: 'danger' as const,
            },
          ]}
        />
      </div>

      <div className="ws-content-area" ref={contentAreaRef}>
        {currentViewMode === 'kanban' ? (
          <KanbanBoard
            ref={(r) => { tasksRef.current = r; }}
            taskListId={taskListId}
            tasks={tasks}
            taskList={taskList}
            onTaskCreated={() => {}}
            onTaskUpdated={() => {}}
            onTaskDeleted={() => {}}
          />
        ) : (
          <TasksTable
            ref={(r) => { tasksRef.current = r; }}
            taskListId={taskListId}
            tasks={tasks}
            taskList={taskList}
            onTaskCreated={() => {}}
            onTaskUpdated={() => {}}
            onTaskDeleted={() => {}}
          />
        )}
      </div>

      {isWorkflowEditorOpen && taskList.workflow && (
        <Modal
          isOpen={isWorkflowEditorOpen}
          onClose={() => setIsWorkflowEditorOpen(false)}
          title={t('tasklist.workflow.editWorkflow', 'Editar Workflow')}
          size="lg"
        >
          <Suspense fallback={<div>{t('tasklist.loading', 'Carregando...')}</div>}>
            <WorkflowEditor
              workflow={taskList.workflow}
              taskCountsByStatus={taskCountsByStatus}
              onSave={handleSaveWorkflow}
              onCancel={() => setIsWorkflowEditorOpen(false)}
            />
          </Suspense>
        </Modal>
      )}
    </div>
  );
}
