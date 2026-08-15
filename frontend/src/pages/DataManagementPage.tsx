import { logger } from '../utils/logger';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { ExportOutlined, ImportOutlined } from '@ant-design/icons';
import { AnalyzeImportData, ExportData, GetConversations, ImportData } from '@wailsjs/go/app/App';
import { GetAllTaskLists } from '@wailsjs/go/wailsapi/Tasklist';
import { GetLLMProvidersWithStatus } from '@wailsjs/go/wailsapi/LLMProviders';
import { GetDatabaseStats, GetMaintenanceSettings, RunDatabaseMaintenance, SaveMaintenanceSettings } from '@wailsjs/go/wailsapi/Database';
import { ListMCPServers } from '@wailsjs/go/wailsapi/MCP';
import { ListMemoryRecords } from '@wailsjs/go/wailsapi/Memory';
import { useTranslation } from 'react-i18next';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import { downloadJSON, generateFilename, ImportFileError, IMPORT_FILE_ERROR_CODES, openImportFileDialog } from '../lib/exportImport';
import { formatPortabilityMessage, portabilityMessageKey, type PortabilityMessage } from '../lib/portabilityMessages';
import { formatRelativeTime } from '../lib/dateUtils';
import { config, database, memory, portability } from '../../wailsjs/go/models';
import './DataManagementPage.css';

const BYTES_PER_MIB = 1024 * 1024;

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const rounded = unitIndex === 0 ? value : Math.round(value * 10) / 10;
  return `${rounded} ${units[unitIndex]}`;
}

function getErrorReason(error: unknown): string | null {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === 'string' && error.trim()) return error;
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === 'string' && message.trim()) return message;
  }
  return null;
}

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
  memoryRecordCount: number;
  includesCredentials: boolean;
  requiresCredentialPassword: boolean;
  includeAudio: boolean;
}

interface ImportConflict {
  resourceType: string;
  identifier: string;
  reason: PortabilityMessage | string;
}

interface ImportAnalysis {
  conflictCount: number;
  conversationConflicts?: ImportConflict[];
  providerConflicts?: ImportConflict[];
  mcpServerConflicts?: ImportConflict[];
  taskListConflicts?: ImportConflict[];
  credentialConflicts?: ImportConflict[];
  unsupportedResourceTypes?: string[];
  warnings?: (PortabilityMessage | string)[];
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
  warnings?: (PortabilityMessage | string)[];
  errors?: (PortabilityMessage | string)[];
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
  const memoryRecords = Array.isArray(resources.memoryRecords) ? resources.memoryRecords : [];
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
    memoryRecordCount: memoryRecords.length,
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
    preview.memoryRecordCount,
    preview.includesCredentials ? 'with-credentials' : 'without-credentials',
    password,
  ].join('::');
}

export default function DataManagementPage() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const location = useLocation();
  const navigate = useNavigate();
  useContentPageLandmarks({ pageClass: 'data-management-page' });

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
  const [includeMemoriesExport, setIncludeMemoriesExport] = useState(false);
  const [exportMemoryRecordIds, setExportMemoryRecordIds] = useState<string[]>([]);
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
  const importAnalysisInFlightKeyRef = useRef<string | null>(null);
  const pendingImportAnalysisRef = useRef<{ jsonData: string; password: string; key: string } | null>(null);
  const lastAnalyzedImportRef = useRef<string | null>(null);
  const activeImportAnalysisKeyRef = useRef<string | null>(null);
  const handledActionSearchRef = useRef<string | null>(null);

  const [maintenance, setMaintenance] = useState<config.MaintenanceSettings | null>(null);
  const [dbStats, setDbStats] = useState<database.DatabaseStats | null>(null);
  const [isLoadingMaintenance, setIsLoadingMaintenance] = useState(true);
  const [isSavingMaintenance, setIsSavingMaintenance] = useState(false);
  const [isCompacting, setIsCompacting] = useState(false);
  const [maintenanceRunError, setMaintenanceRunError] = useState('');

  const loadMaintenance = useCallback(async () => {
    setIsLoadingMaintenance(true);
    // Carrega política e estatísticas de forma independente: uma falha ao ler
    // o tamanho do banco não deve impedir a edição/salvamento da política.
    const [settingsResult, statsResult] = await Promise.allSettled([
      GetMaintenanceSettings(),
      GetDatabaseStats(),
    ]);
    if (settingsResult.status === 'fulfilled') {
      setMaintenance(config.MaintenanceSettings.createFrom(settingsResult.value));
    } else {
      logger.error('Erro ao carregar política de manutenção:', settingsResult.reason);
    }
    if (statsResult.status === 'fulfilled') {
      setDbStats(database.DatabaseStats.createFrom(statsResult.value));
    } else {
      logger.error('Erro ao carregar estatísticas do banco:', statsResult.reason);
    }
    setIsLoadingMaintenance(false);
  }, []);

  const updateMaintenanceField = useCallback(
    (field: keyof config.MaintenanceSettings, value: number) => {
      setMaintenance((prev) => {
        if (!prev) return prev;
        const next = config.MaintenanceSettings.createFrom(prev);
        // O backend espera int/int64; trunca para inteiro não-negativo para
        // evitar falha de unmarshalling (ex.: usuário cola "1.5" no input).
        next[field] = Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
        return next;
      });
    },
    [],
  );

  const handleSaveMaintenance = useCallback(async () => {
    if (!maintenance) return;
    setIsSavingMaintenance(true);
    try {
      await SaveMaintenanceSettings(maintenance);
      // Recarrega do backend: SaveMaintenance normaliza valores inválidos
      // (ex.: job_retention_hours <= 0 vira default). Valores 0 explícitos em
      // runs_per_job_keep/vacuum_min_free_bytes são escolhas válidas e
      // preservados. A UI deve refletir o que foi efetivamente persistido.
      const persisted = await GetMaintenanceSettings();
      setMaintenance(config.MaintenanceSettings.createFrom(persisted));
      announce(t('dataManagement.maintenanceSaved', 'Política de manutenção salva.'));
    } catch (error) {
      logger.error('Erro ao salvar manutenção do banco:', error);
      announce(t('dataManagement.maintenanceSaveError', 'Erro ao salvar a política de manutenção.'), 'assertive');
    } finally {
      setIsSavingMaintenance(false);
    }
  }, [announce, maintenance, t]);

  const handleRunMaintenance = useCallback(async () => {
    setIsCompacting(true);
    setMaintenanceRunError('');
    try {
      const result = await RunDatabaseMaintenance(true);
      const stats = await GetDatabaseStats();
      setDbStats(database.DatabaseStats.createFrom(stats));
      announce(
        t('dataManagement.maintenanceRunDone', {
          defaultValue: 'Manutenção concluída. Espaço liberado: {{reclaimed}}.',
          reclaimed: formatBytes(result.reclaimedBytes),
        }),
      );
    } catch (error) {
      logger.error('Erro ao executar manutenção do banco:', error);
      const message = t('dataManagement.maintenanceRunErrorWithReason', {
        defaultValue: 'Erro ao executar a manutenção do banco: {{reason}}',
        reason: getErrorReason(error) ?? t('dataManagement.maintenanceRunUnknownReason', 'erro desconhecido'),
      });
      setMaintenanceRunError(message);
      announce(message, 'assertive');
    } finally {
      setIsCompacting(false);
    }
  }, [announce, t]);

  const loadConversations = useCallback(async () => {
    setLoadingConversations(true);
    try {
      const conversations = await GetConversations() as ConversationRecord[];
      const ids = (conversations || [])
        .map((conversation) => String(conversation.id ?? '').trim())
        .filter((id) => id.length > 0);
      setConversationIds(ids);
    } catch (error) {
      logger.error('Erro ao carregar conversas para exportação:', error);
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

  useEffect(() => {
    if (location.pathname !== '/settings/data') {
      setIsLoadingMaintenance(false);
      return;
    }
    void loadMaintenance();
  }, [loadMaintenance, location.pathname]);

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

  const loadExportMemoryRecordIds = useCallback(async () => {
    const ids: string[] = [];
    let offset = 0;
    const limit = 250;
    for (;;) {
      const result = await ListMemoryRecords(memory.Filter.createFrom({
        includeArchived: true,
        limit,
        offset,
      }));
      const records = result.records || [];
      ids.push(...records.map((record) => String(record.id ?? '').trim()).filter((id) => id.length > 0));
      offset += records.length;
      if (records.length === 0 || offset >= (result.total || 0)) break;
    }
    setExportMemoryRecordIds(ids);
    return ids;
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

      let memoryRecordIdsToExport: string[] = [];
      let memoriesRequestedButEmpty = false;
      if (includeMemoriesExport) {
        memoryRecordIdsToExport = await loadExportMemoryRecordIds();
        if (memoryRecordIdsToExport.length === 0) {
          memoriesRequestedButEmpty = true;
        }
      }

      const idsToExport = includeConversations && !exportMcpExternalFormat ? conversationIds : [];
      const hasResourcesToExport =
        idsToExport.length > 0 ||
        providerIdsToExport.length > 0 ||
        mcpServerSlugsToExport.length > 0 ||
        taskListIdsToExport.length > 0 ||
        memoryRecordIdsToExport.length > 0 ||
        includeCredentialExport;
      if (!hasResourcesToExport) {
        announce(memoriesRequestedButEmpty
          ? t('history.exportMemoriesEmpty', 'Nenhuma memória encontrada para exportar')
          : t('history.noDataToExport', 'Nenhum dado selecionado para exportar'), 'assertive');
        return;
      }
      if (memoriesRequestedButEmpty) {
        announce(t('history.exportMemoriesSkippedEmpty', 'Nenhuma memória encontrada; exportando os demais dados selecionados.'), 'assertive');
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
      if (memoryRecordIdsToExport.length > 0) payload.memoryRecordIds = memoryRecordIdsToExport;
      if (payload.includeCredentials && exportPassword.trim()) {
        payload.credentialExportPassword = exportPassword.trim();
      }

      const jsonData = await ExportData(payload);
      downloadJSON(jsonData, generateFilename('dados'));
      announce(t('history.exportSuccess', 'Dados exportados com sucesso!'));
    } catch (error) {
      logger.error('Erro ao exportar dados:', error);
      announce(t('history.exportError', 'Erro ao exportar dados'), 'assertive');
    } finally {
      setIsExporting(false);
    }
  }, [announce, conversationIds, exportMcpExternalFormat, exportMcpServerSlugs, exportPassword, exportProviderIds, exportTaskListIds, includeConversations, includeCredentialExport, includeMcpServersExport, includeMemoriesExport, includeProvidersExport, includeTaskListsExport, loadExportMcpServerSlugs, loadExportMemoryRecordIds, loadExportProviderIds, loadExportTaskListIds, t]);

  const analyzeImportPayload = useCallback(async (jsonData: string, credentialPassword: string, requestKey: string) => {
    importAnalysisInFlightKeyRef.current = requestKey;
    setIsAnalyzingImport(true);
    try {
      const analysis = await AnalyzeImportData(jsonData, credentialPassword);
      const nextAnalysis = analysis as ImportAnalysis;
      if (activeImportAnalysisKeyRef.current === requestKey) {
        setImportAnalysis(nextAnalysis);
      }
      return nextAnalysis;
    } finally {
      if (importAnalysisInFlightKeyRef.current === requestKey) {
        importAnalysisInFlightKeyRef.current = null;
      }
      if (activeImportAnalysisKeyRef.current === requestKey) {
        setIsAnalyzingImport(false);
      }
    }
  }, []);

  const runLatestImportAnalysis = useCallback(async () => {
    if (importAnalysisInFlightRef.current) return;
    const queuedRequest = pendingImportAnalysisRef.current;
    if (!queuedRequest) return;

    if (
      lastAnalyzedImportRef.current === queuedRequest.key
      || importAnalysisInFlightKeyRef.current === queuedRequest.key
    ) {
      pendingImportAnalysisRef.current = null;
      return;
    }

    pendingImportAnalysisRef.current = null;
    importAnalysisInFlightRef.current = true;
    const queuedRequestKey = queuedRequest.key;
    activeImportAnalysisKeyRef.current = queuedRequestKey;
    try {
      await analyzeImportPayload(queuedRequest.jsonData, queuedRequest.password, queuedRequestKey);
      if (activeImportAnalysisKeyRef.current === queuedRequestKey) {
        lastAnalyzedImportRef.current = queuedRequestKey;
      }
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
    importAnalysisInFlightKeyRef.current = null;
    lastAnalyzedImportRef.current = null;
    activeImportAnalysisKeyRef.current = null;
    setIsAnalyzingImport(false);
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
    const requestKey = buildImportAnalysisKey(preview, '');
    activeImportAnalysisKeyRef.current = requestKey;
    await analyzeImportPayload(selectedFile.content, '', requestKey);
    if (activeImportAnalysisKeyRef.current === requestKey) {
      lastAnalyzedImportRef.current = requestKey;
    }
  }, [analyzeImportPayload]);

  const handleSelectImportFile = useCallback(async () => {
    try {
      await selectImportFile();
    } catch (error) {
      logger.error('Erro ao selecionar arquivo de importação:', error);
      if (!(error instanceof ImportFileError && error.code === IMPORT_FILE_ERROR_CODES.NO_FILE_SELECTED)) {
        resetImportState();
      }
      const message = getImportErrorMessage(error);
      if (message) announce(message, 'assertive');
    }
  }, [announce, getImportErrorMessage, resetImportState, selectImportFile]);

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
      const failureDetails = [
        result.failed > 0
          ? t('history.importFailedCount', { defaultValue: 'Falhas: {{count}}', count: result.failed })
          : '',
        result.skippedEmptyConversations > 0
          ? t('history.importSkippedEmptyCount', { defaultValue: 'Vazias descartadas: {{count}}', count: result.skippedEmptyConversations })
          : '',
        result.skippedConversationConflict > 0
          ? t('history.importSkippedConversationConflictCount', { defaultValue: 'Ignoradas por conflito de conversa: {{count}}', count: result.skippedConversationConflict })
          : '',
        result.skippedProviderConflict > 0
          ? t('history.importSkippedProviderConflictCount', { defaultValue: 'Ignoradas por conflito de provider: {{count}}', count: result.skippedProviderConflict })
          : '',
        result.skippedMcpServerConflict > 0
          ? t('history.importSkippedMcpServerConflictCount', { defaultValue: 'Ignoradas por conflito de servidor MCP: {{count}}', count: result.skippedMcpServerConflict })
          : '',
        result.skippedTaskListConflict > 0
          ? t('history.importSkippedTaskListConflictCount', { defaultValue: 'Ignoradas por conflito de tasklist: {{count}}', count: result.skippedTaskListConflict })
          : '',
        result.skippedCredentialConflict > 0
          ? t('history.importSkippedCredentialConflictCount', { defaultValue: 'Credenciais duplicadas ignoradas: {{count}}', count: result.skippedCredentialConflict })
          : '',
        result.skippedOther > 0
          ? t('history.importSkippedOtherCount', { defaultValue: 'Outros descartes: {{count}}', count: result.skippedOther })
          : '',
        ...(result.errors?.length
          ? [t('history.importErrorsLabel', 'Erros'), ...result.errors.map((error) => formatPortabilityMessage(error, t))]
          : []),
        ...(result.warnings?.length
          ? [t('history.importWarningsLabel', 'Avisos'), ...result.warnings.map((warning) => formatPortabilityMessage(warning, t))]
          : []),
      ].filter(Boolean);
      announce(
        [
          result.success
            ? t('history.importSuccess', 'Dados importados com sucesso!')
            : t('history.importPartial', 'Alguns recursos não puderam ser importados.'),
          t('history.importCounts', {
            defaultValue: 'Importados: {{imported}} | Ignorados: {{skipped}}',
            imported: result.imported,
            skipped: result.skipped,
          }),
          ...failureDetails,
        ].filter(Boolean).join('. '),
        result.success ? 'polite' : 'assertive',
      );
    } catch (error) {
      logger.error('Erro ao confirmar importação:', error);
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
    activeImportAnalysisKeyRef.current = nextRequest.key;
    if (lastAnalyzedImportRef.current === nextRequest.key) {
      setIsAnalyzingImport(false);
      return;
    }
    if (importAnalysisInFlightKeyRef.current === nextRequest.key) {
      return;
    }
    lastAnalyzedImportRef.current = null;
    setImportAnalysis(null);
    setIsAnalyzingImport(true);
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
  const exportConversationSummary = includeConversations && !exportMcpExternalFormat
    ? (loadingConversations ? t('common.loading', 'Carregando...') : conversationIds.length)
    : t('history.exportConversationsNotIncluded', 'Não incluir');

  const renderConflictGroup = useCallback((title: string, conflicts: ImportConflict[] | undefined) => {
    if (!conflicts?.length) return null;
    return (
      <>
        <strong>{title}</strong>
        <ul className="data-management__list">
          {conflicts.map((conflict) => {
            const reason = formatPortabilityMessage(conflict.reason, t);
            return (
              <li key={`${conflict.resourceType}:${conflict.identifier}`}>
                {conflict.resourceType === 'conversation'
                  ? reason
                  : <><code>{conflict.identifier}</code>: {reason}</>}
              </li>
            );
          })}
        </ul>
      </>
    );
  }, [t]);

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
          <div><dt>{t('history.exportConversationsLabel', 'Conversas')}</dt><dd>{exportConversationSummary}</dd></div>
          <div><dt>{t('history.exportProvidersLabel', 'Providers')}</dt><dd>{includeProvidersExport ? exportProviderIds.length : t('history.exportProvidersNotIncluded', 'Não incluir')}</dd></div>
          <div><dt>{t('history.exportMcpServersLabel', 'Servidores MCP')}</dt><dd>{includeMcpServersExport ? exportMcpServerSlugs.length : t('history.exportMcpServersNotIncluded', 'Não incluir')}</dd></div>
          <div><dt>{t('history.exportTaskListsLabel', 'Tasklists')}</dt><dd>{includeTaskListsExport ? exportTaskListIds.length : t('history.exportTaskListsNotIncluded', 'Não incluir')}</dd></div>
          <div><dt>{t('history.exportMemoriesLabel', 'Memórias')}</dt><dd>{includeMemoriesExport ? exportMemoryRecordIds.length : t('history.exportMemoriesNotIncluded', 'Não incluir')}</dd></div>
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
                logger.error('Erro ao carregar providers para exportação:', error);
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
                logger.error('Erro ao carregar servidores MCP para exportação:', error);
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
                  setIncludeMemoriesExport(false);
                  setExportMemoryRecordIds([]);
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
                logger.error('Erro ao carregar tasklists para exportação:', error);
                setIncludeTaskListsExport(false);
                setExportTaskListIds([]);
                announce(t('history.exportTaskListsLoadError', 'Erro ao carregar tasklists para exportação'), 'assertive');
              });
            }}
            label={t('history.exportTaskListsOption', 'Incluir tasklists persistidas no banco')}
          />
          <Checkbox
            checked={includeMemoriesExport}
            disabled={exportMcpExternalFormat}
            onChange={(event) => {
              const checked = event.target.checked;
              setIncludeMemoriesExport(checked);
              if (!checked) {
                setExportMemoryRecordIds([]);
                return;
              }
              void loadExportMemoryRecordIds().catch((error) => {
                logger.error('Erro ao carregar memórias para exportação:', error);
                setIncludeMemoriesExport(false);
                setExportMemoryRecordIds([]);
                announce(t('history.exportMemoriesLoadError', 'Erro ao carregar memórias para exportação'), 'assertive');
              });
            }}
            label={t('history.exportMemoriesOption', 'Incluir memórias persistidas no banco')}
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
            <div><dt>{t('history.importMemoriesLabel', 'Memórias')}</dt><dd>{importPreview.memoryRecordCount}</dd></div>
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
              <div>
                <strong>{t('history.importWarningsLabel', 'Avisos')}</strong>
                <ul className="data-management__list data-management__list--warning" aria-label={t('history.importWarningsLabel', 'Avisos')}>
                  {importAnalysis.warnings.map((warning, index) => (
                    <li key={portabilityMessageKey(warning, index)}>{formatPortabilityMessage(warning, t)}</li>
                  ))}
                </ul>
              </div>
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
                  {lastImportResult.errors.map((error, index) => (
                    <li key={portabilityMessageKey(error, index)}>{formatPortabilityMessage(error, t)}</li>
                  ))}
                </ul>
              </div>
            )}
            {!!lastImportResult.warnings?.length && (
              <div>
                <strong>{t('history.importWarningsLabel', 'Avisos')}</strong>
                <ul className="data-management__list data-management__list--warning" aria-label={t('history.importWarningsLabel', 'Avisos')}>
                  {lastImportResult.warnings.map((warning, index) => (
                    <li key={portabilityMessageKey(warning, index)}>{formatPortabilityMessage(warning, t)}</li>
                  ))}
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

      <section className="data-management-card" aria-labelledby="data-maintenance-title">
        <div className="data-management-card__header">
          <div>
            <h2 id="data-maintenance-title">{t('dataManagement.maintenanceTitle', 'Manutenção e retenção do banco')}</h2>
            <p>{t('dataManagement.maintenanceDescription', 'Controle por quanto tempo os dados de jobs ficam no banco e recupere espaço em disco. Tool calls de chat seguem o ciclo de vida da conversa.')}</p>
          </div>
        </div>

        {isLoadingMaintenance && !maintenance ? (
          <p className="data-management__note" aria-busy="true">{t('common.loading', 'Carregando...')}</p>
        ) : maintenance ? (
          <>
            <div className="data-management__maintenance-fields">
              <FormField
                label={t('dataManagement.jobRetentionHoursLabel', 'Retenção de dados de jobs (horas)')}
                description={t('dataManagement.jobRetentionHoursDescription', 'Runs e eventos de jobs mais antigos que isto são removidos. Padrão: 24h.')}
              >
                <Input
                  type="number"
                  min={1}
                  step={1}
                  value={String(maintenance.job_retention_hours)}
                  onChange={(event) => updateMaintenanceField('job_retention_hours', Number(event.target.value))}
                />
              </FormField>
              <FormField
                label={t('dataManagement.runsPerJobKeepLabel', 'Execuções mantidas por job')}
                description={t('dataManagement.runsPerJobKeepDescription', 'Teto de execuções recentes preservadas por job, além da janela por idade. 0 desativa o limite.')}
              >
                <Input
                  type="number"
                  min={0}
                  step={1}
                  value={String(maintenance.runs_per_job_keep)}
                  onChange={(event) => updateMaintenanceField('runs_per_job_keep', Number(event.target.value))}
                />
              </FormField>
              <FormField
                label={t('dataManagement.chatRetentionDaysLabel', 'Limite de idade de tool calls de chat (dias)')}
                description={t('dataManagement.chatRetentionDaysDescription', '0 = sem limite (seguem a conversa). Um valor maior que 0 remove tool calls de chat mais antigos que isso.')}
              >
                <Input
                  type="number"
                  min={0}
                  step={1}
                  value={String(maintenance.chat_tool_calls_retention_days)}
                  onChange={(event) => updateMaintenanceField('chat_tool_calls_retention_days', Number(event.target.value))}
                />
              </FormField>
              <FormField
                label={t('dataManagement.vacuumThresholdLabel', 'Limiar para compactação completa (MiB)')}
                description={t('dataManagement.vacuumThresholdDescription', 'Espaço livre acumulado no banco a partir do qual uma compactação completa (VACUUM) é disparada automaticamente.')}
              >
                <Input
                  type="number"
                  min={0}
                  step={1}
                  value={String(Math.floor(maintenance.vacuum_min_free_bytes / BYTES_PER_MIB))}
                  onChange={(event) =>
                    updateMaintenanceField(
                      'vacuum_min_free_bytes',
                      Math.floor(Number(event.target.value)) * BYTES_PER_MIB,
                    )
                  }
                />
              </FormField>
            </div>

            {dbStats && (
              <dl className="data-management__summary">
                <div><dt>{t('dataManagement.dbTotalSizeLabel', 'Tamanho total do banco')}</dt><dd>{formatBytes(dbStats.totalSizeBytes)}</dd></div>
                <div><dt>{t('dataManagement.dbFreeSpaceLabel', 'Espaço recuperável')}</dt><dd>{formatBytes(dbStats.freeBytes)}</dd></div>
                <div><dt>{t('dataManagement.dbAutoVacuumLabel', 'Modo de auto_vacuum')}</dt><dd>{dbStats.autoVacuumMode}</dd></div>
              </dl>
            )}
            {maintenanceRunError && (
              <p className="data-management__note">{maintenanceRunError}</p>
            )}

            <div className="data-management-card__actions data-management-card__actions--start">
              <Button
                type="button"
                variant="primary"
                onClick={() => void handleSaveMaintenance()}
                loading={isSavingMaintenance}
                disabled={isLoadingMaintenance || isCompacting}
              >
                {t('dataManagement.maintenanceSave', 'Salvar política')}
              </Button>
              <Button
                type="button"
                variant="secondary"
                onClick={() => void handleRunMaintenance()}
                loading={isCompacting}
                disabled={isLoadingMaintenance || isSavingMaintenance}
              >
                {t('dataManagement.maintenanceRunNow', 'Limpar agora')}
              </Button>
            </div>
          </>
        ) : (
          <p className="data-management__note">{t('dataManagement.maintenanceUnavailable', 'Não foi possível carregar as configurações de manutenção.')}</p>
        )}
      </section>
    </div>
  );
}
