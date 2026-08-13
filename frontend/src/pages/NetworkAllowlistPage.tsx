import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons';
import { GetNetworkAllowlist, RemoveNetworkAllowlistEntry } from '@wailsjs/go/app/App';

import { logger } from '../utils/logger';
import { useUIStore } from '../store/uiStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { PageLoading } from '../components/ui/PageLoading';
import './NetworkAllowlistPage.css';

interface NetworkAllowlistRow {
  id: string;
  host: string;
  port: string;
  scope: string;
  category: string;
  resolvedIps: string;
  createdBy: string;
  createdAt: string;
  reason: string;
  [key: string]: unknown;
}

const KNOWN_SCOPES = new Set(['session', 'workspace', 'profile', 'global']);

/** Data no idioma de quem lê; o que não for data válida aparece como veio. */
function formatCreatedAt(value: string, language: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(language);
}

export default function NetworkAllowlistPage() {
  const { t, i18n } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'network-allowlist-page' });
  const requestConfirm = useConfirm();

  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState<NetworkAllowlistRow[]>([]);
  const [loadFailed, setLoadFailed] = useState(false);
  const [focused, setFocused] = useState<NetworkAllowlistRow | null>(null);

  const scopeName = useCallback(
    (scope: string) =>
      KNOWN_SCOPES.has(scope)
        ? t(`networkAllowlist.scope.${scope}`)
        : t('networkAllowlist.scope.unknown'),
    [t],
  );

  const load = useCallback(async () => {
    setLoading(true);
    // A entrada sob o foco pode não existir depois desta carga; manter a marca
    // deixaria a barra oferecendo "revogar" sobre algo que saiu da lista.
    setFocused(null);
    try {
      const entries = (await GetNetworkAllowlist()) ?? [];
      setLoadFailed(false);
      setRows(
        entries.map((entry) => ({
          id: `${entry.scope}:${entry.host}:${entry.port ?? ''}`,
          host: entry.host,
          port: entry.port ?? '',
          scope: entry.scope,
          category: entry.category ?? '',
          resolvedIps: (entry.resolvedIps ?? []).join(', '),
          createdBy: entry.createdBy ?? '',
          createdAt: entry.createdAt,
          reason: entry.reason ?? '',
        })),
      );
    } catch (error) {
      logger.error('Erro ao carregar allowlist de rede:', error);
      addToast(t('networkAllowlist.error.loadFailed'), 'error');
      setLoadFailed(true);
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [addToast, t]);

  useEffect(() => {
    load();
  }, [load]);

  const remove = useCallback(
    async (row: NetworkAllowlistRow) => {
      const scope = scopeName(row.scope);
      const confirmed = await requestConfirm({
        title: t('networkAllowlist.confirm.title'),
        message: t('networkAllowlist.confirm.message', { host: row.host, scope }),
        confirmText: t('networkAllowlist.actions.remove'),
        cancelText: t('common.cancel'),
        variant: 'danger',
      });
      if (!confirmed) {
        return;
      }

      try {
        await RemoveNetworkAllowlistEntry(row.scope, row.host, row.port);
        addToast(t('networkAllowlist.toast.removed'), 'success', undefined, undefined, {
          suppressAnnounce: true,
        });
        announce(t('networkAllowlist.announce.removed', { host: row.host, scope }));
        await load();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error ?? '');
        addToast(message || t('networkAllowlist.error.removeFailed'), 'error');
      }
    },
    [addToast, announce, load, requestConfirm, scopeName, t],
  );

  const rowActions = useCallback(
    (row: NetworkAllowlistRow) => [
      {
        id: 'remove',
        label: t('networkAllowlist.actions.remove'),
        icon: <DeleteOutlined aria-hidden="true" />,
        onClick: () => remove(row),
        danger: true,
      },
    ],
    [remove, t],
  );

  const columns: DataGridColumn<NetworkAllowlistRow>[] = [
    { key: 'host', label: t('networkAllowlist.columns.host'), width: '220px', truncate: true },
    {
      key: 'port',
      label: t('networkAllowlist.columns.port'),
      width: '140px',
      // Entrada sem porta cobre apenas as portas default (AEP-0082); dizer isso
      // evita que a célula vazia seja lida como "qualquer porta".
      format: (_value, row) => row.port || t('networkAllowlist.defaultPorts'),
    },
    {
      key: 'scope',
      label: t('networkAllowlist.columns.scope'),
      width: '150px',
      format: (_value, row) => scopeName(row.scope),
    },
    { key: 'category', label: t('networkAllowlist.columns.category'), width: '140px' },
    { key: 'resolvedIps', label: t('networkAllowlist.columns.resolvedIps'), width: '180px', truncate: true },
    { key: 'createdBy', label: t('networkAllowlist.columns.createdBy'), width: '150px', truncate: true },
    {
      key: 'createdAt',
      label: t('networkAllowlist.columns.createdAt'),
      width: '190px',
      format: (_value, row) => formatCreatedAt(row.createdAt, i18n.language),
    },
    { key: 'reason', label: t('networkAllowlist.columns.reason'), truncate: true },
    {
      key: 'actions',
      label: t('common.actions'),
      width: '80px',
      format: (_value, row) => (
        <MenuButton items={rowActions(row)} buttonLabel={t('common.actions')} />
      ),
    },
  ];

  const getRowId = useCallback((row: NetworkAllowlistRow) => row.id, []);
  const handleFocusChange = useCallback(
    (row: NetworkAllowlistRow | null) => setFocused(row),
    [],
  );

  const toolbarActions = [
    {
      key: 'remove',
      label: t('networkAllowlist.actions.remove'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => focused && remove(focused),
      disabled: !focused,
      variant: 'danger' as const,
    },
    {
      key: 'reload',
      label: t('networkAllowlist.actions.reload'),
      icon: <ReloadOutlined aria-hidden="true" />,
      onClick: load,
      disabled: false,
      variant: 'secondary' as const,
    },
  ];

  if (loading) {
    return (
      <div className="network-allowlist-page">
        <PageLoading
          className="network-allowlist-page__loading"
          message={t('networkAllowlist.loading')}
        />
      </div>
    );
  }

  return (
    <div className="network-allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('networkAllowlist.title')}</h1>}
        actions={toolbarActions}
        ariaLabel={t('networkAllowlist.toolbarLabel')}
      />
      <div className="network-allowlist-page__content">
        <p className="network-allowlist-page__description">{t('networkAllowlist.description')}</p>
        <p className="network-allowlist-page__note">{t('networkAllowlist.sessionNote')}</p>
        {loadFailed ? (
          // Cair no texto de lista vazia depois de uma falha diria que nenhum
          // host está autorizado. Os que existirem continuam valendo.
          <p className="network-allowlist-page__empty">{t('networkAllowlist.loadFailedBody')}</p>
        ) : rows.length > 0 ? (
          <DataGrid
            items={rows}
            columns={columns}
            label={t('networkAllowlist.gridLabel')}
            autoFocusOnMount={false}
            getItemId={getRowId}
            onGridReady={handleGridReady}
            getRowActions={rowActions}
            onFocusChange={handleFocusChange}
          />
        ) : (
          <p className="network-allowlist-page__empty">{t('networkAllowlist.empty')}</p>
        )}
      </div>
    </div>
  );
}
