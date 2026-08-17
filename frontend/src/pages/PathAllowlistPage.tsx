import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons';
import { GetPathAllowlist, RemovePathAllowlistEntry } from '@wailsjs/go/wailsapi/FSTrust';

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
import './PathAllowlistPage.css';

interface PathAllowlistRow {
  id: string;
  path: string;
  kind: string;
  operation: string;
  scope: string;
  createdBy: string;
  createdAt: string;
  reason: string;
  [key: string]: unknown;
}

const KNOWN_SCOPES = new Set(['session', 'workspace', 'profile', 'global']);
const KNOWN_KINDS = new Set(['file', 'dir']);

/** Data no idioma de quem lê; o que não for data válida aparece como veio. */
function formatCreatedAt(value: string, language: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(language);
}

export default function PathAllowlistPage() {
  const { t, i18n } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'path-allowlist-page' });
  const requestConfirm = useConfirm();

  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState<PathAllowlistRow[]>([]);
  const [loadFailed, setLoadFailed] = useState(false);
  const [focused, setFocused] = useState<PathAllowlistRow | null>(null);

  const scopeName = useCallback(
    (scope: string) =>
      KNOWN_SCOPES.has(scope)
        ? t(`pathAllowlist.scope.${scope}`)
        : t('pathAllowlist.scope.unknown'),
    [t],
  );

  const kindName = useCallback(
    (kind: string) =>
      KNOWN_KINDS.has(kind) ? t(`pathAllowlist.kind.${kind}`) : t('pathAllowlist.kind.unknown'),
    [t],
  );

  const mapEntries = useCallback(
    (entries: Array<{
      path: string;
      kind: string;
      operation: string;
      scope: string;
      createdBy?: string;
      createdAt: string;
      reason?: string;
    }>) =>
      entries.map((entry) => ({
        id: `${entry.scope}:${entry.kind}:${entry.operation}:${entry.path}`,
        path: entry.path,
        kind: entry.kind,
        operation: entry.operation,
        scope: entry.scope,
        createdBy: entry.createdBy ?? '',
        createdAt: entry.createdAt,
        reason: entry.reason ?? '',
      })),
    [],
  );

  const load = useCallback(async () => {
    setLoading(true);
    // A entrada sob o foco pode não existir depois desta carga; manter a marca
    // deixaria a barra oferecendo "revogar" sobre algo que saiu da lista.
    setFocused(null);
    try {
      const entries = (await GetPathAllowlist()) ?? [];
      setLoadFailed(false);
      setRows(mapEntries(entries));
    } catch (error) {
      logger.error('Erro ao carregar allowlist de path:', error);
      addToast(t('pathAllowlist.error.loadFailed'), 'error');
      setLoadFailed(true);
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [addToast, mapEntries, t]);

  useEffect(() => {
    load();
  }, [load]);

  const remove = useCallback(
    async (row: PathAllowlistRow) => {
      const scope = scopeName(row.scope);
      const confirmed = await requestConfirm({
        title: t('pathAllowlist.confirm.title'),
        message: t('pathAllowlist.confirm.message', { path: row.path, scope }),
        confirmText: t('pathAllowlist.actions.remove'),
        cancelText: t('common.cancel'),
        variant: 'danger',
      });
      if (!confirmed) {
        return;
      }

      try {
        await RemovePathAllowlistEntry(row.scope, row.path, row.kind, row.operation);
        // Remoção já valeu no backend: tira a linha localmente antes de
        // sincronizar. Se a sincronização falhar, a grade otimista permanece
        // (não cair em loadFailedBody, que contradiria o toast de sucesso).
        setRows((current) => current.filter((item) => item.id !== row.id));
        setFocused((current) => (current?.id === row.id ? null : current));
        setLoadFailed(false);
        addToast(t('pathAllowlist.toast.removed'), 'success', undefined, undefined, {
          suppressAnnounce: true,
        });
        announce(t('pathAllowlist.announce.removed', { path: row.path, scope }));
        try {
          const entries = (await GetPathAllowlist()) ?? [];
          setRows(mapEntries(entries));
        } catch (error) {
          logger.error('Erro ao sincronizar allowlist de path após remoção:', error);
          addToast(t('pathAllowlist.error.reloadAfterRemoveFailed'), 'warning');
        }
      } catch {
        addToast(t('pathAllowlist.error.removeFailed'), 'error');
      }
    },
    [addToast, announce, mapEntries, requestConfirm, scopeName, t],
  );

  const rowActions = useCallback(
    (row: PathAllowlistRow) => [
      {
        id: 'remove',
        label: t('pathAllowlist.actions.remove'),
        icon: <DeleteOutlined aria-hidden="true" />,
        onClick: () => remove(row),
        danger: true,
      },
    ],
    [remove, t],
  );

  const columns: DataGridColumn<PathAllowlistRow>[] = [
    { key: 'path', label: t('pathAllowlist.columns.path'), width: '280px', truncate: true },
    {
      key: 'kind',
      label: t('pathAllowlist.columns.kind'),
      width: '120px',
      format: (_value, row) => kindName(row.kind),
    },
    { key: 'operation', label: t('pathAllowlist.columns.operation'), width: '120px' },
    {
      key: 'scope',
      label: t('pathAllowlist.columns.scope'),
      width: '150px',
      format: (_value, row) => scopeName(row.scope),
    },
    { key: 'createdBy', label: t('pathAllowlist.columns.createdBy'), width: '150px', truncate: true },
    {
      key: 'createdAt',
      label: t('pathAllowlist.columns.createdAt'),
      width: '190px',
      format: (_value, row) => formatCreatedAt(row.createdAt, i18n.language),
    },
    { key: 'reason', label: t('pathAllowlist.columns.reason'), truncate: true },
    {
      key: 'actions',
      label: t('common.actions'),
      width: '80px',
      format: (_value, row) => (
        <MenuButton items={rowActions(row)} buttonLabel={t('common.actions')} />
      ),
    },
  ];

  const getRowId = useCallback((row: PathAllowlistRow) => row.id, []);
  const handleFocusChange = useCallback(
    (row: PathAllowlistRow | null) => setFocused(row),
    [],
  );

  const toolbarActions = [
    {
      key: 'remove',
      label: t('pathAllowlist.actions.remove'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => focused && remove(focused),
      disabled: !focused,
      variant: 'danger' as const,
    },
    {
      key: 'reload',
      label: t('pathAllowlist.actions.reload'),
      icon: <ReloadOutlined aria-hidden="true" />,
      onClick: load,
      disabled: false,
      variant: 'secondary' as const,
    },
  ];

  if (loading) {
    return (
      <div className="path-allowlist-page">
        <PageLoading
          className="path-allowlist-page__loading"
          message={t('pathAllowlist.loading')}
        />
      </div>
    );
  }

  return (
    <div className="path-allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('pathAllowlist.title')}</h1>}
        actions={toolbarActions}
        ariaLabel={t('pathAllowlist.toolbarLabel')}
      />
      <div className="path-allowlist-page__content">
        <p className="path-allowlist-page__description">{t('pathAllowlist.description')}</p>
        <p className="path-allowlist-page__note">{t('pathAllowlist.sessionNote')}</p>
        {loadFailed ? (
          // Cair no texto de lista vazia depois de uma falha diria que nenhum
          // path está autorizado. Os que existirem continuam valendo.
          <p className="path-allowlist-page__empty">{t('pathAllowlist.loadFailedBody')}</p>
        ) : rows.length > 0 ? (
          <DataGrid
            items={rows}
            columns={columns}
            label={t('pathAllowlist.gridLabel')}
            autoFocusOnMount={false}
            getItemId={getRowId}
            onGridReady={handleGridReady}
            getRowActions={rowActions}
            onFocusChange={handleFocusChange}
          />
        ) : (
          <p className="path-allowlist-page__empty">{t('pathAllowlist.empty')}</p>
        )}
      </div>
    </div>
  );
}
