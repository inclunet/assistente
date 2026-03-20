import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import { Button } from '../ui/Button';
import { Toolbar } from '../ui/Toolbar';
import type { TaskListWithWorkflow } from '../../types/tasklist';
import './TaskListsDirectory.css';

interface TaskListsDirectoryProps {
  onCreateNew?: () => void;
  onOpenList?: (taskListId: number) => void;
}

export default function TaskListsDirectory({
  onCreateNew,
  onOpenList,
}: TaskListsDirectoryProps) {
  const { t } = useTranslation();
  const taskLists = useTaskListStore((state) => state.taskLists);

  // Usa useMemo para evitar recalcular o array a cada render
  const allLists = useMemo(() => Array.from(taskLists.values()), [taskLists]);

  const formatDate = (dateString: string | undefined) => {
    if (!dateString) return '—';
    const date = new Date(dateString);
    return date.toLocaleDateString('pt-BR', { month: '2-digit', day: '2-digit', year: '2-digit' });
  };

  const handleDeleteList = async (_taskListId: number) => {
    if (!confirm(t('tasklist.confirmDelete', 'Tem certeza que deseja deletar esta lista?'))) {
      return;
    }
    // Delete será feito pelo store
  };

  const handleCloneList = (_taskListId: number) => {
    // Clone será iniciado pelo parent ou store
  };

  const getRowActions = (list: TaskListWithWorkflow) => [
    {
      id: `open-${list.id}`,
      label: t('tasklist.open', 'Abrir'),
      action: () => onOpenList?.(list.id),
    },
    {
      id: `clone-${list.id}`,
      label: t('tasklist.clone', 'Clonar'),
      action: () => handleCloneList(list.id),
    },
    {
      id: `delete-${list.id}`,
      label: t('tasklist.delete', 'Deletar'),
      action: () => handleDeleteList(list.id),
      danger: true,
    },
  ];

  const columns: DataGridColumn<TaskListWithWorkflow>[] = [
    {
      key: 'title',
      label: t('tasklist.title', 'Título'),
      width: '300px',
    },
    {
      key: 'description',
      label: t('tasklist.description', 'Descrição'),
      width: '400px',
    },
    {
      key: 'createdAt',
      label: t('tasklist.created', 'Criado em'),
      width: '120px',
      format: (createdAt) => formatDate(createdAt as string),
    },
  ];

  if (!allLists || allLists.length === 0) {
    return (
      <div className="tasklists-directory-empty">
        <p>{t('tasklist.noLists', 'Nenhuma lista de tarefas criada')}</p>
        <Button onClick={onCreateNew} variant="primary">
          ➕ {t('tasklist.createFirst', 'Criar Primeira Lista')}
        </Button>
      </div>
    );
  }

  return (
    <div className="tasklists-directory-container">
      <Toolbar
        left={<span>{t('tasklist.allLists', 'Todas as Listas')}</span>}
        right={
          <Button onClick={onCreateNew} variant="primary">
            ➕ {t('tasklist.newList', 'Nova Lista')}
          </Button>
        }
      />

      <div className="tasklists-directory-grid">
        <DataGrid<TaskListWithWorkflow>
          items={allLists}
          columns={columns}
          onActivate={(list) => onOpenList?.(list.id)}
          getRowActions={getRowActions}
        />
      </div>
    </div>
  );
}
