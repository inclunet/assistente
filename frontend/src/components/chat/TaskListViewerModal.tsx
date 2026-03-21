import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Modal } from '../ui/Modal';
import { Toolbar } from '../ui/Toolbar';
import { useTaskListStore } from '../../store/taskListStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { useUIStore } from '../../store/uiStore';
import TasksTable, { type TasksTableRef } from '../taskLists/TasksTable';
import type { TaskListWithWorkflow, Task } from '../../types/tasklist';

interface TaskListViewerModalProps {
  isOpen: boolean;
  onClose: () => void;
  taskListId: number;
  onUnlink?: () => void;
}

export const TaskListViewerModal: React.FC<TaskListViewerModalProps> = ({
  isOpen,
  onClose,
  taskListId,
  onUnlink,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { announce } = useAnnouncer();
  const { addToast } = useUIStore();
  const requestConfirm = useConfirm();
  const {
    loadTaskList,
    getCachedTaskList,
    cloneTaskList,
    deleteTaskList,
    unlinkFromConversation,
  } = useTaskListStore();

  const [taskList, setTaskList] = useState<TaskListWithWorkflow | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const tasksTableRef = useRef<TasksTableRef>(null);

  useEffect(() => {
    if (!isOpen || !taskListId) return;

    const cached = getCachedTaskList(taskListId);
    if (cached) {
      setTaskList(cached);
    }

    setIsLoading(true);
    loadTaskList(taskListId)
      .then((loaded) => {
        if (loaded) setTaskList(loaded);
      })
      .finally(() => setIsLoading(false));
  }, [isOpen, taskListId, loadTaskList, getCachedTaskList]);

  const handleRefresh = useCallback(async () => {
    setIsLoading(true);
    const loaded = await loadTaskList(taskListId);
    if (loaded) setTaskList(loaded);
    setIsLoading(false);
  }, [taskListId, loadTaskList]);

  const handleCreateTask = useCallback(() => {
    tasksTableRef.current?.openCreateModal();
  }, []);

  const handleClone = useCallback(async () => {
    if (!taskList) return;
    try {
      const cloned = await cloneTaskList(taskList.id, `${taskList.title} (Cópia)`);
      if (cloned) {
        addToast(t('tasklist.clonedSuccess'), 'success');
        announce(t('tasklist.clonedSuccess'));
      }
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg, 'error');
    }
  }, [taskList, cloneTaskList, addToast, announce, t]);

  const handleDelete = useCallback(async () => {
    if (!taskList) return;
    const confirmed = await requestConfirm({
      title: t('tasklist.deleteConfirmTitle'),
      message: t('tasklist.deleteConfirmMessage', { name: taskList.title }),
    });
    if (!confirmed) return;

    try {
      await deleteTaskList(taskList.id);
      addToast(t('tasklist.deletedSuccess'), 'success');
      announce(t('tasklist.deletedSuccess'));
      onClose();
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg, 'error');
    }
  }, [taskList, requestConfirm, deleteTaskList, addToast, announce, t, onClose]);

  const handleUnlink = useCallback(async () => {
    if (!taskList) return;
    try {
      await unlinkFromConversation(taskList.id);
      addToast(t('tasklist.unlinkedSuccess'), 'success');
      announce(t('tasklist.unlinkedSuccess'));
      onUnlink?.();
      onClose();
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg, 'error');
    }
  }, [taskList, unlinkFromConversation, addToast, announce, t, onUnlink, onClose]);

  const handleOpenInPage = useCallback(async () => {
    if (!taskList) return;
    await useWorkspaceStore.getState().addTab('tasklist', String(taskList.id), taskList.title);
    navigate('/');
    onClose();
  }, [taskList, navigate, onClose]);

  const handleTaskChanged = useCallback(() => {
    void handleRefresh();
  }, [handleRefresh]);

  const title = taskList?.title || t('tasklist.loading');

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={title}
      size="lg"
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', minHeight: '300px' }}>
        <Toolbar
          ariaLabel={t('tasklist.viewLinkedTaskList')}
          isLoading={isLoading}
          actions={[
            {
              key: 'create-task',
              label: t('tasklist.createTask'),
              icon: '➕',
              onClick: handleCreateTask,
            },
            {
              key: 'clone',
              label: t('tasklist.cloneList'),
              icon: '📋',
              onClick: handleClone,
            },
            {
              key: 'open-page',
              label: t('tasklist.open'),
              icon: '🔗',
              onClick: handleOpenInPage,
            },
            {
              key: 'unlink',
              label: t('tasklist.unlinkTaskList'),
              icon: '🔓',
              onClick: handleUnlink,
            },
            {
              key: 'delete',
              label: t('tasklist.deleteList'),
              icon: '🗑️',
              variant: 'danger',
              onClick: handleDelete,
            },
          ]}
        />

        {isLoading && !taskList ? (
          <p style={{ padding: '1rem', color: 'var(--text-muted)' }}>{t('tasklist.loading')}</p>
        ) : taskList ? (
          <TasksTable
            ref={tasksTableRef}
            taskListId={taskList.id}
            tasks={taskList.tasks || []}
            taskList={taskList}
            onTaskCreated={handleTaskChanged as (task: Task) => void}
            onTaskUpdated={handleTaskChanged as (task: Task) => void}
            onTaskDeleted={handleTaskChanged as unknown as (taskId: number) => void}
          />
        ) : (
          <p style={{ padding: '1rem', color: 'var(--text-muted)' }}>{t('tasklist.noLists')}</p>
        )}
      </div>
    </Modal>
  );
};
