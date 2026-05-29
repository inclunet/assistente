import { useCallback, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { ExportOutlined, ImportOutlined } from '@ant-design/icons';
import { AnalyzeImportData, ExportData, GetAllTaskLists, GetConversations, GetLLMProvidersWithStatus, ImportData, ListMCPServers } from '@wailsjs/go/app/App';
import { useTranslation } from 'react-i18next';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { downloadJSON, generateFilename, ImportFileError, IMPORT_FILE_ERROR_CODES, openImportFileDialog } from '../lib/exportImport';
import { formatRelativeTime } from '../lib/dateUtils';
import { portability } from '../../wailsjs/go/models';
import './DataManagementPage.css';

interface ConversationRecord {
  id: string;
}

interface ProviderRecord {
  id: string;
}

interface TaskListRecord {
  id: string;
}

interface MCPServerRecord {
  slug: string;
}

interface ImportPreview {
  fileName: string;
  jsonData: string;
  version: number | null;
  appVersion: string;
  exportedAt: string;
  conversationCount: number;
  messageCount: number;
  providerCount: number;
  mcpServerCount: number;
  taskListCount: number;
  taskCount: number;
  taskNoteCount: number;
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
  conflictCount: number;
  conversationConflicts?: ImportConflict[];
  providerConflicts?: ImportConflict[];
  mcpServerConflicts?: ImportConflict[];
  taskListConflicts?: ImportConflict[];
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
  skippedProviderConflict: number;
  skippedMcpServerConflict: number;
  skippedTaskListConflict: number;
  skippedCredentialConflict: number;
  skippedOther: number;
  warnings?: string[];
  errors?: string[];
  message: string;
}

type ExportRequestPayload = portability.ExportRequest;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function countTaskTree(items: unknown[]): { taskCount: number; taskNoteCount: number } {
  return items.reduce<{ taskCount: number; taskNoteCount: number }>((counts, item) => {
    if (!isRecord(item)) return counts;
    const notes = Array.isArray(item.notes) ? item.notes.length : 0;
    const children = Array.isArray(item.children) ? item.children : [];
    const childCounts = countTaskTree(children);
    return {
      taskCount: counts.taskCount + 1 + childCounts.taskCount,
      taskNoteCount: counts.taskNoteCount + notes + childCounts.taskNoteCount,
    };
  }, { taskCount: 0, taskNoteCount: 0 });
}

function buildImportPreview(fileName: string, jsonData: string): ImportPreview {
  const parsed: unknown = JSON.parse(jsonData);
  if (!isRecord(parsed)) throw new Error('invalid-import-file');

  const resources = isRecord(parsed.resources) ? parsed.resources : {};
  const options = isRecord(parsed.options) ? parsed.options : {};
  const conversations = Array.isArray(resources.conversations) ? resources.conversations : [];
  const taskLists = Array.isArray(resources.taskLists) ? resources.taskLists : [];
  const providers = Array.isArray(resources.providers) ? resources.providers : [];
  const mcpServers = Array.isArray(resources.mcpServers) ? resources.mcpServers : [];
  const mcpServersExternalMap = isRecord(parsed.mcpServers) ? parsed.mcpServers : null;
  const mcpServerCount = mcpServers.length > 0
    ? mcpServers.length
    : (mcpServersExternalMap ? Object.keys(mcpServersExternalMap).length : 0);
  const messageCount = conversations.reduce((count, item) => (
    isRecord(item) && Array.isArray(item.messages) ? count + item.messages.length : count
  ), 0);
  const taskCounts = taskLists.reduce<{ taskCount: number; taskNoteCount: number }>((counts, item) => {
    if (!isRecord(item) || !Array.isArray(item.tasks)) return counts;
    const treeCounts = countTaskTree(item.tasks);
    return {
      taskCount: counts.taskCount + treeCounts.taskCount,
      taskNoteCount: counts.taskNoteCount + treeCounts.taskNoteCount,
    };
  }, { taskCount: 0, taskNoteCount: 0 });
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
    providerCount: providers.length,
    mcpServerCount,
    taskListCount: taskLists.length,
    taskCount: taskCounts.taskCount,
    taskNoteCount: taskCounts.taskNoteCount,
    includesCredentials,
    requiresCredentialPassword: includesCredentials && isRecord(credentials) && credentials.mode === 'encrypted',
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
    preview.providerCount,
    preview.mcpServerCount,
    preview.taskListCount,
    preview.taskCount,
    preview.taskNoteCount,
    preview.includesCredentials ? 'with-credentials' : 'without-credentials',
    password,
  ].join('::');
}

export default function DataManagementPage() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const location = useLocation();
  const navigate = useNavigate();

  const [conversationIds, setConversationIds] = useState<string[]>([]);
  const [loadingConversations, setLoadingConversations] = useState(true);
  const [includeConversations, setIncludeConversations] = useState(true);
  const [includeProvidersExport, setIncludeProvidersExport] = useState(false);
  const [exportProviderIds, setExportProviderIds] = useState<string[]>([]);
  const [includeMcpServersExport, setIncludeMcpServersExport] = useState(false);
  const [exportMcpServerSlugs, setExportMcpServerSlugs] = useState<string[]>([]);
  const [exportMcpExternalFormat, setExportMcpExternalFormat] = useState(false);
  const [includeTaskListsExport, setIncludeTaskListsExport] = useState(false);
  const [exportTaskListIds, setExportTaskListIds] = useState<string[]>([]);
  const [includeCredentialExport, setIncludeCredentialExport] = useState(false);
  const [exportPassword, setExportPassword] = useState('');
  const [exportPasswordError, setExportPasswordError] = useState('');
  const [isExporting, setIsExporting] = useState(false);

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
  const handledActionSearchRef = useRef<string | null>(null);

  const loadConversations = useCallback(async () => {
    setLoadingConversations(true);
    try {
      const conversations = await GetConversations() as ConversationRecord[];
      const ids = (conversations || [])
        .map((conversation) => String(conversation.id ?? '').trim())
        .filter((id) => id.length > 0);
      setConversationIds(ids);
    } catch (error) {
      console.error('Erro ao carregar conversas para exportação:', error);
      setConversationIds([]);
    } finally {
      setLoadingConversations(false);
    }
  }, []);

  useEffect(() => {
    if (location.pathname !== '/settings/data') {
      setLoadingConversations(false);
      return;
    }
    void loadConversations();
  }, [loadConversations, location.pathname]);

  const loadExportProviderIds = useCallback(async () => {
    const providers = await GetLLMProvidersWithStatus() as ProviderRecord[];
    const ids = (providers || [])
      .map((provider) => String(provider.id ?? '').trim())
      .filter((id) => id.length > 0);
    setExportProviderIds(ids);
    return ids;
  }, []);

  const loadExportTaskListIds = useCallback(async () => {
    const taskLists = await GetAllTaskLists() as TaskListRecord[];
    const ids = (taskLists || [])
      .map((taskList) => String(taskList.id ?? '').trim())
      .filter((id) => id.length > 0);
    setExportTaskListIds(ids);
    return ids;
  }, []);

  const loadExportMcpServerSlugs = useCallback(async () => {
    const servers = await ListMCPServers() as MCPServerRecord[];
    const slugs = (servers || [])
      .map((server) => String(server.slug ?? '').trim())
      .filter((slug) => slug.length > 0);
    setExportMcpServerSlugs(slugs);
    return slugs;
  }, []);

  const handleConfirmExport = useCallback(async () => {
    if (includeCredentialExport && !exportPassword.trim()) {
      setExportPasswordError(t('history.exportPasswordRequired', 'Informe uma senha para criptografar as credenciais exportadas.'));
      return;
    }

    setIsExporting(true);
    setExportPasswordError('');
    try {
      let providerIdsToExport: string[] = [];
      if (includeProvidersExport) {
        providerIdsToExport = exportProviderIds.length > 0 ? exportProviderIds : await loadExportProviderIds();
      }

      let mcpServerSlugsToExport: string[] = [];
      if (includeMcpServersExport) {
        mcpServerSlugsToExport = exportMcpServerSlugs.length > 0 ? exportMcpServerSlugs : await loadExportMcpServerSlugs();
      }

      let taskListIdsToExport: string[] = [];
      if (includeTaskListsExport) {
        taskListIdsToExport = exportTaskListIds.length > 0 ? exportTaskListIds : await loadExportTaskListIds();
      }

      const idsToExport = includeConversations && !exportMcpExternalFormat ? conversationIds : [];
      const hasResourcesToExport =
        idsToExport.length > 0 ||
        providerIdsToExport.length > 0 ||
        mcpServerSlugsToExport.length > 0 ||
        taskListIdsToExport.length > 0 ||
        includeCredentialExport;
      if (!hasResourcesToExport) {
        announce(t('history.noDataToExport', 'Nenhum dado selecionado para exportar'), 'assertive');
        return;
      }

      const payload: ExportRequestPayload = {
        all: false,
        explicitSelection: true,
        includeContacts: false,
        includeWorkspace: false,
        includeAudio: false,
        includeCredentials: includeCredentialExport,
        outputFormat: exportMcpExternalFormat ? 'mcp-json' : 'json',
      };
      if (idsToExport.length > 0) payload.conversationIds = idsToExport;
      if (providerIdsToExport.length > 0) payload.providerIds = providerIdsToExport;
      if (mcpServerSlugsToExport.length > 0) payload.mcpServerSlugs = mcpServerSlugsToExport;
      if (taskListIdsToExport.length > 0) payload.taskListIds = taskListIdsToExport;
      if (payload.includeCredentials && exportPassword.trim()) {
        payload.credentialExportPassword = exportPassword.trim();
      }

      const jsonData = await ExportData(payload);
      downloadJSON(jsonData, generateFilename('dados'));
      announce(t('history.exportSuccess', 'Dados exportados com sucesso!'));
    } catch (error) {
      console.error('Erro ao exportar dados:', error);
      announce(t('history.exportError', 'Erro ao exportar dados'), 'assertive');
    } finally {
      setIsExporting(false);
    }
  }, [announce, conversationIds, exportMcpExternalFormat, exportMcpServerSlugs, exportPassword, exportProviderIds, exportTaskListIds, includeConversations, includeCredentialExport, includeMcpServersExport, includeProvidersExport, includeTaskListsExport, loadExportMcpServerSlugs, loadExportProviderIds, loadExportTaskListIds, t]);

  const analyzeImportPayload = useCallback(async (jsonData: string, credentialPassword: string) => {
    setIsAnalyzingImport(true);
    try {
      const analysis = await AnalyzeImportData(jsonData, credentialPassword);
      const nextAnalysis = analysis as ImportAnalysis;
      setImportAnalysis(nextAnalysis);
      return nextAnalysis;
    } finally {
      setIsAnalyzingImport(false);
    }
  }, []);

  const runLatestImportAnalysis = useCallback(async () => {
    if (importAnalysisInFlightRef.current) return;
    const queuedRequest = pendingImportAnalysisRef.current;
    if (!queuedRequest) return;

    pendingImportAnalysisRef.current = null;
    importAnalysisInFlightRef.current = true;
    const queuedRequestKey = queuedRequest.key;
    try {
      await analyzeImportPayload(queuedRequest.jsonData, queuedRequest.password);
      lastAnalyzedImportRef.current = queuedRequestKey;
    } finally {
      importAnalysisInFlightRef.current = false;
      if (pendingImportAnalysisRef.current) void runLatestImportAnalysis();
    }
  }, [analyzeImportPayload]);

  const resetImportState = useCallback(() => {
    setImportPreview(null);
    setImportAnalysis(null);
    setLastImportResult(null);
    setImportPassword('');
    setImportPasswordError('');
    pendingImportAnalysisRef.current = null;
    lastAnalyzedImportRef.current = null;
  }, []);

  const getImportErrorMessage = useCallback((error: unknown) => {
    if (error instanceof ImportFileError) {
      if (error.code === IMPORT_FILE_ERROR_CODES.NO_FILE_SELECTED) return '';
      if (error.code === IMPORT_FILE_ERROR_CODES.FILE_READ_ERROR) {
        return t('history.importReadError', 'Erro ao ler o arquivo selecionado.');
      }
    }
    if (error instanceof SyntaxError) {
      return t('history.importInvalidJson', 'O arquivo selecionado não contém um JSON válido.');
    }
    return t('history.importInvalidFile', 'O arquivo selecionado não é um export canônico suportado.');
  }, [t]);

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
  }, [analyzeImportPayload]);

  const handleSelectImportFile = useCallback(async () => {
    try {
      await selectImportFile();
    } catch (error) {
      console.error('Erro ao selecionar arquivo de importação:', error);
      const message = getImportErrorMessage(error);
      if (message) announce(message, 'assertive');
    }
  }, [announce, getImportErrorMessage, selectImportFile]);

  const handleConfirmImport = useCallback(async () => {
    if (lastImportResult) {
      resetImportState();
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
      setLastImportResult(result);
      await loadConversations();
      announce(
        [
          result.success
            ? t('history.importSuccess', 'Dados importados com sucesso!')
            : t('history.importPartial', 'Alguns recursos não puderam ser importados.'),
          result.message,
          t('history.importCounts', {
            defaultValue: 'Importados: {{imported}} | Ignorados: {{skipped}}',
            imported: result.imported,
            skipped: result.skipped,
          }),
        ].filter(Boolean).join('. '),
        result.success ? 'polite' : 'assertive',
      );
    } catch (error) {
      console.error('Erro ao confirmar importação:', error);
      announce(t('history.importError', 'Erro ao importar conversas'), 'assertive');
    } finally {
      setIsImporting(false);
    }
  }, [announce, importAnalysis?.credentialAnalysisError, importPassword, importPreview, lastImportResult, loadConversations, resetImportState, t]);

  useEffect(() => {
    if (!importPreview?.requiresCredentialPassword) return;
    const nextRequest = {
      jsonData: importPreview.jsonData,
      password: importPassword.trim(),
      key: buildImportAnalysisKey(importPreview, importPassword.trim()),
    };
    if (lastAnalyzedImportRef.current === nextRequest.key) return;
    const timer = window.setTimeout(() => {
      pendingImportAnalysisRef.current = nextRequest;
      void runLatestImportAnalysis();
    }, 600);
    return () => window.clearTimeout(timer);
  }, [importPassword, importPreview, runLatestImportAnalysis]);

  useEffect(() => {
    if (location.pathname !== '/settings/data') return;
    const params = new URLSearchParams(location.search);
    const action = (params.get('action') || '').trim().toLowerCase();
    if (action !== 'export' && action !== 'import') {
      handledActionSearchRef.current = null;
      return;
    }
    if (handledActionSearchRef.current === location.search) {
      return;
    }
    if (action === 'import') void handleSelectImportFile();
    if (action === 'export') {
      document.getElementById('data-export-section')?.scrollIntoView?.({ block: 'start' });
    }
    handledActionSearchRef.current = location.search;
    navigate('/settings/data', { replace: true });
  }, [handleSelectImportFile, location.pathname, location.search, navigate]);

  const exportedAtTimestamp = importPreview?.exportedAt ? Date.parse(importPreview.exportedAt) : NaN;
  const exportedAtLabel = Number.isFinite(exportedAtTimestamp) ? formatRelativeTime(exportedAtTimestamp) : '-';
  const isExportButtonDisabled = includeConversations && !exportMcpExternalFormat && loadingConversations;

  const renderConflictGroup = useCallback((title: string, conflicts: ImportConflict[] | undefined) => {
    if (!conflicts?.length) return null;
    return (
      <>
        <strong>{title}</strong>
        <ul className="data-management__list">
          {conflicts.map((conflict) => (
            <li key={`${conflict.resourceType}:${conflict.identifier}`}>
              {conflict.resourceType === 'conversation'
                ? conflict.reason
                : <><code>{conflict.identifier}</code>: {conflict.reason}</>}
            </li>
          ))}
        </ul>
      </>
    );
  }, []);

  return (
    <div className="data-management-page">
      <header className="data-management-page__header">
        <h1>{t('dataManagement.title', 'Importação e exportação')}</h1>
        <p>
          {t(
            'dataManagement.description',
            'Centralize backups, migrações e restaurações dos dados persistidos do Assistente.',
          )}
        </p>
      </header>

      <section id="data-export-section" className="data-management-card" aria-labelledby="data-export-title">
        <div className="data-management-card__header">
          <ExportOutlined aria-hidden="true" />
          <div>
            <h2 id="data-export-title">{t('history.exportDialogTitle', 'Exportar dados')}</h2>
            <p>{t('history.exportDialogDescription', 'Exporte o JSON canônico dos dados persistidos no banco. Esse arquivo é o formato suportado para importação.')}</p>
          </div>
        </div>

        <dl className="data-management__summary">
          <div><dt>{t('history.exportConversationsLabel', 'Conversas')}</dt><dd>{loadingConversations ? t('common.loading', 'Carregando...') : conversationIds.length}</dd></div>
          <div><dt>{t('history.exportProvidersLabel', 'Providers')}</dt><dd>{includeProvidersExport ? exportProviderIds.length : t('history.exportProvidersNotIncluded', 'Não incluir')}</dd></div>
          <div><dt>{t('history.exportMcpServersLabel', 'Servidores MCP')}</dt><dd>{includeMcpServersExport ? exportMcpServerSlugs.length : t('history.exportMcpServersNotIncluded', 'Não incluir')}</dd></div>
          <div><dt>{t('history.exportTaskListsLabel', 'Tasklists')}</dt><dd>{includeTaskListsExport ? exportTaskListIds.length : t('history.exportTaskListsNotIncluded', 'Não incluir')}</dd></div>
          <div><dt>{t('history.exportFormatLabel', 'Formato')}</dt><dd>{exportMcpExternalFormat ? t('history.exportMcpJson', 'Exportar MCP JSON') : t('history.exportJson', 'Exportar JSON')}</dd></div>
        </dl>

        <div className="data-management__options">
          <Checkbox
            checked={includeConversations}
            disabled={exportMcpExternalFormat}
            onChange={(event) => setIncludeConversations(event.target.checked)}
            label={t('dataManagement.exportConversationsOption', 'Incluir conversas do histórico')}
          />
          <Checkbox
            checked={includeProvidersExport}
            disabled={exportMcpExternalFormat}
            onChange={(event) => {
              const checked = event.target.checked;
              setIncludeProvidersExport(checked);
              if (!checked) {
                setExportProviderIds([]);
                return;
              }
              void loadExportProviderIds().catch((error) => {
                console.error('Erro ao carregar providers para exportação:', error);
                setIncludeProvidersExport(false);
                setExportProviderIds([]);
                announce(t('history.exportProvidersLoadError', 'Erro ao carregar providers para exportação'), 'assertive');
              });
            }}
            label={t('history.exportProvidersOption', 'Incluir providers persistidos no banco')}
          />
          <Checkbox
            checked={includeMcpServersExport}
            onChange={(event) => {
              const checked = event.target.checked;
              setIncludeMcpServersExport(checked);
              if (!checked) {
                setExportMcpServerSlugs([]);
                setExportMcpExternalFormat(false);
                return;
              }
              void loadExportMcpServerSlugs().catch((error) => {
                console.error('Erro ao carregar servidores MCP para exportação:', error);
                setIncludeMcpServersExport(false);
                setExportMcpServerSlugs([]);
                setExportMcpExternalFormat(false);
                announce(t('history.exportMcpServersLoadError', 'Erro ao carregar servidores MCP para exportação'), 'assertive');
              });
            }}
            label={t('history.exportMcpServersOption', 'Incluir servidores MCP persistidos no banco')}
          />
          {includeMcpServersExport && (
            <Checkbox
              checked={exportMcpExternalFormat}
              onChange={(event) => {
                const checked = event.target.checked;
                setExportMcpExternalFormat(checked);
                if (checked) {
                  setIncludeConversations(false);
                  setIncludeProvidersExport(false);
                  setExportProviderIds([]);
                  setIncludeTaskListsExport(false);
                  setExportTaskListIds([]);
                  setIncludeCredentialExport(false);
                  setExportPassword('');
                  setExportPasswordError('');
                }
              }}
              label={t('history.exportMcpJsonOption', 'Exportar servidores MCP em formato compatível (mcp-json)')}
            />
          )}
          <Checkbox
            checked={includeTaskListsExport}
            disabled={exportMcpExternalFormat}
            onChange={(event) => {
              const checked = event.target.checked;
              setIncludeTaskListsExport(checked);
              if (!checked) {
                setExportTaskListIds([]);
                return;
              }
              void loadExportTaskListIds().catch((error) => {
                console.error('Erro ao carregar tasklists para exportação:', error);
                setIncludeTaskListsExport(false);
                setExportTaskListIds([]);
                announce(t('history.exportTaskListsLoadError', 'Erro ao carregar tasklists para exportação'), 'assertive');
              });
            }}
            label={t('history.exportTaskListsOption', 'Incluir tasklists persistidas no banco')}
          />
          <Checkbox
            checked={includeCredentialExport}
            disabled={exportMcpExternalFormat}
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
        </div>

        {includeCredentialExport && (
          <FormField
            label={t('history.exportPasswordLabel', 'Senha de exportação')}
            description={t('history.exportPasswordDescription', 'Use uma senha forte. Sem ela, as credenciais exportadas não poderão ser importadas.')}
            error={exportPasswordError || null}
            required
          >
            <Input
              type="password"
              value={exportPassword}
              onChange={(event) => {
                setExportPassword(event.target.value);
                if (exportPasswordError) setExportPasswordError('');
              }}
              placeholder={t('history.exportPasswordPlaceholder', 'Digite a senha de exportação')}
            />
          </FormField>
        )}

        <div className="data-management-card__actions">
          <Button
            type="button"
            variant="primary"
            onClick={() => void handleConfirmExport()}
            loading={isExporting}
            disabled={isExportButtonDisabled}
          >
            {t('history.exportConfirm', 'Exportar agora')}
          </Button>
        </div>
      </section>

      <section className="data-management-card" aria-labelledby="data-import-title">
        <div className="data-management-card__header">
          <ImportOutlined aria-hidden="true" />
          <div>
            <h2 id="data-import-title">{t('history.importDialogTitle', 'Importar dados')}</h2>
            <p>{t('history.importDialogDescription', 'Revise o arquivo antes de importar. Apenas o JSON canônico é aceito nesta fase, com suporte aos recursos já persistidos no banco.')}</p>
          </div>
        </div>

        <div className="data-management-card__actions data-management-card__actions--start">
          <Button type="button" variant="secondary" onClick={() => void handleSelectImportFile()} disabled={isImporting}>
            {importPreview ? t('history.importChangeFile', 'Trocar arquivo') : t('dataManagement.selectImportFile', 'Selecionar arquivo JSON')}
          </Button>
          {importPreview && (
            <Button type="button" variant="ghost" onClick={resetImportState} disabled={isImporting}>
              {t('common.cancel', 'Cancelar')}
            </Button>
          )}
        </div>

        {importPreview && (
          <dl className="data-management__summary">
            <div><dt>{t('history.importFileLabel', 'Arquivo')}</dt><dd>{importPreview.fileName}</dd></div>
            <div><dt>{t('history.importVersionLabel', 'Versão')}</dt><dd>{importPreview.version ?? '-'}</dd></div>
            <div><dt>{t('history.importExportedAtLabel', 'Exportado em')}</dt><dd>{exportedAtLabel}</dd></div>
            <div><dt>{t('history.importConversationsLabel', 'Conversas')}</dt><dd>{importPreview.conversationCount}</dd></div>
            <div><dt>{t('history.importMessagesLabel', 'Mensagens')}</dt><dd>{importPreview.messageCount}</dd></div>
            <div><dt>{t('history.importProvidersLabel', 'Providers')}</dt><dd>{importPreview.providerCount}</dd></div>
            <div><dt>{t('history.importMcpServersLabel', 'Servidores MCP')}</dt><dd>{importPreview.mcpServerCount}</dd></div>
            <div><dt>{t('history.importTaskListsLabel', 'Tasklists')}</dt><dd>{importPreview.taskListCount}</dd></div>
            <div><dt>{t('history.importCredentialsLabel', 'Credenciais')}</dt><dd>{importPreview.includesCredentials ? t('history.importCredentialsIncluded', 'Incluídas') : t('history.importCredentialsNotIncluded', 'Não incluídas')}</dd></div>
          </dl>
        )}

        {isAnalyzingImport && <p className="data-management__note">{t('history.importAnalyzingConflicts', 'Analisando conflitos do arquivo...')}</p>}

        {importAnalysis && !isAnalyzingImport && (
          <div className="data-management__analysis">
            <div className="data-management__analysis-header">
              <strong>{t('history.importConflictsTitle', 'Conflitos detectados')}</strong>
              <span>
                {importAnalysis.conflictCount > 0
                  ? t('history.importConflictCount', { defaultValue: '{{count}} conflito(s)', count: importAnalysis.conflictCount })
                  : t('history.importNoConflicts', 'Nenhum conflito detectado')}
              </span>
            </div>

            {!!importAnalysis.unsupportedResourceTypes?.length && (
              <p className="data-management__note">
                {t('history.importUnsupportedResourcesNotice', {
                  defaultValue: 'Este arquivo inclui recursos fora do escopo atual ({{resources}}). Eles serão ignorados nesta fase.',
                  resources: importAnalysis.unsupportedResourceTypes.join(', '),
                })}
              </p>
            )}
            {!!importAnalysis.warnings?.length && (
              <ul className="data-management__list data-management__list--warning">
                {importAnalysis.warnings.map((warning) => <li key={warning}>{warning}</li>)}
              </ul>
            )}
            {renderConflictGroup(t('history.importConversationConflicts', 'Conversas em conflito'), importAnalysis.conversationConflicts)}
            {renderConflictGroup(t('history.importProviderConflicts', 'Providers em conflito'), importAnalysis.providerConflicts)}
            {renderConflictGroup(t('history.importMcpServerConflicts', 'Servidores MCP em conflito'), importAnalysis.mcpServerConflicts)}
            {renderConflictGroup(t('history.importTaskListConflicts', 'Tasklists em conflito'), importAnalysis.taskListConflicts)}
            {renderConflictGroup(t('history.importCredentialConflicts', 'Credenciais em conflito'), importAnalysis.credentialConflicts)}
          </div>
        )}

        {lastImportResult && (
          <div className="data-management__analysis">
            <div className="data-management__analysis-header">
              <strong>{t('history.importResultTitle', 'Resultado da importação')}</strong>
              <span>{lastImportResult.success ? t('common.success', 'Sucesso') : t('history.importPartial', 'Alguns recursos não puderam ser importados.')}</span>
            </div>
            <dl className="data-management__summary">
              <div><dt>{t('history.importedLabel', 'Importadas')}</dt><dd>{lastImportResult.imported}</dd></div>
              <div><dt>{t('history.skippedLabel', 'Ignoradas')}</dt><dd>{lastImportResult.skipped}</dd></div>
              <div><dt>{t('history.importFailedLabel', 'Falhas')}</dt><dd>{lastImportResult.failed}</dd></div>
            </dl>
            {!!lastImportResult.errors?.length && (
              <div>
                <strong>{t('history.importErrorsLabel', 'Erros')}</strong>
                <ul className="data-management__list" aria-label={t('history.importErrorsLabel', 'Erros')}>
                  {lastImportResult.errors.map((error) => <li key={error}>{error}</li>)}
                </ul>
              </div>
            )}
            {!!lastImportResult.warnings?.length && (
              <div>
                <strong>{t('history.importWarningsLabel', 'Avisos')}</strong>
                <ul className="data-management__list data-management__list--warning" aria-label={t('history.importWarningsLabel', 'Avisos')}>
                  {lastImportResult.warnings.map((warning) => <li key={warning}>{warning}</li>)}
                </ul>
              </div>
            )}
          </div>
        )}

        {importPreview?.requiresCredentialPassword && (
          <FormField
            label={t('history.importPasswordLabel', 'Senha das credenciais')}
            description={t('history.importPasswordDescription', 'Obrigatória para descriptografar as credenciais exportadas junto com as conversas.')}
            error={importPasswordError || null}
            required
          >
            <Input
              type="password"
              value={importPassword}
              onChange={(event) => {
                setImportPassword(event.target.value);
                if (importPasswordError) setImportPasswordError('');
              }}
              placeholder={t('history.importPasswordPlaceholder', 'Digite a senha de exportação')}
            />
          </FormField>
        )}

        {importPreview && (
          <div className="data-management-card__actions">
            <Button
              type="button"
              variant="primary"
              onClick={() => void handleConfirmImport()}
              loading={isImporting}
              disabled={isAnalyzingImport}
            >
              {lastImportResult ? t('common.close', 'Fechar') : t('history.importConfirm', 'Importar agora')}
            </Button>
          </div>
        )}
      </section>
    </div>
  );
}
