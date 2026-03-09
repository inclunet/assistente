import { useState, useEffect, useCallback } from 'react';
import { useMCPStore } from '../store/mcpStore';
import { mcp } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { McpConnectionSection } from '../components/mcp/McpConnectionSection';
import { McpGeneralSection } from '../components/mcp/McpGeneralSection';
import { Modal } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { useGridFocus } from '../hooks/useGridFocus';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
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
  const confirm = useConfirm();

  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());

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

  const handleDelete = useCallback(async (slug: string, name: string) => {
    const shouldDelete = await confirm({
      title: 'Remover Servidor MCP',
      message: `Tem certeza que deseja remover o servidor "${name}"? A configuração será apagada.`,
      confirmText: 'Remover',
      cancelText: 'Cancelar',
      variant: 'danger',
    });

    if (!shouldDelete) return;

    try {
      await remove(slug);
      addToast('Servidor MCP removido!', 'success');
      announce('Servidor removido');
      if (editingSlug === slug) {
        setEditing(null);
        setEditingSlug(null);
        setIsNew(false);
      }
    } catch (error: any) {
      addToast(error?.message || 'Erro ao remover', 'error');
    }
  }, [addToast, announce, confirm, editingSlug, remove]);

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
      format: (val) => {
        const label = statusLabel(String(val));
        const statusClass = `mcp-badge mcp-badge--${String(val)}`;
        return <span className={statusClass}>{label}</span>;
      },
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
      void handleDelete(item.slug, item.name);
    }
  }, [handleConnect, handleDelete, handleDisconnect, handleReconnect]);

  const filteredRows = rows.filter((row) => {
    const query = searchTerm.trim().toLowerCase();
    if (!query) return true;
    return (
      row.name.toLowerCase().includes(query) ||
      row.slug.toLowerCase().includes(query) ||
      row.description.toLowerCase().includes(query) ||
      row.transport.toLowerCase().includes(query)
    );
  });

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
        searchPlaceholder="Buscar servidores..."
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new',
            label: 'Novo Servidor',
            icon: '➕',
            onClick: handleNew,
            variant: 'primary',
          },
        ]}
      />
      <DataGrid
        columns={columns}
        items={filteredRows}
        getItemId={(row) => row.id}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={(row) => handleEdit(row)}
        onDelete={(row) => {
          void handleDelete(row.slug, row.name);
        }}
        onCellAction={handleCellAction}
        onGridReady={handleGridReady}
        label="Servidores MCP"
      />

      <Modal
        isOpen={!!editing}
        onClose={handleCloseEditor}
        title={isNew ? 'Novo Servidor MCP' : `Editando: ${formName || editingSlug}`}
        size="lg"
      >
        {editing && (
          <div className="mcp-editor" aria-live="polite">
            <McpGeneralSection
              isNew={isNew}
              slug={formSlug}
              name={formName}
              description={formDescription}
              transport={formTransport}
              onSlugChange={setFormSlug}
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
              onCommandChange={setFormCommand}
              onArgsChange={setFormArgs}
              onUrlChange={setFormUrl}
              onEnvTextChange={setFormEnvText}
              onEnabledChange={setFormEnabled}
              onAutoConnectChange={setFormAutoConnect}
            />

            <EditorPanelFooter className="mcp-editor__footer">
              {!isNew && editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => void handleDelete(editingSlug, formName || editingSlug)}
                >
                  Excluir
                </Button>
              )}
              <Button variant="ghost" onClick={handleCloseEditor}>Fechar</Button>
              <Button onClick={handleSave} loading={saving}>Salvar</Button>
            </EditorPanelFooter>
          </div>
        )}
      </Modal>

      {!editing && rows.length > 0 && (
        <div className="mcp-empty" role="status">
          <p>Pressione Enter ou clique no servidor para editar.</p>
        </div>
      )}

      {!editing && rows.length === 0 && (
        <div className="mcp-empty" role="status">
          <p>Nenhum servidor MCP encontrado. Use o botão "Novo Servidor" para começar.</p>
        </div>
      )}
    </div>
  );
}
