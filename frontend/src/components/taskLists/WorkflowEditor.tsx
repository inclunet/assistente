import { useState, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../ui/Button';
import type {
  TaskListWorkflowStatus,
  WorkflowTransitions,
  TaskListWorkflow,
} from '../../types/tasklist';
import './WorkflowEditor.css';

/**
 * Presets de cor para status definidos exclusivamente como tokens do tema
 * (`theme.css`). Cada token tem par claro/escuro com contraste validado por tema,
 * então as cores funcionam em todos os temas sem hex hardcoded. O nome é
 * internacionalizado para servir de rótulo acessível dos botões (icon-only).
 */
const COLOR_PRESETS: { token: string; nameKey: string }[] = [
  { token: 'var(--color-info)', nameKey: 'tasklist.workflow.color.blue' },
  { token: 'var(--color-success)', nameKey: 'tasklist.workflow.color.green' },
  { token: 'var(--color-warning)', nameKey: 'tasklist.workflow.color.amber' },
  { token: 'var(--color-danger)', nameKey: 'tasklist.workflow.color.red' },
  { token: 'var(--accent)', nameKey: 'tasklist.workflow.color.purple' },
  { token: 'var(--text-muted)', nameKey: 'tasklist.workflow.color.gray' },
];

interface WorkflowEditorProps {
  workflow: TaskListWorkflow;
  taskCountsByStatus?: Record<number, number>;
  onSave: (
    statuses: TaskListWorkflowStatus[],
    transitions: WorkflowTransitions,
    initialStatusId: number,
    statusMigration: Record<number, number>,
  ) => Promise<void>;
  onCancel: () => void;
}

export default function WorkflowEditor({
  workflow,
  taskCountsByStatus = {},
  onSave,
  onCancel,
}: WorkflowEditorProps) {
  const { t } = useTranslation();

  const [statuses, setStatuses] = useState<TaskListWorkflowStatus[]>(
    () => [...workflow.statuses].sort((a, b) => a.order - b.order),
  );
  const [transitions, setTransitions] = useState<WorkflowTransitions>(
    () => ({ ...workflow.allowedTransitions }),
  );
  const [initialStatusId, setInitialStatusId] = useState(workflow.initialStatusId);
  const [statusMigration, setStatusMigration] = useState<Record<number, number>>({});
  const [removedStatuses, setRemovedStatuses] = useState<TaskListWorkflowStatus[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  const nextId = useCallback(() => {
    const allIds = [...statuses, ...removedStatuses].map(s => s.id);
    const maxId = allIds.reduce((max, id) => Math.max(max, id), 0);
    return maxId + 1;
  }, [statuses, removedStatuses]);

  const handleAddStatus = useCallback(() => {
    const newId = nextId();
    setStatuses(prev => [
      ...prev,
      {
        id: newId,
        order: prev.length,
        label: '',
        color: COLOR_PRESETS[prev.length % COLOR_PRESETS.length].token,
        icon: '⬜',
      },
    ]);
  }, [nextId]);

  const handleRemoveStatus = useCallback((statusId: number) => {
    const status = statuses.find(s => s.id === statusId);
    if (!status) return;

    const count = taskCountsByStatus[statusId] ?? 0;
    if (count > 0) {
      setRemovedStatuses(prev => [...prev, status]);
    }

    setStatuses(prev => prev.filter(s => s.id !== statusId).map((s, i) => ({ ...s, order: i })));

    setTransitions(prev => {
      const updated = { ...prev };
      delete updated[statusId];
      for (const [key, targets] of Object.entries(updated)) {
        updated[Number(key)] = targets.filter(id => id !== statusId);
      }
      return updated;
    });

    if (initialStatusId === statusId) {
      setInitialStatusId(prev => {
        const remaining = statuses.filter(s => s.id !== statusId);
        return remaining.length > 0 ? remaining[0].id : prev;
      });
    }
  }, [statuses, taskCountsByStatus, initialStatusId]);

  const handleUpdateStatus = useCallback((statusId: number, field: keyof TaskListWorkflowStatus, value: string | number) => {
    setStatuses(prev =>
      prev.map(s => (s.id === statusId ? { ...s, [field]: value } : s)),
    );
  }, []);

  const handleMoveStatus = useCallback((statusId: number, direction: -1 | 1) => {
    setStatuses(prev => {
      const idx = prev.findIndex(s => s.id === statusId);
      if (idx < 0) return prev;
      const newIdx = idx + direction;
      if (newIdx < 0 || newIdx >= prev.length) return prev;
      const updated = [...prev];
      [updated[idx], updated[newIdx]] = [updated[newIdx], updated[idx]];
      return updated.map((s, i) => ({ ...s, order: i }));
    });
  }, []);

  const handleToggleTransition = useCallback((fromId: number, toId: number) => {
    setTransitions(prev => {
      const current = prev[fromId] || [];
      const has = current.includes(toId);
      return {
        ...prev,
        [fromId]: has ? current.filter(id => id !== toId) : [...current, toId],
      };
    });
  }, []);

  const handleMigrationChange = useCallback((oldId: number, newId: number) => {
    setStatusMigration(prev => ({ ...prev, [oldId]: newId }));
  }, []);

  const removedWithTasks = useMemo(() =>
    removedStatuses.filter(s => (taskCountsByStatus[s.id] ?? 0) > 0),
    [removedStatuses, taskCountsByStatus],
  );

  const validate = useCallback((): string | null => {
    if (statuses.length === 0) {
      return t('tasklist.workflow.emptyStatuses', 'Adicione pelo menos um status');
    }

    const ids = new Set<number>();
    for (const s of statuses) {
      if (ids.has(s.id)) {
        return t('tasklist.workflow.duplicateId', 'IDs de status devem ser únicos');
      }
      ids.add(s.id);
      if (!s.label.trim()) {
        return `Status ID ${s.id}: nome não pode estar vazio`;
      }
    }

    if (!ids.has(initialStatusId)) {
      return t('tasklist.workflow.invalidInitialStatus', 'Status inicial deve ser um dos statuses definidos');
    }

    for (const removed of removedWithTasks) {
      if (!statusMigration[removed.id] || !ids.has(statusMigration[removed.id])) {
        return t('tasklist.workflow.migrationRequired', 'Tarefas precisam ser migradas antes de remover o status');
      }
    }

    return null;
  }, [statuses, initialStatusId, removedWithTasks, statusMigration, t]);

  const handleSave = useCallback(async () => {
    const validationError = validate();
    if (validationError) {
      setError(validationError);
      return;
    }

    setError(null);
    setIsSaving(true);

    try {
      const cleanTransitions: WorkflowTransitions = {};
      for (const s of statuses) {
        cleanTransitions[s.id] = transitions[s.id] || [];
      }

      const migration: Record<number, number> = {};
      for (const removed of removedWithTasks) {
        if (statusMigration[removed.id]) {
          migration[removed.id] = statusMigration[removed.id];
        }
      }

      await onSave(statuses, cleanTransitions, initialStatusId, migration);
    } catch (err) {
      setError(String(err));
    } finally {
      setIsSaving(false);
    }
  }, [validate, statuses, transitions, initialStatusId, removedWithTasks, statusMigration, onSave]);

  return (
    <div className="workflow-editor">
      {error && <div className="workflow-editor-error">{error}</div>}

      {/* Statuses Section */}
      <div className="workflow-section">
        <div className="workflow-section-header">
          <h3 className="workflow-section-title">{t('tasklist.workflow.statuses', 'Statuses')}</h3>
          <Button variant="secondary" onClick={handleAddStatus} disabled={isSaving}>
            {t('tasklist.workflow.addStatus', 'Adicionar Status')}
          </Button>
        </div>

        <div className="workflow-status-list">
          {statuses.map((status, idx) => (
            <div
              key={status.id}
              className={`workflow-status-item ${status.id === initialStatusId ? 'workflow-status-item--initial' : ''}`}
            >
              <span className="workflow-status-id">#{status.id}</span>

              <input
                className="workflow-status-label-input"
                type="text"
                value={status.label}
                onChange={(e) => handleUpdateStatus(status.id, 'label', e.target.value)}
                placeholder={t('tasklist.workflow.statusLabelPlaceholder', 'Ex: Em Andamento')}
                disabled={isSaving}
                maxLength={50}
              />

              <div
                className="workflow-color-presets"
                role="group"
                aria-label={t('tasklist.workflow.colorGroup', 'Cor do status')}
              >
                {COLOR_PRESETS.map(({ token, nameKey }) => {
                  const name = t(nameKey);
                  const isActive = status.color === token;
                  return (
                    <button
                      key={token}
                      className={`workflow-color-preset ${isActive ? 'workflow-color-preset--active' : ''}`}
                      style={{ backgroundColor: token }}
                      onClick={() => handleUpdateStatus(status.id, 'color', token)}
                      title={name}
                      aria-label={name}
                      aria-pressed={isActive}
                      type="button"
                      disabled={isSaving}
                    />
                  );
                })}
              </div>

              <input
                className="workflow-status-icon-input"
                type="text"
                value={status.icon}
                onChange={(e) => handleUpdateStatus(status.id, 'icon', e.target.value)}
                placeholder={t('tasklist.workflow.statusIconPlaceholder', '⏳')}
                disabled={isSaving}
                maxLength={4}
              />

              <div className="workflow-status-actions">
                <button
                  className={`workflow-status-action-btn ${status.id === initialStatusId ? 'workflow-status-action-btn--initial' : ''}`}
                  onClick={() => setInitialStatusId(status.id)}
                  title={t('tasklist.workflow.initialStatus', 'Status Inicial')}
                  type="button"
                  disabled={isSaving}
                >
                  {status.id === initialStatusId ? '★' : '☆'}
                </button>
                <button
                  className="workflow-status-action-btn"
                  onClick={() => handleMoveStatus(status.id, -1)}
                  disabled={idx === 0 || isSaving}
                  title={t('tasklist.workflow.moveUp', 'Mover acima')}
                  type="button"
                >
                  ▲
                </button>
                <button
                  className="workflow-status-action-btn"
                  onClick={() => handleMoveStatus(status.id, 1)}
                  disabled={idx === statuses.length - 1 || isSaving}
                  title={t('tasklist.workflow.moveDown', 'Mover abaixo')}
                  type="button"
                >
                  ▼
                </button>
                <button
                  className="workflow-status-action-btn workflow-status-action-btn--danger"
                  onClick={() => handleRemoveStatus(status.id)}
                  disabled={statuses.length <= 1 || isSaving}
                  title={t('tasklist.workflow.removeStatus', 'Remover Status')}
                  type="button"
                >
                  ✕
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Migration Warnings */}
      {removedWithTasks.length > 0 && (
        <div className="workflow-section">
          {removedWithTasks.map((removed) => (
            <div key={removed.id} className="workflow-migration-warning">
              <strong>
                {t('tasklist.workflow.tasksUsingStatus', { count: taskCountsByStatus[removed.id] ?? 0 })}
              </strong>
              {' — '}{removed.icon} {removed.label} (ID: {removed.id})
              <div className="workflow-migration-row">
                <span>{t('tasklist.workflow.migrateTasksTo', 'Migrar tarefas para')}:</span>
                <select
                  className="workflow-migration-select"
                  value={statusMigration[removed.id] || ''}
                  onChange={(e) => handleMigrationChange(removed.id, Number(e.target.value))}
                >
                  <option value="">—</option>
                  {statuses.map((s) => (
                    <option key={s.id} value={s.id}>{s.icon} {s.label}</option>
                  ))}
                </select>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Initial Status */}
      <div className="workflow-section">
        <div className="workflow-section-header">
          <h3 className="workflow-section-title">{t('tasklist.workflow.initialStatus', 'Status Inicial')}</h3>
        </div>
        <div className="workflow-initial-status">
          <select
            value={initialStatusId}
            onChange={(e) => setInitialStatusId(Number(e.target.value))}
            disabled={isSaving}
          >
            {statuses.map((s) => (
              <option key={s.id} value={s.id}>{s.icon} {s.label}</option>
            ))}
          </select>
        </div>
      </div>

      {/* Transitions Section */}
      <div className="workflow-section">
        <div className="workflow-section-header">
          <h3 className="workflow-section-title">{t('tasklist.workflow.transitions', 'Transições')}</h3>
        </div>

        <div className="workflow-transitions-grid">
          {statuses.map((fromStatus) => (
            <div key={fromStatus.id} className="workflow-transition-row">
              <span className="workflow-transition-from">
                {fromStatus.icon} {fromStatus.label}
              </span>
              <span className="workflow-transition-arrow">→</span>
              <div className="workflow-transition-targets">
                {statuses
                  .filter(s => s.id !== fromStatus.id)
                  .map((toStatus) => {
                    const isActive = (transitions[fromStatus.id] || []).includes(toStatus.id);
                    return (
                      <button
                        key={toStatus.id}
                        className={`workflow-transition-chip ${isActive ? 'workflow-transition-chip--active' : ''}`}
                        onClick={() => handleToggleTransition(fromStatus.id, toStatus.id)}
                        disabled={isSaving}
                        type="button"
                      >
                        {toStatus.icon} {toStatus.label}
                      </button>
                    );
                  })}
                {statuses.length <= 1 && (
                  <span style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                    {t('tasklist.workflow.noTransitions', 'Sem transições')}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Actions */}
      <div className="workflow-editor-actions">
        <Button variant="secondary" onClick={onCancel} disabled={isSaving}>
          {t('tasklist.workflow.cancel', 'Cancelar')}
        </Button>
        <Button variant="primary" onClick={handleSave} disabled={isSaving}>
          {isSaving ? t('common.saving', 'Salvando...') : t('tasklist.workflow.save', 'Salvar Workflow')}
        </Button>
      </div>
    </div>
  );
}
