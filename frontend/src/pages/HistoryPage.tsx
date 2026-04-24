import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  CheckOutlined,
  DeleteOutlined,
  ExportOutlined,
  FilePdfOutlined,
  FileTextOutlined,
  FolderOpenOutlined,
  ImportOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { GetConversations, DeleteConversation, UpdateConversation, ExportConversations, ExportConversationsToFile, ImportData, SearchConversationHistory } from '@wailsjs/go/app/App';
import { useTranslation } from 'react-i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import type { MenuItem as ContextMenuItem } from '../components/menu';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components/ui/Button';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { Modal } from '../components/ui/Modal';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useConfirm } from '../hooks/useConfirm';
import { useWorkspaceStore } from '../store/workspaceStore';
import { executeDeepLink } from '../lib/deepLinks';
import { formatRelativeTime } from '../lib/dateUtils';
import { downloadJSON, openImportFileDialog, generateFilename } from '../lib/exportImport';
import './HistoryPage.css';

interface Conversation {
  id: number;
  title: string;
  created_at: string;
  updated_at: string;
  message_count: number;
  snippet?: string;
}

interface ImportPreview {
  fileName: string;
  jsonData: string;
  version: number | null;
  appVersion: string;
  exportedAt: string;
  conversationCount: number;
  messageCount: number;
  includesCredentials: boolean;
  requiresCredentialPassword: boolean;
  includeAudio: boolean;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function buildImportPreview(fileName: string, jsonData: string): ImportPreview {
  const parsed: unknown = JSON.parse(jsonData);
  if (!isRecord(parsed)) {
    throw new Error('invalid-import-file');
  }

  const resources = isRecord(parsed.resources) ? parsed.resources : {};
  const options = isRecord(parsed.options) ? parsed.options : {};
  const conversations = Array.isArray(resources.conversations) ? resources.conversations : [];
  const messageCount = conversations.reduce((count, item) => {
    if (!isRecord(item) || !Array.isArray(item.messages)) {
      return count;
    }
    return count + item.messages.length;
  }, 0);
  const credentials = resources.credentials;

  return {
    fileName,
    jsonData,
    version: typeof parsed.version === 'number' ? parsed.version : null,
    appVersion: typeof parsed.appVersion === 'string' ? parsed.appVersion : '',
    exportedAt: typeof parsed.exportedAt === 'string' ? parsed.exportedAt : '',
    conversationCount: conversations.length,
    messageCount,
    includesCredentials: credentials !== undefined && credentials !== null,
    requiresCredentialPassword:
      isRecord(credentials) && credentials.mode === 'encrypted',
    includeAudio: options.includeAudio === true,
  };
}

export default function HistoryPage() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const navigate = useNavigate();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchResultIds, setSearchResultIds] = useState<Set<number> | null>(null);
  const [snippetsMap, setSnippetsMap] = useState<Map<number, string>>(new Map());
  const [searching, setSearching] = useState(false);
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [focusedRow, setFocusedRow] = useState<Conversation | null>(null);
  const [isImportModalOpen, setIsImportModalOpen] = useState(false);
  const [importPreview, setImportPreview] = useState<ImportPreview | null>(null);
  const [importPassword, setImportPassword] = useState('');
  const [importPasswordError, setImportPasswordError] = useState('');
  const [isImporting, setIsImporting] = useState(false);
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'history-page' });
  const moveTabToWorkspace = useWorkspaceStore(state => state.moveTabToWorkspace);
  const addWorkspaceTab = useWorkspaceStore(state => state.addTab);
  const workspaces = useWorkspaceStore(state => state.workspaces);

  useEffect(() => {
    loadConversations();
  }, []);

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
        const ids = new Set<number>();
        const snippets = new Map<number, string>();
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
      console.error('Erro na busca:', error);
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

  const loadConversations = async () => {
    setLoading(true);
    try {
      const result = await GetConversations();
      const mapped = (result || []).map((c: Conversation) => ({
        id: c.id,
        title: c.title || t('history.untitled', 'Sem título'),
        created_at: c.created_at,
        updated_at: c.updated_at,
        message_count: c.message_count || 0
      }));
      setConversations(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar conversas:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleOpenConversation = useCallback(async (conversationId: number, title?: string) => {
    await executeDeepLink(
      { type: 'conversation:open', conversationId, title },
      { navigate },
    );
  }, [navigate]);

  const handleNewConversation = () => {
    navigate('/');
  };

  const handleDeleteConversation = useCallback(async (conversationId: number) => {
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
      console.error('Erro ao deletar conversa:', error);
    }
  }, [confirm, conversations, t]);

  const handleDeleteSelected = useCallback(async () => {
    if (selectedIds.size === 0) return;
    const ids = Array.from(selectedIds);
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
      await Promise.all(ids.map((id) => DeleteConversation(Number(id))));
      const idSet = new Set(ids);
      setConversations((prev) => prev.filter((c) => !idSet.has(c.id)));
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Erro ao deletar conversas:', error);
    }
  }, [confirm, selectedIds, t]);

  const exportJsonByIds = useCallback(async (idsToExport: number[]) => {
    if (idsToExport.length === 0) {
      alert(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'));
      return;
    }

    try {
      const jsonData = await ExportConversations(idsToExport);
      const filename = generateFilename('conversas');
      downloadJSON(jsonData, filename);
    } catch (error) {
      console.error('Erro ao exportar conversas:', error);
      alert(t('history.exportError', 'Erro ao exportar conversas'));
    }
  }, [t]);

  const exportRichByIds = useCallback(async (idsToExport: number[], format: 'html' | 'pdf') => {
    if (idsToExport.length === 0) {
      alert(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'));
      return;
    }

    try {
      const savedPath = await ExportConversationsToFile(idsToExport, format);
      if (!savedPath) return;
      alert(t('history.exportSaved', { path: savedPath, defaultValue: `Arquivo exportado: ${savedPath}` }));
    } catch (error) {
      console.error(`Erro ao exportar conversas em ${format}:`, error);
      alert(t('history.exportError', 'Erro ao exportar conversas'));
    }
  }, [t]);

  const handleExport = useCallback(async () => {
    const idsToExport = selectedIds.size > 0
      ? Array.from(selectedIds).map(id => Number(id))
      : conversations.map(c => c.id);
    await exportJsonByIds(idsToExport);
  }, [conversations, exportJsonByIds, selectedIds]);

  const handleRichExport = useCallback(async (format: 'html' | 'pdf') => {
    const idsToExport = selectedIds.size > 0
      ? Array.from(selectedIds).map(id => Number(id))
      : conversations.map(c => c.id);
    await exportRichByIds(idsToExport, format);
  }, [conversations, exportRichByIds, selectedIds]);

  const closeImportModal = useCallback(() => {
    setIsImportModalOpen(false);
    setImportPreview(null);
    setImportPassword('');
    setImportPasswordError('');
  }, []);

  const selectImportFile = useCallback(async () => {
    const selectedFile = await openImportFileDialog('.json,application/json');
    const preview = buildImportPreview(selectedFile.name, selectedFile.content);
    setImportPreview(preview);
    setImportPassword('');
    setImportPasswordError('');
    setIsImportModalOpen(true);
  }, []);

  const getImportErrorMessage = useCallback((error: unknown) => {
    if (error instanceof Error) {
      if (error.message === 'Nenhum arquivo selecionado') {
        return '';
      }
      if (error.message === 'Erro ao ler arquivo') {
        return t('history.importReadError', 'Erro ao ler o arquivo selecionado.');
      }
    }
    if (error instanceof SyntaxError) {
      return t('history.importInvalidJson', 'O arquivo selecionado não contém um JSON válido.');
    }
    return t('history.importInvalidFile', 'O arquivo selecionado não é um export canônico suportado.');
  }, [t]);

  const handleImport = async () => {
    try {
      await selectImportFile();
    } catch (error) {
      console.error('Erro ao importar conversas:', error);
      const message = getImportErrorMessage(error);
      if (message) {
        alert(message);
      }
    }
  };

  const handleReplaceImportFile = useCallback(async () => {
    try {
      await selectImportFile();
    } catch (error) {
      console.error('Erro ao trocar arquivo de importação:', error);
      const message = getImportErrorMessage(error);
      if (message) {
        alert(message);
      }
    }
  }, [getImportErrorMessage, selectImportFile]);

  const handleConfirmImport = useCallback(async () => {
    if (!importPreview) return;
    if (importPreview.requiresCredentialPassword && !importPassword.trim()) {
      setImportPasswordError(t('history.importPasswordRequired', 'Informe a senha usada para exportar as credenciais.'));
      return;
    }

    setIsImporting(true);
    setImportPasswordError('');
    try {
      const result = await ImportData(importPreview.jsonData, importPassword.trim());
      const details = [
        result.success
          ? t('history.importSuccess', 'Conversas importadas com sucesso!')
          : t('history.importPartial', 'Algumas conversas não puderam ser importadas.'),
        result.message,
        t('history.importCounts', {
          defaultValue: 'Importadas: {{imported}} | Ignoradas: {{skipped}}',
          imported: result.imported,
          skipped: result.skipped,
        }),
      ];

      if (result.errors?.length) {
        details.push(
          `${t('history.importErrorsLabel', 'Erros')}:\n${result.errors.join('\n')}`,
        );
      }

      alert(details.filter(Boolean).join('\n\n'));
      closeImportModal();
      await loadConversations();
    } catch (error) {
      console.error('Erro ao confirmar importação:', error);
      alert(t('history.importError', 'Erro ao importar conversas'));
    } finally {
      setIsImporting(false);
    }
  }, [closeImportModal, importPassword, importPreview, t]);

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
    if (searchResultIds === null) return conversations;
    return conversations.filter(c => searchResultIds.has(c.id));
  }, [conversations, searchResultIds]);

  const handleFocusChange = useCallback((item: Conversation | null) => {
    setFocusedRow(item);
  }, []);

  const handleDeleteRow = useCallback((item: Conversation) => {
    handleDeleteConversation(item.id);
  }, [handleDeleteConversation]);

  const handleSendToWorkspace = useCallback(async (_conversationId: number, title: string, targetWorkspaceId: string, isActive: boolean) => {
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
      console.error('Erro ao enviar conversa ao workspace:', error);
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
            action: () => void exportRichByIds([item.id], 'html'),
          },
          {
            id: 'export-pdf',
            label: t('history.exportPdf', 'Exportar PDF'),
            icon: <FilePdfOutlined />,
            action: () => void exportRichByIds([item.id], 'pdf'),
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
    [exportJsonByIds, exportRichByIds, handleDeleteConversation, handleOpenConversation, handleSendToWorkspace, workspaces, t]
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
        if (snippet) {
          return (
            <span className="history-page__title-cell">
              <span className="history-page__title-text">{item.title}</span>
              <span className="history-page__title-snippet">{snippet}</span>
            </span>
          );
        }
        return item.title;
      },
    },
    {
      key: 'message_count',
      label: t('history.messages', 'Mensagens'),
      width: '15%',
      format: (value) => String(value || 0),
    },
    {
      key: 'created_at',
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
      key: 'updated_at',
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
        console.error('Erro ao atualizar título:', error);
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
              ? t('history.exportSelected', `Exportar JSON (${selectedIds.size})`)
              : t('history.exportAll', 'Exportar JSON'),
            icon: <ExportOutlined />,
            onClick: handleExport,
            variant: 'secondary',
          },
          {
            key: 'export-html',
            label: t('history.exportHtml', 'Exportar HTML'),
            icon: <FileTextOutlined />,
            onClick: () => void handleRichExport('html'),
            variant: 'secondary',
          },
          {
            key: 'export-pdf',
            label: t('history.exportPdf', 'Exportar PDF'),
            icon: <FilePdfOutlined />,
            onClick: () => void handleRichExport('pdf'),
            variant: 'secondary',
          },
          {
            key: 'import',
            label: t('history.import', 'Importar'),
            icon: <ImportOutlined />,
            onClick: handleImport,
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
        onSelectionChange={setSelectedIds}
        onGridReady={handleGridReady}
        onFocusChange={handleFocusChange}
        getRowActions={getRowActions}
      />

      <Modal
        isOpen={isImportModalOpen}
        onClose={closeImportModal}
        title={t('history.importDialogTitle', 'Importar conversas')}
        size="md"
        allowClose={!isImporting}
      >
        <div className="history-page__import-modal">
          <p className="history-page__import-description">
            {t(
              'history.importDialogDescription',
              'Revise o arquivo antes de importar. Apenas o JSON canônico é aceito.'
            )}
          </p>

          {importPreview && (
            <dl className="history-page__import-summary">
              <div className="history-page__import-row">
                <dt>{t('history.importFileLabel', 'Arquivo')}</dt>
                <dd>{importPreview.fileName}</dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importVersionLabel', 'Versão')}</dt>
                <dd>{importPreview.version ?? '-'}</dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importExportedAtLabel', 'Exportado em')}</dt>
                <dd>{importPreview.exportedAt ? formatRelativeTime(new Date(importPreview.exportedAt).getTime()) : '-'}</dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importAppVersionLabel', 'Versão do app')}</dt>
                <dd>{importPreview.appVersion || '-'}</dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importConversationsLabel', 'Conversas')}</dt>
                <dd>{importPreview.conversationCount}</dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importMessagesLabel', 'Mensagens')}</dt>
                <dd>{importPreview.messageCount}</dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importCredentialsLabel', 'Credenciais')}</dt>
                <dd>
                  {importPreview.includesCredentials
                    ? t('history.importCredentialsIncluded', 'Incluídas')
                    : t('history.importCredentialsNotIncluded', 'Não incluídas')}
                </dd>
              </div>
              <div className="history-page__import-row">
                <dt>{t('history.importAudioLabel', 'Áudio')}</dt>
                <dd>
                  {importPreview.includeAudio
                    ? t('common.yes', 'Sim')
                    : t('common.no', 'Não')}
                </dd>
              </div>
            </dl>
          )}

          {importPreview?.requiresCredentialPassword && (
            <FormField
              label={t('history.importPasswordLabel', 'Senha das credenciais')}
              description={t(
                'history.importPasswordDescription',
                'Obrigatória para descriptografar as credenciais exportadas junto com as conversas.'
              )}
              error={importPasswordError || null}
              required
            >
              <Input
                type="password"
                value={importPassword}
                onChange={(event) => {
                  setImportPassword(event.target.value);
                  if (importPasswordError) {
                    setImportPasswordError('');
                  }
                }}
                placeholder={t('history.importPasswordPlaceholder', 'Digite a senha de exportação')}
              />
            </FormField>
          )}

          {!importPreview?.requiresCredentialPassword && importPreview?.includesCredentials && (
            <p className="history-page__import-note">
              {t(
                'history.importCredentialsNotice',
                'Este arquivo inclui credenciais e será importado usando a proteção configurada na instância atual.'
              )}
            </p>
          )}

          <div className="history-page__import-actions">
            <Button
              type="button"
              variant="secondary"
              onClick={() => void handleReplaceImportFile()}
              disabled={isImporting}
            >
              {t('history.importChangeFile', 'Trocar arquivo')}
            </Button>
            <div className="history-page__import-actions-spacer" />
            <Button
              type="button"
              variant="ghost"
              onClick={closeImportModal}
              disabled={isImporting}
            >
              {t('common.cancel', 'Cancelar')}
            </Button>
            <Button
              type="button"
              variant="primary"
              onClick={() => void handleConfirmImport()}
              loading={isImporting}
            >
              {t('history.importConfirm', 'Importar agora')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
