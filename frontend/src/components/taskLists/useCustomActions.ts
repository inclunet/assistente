import { useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useUIStore } from '../../store/uiStore';
import { requestConfirm } from '../../store/confirmStore';
import { openTaskLink } from '../../lib/deepLinks';
import type { CustomActionView } from '../../types/tasklist';

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error ?? '');
}

/**
 * Hook que executa uma custom action (AEP-0067) a partir de qualquer surface
 * (menu do card, detalhe do card ou menu do quadro).
 *
 * Fluxo: confirma (se necessário) → dispara no backend (publica evento e/ou
 * renderiza link) → abre o link renderizado quando houver → notifica erros.
 */
export function useCustomActions() {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const triggerCustomAction = useTaskListStore((s) => s.triggerCustomAction);

  const runCustomAction = useCallback(
    async (action: CustomActionView, taskListId: string, taskId: string) => {
      if (action.confirm) {
        const ok = await requestConfirm({
          title: action.label,
          message: action.confirm,
          variant: action.danger ? 'danger' : 'warning',
        });
        if (!ok) return;
      }

      try {
        const link = await triggerCustomAction(taskListId, taskId, action.id);
        if (link) {
          openTaskLink(link, { navigate });
        }
      } catch (error) {
        addToast(
          t('tasklist.customActions.runError', 'Falha ao executar ação: {{error}}', {
            error: getErrorMessage(error),
          }),
          'error',
        );
      }
    },
    [navigate, addToast, triggerCustomAction, t],
  );

  return { runCustomAction };
}
