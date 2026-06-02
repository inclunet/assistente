import { useEffect, useState, useCallback } from 'react';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useUIStore } from '../../store/uiStore';
import type { CustomAction, CustomActionSurface } from '../../types/tasklist';
import './CustomActionsEditor.css';

interface CustomActionsEditorProps {
  taskListId: string;
  onClose: () => void;
  onSaved?: () => void;
}

const SURFACES: { value: CustomActionSurface; labelKey: string; fallback: string }[] = [
  { value: 'card_menu', labelKey: 'tasklist.customActions.surface.cardMenu', fallback: 'Menu do card' },
  { value: 'card_detail', labelKey: 'tasklist.customActions.surface.cardDetail', fallback: 'Detalhe do card' },
  { value: 'board_menu', labelKey: 'tasklist.customActions.surface.boardMenu', fallback: 'Menu do quadro' },
];

// EditableAction adiciona um id de UI estável (não persistido) para usar como
// React key — evita bugs visuais de inputs controlados ao remover/reordenar
// (com key={idx} o React reaproveita DOM/estado entre linhas).
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

/**
 * Editor estruturado das custom actions (AEP-0067) de uma TaskList.
 * Serializa para o JSON persistido em TaskList.CustomActions; a validação
 * forte (ids únicos, evento/link obrigatório, surfaces válidas) ocorre no
 * backend ao salvar.
 */
export default function CustomActionsEditor({ taskListId, onClose, onSaved }: CustomActionsEditorProps) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const getTaskListCustomActions = useTaskListStore((s) => s.getTaskListCustomActions);
  const setTaskListCustomActions = useTaskListStore((s) => s.setTaskListCustomActions);

  const [actions, setActions] = useState<EditableAction[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getTaskListCustomActions(taskListId)
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

  const updateAction = useCallback((idx: number, patch: Partial<CustomAction>) => {
    setActions((prev) => prev.map((a, i) => (i === idx ? { ...a, ...patch } : a)));
  }, []);

  const toggleSurface = useCallback((idx: number, surface: CustomActionSurface) => {
    setActions((prev) => prev.map((a, i) => {
      if (i !== idx) return a;
      const current = new Set(a.surfaces ?? []);
      if (current.has(surface)) current.delete(surface);
      else current.add(surface);
      return { ...a, surfaces: Array.from(current) };
    }));
  }, []);

  const addAction = useCallback(() => {
    setActions((prev) => [...prev, emptyAction()]);
  }, []);

  const removeAction = useCallback((idx: number) => {
    setActions((prev) => prev.filter((_, i) => i !== idx));
  }, []);

  const handleSave = useCallback(async () => {
    setIsSaving(true);
    try {
      const cleaned = actions.map(({ _uiId, ...a }) => ({
        ...a,
        surfaces: a.surfaces && a.surfaces.length > 0 ? a.surfaces : ['card_menu'],
      }));
      const json = JSON.stringify({ actions: cleaned });
      await setTaskListCustomActions(taskListId, json);
      addToast(t('tasklist.customActions.saved', 'Ações customizadas salvas'), 'success');
      onSaved?.();
      onClose();
    } catch (error) {
      addToast(
        t('tasklist.customActions.saveError', 'Falha ao salvar ações: {{error}}', { error: String(error) }),
        'error',
      );
    } finally {
      setIsSaving(false);
    }
  }, [actions, taskListId, setTaskListCustomActions, addToast, t, onSaved, onClose]);

  if (isLoading) {
    return <div className="custom-actions-editor__loading">{t('tasklist.loading', 'Carregando...')}</div>;
  }

  return (
    <div className="custom-actions-editor">
      <p className="custom-actions-editor__hint">
        {t(
          'tasklist.customActions.hint',
          'Defina ações por card ou quadro. Cada ação pode publicar um evento (que pode disparar jobs) e/ou abrir um link. Os templates têm acesso aos campos do card (.task.code, .task.link, etc.).',
        )}
      </p>

      {actions.length === 0 && (
        <p className="custom-actions-editor__empty">
          {t('tasklist.customActions.empty', 'Nenhuma ação customizada definida.')}
        </p>
      )}

      <div className="custom-actions-editor__list">
        {actions.map((action, idx) => (
          <div key={action._uiId} className="custom-actions-editor__item">
            <div className="custom-actions-editor__row">
              <label className="custom-actions-editor__field">
                <span>{t('tasklist.customActions.field.id', 'ID')}</span>
                <input
                  type="text"
                  value={action.id}
                  placeholder={t('tasklist.customActions.field.idPlaceholder', 'investigar')}
                  onChange={(e) => updateAction(idx, { id: e.target.value })}
                />
              </label>
              <label className="custom-actions-editor__field">
                <span>{t('tasklist.customActions.field.label', 'Rótulo')}</span>
                <input
                  type="text"
                  value={action.label}
                  placeholder={t('tasklist.customActions.field.labelPlaceholder', 'Investigar')}
                  onChange={(e) => updateAction(idx, { label: e.target.value })}
                />
              </label>
              <label className="custom-actions-editor__field custom-actions-editor__field--narrow">
                <span>{t('tasklist.customActions.field.icon', 'Ícone')}</span>
                <input
                  type="text"
                  value={action.icon ?? ''}
                  placeholder={t('tasklist.customActions.field.iconPlaceholder', '🔍')}
                  onChange={(e) => updateAction(idx, { icon: e.target.value })}
                />
              </label>
              <button
                type="button"
                className="custom-actions-editor__remove"
                onClick={() => removeAction(idx)}
                aria-label={t('tasklist.customActions.remove', 'Remover ação')}
                title={t('tasklist.customActions.remove', 'Remover ação')}
              >
                <DeleteOutlined aria-hidden="true" />
              </button>
            </div>

            <div className="custom-actions-editor__surfaces">
              <span>{t('tasklist.customActions.field.surfaces', 'Onde aparece')}</span>
              {SURFACES.map((s) => (
                <label key={s.value} className="custom-actions-editor__checkbox">
                  <input
                    type="checkbox"
                    checked={(action.surfaces ?? []).includes(s.value)}
                    onChange={() => toggleSurface(idx, s.value)}
                  />
                  {t(s.labelKey, s.fallback)}
                </label>
              ))}
            </div>

            <div className="custom-actions-editor__row">
              <label className="custom-actions-editor__field">
                <span>{t('tasklist.customActions.field.event', 'Evento (opcional)')}</span>
                <input
                  type="text"
                  value={action.event ?? ''}
                  placeholder={t('tasklist.customActions.field.eventPlaceholder', 'tasklist.card.investigate_requested')}
                  onChange={(e) => updateAction(idx, { event: e.target.value })}
                />
              </label>
              <label className="custom-actions-editor__field">
                <span>{t('tasklist.customActions.field.link', 'Link (opcional, template)')}</span>
                <input
                  type="text"
                  value={action.link ?? ''}
                  placeholder={t('tasklist.customActions.field.linkPlaceholder', '{{ .task.link }}')}
                  onChange={(e) => updateAction(idx, { link: e.target.value })}
                />
              </label>
            </div>

            <label className="custom-actions-editor__field custom-actions-editor__field--full">
              <span>{t('tasklist.customActions.field.payload', 'Payload template (JSON, opcional)')}</span>
              <textarea
                rows={2}
                value={action.payload_template ?? ''}
                placeholder={t(
                  'tasklist.customActions.field.payloadPlaceholder',
                  '{"code": {{ json .task.code }}, "title": {{ json .task.title }}}',
                )}
                onChange={(e) => updateAction(idx, { payload_template: e.target.value })}
              />
            </label>

            <div className="custom-actions-editor__row">
              <label className="custom-actions-editor__field">
                <span>{t('tasklist.customActions.field.when', 'Condição "when" (opcional, template)')}</span>
                <input
                  type="text"
                  value={action.when ?? ''}
                  placeholder={t('tasklist.customActions.field.whenPlaceholder', '{{ ne .task.code "" }}')}
                  onChange={(e) => updateAction(idx, { when: e.target.value })}
                />
              </label>
              <label className="custom-actions-editor__field">
                <span>{t('tasklist.customActions.field.confirm', 'Confirmação (opcional)')}</span>
                <input
                  type="text"
                  value={action.confirm ?? ''}
                  placeholder={t('tasklist.customActions.field.confirmPlaceholder', 'Confirmar esta ação?')}
                  onChange={(e) => updateAction(idx, { confirm: e.target.value })}
                />
              </label>
              <label className="custom-actions-editor__checkbox custom-actions-editor__checkbox--inline">
                <input
                  type="checkbox"
                  checked={!!action.danger}
                  onChange={(e) => updateAction(idx, { danger: e.target.checked })}
                />
                {t('tasklist.customActions.field.danger', 'Destrutiva')}
              </label>
            </div>
          </div>
        ))}
      </div>

      <div className="custom-actions-editor__footer">
        <button type="button" className="custom-actions-editor__add" onClick={addAction}>
          <PlusOutlined aria-hidden="true" /> {t('tasklist.customActions.add', 'Adicionar ação')}
        </button>
        <div className="custom-actions-editor__footer-spacer" />
        <button type="button" className="custom-actions-editor__cancel" onClick={onClose} disabled={isSaving}>
          {t('common.cancel', 'Cancelar')}
        </button>
        <button type="button" className="custom-actions-editor__save" onClick={() => void handleSave()} disabled={isSaving}>
          {isSaving ? t('common.saving', 'Salvando...') : t('common.save', 'Salvar')}
        </button>
      </div>
    </div>
  );
}
