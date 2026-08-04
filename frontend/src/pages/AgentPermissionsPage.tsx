import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons';
import { GetAgentPermissions, RevokeAgentPermission } from '@wailsjs/go/app/App';

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
import { agentActionClassName } from '../lib/agentAction';
import './AgentPermissionsPage.css';

interface PermissionRow {
  id: string;
  profileSlug: string;
  profileName: string;
  action: string;
  grantedAt: string;
}

/**
 * Data no idioma de quem lê. Data crua em ISO é ruído para leitor de telas, e a
 * data que não puder ser interpretada aparece como veio — melhor um texto
 * estranho do que uma célula vazia numa linha que a pessoa precisa reconhecer.
 */
function formatGrantedAt(value: string, language: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(language);
}

export default function AgentPermissionsPage() {
  const { t, i18n } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'agent-permissions-page' });
  const requestConfirm = useConfirm();

  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState<PermissionRow[]>([]);
  const [loadFailed, setLoadFailed] = useState(false);
  const [focused, setFocused] = useState<PermissionRow | null>(null);

  const actionName = useCallback((action: string) => agentActionClassName(t, action), [t]);

  const load = useCallback(async () => {
    setLoading(true);
    // A linha que estava sob o foco pode não existir depois desta carga. Manter
    // a marca deixaria a barra oferecendo "revogar" sobre algo que saiu da
    // lista, e quem navega por teclado só descobriria isso pelo erro.
    setFocused(null);
    try {
      const permissions = (await GetAgentPermissions()) ?? [];
      setLoadFailed(false);
      setRows(
        permissions.map((permission) => ({
          id: `${permission.profileSlug}:${permission.action}`,
          profileSlug: permission.profileSlug,
          // Perfil apagado deixa a autorização para trás; o slug é o que sobrou
          // dele, e é por ele que a pessoa a reconhece para revogar.
          profileName: permission.profileName || permission.profileSlug,
          action: permission.action,
          grantedAt: permission.grantedAt,
        })),
      );
    } catch (error) {
      logger.error('Erro ao carregar autorizações do agente:', error);
      addToast(t('agentPermissions.error.loadFailed'), 'error');
      setLoadFailed(true);
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [addToast, t]);

  useEffect(() => {
    load();
  }, [load]);

  const revoke = useCallback(
    async (row: PermissionRow) => {
      const action = actionName(row.action);
      const confirmed = await requestConfirm({
        title: t('agentPermissions.confirm.title'),
        message: t('agentPermissions.confirm.message', { action, profile: row.profileName }),
        confirmText: t('agentPermissions.actions.revoke'),
        cancelText: t('common.cancel'),
        variant: 'danger',
      });
      if (!confirmed) {
        return;
      }

      try {
        await RevokeAgentPermission(row.profileSlug, row.action);
        addToast(t('agentPermissions.toast.revoked'), 'success', undefined, undefined, {
          suppressAnnounce: true,
        });
        announce(t('agentPermissions.announce.revoked', { action, profile: row.profileName }));
        await load();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error ?? '');
        addToast(message || t('agentPermissions.error.revokeFailed'), 'error');
      }
    },
    [actionName, addToast, announce, load, requestConfirm, t],
  );

  const rowActions = useCallback(
    (row: PermissionRow) => [
      {
        id: 'revoke',
        label: t('agentPermissions.actions.revoke'),
        icon: <DeleteOutlined aria-hidden="true" />,
        onClick: () => revoke(row),
        danger: true,
      },
    ],
    [revoke, t],
  );

  const columns: DataGridColumn<PermissionRow>[] = [
    { key: 'profileName', label: t('agentPermissions.columns.profile'), width: '200px', truncate: true },
    {
      key: 'action',
      label: t('agentPermissions.columns.action'),
      width: '220px',
      format: (_value, row) => actionName(row.action),
    },
    {
      key: 'grantedAt',
      label: t('agentPermissions.columns.grantedAt'),
      width: '200px',
      format: (_value, row) => formatGrantedAt(row.grantedAt, i18n.language),
    },
    {
      key: 'actions',
      label: t('common.actions'),
      width: '80px',
      format: (_value, row) => (
        <MenuButton items={rowActions(row)} buttonLabel={t('common.actions')} />
      ),
    },
  ];

  const getRowId = useCallback((row: PermissionRow) => row.id, []);
  const handleFocusChange = useCallback((row: PermissionRow | null) => setFocused(row), []);

  const toolbarActions = [
    {
      key: 'revoke',
      label: t('agentPermissions.actions.revoke'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => focused && revoke(focused),
      disabled: !focused,
      variant: 'danger' as const,
    },
    {
      key: 'reload',
      label: t('agentPermissions.actions.reload'),
      icon: <ReloadOutlined aria-hidden="true" />,
      onClick: load,
      disabled: false,
      variant: 'secondary' as const,
    },
  ];

  if (loading) {
    return (
      <div className="agent-permissions-page">
        <PageLoading
          className="agent-permissions-page__loading"
          message={t('agentPermissions.loading')}
        />
      </div>
    );
  }

  return (
    <div className="agent-permissions-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('agentPermissions.title')}</h1>}
        actions={toolbarActions}
        ariaLabel={t('agentPermissions.toolbarLabel')}
      />
      <div className="agent-permissions-page__content">
        <p className="agent-permissions-page__description">{t('agentPermissions.description')}</p>
        {loadFailed ? (
          // Cair no texto de lista vazia depois de uma falha diria que não há
          // autorização nenhuma. As que existirem continuam valendo, e quem
          // acreditasse na tela não iria procurá-las.
          <p className="agent-permissions-page__empty">{t('agentPermissions.loadFailedBody')}</p>
        ) : rows.length > 0 ? (
          <DataGrid
            items={rows}
            columns={columns}
            label={t('agentPermissions.gridLabel')}
            autoFocusOnMount={false}
            getItemId={getRowId}
            onGridReady={handleGridReady}
            getRowActions={rowActions}
            onFocusChange={handleFocusChange}
          />
        ) : (
          <p className="agent-permissions-page__empty">{t('agentPermissions.empty')}</p>
        )}
      </div>
    </div>
  );
}
