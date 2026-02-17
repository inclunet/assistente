import { useState, useEffect, useCallback } from 'react';
import { useMCPStore } from '../store/mcpStore';
import { mcp } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import { SimpleModal } from '../components/ui/SimpleModal';
import { useGridFocus } from '../hooks/useGridFocus';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
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

function statusLabel(status: string): string {
  const labels: Record<string, string> = {
    connected: 'Conectado',
    connecting: 'Conectando...',
    disconnected: 'Desconectado',
    error: 'Erro',
  };
  return labels[status] || status;
}

export default function McpPage() {
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { focusFirstCell, handleGridReady } = useGridFocus();

  const {
    servers,
    isLoading,
    loadServers,
    connect,
    disconnect,
    reconnect,
    save,
    remove,
    setupEventListeners,
  } = useMCPStore();

  // Editor state
  const [editing, setEditing] = useState<ServerConfig | null>(null);
  const [editingSlug, setEditingSlug] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  // Form fields
  const [formSlug, setFormSlug] = useState('');
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formTransport, setFormTransport] = useState('stdio');
  const [formCommand, setFormCommand] = useState('');
  const [formArgs, setFormArgs] = useState('');
  const [formEnvText, setFormEnvText] = useState('');
  const [formUrl, setFormUrl] = useState('');
  const [formEnabled, setFormEnabled] = useState(true);
  const [formAutoConnect, setFormAutoConnect] = useState(true);

  // Delete confirmation
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ServerRow | null>(null);

  useEffect(() => {
    loadServers();
    const cleanup = setupEventListeners();
    return cleanup;
  }, [loadServers, setupEventListeners]);

  // Convert servers to rows
  const rows: ServerRow[] = (servers || []).map((s: ServerInfo) => ({
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
  }));

  const populateForm = (config: ServerConfig | null, slug: string | null) => {
    setFormSlug(slug || '');
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
  };

  const handleEdit = useCallback((row: ServerRow) => {
    const config = new mcp.ServerConfig({
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
  }, []);

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
  }, []);

  const handleCloseEditor = useCallback(() => {
    setEditing(null);
    setEditingSlug(null);
    setIsNew(false);
    announce('Editor fechado');
  }, [announce]);

  const handleSave = useCallback(async () => {
    const slug = isNew
      ? formSlug.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '-')
      : editingSlug;

    if (!slug) {
      addToast('Slug (identificador) é obrigatório', 'error');
      return;
    }
    if (!formName.trim()) {
      addToast('Nome é obrigatório', 'error');
      return;
    }

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

    const config = new mcp.ServerConfig({
      name: formName.trim(),
      description: formDescription.trim() || undefined,
      transport: formTransport,
      command: formTransport === 'stdio' ? formCommand.trim() : undefined,
      args: formTransport === 'stdio' ? argsArr : undefined,
      env: Object.keys(env).length > 0 ? env : undefined,
      url: formTransport === 'sse' ? formUrl.trim() : undefined,
      enabled: formEnabled,
      auto_connect: formAutoConnect,
    });

    setSaving(true);
    try {
      await save(slug, config);
      addToast(isNew ? 'Servidor MCP criado!' : 'Servidor MCP atualizado!', 'success');
      announce(isNew ? 'Servidor criado com sucesso' : 'Servidor atualizado com sucesso');
      handleCloseEditor();
    } catch (error: any) {
      addToast(error?.message || 'Erro ao salvar', 'error');
    } finally {
      setSaving(false);
    }
  }, [isNew, editingSlug, formSlug, formName, formDescription, formTransport, formCommand, formArgs, formEnvText, formUrl, formEnabled, formAutoConnect, save, addToast, announce, handleCloseEditor]);

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return;
    try {
      await remove(deleteTarget.slug);
      addToast('Servidor MCP removido!', 'success');
      announce('Servidor removido');
      setDeleteOpen(false);
      setDeleteTarget(null);
      if (editingSlug === deleteTarget.slug) {
        setEditing(null);
        setEditingSlug(null);
      }
    } catch (error: any) {
      addToast(error?.message || 'Erro ao remover', 'error');
    }
  }, [deleteTarget, remove, editingSlug, addToast, announce]);

  const handleConnect = useCallback(async (row: ServerRow) => {
    try {
      await connect(row.slug);
      addToast(`Servidor "${row.name}" conectado!`, 'success');
      announce(`Servidor ${row.name} conectado`);
    } catch (error: any) {
      addToast(error?.message || 'Erro ao conectar', 'error');
    }
  }, [connect, addToast, announce]);

  const handleDisconnect = useCallback(async (row: ServerRow) => {
    try {
      await disconnect(row.slug);
      addToast(`Servidor "${row.name}" desconectado`, 'success');
      announce(`Servidor ${row.name} desconectado`);
    } catch (error: any) {
      addToast(error?.message || 'Erro ao desconectar', 'error');
    }
  }, [disconnect, addToast, announce]);

  const handleReconnect = useCallback(async (row: ServerRow) => {
    try {
      await reconnect(row.slug);
      addToast(`Servidor "${row.name}" reconectado!`, 'success');
      announce(`Servidor ${row.name} reconectado`);
    } catch (error: any) {
      addToast(error?.message || 'Erro ao reconectar', 'error');
    }
  }, [reconnect, addToast, announce]);

  const columns: DataGridColumn<ServerRow>[] = [
    {
      key: 'name',
      label: 'Nome',
      width: '30%',
    },
    {
      key: 'transport',
      label: 'Transporte',
      width: '12%',
      format: (val) => String(val).toUpperCase(),
    },
    {
      key: 'status',
      label: 'Status',
      width: '15%',
      format: (val) => statusLabel(val),
    },
    {
      key: 'toolCount',
      label: 'Ferramentas',
      width: '12%',
      format: (val) => `${val}`,
    },
    {
      key: 'connect',
      label: '',
      width: '3%',
      action: true,
      actionIcon: '🔌',
      actionLabel: 'Conectar/Desconectar',
    },
    {
      key: 'reconnect',
      label: '',
      width: '3%',
      action: true,
      actionIcon: '🔄',
      actionLabel: 'Reconectar',
    },
    {
      key: 'delete',
      label: '',
      width: '3%',
      action: true,
      actionIcon: '🗑️',
      actionLabel: 'Remover servidor',
    },
  ];

  const handleCellAction = useCallback((item: ServerRow, column: DataGridColumn<ServerRow>) => {
    if (column.key === 'connect') {
      if (item.status === 'connected') {
        handleDisconnect(item);
      } else {
        handleConnect(item);
      }
    } else if (column.key === 'reconnect') {
      handleReconnect(item);
    } else if (column.key === 'delete') {
      setDeleteTarget(item);
      setDeleteOpen(true);
    }
  }, [handleConnect, handleDisconnect, handleReconnect]);

  if (isLoading && rows.length === 0) {
    return (
      <div className="mcp-page">
        <div className="loading">Carregando servidores MCP...</div>
      </div>
    );
  }

  return (
    <div className="mcp-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">Servidores MCP</h1>}
        ariaLabel="Barra de ferramentas de MCP"
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new',
            label: 'Novo Servidor',
            icon: '+',
            onClick: handleNew,
            variant: 'primary',
          },
        ]}
      />

      <div className="mcp-page__content">
        <DataGrid
          columns={columns}
          items={rows}
          getItemId={(row) => row.id}
          onActivate={(row) => handleEdit(row)}
          onDelete={(row) => {
            setDeleteTarget(row);
            setDeleteOpen(true);
          }}
          onCellAction={handleCellAction}
          onGridReady={handleGridReady}
          label="Servidores MCP"
        />

        <SimpleModal
          isOpen={!!editing}
          onClose={handleCloseEditor}
          title={isNew ? 'Novo Servidor MCP' : `Editando: ${formName || editingSlug}`}
          size="lg"
        >
          {editing && (
            <div className="mcp-page__editor" aria-live="polite">
              <div className="mcp-page__fields">
                {isNew && (
                  <div className="mcp-page__field">
                    <label className="mcp-page__label">Slug (identificador)</label>
                    <input
                      type="text"
                      className="mcp-page__input"
                      value={formSlug}
                      onChange={(e) => setFormSlug(e.target.value)}
                      placeholder="ex: github, filesystem"
                      required
                    />
                    <span className="mcp-page__hint">
                      Identificador único. Apenas letras minúsculas, números, - e _
                    </span>
                  </div>
                )}

                <div className="mcp-page__field">
                  <label className="mcp-page__label">Nome</label>
                  <input
                    type="text"
                    className="mcp-page__input"
                    value={formName}
                    onChange={(e) => setFormName(e.target.value)}
                    placeholder="ex: GitHub Tools"
                    required
                  />
                </div>

                <div className="mcp-page__field">
                  <label className="mcp-page__label">Descrição</label>
                  <input
                    type="text"
                    className="mcp-page__input"
                    value={formDescription}
                    onChange={(e) => setFormDescription(e.target.value)}
                    placeholder="Descrição opcional do servidor"
                  />
                </div>

                <div className="mcp-page__field">
                  <label className="mcp-page__label">Transporte</label>
                  <select
                    className="mcp-page__input"
                    value={formTransport}
                    onChange={(e) => setFormTransport(e.target.value)}
                  >
                    <option value="stdio">stdio (processo local)</option>
                    <option value="sse">SSE (servidor remoto)</option>
                  </select>
                </div>

                {formTransport === 'stdio' && (
                  <>
                    <div className="mcp-page__field">
                      <label className="mcp-page__label">Comando</label>
                      <input
                        type="text"
                        className="mcp-page__input"
                        value={formCommand}
                        onChange={(e) => setFormCommand(e.target.value)}
                        placeholder="ex: npx, node, python"
                        required
                      />
                    </div>
                    <div className="mcp-page__field">
                      <label className="mcp-page__label">Argumentos (separados por espaço)</label>
                      <input
                        type="text"
                        className="mcp-page__input"
                        value={formArgs}
                        onChange={(e) => setFormArgs(e.target.value)}
                        placeholder="ex: -y @modelcontextprotocol/server-filesystem /home"
                      />
                    </div>
                  </>
                )}

                {formTransport === 'sse' && (
                  <div className="mcp-page__field">
                    <label className="mcp-page__label">URL do servidor</label>
                    <input
                      type="url"
                      className="mcp-page__input"
                      value={formUrl}
                      onChange={(e) => setFormUrl(e.target.value)}
                      placeholder="https://example.com/mcp"
                      required
                    />
                  </div>
                )}

                <div className="mcp-page__field">
                  <label className="mcp-page__label">
                    Variáveis de ambiente (KEY=VALUE, uma por linha)
                  </label>
                  <textarea
                    className="mcp-page__textarea"
                    rows={4}
                    value={formEnvText}
                    onChange={(e) => setFormEnvText(e.target.value)}
                    placeholder={"GITHUB_TOKEN=ghp_xxx\nNODE_ENV=production"}
                  />
                  <span className="mcp-page__hint">
                    Linhas começando com # são ignoradas.
                  </span>
                </div>

                <div className="mcp-page__field">
                  <label className="mcp-page__label">Opções</label>
                  <div className="mcp-page__checkboxes">
                    <label className="mcp-page__checkbox-label">
                      <input
                        type="checkbox"
                        checked={formEnabled}
                        onChange={(e) => setFormEnabled(e.target.checked)}
                      />
                      Habilitado
                    </label>
                    <label className="mcp-page__checkbox-label">
                      <input
                        type="checkbox"
                        checked={formAutoConnect}
                        onChange={(e) => setFormAutoConnect(e.target.checked)}
                      />
                      Conectar automaticamente no início
                    </label>
                  </div>
                </div>
              </div>

              <div className="mcp-page__editor-footer">
                {!isNew && editingSlug && (
                  <Button
                    variant="danger"
                    onClick={() => {
                      const row = rows.find(r => r.slug === editingSlug);
                      if (row) { setDeleteTarget(row); setDeleteOpen(true); }
                    }}
                  >
                    Excluir
                  </Button>
                )}
                <Button variant="ghost" onClick={handleCloseEditor}>Fechar</Button>
                <Button onClick={handleSave} loading={saving}>Salvar</Button>
              </div>
            </div>
          )}
        </SimpleModal>
      </div>

      <ConfirmDialog
        isOpen={deleteOpen}
        title="Remover Servidor MCP"
        message={`Tem certeza que deseja remover o servidor "${deleteTarget?.name}"? A configuração será apagada.`}
        confirmText="Remover"
        cancelText="Cancelar"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => { setDeleteOpen(false); setDeleteTarget(null); }}
      />
    </div>
  );
}
