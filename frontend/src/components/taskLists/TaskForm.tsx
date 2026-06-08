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
  taskListId: string;
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
  const { createTask, updateTaskFull } = useTaskListStore();

  const [formData, setFormData] = useState({
    title: task?.title || '',
    description: task?.description || '',
    code: task?.code || '',
    link: task?.link || '',
    assigneeName: task?.assigneeName || '',
    assigneeId: task?.assigneeId || '',
    creatorName: task?.creatorName || '',
    creatorId: task?.creatorId || '',
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
        await updateTaskFull(
          task.id,
          formData.title,
          formData.description || undefined,
          formData.code || undefined,
          formData.link || undefined,
          formData.assigneeName || undefined,
          formData.assigneeId || undefined,
          formData.creatorName || undefined,
          formData.creatorId || undefined,
        );
        onSuccess?.(task);
      } else {
        const newTask = await createTask(
          taskListId,
          formData.title,
          formData.description || undefined,
          formData.code || undefined,
          formData.link || undefined,
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
        label={t('tasklist.code', 'Código')}
        description={t('tasklist.codeDescription', 'Identificador externo ou código de ticket (opcional)')}
      >
        <Input
          type="text"
          value={formData.code}
          onChange={(e) => setFormData({ ...formData, code: e.target.value })}
          placeholder={t('tasklist.codePlaceholder', 'Ex: FSD-12345')}
          disabled={isLoading}
          maxLength={128}
        />
      </FormField>

      <FormField
        label={t('tasklist.link', 'Link')}
        description={t('tasklist.linkDescription', 'URL ou deep link associado à tarefa (opcional)')}
      >
        <Input
          type="text"
          value={formData.link}
          onChange={(e) => setFormData({ ...formData, link: e.target.value })}
          placeholder={t('tasklist.linkPlaceholder', 'Ex: assistente://conversation/open?id=123')}
          disabled={isLoading}
          maxLength={512}
        />
      </FormField>

      <FormField
        label={t('tasklist.assigneeName', 'Nome do Responsável')}
        description={t('tasklist.assigneeDescription', 'Quem está trabalhando nisso agora? (opcional)')}
      >
        <Input
          type="text"
          value={formData.assigneeName}
          onChange={(e) => setFormData({ ...formData, assigneeName: e.target.value })}
          placeholder={t('tasklist.assigneePlaceholder', 'Ex: João Silva')}
          disabled={isLoading}
          maxLength={200}
        />
      </FormField>

      <FormField
        label={t('tasklist.assigneeId', 'ID do Responsável')}
        description={t('tasklist.assigneeIdDescription', 'Identificador estável (e-mail, UUID, etc.) — opcional')}
      >
        <Input
          type="text"
          value={formData.assigneeId}
          onChange={(e) => setFormData({ ...formData, assigneeId: e.target.value })}
          placeholder={t('tasklist.assigneeIdPlaceholder', 'Ex: joao@empresa.com')}
          disabled={isLoading}
          maxLength={200}
        />
      </FormField>

      <FormField
        label={t('tasklist.creatorName', 'Criado por')}
        description={t('tasklist.creatorDescription', 'Quem criou/originou essa tarefa? (opcional)')}
      >
        <Input
          type="text"
          value={formData.creatorName}
          onChange={(e) => setFormData({ ...formData, creatorName: e.target.value })}
          placeholder={t('tasklist.creatorPlaceholder', 'Ex: Maria Santos')}
          disabled={isLoading}
          maxLength={200}
        />
      </FormField>

      <FormField
        label={t('tasklist.creatorId', 'ID do Criador')}
        description={t('tasklist.creatorIdDescription', 'Identificador estável do criador (e-mail, UUID, etc.) — opcional')}
      >
        <Input
          type="text"
          value={formData.creatorId}
          onChange={(e) => setFormData({ ...formData, creatorId: e.target.value })}
          placeholder={t('tasklist.creatorIdPlaceholder', 'Ex: maria@empresa.com')}
          disabled={isLoading}
          maxLength={200}
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
