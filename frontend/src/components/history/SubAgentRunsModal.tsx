import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { StopOutlined } from '@ant-design/icons';
import type { subagent } from '@wailsjs/go/models';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import type { MenuItem as ContextMenuItem } from '../menu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useSubAgentRunsStore } from '../../store/subAgentRunsStore';
import { formatRelativeTime } from '../../lib/dateUtils';
import { logger } from '../../utils/logger';
import './SubAgentRunsModal.css';

export interface SubAgentRunsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

/**
 * Painel de runs de sub-agente (AEP-0068 F5): mostra o que está rodando em
 * segundo plano e o que terminou há pouco, com a ação de cancelar.
 *
 * A listagem do histórico responde "que conversas existem"; esta responde "o
 * que está rodando agora" — e, sem ela, interromper um sub-agente só seria
 * possível pelo LLM, nunca pela pessoa.
 */
export function SubAgentRunsModal({ isOpen, onClose }: SubAgentRunsModalProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const runs = useSubAgentRunsStore((state) => state.runs);
  const activeForUser = useSubAgentRunsStore((state) => state.activeForUser);
  const activeGlobal = useSubAgentRunsStore((state) => state.activeGlobal);
  const maxConcurrentPerUser = useSubAgentRunsStore((state) => state.maxConcurrentPerUser);
  const maxConcurrentGlobal = useSubAgentRunsStore((state) => state.maxConcurrentGlobal);
  const isLoading = useSubAgentRunsStore((state) => state.isLoading);
  const fetchRuns = useSubAgentRunsStore((state) => state.fetchRuns);
  const cancelRun = useSubAgentRunsStore((state) => state.cancelRun);

  useEffect(() => {
    if (!isOpen) return;
    void fetchRuns();
  }, [isOpen, fetchRuns]);

  const handleCancel = useCallback(
    async (run: subagent.RunListItem) => {
      const label = runTitle(run, t);
      try {
        const result = await cancelRun(run.conversationId, run.runId);
        // `cancelled: false` é o no-op do contrato: o run já havia terminado
        // entre a renderização da lista e o clique. Dizer "cancelado" nesse caso
        // seria mentir sobre o que aconteceu.
        announce(
          result?.cancelled
            ? t('subAgentRuns.announce.cancelled', { title: label })
            : t('subAgentRuns.announce.cancelNoop', {
                title: label,
                status: statusLabel(result?.status, t),
              }),
        );
      } catch (error) {
        logger.error('Erro ao cancelar run de sub-agente:', error);
        announce(t('subAgentRuns.announce.cancelError', { title: label }), 'assertive');
      }
      await fetchRuns();
    },
    [announce, cancelRun, fetchRuns, t],
  );

  const getRowActions = useCallback(
    (run: subagent.RunListItem): ContextMenuItem[] =>
      run.active
        ? [
            {
              id: 'cancel',
              label: t('subAgentRuns.cancel', 'Cancelar run'),
              icon: <StopOutlined />,
              action: () => void handleCancel(run),
            },
          ]
        : [],
    [handleCancel, t],
  );

  const columns: DataGridColumn<subagent.RunListItem>[] = [
    {
      key: 'title',
      label: t('subAgentRuns.columnTitle', 'Sub-agente'),
      width: '40%',
      format: (_value, run) => runTitle(run, t),
    },
    {
      key: 'status',
      label: t('subAgentRuns.columnStatus', 'Situação'),
      width: '18%',
      format: (_value, run) => statusLabel(run.status, t),
    },
    {
      key: 'background',
      label: t('subAgentRuns.columnMode', 'Modo'),
      width: '16%',
      format: (_value, run) =>
        run.background
          ? t('subAgentRuns.modeBackground', 'Segundo plano')
          : t('subAgentRuns.modeInline', 'Em linha'),
    },
    {
      key: 'createdAt',
      label: t('subAgentRuns.columnStarted', 'Início'),
      width: '16%',
      format: (_value, run) => formatRelativeTime(new Date(String(run.startedAt ?? run.createdAt)).getTime()),
    },
    {
      key: 'actions',
      label: t('subAgentRuns.columnActions', 'Ações'),
      width: '10%',
      format: (_value, run) =>
        run.active ? (
          <Button
            variant="danger"
            size="sm"
            aria-label={t('subAgentRuns.cancelRunLabel', {
              title: runTitle(run, t),
              defaultValue: 'Cancelar run {{title}}',
            })}
            onClick={() => void handleCancel(run)}
          >
            {t('subAgentRuns.cancel', 'Cancelar run')}
          </Button>
        ) : (
          ''
        ),
    },
  ];

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('subAgentRuns.title', 'Runs de sub-agentes')}
      size="lg"
      ariaDescribedBy="subagent-runs-usage"
    >
      <p id="subagent-runs-usage" className="subagent-runs__usage">
        {t('subAgentRuns.usage', {
          active: activeForUser,
          max: maxConcurrentPerUser,
          globalActive: activeGlobal,
          globalMax: maxConcurrentGlobal,
          defaultValue:
            '{{active}} de {{max}} runs simultâneos seus em execução; {{globalActive}} de {{globalMax}} no aplicativo.',
        })}
      </p>

      {runs.length === 0 ? (
        <p className="subagent-runs__empty">
          {isLoading
            ? t('subAgentRuns.loading', 'Carregando runs...')
            : t('subAgentRuns.empty', 'Nenhum run de sub-agente registrado.')}
        </p>
      ) : (
        <DataGrid
          items={runs}
          columns={columns}
          label={t('subAgentRuns.gridLabel', 'Runs de sub-agentes')}
          getItemId={(run) => run.runId}
          getRowActions={getRowActions}
          autoFocusOnMount={false}
        />
      )}
    </Modal>
  );
}

function runTitle(run: subagent.RunListItem, t: TFunction): string {
  return run.title?.trim() || t('subAgentRuns.untitled', 'Sub-agente');
}

function statusLabel(status: string | undefined, t: TFunction): string {
  if (!status) return t('history.subAgentStatus.unknown');
  return t(`history.subAgentStatus.${status}`, {
    defaultValue: t('history.subAgentStatus.unknown'),
  });
}
