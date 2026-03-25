import { useState, useEffect, useCallback, forwardRef, useImperativeHandle } from 'react';
import { UnorderedListOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { GetAllTaskLists } from '@wailsjs/go/main/App';
import { database } from '../../../wailsjs/go/models';
import { EventsOn } from '@wailsjs/runtime/runtime';

export interface TaskListHistoryPickerProps {
  value?: number;
  onChange: (taskListId: number, taskList: database.TaskList) => void;
  label?: string;
  disabled?: boolean;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface TaskListHistoryPickerRef {
  reload: () => Promise<void>;
}

export const TaskListHistoryPicker = forwardRef<TaskListHistoryPickerRef, TaskListHistoryPickerProps>(({
  value,
  onChange,
  label,
  disabled = false,
  maxWidth = '200px',
  onAnnounce,
}, ref) => {
  const { t } = useTranslation();
  const [taskLists, setTaskLists] = useState<database.TaskList[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const loadTaskLists = useCallback(async () => {
    try {
      setIsLoading(true);
      const result = await GetAllTaskLists();
      const sorted = (result || []).sort((a, b) => {
        const dateA = new Date(a.updated_at as string | number | Date).getTime();
        const dateB = new Date(b.updated_at as string | number | Date).getTime();
        return dateB - dateA;
      });
      setTaskLists(sorted);
    } catch {
      // silently fail
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadTaskLists();
  }, [loadTaskLists]);

  useEffect(() => {
    if (typeof window === 'undefined' || !(window as unknown as Record<string, unknown>).runtime) return;
    const handleUpdate = () => { loadTaskLists(); };
    const unsubCreated = EventsOn('taskList:created', handleUpdate);
    const unsubUpdated = EventsOn('taskList:updated', handleUpdate);
    const unsubDeleted = EventsOn('taskList:deleted', handleUpdate);

    return () => {
      unsubCreated();
      unsubUpdated();
      unsubDeleted();
    };
  }, [loadTaskLists]);

  useImperativeHandle(ref, () => ({
    reload: loadTaskLists,
  }));

  const formatDate = (dateValue: string | number | Date): string => {
    const date = new Date(dateValue);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffMins < 1) return t('common.now', 'agora');
    if (diffMins < 60) return `${diffMins}min`;
    if (diffHours < 24) return `${diffHours}h`;
    if (diffDays < 7) return `${diffDays}d`;

    return date.toLocaleDateString(undefined, {
      day: '2-digit',
      month: '2-digit',
    });
  };

  const items: ComboboxItem[] = taskLists.map(tl => ({
    value: tl.id.toString(),
    label: tl.title || t('tasklist.noTitle', 'Sem título'),
    sublabel: `${tl.tasks?.length || 0} ${t('tasklist.totalTasks', 'tarefas')} • ${formatDate(tl.updated_at)}`,
  }));

  const selectedValue = value ? value.toString() : '';

  const handleSelect = (selectedValue: string) => {
    const id = parseInt(selectedValue, 10);
    const taskList = taskLists.find(tl => tl.id === id);
    if (taskList) {
      onChange(id, taskList);
    }
  };

  return (
    <BasePicker
      variant="toolbar"
      items={items}
      selected={selectedValue}
      onSelect={handleSelect}
      label={label || t('tasklist.historyPicker', 'Histórico (Ctrl+H)')}
      icon={<UnorderedListOutlined />}
      placeholder={isLoading ? t('tasklist.loading', 'Carregando...') : t('tasklist.searchHistory', 'Buscar lista...')}
      disabled={disabled || isLoading}
      maxWidth={maxWidth}
      onAnnounce={onAnnounce}
      onOpen={loadTaskLists}
      showLoadingState={false}
      showEmptyState={false}
      wrapCombobox={false}
    />
  );
});
