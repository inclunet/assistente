import { apidto } from '@wailsjs/go/models';
import { ChatGPTConnection } from '../components/settings/ChatGPTConnection';
import { logger } from '../utils/logger';
import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  StarFilled,
  StarOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  ACPAgentInstallPlan,
  CanRemoveACPAgent,
  RemoveACPAgent,
} from '@wailsjs/go/wailsapi/ACPInstall';
import {
  GetLLMProvidersWithStatus,
  CreateLLMProvider,
  DeleteLLMProvider,
  SetDefaultProvider,
} from '@wailsjs/go/wailsapi/LLMProviders';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { DecisionDialog } from '../components/ui/DecisionDialog';
import { Toolbar } from '../components/ui/Toolbar';
import { Modal } from '../components/ui/Modal';
import { ProviderForm, ProviderFormData } from '../components/settings/ProviderForm';
import { AGENT_API_FORMAT } from '../config/providers';
import { requestACPAgentUpdate } from '../services/acpInstall';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import { useConfirm } from '../hooks/useConfirm';
import { useLocation } from 'react-router-dom';
import { useProviderCreationCommands, type ProviderCreationKind } from '../lib/providerCreationCommands';
import { useCommandShortcutHint } from '../lib/commandShortcutHints';
import './ProvidersPage.css';

interface Provider {
  auth_mode?: string;
  credential_pattern?: string;
  reasoning_content_mode?: string;
  id: string;
  name: string;
  type: string;
  api_format?: string;
  base_url: string;
  credential_required: boolean;
  credential_status: 'none' | 'configured' | 'missing';
  credential_domain_patterns?: string[];
  is_default?: boolean;
  default_model?: string;
  /**
   * Comando e argumentos do agente de código (AEP-0084 D12). Para um provedor
   * ACP é isto que faz o papel de `base_url`: sem os dois, o que chega ao
   * formulário não descreve o provedor salvo.
   */
  acp_command?: string;
  acp_args?: string[];
  /** Qual agente do registro é o provedor (AEP-0086 D11). */
  acp_agent_id?: string;
  /** Ambiente do processo do agente (inclui env{} do binário instalado). */
  acp_env?: Record<string, string>;
  /**
   * Quais variáveis do ambiente do agente recebem credencial do cofre, e de
   * qual entrada (AEP-0086 D12). Referência, nunca o segredo.
   */
  acp_credential_env?: Record<string, string>;
}

interface ProviderRow extends Provider {
  statusText: string;
}

type InstallPlan = apidto.ACPInstallPlan;

export default function ProvidersPage() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const pageRef = useRef<HTMLDivElement>(null);
  const newShortcut = useCommandShortcutHint('providers.create.open', 'providers', 'settings');
  const [creationKind, setCreationKind] = useState<'api' | 'acp'>('api');
  const confirm = useConfirm();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'providers-page' });

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  const [providers, setProviders] = useState<ProviderRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [providerLoadSucceeded, setProviderLoadSucceeded] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [isEditing, setIsEditing] = useState(false);
  const [providerSaving, setProviderSaving] = useState(false);
  const [oauthCloseBlocked, setOAuthCloseBlocked] = useState(false);
  const [oauthDialog, setOAuthDialog] = useState<{ id?: string } | null>(null);
  const [editingProvider, setEditingProvider] = useState<ProviderFormData | undefined>(undefined);
  const [focusedRow, setFocusedRow] = useState<ProviderRow | null>(null);
  const [updatePlans, setUpdatePlans] = useState<Record<string, InstallPlan>>({});
  const [updateTarget, setUpdateTarget] = useState<{ provider: ProviderRow; plan: InstallPlan } | null>(null);
  const updatePlanSeq = useRef(0);

  const loadUpdatePlans = async (items: ProviderRow[]) => {
    const seq = ++updatePlanSeq.current;
    setUpdatePlans({});
    const agentIDs = Array.from(new Set(
      items
        .filter((provider) => provider.api_format === AGENT_API_FORMAT)
        .map((provider) => (provider.acp_agent_id || '').trim())
        .filter(Boolean),
    ));
    const plans = await Promise.allSettled(
      agentIDs.map(async (agentID) => [agentID, await ACPAgentInstallPlan(agentID)] as const),
    );
    if (seq !== updatePlanSeq.current) return;
    setUpdatePlans(Object.fromEntries(
      plans.flatMap((result) => result.status === 'fulfilled' ? [result.value] : []),
    ));
  };

  const loadProviders = async () => {
    setLoading(true);
    try {
      const result = await GetLLMProvidersWithStatus();
      const items = (result || []) as Provider[];
      const mapped = items.map((p) => ({
        ...p,
        id: p.id,
        statusText: getStatusText(p.credential_status),
      })) as ProviderRow[];
      setProviders(mapped);
      setProviderLoadSucceeded(true);
      void loadUpdatePlans(mapped);
    } catch (error) {
      setProviderLoadSucceeded(false);
      logger.error('Erro ao carregar provedores:', error);
      addToast(t('providers.error.loadFailed', 'Erro ao carregar provedores'), 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProviders();
  }, []);

  useResourceEditRequest('providers', {
    onEdit: (id) => {
      const found = providers.find((p) => p.id === id);
      if (found) handleEditProvider(found);
    },
    onNew: () => handleAddProvider(),
    ready: !loading && providerLoadSucceeded,
  });

  const getStatusText = (status: string): string => {
    switch (status) {
      case 'configured':
        return t('providers.status.configured', 'Configurado');
      case 'missing':
        return t('providers.status.missing', 'Credencial faltando');
      case 'none':
      default:
        return t('providers.status.none', 'Não requer');
    }
  };

  const openCreation = (kind: ProviderCreationKind) => {
    if (kind === 'chatgpt') { setOAuthDialog({}); return; }
    setCreationKind(kind);
    setEditingProvider(undefined);
    setIsEditing(true);
  };
  const creation = useProviderCreationCommands({
    root: pageRef, pathname,
    ready: !loading && !isEditing && !oauthDialog && !updateTarget,
    open: openCreation,
  });
  const handleAddProvider = () => creation.requestOpen();

  const handleEditProvider = useCallback((provider: ProviderRow) => {
 if (provider.type === 'chatgpt') { setOAuthDialog({id: provider.id}); return; }
    setCreationKind(provider.api_format === AGENT_API_FORMAT ? 'acp' : 'api');
    setEditingProvider({
      id: provider.id,
      name: provider.name,
      type: provider.type,
      base_url: provider.base_url,
      api_key: '',
      auth_mode: provider.auth_mode,
      credential_pattern: provider.credential_pattern,
      reasoning_content_mode: provider.reasoning_content_mode,
      default_model: (provider as Provider).default_model || '',
      api_format: (provider as Provider).api_format || '',
      acp_command: provider.acp_command || '',
      acp_args: provider.acp_args || [],
      acp_agent_id: provider.acp_agent_id || '',
      acp_env: provider.acp_env || {},
      acp_credential_env: provider.acp_credential_env || {},
    });
    setIsEditing(true);
  }, []);

  const handleSetDefault = async (provider: ProviderRow) => {
    try {
      await SetDefaultProvider(provider.id);
      addToast(t('providers.toast.defaultSet', { name: provider.name }), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('providers.toast.defaultSet', { name: provider.name }));
      await loadProviders();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('providers.error.defaultFailed', 'Erro ao definir padrão'), 'error');
    }
  };

  const getDuplicateName = (name: string) => {
    const base = `${name} (Copia)`;
    const existing = new Set(providers.map((item) => item.name.toLowerCase()));
    if (!existing.has(base.toLowerCase())) return base;
    let index = 2;
    while (existing.has(`${base} ${index}`.toLowerCase())) {
      index += 1;
    }
    return `${base} ${index}`;
  };

  const handleDuplicateProvider = async (provider: ProviderRow) => {
 if (provider.type === 'chatgpt') { setOAuthDialog({}); return; }
    try {
      const name = getDuplicateName(provider.name);
      await CreateLLMProvider(apidto.CreateLLMProviderRequest.createFrom({
        id: `${provider.type}-${Date.now()}`,
        name,
        credential_from_provider_id: provider.api_format === AGENT_API_FORMAT ? undefined : provider.id,
        type: provider.type,
        base_url: provider.base_url,
        api_format: (provider as Provider).api_format || undefined,
        auth_mode: provider.api_format === AGENT_API_FORMAT ? undefined : provider.auth_mode,
        // A cópia de um agente precisa subir o mesmo agente: o backend recusa o
        // formato acp sem comando, e sem argumentos o processo subiria em outro
        // modo que não o do original.
        acp_command: provider.acp_command || undefined,
        acp_args: provider.acp_args || undefined,
        // Sem o id do agente a cópia perderia de qual linha do registro ela
        // veio, e a tela de provedor não teria o que oferecer de catálogo.
        acp_agent_id: provider.acp_agent_id || undefined,
        // ACPEnv não atravessa Create: o backend reaplica o env do binário
        // instalado a partir do acp_agent_id / installed.json.
        // A passagem de credencial vai junto porque é referência: a cópia sobe
        // o mesmo agente e precisa da mesma chave, e o segredo continua onde
        // sempre esteve, no cofre.
        acp_credential_env: provider.acp_credential_env || undefined,
      }));
      addToast(t('providers.toast.duplicated'), 'success', undefined, undefined, { suppressAnnounce: true });
      announce(t('providers.toast.duplicated'));
      await loadProviders();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('providers.error.duplicateFailed'), 'error');
    }
  };

  const handleUpdateAgent = (provider: ProviderRow) => {
    if (provider.api_format !== AGENT_API_FORMAT) return;
    const agentID = (provider.acp_agent_id || '').trim();
    const plan = updatePlans[agentID];
    if (!agentID || !plan?.update || !plan.can_update) return;
    setUpdateTarget({ provider, plan });
  };

  const confirmUpdateAgent = async (acceptUnverified: boolean) => {
    const target = updateTarget;
    if (!target) return;
    setUpdateTarget(null);
    try {
      const installation = await requestACPAgentUpdate(target.plan, acceptUnverified);
      const message = t('providers.toast.agentUpdated', {
        name: target.provider.name,
        version: installation.version,
      });
      addToast(message, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(message);
      await loadProviders();
    } catch (error: unknown) {
      const message = t('providers.error.updateAgentFailed', {
        reason: getErrorMessage(error) || t('providers.error.updateAgentFailedUnknown'),
      });
      addToast(message, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(message, 'assertive');
    }
  };

  const handleDeleteProvider = async (provider: ProviderRow) => {
    const confirmed = await confirm({
      title: t('providers.confirm.deleteTitle'),
      message: t('providers.confirm.deleteMessage', { name: provider.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    const removedAgentID = provider.api_format === AGENT_API_FORMAT
      ? (provider.acp_agent_id || '').trim()
      : '';
    try {
      await DeleteLLMProvider(provider.id);
      addToast(t('providers.toast.deleted'), 'success', undefined, undefined, { suppressAnnounce: true });
      announce(t('providers.toast.deleted'));
      await loadProviders();
    } catch (error: unknown) {
      const message = getErrorMessage(error);
      const errorKey = message.includes('oauth_vault_persistence_required') || message.includes('oauth_vault_unavailable')
        ? 'chatgpt.vaultUnavailable'
        : message.includes('oauth_authorization_changed') ? 'chatgpt.authorizationChanged'
        : message.includes('chatgpt_authorization_in_progress') ? 'chatgpt.authorizationInProgress'
        : message.includes('chatgpt_disconnect_before_delete') ? 'chatgpt.disconnectBeforeDelete' : '';
      addToast(errorKey ? t(errorKey) : (message || t('providers.error.deleteFailed')), 'error');
      return;
    }

    if (!removedAgentID) return;
    try {
      const canRemove = await CanRemoveACPAgent(removedAgentID);
      if (!canRemove) return;
      const removeAgent = await confirm({
        title: t('providers.confirm.removeUnusedAgentTitle'),
        message: t('providers.confirm.removeUnusedAgentMessage', { agent: removedAgentID }),
        confirmText: t('providers.confirm.removeUnusedAgentConfirm'),
        cancelText: t('providers.confirm.keepAgent'),
        variant: 'danger',
      });
      if (!removeAgent) return;
      await RemoveACPAgent(removedAgentID);
      addToast(t('providers.toast.agentRemoved'), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('providers.toast.agentRemoved'));
    } catch (error: unknown) {
      addToast(
        getErrorMessage(error) || t('providers.error.removeUnusedAgentFailed'),
        'error',
      );
    }
  };

  const handleSaveSuccess = async () => {
    setIsEditing(false);
    setEditingProvider(undefined);
    await loadProviders();
  };

  const handleCancelEdit = () => {
    setIsEditing(false);
    setEditingProvider(undefined);
  };

  const columns: DataGridColumn<ProviderRow>[] = [
    {
      key: 'name',
      label: t('providers.columns.name', 'Nome'),
      width: '25%',
      format: (value, row) => (
        <span>
          {String(value || '')}
          {(row as Provider).is_default && (
            <span className="provider-badge-default" title={t('providers.badge.default', 'Provedor padrão')}>
              {' '}
              <StarFilled aria-hidden="true" /> {t('providers.badge.default', 'Padrão')}
            </span>
          )}
        </span>
      ),
    },
    {
      key: 'type',
      label: t('providers.columns.type', 'Tipo'),
      width: '15%',
    },
    {
      key: 'base_url',
      label: t('providers.columns.baseUrl', 'Base URL'),
      width: '40%',
    },
    {
      key: 'statusText',
      label: t('providers.columns.status', 'Status Credencial'),
      width: '15%',
      format: (value, row: ProviderRow) => (
        <span
          className={`provider-status provider-status--${row.credential_status}`}
        >
          {String(value || '')}
        </span>
      ),
    },
    {
      key: 'actions',
      label: '',
      width: '5%',
      format: (_value, item) => (
        <MenuButton
          items={getProviderRowActions(item)}
          buttonLabel={t('providers.actions.actions', 'Ações')}
        />
      ),
    },
  ];
  // Gera as ações contextuais para cada linha de provedor
  function getProviderRowActions(item: ProviderRow) {
    const agentID = (item.acp_agent_id || '').trim();
    const updatePlan = updatePlans[agentID];
    const canUpdate = item.api_format === AGENT_API_FORMAT
      && !!agentID
      && !!updatePlan?.update
      && !!updatePlan.can_update;
    const actions = [
      {
        id: 'edit',
        label: t('providers.actions.edit', 'Editar'),
        icon: <EditOutlined aria-hidden="true" />,
        onClick: () => handleEditProvider(item),
      },
      ...((item as Provider).is_default ? [] : [{
        id: 'setDefault',
        label: t('providers.actions.setDefault', 'Tornar Padrão'),
        icon: <StarOutlined aria-hidden="true" />,
        onClick: () => handleSetDefault(item),
      }]),
      {
        id: 'duplicate',
        label: t('providers.actions.duplicate', 'Duplicar'),
        icon: <CopyOutlined aria-hidden="true" />,
        onClick: () => handleDuplicateProvider(item),
      },
      {
        id: 'updateAgent',
        label: t('providers.actions.updateAgent', 'Atualizar agente'),
        icon: <ReloadOutlined aria-hidden="true" />,
        onClick: () => handleUpdateAgent(item),
        disabled: !canUpdate,
      },
      {
        id: 'delete',
        label: t('providers.actions.delete', 'Excluir'),
        icon: <DeleteOutlined aria-hidden="true" />,
        onClick: () => handleDeleteProvider(item),
        danger: true,
      },
    ];
    return actions;
  }

  const filteredRows = useMemo(
    () =>
      providers.filter((row) =>
        searchTerm === ''
          ? true
          : row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
            row.type.toLowerCase().includes(searchTerm.toLowerCase()) ||
            row.base_url.toLowerCase().includes(searchTerm.toLowerCase())
      ),
    [providers, searchTerm]
  );

  const actionTarget = filteredRows.find(row => row.id === focusedRow?.id);

  const getRowId = useCallback((item: ProviderRow) => item.id, []);
  const handleFocusChange = useCallback((item: ProviderRow | null) => setFocusedRow(item), []);

  return (
    <div className="providers-page" ref={pageRef}>
      {creation.menu}
      {loading && <div className="loading">{t('providers.loading', 'Carregando...')}</div>}
          <Modal isOpen={oauthDialog !== null} allowClose={!oauthCloseBlocked} onClose={() => { if (!oauthCloseBlocked) { setOAuthDialog(null); void loadProviders(); } }} title={t('chatgpt.title')} size="md">
            {oauthDialog && <ChatGPTConnection onCloseBlockedChange={setOAuthCloseBlocked} id={oauthDialog.id} onClose={() => setOAuthDialog(null)} onChanged={() => void loadProviders()} />}
          </Modal>
      {!loading && (
        <>
          <Toolbar
            left={<h2>{t('providers.pageTitle', 'Provedores LLM')}</h2>}
            searchPlaceholder={t('providers.search', 'Buscar provedores...')}
            searchValue={searchTerm}
            onSearchChange={setSearchTerm}
            rightEnd={<MenuButton
              buttonLabel={t('providers.actions.actions', 'Ações')}
              contextKey={JSON.stringify([actionTarget, updatePlans, searchTerm])}
              items={actionTarget
                ? getProviderRowActions(actionTarget)
                : [
                  { id: 'edit', label: t('providers.actions.edit', 'Editar'), disabled: true },
                  { id: 'duplicate', label: t('providers.actions.duplicate', 'Duplicar'), disabled: true },
                  { id: 'delete', label: t('providers.actions.delete', 'Excluir'), disabled: true },
                ]}
            />}
            actions={[
              {
                key: 'add',
                label: t('providers.creation.title'),
                onClick: handleAddProvider,
                shortcut: newShortcut,
                buttonRef: creation.buttonRef,
                'aria-haspopup': 'menu',
                'aria-expanded': creation.isOpen,
                variant: 'primary',
              },
            ]}
          />

          <DataGrid
            columns={columns}
            items={filteredRows}
            selectedIds={selectedIds}
            onSelectionChange={setSelectedIds}
            onActivate={handleEditProvider}
            onGridReady={handleGridReady}
            label={t('providers.gridLabel', 'Lista de provedores LLM')}
            getItemId={getRowId}
            getRowActions={getProviderRowActions}
            onFocusChange={handleFocusChange}
          />


          <Modal
            isOpen={isEditing}
            allowClose={!providerSaving}
            onClose={() => { if (!providerSaving) handleCancelEdit(); }}
            title={editingProvider?.id
              ? t('providers.modal.editTitle', 'Editar Provedor')
              : t('providers.modal.newTitle', 'Novo Provedor')}
            size="md"
          >
            <div className="providers-editor">
              <ProviderForm
                provider={editingProvider}
                kind={creationKind}
                onBusyChange={setProviderSaving}
                onSave={handleSaveSuccess}
                onCancel={handleCancelEdit}
              />
            </div>
          </Modal>

          <DecisionDialog
            isOpen={!!updateTarget}
            onCancel={() => setUpdateTarget(null)}
            title={t('providerForm.agent.catalog.confirm.titleUpdate', {
              agent: updateTarget?.plan.name || updateTarget?.provider.name || '',
              version: updateTarget?.plan.version || '',
            })}
            description={t('providerForm.agent.catalog.confirm.introUpdate')}
            severity={updateTarget?.plan.unverified ? 'permission' : 'info'}
            actions={[
              {
                id: 'confirm',
                label: t(
                  updateTarget?.plan.unverified
                    ? 'providerForm.agent.catalog.confirm.confirmUpdateUnverifiedBtn'
                    : 'providerForm.agent.catalog.confirm.confirmUpdateBtn',
                ),
                primary: true,
                variant: 'primary',
                polarity: 'affirmative',
                scope: 'current',
              },
              {
                id: 'cancel',
                label: t('providerForm.agent.catalog.confirm.cancelBtn'),
                variant: 'outline',
                polarity: 'negative',
                scope: 'current',
              },
            ]}
            onAction={(actionId) => {
              if (actionId === 'confirm') {
                void confirmUpdateAgent(!!updateTarget?.plan.unverified);
              } else {
                setUpdateTarget(null);
              }
            }}
            body={updateTarget ? (
              <dl>
                <dt>{t('providerForm.agent.catalog.confirm.agent')}</dt>
                <dd>{updateTarget.plan.name || updateTarget.provider.name}</dd>
                <dt>{t('providerForm.agent.catalog.confirm.installedVersion')}</dt>
                <dd>{updateTarget.plan.installed?.version}</dd>
                <dt>{t('providerForm.agent.catalog.confirm.newVersion')}</dt>
                <dd>{updateTarget.plan.version}</dd>
                <dt>{t('providerForm.agent.catalog.confirm.origin')}</dt>
                <dd>{updateTarget.plan.origin}</dd>
              </dl>
            ) : undefined}
          />

        </>
      )}
    </div>
  );
}
