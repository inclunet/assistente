import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { Button } from '../ui/Button';
import { FormField } from '../ui/FormField';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import type { Task } from '../../types/tasklist';
import './TaskForm.css';

interface TaskFormProps {
  taskListId: number;
  task?: Task;
  onSuccess?: (task: Task) => void;
  onCancel?: () => void;
}

export default function TaskForm({
  taskListId,
  task,
  onSuccess,
  onCancel,
}: TaskFormProps) {
  const { t } = useTranslation();
  const { createTask, updateTask } = useTaskListStore();

  const [formData, setFormData] = useState({
    title: task?.title || '',
    description: task?.description || '',
    dueDate: task?.dueDate ? new Date(task.dueDate).toISOString().split('T')[0] : '',
  });

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!formData.title.trim()) {
      setError(t('tasklist.emptyTaskTitle', 'Título da tarefa é obrigatório'));
      return;
    }

    setIsLoading(true);
    try {
      if (task) {
        // Editar tarefa existente
        await updateTask(task.id, formData.title, formData.description || undefined);
        onSuccess?.(task);
      } else {
        // Criar nova tarefa
        const newTask = await createTask(
          taskListId,
          formData.title,
          formData.description || undefined,
        );
        if (newTask) {
          onSuccess?.(newTask);
        }
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setIsLoading(false);
    }
  };

  const handleCancel = () => {
    setError(null);
    onCancel?.();
  };

  return (
    <form className="task-form" onSubmit={handleSubmit}>
      {error && <div className="task-form-error">{error}</div>}

      <FormField
        label={t('tasklist.taskTitle', 'Título')}
        required
        description={t('tasklist.taskTitleDescription', 'Resumo breve da tarefa')}
      >
        <Input
          type="text"
          value={formData.title}
          onChange={(e) => setFormData({ ...formData, title: e.target.value })}
          placeholder={t('tasklist.taskTitlePlaceholder', 'O que precisa ser feito?')}
          disabled={isLoading}
          maxLength={200}
        />
      </FormField>

      <FormField
        label={t('tasklist.description', 'Descrição')}
        description={t('tasklist.descriptionDescription', 'Detalhes adicionais sobre a tarefa (opcional)')}
      >
        <Textarea
          value={formData.description}
          onChange={(e) => setFormData({ ...formData, description: e.target.value })}
          placeholder={t('tasklist.descriptionPlaceholder', 'Adicione mais detalhes...')}
          disabled={isLoading}
          rows={4}
          maxLength={1000}
        />
      </FormField>

      <FormField
        label={t('tasklist.dueDate', 'Data de Vencimento')}
        description={t('tasklist.dueDateDescription', 'Quando essa tarefa vence? (opcional)')}
      >
        <Input
          type="date"
          value={formData.dueDate}
          onChange={(e) => setFormData({ ...formData, dueDate: e.target.value })}
          disabled={isLoading}
        />
      </FormField>

      <div className="task-form-actions">
        <Button type="submit" variant="primary" disabled={isLoading}>
          {isLoading
            ? t('common.loading', 'Salvando...')
            : task
              ? t('common.save', 'Salvar')
              : t('tasklist.create', 'Criar')}
        </Button>
        <Button type="button" variant="secondary" onClick={handleCancel} disabled={isLoading}>
          {t('common.cancel', 'Cancelar')}
        </Button>
      </div>
    </form>
  );
}
