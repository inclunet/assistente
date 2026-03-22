import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import type { TaskListWithWorkflow } from '../../types/tasklist';
import './TaskListHeader.css';

interface TaskListHeaderProps {
  taskList: TaskListWithWorkflow;
  onRefresh?: () => void;
}

export default function TaskListHeader({ taskList, onRefresh }: TaskListHeaderProps) {
  const { t } = useTranslation();
  const { updateTaskList } = useTaskListStore();

  const [isEditing, setIsEditing] = useState(false);
  const [formData, setFormData] = useState({
    title: taskList.title,
    description: taskList.description || '',
  });
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = async () => {
    if (!formData.title.trim()) {
      setError(t('tasklist.emptyTitle', 'Título não pode estar vazio'));
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      await updateTaskList(taskList.id, JSON.stringify({
        title: formData.title,
        description: formData.description,
      }));
      setIsEditing(false);
      onRefresh?.();
    } catch (err) {
      setError(String(err));
    } finally {
      setIsLoading(false);
    }
  };

  const handleCancel = () => {
    setFormData({
      title: taskList.title,
      description: taskList.description || '',
    });
    setError(null);
    setIsEditing(false);
  };

  // Calcula estatísticas
  const totalTasks = taskList.tasks?.length || 0;
  // Nota: Para calcular completedTasks, precisaríamos determinar qual ID de status é "completo"
  // Por enquanto, usamos 0 como padrão (implementar em iteração futura)
  const completedTasks = 0;
  const completionPercentage = totalTasks === 0 ? 0 : Math.round((completedTasks / totalTasks) * 100);

  if (isEditing) {
    return (
      <div className="tasklist-header tasklist-header-editing">
        {error && <div className="tasklist-header-error">{error}</div>}

        <div className="tasklist-header-edit-form">
          <div className="tasklist-header-form-group">
            <label htmlFor="header-title" className="tasklist-header-label">
              {t('tasklist.title', 'Título')}
            </label>
            <Input
              id="header-title"
              type="text"
              value={formData.title}
              onChange={(e) => setFormData({ ...formData, title: e.target.value })}
              placeholder={t('tasklist.titlePlaceholder', 'Título da lista')}
              disabled={isLoading}
              maxLength={200}
              autoFocus
            />
          </div>

          <div className="tasklist-header-form-group">
            <label htmlFor="header-description" className="tasklist-header-label">
              {t('tasklist.description', 'Descrição')}
            </label>
            <Textarea
              id="header-description"
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder={t('tasklist.descriptionPlaceholder', 'Descrição da lista (opcional)')}
              disabled={isLoading}
              rows={3}
              maxLength={500}
            />
          </div>

          <div className="tasklist-header-form-actions">
            <Button
              onClick={handleSave}
              variant="primary"
              disabled={isLoading}
            >
              {isLoading ? t('common.saving', 'Salvando...') : t('common.save', 'Salvar')}
            </Button>
            <Button
              onClick={handleCancel}
              variant="secondary"
              disabled={isLoading}
            >
              {t('common.cancel', 'Cancelar')}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="tasklist-header">
      <div className="tasklist-header-info">
        <div className="tasklist-header-title-section">
          <h1 className="tasklist-header-title">{taskList.title}</h1>
          <button
            className="tasklist-header-edit-btn"
            onClick={() => setIsEditing(true)}
            aria-label={t('tasklist.editTitle', 'Editar título')}
          >
            ✎
          </button>
        </div>

        {taskList.description && (
          <p className="tasklist-header-description">{taskList.description}</p>
        )}
      </div>

      <div className="tasklist-header-stats">
        <div className="tasklist-stat">
          <span className="tasklist-stat-label">{t('tasklist.totalTasks', 'Total de tarefas')}</span>
          <span className="tasklist-stat-value">{totalTasks}</span>
        </div>

        <div className="tasklist-stat">
          <span className="tasklist-stat-label">{t('tasklist.completed', 'Completas')}</span>
          <span className="tasklist-stat-value">
            {completedTasks} / {totalTasks}
          </span>
        </div>

        <div className="tasklist-stat">
          <span className="tasklist-stat-label">{t('tasklist.progress', 'Progresso')}</span>
          <div className="tasklist-progress-bar">
            <div className="tasklist-progress-fill" style={{ width: `${completionPercentage}%` }} />
            <span className="tasklist-progress-text">{completionPercentage}%</span>
          </div>
        </div>
      </div>
    </div>
  );
}
