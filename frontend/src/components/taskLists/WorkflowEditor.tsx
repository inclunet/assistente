import { useState, useCallback, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { Toolbar } from '../ui/Toolbar';
import { DataGrid, type DataGridColumn } from '../ui/DataGrid';
import { MenuButton } from '../layout/MenuButton';
import { Modal } from '../ui/Modal';
import { FormField } from '../ui/FormField';
import { Input } from '../ui/Input';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { useGridFocus } from '../../hooks/useGridFocus';
import { useUIStore } from '../../store/uiStore';
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

function colorName(
  t: (key: string, fallback: string) => string,
  token: string,
): string {
  const preset = COLOR_PRESETS.find((p) => p.token === token);
  return preset ? t(preset.nameKey, preset.token) : token;
}

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

interface StatusDraft {
  label: string;
  icon: string;
  color: string;
}

function emptyDraft(colorToken: string): StatusDraft {
  return { label: '', icon: '⬜', color: colorToken };
}

export default function WorkflowEditor({
  workflow,
  taskCountsByStatus = {},
  onSave,
  onCancel,
}: WorkflowEditorProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const addToast = useUIStore((s) => s.addToast);
  const requestConfirm = useConfirm();
  const { handleGridReady, requestGridFocus } = useGridFocus();

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
  const [focused, setFocused] = useState<TaskListWorkflowStatus | null>(null);

  // Modal de edição por status: 'create' parte do vazio, 'edit' do focado.
  const [itemModal, setItemModal] = useState<{ mode: 'create' } | { mode: 'edit'; id: number } | null>(null);
  const [draft, setDraft] = useState<StatusDraft>(() => emptyDraft(COLOR_PRESETS[0].token));
  const newButtonRef = useRef<HTMLButtonElement | null>(null);

  const nextId = useCallback(() => {
    const allIds = [...statuses, ...removedStatuses].map(s => s.id);
    const maxId = allIds.reduce((max, id) => Math.max(max, id), 0);
    return maxId + 1;
  }, [statuses, removedStatuses]);

  const openNewStatus = useCallback(() => {
    setDraft(emptyDraft(COLOR_PRESETS[statuses.length % COLOR_PRESETS.length].token));
    setItemModal({ mode: 'create' });
  }, [statuses.length]);

  const openEditStatus = useCallback((status: TaskListWorkflowStatus) => {
    setDraft({ label: status.label, icon: status.icon, color: status.color });
    setItemModal({ mode: 'edit', id: status.id });
  }, []);

  const closeItemModal = useCallback(() => {
    setItemModal(null);
    // Volta o foco ao grid para seguir editando em série por teclado.
    requestAnimationFrame(() => { requestGridFocus(); });
  }, [requestGridFocus]);

  const confirmItemModal = useCallback(() => {
    const label = draft.label.trim();
    if (!label) {
      const msg = t('tasklist.workflow.emptyStatusName', 'Dê um nome ao status');
      addToast(msg, 'error');
      announce(msg);
      return;
    }
    if (itemModal?.mode === 'edit') {
      setStatuses(prev => prev.map(s => (s.id === itemModal.id ? { ...s, ...draft, label } : s)));
      announce(t('tasklist.workflow.statusUpdated', 'Status atualizado'));
    } else {
      const newStatus: TaskListWorkflowStatus = {
        id: nextId(),
        order: statuses.length,
        label,
        color: draft.color,
        icon: draft.icon.trim() || '⬜',
      };
      setStatuses(prev => [...prev, newStatus]);
      setFocused(newStatus);
      announce(t('tasklist.workflow.statusAdded', 'Status adicionado'));
    }
    setError(null);
    closeItemModal();
  }, [draft, itemModal, nextId, statuses.length, t, addToast, announce, closeItemModal]);

  const deleteStatus = useCallback(async (status: TaskListWorkflowStatus) => {
    const count = taskCountsByStatus[status.id] ?? 0;
    const confirmed = await requestConfirm({
      title: t('tasklist.workflow.removeStatus', 'Remover Status'),
      message: count > 0
        ? t('tasklist.workflow.statusInUse', 'Status "{{label}}" (ID: {{id}}) está em uso por {{count}} tarefa(s)', {
          label: status.label || `#${status.id}`,
          id: String(status.id),
          count,
        })
        : t('tasklist.workflow.confirmRemoveStatus', 'Remover status "{{label}}"?', {
          label: status.label || `#${status.id}`,
        }),
    });
    if (!confirmed) return;

    if (count > 0) {
      setRemovedStatuses(prev => [...prev, status]);
    }
    setStatuses(prev => prev.filter(s => s.id !== status.id).map((s, i) => ({ ...s, order: i })));
    setFocused(prev => (prev?.id === status.id ? null : prev));

    setTransitions(prev => {
      const updated = { ...prev };
      delete updated[status.id];
      for (const [key, targets] of Object.entries(updated)) {
        updated[Number(key)] = targets.filter(id => id !== status.id);
      }
      return updated;
    });

    if (initialStatusId === status.id) {
      const remaining = statuses.filter(s => s.id !== status.id);
      if (remaining.length > 0) setInitialStatusId(remaining[0].id);
    }
    announce(t('tasklist.workflow.statusRemoved', 'Status removido'));
    // A linha some e o foco cairia no body: devolve ao grid (ou ao Novo,
    // se a trava de mínimo impedir — nesse caso nada muda).
    requestAnimationFrame(() => {
      if (!requestGridFocus()) newButtonRef.current?.focus();
    });
  }, [statuses, taskCountsByStatus, initialStatusId, requestConfirm, t, announce, requestGridFocus]);

  const handleMoveStatus = useCallback((fromIndex: number, toIndex: number) => {
    setStatuses(prev => {
      if (toIndex < 0 || toIndex >= prev.length) return prev;
      const updated = [...prev];
      [updated[fromIndex], updated[toIndex]] = [updated[toIndex], updated[fromIndex]];
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
        return t('tasklist.workflow.emptyStatusName', 'Status ID {{id}}: nome não pode estar vazio', { id: String(s.id) });
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
      announce(validationError);
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
  }, [validate, statuses, transitions, initialStatusId, removedWithTasks, statusMigration, onSave, announce]);

  const getRowActions = useCallback((status: TaskListWorkflowStatus) => [
    {
      id: 'edit',
      label: t('tasklist.edit', 'Editar'),
      icon: <EditOutlined aria-hidden="true" />,
      onClick: () => openEditStatus(status),
      disabled: isSaving,
    },
    {
      id: 'delete',
      label: t('tasklist.delete', 'Deletar'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => void deleteStatus(status),
      danger: true,
      disabled: isSaving || statuses.length <= 1,
    },
  ], [t, openEditStatus, deleteStatus, isSaving, statuses.length]);

  const columns: DataGridColumn<TaskListWorkflowStatus>[] = useMemo(() => [
    {
      key: 'id',
      label: t('tasklist.workflow.statusId', 'ID'),
      width: '10%',
      format: (_value, item) => <code className="workflow-editor__mono">#{item.id}</code>,
    },
    {
      key: 'label',
      label: t('tasklist.workflow.statusLabel', 'Nome'),
      width: '30%',
      format: (_value, item) => (
        <span>{item.icon ? <span aria-hidden="true">{item.icon} </span> : null}{item.label}</span>
      ),
    },
    {
      key: 'color',
      label: t('tasklist.workflow.statusColor', 'Cor'),
      width: '20%',
      format: (_value, item) => colorName((k, f) => t(k, f), item.color),
    },
    {
      key: 'icon',
      label: t('tasklist.workflow.statusIcon', 'Ícone'),
      width: '10%',
      format: (_value, item) => item.icon || '—',
    },
    {
      key: 'initial',
      label: t('tasklist.workflow.initialStatus', 'Status Inicial'),
      width: '15%',
      format: (_value, item) => (item.id === initialStatusId
        ? t('common.yes', 'Sim')
        : t('common.no', 'Não')),
    },
    {
      key: 'actions',
      label: '',
      width: '5%',
      format: (_value, item) => (
        <MenuButton
          items={getRowActions(item)}
          buttonLabel={t('common.actions', 'Ações')}
        />
      ),
    },
  ], [t, getRowActions, initialStatusId]);

  return (
    <div className="workflow-editor">
      {error && <div className="workflow-editor-error">{error}</div>}

      {/* Statuses Section */}
      <div className="workflow-section">
        <div className="workflow-section-header">
          <h3 className="workflow-section-title">{t('tasklist.workflow.statuses', 'Statuses')}</h3>
        </div>

        <Toolbar
          ariaLabel={t('tasklist.workflow.statusToolbar', 'Barra de ferramentas de status')}
          actions={[
            {
              key: 'new-status',
              label: t('tasklist.workflow.addStatus', 'Adicionar Status'),
              icon: <PlusOutlined aria-hidden="true" />,
              onClick: openNewStatus,
              variant: 'primary',
              buttonRef: newButtonRef,
              disabled: isSaving,
            },
            {
              key: 'edit-status',
              label: t('tasklist.edit', 'Editar'),
              icon: <EditOutlined aria-hidden="true" />,
              onClick: () => focused && openEditStatus(focused),
              disabled: !focused || isSaving,
            },
            {
              key: 'delete-status',
              label: t('tasklist.delete', 'Deletar'),
              icon: <DeleteOutlined aria-hidden="true" />,
              onClick: () => focused && void deleteStatus(focused),
              disabled: !focused || isSaving || statuses.length <= 1,
              variant: 'danger',
            },
          ]}
        />

        <DataGrid
          items={statuses}
          columns={columns}
          getItemId={(item) => item.id}
          label={t('tasklist.workflow.statusGrid', 'Lista de status do workflow')}
          autoFocusOnMount={false}
          onFocusChange={(item) => setFocused(item)}
          onActivate={(item) => openEditStatus(item)}
          onMoveItem={handleMoveStatus}
          getRowActions={getRowActions}
          onGridReady={handleGridReady}
        />
        <p className="workflow-editor__hint">
          {t('tasklist.workflow.moveHint', 'Use Alt+Setas para reordenar o status focado.')}
        </p>
      </div>

      {/* Migration Warnings */}
      {removedWithTasks.length > 0 && (
        <div className="workflow-section">
          {removedWithTasks.map((removed) => (
            <div key={removed.id} className="workflow-migration-warning" role="group" aria-label={t('tasklist.workflow.migrationGroup', 'Migração do status {{label}}', { label: removed.label || `#${removed.id}` })}>
              <strong>
                {t('tasklist.workflow.tasksUsingStatus', '{{count}} tarefa(s) usando este status', { count: taskCountsByStatus[removed.id] ?? 0 })}
              </strong>
              {' — '}{removed.icon} {removed.label} (ID: {removed.id})
              <div className="workflow-migration-row">
                <span id={`migrate-label-${removed.id}`}>{t('tasklist.workflow.migrateTasksTo', 'Migrar tarefas para')}:</span>
                <select
                  className="workflow-migration-select"
                  value={statusMigration[removed.id] || ''}
                  onChange={(e) => handleMigrationChange(removed.id, Number(e.target.value))}
                  aria-labelledby={`migrate-label-${removed.id}`}
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
            aria-label={t('tasklist.workflow.initialStatus', 'Status Inicial')}
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
            <div
              key={fromStatus.id}
              className="workflow-transition-row"
              role="group"
              aria-label={t('tasklist.workflow.transitionsFrom', 'Transições de {{label}}', {
                label: fromStatus.label.trim() || `#${fromStatus.id}`,
              })}
            >
              <span className="workflow-transition-from" aria-hidden="true">
                {fromStatus.icon} {fromStatus.label}
              </span>
              <span className="workflow-transition-arrow" aria-hidden="true">→</span>
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
                        aria-pressed={isActive}
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

      {/* Actions — AEP-0090: primária antes de cancelar */}
      <DialogActions
        className="workflow-editor-actions"
        primary={
          <Button variant="primary" onClick={handleSave} disabled={isSaving}>
            {isSaving ? t('common.saving', 'Salvando...') : t('tasklist.workflow.save', 'Salvar Workflow')}
          </Button>
        }
        secondary={
          <Button variant="secondary" onClick={onCancel} disabled={isSaving}>
            {t('tasklist.workflow.cancel', 'Cancelar')}
          </Button>
        }
      />

      <Modal
        isOpen={itemModal !== null}
        onClose={closeItemModal}
        title={itemModal?.mode === 'edit'
          ? t('tasklist.workflow.editStatus', 'Editar status')
          : t('tasklist.workflow.newStatus', 'Novo status')}
      >
        <div className="workflow-status-form">
          <FormField label={t('tasklist.workflow.statusLabel', 'Nome')} required>
            <Input
              type="text"
              value={draft.label}
              placeholder={t('tasklist.workflow.statusLabelPlaceholder', 'Ex: Em Andamento')}
              onChange={(e) => setDraft((prev) => ({ ...prev, label: e.target.value }))}
              maxLength={50}
            />
          </FormField>
          <FormField label={t('tasklist.workflow.statusIcon', 'Ícone')}>
            <Input
              type="text"
              value={draft.icon}
              placeholder={t('tasklist.workflow.statusIconPlaceholder', 'Ex: ⏳')}
              onChange={(e) => setDraft((prev) => ({ ...prev, icon: e.target.value }))}
              maxLength={4}
            />
          </FormField>
          <FormField label={t('tasklist.workflow.statusColor', 'Cor')}>
            <div
              className="workflow-color-presets"
              role="group"
              aria-label={t('tasklist.workflow.colorGroup', 'Cor do status')}
            >
              {COLOR_PRESETS.map(({ token, nameKey }) => {
                const name = t(nameKey, token);
                const isActive = draft.color === token;
                return (
                  <button
                    key={token}
                    className={`workflow-color-preset ${isActive ? 'workflow-color-preset--active' : ''}`}
                    style={{ backgroundColor: token }}
                    onClick={() => setDraft((prev) => ({ ...prev, color: token }))}
                    title={name}
                    aria-label={name}
                    aria-pressed={isActive}
                    type="button"
                  />
                );
              })}
            </div>
          </FormField>
          <DialogActions
            primary={
              <Button type="button" variant="primary" onClick={confirmItemModal}>
                {t('tasklist.customActions.apply', 'Aplicar')}
              </Button>
            }
            secondary={
              <Button type="button" variant="secondary" onClick={closeItemModal}>
                {t('common.cancel', 'Cancelar')}
              </Button>
            }
          />
        </div>
      </Modal>
    </div>
  );
}
