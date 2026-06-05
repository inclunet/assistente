import { logger } from '../utils/logger';
import { useState, useEffect, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { FolderOpenOutlined, ReloadOutlined } from '@ant-design/icons';
import { GetSubAgentConversations } from '@wailsjs/go/app/App';
import type { subagent } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components/ui/Button';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { executeDeepLink } from '../lib/deepLinks';
import { formatRelativeTimeLocalized } from '../lib/dateUtils';
import './SubAgentsPage.css';

type SubAgent = subagent.SubConversationSummary;

const KNOWN_STATUSES = new Set([
  'queued',
  'running',
  'succeeded',
  'failed',
  'cancelled',
  'timed_out',
]);

function statusLabel(status: string, t: TFunction): string {
  const key = KNOWN_STATUSES.has(status) ? status : 'unknown';
  return t(`subagents.status.${key}`);
}

export default function SubAgentsPage() {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const navigate = useNavigate();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'subagents-page' });

  const [subAgents, setSubAgents] = useState<SubAgent[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [focusedRow, setFocusedRow] = useState<SubAgent | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    try {
      const result = await GetSubAgentConversations();
      setSubAgents(result || []);
    } catch (error) {
      logger.error('Erro ao carregar sub-agentes:', error);
      setLoadError(true);
      announce(t('subagents.errorLoading'), 'assertive');
    } finally {
      setLoading(false);
    }
  }, [announce, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleOpen = useCallback(async (conversationId: string, title?: string) => {
    try {
      await executeDeepLink(
        { type: 'conversation:open', conversationId, title },
        { navigate },
      );
    } catch (error) {
      logger.error('Erro ao abrir sub-conversa via deep link:', error);
      announce(t('subagents.openError'), 'assertive');
    }
  }, [navigate, announce, t]);

  const displayItems = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    if (!term) return subAgents;
    return subAgents.filter((s) =>
      (s.title || '').toLowerCase().includes(term),
    );
  }, [subAgents, searchTerm]);

  const handleFocusChange = useCallback((item: SubAgent | null) => {
    setFocusedRow(item);
  }, []);

  // Item focado VÁLIDO: só conta se ainda estiver na lista filtrada. Ao mudar o
  // termo de busca, o item antes focado pode não estar mais em displayItems — o
  // DataGrid não zera o foco nesse caso, então a ação "Abrir" não pode operar
  // sobre um item oculto.
  const focusedItem = useMemo(
    () =>
      focusedRow && displayItems.some((s) => s.conversationId === focusedRow.conversationId)
        ? focusedRow
        : null,
    [focusedRow, displayItems],
  );

  // Reconcilia o estado: se o item focado saiu da lista filtrada, limpa o foco
  // (mantém o estado consistente além da guarda derivada acima).
  useEffect(() => {
    if (focusedRow && !displayItems.some((s) => s.conversationId === focusedRow.conversationId)) {
      setFocusedRow(null);
    }
  }, [displayItems, focusedRow]);

  const columns: DataGridColumn<SubAgent>[] = [
    {
      key: 'title',
      label: t('subagents.colTitle'),
      width: '34%',
      format: (_value, item) => item.title || t('subagents.untitled'),
    },
    {
      key: 'latestStatus',
      label: t('subagents.colStatus'),
      width: '16%',
      format: (_value, item) => {
        const key = KNOWN_STATUSES.has(item.latestStatus) ? item.latestStatus : 'unknown';
        return (
          <span className={`subagents-page__status subagents-page__status--${key}`}>
            <span className="subagents-page__status-dot" aria-hidden="true" />
            {statusLabel(item.latestStatus, t)}
            {item.background ? (
              <span className="subagents-page__badge">{t('subagents.background')}</span>
            ) : null}
          </span>
        );
      },
    },
    {
      key: 'runCount',
      label: t('subagents.colRuns'),
      width: '10%',
      format: (value) => String(value || 0),
    },
    {
      key: 'messageCount',
      label: t('subagents.colMessages'),
      width: '12%',
      format: (value) => String(value || 0),
    },
    {
      key: 'totalTokens',
      label: t('subagents.colTokens'),
      width: '14%',
      format: (value) => String(value || 0),
    },
    {
      key: 'updatedAt',
      label: t('subagents.colUpdated'),
      width: '14%',
      format: (value) => {
        const dateValue = value instanceof Date || typeof value === 'string' || typeof value === 'number'
          ? value
          : undefined;
        const timestamp = dateValue ? new Date(dateValue).getTime() : 0;
        return formatRelativeTimeLocalized(timestamp, i18n.language);
      },
    },
  ];

  if (loading) {
    return (
      <div className="subagents-page">
        <div className="loading">{t('subagents.loading')}</div>
      </div>
    );
  }

  if (loadError) {
    return (
      <div className="subagents-page">
        <div className="subagents-page__error" role="alert">
          <p className="subagents-page__error-message">{t('subagents.errorLoading')}</p>
          <Button variant="secondary" onClick={() => void load()}>
            {t('subagents.retry')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="subagents-page">
      <Toolbar
        ariaLabel={t('subagents.toolbarLabel')}
        left={<h1 className="page-toolbar__title">{t('subagents.pageTitle')}</h1>}
        searchPlaceholder={t('subagents.search')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        actions={[
          {
            key: 'refresh',
            label: t('subagents.refresh'),
            icon: <ReloadOutlined />,
            onClick: () => void load(),
            variant: 'secondary',
          },
          {
            key: 'open',
            label: t('subagents.open'),
            icon: <FolderOpenOutlined />,
            onClick: () => focusedItem && handleOpen(focusedItem.conversationId, focusedItem.title),
            disabled: !focusedItem,
            variant: 'primary',
          },
        ]}
      />

      {displayItems.length === 0 ? (
        <p className="subagents-page__empty">{t('subagents.empty')}</p>
      ) : (
        <DataGrid
          items={displayItems}
          columns={columns}
          label={t('subagents.gridLabel')}
          getItemId={(item) => item.conversationId}
          onActivate={(item: SubAgent) => handleOpen(item.conversationId, item.title)}
          onGridReady={handleGridReady}
          onFocusChange={handleFocusChange}
        />
      )}
    </div>
  );
}
