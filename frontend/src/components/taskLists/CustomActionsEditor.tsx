import { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useUIStore } from '../../store/uiStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { useGridFocus } from '../../hooks/useGridFocus';
import { useInitialContentFocus } from '../../hooks/useInitialContentFocus';
import { useNewItemShortcut } from '../../hooks/useNewItemShortcut';
import { enqueueSave, taskListCustomActionsSaveKey, whenSavesSettled } from '../../lib/serialSaveQueue';
import type { CustomAction, CustomActionSurface } from '../../types/tasklist';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { Toolbar } from '../ui/Toolbar';
import { DataGrid, type DataGridColumn } from '../ui/DataGrid';
import { MenuButton } from '../layout/MenuButton';
import { FormField } from '../ui/FormField';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import { Checkbox } from '../ui/Checkbox';
import { DialogActions } from '../ui/DialogActions';
import './CustomActionsEditor.css';

interface CustomActionsEditorProps {
  taskListId: string;
  /** Chamado após cada persistência bem-sucedida. */
  onSaved?: () => void;
}

const SURFACES: { value: CustomActionSurface; labelKey: string; fallback: string }[] = [
  { value: 'card_menu', labelKey: 'tasklist.customActions.surface.cardMenu', fallback: 'Menu do card' },
  { value: 'card_detail', labelKey: 'tasklist.customActions.surface.cardDetail', fallback: 'Detalhe do card' },
  { value: 'board_menu', labelKey: 'tasklist.customActions.surface.boardMenu', fallback: 'Menu do quadro' },
];

// EditableAction adiciona um id de UI estável (não persistido) para usar como
// React key e id de linha do grid — evita bugs visuais ao remover/reordenar.
type EditableAction = CustomAction & { _uiId: string };

function newUiId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `ca-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function withUiId(a: CustomAction): EditableAction {
  return { ...a, _uiId: newUiId() };
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error ?? '');
}

function emptyAction(): EditableAction {
  return {
    id: '',
    label: '',
    surfaces: ['card_menu'],
    event: '',
    payload_template: '',
    link: '',
    when: '',
    danger: false,
    confirm: '',
    _uiId: newUiId(),
  };
}

function surfaceLabels(t: (key: string, fallback: string) => string, surfaces?: CustomActionSurface[]): string {
  if (!surfaces || surfaces.length === 0) return '—';
  return surfaces
    .map((s) => {
      const known = SURFACES.find((k) => k.value === s);
      return known ? t(known.labelKey, known.fallback) : s;
    })
    .join(', ');
}

/**
 * Editor das custom actions (AEP-0067) de uma TaskList, no padrão do sistema:
 * Toolbar (Nova/Editar/Apagar) + DataGrid + modal de edição por ação.
 * Cada criação, edição ou exclusão persiste na hora a lista inteira como o
 * JSON de TaskList.CustomActions; não há rascunho nem Salvar em lote.
 */
export default function CustomActionsEditor({ taskListId, onSaved }: CustomActionsEditorProps) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const requestConfirm = useConfirm();
  const { handleGridReady, requestGridFocus } = useGridFocus();
  const getTaskListCustomActions = useTaskListStore((s) => s.getTaskListCustomActions);
  const setTaskListCustomActions = useTaskListStore((s) => s.setTaskListCustomActions);

  const [actions, setActions] = useState<EditableAction[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const savingRef = useRef(false);
  const [focused, setFocused] = useState<EditableAction | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Modal de edição por ação: 'create' parte do vazio, 'edit' do item focado.
  const [itemModal, setItemModal] = useState<{ mode: 'create' } | { mode: 'edit'; uiId: string } | null>(null);
  const [draft, setDraft] = useState<EditableAction>(emptyAction);
  const newButtonRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    // Reaberto logo após fechar no meio de um salvamento: lê só depois dele.
    whenSavesSettled(taskListCustomActionsSaveKey(taskListId))
      .then(() => getTaskListCustomActions(taskListId))
      .then((res) => {
        if (!cancelled) setActions((res.actions ?? []).map(withUiId));
      })
      .catch(() => {
        if (!cancelled) setActions([]);
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => { cancelled = true; };
  }, [taskListId, getTaskListCustomActions]);

  // Ao entrar na tela (dados carregados), o foco vai para o grid — ou para
  // "Nova ação" quando a lista está vazia e não há grid.
  useInitialContentFocus(rootRef, !isLoading, () => {
    if (!requestGridFocus()) newButtonRef.current?.focus();
  });

  const openNewAction = useCallback(() => {
    setDraft(emptyAction());
    setItemModal({ mode: 'create' });
  }, []);

  const openEditAction = useCallback((action: EditableAction) => {
    setDraft({ ...action, surfaces: [...(action.surfaces ?? [])] });
    setItemModal({ mode: 'edit', uiId: action._uiId });
  }, []);

  const closeItemModal = useCallback(() => {
    setItemModal(null);
    // Volta o foco ao grid para seguir editando em série por teclado.
    requestAnimationFrame(() => { requestGridFocus(); });
  }, [requestGridFocus]);

  const patchDraft = useCallback((patch: Partial<CustomAction>) => {
    setDraft((prev) => ({ ...prev, ...patch }));
  }, []);

  const toggleDraftSurface = useCallback((surface: CustomActionSurface) => {
    setDraft((prev) => {
      const current = new Set(prev.surfaces ?? []);
      if (current.has(surface)) current.delete(surface);
      else current.add(surface);
      return { ...prev, surfaces: Array.from(current) };
    });
  }, []);

  // Cada operação persiste a lista inteira na hora; o estado local só muda
  // depois que o backend aceita, para a tela nunca mostrar algo não salvo.
  const persist = useCallback(async (next: EditableAction[]): Promise<boolean> => {
    // Um salvamento por vez: cliques repetidos em Aplicar não duplicam a ação.
    if (savingRef.current) return false;
    savingRef.current = true;
    setIsSaving(true);
    try {
      const cleaned = next.map(({ _uiId, ...a }) => ({
        ...a,
        surfaces: a.surfaces && a.surfaces.length > 0 ? a.surfaces : ['card_menu'],
      }));
      // Sem ações: persiste string vazia (não `{"actions":[]}`). O backend trata
      // vazio como "sem ações" e isso mantém custom_actions limpo no round-trip/clone.
      const json = cleaned.length > 0 ? JSON.stringify({ actions: cleaned }) : '';
      await enqueueSave(taskListCustomActionsSaveKey(taskListId), () => setTaskListCustomActions(taskListId, json));
      setActions(next);
      onSaved?.();
      return true;
    } catch (error) {
      addToast(
        t('tasklist.customActions.saveError', 'Falha ao salvar ações: {{error}}', { error: getErrorMessage(error) }),
        'error',
      );
      return false;
    } finally {
      savingRef.current = false;
      setIsSaving(false);
    }
  }, [taskListId, setTaskListCustomActions, addToast, t, onSaved]);

  const confirmItemModal = useCallback(async () => {
    const id = draft.id.trim();
    const label = draft.label.trim();
    if (!id || !label) {
      const msg = t('tasklist.customActions.requiredFields', 'Preencha ID e Rótulo da ação');
      addToast(msg, 'error');
      announce(msg);
      return;
    }
    const editingUiId = itemModal?.mode === 'edit' ? itemModal.uiId : null;
    if (actions.some((a) => a.id === id && a._uiId !== editingUiId)) {
      const msg = t('tasklist.customActions.duplicateId', 'Já existe uma ação com este ID');
      addToast(msg, 'error');
      announce(msg);
      return;
    }
    const cleaned: EditableAction = { ...draft, id, label };
    const isEdit = itemModal?.mode === 'edit';
    const next = isEdit
      ? actions.map((a) => (a._uiId === itemModal.uiId ? cleaned : a))
      : [...actions, cleaned];
    // Falha ao salvar mantém o modal aberto com o rascunho, para tentar de novo.
    if (!(await persist(next))) return;
    announce(isEdit
      ? t('tasklist.customActions.updated', 'Ação atualizada')
      : t('tasklist.customActions.added', 'Ação adicionada'));
    setFocused(cleaned);
    closeItemModal();
  }, [draft, actions, itemModal, t, addToast, announce, closeItemModal, persist]);

  const deleteAction = useCallback(async (action: EditableAction) => {
    const confirmed = await requestConfirm({
      title: t('tasklist.customActions.deleteConfirmTitle', 'Apagar ação'),
      message: t(
        'tasklist.customActions.deleteConfirm',
        'Apagar a ação "{{label}}"?',
        { label: action.label || action.id },
      ),
    });
    if (!confirmed) return;
    if (!(await persist(actions.filter((a) => a._uiId !== action._uiId)))) return;
    setFocused((prev) => (prev?._uiId === action._uiId ? null : prev));
    announce(t('tasklist.customActions.deleted', 'Ação apagada'));
    // A linha some e o foco cairia no body: devolve ao grid (ou ao Novo,
    // se a lista esvaziou e o grid desmontou).
    requestAnimationFrame(() => {
      if (!requestGridFocus()) newButtonRef.current?.focus();
    });
  }, [requestConfirm, t, announce, requestGridFocus, persist, actions]);

  useNewItemShortcut(openNewAction, !isSaving);

  const getRowActions = useCallback((action: EditableAction) => [
    {
      id: 'edit',
      label: t('tasklist.edit', 'Editar'),
      icon: <EditOutlined aria-hidden="true" />,
      onClick: () => openEditAction(action),
      disabled: isSaving,
    },
    {
      id: 'delete',
      label: t('tasklist.delete', 'Deletar'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => void deleteAction(action),
      danger: true,
      disabled: isSaving,
    },
  ], [t, openEditAction, deleteAction, isSaving]);

  const columns: DataGridColumn<EditableAction>[] = useMemo(() => [
    {
      key: 'label',
      label: t('tasklist.customActions.field.label', 'Rótulo'),
      width: '30%',
      format: (_value, item) => (
        <span>{item.icon ? <span aria-hidden="true">{item.icon} </span> : null}{item.label || item.id}</span>
      ),
    },
    {
      key: 'surfaces',
      label: t('tasklist.customActions.field.surfaces', 'Onde aparece'),
      width: '30%',
      truncate: true,
      format: (_value, item) => surfaceLabels((k, f) => t(k, f), item.surfaces),
    },
    {
      key: 'action',
      label: t('tasklist.customActions.action', 'Ação'),
      width: '25%',
      truncate: true,
      format: (_value, item) => item.link
        || t('tasklist.customActions.publishesEvent', 'Publica evento'),
    },
    {
      key: 'danger',
      label: t('tasklist.customActions.field.danger', 'Destrutiva'),
      width: '10%',
      format: (_value, item) => (item.danger ? t('common.yes', 'Sim') : t('common.no', 'Não')),
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
  ], [t, getRowActions]);

  if (isLoading) {
    return <div className="custom-actions-editor__loading">{t('tasklist.loading', 'Carregando...')}</div>;
  }

  return (
    <div className="custom-actions-editor" ref={rootRef}>
      <p className="custom-actions-editor__hint">
        {t(
          'tasklist.customActions.hint',
          'Defina ações por card ou quadro. Cada ação pode publicar um evento (que pode disparar jobs) e/ou abrir um link. Os templates têm acesso aos campos do card (.task.code, .task.link, etc.). As alterações são salvas automaticamente.',
        )}
      </p>

      <Toolbar
        ariaLabel={t('tasklist.customActions.toolbar', 'Barra de ferramentas de ações customizadas')}
        actions={[
          {
            key: 'new-action',
            label: t('tasklist.customActions.newAction', 'Nova ação'),
            icon: <PlusOutlined aria-hidden="true" />,
            onClick: openNewAction,
            shortcut: 'Ctrl+N',
            variant: 'primary',
            buttonRef: newButtonRef,
            disabled: isSaving,
          },
          {
            key: 'edit-action',
            label: t('tasklist.edit', 'Editar'),
            icon: <EditOutlined aria-hidden="true" />,
            onClick: () => focused && openEditAction(focused),
            disabled: !focused || isSaving,
          },
          {
            key: 'delete-action',
            label: t('tasklist.delete', 'Deletar'),
            icon: <DeleteOutlined aria-hidden="true" />,
            onClick: () => focused && void deleteAction(focused),
            disabled: !focused || isSaving,
            variant: 'danger',
          },
        ]}
      />

      {actions.length === 0 ? (
        <p className="custom-actions-editor__empty">
          {t('tasklist.customActions.empty', 'Nenhuma ação customizada definida.')}
        </p>
      ) : (
        <DataGrid
          items={actions}
          columns={columns}
          getItemId={(item) => item._uiId}
          label={t('tasklist.customActions.grid', 'Lista de ações customizadas')}
          onFocusChange={(item) => setFocused(item)}
          onActivate={(item) => openEditAction(item)}
          getRowActions={getRowActions}
          onGridReady={handleGridReady}
        />
      )}

      <Modal
        isOpen={itemModal !== null}
        onClose={closeItemModal}
        title={itemModal?.mode === 'edit'
          ? t('tasklist.customActions.editActionNamed', 'Editar ação: {{label}}', {
            label: actions.find((a) => a._uiId === itemModal.uiId)?.label
              || actions.find((a) => a._uiId === itemModal.uiId)?.id
              || '',
          })
          : t('tasklist.customActions.newAction', 'Nova ação')}
      >
        <div className="custom-action-form">
          <FormField label={t('tasklist.customActions.field.id', 'ID')} required>
            <Input
              type="text"
              value={draft.id}
              placeholder={t('tasklist.customActions.field.idPlaceholder', 'investigar')}
              onChange={(e) => patchDraft({ id: e.target.value })}
              maxLength={128}
            />
          </FormField>
          <FormField label={t('tasklist.customActions.field.label', 'Rótulo')} required>
            <Input
              type="text"
              value={draft.label}
              placeholder={t('tasklist.customActions.field.labelPlaceholder', 'Investigar')}
              onChange={(e) => patchDraft({ label: e.target.value })}
              maxLength={200}
            />
          </FormField>
          <FormField label={t('tasklist.customActions.field.icon', 'Ícone')}>
            <Input
              type="text"
              value={draft.icon ?? ''}
              placeholder={t('tasklist.customActions.field.iconPlaceholder', '🔍')}
              onChange={(e) => patchDraft({ icon: e.target.value })}
              maxLength={32}
            />
          </FormField>
          <FormField label={t('tasklist.customActions.field.surfaces', 'Onde aparece')}>
            <div className="custom-action-form__surfaces">
              {SURFACES.map((s) => (
                <Checkbox
                  key={s.value}
                  label={t(s.labelKey, s.fallback)}
                  checked={(draft.surfaces ?? []).includes(s.value)}
                  onChange={() => toggleDraftSurface(s.value)}
                />
              ))}
            </div>
          </FormField>
          <FormField
            label={t('tasklist.customActions.field.event', 'Evento (opcional)')}
          >
            <Input
              type="text"
              value={draft.event ?? ''}
              placeholder={t('tasklist.customActions.field.eventPlaceholder', 'tasklist.card.investigate_requested')}
              onChange={(e) => patchDraft({ event: e.target.value })}
              maxLength={256}
            />
          </FormField>
          <FormField
            label={t('tasklist.customActions.field.link', 'Link (opcional, template)')}
          >
            <Input
              type="text"
              value={draft.link ?? ''}
              placeholder={t('tasklist.customActions.field.linkPlaceholder', '{{ .task.link }}')}
              onChange={(e) => patchDraft({ link: e.target.value })}
              maxLength={512}
            />
          </FormField>
          <FormField
            label={t('tasklist.customActions.field.payload', 'Payload template (JSON, opcional)')}
          >
            <Textarea
              rows={3}
              value={draft.payload_template ?? ''}
              placeholder={t(
                'tasklist.customActions.field.payloadPlaceholder',
                '{"code": {{ json .task.code }}, "title": {{ json .task.title }}}',
              )}
              onChange={(e) => patchDraft({ payload_template: e.target.value })}
            />
          </FormField>
          <FormField
            label={t('tasklist.customActions.field.when', 'Condição "when" (opcional, template)')}
          >
            <Input
              type="text"
              value={draft.when ?? ''}
              placeholder={t('tasklist.customActions.field.whenPlaceholder', '{{ ne .task.code "" }}')}
              onChange={(e) => patchDraft({ when: e.target.value })}
              maxLength={512}
            />
          </FormField>
          <FormField
            label={t('tasklist.customActions.field.confirm', 'Confirmação (opcional)')}
          >
            <Input
              type="text"
              value={draft.confirm ?? ''}
              placeholder={t('tasklist.customActions.field.confirmPlaceholder', 'Confirmar esta ação?')}
              onChange={(e) => patchDraft({ confirm: e.target.value })}
              maxLength={512}
            />
          </FormField>
          <Checkbox
            label={t('tasklist.customActions.field.danger', 'Destrutiva')}
            checked={!!draft.danger}
            onChange={(e) => patchDraft({ danger: e.target.checked })}
          />
          <DialogActions
            primary={
              <Button type="button" variant="primary" onClick={() => void confirmItemModal()}>
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
