import { useEffect, useRef, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useUIStore } from '../../store/uiStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { Toolbar } from '../ui/Toolbar';
import { ProfilePicker } from '../pickers/ProfilePicker';
import TasksTable, { type TasksTableRef } from './TasksTable';
import KanbanBoard, { type KanbanBoardRef } from './KanbanBoard';
import type { ViewMode } from '../../types/tasklist';

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
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const tabProfileSlug = wsActiveTab?.type === 'tasklist'
    ? (wsActiveTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';

  const taskList = useTaskListStore((s) => s.taskLists.get(taskListId));
  const { loadTaskList, setViewMode, cloneTaskList, deleteTaskList } = useTaskListStore();

  const tasksRef = useRef<TasksTableRef | KanbanBoardRef | null>(null);

  useEffect(() => {
    if (!taskList) {
      void loadTaskList(taskListId);
    }
  }, [taskListId, taskList, loadTaskList]);

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
          rightEnd={
            <ProfilePicker
              value={effectiveProfileSlug}
              onChange={(slug) => {
                if (wsActiveTab) {
                  void updateWsTab(wsActiveTab.id, { profile_override: { slug } });
                }
              }}
              variant="toolbar"
              label={t('workspace.tabProfileLabel', 'Perfil')}
              description={t('workspace.tabProfileDescription')}
              icon="✅"
              maxWidth="180px"
            />
          }
          actions={[
            {
              key: 'new-task',
              label: t('tasklist.createTask', 'Nova Tarefa'),
              icon: '➕',
              onClick: handleOpenCreateTask,
              shortcut: 'Ctrl+N',
              variant: 'primary',
            },
            ...(hasTasks
              ? [
                  {
                    key: 'toggle-view',
                    label: currentViewMode === 'list' ? 'Kanban' : 'Lista',
                    icon: currentViewMode === 'list' ? '🎯' : '📋',
                    onClick: handleToggleViewMode,
                    variant: 'secondary' as const,
                  },
                ]
              : []),
            {
              key: 'clone-list',
              label: t('tasklist.cloneList', 'Clonar Lista'),
              icon: '📋',
              onClick: handleClone,
              variant: 'secondary' as const,
            },
            {
              key: 'delete-list',
              label: t('tasklist.deleteList', 'Deletar Lista'),
              icon: '🗑️',
              onClick: handleDelete,
              variant: 'danger' as const,
            },
          ]}
        />
      </div>

      <div className="ws-content-area">
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
    </div>
  );
}
