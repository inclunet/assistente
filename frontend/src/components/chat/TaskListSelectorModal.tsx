import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { DataGrid, type DataGridColumn } from '../ui/DataGrid';
import { Toolbar } from '../ui/Toolbar';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useTaskListStore } from '../../store/taskListStore';
import type { database } from '@wailsjs/go/models';

interface TaskListSelectorModalProps {
  isOpen: boolean;
  onClose: () => void;
  conversationId: number;
  linkedTaskListIds: number[];
}

interface TaskListRow {
  id: number;
  title: string;
  description: string;
  created_at: string;
  taskCount: number;
  isLinked: boolean;
  [key: string]: unknown;
}

export const TaskListSelectorModal: React.FC<TaskListSelectorModalProps> = ({
  isOpen,
  onClose,
  conversationId,
  linkedTaskListIds,
}) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const { linkToConversation, fetchAllTaskLists } = useTaskListStore();

  const [taskLists, setTaskLists] = useState<database.TaskList[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    setIsLoading(true);
    fetchAllTaskLists()
      .then((lists) => setTaskLists(lists))
      .finally(() => setIsLoading(false));
  }, [isOpen, fetchAllTaskLists]);

  const rows = useMemo<TaskListRow[]>(() => {
    const mapped = taskLists.map((tl) => ({
      id: tl.id,
      title: tl.title,
      description: tl.description || '',
      created_at: tl.created_at,
      taskCount: tl.tasks?.length ?? 0,
      isLinked: linkedTaskListIds.includes(tl.id),
    }));

    if (!searchTerm.trim()) return mapped;
    const lower = searchTerm.toLowerCase();
    return mapped.filter(
      (r) =>
        r.title.toLowerCase().includes(lower) ||
        r.description.toLowerCase().includes(lower)
    );
  }, [taskLists, searchTerm, linkedTaskListIds]);

  const handleLink = useCallback(
    async (item: TaskListRow) => {
      if (item.isLinked) return;
      await linkToConversation(item.id, conversationId);
      announce(t('tasklist.linkedSuccess'));
      onClose();
    },
    [linkToConversation, conversationId, announce, t, onClose]
  );

  const getRowId = useCallback((item: TaskListRow) => item.id, []);

  const columns = useMemo<DataGridColumn<TaskListRow>[]>(
    () => [
      {
        key: 'title',
        label: t('tasklist.title'),
        width: '40%',
      },
      {
        key: 'description',
        label: t('tasklist.description'),
        width: '35%',
        truncate: true,
      },
      {
        key: 'taskCount',
        label: t('tasklist.totalTasks'),
        width: '15%',
      },
      {
        key: 'isLinked',
        label: '',
        width: '10%',
        format: (value: unknown) =>
          value ? `✅ ${t('tasklist.linked')}` : '',
      },
    ],
    [t]
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('tasklist.selectTaskList')}
      size="lg"
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', minHeight: '300px' }}>
        <Toolbar
          ariaLabel={t('tasklist.selectTaskListHint')}
          searchPlaceholder={t('tasklist.search')}
          searchValue={searchTerm}
          onSearchChange={setSearchTerm}
        />

        {isLoading ? (
          <p style={{ padding: '1rem', color: 'var(--text-muted)' }}>{t('tasklist.loading')}</p>
        ) : rows.length === 0 ? (
          <p style={{ padding: '1rem', color: 'var(--text-muted)' }}>
            {searchTerm ? t('tasklist.noLists') : t('tasklist.noLists')}
          </p>
        ) : (
          <DataGrid
            items={rows}
            columns={columns}
            getItemId={getRowId}
            onActivate={handleLink}
            label={t('tasklist.selectTaskList')}
          />
        )}
      </div>
    </Modal>
  );
};
