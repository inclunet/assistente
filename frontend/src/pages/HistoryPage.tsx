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
import { AnalyzeImportData, GetConversations, DeleteConversation, UpdateConversation, ExportConversationsToFile, ExportData, ImportData, SearchConversationHistory } from '@wailsjs/go/app/App';
import { useTranslation } from 'react-i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import type { MenuItem as ContextMenuItem } from '../components/menu';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { Modal } from '../components/ui/Modal';
import { useAnnouncer } from '../hooks/useAnnouncer';
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

interface ImportConflict {
  resourceType: string;
  identifier: string;
  reason: string;
}

interface ImportAnalysis {
  version: number;
  appVersion?: string;
  conversationCount: number;
  messageCount: number;
  includesCredentials: boolean;
  requiresCredentialPassword: boolean;
  credentialCount: number;
  conflictCount: number;
  conversationConflicts?: ImportConflict[];
  credentialConflicts?: ImportConflict[];
  unsupportedResourceTypes?: string[];
  warnings?: string[];
  credentialAnalysisError?: string;
}

interface ImportResultSummary {
  success: boolean;
  imported: number;
  skipped: number;
  failed: number;
  skippedEmptyConversations: number;
  skippedConversationConflict: number;
  skippedCredentialConflict: number;
  skippedOther: number;
  errors?: string[];
  message: string;
}

interface ExportRequestPayload {
  conversationIds: string[];
  includeCredentials: boolean;
  credentialExportPassword?: string;
  outputFormat: 'json';
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
  const includesCredentials = options.includeCredentials === true && credentials !== undefined && credentials !== null;

  return {
    fileName,
    jsonData,
    version: typeof parsed.version === 'number' ? parsed.version : null,
    appVersion: typeof parsed.appVersion === 'string' ? parsed.appVersion : '',
    exportedAt: typeof parsed.exportedAt === 'string' ? parsed.exportedAt : '',
    conversationCount: conversations.length,
    messageCount,
    includesCredentials,
    requiresCredentialPassword:
      includesCredentials && isRecord(credentials) && credentials.mode === 'encrypted',
    includeAudio: options.includeAudio === true,
  };
}

function buildImportAnalysisKey(preview: ImportPreview, password: string): string {
  return [
    preview.fileName,
    preview.exportedAt,
    preview.version ?? 'unknown',
    preview.conversationCount,
    preview.messageCount,
    preview.includesCredentials ? 'with-credentials' : 'without-credentials',
    password,
  ].join('::');
}

export default function HistoryPage() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
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
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);
  const [exportTargetIds, setExportTargetIds] = useState<number[]>([]);
  const [includeCredentialExport, setIncludeCredentialExport] = useState(false);
  const [exportPassword, setExportPassword] = useState('');
  const [exportPasswordError, setExportPasswordError] = useState('');
  const [isExporting, setIsExporting] = useState(false);
  const [isImportModalOpen, setIsImportModalOpen] = useState(false);
  const [importPreview, setImportPreview] = useState<ImportPreview | null>(null);
  const [importAnalysis, setImportAnalysis] = useState<ImportAnalysis | null>(null);
  const [lastImportResult, setLastImportResult] = useState<ImportResultSummary | null>(null);
  const [isAnalyzingImport, setIsAnalyzingImport] = useState(false);
  const [importPassword, setImportPassword] = useState('');
  const [importPasswordError, setImportPasswordError] = useState('');
  const [isImporting, setIsImporting] = useState(false);
  const importAnalysisInFlightRef = useRef(false);
  const pendingImportAnalysisRef = useRef<{ jsonData: string; password: string; key: string } | null>(null);
  const lastAnalyzedImportRef = useRef<string | null>(null);
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

  const getTargetConversationIds = useCallback(() => (
    selectedIds.size > 0
      ? Array.from(selectedIds).map((id) => Number(id))
      : conversations.map((c) => c.id)
  ), [conversations, selectedIds]);

  const openExportModal = useCallback((idsToExport: number[]) => {
    if (idsToExport.length === 0) {
      announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
      return;
    }
    setExportTargetIds(idsToExport);
    setIncludeCredentialExport(false);
    setExportPassword('');
    setExportPasswordError('');
    setIsExportModalOpen(true);
  }, [announce, t]);

  const closeExportModal = useCallback(() => {
    setIsExportModalOpen(false);
    setExportTargetIds([]);
    setIncludeCredentialExport(false);
    setExportPassword('');
    setExportPasswordError('');
  }, []);

  const exportJsonByIds = useCallback(async (idsToExport: number[], options?: {
    includeCredentials?: boolean;
    credentialExportPassword?: string;
  }) => {
    try {
      const payload: ExportRequestPayload = {
        conversationIds: idsToExport.map((id) => String(id)),
        includeCredentials: options?.includeCredentials === true,
        outputFormat: 'json',
      };
      if (payload.includeCredentials && options?.credentialExportPassword?.trim()) {
        payload.credentialExportPassword = options.credentialExportPassword.trim();
      }
      const jsonData = await ExportData(payload);
      const filename = generateFilename('conversas');
      downloadJSON(jsonData, filename);
    } catch (error) {
      console.error('Erro ao exportar conversas:', error);
      announce(t('history.exportError', 'Erro ao exportar conversas'), 'assertive');
    }
  }, [announce, t]);

  const exportRichByIds = useCallback(async (idsToExport: number[], format: 'html' | 'pdf') => {
    if (idsToExport.length === 0) {
      announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
      return;
    }

    try {
      const savedPath = await ExportConversationsToFile(idsToExport, format);
      if (!savedPath) return;
      announce(t('history.exportSaved', { path: savedPath, defaultValue: `Arquivo exportado: ${savedPath}` }));
    } catch (error) {
      console.error(`Erro ao exportar conversas em ${format}:`, error);
      announce(t('history.exportError', 'Erro ao exportar conversas'), 'assertive');
    }
  }, [announce, t]);

  const handleExport = useCallback(() => {
    openExportModal(getTargetConversationIds());
  }, [getTargetConversationIds, openExportModal]);

  const handleRichExport = useCallback(async (format: 'html' | 'pdf') => {
    const idsToExport = getTargetConversationIds();
    await exportRichByIds(idsToExport, format);
  }, [exportRichByIds, getTargetConversationIds]);

  const handleConfirmExport = useCallback(async () => {
    if (exportTargetIds.length === 0) {
      announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
      return;
    }
    if (includeCredentialExport && !exportPassword.trim()) {
      setExportPasswordError(t('history.exportPasswordRequired', 'Informe uma senha para criptografar as credenciais exportadas.'));
      return;
    }

    setIsExporting(true);
    setExportPasswordError('');
    try {
      await exportJsonByIds(exportTargetIds, {
        includeCredentials: includeCredentialExport,
        credentialExportPassword: exportPassword,
      });
      closeExportModal();
    } finally {
      setIsExporting(false);
    }
  }, [announce, closeExportModal, exportJsonByIds, exportPassword, exportTargetIds, includeCredentialExport, t]);

  const closeImportModal = useCallback(() => {
    setIsImportModalOpen(false);
    setImportPreview(null);
    setImportAnalysis(null);
    setLastImportResult(null);
    setImportPassword('');
    setImportPasswordError('');
    pendingImportAnalysisRef.current = null;
    lastAnalyzedImportRef.current = null;
  }, []);

  const analyzeImportPayload = useCallback(async (jsonData: string, credentialPassword: string) => {
    setIsAnalyzingImport(true);
    try {
      const analysis = await AnalyzeImportData(jsonData, credentialPassword);
      setImportAnalysis(analysis as ImportAnalysis);
      return analysis as ImportAnalysis;
    } finally {
      setIsAnalyzingImport(false);
    }
  }, []);

  const runLatestImportAnalysis = useCallback(async () => {
    if (importAnalysisInFlightRef.current) {
      return;
    }

    const queuedRequest = pendingImportAnalysisRef.current;
    if (!queuedRequest) {
      return;
    }

    pendingImportAnalysisRef.current = null;
    importAnalysisInFlightRef.current = true;
    const queuedRequestKey = queuedRequest.key;

    try {
      await analyzeImportPayload(queuedRequest.jsonData, queuedRequest.password);
      lastAnalyzedImportRef.current = queuedRequestKey;
    } finally {
      importAnalysisInFlightRef.current = false;
      if (pendingImportAnalysisRef.current) {
        void runLatestImportAnalysis();
      }
    }
  }, [analyzeImportPayload]);

  const selectImportFile = useCallback(async () => {
    const selectedFile = await openImportFileDialog('.json,application/json');
    const preview = buildImportPreview(selectedFile.name, selectedFile.content);
    setImportPreview(preview);
    setImportAnalysis(null);
    setLastImportResult(null);
    setImportPassword('');
    setImportPasswordError('');
    await analyzeImportPayload(selectedFile.content, '');
    lastAnalyzedImportRef.current = buildImportAnalysisKey(preview, '');
    setIsImportModalOpen(true);
  }, [analyzeImportPayload]);

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
        announce(message, 'assertive');
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
        announce(message, 'assertive');
      }
    }
  }, [announce, getImportErrorMessage, selectImportFile]);

  const handleConfirmImport = useCallback(async () => {
    if (lastImportResult) {
      closeImportModal();
      return;
    }
    if (!importPreview) return;
    if (importPreview.requiresCredentialPassword && !importPassword.trim()) {
      setImportPasswordError(t('history.importPasswordRequired', 'Informe a senha usada para exportar as credenciais.'));
      return;
    }
    if (importAnalysis?.credentialAnalysisError) {
      setImportPasswordError(t('history.importPasswordInvalid', 'Não foi possível validar a senha informada para as credenciais.'));
      return;
    }

    setIsImporting(true);
    setImportPasswordError('');
    try {
      const result = await ImportData(importPreview.jsonData, importPassword.trim()) as ImportResultSummary;
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
      if (result.failed > 0) {
        details.push(t('history.importFailedCount', {
          defaultValue: 'Falhas: {{count}}',
          count: result.failed,
        }));
      }
      if (result.skippedEmptyConversations > 0) {
        details.push(t('history.importSkippedEmptyCount', {
          defaultValue: 'Vazias descartadas: {{count}}',
          count: result.skippedEmptyConversations,
        }));
      }
      if (result.skippedConversationConflict > 0) {
        details.push(t('history.importSkippedConversationConflictCount', {
          defaultValue: 'Ignoradas por conflito de conversa: {{count}}',
          count: result.skippedConversationConflict,
        }));
      }
      if (result.skippedCredentialConflict > 0) {
        details.push(t('history.importSkippedCredentialConflictCount', {
          defaultValue: 'Credenciais duplicadas ignoradas: {{count}}',
          count: result.skippedCredentialConflict,
        }));
      }
      if (result.skippedOther > 0) {
        details.push(t('history.importSkippedOtherCount', {
          defaultValue: 'Outros descartes: {{count}}',
          count: result.skippedOther,
        }));
      }

      if (result.errors?.length) {
        details.push(
          `${t('history.importErrorsLabel', 'Erros')}:\n${result.errors.join('\n')}`,
        );
      }

      setLastImportResult(result);
      announce(details.filter(Boolean).join('. '), result.success ? 'polite' : 'assertive');
      await loadConversations();
    } catch (error) {
      console.error('Erro ao confirmar importação:', error);
      announce(t('history.importError', 'Erro ao importar conversas'), 'assertive');
    } finally {
      setIsImporting(false);
    }
  }, [announce, closeImportModal, importAnalysis?.credentialAnalysisError, importPassword, importPreview, lastImportResult, t]);

  useEffect(() => {
    if (!isImportModalOpen || !importPreview) return;
    if (!importPreview.requiresCredentialPassword) return;

    const nextRequest = {
      jsonData: importPreview.jsonData,
      password: importPassword.trim(),
      key: buildImportAnalysisKey(importPreview, importPassword.trim()),
    };
    const requestKey = nextRequest.key;
    if (lastAnalyzedImportRef.current === requestKey) {
      return;
    }

    const timer = window.setTimeout(() => {
      pendingImportAnalysisRef.current = nextRequest;
      void runLatestImportAnalysis();
    }, 600);

    return () => {
      window.clearTimeout(timer);
    };
  }, [importPassword, importPreview, isImportModalOpen, runLatestImportAnalysis]);

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
            action: () => openExportModal([item.id]),
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
    [exportRichByIds, handleDeleteConversation, handleOpenConversation, handleSendToWorkspace, openExportModal, workspaces, t]
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
              ? t('history.exportJsonSelected', {
                  count: selectedIds.size,
                  defaultValue: 'Exportar JSON ({{count}})',
                })
              : t('history.exportJson', 'Exportar JSON'),
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
        isOpen={isExportModalOpen}
        onClose={closeExportModal}
        title={t('history.exportDialogTitle', 'Exportar conversas')}
        size="md"
        allowClose={!isExporting}
      >
        <div className="history-page__import-modal">
          <p className="history-page__import-description">
            {t(
              'history.exportDialogDescription',
              'Exporte o JSON canônico das conversas selecionadas. Esse arquivo é o formato suportado para importação.'
            )}
          </p>

          <dl className="history-page__import-summary">
            <div className="history-page__import-row">
              <dt>{t('history.exportConversationsLabel', 'Conversas')}</dt>
              <dd>{exportTargetIds.length}</dd>
            </div>
            <div className="history-page__import-row">
              <dt>{t('history.exportFormatLabel', 'Formato')}</dt>
              <dd>{t('history.exportJson', 'Exportar JSON')}</dd>
            </div>
            <div className="history-page__import-row">
              <dt>{t('history.exportCredentialsLabel', 'Credenciais')}</dt>
              <dd>
                {includeCredentialExport
                  ? t('history.exportCredentialsIncluded', 'Incluir bloco criptografado')
                  : t('history.exportCredentialsNotIncluded', 'Não incluir')}
              </dd>
            </div>
          </dl>

          <Checkbox
            checked={includeCredentialExport}
            onChange={(event) => {
              const checked = event.target.checked;
              setIncludeCredentialExport(checked);
              if (!checked) {
                setExportPassword('');
                setExportPasswordError('');
              }
            }}
            label={t('history.exportCredentialsOption', 'Incluir credenciais criptografadas no export')}
          />

          {includeCredentialExport && (
            <>
              <p className="history-page__import-note">
                {t(
                  'history.exportCredentialsDescription',
                  'As credenciais serão incluídas em um bloco criptografado. Essa senha será necessária na importação.'
                )}
              </p>
              <FormField
                label={t('history.exportPasswordLabel', 'Senha de exportação')}
                description={t(
                  'history.exportPasswordDescription',
                  'Use uma senha forte. Sem ela, as credenciais exportadas não poderão ser importadas.'
                )}
                error={exportPasswordError || null}
                required
              >
                <Input
                  type="password"
                  value={exportPassword}
                  onChange={(event) => {
                    setExportPassword(event.target.value);
                    if (exportPasswordError) {
                      setExportPasswordError('');
                    }
                  }}
                  placeholder={t('history.exportPasswordPlaceholder', 'Digite a senha de exportação')}
                />
              </FormField>
            </>
          )}

          <div className="history-page__import-actions">
            <div className="history-page__import-actions-spacer" />
            <Button
              type="button"
              variant="ghost"
              onClick={closeExportModal}
              disabled={isExporting}
            >
              {t('common.cancel', 'Cancelar')}
            </Button>
            <Button
              type="button"
              variant="primary"
              onClick={() => void handleConfirmExport()}
              loading={isExporting}
            >
              {t('history.exportConfirm', 'Exportar agora')}
            </Button>
          </div>
        </div>
      </Modal>

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
              'Revise o arquivo antes de importar. Apenas o JSON canônico é aceito nesta fase, com suporte aos recursos já persistidos no banco.'
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

          {isAnalyzingImport && (
            <p className="history-page__import-note">
              {t('history.importAnalyzingConflicts', 'Analisando conflitos do arquivo...')}
            </p>
          )}

          {importAnalysis && !isAnalyzingImport && (
            <div className="history-page__import-analysis">
              <div className="history-page__import-analysis-header">
                <strong>{t('history.importConflictsTitle', 'Conflitos detectados')}</strong>
                <span>
                  {importAnalysis.conflictCount > 0
                    ? t('history.importConflictCount', {
                        defaultValue: '{{count}} conflito(s)',
                        count: importAnalysis.conflictCount,
                      })
                    : t('history.importNoConflicts', 'Nenhum conflito detectado')}
                </span>
              </div>

              {!!importAnalysis.unsupportedResourceTypes?.length && (
                <p className="history-page__import-note">
                  {t('history.importUnsupportedResourcesNotice', {
                    defaultValue: 'Este arquivo inclui recursos fora do escopo atual ({{resources}}). Eles serão ignorados nesta fase e poderão ser suportados após as migrações planejadas nas AEP-0046, AEP-0048, AEP-0050, AEP-0051 e AEP-0052.',
                    resources: importAnalysis.unsupportedResourceTypes.join(', '),
                  })}
                </p>
              )}

              {!!importAnalysis.warnings?.length && (
                <ul className="history-page__import-list history-page__import-list--warning">
                  {importAnalysis.warnings.map((warning) => (
                    <li key={warning}>{warning}</li>
                  ))}
                </ul>
              )}

              {!!importAnalysis.conversationConflicts?.length && (
                <>
                  <strong>{t('history.importConversationConflicts', 'Conversas em conflito')}</strong>
                  <ul className="history-page__import-list">
                    {importAnalysis.conversationConflicts.map((conflict) => (
                      <li key={`${conflict.resourceType}-${conflict.identifier}`}>
                        {conflict.reason}
                      </li>
                    ))}
                  </ul>
                </>
              )}

              {!!importAnalysis.credentialConflicts?.length && (
                <>
                  <strong>{t('history.importCredentialConflicts', 'Credenciais em conflito')}</strong>
                  <ul className="history-page__import-list">
                    {importAnalysis.credentialConflicts.map((conflict) => (
                      <li key={`${conflict.resourceType}-${conflict.identifier}`}>
                        <code>{conflict.identifier}</code>: {conflict.reason}
                      </li>
                    ))}
                  </ul>
                </>
              )}

              {importAnalysis.conflictCount > 0 && (
                <p className="history-page__import-note">
                  {t(
                    'history.importConflictNotice',
                    'Os itens em conflito serão ignorados nesta importação para evitar duplicidade.'
                  )}
                </p>
              )}
            </div>
          )}

          {lastImportResult && (
            <div className="history-page__import-analysis">
              <div className="history-page__import-analysis-header">
                <strong>{t('history.importResultTitle', 'Resultado da importação')}</strong>
                <span>
                  {lastImportResult.success
                    ? t('common.success', 'Sucesso')
                    : t('history.importPartial', 'Algumas conversas não puderam ser importadas.')}
                </span>
              </div>

              <dl className="history-page__import-summary">
                <div className="history-page__import-row">
                  <dt>{t('history.importedLabel', 'Importadas')}</dt>
                  <dd>{lastImportResult.imported}</dd>
                </div>
                <div className="history-page__import-row">
                  <dt>{t('history.skippedLabel', 'Ignoradas')}</dt>
                  <dd>{lastImportResult.skipped}</dd>
                </div>
                <div className="history-page__import-row">
                  <dt>{t('history.importFailedLabel', 'Falhas')}</dt>
                  <dd>{lastImportResult.failed}</dd>
                </div>
                {lastImportResult.skippedEmptyConversations > 0 && (
                  <div className="history-page__import-row">
                    <dt>{t('history.importSkippedEmptyLabel', 'Conversas vazias')}</dt>
                    <dd>{lastImportResult.skippedEmptyConversations}</dd>
                  </div>
                )}
                {lastImportResult.skippedConversationConflict > 0 && (
                  <div className="history-page__import-row">
                    <dt>{t('history.importSkippedConversationConflictLabel', 'Conflitos de conversa')}</dt>
                    <dd>{lastImportResult.skippedConversationConflict}</dd>
                  </div>
                )}
                {lastImportResult.skippedCredentialConflict > 0 && (
                  <div className="history-page__import-row">
                    <dt>{t('history.importSkippedCredentialConflictLabel', 'Credenciais duplicadas')}</dt>
                    <dd>{lastImportResult.skippedCredentialConflict}</dd>
                  </div>
                )}
                {lastImportResult.skippedOther > 0 && (
                  <div className="history-page__import-row">
                    <dt>{t('history.importSkippedOtherLabel', 'Outros descartes')}</dt>
                    <dd>{lastImportResult.skippedOther}</dd>
                  </div>
                )}
              </dl>

              {!!lastImportResult.errors?.length && (
                <>
                  <strong>{t('history.importErrorsLabel', 'Erros')}</strong>
                  <ul className="history-page__import-list">
                    {lastImportResult.errors.map((error) => (
                      <li key={error}>{error}</li>
                    ))}
                  </ul>
                </>
              )}
            </div>
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
              disabled={isAnalyzingImport}
            >
              {lastImportResult
                ? t('common.close', 'Fechar')
                : t('history.importConfirm', 'Importar agora')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
