import { useState, useCallback, useEffect, useId, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { Toolbar } from '../ui/Toolbar';
import { DataGrid, type DataGridColumn } from '../ui/DataGrid';
import { MenuButton } from '../layout/MenuButton';
import { Modal } from '../ui/Modal';
import { DecisionDialog } from '../ui/DecisionDialog';
import { FormField } from '../ui/FormField';
import { Input } from '../ui/Input';
import { Checkbox } from '../ui/Checkbox';
import { Select } from '../ui/Select';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { useGridFocus } from '../../hooks/useGridFocus';
import { useInitialContentFocus } from '../../hooks/useInitialContentFocus';
import { useNewItemShortcut } from '../../hooks/useNewItemShortcut';
import { enqueueSave } from '../../lib/serialSaveQueue';
import { isTaskListConfigConflict } from '../../lib/taskListConfigConflict';
import { useUIStore } from '../../store/uiStore';
import type {
  TaskListWorkflowStatus,
  TaskListWorkflowSnapshot,
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
  /**
   * Persiste o workflow completo; chamado a cada alteração. `expected` é o
   * workflow sobre o qual a alteração foi calculada: o backend recusa com
   * conflito se o gravado já for outro.
   */
  onSave: (
    statuses: TaskListWorkflowStatus[],
    transitions: WorkflowTransitions,
    initialStatusId: number,
    statusMigration: Record<number, number>,
    expected: TaskListWorkflowSnapshot,
  ) => Promise<void>;
  /**
   * Chamado quando o backend recusa uma gravação por conflito. Quem abre o
   * editor recarrega o workflow e incrementa `syncToken`.
   */
  onConflict?: () => void;
  /** Ao mudar, a tela passa a refletir `workflow` e `taskCountsByStatus`. */
  syncToken?: number;
  /** Chave da fila de salvamentos compartilhada (ver `serialSaveQueue`). */
  saveQueueKey?: string;
}

interface StatusDraft {
  label: string;
  icon: string;
  color: string;
  transitions: number[];
  initial: boolean;
}

interface WorkflowState {
  statuses: TaskListWorkflowStatus[];
  transitions: WorkflowTransitions;
  initialStatusId: number;
}

/**
 * Alteração do workflow expressa por IDs, para valer tanto sobre o que a tela
 * mostra quanto sobre o último estado aceito pelo backend.
 */
type WorkflowChange = (base: WorkflowState) => WorkflowState;

function addStatusChange(created: TaskListWorkflowStatus, targets: number[], initial: boolean): WorkflowChange {
  return (base) => {
    if (base.statuses.some(s => s.id === created.id)) return base;
    return {
      statuses: [...base.statuses, { ...created, order: base.statuses.length }],
      transitions: { ...base.transitions, [created.id]: [...targets] },
      initialStatusId: initial ? created.id : base.initialStatusId,
    };
  };
}

function editStatusChange(
  id: number,
  fields: Pick<TaskListWorkflowStatus, 'label' | 'icon' | 'color'>,
  targets: number[],
  initial: boolean,
): WorkflowChange {
  return (base) => {
    if (!base.statuses.some(s => s.id === id)) return base;
    let initialStatusId = base.initialStatusId;
    if (initial) {
      initialStatusId = id;
    } else if (initialStatusId === id) {
      initialStatusId = base.statuses.find(s => s.id !== id)?.id ?? id;
    }
    return {
      statuses: base.statuses.map(s => (s.id === id ? { ...s, ...fields } : s)),
      transitions: { ...base.transitions, [id]: targets.filter(target => base.statuses.some(s => s.id === target)) },
      initialStatusId,
    };
  };
}

function removeStatusChange(id: number): WorkflowChange {
  return (base) => {
    const transitions: WorkflowTransitions = {};
    for (const [key, targets] of Object.entries(base.transitions)) {
      if (Number(key) === id) continue;
      transitions[Number(key)] = targets.filter(target => target !== id);
    }
    const remaining = base.statuses.filter(s => s.id !== id).map((s, i) => ({ ...s, order: i }));
    return {
      statuses: remaining,
      transitions,
      initialStatusId: base.initialStatusId === id ? (remaining[0]?.id ?? id) : base.initialStatusId,
    };
  };
}

function swapStatusesChange(aId: number, bId: number): WorkflowChange {
  return (base) => {
    const a = base.statuses.findIndex(s => s.id === aId);
    const b = base.statuses.findIndex(s => s.id === bId);
    if (a < 0 || b < 0) return base;
    const reordered = [...base.statuses];
    [reordered[a], reordered[b]] = [reordered[b], reordered[a]];
    return { ...base, statuses: reordered.map((s, i) => ({ ...s, order: i })) };
  };
}

/**
 * Maior ID de status já visto por fila (tasklist), além da vida do editor e
 * compartilhado entre editores montados: IDs nunca são reaproveitados na
 * sessão, nem os de status já removidos.
 */
const maxStatusIdByQueue = new Map<string, number>();

function observeStatusIds(queueKey: string, ids: number[]): number {
  const max = Math.max(maxStatusIdByQueue.get(queueKey) ?? 0, ...ids, 0);
  maxStatusIdByQueue.set(queueKey, max);
  return max;
}

function workflowState(workflow: TaskListWorkflow): WorkflowState {
  return {
    statuses: [...workflow.statuses].sort((a, b) => a.order - b.order),
    transitions: { ...workflow.allowedTransitions },
    initialStatusId: workflow.initialStatusId,
  };
}

function emptyDraft(colorToken: string): StatusDraft {
  return { label: '', icon: '⬜', color: colorToken, transitions: [], initial: false };
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error ?? '');
}

export default function WorkflowEditor({
  workflow,
  taskCountsByStatus = {},
  onSave,
  onConflict,
  syncToken,
  saveQueueKey,
}: WorkflowEditorProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const addToast = useUIStore((s) => s.addToast);
  const requestConfirm = useConfirm();
  const { handleGridReady, requestGridFocus } = useGridFocus();

  const [wf, setWf] = useState<WorkflowState>(() => workflowState(workflow));
  const { statuses, transitions, initialStatusId } = wf;
  const [counts, setCounts] = useState<Record<number, number>>(() => ({ ...taskCountsByStatus }));
  // Um Aplicar/Remover por vez: cliques repetidos não duplicam o status.
  const applyingRef = useRef(false);
  const [focused, setFocused] = useState<TaskListWorkflowStatus | null>(null);

  // Modal de edição por status: 'create' parte do vazio, 'edit' do focado.
  const [itemModal, setItemModal] = useState<{ mode: 'create' } | { mode: 'edit'; id: number } | null>(null);
  const [draft, setDraft] = useState<StatusDraft>(() => emptyDraft(COLOR_PRESETS[0].token));
  // Remoção de status em uso: pergunta para onde migrar as tarefas.
  const [migration, setMigration] = useState<{ status: TaskListWorkflowStatus; targetId: number } | null>(null);
  const newButtonRef = useRef<HTMLButtonElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const instanceQueueId = useId();
  const queueKey = saveQueueKey ?? `workflow-editor:${instanceQueueId}`;

  // Registra os IDs do workflow aberto: se um deles for removido e o editor
  // reaberto, o ID não volta a ser usado.
  useEffect(() => {
    observeStatusIds(queueKey, workflow.statuses.map((s) => s.id));
  }, [queueKey, workflow]);

  // Salvamentos em fila, na ordem das alterações. Cada alteração é uma
  // transformação aplicada, na hora do envio, sobre o último workflow aceito
  // pelo backend. Se algo falhar, o que estava na fila depois é descartado e a
  // tela volta a esse estado quando a fila esvaziar. A fila é a
  // compartilhada de `saveQueueKey`, para quem reabre o editor esperar também
  // as alterações que ainda não saíram.
  const persistedRef = useRef<WorkflowState>(wf);
  const pendingRef = useRef(0);
  const failedRef = useRef(false);

  const persist = useCallback((change: WorkflowChange, statusMigration: Record<number, number> = {}) => {
    pendingRef.current += 1;
    return enqueueSave(queueKey, async () => {
      try {
        // As alterações posteriores a uma falha partiram de uma tela que a
        // incluía: são descartadas junto, e a tela volta ao último estado salvo.
        if (failedRef.current) return false;
        const base = persistedRef.current;
        const next = change(base);
        const cleanTransitions: WorkflowTransitions = {};
        for (const s of next.statuses) cleanTransitions[s.id] = next.transitions[s.id] || [];
        const expected: TaskListWorkflowSnapshot = {
          statuses: base.statuses,
          transitions: base.transitions,
          initialStatusId: base.initialStatusId,
        };
        await onSave(next.statuses, cleanTransitions, next.initialStatusId, statusMigration, expected);
        persistedRef.current = next;
        return true;
      } catch (error) {
        failedRef.current = true;
        if (isTaskListConfigConflict(error)) {
          addToast(t(
            'tasklist.workflow.conflict',
            'O workflow foi alterado em outro lugar, por outra aba ou pelo agente. A tela foi atualizada com a versão atual; confira e refaça a alteração.',
          ), 'warning', 10000);
          onConflict?.();
        } else {
          const msg = `${t('tasklist.workflow.saveFailed', 'Erro ao salvar workflow')}: ${getErrorMessage(error)}`;
          addToast(msg, 'error');
        }
        return false;
      } finally {
        pendingRef.current -= 1;
        if (pendingRef.current === 0 && failedRef.current) {
          failedRef.current = false;
          setWf(persistedRef.current);
        }
      }
    });
  }, [queueKey, onSave, onConflict, t, addToast]);

  // Depois de um conflito, quem abriu o editor recarrega o workflow e muda o
  // token: a tela e a base das próximas gravações passam a ser o gravado.
  const syncedTokenRef = useRef(syncToken);
  useEffect(() => {
    if (syncToken === syncedTokenRef.current) return;
    syncedTokenRef.current = syncToken;
    const fresh = workflowState(workflow);
    persistedRef.current = fresh;
    setWf(fresh);
    setCounts({ ...taskCountsByStatus });
    setFocused(prev => (prev ? fresh.statuses.find(s => s.id === prev.id) ?? null : null));
    setMigration(null);
  }, [syncToken, workflow, taskCountsByStatus]);

  // Ao entrar na tela, o foco vai para o grid de status.
  useInitialContentFocus(rootRef, true, () => {
    if (!requestGridFocus()) newButtonRef.current?.focus();
  });

  const openNewStatus = useCallback(() => {
    setDraft(emptyDraft(COLOR_PRESETS[statuses.length % COLOR_PRESETS.length].token));
    setItemModal({ mode: 'create' });
  }, [statuses.length]);

  useNewItemShortcut(openNewStatus);

  const openEditStatus = useCallback((status: TaskListWorkflowStatus) => {
    setDraft({
      label: status.label,
      icon: status.icon,
      color: status.color,
      transitions: [...(transitions[status.id] ?? [])],
      initial: status.id === initialStatusId,
    });
    setItemModal({ mode: 'edit', id: status.id });
  }, [transitions, initialStatusId]);

  const closeItemModal = useCallback(() => {
    setItemModal(null);
    // Volta o foco ao grid para seguir editando em série por teclado.
    requestAnimationFrame(() => { requestGridFocus(); });
  }, [requestGridFocus]);

  // X, Esc e Cancelar esperam o Aplicar em andamento: se o backend recusar,
  // o rascunho continua no formulário para tentar de novo.
  const cancelItemModal = useCallback(() => {
    if (!applyingRef.current) closeItemModal();
  }, [closeItemModal]);

  const toggleDraftTransition = useCallback((targetId: number) => {
    setDraft((prev) => {
      const current = new Set(prev.transitions);
      if (current.has(targetId)) current.delete(targetId);
      else current.add(targetId);
      return { ...prev, transitions: Array.from(current) };
    });
  }, []);

  const editingGone = itemModal?.mode === 'edit' && !statuses.some(s => s.id === itemModal.id);

  const closeGoneStatus = useCallback(() => {
    const msg = t('tasklist.workflow.statusGone', 'Este status não existe mais: foi removido em outro lugar.');
    addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
    announce(msg);
    closeItemModal();
  }, [t, addToast, announce, closeItemModal]);

  // Após um conflito a tela recarrega: se o status em edição foi removido, o
  // formulário fecha na hora, sem esperar outro Aplicar.
  useEffect(() => {
    if (editingGone) closeGoneStatus();
  }, [editingGone, closeGoneStatus]);

  const confirmItemModal = useCallback(async () => {
    if (editingGone) {
      closeGoneStatus();
      return;
    }
    const label = draft.label.trim();
    if (!label) {
      const msg = t('tasklist.workflow.emptyStatusName', 'Dê um nome ao status');
      addToast(msg, 'error');
      announce(msg);
      return;
    }
    let change: WorkflowChange;
    let created: TaskListWorkflowStatus | null = null;
    if (itemModal?.mode === 'edit') {
      change = editStatusChange(itemModal.id, {
        label, icon: draft.icon, color: draft.color,
      }, draft.transitions, draft.initial);
    } else {
      created = {
        id: observeStatusIds(queueKey, statuses.map((s) => s.id)) + 1,
        order: statuses.length,
        label,
        color: draft.color,
        icon: draft.icon.trim() || '⬜',
      };
      change = addStatusChange(created, draft.transitions, draft.initial);
    }

    if (applyingRef.current) return;
    applyingRef.current = true;
    if (created) observeStatusIds(queueKey, [created.id]);
    const ok = await persist(change);
    applyingRef.current = false;
    // Falha ao salvar mantém o modal aberto com o rascunho, para tentar de novo.
    if (!ok) return;
    setWf(change);
    if (created) {
      setFocused(created);
      announce(t('tasklist.workflow.statusAdded', 'Status adicionado'));
    } else {
      announce(t('tasklist.workflow.statusUpdated', 'Status atualizado'));
    }
    closeItemModal();
  }, [draft, itemModal, statuses, queueKey, editingGone, closeGoneStatus, t, addToast, announce, closeItemModal, persist]);

  const finishRemoval = useCallback((status: TaskListWorkflowStatus) => {
    setWf(removeStatusChange(status.id));
    setFocused(prev => (prev?.id === status.id ? null : prev));
    announce(t('tasklist.workflow.statusRemoved', 'Status removido'));
    // A linha some e o foco cairia no body: devolve ao grid (ou ao Novo).
    requestAnimationFrame(() => {
      if (!requestGridFocus()) newButtonRef.current?.focus();
    });
  }, [announce, t, requestGridFocus]);

  const deleteStatus = useCallback(async (status: TaskListWorkflowStatus) => {
    if ((counts[status.id] ?? 0) > 0) {
      const firstOther = statuses.find(s => s.id !== status.id);
      if (firstOther) setMigration({ status, targetId: firstOther.id });
      return;
    }
    const confirmed = await requestConfirm({
      title: t('tasklist.workflow.removeStatus', 'Remover Status'),
      message: t('tasklist.workflow.confirmRemoveStatus', 'Remover status "{{label}}"?', {
        label: status.label || `#${status.id}`,
      }),
    });
    if (!confirmed) return;
    if (await persist(removeStatusChange(status.id))) finishRemoval(status);
  }, [counts, statuses, requestConfirm, t, persist, finishRemoval]);

  const closeMigration = useCallback(() => {
    setMigration(null);
    requestAnimationFrame(() => { requestGridFocus(); });
  }, [requestGridFocus]);

  const cancelMigration = useCallback(() => {
    if (!applyingRef.current) closeMigration();
  }, [closeMigration]);

  const confirmMigration = useCallback(async () => {
    if (!migration) return;
    const { status, targetId } = migration;
    if (applyingRef.current) return;
    applyingRef.current = true;
    const ok = await persist(removeStatusChange(status.id), { [status.id]: targetId });
    applyingRef.current = false;
    if (!ok) return;
    setCounts(prev => {
      const updated = { ...prev, [targetId]: (prev[targetId] ?? 0) + (prev[status.id] ?? 0) };
      delete updated[status.id];
      return updated;
    });
    setMigration(null);
    finishRemoval(status);
  }, [migration, persist, finishRemoval]);

  const handleMoveStatus = useCallback((fromIndex: number, toIndex: number) => {
    // O grid move a linha e o foco na hora: a reordenação é otimista e o
    // salvamento entra na fila (revertido se o backend recusar).
    if (toIndex < 0 || toIndex >= statuses.length) return;
    const change = swapStatusesChange(statuses[fromIndex].id, statuses[toIndex].id);
    setWf(change);
    void persist(change);
  }, [statuses, persist]);

  const getRowActions = useCallback((status: TaskListWorkflowStatus) => [
    {
      id: 'edit',
      label: t('tasklist.edit', 'Editar'),
      icon: <EditOutlined aria-hidden="true" />,
      onClick: () => openEditStatus(status),
    },
    {
      id: 'delete',
      label: t('tasklist.delete', 'Deletar'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => void deleteStatus(status),
      danger: true,
      disabled: statuses.length <= 1,
    },
  ], [t, openEditStatus, deleteStatus, statuses.length]);

  const columns: DataGridColumn<TaskListWorkflowStatus>[] = useMemo(() => [
    {
      key: 'label',
      label: t('tasklist.workflow.statusLabel', 'Nome'),
      width: '35%',
      format: (_value, item) => (
        <span>{item.icon ? <span aria-hidden="true">{item.icon} </span> : null}{item.label}</span>
      ),
    },
    {
      key: 'color',
      label: t('tasklist.workflow.statusColor', 'Cor'),
      width: '25%',
      format: (_value, item) => colorName((k, f) => t(k, f), item.color),
    },
    {
      key: 'initial',
      label: t('tasklist.workflow.initialStatus', 'Status Inicial'),
      width: '20%',
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
    <div className="workflow-editor" ref={rootRef}>
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
              shortcut: 'Ctrl+N',
              variant: 'primary',
              buttonRef: newButtonRef,
            },
            {
              key: 'edit-status',
              label: t('tasklist.edit', 'Editar'),
              icon: <EditOutlined aria-hidden="true" />,
              onClick: () => focused && openEditStatus(focused),
              disabled: !focused,
            },
            {
              key: 'delete-status',
              label: t('tasklist.delete', 'Deletar'),
              icon: <DeleteOutlined aria-hidden="true" />,
              onClick: () => focused && void deleteStatus(focused),
              disabled: !focused || statuses.length <= 1,
              variant: 'danger',
            },
          ]}
        />

        <DataGrid
          items={statuses}
          columns={columns}
          getItemId={(item) => item.id}
          label={t('tasklist.workflow.statusGrid', 'Lista de status do workflow')}
          onFocusChange={(item) => setFocused(item)}
          onActivate={(item) => openEditStatus(item)}
          onMoveItem={handleMoveStatus}
          getRowActions={getRowActions}
          onGridReady={handleGridReady}
        />
        <p className="workflow-editor__hint">
          {t('tasklist.workflow.moveHint', 'Use Alt+Setas para reordenar o status focado. As alterações são salvas automaticamente.')}
        </p>
      </div>

      <Modal
        isOpen={itemModal !== null}
        onClose={cancelItemModal}
        title={itemModal?.mode === 'edit'
          ? t('tasklist.workflow.editStatusNamed', 'Editar status: {{label}}', {
            label: statuses.find((s) => s.id === itemModal.id)?.label || `#${itemModal.id}`,
          })
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
          <FormField label={t('tasklist.workflow.allowedTransitions', 'Pode transicionar para')}>
            <div
              className="workflow-status-form__transitions"
              role="group"
              aria-label={t('tasklist.workflow.allowedTransitions', 'Pode transicionar para')}
            >
              {statuses
                .filter((s) => s.id !== (itemModal?.mode === 'edit' ? itemModal.id : -1))
                .map((s) => (
                  <Checkbox
                    key={s.id}
                    label={`${s.icon} ${s.label}`}
                    checked={draft.transitions.includes(s.id)}
                    onChange={() => toggleDraftTransition(s.id)}
                  />
                ))}
            </div>
          </FormField>
          <Checkbox
            label={t('tasklist.workflow.initialStatus', 'Status Inicial')}
            checked={draft.initial}
            onChange={(e) => setDraft((prev) => ({ ...prev, initial: e.target.checked }))}
            disabled={statuses.length <= 1}
          />
          <DialogActions
            primary={
              <Button type="button" variant="primary" onClick={() => void confirmItemModal()}>
                {t('tasklist.customActions.apply', 'Aplicar')}
              </Button>
            }
            secondary={
              <Button type="button" variant="secondary" onClick={cancelItemModal}>
                {t('common.cancel', 'Cancelar')}
              </Button>
            }
          />
        </div>
      </Modal>

      <DecisionDialog
        isOpen={migration !== null}
        severity="destructive"
        title={t('tasklist.workflow.removeStatus', 'Remover Status')}
        description={migration
          ? t('tasklist.workflow.statusInUseMigrate', 'O status "{{label}}" tem {{count}} tarefa(s). Escolha para qual status movê-las antes de remover.', {
            label: migration.status.label || `#${migration.status.id}`,
            count: counts[migration.status.id] ?? 0,
          })
          : ''}
        body={migration && (
          <Select
            label={t('tasklist.workflow.migrateTasksTo', 'Migrar tarefas para')}
            value={String(migration.targetId)}
            onChange={(e) => setMigration((prev) => (prev ? { ...prev, targetId: Number(e.target.value) } : prev))}
            options={statuses
              .filter((s) => s.id !== migration.status.id)
              .map((s) => ({ value: String(s.id), label: `${s.icon} ${s.label}` }))}
          />
        )}
        actions={[
          {
            id: 'confirm',
            label: t('tasklist.workflow.removeAndMigrate', 'Remover e migrar'),
            variant: 'danger',
            primary: true,
            polarity: 'affirmative',
            scope: 'current',
          },
          {
            id: 'cancel',
            label: t('common.cancel', 'Cancelar'),
            variant: 'secondary',
            polarity: 'negative',
            scope: 'current',
          },
        ]}
        onAction={(actionId) => {
          if (actionId === 'confirm') void confirmMigration();
          else cancelMigration();
        }}
        onCancel={cancelMigration}
      />
    </div>
  );
}
