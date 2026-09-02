import { useState, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ApiOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { useMCPStore } from '../store/mcpStore';
import { mcp } from '../../wailsjs/go/models';
import {
  SaveMCPServerAuth,
  DeleteMCPServerAuth,
  GetMCPServerAuthInfo,
  DiscoverMCPServerAuth,
  DuplicateMCPServer,
} from '@wailsjs/go/wailsapi/MCP';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { MenuButton } from '../components/layout/MenuButton';
import { Button, PageLoading } from '../components';
import { McpConnectionSection } from '../components/mcp/McpConnectionSection';
import { McpGeneralSection } from '../components/mcp/McpGeneralSection';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { DialogActions } from '../components/ui/DialogActions';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { useUIStore } from '../store/uiStore';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import './McpPage.css';

type ServerInfo = mcp.ServerInfo;
type ServerConfig = mcp.ServerConfig;

interface ServerRow {
  id: string;
  slug: string;
  name: string;
  description: string;
  transport: string;
  status: string;
  toolCount: number;
  enabled: boolean;
  autoConnect: boolean;
  command?: string;
  args?: string[];
  url?: string;
  error?: string;
}

function statusLabel(status: string, t: (key: string) => string): string {
  const labels: Record<string, string> = {
    connected: t('mcp.status.connected'),
    connecting: t('mcp.status.connecting'),
    disconnected: t('mcp.status.disconnected'),
    error: t('mcp.status.error'),
  };
  return labels[status] || status;
}

export default function McpPage() {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'mcp-page' });

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');
  const confirm = useConfirm();

  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [focusedRow, setFocusedRow] = useState<ServerRow | null>(null);

  const {
    servers,
    isLoading,
    loadServers,
    connect,
    disconnect,
    reconnect,
    save,
    remove,
    getConfig,
    setupEventListeners,
  } = useMCPStore();

  // Editor state
  const [editing, setEditing] = useState<ServerConfig | null>(null);
  const [editingSlug, setEditingSlug] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  // Form fields
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formTransport, setFormTransport] = useState('stdio');
  const [formCommand, setFormCommand] = useState('');
  const [formArgs, setFormArgs] = useState('');
  const [formEnvText, setFormEnvText] = useState('');
  const [formUrl, setFormUrl] = useState('');
  const [formEnabled, setFormEnabled] = useState(true);
  const [formAutoConnect, setFormAutoConnect] = useState(true);
  const [formPreferBridge, setFormPreferBridge] = useState(false);

  // Auth fields (armazenados no credential manager, não no config JSON)
  const [formAuthType, setFormAuthType] = useState('none');
  const [formAuthToken, setFormAuthToken] = useState('');
  const [formAuthUsername, setFormAuthUsername] = useState('');
  const [formAuthPassword, setFormAuthPassword] = useState('');
  const [hasExistingAuth, setHasExistingAuth] = useState(false);

  // OAuth2 fields (config JSON para não-sensíveis, credential manager para secrets)
  const [formOAuth2ClientId, setFormOAuth2ClientId] = useState('');
  const [formOAuth2ClientSecret, setFormOAuth2ClientSecret] = useState('');
  const [formOAuth2TokenUrl, setFormOAuth2TokenUrl] = useState('');
  const [formOAuth2AuthUrl, setFormOAuth2AuthUrl] = useState('');
  const [formOAuth2Scopes, setFormOAuth2Scopes] = useState('');
  const [formOAuth2CallbackPort, setFormOAuth2CallbackPort] = useState('');
  const [formOAuth2CallbackHost, setFormOAuth2CallbackHost] = useState('');

  // OAuth auto-discovery
  type DiscoveryStatus = 'idle' | 'loading' | 'found' | 'partial' | 'not_found';
  const [discoveryStatus, setDiscoveryStatus] = useState<DiscoveryStatus>('idle');
  const [discoveredFields, setDiscoveredFields] = useState<Set<string>>(new Set());
  const [discoveryResourceName, setDiscoveryResourceName] = useState('');
  const [discoveryRegistrationUrl, setDiscoveryRegistrationUrl] = useState('');
  const [manualRegistrationUrl, setManualRegistrationUrl] = useState('');
  const [lastDiscoveredUrl, setLastDiscoveredUrl] = useState('');

  useEffect(() => {
    loadServers();
    const cleanup = setupEventListeners();
    return cleanup;
  }, [loadServers, setupEventListeners]);

  // Convert servers to rows (memoizado para evitar re-renders em cascata no DataGrid)
  const rows: ServerRow[] = useMemo(() => (servers || []).map((s: ServerInfo) => ({
    id: s.slug,
    slug: s.slug,
    name: s.name || s.slug,
    description: s.description || '',
    transport: s.transport,
    status: s.status,
    toolCount: s.toolCount,
    enabled: s.enabled,
    autoConnect: s.autoConnect,
    command: s.command,
    args: s.args,
    url: s.url,
    error: s.error,
  })), [servers]);

  const populateForm = (config: ServerConfig | null, _slug: string | null) => {
    setFormName(config?.name || '');
    setFormDescription(config?.description || '');
    setFormTransport(config?.transport || 'stdio');
    setFormCommand(config?.command || '');
    setFormArgs(config?.args?.join(' ') || '');
    setFormEnvText(
      config?.env ? Object.entries(config.env).map(([k, v]) => `${k}=${v}`).join('\n') : ''
    );
    setFormUrl(config?.url || '');
    setFormEnabled(config?.enabled ?? true);
    setFormAutoConnect(config?.auto_connect ?? true);
    setFormPreferBridge(config?.prefer_bridge ?? false);

    setFormAuthToken('');
    setFormAuthUsername('');
    setFormAuthPassword('');
    setFormAuthType(config?.auth_type || 'none');
    setHasExistingAuth(false);

    setFormOAuth2ClientId(config?.oauth2_client_id || '');
    setFormOAuth2ClientSecret('');
    setFormOAuth2TokenUrl(config?.oauth2_token_url || '');
    setFormOAuth2AuthUrl(config?.oauth2_auth_url || '');
    setFormOAuth2Scopes(config?.oauth2_scopes?.join(' ') || '');
    setFormOAuth2CallbackPort(config?.oauth2_callback_port ? String(config.oauth2_callback_port) : '');
    setFormOAuth2CallbackHost(config?.oauth2_callback_host || '');

    setDiscoveryStatus('idle');
    setDiscoveredFields(new Set());
    setDiscoveryResourceName('');
    setDiscoveryRegistrationUrl('');
    setManualRegistrationUrl(config?.oauth2_registration_url || '');
    setLastDiscoveredUrl('');
  };

  const loadAuthInfo = useCallback(async (slug: string, configAuthType?: string) => {
    try {
      const info = await GetMCPServerAuthInfo(slug);
      if (info?.hasAuth) {
        setHasExistingAuth(true);
        if (!configAuthType || configAuthType === 'none') {
          setFormAuthType(info.authType || 'bearer');
        }
      }
    } catch {
      // auth info not available
    }
  }, []);

  const handleEdit = useCallback(async (row: ServerRow) => {
    const fullConfig = await getConfig(row.slug);
    const config = fullConfig ?? new mcp.ServerConfig({
      name: row.name,
      description: row.description,
      transport: row.transport,
      command: row.command,
      args: row.args,
      url: row.url,
      enabled: row.enabled,
      auto_connect: row.autoConnect,
    });
    setEditing(config);
    setEditingSlug(row.slug);
    setIsNew(false);
    populateForm(config, row.slug);

    const isHTTP = config.transport === 'streamable' || config.transport === 'sse';
    if (isHTTP && !isNew) {
      loadAuthInfo(row.slug, config.auth_type);
    }
  }, [getConfig, loadAuthInfo, isNew]);

  const handleNew = useCallback(() => {
    setEditing(new mcp.ServerConfig({
      name: '',
      transport: 'stdio',
      enabled: true,
      auto_connect: true,
    }));
    setEditingSlug(null);
    setIsNew(true);
    populateForm(null, null);
    setFormEnabled(true);
    setFormAutoConnect(true);
    setFormPreferBridge(false);
  }, []);

  useResourceEditRequest('mcp', {
    onEdit: (slug) => {
      const found = rows.find((r) => r.slug === slug);
      if (found) handleEdit(found);
    },
    onNew: () => handleNew(),
    ready: !isLoading && rows.length > 0,
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isModalOpen()) return;
      if (!event.ctrlKey || event.shiftKey || event.altKey) return;
      if (event.key !== 'n' && event.key !== 'N') return;
      const target = event.target as HTMLElement | null;
      const isInput =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.isContentEditable;
      if (isInput) return;
      event.preventDefault();
      handleNew();
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [handleNew]);

  const handleCloseEditor = useCallback(() => {
    setEditing(null);
    setEditingSlug(null);
    setIsNew(false);
    announce(t('mcp.announce.editorClosed'));
  }, [announce, t]);

  const runDiscovery = useCallback(async (urlToDiscover: string) => {
    if (!urlToDiscover || !urlToDiscover.startsWith('https://')) return;
    if (urlToDiscover === lastDiscoveredUrl) return;

    setDiscoveryStatus('loading');
    setDiscoveryRegistrationUrl('');
    setLastDiscoveredUrl(urlToDiscover);

    try {
      const result = await DiscoverMCPServerAuth(urlToDiscover);
      if (result.found) {
        const fields = new Set<string>();

        if (result.authType && formAuthType === 'none') {
          setFormAuthType(result.authType);
          fields.add('authType');
        }
        if (result.authUrl && !formOAuth2AuthUrl) {
          setFormOAuth2AuthUrl(result.authUrl);
          fields.add('oauth2AuthUrl');
        }
        if (result.tokenUrl && !formOAuth2TokenUrl) {
          setFormOAuth2TokenUrl(result.tokenUrl);
          fields.add('oauth2TokenUrl');
        }
        if (result.scopes?.length > 0 && !formOAuth2Scopes) {
          setFormOAuth2Scopes(result.scopes.join(' '));
          fields.add('oauth2Scopes');
        }
        setDiscoveredFields(fields);
        const resName = result.resourceName || '';
        setDiscoveryResourceName(resName);
        setDiscoveryRegistrationUrl(result.registrationUrl || '');
        if (resName && !formName) setFormName(resName);
        setDiscoveryStatus('found');
      } else if (result.status === 'partial' || result.protectedResourceFound) {
        setDiscoveredFields(new Set());
        setDiscoveryResourceName(result.resourceName || '');
        setDiscoveryRegistrationUrl('');
        setDiscoveryStatus('partial');
      } else {
        setDiscoveryRegistrationUrl('');
        setDiscoveryStatus('not_found');
      }
    } catch {
      setDiscoveryStatus('not_found');
    }
  }, [
    lastDiscoveredUrl,
    formAuthType,
    formName,
    formOAuth2AuthUrl,
    formOAuth2Scopes,
    formOAuth2TokenUrl,
  ]);

  const handleUrlBlur = useCallback(() => {
    const isHTTP = formTransport === 'streamable' || formTransport === 'sse';
    if (!isHTTP || !formUrl.trim()) return;

    runDiscovery(formUrl.trim());

    if (!isNew) return;
    try {
      const host = new URL(formUrl.trim()).hostname;
      const parts = host.replace(/^(mcp|api|www)\./, '').split('.');
      const derived = parts[0] || '';
      if (derived && !formName) setFormName(derived.charAt(0).toUpperCase() + derived.slice(1));
    } catch { /* URL inválida */ }
  }, [formTransport, formUrl, formName, isNew, runDiscovery]);

  const handleManualOverride = useCallback(() => {
    setDiscoveredFields(new Set());
    setDiscoveryRegistrationUrl('');
    setDiscoveryStatus('not_found');
  }, []);

  // Dispara discovery quando transport muda para HTTP e URL já está preenchida
  useEffect(() => {
    const isHTTP = formTransport === 'streamable' || formTransport === 'sse';
    if (isHTTP && formUrl.trim() && formUrl.trim().startsWith('https://') && editing) {
      runDiscovery(formUrl.trim());
    }
  }, [editing, formTransport, formUrl, runDiscovery]);

  const handleSave = useCallback(async () => {
    const slug = isNew
      ? formName.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9_-]/g, '').replace(/-+/g, '-')
      : editingSlug;

    if (!formName.trim()) {
      addToast(t('mcp.error.nameRequired'), 'error');
      return;
    }
    if (!slug) {
      addToast(t('mcp.error.nameRequired'), 'error');
      return;
    }

    const isHTTP = formTransport === 'streamable' || formTransport === 'sse';

    // Parse env
    const env: Record<string, string> = {};
    if (formEnvText.trim()) {
      for (const line of formEnvText.split('\n')) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith('#')) continue;
        const eqIdx = trimmed.indexOf('=');
        if (eqIdx > 0) {
          env[trimmed.substring(0, eqIdx)] = trimmed.substring(eqIdx + 1);
        }
      }
    }

    const argsArr = formArgs.trim() ? formArgs.trim().split(/\s+/) : [];

    const isOAuth2 = formAuthType === 'oauth2_client_credentials' || formAuthType === 'oauth2_pkce';
    const scopesArr = formOAuth2Scopes.trim() ? formOAuth2Scopes.trim().split(/\s+/) : undefined;

    const config = new mcp.ServerConfig({
      name: formName.trim(),
      description: formDescription.trim() || undefined,
      transport: formTransport,
      command: formTransport === 'stdio' ? formCommand.trim() : undefined,
      args: formTransport === 'stdio' ? argsArr : undefined,
      env: Object.keys(env).length > 0 ? env : undefined,
      url: isHTTP ? formUrl.trim() : undefined,
      enabled: formEnabled,
      auto_connect: formAutoConnect,
      prefer_bridge: isHTTP ? formPreferBridge : undefined,
      auth_type: isHTTP ? formAuthType : undefined,
      oauth2_client_id: isHTTP && isOAuth2 ? formOAuth2ClientId.trim() || undefined : undefined,
      oauth2_token_url: isHTTP && isOAuth2 ? formOAuth2TokenUrl.trim() || undefined : undefined,
      oauth2_auth_url: isHTTP && formAuthType === 'oauth2_pkce' ? formOAuth2AuthUrl.trim() || undefined : undefined,
      oauth2_scopes: isHTTP && isOAuth2 ? scopesArr : undefined,
      oauth2_registration_url: isHTTP && formAuthType === 'oauth2_pkce'
        ? manualRegistrationUrl || discoveryRegistrationUrl || undefined
        : undefined,
      oauth2_callback_port: isHTTP && formAuthType === 'oauth2_pkce' && formOAuth2CallbackPort
        ? parseInt(formOAuth2CallbackPort, 10) || undefined
        : undefined,
      oauth2_callback_host: isHTTP && formAuthType === 'oauth2_pkce' && formOAuth2CallbackHost
        ? formOAuth2CallbackHost
        : undefined,
    });

    setSaving(true);
    try {
      await save(slug, config);

      // Salva auth no credential manager (separado do config JSON)
      if (isHTTP && formAuthType !== 'none') {
        if (formAuthType === 'oauth2_client_credentials') {
          if (formOAuth2ClientSecret.trim()) {
            await SaveMCPServerAuth(slug, formAuthType, '', '', '', formOAuth2ClientSecret.trim());
          }
        } else if (formAuthType === 'oauth2_pkce') {
          if (formOAuth2ClientSecret.trim()) {
            await SaveMCPServerAuth(slug, formAuthType, '', '', '', formOAuth2ClientSecret.trim());
          }
        } else {
          const hasNewCredentials =
            formAuthToken.trim() || formAuthUsername.trim() || formAuthPassword.trim();
          if (hasNewCredentials) {
            await SaveMCPServerAuth(
              slug,
              formAuthType,
              formAuthToken.trim(),
              formAuthUsername.trim(),
              formAuthPassword.trim(),
              '',
            );
          }
        }
      } else if (isHTTP && formAuthType === 'none' && hasExistingAuth) {
        await DeleteMCPServerAuth(slug);
      }

      addToast(isNew ? t('mcp.toast.created') : t('mcp.toast.updated'), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(isNew ? t('mcp.toast.created') : t('mcp.toast.updated'));
      handleCloseEditor();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('mcp.error.saveFailed'), 'error');
    } finally {
      setSaving(false);
    }
  }, [isNew, editingSlug, formName, formDescription, formTransport, formCommand, formArgs, formEnvText, formUrl, formEnabled, formAutoConnect, formPreferBridge, formAuthType, formAuthToken, formAuthUsername, formAuthPassword, formOAuth2ClientId, formOAuth2ClientSecret, formOAuth2TokenUrl, formOAuth2AuthUrl, formOAuth2Scopes, formOAuth2CallbackPort, formOAuth2CallbackHost, discoveryRegistrationUrl, manualRegistrationUrl, hasExistingAuth, save, addToast, announce, handleCloseEditor, t]);

  const handleDelete = useCallback(async (slug: string, name: string) => {
    const shouldDelete = await confirm({
      title: t('mcp.confirm.removeTitle'),
      message: t('mcp.confirm.removeMessage', { name }),
      confirmText: t('common.remove'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });

    if (!shouldDelete) return;

    try {
      await remove(slug);
      addToast(t('mcp.toast.removed'), 'success', undefined, undefined, { suppressAnnounce: true });
      announce(t('mcp.announce.serverRemoved'));
      if (editingSlug === slug) {
        setEditing(null);
        setEditingSlug(null);
        setIsNew(false);
      }
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('mcp.error.removeFailed'), 'error');
    }
  }, [addToast, announce, confirm, editingSlug, remove, t]);

  const handleConnect = useCallback(async (row: ServerRow) => {
    try {
      await connect(row.slug);
      addToast(t('mcp.toast.serverConnected', { name: row.name }), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('mcp.announce.serverConnected', { name: row.name }));
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('mcp.error.connectFailed'), 'error');
    }
  }, [connect, addToast, announce, t]);

  const handleDisconnect = useCallback(async (row: ServerRow) => {
    try {
      await disconnect(row.slug);
      addToast(t('mcp.toast.serverDisconnected', { name: row.name }), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('mcp.announce.serverDisconnected', { name: row.name }));
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('mcp.error.disconnectFailed'), 'error');
    }
  }, [disconnect, addToast, announce, t]);

  const handleReconnect = useCallback(async (row: ServerRow) => {
    try {
      await reconnect(row.slug);
      addToast(t('mcp.toast.serverReconnected', { name: row.name }), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('mcp.announce.serverReconnected', { name: row.name }));
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('mcp.error.reconnectFailed'), 'error');
    }
  }, [reconnect, addToast, announce, t]);

  const handleDuplicate = useCallback(async (row: ServerRow) => {
    try {
      const newSlug = await DuplicateMCPServer(row.slug);
      addToast(t('mcp.toast.duplicated', 'Servidor MCP duplicado!'), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('mcp.toast.duplicated', 'Servidor MCP duplicado!'));
      await loadServers();

      const config = await getConfig(newSlug);
      if (config) {
        setEditing(config);
        setEditingSlug(newSlug);
        setIsNew(false);
        populateForm(config, newSlug);

        const isHTTP = config.transport === 'streamable' || config.transport === 'sse';
        if (isHTTP) {
          loadAuthInfo(newSlug, config.auth_type);
        }
      }
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('mcp.error.duplicate', 'Erro ao duplicar'), 'error');
    }
  }, [addToast, announce, getConfig, loadAuthInfo, loadServers, t]);

  const getRowId = useCallback((row: ServerRow) => row.id, []);
  const handleActivate = useCallback((row: ServerRow) => handleEdit(row), [handleEdit]);
  const handleDeleteRow = useCallback((row: ServerRow) => {
    void handleDelete(row.slug, row.name);
  }, [handleDelete]);
  const handleFocusChange = useCallback((row: ServerRow | null) => {
    setFocusedRow(row as ServerRow | null);
  }, []);

  const columns: DataGridColumn<ServerRow>[] = [
    {
      key: 'name',
      label: t('mcp.columns.name'),
      width: '30%',
    },
    {
      key: 'transport',
      label: t('mcp.columns.transport'),
      width: '12%',
      format: (val) => String(val).toUpperCase(),
    },
    {
      key: 'status',
      label: t('mcp.columns.status'),
      width: '15%',
      format: (val) => {
        const label = statusLabel(String(val), t);
        const statusClass = `mcp-badge mcp-badge--${String(val)}`;
        return <span className={statusClass}>{label}</span>;
      },
    },
    {
      key: 'toolCount',
      label: t('mcp.columns.tools'),
      width: '12%',
      format: (val) => `${val}`,
    },
    {
      key: 'actions',
      label: t('common.actions'),
      width: '6%',
      format: (_val, row) => (
        <MenuButton
          items={getRowActions(row)}
          buttonLabel={t('mcp.actions.actions')}
        />
      ),
    },
  ];

  function getRowActions(row: ServerRow) {
    const actions = [];
    if (row.status === 'connected' || row.status === 'connecting') {
      actions.push({
        id: 'disconnect',
        label: row.status === 'connecting'
          ? t('mcp.actions.cancel')
          : t('mcp.actions.disconnect'),
        icon: <ApiOutlined aria-hidden="true" />,
        onClick: () => handleDisconnect(row),
      });
    } else {
      actions.push({
        id: 'connect',
        label: t('mcp.actions.connect'),
        icon: <ApiOutlined aria-hidden="true" />,
        onClick: () => handleConnect(row),
      });
    }
    if (row.status !== 'connecting') {
      actions.push({
        id: 'reconnect',
        label: t('mcp.actions.reconnect'),
        icon: <ReloadOutlined aria-hidden="true" />,
        onClick: () => handleReconnect(row),
      });
    }
    actions.push({
      id: 'duplicate',
      label: t('mcp.actions.duplicate'),
      icon: <CopyOutlined aria-hidden="true" />,
      onClick: () => handleDuplicate(row),
    });
    actions.push({
      id: 'edit',
      label: t('mcp.actions.edit'),
      icon: <EditOutlined aria-hidden="true" />,
      onClick: () => handleEdit(row),
    });
    actions.push({
      id: 'delete',
      label: t('mcp.actions.removeServer'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => handleDelete(row.slug, row.name),
      danger: true,
    });
    return actions;
  }

  // Removido: handleCellAction (não é mais necessário)

  const filteredRows = useMemo(() => rows.filter((row) => {
    const query = searchTerm.trim().toLowerCase();
    if (!query) return true;
    return (
      row.name.toLowerCase().includes(query) ||
      row.slug.toLowerCase().includes(query) ||
      row.description.toLowerCase().includes(query) ||
      row.transport.toLowerCase().includes(query)
    );
  }), [rows, searchTerm]);

  if (isLoading && rows.length === 0) {
    return (
      <div className="mcp-page">
        <PageLoading message={t('mcp.loading')} />
      </div>
    );
  }

  return (
    <div className="mcp-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('mcp.pageTitle')}</h1>}
        ariaLabel={t('mcp.aria.toolbar')}
        searchPlaceholder={t('mcp.searchPlaceholder')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        actions={[
          {
            key: 'new',
            label: t('mcp.buttons.newServer'),
            icon: <PlusOutlined aria-hidden="true" />,
            onClick: handleNew,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'connect-toggle',
            label: focusedRow?.status === 'connected'
              ? t('mcp.actions.disconnect', 'Desconectar')
              : focusedRow?.status === 'connecting'
                ? t('mcp.actions.cancel', 'Cancelar')
                : t('mcp.actions.connect', 'Conectar'),
            icon: <ApiOutlined aria-hidden="true" />,
            onClick: () => {
              if (!focusedRow) return;
              if (focusedRow.status === 'connected' || focusedRow.status === 'connecting') {
                void handleDisconnect(focusedRow);
              } else {
                void handleConnect(focusedRow);
              }
            },
            disabled: !focusedRow,
          },
          {
            key: 'reconnect',
            label: t('mcp.actions.reconnect'),
            icon: <ReloadOutlined aria-hidden="true" />,
            onClick: () => focusedRow && handleReconnect(focusedRow),
            disabled: !focusedRow || focusedRow.status === 'connecting',
          },
          {
            key: 'duplicate',
            label: t('mcp.actions.duplicate'),
            icon: <CopyOutlined aria-hidden="true" />,
            onClick: () => focusedRow && handleDuplicate(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'edit',
            label: t('mcp.actions.edit'),
            icon: <EditOutlined aria-hidden="true" />,
            onClick: () => focusedRow && handleEdit(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'delete',
            label: t('mcp.actions.removeServer'),
            icon: <DeleteOutlined aria-hidden="true" />,
            onClick: () => focusedRow && void handleDelete(focusedRow.slug, focusedRow.name),
            disabled: !focusedRow,
            variant: 'danger',
          },
        ]}
      />
      <DataGrid
        columns={columns}
        items={filteredRows}
        getItemId={getRowId}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={handleActivate}
        onDelete={handleDeleteRow}
        onGridReady={handleGridReady}
        label={t('mcp.pageTitle')}
        getRowActions={getRowActions}
        onFocusChange={handleFocusChange}
      />

      <Modal
        isOpen={!!editing}
        onClose={handleCloseEditor}
        title={isNew ? t('mcp.modal.newTitle') : t('mcp.modal.editTitle', { name: formName || editingSlug })}
        size="lg"
      >
        {editing && (
          <div className="mcp-editor">
            <McpGeneralSection
              name={formName}
              description={formDescription}
              transport={formTransport}
              onNameChange={setFormName}
              onDescriptionChange={setFormDescription}
              onTransportChange={setFormTransport}
            />

            <McpConnectionSection
              transport={formTransport}
              command={formCommand}
              args={formArgs}
              url={formUrl}
              envText={formEnvText}
              enabled={formEnabled}
              autoConnect={formAutoConnect}
              preferBridge={formPreferBridge}
              authType={formAuthType}
              authToken={formAuthToken}
              authUsername={formAuthUsername}
              authPassword={formAuthPassword}
              hasExistingAuth={hasExistingAuth}
              oauth2ClientId={formOAuth2ClientId}
              oauth2ClientSecret={formOAuth2ClientSecret}
              oauth2TokenUrl={formOAuth2TokenUrl}
              oauth2AuthUrl={formOAuth2AuthUrl}
              oauth2Scopes={formOAuth2Scopes}
              discoveryStatus={discoveryStatus}
              discoveredFields={discoveredFields}
              discoveryResourceName={discoveryResourceName}
              discoveryRegistrationUrl={manualRegistrationUrl || discoveryRegistrationUrl}
              onCommandChange={setFormCommand}
              onArgsChange={setFormArgs}
              onUrlChange={setFormUrl}
              onEnvTextChange={setFormEnvText}
              onEnabledChange={setFormEnabled}
              onAutoConnectChange={setFormAutoConnect}
              onPreferBridgeChange={setFormPreferBridge}
              onAuthTypeChange={setFormAuthType}
              onAuthTokenChange={setFormAuthToken}
              onAuthUsernameChange={setFormAuthUsername}
              onAuthPasswordChange={setFormAuthPassword}
              onOAuth2ClientIdChange={setFormOAuth2ClientId}
              onOAuth2ClientSecretChange={setFormOAuth2ClientSecret}
              onOAuth2TokenUrlChange={setFormOAuth2TokenUrl}
              onOAuth2AuthUrlChange={setFormOAuth2AuthUrl}
              onOAuth2ScopesChange={setFormOAuth2Scopes}
              oauth2CallbackPort={formOAuth2CallbackPort}
              oauth2CallbackHost={formOAuth2CallbackHost}
              onOAuth2CallbackPortChange={setFormOAuth2CallbackPort}
              onOAuth2CallbackHostChange={setFormOAuth2CallbackHost}
              onUrlBlur={handleUrlBlur}
              onManualOverride={handleManualOverride}
            />

            <EditorPanelFooter className="mcp-editor__footer">
              {!isNew && editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => void handleDelete(editingSlug, formName || editingSlug)}
                  aria-label={t('mcp.aria.deleteServer', {
                    name: formName || editingSlug || '',
                  })}
                >
                  {t('mcp.buttons.delete')}
                </Button>
              )}
              <DialogActions
                primary={
                  <Button
                    onClick={handleSave}
                    loading={saving}
                    aria-label={
                      saving
                        ? t('mcp.aria.saving')
                        : t('mcp.aria.saveServer', {
                            name: formName || editingSlug || t('mcp.pageTitle'),
                          })
                    }
                  >
                    {t('common.save')}
                  </Button>
                }
                secondary={
                  <Button variant="ghost" onClick={handleCloseEditor} aria-label={t('mcp.aria.closeEditor')}>
                    {t('mcp.buttons.close')}
                  </Button>
                }
              />
            </EditorPanelFooter>
          </div>
        )}
      </Modal>

      {!editing && rows.length > 0 && (
        <div className="mcp-empty">
          <p>{t('mcp.hint.edit', 'Pressione Enter ou clique no servidor para editar.')}</p>
        </div>
      )}

      {!editing && rows.length === 0 && (
        <div className="mcp-empty">
          <p>{t('mcp.empty.noServers', 'Nenhum servidor MCP encontrado. Use o botão "Novo Servidor" para começar.')}</p>
        </div>
      )}
    </div>
  );
}
