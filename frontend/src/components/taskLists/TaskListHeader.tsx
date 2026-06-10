import { useEffect, useState } from 'react';
import { EditOutlined, MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useTaskListStore } from '../../store/taskListStore';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import { Select, type SelectOption } from '../ui/Select';
import { GetConversations } from '@wailsjs/go/app/App';
import type { database } from '@wailsjs/go/models';
import { openTaskLink } from '../../lib/deepLinks';
import { logger } from '../../utils/logger';
import type { TaskListWithWorkflow } from '../../types/tasklist';
import './TaskListHeader.css';

interface TaskListHeaderProps {
  taskList: TaskListWithWorkflow;
  onRefresh?: () => void;
}

export default function TaskListHeader({ taskList, onRefresh }: TaskListHeaderProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { updateTaskList, setTaskListConversation } = useTaskListStore();

  const [isEditing, setIsEditing] = useState(false);
  const [formData, setFormData] = useState({
    title: taskList.title,
    description: taskList.description || '',
    conversationId: taskList.conversationId || '',
  });
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [conversations, setConversations] = useState<database.Conversation[]>([]);

  useEffect(() => {
    if (!isEditing) return;
    let active = true;
    void (async () => {
      try {
        const result = await GetConversations();
        if (!active) return;
        const sorted = [...result].sort((a, b) => {
          const dateA = new Date(a.updatedAt as string | number | Date).getTime();
          const dateB = new Date(b.updatedAt as string | number | Date).getTime();
          return dateB - dateA;
        });
        setConversations(sorted);
      } catch (err) {
        logger.error('[TaskListHeader] erro ao carregar conversas:', err);
      }
    })();
    return () => {
      active = false;
    };
  }, [isEditing]);

  const conversationOptions: SelectOption[] = [
    { value: '', label: t('tasklist.conversationNone', 'Nenhuma') },
    ...conversations.map((c) => ({
      value: String(c.id),
      label: c.title || t('tasklist.conversationUntitled', 'Sem título'),
    })),
  ];
  if (
    formData.conversationId &&
    !conversationOptions.some((o) => o.value === formData.conversationId)
  ) {
    conversationOptions.push({ value: formData.conversationId, label: formData.conversationId });
  }

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
      if ((formData.conversationId || '') !== (taskList.conversationId || '')) {
        await setTaskListConversation(taskList.id, formData.conversationId || null);
      }
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
      conversationId: taskList.conversationId || '',
    });
    setError(null);
    setIsEditing(false);
  };

  const handleOpenConversation = () => {
    if (!taskList.conversationId) return;
    openTaskLink(`assistente://conversation/${taskList.conversationId}`, { navigate });
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

          <div className="tasklist-header-form-group">
            <label htmlFor="header-conversation" className="tasklist-header-label">
              {t('tasklist.conversation', 'Conversa vinculada')}
            </label>
            <Select
              id="header-conversation"
              fullWidth
              value={formData.conversationId}
              onChange={(e) => setFormData({ ...formData, conversationId: e.target.value })}
              disabled={isLoading}
              options={conversationOptions}
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
            <EditOutlined aria-hidden="true" />
          </button>
        </div>

        {taskList.description && (
          <p className="tasklist-header-description">{taskList.description}</p>
        )}

        {taskList.conversationId && (
          <button
            type="button"
            className="tasklist-header-conversation-link"
            onClick={handleOpenConversation}
            title={taskList.conversationId}
          >
            <MessageOutlined aria-hidden="true" /> {t('tasklist.conversation', 'Conversa vinculada')}
          </button>
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
