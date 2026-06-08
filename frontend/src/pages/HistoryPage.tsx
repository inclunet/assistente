import { logger } from '../utils/logger';
import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ApartmentOutlined,
  CheckOutlined,
  CodeOutlined,
  DeleteOutlined,
  ExportOutlined,
  FilePdfOutlined,
  FileTextOutlined,
  FolderOpenOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { GetConversations, DeleteConversation, UpdateConversation, ExportConversations, ExportConversationsToFile, SearchConversationHistory } from '@wailsjs/go/app/App';
import { portability } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import type { MenuItem as ContextMenuItem } from '../components/menu';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Modal } from '../components/ui/Modal';
import { Checkbox } from '../components/ui/Checkbox';
import { Button } from '../components/ui/Button';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useConfirm } from '../hooks/useConfirm';
import { useWorkspaceStore } from '../store/workspaceStore';
import { executeDeepLink } from '../lib/deepLinks';
import { formatRelativeTime } from '../lib/dateUtils';
import { downloadJSON, generateFilename } from '../lib/exportImport';
import './HistoryPage.css';

interface Conversation {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  message_count: number;
  snippet?: string;
  // Sub-conversas de sub-agentes (AEP-0068) são mescladas nesta listagem como
  // conversas comuns; isSubAgent só controla o badge/indicador de status na UI.
  isSubAgent?: boolean;
  subAgentStatus?: string;
}

// Status de run de sub-agente considerados "ativos" — só nesses casos exibimos o
// indicador de status ao lado do título (AEP-0068 Fase 5).
const ACTIVE_SUBAGENT_STATUSES = new Set(['queued', 'running']);

type RichExportFormat = 'html' | 'pdf' | 'md';

interface ContentExportOptions {
  includeTimestamps: boolean;
  includeReasoning: boolean;
  includeMetadata: boolean;
}

const DEFAULT_EXPORT_OPTIONS: ContentExportOptions = {
  includeTimestamps: true,
  includeReasoning: true,
  includeMetadata: true,
};

interface ActiveRichExport {
  format: RichExportFormat;
  ids: string[];
}

export default function HistoryPage() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const navigate = useNavigate();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [searchResultIds, setSearchResultIds] = useState<Set<string> | null>(null);
  const [snippetsMap, setSnippetsMap] = useState<Map<string, string>>(new Map());
  const [searching, setSearching] = useState(false);
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [focusedRow, setFocusedRow] = useState<Conversation | null>(null);
  const [showSubAgents, setShowSubAgents] = useState(true);
  const [exportRequest, setExportRequest] = useState<ActiveRichExport | null>(null);
  const [exportOptions, setExportOptions] = useState<ContentExportOptions>(DEFAULT_EXPORT_OPTIONS);
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'history-page' });
  const moveTabToWorkspace = useWorkspaceStore(state => state.moveTabToWorkspace);
  const addWorkspaceTab = useWorkspaceStore(state => state.addTab);
  const workspaces = useWorkspaceStore(state => state.workspaces);

  const loadConversations = useCallback(async () => {
    setLoading(true);
    try {
      // Listagem unificada (AEP-0068): GetConversations retorna conversas comuns
      // E sub-conversas de sub-agentes (kind=subagent), já ordenadas por recência,
      // com latestStatus preenchido para sub-agentes. Uma única chamada.
      const result = await GetConversations();
      const mapped: Conversation[] = (result || []).map((c) => ({
        id: c.id,
        title: c.title || t('history.untitled', 'Sem título'),
        createdAt: String(c.createdAt ?? ''),
        updatedAt: String(c.updatedAt ?? ''),
        message_count: c.message_count || 0,
        isSubAgent: c.kind === 'subagent',
        subAgentStatus: c.latestStatus || undefined,
      }));
      setConversations(mapped);
    } catch (error) {
      logger.error('Erro ao carregar conversas:', error);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void loadConversations();
  }, [loadConversations]);

  const doSearch = useCallback(async (query: string) => {
    if (!query.trim()) {
      setSearchResultIds(null);
      setSnippetsMap(new Map());
      return;
    }
    setSearching(true);
    try {
      const results = await SearchConversationHistory(query, 50);
      if (!results || results.length === 0) {
        setSearchResultIds(new Set());
        setSnippetsMap(new Map());
      } else {
        const ids = new Set<string>();
        const snippets = new Map<string, string>();
        for (const r of results) {
          ids.add(r.conversation_id);
          if (!snippets.has(r.conversation_id)) {
            const snippet = (r.snippet || '')
              .replace(/>>>/g, '\u00AB').replace(/<<</g, '\u00BB');
            snippets.set(r.conversation_id, snippet);
          }
        }
        setSearchResultIds(ids);
        setSnippetsMap(snippets);
      }
    } catch (error) {
      logger.error('Erro na busca:', error);
      setSearchResultIds(new Set());
      setSnippetsMap(new Map());
    } finally {
      setSearching(false);
    }
  }, []);

  useEffect(() => {
    if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
    if (!searchTerm.trim()) {
      setSearchResultIds(null);
      setSnippetsMap(new Map());
      return;
    }
    searchDebounceRef.current = setTimeout(() => doSearch(searchTerm), 300);
    return () => { if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current); };
  }, [searchTerm, doSearch]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const handleOpenConversation = useCallback(async (conversationId: string, title?: string) => {
    await executeDeepLink(
      { type: 'conversation:open', conversationId, title },
      { navigate },
    );
  }, [navigate]);

  const handleNewConversation = () => {
    navigate('/');
  };

  const handleToggleSubAgents = useCallback(() => {
    setShowSubAgents((prev) => {
      const next = !prev;
      announce(
        next
          ? t('history.subAgentsShown', 'Sub-agentes exibidos')
          : t('history.subAgentsHidden', 'Sub-agentes ocultos'),
      );
      return next;
    });
  }, [announce, t]);

  const handleDeleteConversation = useCallback(async (conversationId: string) => {
    const conv = conversations.find((c) => c.id === conversationId);
    const title = conv?.title || t('history.untitled');
    const ok = await confirm({
      title: t('history.confirmDeleteTitle'),
      message: t('history.confirmDelete', { title }),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!ok) return;

    try {
      await DeleteConversation(conversationId);
      setConversations(prev => prev.filter(c => c.id !== conversationId));
      setSelectedIds(prev => {
        const newSet = new Set(prev);
        newSet.delete(conversationId);
        return newSet;
      });
    } catch (error) {
      logger.error('Erro ao deletar conversa:', error);
    }
  }, [confirm, conversations, t]);

  const handleDeleteSelected = useCallback(async () => {
    if (selectedIds.size === 0) return;
    const ids = Array.from(selectedIds).map(String);
    const count = ids.length;
    const ok = await confirm({
      title: t('history.confirmDeleteMultipleTitle'),
      message: t('history.confirmDeleteMultiple', { count }),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!ok) return;

    try {
      await Promise.all(ids.map((id) => DeleteConversation(id)));
      const idSet = new Set(ids);
      setConversations((prev) => prev.filter((c) => !idSet.has(c.id)));
      setSelectedIds(new Set());
    } catch (error) {
      logger.error('Erro ao deletar conversas:', error);
    }
  }, [confirm, selectedIds, t]);

  const getContextConversationIds = useCallback(() => (
    selectedIds.size > 0
      ? Array.from(selectedIds).map(String)
      : (focusedRow && conversations.some((conversation) => conversation.id === focusedRow.id) ? [focusedRow.id] : [])
  ), [conversations, focusedRow, selectedIds]);

  const exportJsonByIds = useCallback(async (idsToExport: string[]) => {
    if (idsToExport.length === 0) {
      announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
      return;
    }

    try {
      const jsonData = await ExportConversations(idsToExport);
      const filename = generateFilename('conversas');
      downloadJSON(jsonData, filename);
    } catch (error) {
      logger.error('Erro ao exportar conversas em JSON:', error);
      announce(t('history.exportError', 'Erro ao exportar conversas'), 'assertive');
    }
  }, [announce, t]);

  const exportRichByIds = useCallback(async (
    idsToExport: string[],
    format: RichExportFormat,
    options: ContentExportOptions,
  ) => {
    if (idsToExport.length === 0) {
      announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
      return;
    }

    try {
      const savedPath = await ExportConversationsToFile(
        idsToExport,
        format,
        portability.ContentExportOptions.createFrom(options),
      );
      if (!savedPath) return;
      announce(t('history.exportSaved', { path: savedPath, defaultValue: `Arquivo exportado: ${savedPath}` }));
    } catch (error) {
      logger.error(`Erro ao exportar conversas em ${format}:`, error);
      announce(t('history.exportError', 'Erro ao exportar conversas'), 'assertive');
    }
  }, [announce, t]);

  const handleExport = useCallback(() => {
    void exportJsonByIds(getContextConversationIds());
  }, [exportJsonByIds, getContextConversationIds]);

  const openRichExport = useCallback((format: RichExportFormat, ids: string[]) => {
    if (ids.length === 0) {
      announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
      return;
    }
    setExportOptions(DEFAULT_EXPORT_OPTIONS);
    setExportRequest({ format, ids });
  }, [announce, t]);

  const handleRichExport = useCallback((format: RichExportFormat) => {
    openRichExport(format, getContextConversationIds());
  }, [openRichExport, getContextConversationIds]);

  const closeExportModal = useCallback(() => {
    setExportRequest(null);
  }, []);

  const confirmRichExport = useCallback(async () => {
    if (!exportRequest) return;
    const { ids, format } = exportRequest;
    setExportRequest(null);
    await exportRichByIds(ids, format, exportOptions);
  }, [exportRequest, exportOptions, exportRichByIds]);

  const handleDeleteAction = useCallback(() => {
    if (selectedIds.size > 0) {
      void handleDeleteSelected();
      return;
    }
    if (focusedRow) {
      void handleDeleteConversation(focusedRow.id);
    }
  }, [focusedRow, handleDeleteConversation, handleDeleteSelected, selectedIds]);

  const displayItems = useMemo(() => {
    const base = showSubAgents ? conversations : conversations.filter((c) => !c.isSubAgent);
    if (searchResultIds === null) return base;
    // A busca FTS (SearchConversationHistory) já cobre TODAS as conversas do
    // usuário — o índice de mensagens não filtra por kind, então sub-conversas
    // de sub-agentes entram nos resultados como qualquer outra. Busca uniforme:
    // o mesmo conjunto de ids vale para conversas comuns e sub-agentes.
    return base.filter((c) => searchResultIds.has(c.id));
  }, [conversations, searchResultIds, showSubAgents]);

  // Reconcilia foco e seleção com o que está visível: ao ocultar sub-agentes
  // (toggle) ou aplicar busca, itens saem de displayItems. Sem isso, as ações
  // da toolbar (Abrir/Excluir/Exportar) continuariam habilitadas e operariam
  // sobre conversas fora da lista (risco de exclusão/export indevidos).
  useEffect(() => {
    const visibleIds = new Set(displayItems.map((c) => c.id));
    if (focusedRow && !visibleIds.has(focusedRow.id)) {
      setFocusedRow(null);
    }
    setSelectedIds((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set([...prev].filter((id) => visibleIds.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [displayItems, focusedRow]);

  const handleFocusChange = useCallback((item: Conversation | null) => {
    setFocusedRow(item);
  }, []);

  const handleDeleteRow = useCallback((item: Conversation) => {
    handleDeleteConversation(item.id);
  }, [handleDeleteConversation]);

  const handleSendToWorkspace = useCallback(async (_conversationId: string, title: string, targetWorkspaceId: string, isActive: boolean) => {
    try {
      const tabTitle = title || t('chat.newConversation', 'Nova conversa');
      if (isActive) {
        await addWorkspaceTab('chat', tabTitle);
        navigate('/');
      } else {
        const tabId = await addWorkspaceTab('chat', tabTitle);
        await moveTabToWorkspace(tabId, targetWorkspaceId);
      }
    } catch (error) {
      logger.error('Erro ao enviar conversa ao workspace:', error);
    }
  }, [addWorkspaceTab, moveTabToWorkspace, navigate, t]);

  const getRowActions = useCallback(
    (item: Conversation): ContextMenuItem[] => {
      const actions: ContextMenuItem[] = [
        {
          id: 'open',
          label: t('history.openConversation', 'Abrir conversa'),
          icon: <FolderOpenOutlined />,
          action: () => handleOpenConversation(item.id, item.title),
        },
      ];

      if (workspaces.length > 0) {
        actions.push({
          id: 'send-to-workspace',
          label: t('history.sendToWorkspace', 'Enviar ao workspace'),
          icon: <ExportOutlined />,
          submenu: workspaces.map(ws => ({
            id: `ws-${ws.id}`,
            label: ws.name,
            icon: ws.is_active ? <CheckOutlined /> : undefined,
            action: () => handleSendToWorkspace(item.id, item.title, ws.id, ws.is_active),
          })),
        });
      }

      actions.push({
        id: 'export',
        label: t('history.exportMenu', 'Exportar'),
        icon: <ExportOutlined />,
        submenu: [
          {
            id: 'export-json',
            label: t('history.exportJson', 'Exportar JSON'),
            action: () => void exportJsonByIds([item.id]),
          },
          {
            id: 'export-html',
            label: t('history.exportHtml', 'Exportar HTML'),
            icon: <FileTextOutlined />,
            action: () => openRichExport('html', [item.id]),
          },
          {
            id: 'export-markdown',
            label: t('history.exportMarkdown', 'Exportar Markdown'),
            icon: <CodeOutlined />,
            action: () => openRichExport('md', [item.id]),
          },
          {
            id: 'export-pdf',
            label: t('history.exportPdf', 'Exportar PDF'),
            icon: <FilePdfOutlined />,
            action: () => openRichExport('pdf', [item.id]),
          },
        ],
      });

      actions.push({
        id: 'delete',
        label: t('history.deleteConversation', 'Excluir conversa'),
        icon: <DeleteOutlined />,
        action: () => handleDeleteConversation(item.id),
      });

      return actions;
    },
    [exportJsonByIds, openRichExport, handleDeleteConversation, handleOpenConversation, handleSendToWorkspace, workspaces, t]
  );

  const getMenuButtonItems = useCallback(
    (item: Conversation) =>
      getRowActions(item).map((action) => ({
        id: action.id,
        label: action.label ?? '',
        icon: action.icon,
        shortcut: action.shortcut,
        onClick: action.action,
        submenu: action.submenu
          ? action.submenu.map((submenuItem) => ({
              id: submenuItem.id,
              label: submenuItem.label ?? '',
              icon: submenuItem.icon,
              shortcut: submenuItem.shortcut,
              onClick: submenuItem.action,
            }))
          : undefined,
      })),
    [getRowActions]
  );

  const columns: DataGridColumn<Conversation>[] = [
    {
      key: 'title',
      label: t('history.title', 'Título'),
      width: '40%',
      editable: true,
      format: (_value, item) => {
        const snippet = snippetsMap.get(item.id);
        const titleMain = (
          <span className="history-page__title-main">
            <span className="history-page__title-text">{item.title}</span>
            {item.isSubAgent && (
              <span className="history-page__subagent-badge">
                {t('history.subAgent', 'Sub-agente')}
              </span>
            )}
            {item.isSubAgent && item.subAgentStatus && ACTIVE_SUBAGENT_STATUSES.has(item.subAgentStatus) && (
              <span className={`history-page__status history-page__status--${item.subAgentStatus}`}>
                <span className="history-page__status-dot" aria-hidden="true" />
                {t(`history.subAgentStatus.${item.subAgentStatus}`)}
              </span>
            )}
          </span>
        );
        if (snippet) {
          return (
            <span className="history-page__title-cell">
              {titleMain}
              <span className="history-page__title-snippet">{snippet}</span>
            </span>
          );
        }
        return titleMain;
      },
    },
    {
      key: 'message_count',
      label: t('history.messages', 'Mensagens'),
      width: '15%',
      format: (value) => String(value || 0),
    },
    {
      key: 'createdAt',
      label: t('history.created', 'Criada em'),
      width: '20%',
      format: (value) => {
        const dateValue = value instanceof Date || typeof value === 'string' || typeof value === 'number'
          ? value
          : undefined;
        const timestamp = dateValue ? new Date(dateValue).getTime() : 0;
        return formatRelativeTime(timestamp);
      },
    },
    {
      key: 'updatedAt',
      label: t('history.updated', 'Atualizada em'),
      width: '20%',
      format: (value) => {
        const dateValue = value instanceof Date || typeof value === 'string' || typeof value === 'number'
          ? value
          : undefined;
        const timestamp = dateValue ? new Date(dateValue).getTime() : 0;
        return formatRelativeTime(timestamp);
      },
    },
    {
      key: 'actions',
      label: '',
      width: '5%',
      format: (_value, item) => (
        <MenuButton
          items={getMenuButtonItems(item)}
          buttonLabel={t('history.actions', 'Ações')}
        />
      ),
    },
  ];

  // handleCellAction não é mais necessário, pois as ações estão no MenuButton

  const handleCellEdit = useCallback(async (item: Conversation, column: DataGridColumn<Conversation>, newValue: string) => {
    if (column.key === 'title') {
      try {
        await UpdateConversation(item.id, newValue, '');
        setConversations(prev =>
          prev.map(conv =>
            conv.id === item.id ? { ...conv, title: newValue } : conv
          )
        );
      } catch (error) {
        logger.error('Erro ao atualizar título:', error);
      }
    }
  }, []);

  if (loading) {
    return (
      <div className="history-page">
        <div className="loading">{t('history.loading', 'Carregando histórico...')}</div>
      </div>
    );
  }

  return (
    <div className="history-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('history.pageTitle', 'Histórico de Conversas')}</h1>}
        searchPlaceholder={t('history.search', 'Buscar conversas...')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        actions={[
          {
            key: 'new-conversation',
            label: t('history.newConversation', 'Nova Conversa'),
            icon: <PlusOutlined />,
            onClick: handleNewConversation,
            variant: 'primary',
            shortcut: 'Ctrl+N',
          },
          {
            key: 'open-conversation',
            label: t('history.openConversation', 'Abrir conversa'),
            icon: <FolderOpenOutlined />,
            onClick: () => focusedRow && handleOpenConversation(focusedRow.id, focusedRow.title),
            disabled: !focusedRow,
          },
          {
            key: 'toggle-subagents',
            label: showSubAgents
              ? t('history.hideSubAgents', 'Ocultar sub-agentes')
              : t('history.showSubAgents', 'Mostrar sub-agentes'),
            icon: <ApartmentOutlined />,
            onClick: handleToggleSubAgents,
            variant: 'secondary',
            'aria-pressed': showSubAgents,
          },
          {
            key: 'delete',
            label: selectedIds.size > 0
              ? t('history.deleteSelected', `Deletar (${selectedIds.size})`)
              : t('history.deleteConversation', 'Excluir conversa'),
            icon: <DeleteOutlined />,
            onClick: handleDeleteAction,
            disabled: selectedIds.size === 0 && !focusedRow,
            variant: 'danger',
          },
          {
            key: 'export-json',
            label: selectedIds.size > 0 
              ? t('history.exportJsonSelected', {
                  count: selectedIds.size,
                  defaultValue: 'Exportar JSON ({{count}})',
                })
              : t('history.exportJson', 'Exportar JSON'),
            icon: <ExportOutlined />,
            onClick: handleExport,
            disabled: selectedIds.size === 0 && !focusedRow,
            variant: 'secondary',
          },
          {
            key: 'export-html',
            label: t('history.exportHtml', 'Exportar HTML'),
            icon: <FileTextOutlined />,
            onClick: () => handleRichExport('html'),
            disabled: selectedIds.size === 0 && !focusedRow,
            variant: 'secondary',
          },
          {
            key: 'export-markdown',
            label: t('history.exportMarkdown', 'Exportar Markdown'),
            icon: <CodeOutlined />,
            onClick: () => handleRichExport('md'),
            disabled: selectedIds.size === 0 && !focusedRow,
            variant: 'secondary',
          },
          {
            key: 'export-pdf',
            label: t('history.exportPdf', 'Exportar PDF'),
            icon: <FilePdfOutlined />,
            onClick: () => handleRichExport('pdf'),
            disabled: selectedIds.size === 0 && !focusedRow,
            variant: 'secondary',
          },
        ]}
      />

      {searching && (
        <div className="history-page__search-status">{t('history.searching', 'Buscando...')}</div>
      )}

      <DataGrid
        items={displayItems}
        columns={columns}
        onActivate={(item: Conversation) => handleOpenConversation(item.id, item.title)}
        onCellEdit={handleCellEdit}
        onDelete={handleDeleteRow}
        selectedIds={selectedIds}
        multiSelect={true}
        onSelectionChange={(ids: Set<string | number>) => setSelectedIds(new Set([...ids].map(String)))}
        onGridReady={handleGridReady}
        onFocusChange={handleFocusChange}
        getRowActions={getRowActions}
      />

      <Modal
        isOpen={exportRequest !== null}
        onClose={closeExportModal}
        title={t('history.exportOptionsTitle', 'Opções de exportação')}
        size="sm"
        ariaDescribedBy="history-export-options-desc"
      >
        <p id="history-export-options-desc" className="history-page__export-desc">
          {exportRequest
            ? t('history.exportOptionsDescription', {
                format: exportFormatLabel(exportRequest.format, t),
                count: exportRequest.ids.length,
                defaultValue: 'Escolha o que incluir na exportação ({{format}}) de {{count}} conversa(s).',
              })
            : ''}
        </p>
        <fieldset className="history-page__export-fieldset">
          <legend className="history-page__export-legend">
            {t('history.exportOptionsLegend', 'Conteúdo incluído')}
          </legend>
          <Checkbox
            checked={exportOptions.includeTimestamps}
            onChange={(e) => setExportOptions((prev) => ({ ...prev, includeTimestamps: e.target.checked }))}
            label={t('history.exportIncludeTimestamps', 'Incluir datas e horários')}
          />
          <Checkbox
            checked={exportOptions.includeReasoning}
            onChange={(e) => setExportOptions((prev) => ({ ...prev, includeReasoning: e.target.checked }))}
            label={t('history.exportIncludeReasoning', 'Incluir raciocínio (reasoning)')}
          />
          <Checkbox
            checked={exportOptions.includeMetadata}
            onChange={(e) => setExportOptions((prev) => ({ ...prev, includeMetadata: e.target.checked }))}
            label={t('history.exportIncludeMetadata', 'Incluir metadados (modelo, provedor, tokens)')}
          />
        </fieldset>
        <div className="history-page__export-actions">
          <Button variant="secondary" onClick={closeExportModal}>
            {t('common.cancel', 'Cancelar')}
          </Button>
          <Button variant="primary" onClick={() => void confirmRichExport()}>
            {t('history.exportConfirm', 'Exportar')}
          </Button>
        </div>
      </Modal>

    </div>
  );
}

function exportFormatLabel(format: RichExportFormat, t: TFunction): string {
  switch (format) {
    case 'html':
      return t('history.exportFormat.html', { defaultValue: 'HTML' });
    case 'pdf':
      return t('history.exportFormat.pdf', { defaultValue: 'PDF' });
    case 'md':
      return t('history.exportFormat.markdown', { defaultValue: 'Markdown' });
    default:
      return format;
  }
}
