/* eslint-disable @typescript-eslint/no-explicit-any */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { CompassOutlined, FileOutlined, MessageOutlined, PlusOutlined, SlidersOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Toolbar, ToolbarButton } from '../components/ui/Toolbar';
import { CodeEditor } from '../components/ui/CodeEditor';
import { MarkdownRenderer } from '../components/ui/MarkdownRenderer';
import { Menu, type MenuItem } from '../components/menu';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import { MermaidEditorModal } from '../components/editor/MermaidEditorModal';
import { RichTextEditor } from '../components/editor/RichTextEditor';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import { useRichEditorFlushEvents } from './useRichEditorFlushEvents';
import { useRegisterWorkspaceChatAdapter } from '../hooks/useRegisterWorkspaceChatAdapter';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import type {
  WorkspaceChatModalAdapter,
  WorkspaceChatModalPrepareResult,
  WorkspaceChatSendPlan,
  WorkspaceChatModalSession,
} from '../store/workspaceChatModalStore';
import { useEditorStore, DEFAULT_MD, type EditorMode, type EditorDocument, type EditorInsertRequest } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { createTwoFilesPatch } from 'diff';
import { applyTextReplacementByOffset } from '../lib/editorPatchApply';
import { normalizeEditorInsertContent } from '../lib/editorInsertNormalize';
import { applyRichTextInsert, applyRichTextInsertAtEnd } from '../lib/richTextPatchApply';
import { validateRichTextSelectionSnapshot } from '../lib/richTextSelectionValidation';
import { markdownToHtml } from '../lib/markdownToHtml';
import { computeMonacoInsertText } from '../lib/monacoInsertHeuristics';
import { buildChatSurfaceParams } from '../lib/chatSurface';
import { findMermaidFenceByIndex, removeMermaidFence, replaceMermaidFenceCode } from '../lib/mermaidFence';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import { useEditorInlineChatPatch } from '../hooks/useEditorInlineChatPatch';
import { isModalOpen } from '../components/ui/Modal';
import type { MediaFile } from '../services/mediaService';
import type { Message } from '../store/chatStore';
import {
  buildFileMenuItemsForContextMenu,
  buildFormatMenuItemsForContextMenu,
  buildInsertMenuItemsForContextMenu,
  buildModeMenuItemsForContextMenu,
} from './editorMenus';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  EditorDeleteDraft,
  EditorGetDraftPath,
  EditorGetFileInfo,
  EditorLoadState,
  EditorSaveState,
  GetProfile,
  EditorOpenFile,
  EditorReadDraft,
  EditorReadFile,
  EditorSaveFileDialog,
  EditorUnwatchFile,
  EditorWatchFile,
  EditorWriteDraft,
  EditorWriteFile,
} from '@wailsjs/go/app/App';
import './EditorPage.css';

export default function EditorPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const { waitForChatDone, waitForEditorPatch, getMaxNumericMessageId } = useEditorInlineChatPatch();


  const documents = useEditorStore((s) => s.documents);
  const activeDocumentId = useEditorStore((s) => s.activeDocumentId);
  const createDocument = useEditorStore((s) => s.createDocument);
  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const renameDocument = useEditorStore((s) => s.renameDocument);
  const setDocFilePath = useEditorStore((s) => s.setDocFilePath);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const hydrate = useEditorStore((s) => s.hydrate);
  const addWorkspaceTab = useWorkspaceStore((s) => s.addTab);
  const setActiveWsTab = useWorkspaceStore((s) => s.setActiveTab);
  const wsActiveTab = useWorkspaceStore((s) => s.getActiveTab());
  const wsTabs = useWorkspaceStore((s) => s.workspace?.tabs);
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);

  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const tabProfileSlug = wsActiveTab?.profileOverride?.slug as string | undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || 'editor-texto';

  const activeTab = useMemo(() => activeDocumentId ? documents[activeDocumentId] ?? null : null, [documents, activeDocumentId]);


  const pageRootRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<any>(null);
  const monacoRef = useRef<any>(null);
  const richEditorRef = useRef<any>(null);
  const richEditorHandleRef = useRef<RichTextEditorHandle | null>(null);

  const [isAsking, setIsAsking] = useState(false);

  const [activeMermaidIndex, setActiveMermaidIndex] = useState<number | null>(null);
  const [mermaidInitialCode, setMermaidInitialCode] = useState('');
  const [mermaidInsertText, setMermaidInsertText] = useState('');
  interface RichMermaidSession {
    mermaidBlockId: string;
    initialCode: string;
    insertText: string;
    apply: (nextCode: string) => void;
    remove: () => void;
  }

  const [richMermaidSession, setRichMermaidSession] = useState<RichMermaidSession | null>(null);

  // Foco previsível após fechar o modal Mermaid.
  const prevMermaidOpenRef = useRef(false);
  useEffect(() => {
    const isOpen = activeMermaidIndex !== null;
    if (prevMermaidOpenRef.current && !isOpen) {
      focusEditorSoon();
    }
    prevMermaidOpenRef.current = isOpen;
  }, [activeMermaidIndex]);

  type InlineChatSelection =
    | {
        mode: 'markdown';
        tabId: string;
        selectedText: string;
        /** True quando não há seleção (inserção no cursor) */
        selectionIsEmpty?: boolean;
        /** Contexto ao redor do cursor para orientar inserção */
        cursorContext?: string;
        /** Texto a exibir no painel "Contexto" do chat modal */
        displayText?: string;
        startOffset: number;
        endOffset: number;
        snapshot: string;
      }
    | {
        mode: 'rich';
        tabId: string;
        selectedText: string;
        /** Versão Markdown do trecho selecionado (melhor para prompt/preview) */
        selectedMarkdown?: string;
        selectionIsEmpty?: boolean;
        cursorContext?: string;
        displayText?: string;
        displayMarkdown?: string;
        from: number;
        to: number;
        snapshot: string;
      };

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  const getMaybeContent = (res: unknown) => {
    if (typeof res === 'string') return res;
    if (res && typeof res === 'object' && 'content' in res) {
      const value = (res as { content?: string }).content;
      return typeof value === 'string' ? value : String(value ?? '');
    }
    return '';
  };

  const inlineChatRunIdRef = useRef(0);
  const chatModalOpen = useWorkspaceChatModalStore((s) => s.isOpen);
  const prevChatModalOpenRef = useRef(false);

  const [sessionLoaded, setSessionLoaded] = useState(false);

  const [editorReadyNonce, setEditorReadyNonce] = useState(0);
  const [pendingInsert, setPendingInsert] = useState<EditorInsertRequest | null>(null);

  const fileModeByPathRef = useRef<Record<string, 'markdown' | 'rich'>>({});

  // Autosave robusto: mantém a última versão conhecida do markdown por aba e agenda persistência.
  // Isso reduz o risco de perder texto caso o estado do store não reflita exatamente o que está na UI.
  const latestMarkdownByTabRef = useRef<Record<string, string>>({});
  const autosaveTimersByTabRef = useRef<Record<string, number>>({});

  type DiskInfo = { exists: boolean; isDir: boolean; size: number; modTimeMs: number };
  const diskInfoByTabRef = useRef<Record<string, DiskInfo>>({});
  const diskContentHashByTabRef = useRef<Record<string, number>>({});
  const diskBaselineContentByTabRef = useRef<Record<string, string>>({});
  const externalConflictLockedByTabRef = useRef<Record<string, boolean>>({});
  const lastSelfWriteAtByPathRef = useRef<Record<string, number>>({});

  type MergeSession = {
    originalPath: string;
    mineDraftId: string;
    diskDraftId: string;
    conflictDraftId: string;
    createdAt: number;
  };

  const mergeSessionByTabRef = useRef<Record<string, MergeSession>>({});

  const getMergeSession = (tabId: string): MergeSession | null => {
    const id = String(tabId || '');
    if (!id) return null;
    return mergeSessionByTabRef.current[id] || null;
  };

  const markSelfWrite = (filePath: string) => {
    const key = normalizePathKey(String(filePath || ''));
    if (!key) return;
    lastSelfWriteAtByPathRef.current[key] = Date.now();
  };

  const isProbablySelfWrite = (filePath: string, withinMs = 900) => {
    const key = normalizePathKey(String(filePath || ''));
    if (!key) return false;
    const t = Number(lastSelfWriteAtByPathRef.current[key] || 0);
    if (!t) return false;
    return Date.now() - t < Math.max(0, withinMs);
  };

  const updateLatestMarkdownForTab = (tabId: string, markdown: string) => {
    latestMarkdownByTabRef.current[String(tabId || '')] = String(markdown ?? '');
  };

  const setExternalConflictLocked = (tabId: string, locked: boolean) => {
    externalConflictLockedByTabRef.current[String(tabId || '')] = !!locked;
  };

  const externalPromptInFlightByTabRef = useRef<Record<string, boolean>>({});

  const isExternalPromptInFlight = (tabId: string) => {
    return !!externalPromptInFlightByTabRef.current[String(tabId || '')];
  };

  const setExternalPromptInFlight = (tabId: string, inFlight: boolean) => {
    externalPromptInFlightByTabRef.current[String(tabId || '')] = !!inFlight;
  };

  const isExternalConflictLocked = (tabId: string) => {
    return !!externalConflictLockedByTabRef.current[String(tabId || '')];
  };

  const diskInfoEquals = (a?: DiskInfo | null, b?: DiskInfo | null) => {
    if (!a || !b) return false;
    return a.exists === b.exists && a.isDir === b.isDir && a.size === b.size && a.modTimeMs === b.modTimeMs;
  };

  const refreshDiskInfoForTab = async (tab: EditorDocument): Promise<DiskInfo | null> => {
    const filePath = tab?.filePath ? String(tab.filePath) : '';
    if (!filePath) return null;
    try {
      const info = (await EditorGetFileInfo(filePath)) as any;
      const di: DiskInfo = {
        exists: !!info?.exists,
        isDir: !!info?.isDir,
        size: Number(info?.size ?? 0),
        modTimeMs: Number(info?.modTimeMs ?? 0),
      };
      diskInfoByTabRef.current[String(tab.id)] = di;
      return di;
    } catch {
      return null;
    }
  };

  const getCachedMarkdownForTab = (tab: EditorDocument): string => {
    if (!tab) return '';
    return latestMarkdownByTabRef.current[tab.id] ?? String(tab.markdown ?? '');
  };

  const truncatePreview = (text: string, limit = 20000) => {
    const s = String(text ?? '');
    if (s.length <= limit) return { preview: s, truncated: false, total: s.length };
    return { preview: s.slice(0, Math.max(0, limit)) + `\n\n… (truncado; total: ${s.length} chars)`, truncated: true, total: s.length };
  };

  const hashStringFNV1a32 = (text: string) => {
    const s = String(text ?? '');
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0;
    }
    return h >>> 0;
  };

  const setDiskBaselineForTab = (tabId: string, content: string) => {
    diskContentHashByTabRef.current[String(tabId || '')] = hashStringFNV1a32(content);
    diskBaselineContentByTabRef.current[String(tabId || '')] = String(content ?? '');
  };

  const hasConflictMarkers = (text: string) => {
    const s = String(text ?? '');
    return /^<{7} /m.test(s) || /^={7}$/m.test(s) || /^>{7} /m.test(s);
  };

  const makeGitStyleConflictText = (diskContent: string, localContent: string, labels?: { disk?: string; local?: string }) => {
    const diskLabel = String(labels?.disk || 'disco');
    const localLabel = String(labels?.local || 'minha');
    return [
      `<<<<<<< ${diskLabel}`,
      String(diskContent ?? ''),
      `=======`,
      String(localContent ?? ''),
      `>>>>>>> ${localLabel}`,
      '',
    ].join('\n');
  };

  const safeDraftIdPart = (raw: string) => {
    return String(raw || '')
      .trim()
      .slice(0, 60)
      .replace(/[^a-zA-Z0-9_-]+/g, '_')
      .replace(/^_+/, '')
      .replace(/_+$/, '') || 'tab';
  };

  const startMergeSessionForTab = async (tabId: string, filePath: string, diskContent: string, localContent: string) => {
    const stamp = Date.now();
    const safeTab = safeDraftIdPart(tabId);
    const mineDraftId = `merge-mine-${safeTab}-${stamp}`;
    const diskDraftId = `merge-disk-${safeTab}-${stamp}`;
    const conflictDraftId = `merge-conflict-${safeTab}-${stamp}`;

    const conflictText = makeGitStyleConflictText(diskContent, localContent, { disk: 'disco', local: 'minha' });

    await EditorWriteDraft(mineDraftId, localContent);
    await EditorWriteDraft(diskDraftId, diskContent);
    await EditorWriteDraft(conflictDraftId, conflictText);

    mergeSessionByTabRef.current[String(tabId || '')] = {
      originalPath: String(filePath || ''),
      mineDraftId,
      diskDraftId,
      conflictDraftId,
      createdAt: stamp,
    };

    // Garante travamento mesmo se o fluxo tiver sido iniciado fora do questionário.
    setExternalConflictLocked(tabId, true);

    setDocMarkdown(tabId, conflictText);
    updateLatestMarkdownForTab(tabId, conflictText);
    setDocDirty(tabId, true);

    addToast(
      'Conflito aberto para mesclagem manual (estilo Git). Resolva os marcadores e use Salvar para gravar no arquivo real.',
      'warning'
    );
  };

  const cleanupMergeSessionForTab = async (tabId: string) => {
    const sess = getMergeSession(tabId);
    if (!sess) return;
    delete mergeSessionByTabRef.current[String(tabId || '')];
    const ids = [sess.mineDraftId, sess.diskDraftId, sess.conflictDraftId].filter(Boolean);
    await Promise.all(
      ids.map((id) =>
        EditorDeleteDraft(id).catch(() => null)
      )
    );
  };

  const buildUnifiedDiff = (diskContent: string, localContent: string) => {
    try {
      return createTwoFilesPatch('disco', 'minha-versao', String(diskContent ?? ''), String(localContent ?? ''), '', '', {
        context: 3,
      });
    } catch {
      return '';
    }
  };

  const promptResolveExternalChangeForTab = async (
    tabId: string,
    filePath: string,
    opts?: { diskContent?: string; diskReadError?: string }
  ) => {
    if (isExternalPromptInFlight(tabId)) return;

    const { documents: currentDocs } = useEditorStore.getState();
    const tab = currentDocs[tabId] || null;
    if (!tab || !tab.filePath) return;

    setExternalPromptInFlight(tabId, true);
    try {
      setExternalConflictLocked(tabId, true);
      setDocDirty(tabId, true);

      const localContent = getCachedMarkdownForTab(tab);
      const localPreview = truncatePreview(localContent);

      let diskContent = typeof opts?.diskContent === 'string' ? String(opts?.diskContent) : '';
      let diskReadError = typeof opts?.diskReadError === 'string' ? String(opts?.diskReadError) : '';

      if (!diskReadError && opts?.diskContent === undefined) {
        try {
          diskContent = String((await EditorReadFile(filePath)) || '');
        } catch (e: any) {
          diskReadError = String(e?.message || e || '').trim();
        }
      }

      // Se o conteúdo no disco é igual ao local, não há conflito real.
      if (!diskReadError && diskContent === localContent) {
        setDiskBaselineForTab(tabId, localContent);
        setDocDirty(tabId, false);
        const { documents: afterDocs } = useEditorStore.getState();
        const afterTab = afterDocs[tabId] || tab;
        void refreshDiskInfoForTab(afterTab);
        setExternalConflictLocked(tabId, false);
        return;
      }

      const diskPreview = diskReadError
        ? { preview: `Erro ao ler do disco:\n${diskReadError}`, truncated: false, total: 0 }
        : truncatePreview(diskContent);

      const diffText = diskReadError ? '' : buildUnifiedDiff(diskContent, localContent);
      const diffPreview = diffText ? truncatePreview(diffText, 30000) : { preview: '', truncated: false, total: 0 };

      const resp = await requestQuestionnaire({
        id: `ui-editor-external-change-${Date.now()}`,
        title: t('editor.questionnaire.externalChangeTitle'),
        description: t('editor.questionnaire.externalChangeDesc'),
        submitLabel: t('editor.buttons.apply'),
        cancelLabel: t('editor.buttons.notNow'),
        allowCancel: true,
        questions: [
          {
            id: 'path',
            type: 'readonly_code' as const,
            prompt: t('editor.prompts.file'),
            content: String(filePath || ''),
          },
          ...(diffPreview.preview
            ? [
                {
                  id: 'diff',
                  type: 'readonly_code' as const,
                  prompt: t('editor.prompts.diff'),
                  content: diffPreview.preview,
                },
              ]
            : []),
          {
            id: 'disk',
            type: 'readonly_code' as const,
            prompt: t('editor.prompts.diskPreview'),
            content: diskPreview.preview,
          },
          {
            id: 'local',
            type: 'readonly_code' as const,
            prompt: t('editor.prompts.localPreview'),
            content: localPreview.preview,
          },
          {
            id: 'choice',
            type: 'single_choice' as const,
            prompt: t('editor.prompts.action'),
            required: true,
            options: [
              t('editor.options.useDisk'),
              t('editor.options.resolveMerge'),
              t('editor.options.useMine'),
              t('editor.options.saveAs'),
            ],
            default: t('editor.options.useDisk'),
          },
        ],
      });

      if (resp.cancelled) {
        addToast(t('editor.toast.externalChange'), 'warning');
        return;
      }

      const choice = String(resp.answers?.choice || '').trim();

      if (choice === t('editor.options.resolveMerge')) {
        if (diskReadError) {
          addToast('Não foi possível ler do disco. Tente novamente.', 'error');
          return;
        }
        try {
          await startMergeSessionForTab(tabId, filePath, diskContent, localContent);
        } catch (e: any) {
          console.error('[EditorPage] startMergeSession error:', e);
          addToast(e?.message || 'Erro ao iniciar mesclagem', 'error');
        }
        return;
      }

      if (choice === t('editor.options.useDisk')) {
        if (diskReadError) {
          addToast('Não foi possível ler do disco. Tente novamente.', 'error');
          return;
        }

        try {
          setDocMarkdown(tabId, diskContent);
          updateLatestMarkdownForTab(tabId, diskContent);
          setDiskBaselineForTab(tabId, diskContent);
          setDocDirty(tabId, false);
          const { documents: afterDocs } = useEditorStore.getState();
          const afterTab = afterDocs[tabId] || tab;
          void refreshDiskInfoForTab(afterTab);
          setExternalConflictLocked(tabId, false);
          addToast('Recarregado do disco', 'success');
        } catch (e: any) {
          addToast(e?.message || 'Erro ao recarregar arquivo', 'error');
        }
        return;
      }

      if (choice.startsWith(t('editor.options.saveAs'))) {
        const suggested = basenameFromPath(filePath) || 'documento.md';
        const newPath = String(await EditorSaveFileDialog(suggested) || '').trim();
        if (!newPath) return;

        updateLatestMarkdownForTab(tabId, localContent);
        markSelfWrite(newPath);
        await EditorWriteFile(newPath, localContent);
        setDiskBaselineForTab(tabId, localContent);

        const title = basenameFromPath(newPath);
        setDocFilePath(tabId, newPath);
        renameDocument(tabId, title);
        setDocDraftId(tabId, null);
        setDocDirty(tabId, false);

        // filePath+title são sincronizados pelo useWorkspaceEditorBridge

        const { documents: afterDocs } = useEditorStore.getState();
        const afterTab = afterDocs[tabId] || tab;
        void refreshDiskInfoForTab(afterTab);
        setExternalConflictLocked(tabId, false);
        addToast('Salvo em novo arquivo', 'success');
        return;
      }

      // Usar minha versão (sobrescrever no disco)
      try {
        markSelfWrite(filePath);
        await EditorWriteFile(filePath, localContent);
        setDiskBaselineForTab(tabId, localContent);
        setDocDirty(tabId, false);
        const { documents: afterDocs } = useEditorStore.getState();
        const afterTab = afterDocs[tabId] || tab;
        void refreshDiskInfoForTab(afterTab);
        setExternalConflictLocked(tabId, false);
        addToast('Sobrescrito no disco', 'success');
      } catch (e: any) {
        addToast(e?.message || 'Erro ao sobrescrever no disco', 'error');
      }
    } finally {
      setExternalPromptInFlight(tabId, false);
    }
  };

  const persistTabContentNow = async (tabId: string) => {
    if (!sessionLoaded) return;
    const { documents: currentDocs } = useEditorStore.getState();
    const tab = currentDocs[tabId] || null;
    if (!tab) return;

    if (tab.mode === 'rich' && useEditorStore.getState().activeDocumentId === tabId) {
      flushActiveRichMarkdownNow();
    }

    const mergeSession = getMergeSession(tabId);
    if (mergeSession) {
      const markdown = getCachedMarkdownForTab(tab);
      updateLatestMarkdownForTab(tab.id, markdown);
      try {
        await EditorWriteDraft(mergeSession.conflictDraftId, markdown);
      } catch {
        // best-effort
      }
      return;
    }

    if (isExternalConflictLocked(tabId)) return;

    const markdown = getCachedMarkdownForTab(tab);
    updateLatestMarkdownForTab(tab.id, markdown);

    const filePath = tab.filePath ? String(tab.filePath) : '';
    const draftId = tab.draftId ? String(tab.draftId) : String(tab.id);

    try {
      if (!filePath) {
        if (!draftId) return;
        await EditorWriteDraft(draftId, markdown);
        return;
      }

      // Detecta mudança externa antes de escrever (evita sobrescrever sem avisar)
      try {
        const info = (await EditorGetFileInfo(filePath)) as any;
        const currentDisk: DiskInfo = {
          exists: !!info?.exists,
          isDir: !!info?.isDir,
          size: Number(info?.size ?? 0),
          modTimeMs: Number(info?.modTimeMs ?? 0),
        };
        const lastDisk = diskInfoByTabRef.current[String(tabId)];

        if (lastDisk && !diskInfoEquals(lastDisk, currentDisk)) {
          setExternalConflictLocked(tabId, true);
          setDocDirty(tabId, true);
          addToast('Arquivo foi modificado fora do Assistente. Escolha como resolver.', 'warning');
          if (!isExternalPromptInFlight(tabId)) {
            void promptResolveExternalChangeForTab(tabId, filePath);
          }
          return;
        }

        if (!lastDisk) diskInfoByTabRef.current[String(tabId)] = currentDisk;
      } catch {
        // best-effort
      }

      markSelfWrite(filePath);
      await EditorWriteFile(filePath, markdown);
      setDiskBaselineForTab(tab.id, markdown);
      setDocDirty(tab.id, false);

      // Atualiza baseline após salvar
      void refreshDiskInfoForTab(tab);
    } catch (e: any) {
      console.warn('[EditorPage] falha ao salvar:', e);
    }
  };

  const schedulePersistForTab = (tabId: string, delayMs = 650) => {
    if (!sessionLoaded) return;
    const id = String(tabId || '');
    if (!id) return;
    if (isExternalConflictLocked(id) && !getMergeSession(id)) return;
    const prev = autosaveTimersByTabRef.current[id];
    if (prev) window.clearTimeout(prev);
    autosaveTimersByTabRef.current[id] = window.setTimeout(() => {
      void persistTabContentNow(id);
    }, Math.max(0, delayMs));
  };

  // Salva o estado do editor (fileModeByPath + mergeSessionsByTabId) em disco
  const saveEditorState = () => {
    try {
      for (const t of useEditorStore.getState().documents ? Object.values(useEditorStore.getState().documents) : []) {
        if (t.filePath && (t.mode === 'markdown' || t.mode === 'rich')) {
          fileModeByPathRef.current[normalizePathKey(String(t.filePath))] = t.mode;
        }
      }
    } catch {
      // best-effort
    }
    EditorSaveState({
      fileModeByPath: fileModeByPathRef.current,
      mergeSessionsByTabId: mergeSessionByTabRef.current as any,
    } as any).catch((e: unknown) => {
      console.warn('[EditorPage] falha ao salvar estado:', e);
    });
  };

  // Restaura sessão (abas abertas) via workspace YAML + EditorLoadState (arquivo JSON)
  useEffect(() => {
    if (!isWsInitialized) return;
    let cancelled = false;

    (async () => {
      try {
        const wsState = useWorkspaceStore.getState();
        const wsEditorTabs = (wsState.workspace?.tabs || []).filter((t) => t.type === 'editor');
        const wsActiveTabId = wsState.workspace?.activeTabId || null;

        const editorState = await EditorLoadState();
        if (cancelled) return;

        // Preferências por arquivo
        try {
          const fromState = editorState?.fileModeByPath || {};
          const next: Record<string, 'markdown' | 'rich'> = {};
          for (const [k, v] of Object.entries(fromState)) {
            const key = normalizePathKey(String(k || ''));
            if (!key) continue;
            next[key] = v === 'rich' ? 'rich' : 'markdown';
          }
          fileModeByPathRef.current = next;
        } catch {
          // best-effort
        }

        const mergeFromState = ((editorState?.mergeSessionsByTabId as Record<string, any>) || {});

        const loadedTabs: EditorDocument[] = [];

        for (const tab of wsEditorTabs) {
          const tabId = tab.id;
          const filePath = String(tab.state?.filePath || '').trim();
          const draftId = String(tab.state?.draftId || '').trim();

          const mergeSessRaw = mergeFromState[tabId] as any;
          const hasMergeSess = !!mergeSessRaw && typeof mergeSessRaw === 'object' && String(mergeSessRaw?.conflictDraftId || '').trim();

          let markdown = '';
          try {
            if (filePath) {
              if (hasMergeSess) {
                const conflictDraftId = String(mergeSessRaw?.conflictDraftId || '').trim();
                const resDraft = await EditorReadDraft(conflictDraftId);
                markdown = getMaybeContent(resDraft);
              } else {
                const res = await EditorReadFile(filePath);
                markdown = getMaybeContent(res);
              }
            }
          } catch {
            markdown = '';
          }

          const pathKey = filePath ? normalizePathKey(filePath) : '';
          const mode: EditorMode = pathKey
            ? (fileModeByPathRef.current[pathKey] || 'markdown')
            : 'markdown';
          const title = filePath ? basenameFromPath(filePath) : (tab.title || 'Novo documento');

          loadedTabs.push({
            id: tabId,
            title,
            markdown: markdown || DEFAULT_MD,
            mode,
            filePath: filePath || null,
            draftId: filePath ? null : (draftId || null),
            isDirty: !!hasMergeSess,
          });
        }

        // Popula o cache do autosave com o conteúdo carregado.
        try {
          for (const t of loadedTabs) {
            updateLatestMarkdownForTab(t.id, String(t.markdown ?? ''));
          }
        } catch {
          // best-effort
        }

        // Baseline do disco para arquivos reais (best-effort)
        try {
          for (const t of loadedTabs) {
            if (t.filePath) {
              setDiskBaselineForTab(t.id, String(t.markdown ?? ''));
              void refreshDiskInfoForTab(t);
            }
          }
        } catch {
          // best-effort
        }

        const loadedDocs: Record<string, EditorDocument> = {};
        for (const t of loadedTabs) {
          loadedDocs[t.id] = t;
        }

        // Aba ativa: preferir a aba ativa do workspace se for editor, senão a primeira
        const activeEditorId = loadedDocs[wsActiveTabId || '']
          ? wsActiveTabId!
          : (loadedTabs[0]?.id ?? null);

        hydrate({
          documents: loadedDocs,
          activeDocumentId: activeEditorId,
        });

        // Restaura merge sessions em refs antes de liberar autosave.
        try {
          for (const t of loadedTabs) {
            if (!t?.id) continue;
            const raw = mergeFromState[t.id] as any;
            if (raw && typeof raw === 'object') {
              const conflictDraftId = String(raw?.conflictDraftId || '').trim();
              const mineDraftId = String(raw?.mineDraftId || '').trim();
              const diskDraftId = String(raw?.diskDraftId || '').trim();
              const originalPath = String(raw?.originalPath || t.filePath || '').trim();
              if (conflictDraftId && mineDraftId && diskDraftId && originalPath) {
                mergeSessionByTabRef.current[String(t.id)] = {
                  originalPath,
                  mineDraftId,
                  diskDraftId,
                  conflictDraftId,
                  createdAt: Number(raw?.createdAt || Date.now()),
                };
                setExternalConflictLocked(t.id, true);
              }
            }
          }
        } catch {
          // best-effort
        }

        setSessionLoaded(true);
      } catch {
        setSessionLoaded(true);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isWsInitialized]);

  const debouncedMarkdownForPreview = useDebouncedValue(activeTab?.markdown || '', 120);

  // Mantém cache do markdown atual da aba ativa para flush/abas.
  useEffect(() => {
    if (!sessionLoaded) return;
    if (!activeTab) return;
    updateLatestMarkdownForTab(activeTab.id, String(activeTab.markdown ?? ''));
  }, [sessionLoaded, activeTab?.id]);

  const allDocs = useMemo(() => Object.values(documents), [documents]);

  // Persiste o estado do editor (fileModeByPath + mergeSessionsByTabId)
  useEffect(() => {
    if (!sessionLoaded) return;

    const timer = window.setTimeout(() => {
      saveEditorState();
    }, 500);

    return () => window.clearTimeout(timer);
  }, [sessionLoaded, allDocs, activeDocumentId]);

  // Flush imediato ao fechar/minimizar para reduzir chance de perder o estado
  useEffect(() => {
    if (!sessionLoaded) return;

    const persistNow = () => {
      try {
        const { activeDocumentId: currentActive } = useEditorStore.getState();
        if (currentActive) {
          void persistTabContentNow(currentActive);
        }
      } catch {
        // best-effort
      }
      saveEditorState();
    };

    const onBeforeUnload = () => persistNow();
    const onPageHide = () => persistNow();
    const checkActiveFileExternalChange = async () => {
      const { documents: currentDocs, activeDocumentId: currentActiveDocId } = useEditorStore.getState();
      const tab = currentActiveDocId ? (currentDocs[currentActiveDocId] || null) : null;
      if (!tab?.filePath) return;
      if (isExternalConflictLocked(tab.id)) return;

      const lastDisk = diskInfoByTabRef.current[String(tab.id)];
      const currentDisk = await refreshDiskInfoForTab(tab);
      if (!currentDisk) return;

      if (lastDisk && !diskInfoEquals(lastDisk, currentDisk)) {
        setExternalConflictLocked(tab.id, true);
        setDocDirty(tab.id, true);
        addToast('Arquivo foi modificado fora do Assistente. Escolha como resolver.', 'warning');
        void promptResolveExternalChangeForTab(tab.id, String(tab.filePath));
      }
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === 'hidden') {
        persistNow();
        return;
      }
      if (document.visibilityState === 'visible') {
        void checkActiveFileExternalChange();
      }
    };

    const onFocus = () => {
      void checkActiveFileExternalChange();
    };

    window.addEventListener('beforeunload', onBeforeUnload);
    window.addEventListener('pagehide', onPageHide);
    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('focus', onFocus);

    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload);
      window.removeEventListener('pagehide', onPageHide);
      document.removeEventListener('visibilitychange', onVisibilityChange);
      window.removeEventListener('focus', onFocus);
    };
  }, [sessionLoaded, allDocs, activeDocumentId]);

  // Watcher de mudanças externas (backend emite editor:fileChanged)
  const watchedFilesRef = useRef<Record<string, { path: string; count: number }>>({});

  useEffect(() => {
    if (!sessionLoaded) return;

    const next: Record<string, { path: string; count: number }> = {};
    for (const t of allDocs) {
      if (!t.filePath) continue;
      const p = String(t.filePath || '').trim();
      const key = normalizePathKey(p);
      if (!key) continue;
      if (!next[key]) next[key] = { path: p, count: 0 };
      next[key].count += 1;
    }

    const prev = watchedFilesRef.current;

    for (const [key, entry] of Object.entries(prev)) {
      const prevCount = entry.count;
      const nextCount = next[key]?.count ?? 0;
      const diff = prevCount - nextCount;
      if (diff <= 0) continue;
      for (let i = 0; i < diff; i++) {
        EditorUnwatchFile(entry.path).catch(() => null);
      }
    }

    for (const [key, entry] of Object.entries(next)) {
      const prevCount = prev[key]?.count ?? 0;
      const diff = entry.count - prevCount;
      if (diff <= 0) continue;
      for (let i = 0; i < diff; i++) {
        EditorWatchFile(entry.path).catch(() => null);
      }
    }

    watchedFilesRef.current = next;
  }, [sessionLoaded, allDocs]);

  useEffect(() => {
    return () => {
      const prev = watchedFilesRef.current;
      watchedFilesRef.current = {};
      for (const entry of Object.values(prev)) {
        for (let i = 0; i < entry.count; i++) {
          EditorUnwatchFile(entry.path).catch(() => null);
        }
      }
    };
  }, []);

  useEffect(() => {
    if (!sessionLoaded) return;

    const unsub = EventsOn('editor:fileChanged', async (data: any) => {
      const changedPath = String(data?.path || data?.filePath || '').trim();
      if (!changedPath) return;

      if (isProbablySelfWrite(changedPath)) {
        return;
      }

      const key = normalizePathKey(changedPath);
      if (!key) return;

      const { documents: currentDocs } = useEditorStore.getState();
      const affected = Object.values(currentDocs).filter((t) => t.filePath && normalizePathKey(String(t.filePath)) === key);
      if (affected.length === 0) return;
      let diskContent = '';
      let diskReadError = '';
      try {
        diskContent = String((await EditorReadFile(changedPath)) || '');
      } catch (e: any) {
        diskReadError = String(e?.message || e || '').trim();
      }

      const diskHash = !diskReadError ? hashStringFNV1a32(diskContent) : 0;

      for (const t of affected) {
        if (!t.filePath) continue;
        if (isExternalConflictLocked(t.id)) continue;

        // Se conseguimos ler o disco, podemos decidir se há conflito real.
        if (!diskReadError) {
          const localContent = getCachedMarkdownForTab(t);
          const localHash = hashStringFNV1a32(localContent);
          const lastDiskHash = Number(diskContentHashByTabRef.current[String(t.id)] || 0);

          // Caso comum: ferramenta externa salvou sem mudar o conteúdo (touch/reformat idêntico)
          if (lastDiskHash && lastDiskHash === diskHash) {
            void refreshDiskInfoForTab(t);
            continue;
          }

          // Caso comum: o arquivo no disco já está igual ao que temos localmente
          if (diskHash === localHash) {
            setDiskBaselineForTab(t.id, localContent);
            setDocDirty(t.id, false);
            void refreshDiskInfoForTab(t);
            // Não abre prompt.
            continue;
          }

          // Aba limpa: recarrega automaticamente, mas só se realmente mudou
          if (!t.isDirty) {
            try {
              setDocMarkdown(t.id, diskContent);
              updateLatestMarkdownForTab(t.id, diskContent);
              setDiskBaselineForTab(t.id, diskContent);
              setDocDirty(t.id, false);
              void refreshDiskInfoForTab(t);
              if (t.id === activeDocumentId) addToast('Arquivo recarregado do disco (mudança externa)', 'info');
            } catch {
              // Se não der pra aplicar automaticamente, cai pro fluxo existente
              setExternalConflictLocked(t.id, true);
              setDocDirty(t.id, true);
              if (!isExternalPromptInFlight(t.id)) {
                void promptResolveExternalChangeForTab(t.id, String(t.filePath), { diskContent, diskReadError });
              }
            }
            continue;
          }
        }
        // Aba dirty (ou falha ao ler o disco): pede decisão explícita
        setExternalConflictLocked(t.id, true);
        setDocDirty(t.id, true);
        if (!isExternalPromptInFlight(t.id)) {
          void promptResolveExternalChangeForTab(t.id, String(t.filePath), { diskContent, diskReadError });
        }
      }
    });

    return () => {
      try {
        unsub();
      } catch {
        // ignore
      }
    };
  }, [sessionLoaded, activeDocumentId]);

  // Ao entrar no Editor (e ao trocar de aba/modo), foca automaticamente a área de texto.
  // Não rouba foco de modais nem de campos de digitação.
  const didInitialEditorAutofocusRef = useRef(false);
  useEffect(() => {
    if (!sessionLoaded) return;
    if (!activeTab) return;
    if (chatModalOpen) return;
    if (isModalOpen()) return;

    const el = document.activeElement as HTMLElement | null;
    const tag = el?.tagName || '';
    const isTypingTarget =
      !!el &&
      (tag === 'INPUT' ||
        tag === 'TEXTAREA' ||
        el.isContentEditable ||
        el.getAttribute?.('role') === 'textbox');

    // Primeira entrada: sempre tenta focar o editor.
    if (!didInitialEditorAutofocusRef.current) {
      didInitialEditorAutofocusRef.current = true;
      focusEditorSoon();
      return;
    }

    // Mudança de aba/modo: só foca automaticamente se não houver um alvo de foco claro.
    // (Evita “puxar” o foco de tabs/toolbar, o que quebra navegação por teclado, ex: F6.)
    const isEditorZone =
      !!el &&
      (!!el.closest?.('.rich-text-editor__content') || !!el.closest?.('.monaco-editor'));
    const isDocumentBody = !el || el === document.body;

    if (!isTypingTarget && (isDocumentBody || isEditorZone)) {
      focusEditorSoon();
    }
  }, [sessionLoaded, activeTab?.id, activeTab?.mode, chatModalOpen]);


  const prevDocsRef = useRef<Record<string, EditorDocument>>({});
  useEffect(() => {
    if (!sessionLoaded) return;

    const prev = prevDocsRef.current;
    prevDocsRef.current = documents;

    const removedIds = Object.keys(prev).filter((id) => !documents[id]);
    for (const id of removedIds) {
      const was = prev[id];
      if (!was) continue;
      if (was.filePath) continue;
      const draftId = was.draftId || was.id;
      EditorDeleteDraft(draftId).catch(() => null);
    }
  }, [sessionLoaded, documents]);

  const getSelectionSnapshot = () => {
    const editor = editorRef.current;
    const monaco = monacoRef.current;
    if (!editor || !monaco) return null;

    const model = editor.getModel();
    if (!model) return null;

    const selection = editor.getSelection();
    const position = editor.getPosition();
    if (!selection || !position) return null;

    const start = selection.getStartPosition();
    const end = selection.getEndPosition();
    const selectedText = model.getValueInRange(selection);

    const startOffset = model.getOffsetAt(start);
    const endOffset = model.getOffsetAt(end);

    const full = model.getValue();
    const cursorOffset = model.getOffsetAt(position);
    const windowSize = 260;
    const before = full.slice(Math.max(0, cursorOffset - windowSize), cursorOffset);
    const after = full.slice(cursorOffset, Math.min(full.length, cursorOffset + windowSize));
    const cursorContext = (before + '⟂' + after).trimEnd();

    const selectionIsEmpty = !selectedText;
    const displayText = selectionIsEmpty ? cursorContext : selectedText;

    return {
      selectedText,
      selectionIsEmpty,
      cursorContext,
      displayText,
      startOffset,
      endOffset,
    };
  };

  const getRichSelectionSnapshot = () => {
    const editor = richEditorRef.current;
    if (!editor) return null;

    const sel = editor.state?.selection;
    if (!sel) return null;

    const { from, to, empty } = sel;
    const selectedText = editor.state.doc.textBetween(from, to, '\n');

    const serializer = (editor.storage as any)?.markdown?.serializer;
    const serializeNodeToMarkdown = (node: any): string => {
      try {
        if (serializer?.serialize) return String(serializer.serialize(node) ?? '');
      } catch {
        // best-effort
      }
      return '';
    };

    const getMarkdownForRange = (fromPos: number, toPos: number) => {
      try {
        const doc = editor.state?.doc;
        if (!doc) return '';
        // `cut` retorna um Node do tipo doc com o conteúdo do range.
        const cut = doc.cut(Math.max(0, fromPos), Math.max(0, toPos));
        return serializeNodeToMarkdown(cut);
      } catch {
        return '';
      }
    };

    let selectedMarkdown = '';
    if (!empty && to > from) {
      selectedMarkdown = getMarkdownForRange(from, to);
    }

    // Contexto ao redor do cursor (para inserção quando empty=true)
    let cursorContext = '';
    try {
      const docSize = editor.state.doc.content.size;
      const windowSize = 260;
      const winFrom = Math.max(0, from - windowSize);
      const winTo = Math.min(docSize, from + windowSize);
      const before = editor.state.doc.textBetween(winFrom, from, '\n');
      const after = editor.state.doc.textBetween(from, winTo, '\n');
      cursorContext = (before + '⟂' + after).trimEnd();
    } catch {
      cursorContext = '';
    }

    const selectionIsEmpty = !!empty || !selectedText;

    // Quando não há seleção, usa o bloco atual como “contexto” em Markdown.
    let displayMarkdown = selectedMarkdown;
    if (selectionIsEmpty) {
      try {
        const $from = (sel as any).$from;
        if ($from) {
          let depth = $from.depth;
          while (depth > 0 && !$from.node(depth)?.isBlock) depth -= 1;
          if (depth > 0) {
            const nodeStart = $from.before(depth);
            const nodeSize = $from.node(depth)?.nodeSize ?? 0;
            const nodeEnd = nodeStart + nodeSize;
            if (nodeSize > 0) {
              displayMarkdown = getMarkdownForRange(nodeStart, nodeEnd);
            }
          }
        }
      } catch {
        // best-effort
      }
    }

    const displayText = selectionIsEmpty ? (cursorContext || '(cursor)') : selectedText;
    const displayForContextPanel = displayMarkdown || (selectionIsEmpty ? (cursorContext || '(cursor)') : selectedText);

    // Snapshot do documento (para debug/consistência): prefere o Markdown atual do TipTap.
    let snapshot = '';
    try {
      snapshot = String((editor.storage as any)?.markdown?.getMarkdown?.() ?? '');
    } catch {
      snapshot = '';
    }

    return {
      selectedText,
      selectedMarkdown: selectedMarkdown || undefined,
      selectionIsEmpty,
      cursorContext,
      displayText,
      displayMarkdown: displayForContextPanel || undefined,
      from,
      to,
      snapshot,
    };
  };

  function focusEditorSoon() {
    window.setTimeout(() => {
      try {
        if (!activeTab) return;
        if (activeTab.mode === 'markdown') {
          editorRef.current?.focus?.();
        } else if (activeTab.mode === 'rich') {
          richEditorRef.current?.commands?.focus?.();
          richEditorRef.current?.view?.focus?.();
        }
      } catch {
        // best-effort
      }
    }, 20);
  }

  useEffect(() => {
    if (prevChatModalOpenRef.current && !chatModalOpen) {
      inlineChatRunIdRef.current += 1;
      setIsAsking(false);
      focusEditorSoon();
    }
    prevChatModalOpenRef.current = chatModalOpen;
  }, [chatModalOpen, activeTab]);

  const flushActiveRichMarkdownNow = useCallback(() => {
    try {
      const st = useEditorStore.getState();
      const tab = st.activeDocumentId ? st.documents[st.activeDocumentId] ?? null : null;
      if (!tab || tab.mode !== 'rich') return;
      richEditorHandleRef.current?.flushMarkdown?.();
    } catch {
      // best-effort
    }
  }, []);

  useRichEditorFlushEvents({ flushNow: flushActiveRichMarkdownNow });

  const applyInsertRequest = async (req: EditorInsertRequest): Promise<boolean> => {
    const r = req;
    const rawContent = String(r?.content ?? '');
    if (!rawContent) return true;

    const requestedDocumentId = String(r.targetDocumentId || '').trim();
    if (r.target === 'document' && !requestedDocumentId) {
      console.error('[EditorPage] applyInsertRequest rejected: document target requires targetDocumentId');
      return false;
    }
    const currentEditorState = useEditorStore.getState();
    let targetTab = requestedDocumentId
      ? currentEditorState.documents[requestedDocumentId] ?? null
      : activeTab;

    if (requestedDocumentId && currentEditorState.activeDocumentId !== requestedDocumentId) {
      return false;
    }

    if (r.target === 'new_document' || !targetTab) {
      if (requestedDocumentId) return false;
      const title = String(r.title || t('editor.fallback.fromChat'));
      const draftId = (typeof crypto !== 'undefined' && crypto.randomUUID) ? crypto.randomUUID() : `editor-${Date.now()}`;
      const draftPath = String(await EditorGetDraftPath(draftId) ?? '');
      const tabId = await addWorkspaceTab('editor', title, { filePath: draftPath, draftId });
      useEditorStore.getState().createDocument({ id: tabId, title, markdown: '', mode: 'markdown', filePath: draftPath, draftId });
      targetTab = useEditorStore.getState().documents[tabId] ?? null;
      await new Promise((res) => setTimeout(res, 0));
    }

    if (!targetTab) return false;

    const normalized = normalizeEditorInsertContent({
      content: rawContent,
      format: r.format,
      targetMode: targetTab.mode,
    });

    const content = normalized.content;
    const format = normalized.format;

    const focusAfter = r.focus !== false;

    if (targetTab.mode === 'markdown') {
      const editor = editorRef.current;
      const monaco = monacoRef.current;
      const model = editor?.getModel?.();
      const selection = editor?.getSelection?.();

      if (editor && monaco && model && selection) {
        const hasFocus = !!editor.hasTextFocus?.();
        const isEmptySel = !!selection.isEmpty?.();
        const selStart = selection.getStartPosition();
        const currentText = model.getValue?.() ?? '';
        const { useSelection, textToInsert } = computeMonacoInsertText({
          hasFocus,
          selectionIsEmpty: isEmptySel,
          selectionStart: { lineNumber: selStart.lineNumber, column: selStart.column },
          currentText,
          content,
        });

        // Se não há foco e a seleção é vazia (comum após navegação),
        // inserir no fim do documento é mais previsível do que no início.
        const endOffset = model.getValueLength?.() ?? currentText.length;
        const endPos = model.getPositionAt(endOffset);

        const insertRange = useSelection
          ? selection
          : (typeof (monaco as any).Range === 'function'
              ? new (monaco as any).Range(endPos.lineNumber, endPos.column, endPos.lineNumber, endPos.column)
              : {
                  startLineNumber: endPos.lineNumber,
                  startColumn: endPos.column,
                  endLineNumber: endPos.lineNumber,
                  endColumn: endPos.column,
                });

        const startPos = useSelection ? selStart : endPos;
        const startOffset = model.getOffsetAt(startPos);

        editor.executeEdits('chat-to-editor-insert', [
          {
            range: insertRange as any,
            text: textToInsert,
            forceMoveMarkers: true,
          },
        ]);

        const nextOffset = startOffset + textToInsert.length;
        const nextPos = model.getPositionAt(nextOffset);
        editor.setPosition(nextPos);
        editor.revealPositionInCenter(nextPos);
        if (focusAfter) editor.focus();
        return true;
      }

      // Fallback: se o Monaco ainda não montou, aplica no markdown da aba (no final) e tenta focar depois.
      const current = String(targetTab.markdown ?? '');
      const nextText = current ? current + '\n\n' + content : content;
      setDocMarkdown(targetTab.id, nextText);
      updateLatestMarkdownForTab(targetTab.id, nextText);
      schedulePersistForTab(targetTab.id);
      if (focusAfter) focusEditorSoon();
      return true;
    }

    // Rich: insere no cursor/seleção atual.
    const rich = richEditorRef.current;
    if (!rich) return false;
    const sel = rich.state?.selection;
    if (!sel) return false;

    const richHasFocus = !!((rich as any)?.view?.hasFocus?.() ?? (rich as any)?.isFocused);

    const from = Number(sel.from);
    const to = Number(sel.to);

    let contentToInsert: any = content;
    if (format === 'markdown') {
      contentToInsert = markdownToHtml(content);
    } else if (format === 'plain') {
      // Inserção como texto puro (sem interpretar como HTML).
      // Para manter comportamento previsível, tratamos como texto.
      contentToInsert = { type: 'text', text: content };
    }

    // Se não há foco (comum após navegar do Chat), a seleção pode estar no início.
    // Para um comportamento mais previsível, inserimos no fim do documento.
    if (!richHasFocus) {
      applyRichTextInsertAtEnd({ rich, contentToInsert });
    } else {
      applyRichTextInsert({ rich, from, to, contentToInsert });
    }
    flushActiveRichMarkdownNow();
    if (focusAfter) {
      try {
        rich.commands?.focus?.();
        rich.view?.focus?.();
      } catch {
        // best-effort
      }
    }
    return true;
  };

  // Consome requisições vindas do Chat → Editor (aba atual ou nova)
  useEffect(() => {
    if (!sessionLoaded) return;
    if (pendingInsert) return;
    const req = useEditorStore.getState().consumePendingInsert();
    if (req) setPendingInsert(req);
  }, [sessionLoaded, pendingInsert]);

  // Tenta aplicar quando o editor (Monaco/TipTap) estiver pronto.
  useEffect(() => {
    if (!pendingInsert) return;

    let cancelled = false;
    (async () => {
      // Inserções direcionadas podem precisar esperar a aba/documento terminar de sincronizar.
      const targetedInsert = !!String(pendingInsert.targetDocumentId || '').trim();
      const maxAttempts = targetedInsert ? 40 : 10;
      const delayMs = targetedInsert ? 100 : 60;
      for (let i = 0; i < maxAttempts; i += 1) {
        if (cancelled) return;
        const ok = await applyInsertRequest(pendingInsert);
        if (ok) {
          setPendingInsert(null);
          return;
        }
        await new Promise((r) => setTimeout(r, delayMs));
      }

      // Se falhar, mantém pendente mas avisa.
      addToast(t('editor.chatModal.insertExhausted'), 'error');
      setPendingInsert(null);
    })();

    return () => {
      cancelled = true;
    };
  }, [pendingInsert, editorReadyNonce]);

  type EditorPatch = {
    replacement?: string;
    format?: string;
    notes?: string;
  };

  const sendEditorChatModalMessage = async (
    instruction: string,
    mediaFiles: MediaFile[] | undefined,
    inlineChatSelection: InlineChatSelection,
    session?: WorkspaceChatModalSession,
  ): Promise<WorkspaceChatSendPlan> => {
    if (!activeTab) return null;

    // Não bloquear pelo `isLoading` global: o chat modal partilha o chatStore e um estado
    // preso (ou outro painel) desativava o input sem feedback claro; `sendMessage` já
    // trata pedidos em paralelo / substitui listeners.

    const expectedConversationId = session?.conversationId ?? useChatStore.getState().activeConversationId ?? undefined;

    const beforeMessages = useChatStore.getState().getMessages();
    const afterMessageId = getMaxNumericMessageId(beforeMessages as Message[]);

    const trimmed = String(instruction || '').trim();
    if (!trimmed) return null;

    const prompt = trimmed;
    const editorSurfaceTab = wsActiveTab ?? {
      type: 'editor',
      title: activeTab.title,
      state: {
        filePath: activeTab.filePath ?? undefined,
        draftId: activeTab.draftId ?? undefined,
      },
    };
    const surfaceContext = inlineChatSelection.mode === 'rich'
      ? {
          mode: 'rich',
          selectedText: inlineChatSelection.selectedText,
          selectedMarkdown: inlineChatSelection.selectedMarkdown,
          selectionIsEmpty: inlineChatSelection.selectionIsEmpty,
          cursorContext: inlineChatSelection.cursorContext,
          from: inlineChatSelection.from,
          to: inlineChatSelection.to,
        }
      : {
          mode: 'markdown',
          selectedText: inlineChatSelection.selectedText,
          selectionIsEmpty: inlineChatSelection.selectionIsEmpty,
          cursorContext: inlineChatSelection.cursorContext,
          startOffset: inlineChatSelection.startOffset,
          endOffset: inlineChatSelection.endOffset,
        };

    const runId = (inlineChatRunIdRef.current += 1);
    useWorkspaceChatModalStore.getState().setAdapterError(null);

    const isToolCallingEnabledForProfileSlug = async (slug: string): Promise<boolean> => {
      const s = String(slug || '').trim();
      if (!s) return true;
      try {
        const prof = await GetProfile(s);
        const disabled = !!(prof as { chat?: { disable_tools?: boolean } })?.chat?.disable_tools;
        return !disabled;
      } catch {
        // Best-effort: se não conseguimos ler o perfil, assume tools on.
        return true;
      }
    };

    const normalizeReplacementForEditor = (raw: string, patchFormat: string | undefined, selectedText: string) => {
      const text = String(raw ?? '');
      const sel = String(selectedText ?? '');

      // Alguns modelos colocam o conteúdo dentro de um bloco ```markdown ... ```.
      // Para o editor, isso costuma ser ruído (a não ser que o usuário já tenha selecionado um bloco fence).
      const looksLikeUserSelectedFence = /^\s*```/m.test(sel);

      const fence = text.match(/^\s*```\s*([a-z0-9_-]+)?\s*\r?\n([\s\S]*?)\r?\n```\s*$/i);
      if (!fence) return text;

      if (looksLikeUserSelectedFence) return text;

      const lang = String(fence[1] || '').trim().toLowerCase();
      const unwrapped = String(fence[2] || '');

      // Só unwrap para fences de markdown/texto (evita remover mermaid, etc.).
      const unwrapLangs = new Set(['markdown', 'md', 'text', 'plain', 'txt']);
      if (lang && unwrapLangs.has(lang)) return unwrapped;

      // Para patches plain, fences são quase sempre acidentais.
      if (patchFormat === 'plain' && (lang === '' || unwrapLangs.has(lang))) return unwrapped;

      return text;
    };

    const applyInlinePatchNow = (selection: InlineChatSelection, patch: EditorPatch) => {
      const replacement = normalizeReplacementForEditor(String(patch?.replacement || ''), patch?.format, selection?.selectedText);
      const { documents: currentDocs, activeDocumentId: currentActiveDocId } = useEditorStore.getState();
      const tab = currentDocs[selection.tabId] || null;
      if (!tab) {
        addToast('Aba do editor não encontrada para aplicar a alteração.', 'error');
        setIsAsking(false);
        focusEditorSoon();
        return;
      }

      if (selection.mode === 'markdown') {
        const s = selection;

        if (currentActiveDocId !== s.tabId) {
          addToast(t('editor.chatModal.openOriginalTabToApply'), 'info');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const model = editorRef.current?.getModel?.();
        const current = model?.getValue?.() ?? String(tab.markdown ?? '');

        const applied = applyTextReplacementByOffset({
          current,
          startOffset: s.startOffset,
          endOffset: s.endOffset,
          expectedSelectedText: s.selectedText,
          replacement,
        });

        // Se o conteúdo mudou desde o snapshot, evita aplicar offsets errados.
        if (!applied.ok) {
          addToast(t('editor.chatModal.selectionChangedRetry'), 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const nextMarkdown = applied.nextText;
        setDocMarkdown(s.tabId, nextMarkdown);
        updateLatestMarkdownForTab(s.tabId, nextMarkdown);
        schedulePersistForTab(s.tabId);
        addToast(t('editor.chatModal.patchApplied'), 'success');

        requestAnimationFrame(() => {
          try {
            const editor = editorRef.current;
            const m = editor?.getModel?.();
            if (!editor || !m) return;
            if (currentActiveDocId !== s.tabId) return;
            const startPos = m.getPositionAt(s.startOffset);
            const endPos = m.getPositionAt(s.startOffset + replacement.length);
            editor.setSelection({
              startLineNumber: startPos.lineNumber,
              startColumn: startPos.column,
              endLineNumber: endPos.lineNumber,
              endColumn: endPos.column,
            });
            editor.focus();
          } catch {
            // best-effort
          }
        });
      } else {
        const s = selection;
        if (currentActiveDocId !== s.tabId) {
          addToast(t('editor.chatModal.openOriginalTabToApply'), 'info');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }
        const rich = richEditorRef.current;
        if (!rich) {
          addToast(t('editor.chatModal.richEditorNotReady'), 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        // Evita aplicar em um range errado caso a seleção tenha mudado enquanto o chat estava aberto.
        try {
          const currentSel = rich.state?.selection;
          const expectedEmpty = !!s.selectionIsEmpty;
          const expectedFrom = Number(s.from);
          const expectedTo = Number(s.to);
          const expectedSelectedText = String(s.selectedText || '');

          const validation = validateRichTextSelectionSnapshot({
            currentSelection: currentSel
              ? { from: Number(currentSel.from), to: Number(currentSel.to), empty: !!currentSel.empty }
              : null,
            expectedFrom,
            expectedTo,
            expectedEmpty,
            expectedSelectedText,
            getCurrentSelectedText: expectedEmpty
              ? undefined
              : () => String(rich.state?.doc?.textBetween?.(currentSel!.from, currentSel!.to, '\n') ?? ''),
          });

          if (!validation.ok) {
            if (validation.reason === 'no_selection') {
              addToast(t('editor.chatModal.richSelectionReadFailed'), 'error');
            } else if (validation.reason === 'selected_text_mismatch') {
              addToast(t('editor.chatModal.selectionChangedRetry'), 'error');
            } else if (validation.reason === 'cannot_read_selected_text') {
              addToast(t('editor.chatModal.richSelectionValidateFailed'), 'error');
            } else {
              addToast(t('editor.chatModal.selectionSnapshotChanged'), 'error');
            }
            setIsAsking(false);
            focusEditorSoon();
            return;
          }
        } catch {
          addToast(t('editor.chatModal.richSelectionValidateFailed'), 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const isMarkdown = patch?.format === 'markdown';
        const contentToInsert = !isMarkdown ? replacement : markdownToHtml(replacement);
        applyRichTextInsert({ rich, from: s.from, to: s.to, contentToInsert });
        addToast(t('editor.chatModal.patchApplied'), 'success');
        flushActiveRichMarkdownNow();
      }

      useWorkspaceChatModalStore.getState().setAdapterError(null);
      useWorkspaceChatModalStore.getState().close();
      setIsAsking(false);
      focusEditorSoon();
    };

    const confirmInlinePatch = async (selection: InlineChatSelection, patch: EditorPatch) => {
      const before =
        selection.mode === 'rich'
          ? String(selection.selectedMarkdown || selection.selectedText || '')
          : String(selection.selectedText || '');
      const after = normalizeReplacementForEditor(String(patch?.replacement || ''), patch?.format, selection?.selectedText);
      const notes = String(patch?.notes || '').trim();

      const normalizedPatch = { ...patch, replacement: after };

      const resp = await requestQuestionnaire({
        id: `ui-editor-inline-patch-confirm-${Date.now()}`,
        title: 'Aplicar alteração?',
        description: notes || 'Revise o antes/depois. Se estiver ok, confirme para aplicar no trecho selecionado.',
        submitLabel: 'Sim, aplicar',
        cancelLabel: 'Não, cancelar',
        allowCancel: true,
        questions: [
          {
            id: 'before',
            type: 'readonly_code',
            prompt: 'Antes',
            content: before,
          },
          {
            id: 'after',
            type: 'readonly_code',
            prompt: 'Depois',
            content: after,
          },
        ],
      });

      if (!resp.cancelled) {
        applyInlinePatchNow(selection, normalizedPatch);
        return;
      }

      addToast(t('editor.chatModal.patchRejected'), 'info');
      setIsAsking(false);
      // Mantém o chat modal aberto para você criticar/explicar detalhes.
      // Apenas devolve o foco para o input do chat.
      useWorkspaceChatModalStore.getState().bumpFocus();
    };

    try {
      setIsAsking(true);

      // Regra importante:
      // - tools ON  => edit_file com confirmação contextual (Go-side); frontend fecha o chat modal
      // - tools OFF => body-only (extrai ```editor_patch``` do texto e confirma aqui)
      const toolCallingEnabled = await isToolCallingEnabledForProfileSlug(effectiveProfileSlug);

      // Drafts sem filePath não conseguem usar edit_file; nesse caso, cai para o
      // mesmo fluxo principal com fallback body-only e aplicação local do patch.
      const canUseToolCalling = toolCallingEnabled && !!activeTab?.filePath;
      const surfaceParams = buildChatSurfaceParams(editorSurfaceTab, {
        profileSlug: effectiveProfileSlug,
        context: surfaceContext,
      });

      const donePromise = waitForChatDone(expectedConversationId);
      return {
        content: prompt,
        mediaFiles,
        paramsOverride: surfaceParams,
        afterSend: async () => {
          try {
            await donePromise;

            if (runId !== inlineChatRunIdRef.current) return;

            // Tool calling: edit_file já fez tudo (questionnaire + escrita no disco).
            // O fsnotify detecta a mudança e recarrega o arquivo automaticamente.
            if (canUseToolCalling) {
              useWorkspaceChatModalStore.getState().setAdapterError(null);
              useWorkspaceChatModalStore.getState().close();
              setIsAsking(false);
              focusEditorSoon();
              return;
            }

            // Fallback (sem tool calling): extrai patch do corpo da resposta e confirma.
            const extracted = await waitForEditorPatch({
              afterMessageId,
              timeoutMs: 8000,
            });
            if (!extracted.ok) {
              const errText = String(extracted.error || '').trim();
              if (/nenhum patch encontrado|não contém patch|patch vazio|json inválido|patch inválido|muito grande/i.test(errText)) {
                addToast(t('editor.chatModal.patchNotApplicable'), 'error');
              }
              useWorkspaceChatModalStore.getState().setAdapterError(errText || t('editor.chatModal.patchExtractDefault'));
              setIsAsking(false);
              return;
            }

            await confirmInlinePatch(inlineChatSelection, extracted.patch as EditorPatch);
          } catch (e: unknown) {
            console.error('[EditorPage] inline chat error:', e);
            useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
            setIsAsking(false);
          }
        },
        onSendError: (e: unknown) => {
          console.error('[EditorPage] inline chat error:', e);
          useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
          setIsAsking(false);
        },
      };
    } catch (e: unknown) {
      console.error('[EditorPage] inline chat error:', e);
      useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
      setIsAsking(false);
      return null;
    }
  };

  const sendEditorChatModalRef = useRef(sendEditorChatModalMessage);
  sendEditorChatModalRef.current = sendEditorChatModalMessage;

  const editorChatModalAdapter = useMemo((): WorkspaceChatModalAdapter | null => {
    if (!wsActiveTab || wsActiveTab.type !== 'editor') return null;

    return {
      prepare: async (): Promise<WorkspaceChatModalPrepareResult> => {
        if (!activeTab) return { ok: false, message: t('workspace.chatModal.panelLoading') };
        if (activeTab.mode === 'view') {
          addToast(t('editor.chatModal.prepareNeedCodeOrRich'), 'info');
          return { ok: false };
        }
        if (isAsking) {
          return { ok: false, message: t('workspace.chatModal.panelLoading') };
        }

        const selectionRaw =
          activeTab.mode === 'markdown'
            ? getSelectionSnapshot()
            : activeTab.mode === 'rich'
              ? getRichSelectionSnapshot()
              : null;

        if (!selectionRaw) {
          addToast(t('editor.chatModal.prepareSelectionFailed'), 'error');
          return { ok: false };
        }

        if (selectionRaw.selectedText.length > 20000) {
          addToast(t('editor.chatModal.prepareSelectionTooLarge', { max: 20000 }), 'error');
          return { ok: false };
        }

        const snapshot =
          activeTab.mode === 'markdown'
            ? (editorRef.current?.getModel?.()?.getValue?.() ?? activeTab.markdown)
            : (selectionRaw as any)?.snapshot ?? activeTab.markdown;
        const selection: InlineChatSelection =
          activeTab.mode === 'markdown'
            ? {
                mode: 'markdown',
                tabId: activeTab.id,
                selectedText: selectionRaw.selectedText,
                selectionIsEmpty: !!(selectionRaw as any).selectionIsEmpty,
                cursorContext: (selectionRaw as any).cursorContext,
                displayText: (selectionRaw as any).displayText,
                startOffset: (selectionRaw as any).startOffset,
                endOffset: (selectionRaw as any).endOffset,
                snapshot,
              }
            : {
                mode: 'rich',
                tabId: activeTab.id,
                selectedText: selectionRaw.selectedText,
                selectedMarkdown: (selectionRaw as any)?.selectedMarkdown,
                selectionIsEmpty: !!(selectionRaw as any).selectionIsEmpty,
                cursorContext: (selectionRaw as any).cursorContext,
                displayText: (selectionRaw as any).displayText,
                displayMarkdown: (selectionRaw as any)?.displayMarkdown,
                from: (selectionRaw as any).from,
                to: (selectionRaw as any).to,
                snapshot,
              };

        const contextDisplay =
          selection.displayText ||
          (selection.mode === 'rich' ? selection.selectedMarkdown : '') ||
          selection.selectedText ||
          '';

        useWorkspaceChatModalStore.getState().setAdapterError(null);
        return { ok: true, contextDisplay, meta: selection };
      },
      send: (instruction, media, meta, session) =>
        sendEditorChatModalRef.current(instruction, media, meta as InlineChatSelection, session),
    };
  }, [wsActiveTab, activeTab, isAsking, addToast, editorReadyNonce, t]);

  useRegisterWorkspaceChatAdapter(wsActiveTab?.id, editorChatModalAdapter);

  const openFile = async () => {
    try {
      const res = await EditorOpenFile();
      const path = String(res?.path || '').trim();
      if (!path) return;

      const key = normalizePathKey(path);
      const content = String(res?.content || '');

      // Se o arquivo já está aberto em outra aba, apenas ativa essa aba.
      const existingDoc = Object.values(documents).find(
        (t) => t.filePath && normalizePathKey(String(t.filePath)) === key,
      );
      if (existingDoc) {
        const wsTab = (wsTabs || []).find(
          (t) => t.type === 'editor' && t.id === existingDoc.id,
        );
        if (wsTab) {
          await setActiveWsTab(wsTab.id);
          addToast('Arquivo já aberto — aba ativada', 'info');
          focusEditorSoon();
          return;
        }
      }

      const preferredMode: EditorMode =
        fileModeByPathRef.current[key] || (existingDoc?.mode === 'rich' ? 'rich' : 'markdown');
      const title = basenameFromPath(path);

      // Se a aba atual está "virgem" (sem arquivo, conteúdo padrão), reutiliza-a.
      const isPristine = activeTab && !activeTab.filePath && !activeTab.isDirty && activeTab.markdown === DEFAULT_MD;
      let id: string;

      if (isPristine) {
        id = activeTab.id;
        renameDocument(id, title);
        setDocMarkdown(id, content);
        useEditorStore.getState().setDocMode(id, preferredMode);
        // filePath+title são sincronizados pelo useWorkspaceEditorBridge
      } else {
        const tabId = await addWorkspaceTab('editor', title, { filePath: path });
        id = tabId;
        createDocument({ id: tabId, title, markdown: content, mode: preferredMode, filePath: path });
      }

      setDocFilePath(id, path);
      setDocDraftId(id, null);
      setDocDirty(id, false);

      updateLatestMarkdownForTab(id, content);
      setDiskBaselineForTab(id, content);
      const diskTab: EditorDocument = {
        id,
        title,
        markdown: content,
        mode: preferredMode,
        filePath: path,
      };
      void refreshDiskInfoForTab(diskTab);

      fileModeByPathRef.current[key] = preferredMode === 'rich' ? 'rich' : 'markdown';

      EditorDeleteDraft(id).catch(() => null);
      addToast('Arquivo aberto', 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      console.error('[EditorPage] openFile error:', e);
      addToast(getErrorMessage(e) || 'Erro ao abrir arquivo', 'error');
    }
  };

  const abortMerge = async () => {
    if (!activeTab?.filePath) return;

    const sess = getMergeSession(activeTab.id);
    if (!sess) return;

    let mineContent = '';
    try {
      const res = await EditorReadDraft(sess.mineDraftId);
      mineContent = getMaybeContent(res);
    } catch {
      mineContent = '';
    }

    const minePreview = truncatePreview(mineContent);

    const resp = await requestQuestionnaire({
      id: `ui-editor-abort-merge-${Date.now()}`,
      title: 'Abortar merge (estilo Git)?',
      description:
        'Isso vai descartar o texto com marcadores de conflito nesta aba e restaurar a sua versão original. O arquivo continuará com salvamento travado até você escolher como resolver a modificação externa.',
      submitLabel: 'Abortar merge',
      cancelLabel: 'Continuar editando',
      allowCancel: true,
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: 'Arquivo',
          content: String(activeTab.filePath || ''),
        },
        {
          id: 'mine',
          type: 'readonly_code' as const,
          prompt: 'Sua versão original (preview)',
          content: minePreview.preview || '(vazio)',
        },
      ],
    });

    if (resp.cancelled) return;

    // Mantém travado: evita autosave sobrescrever o arquivo real sem decisão explícita.
    setExternalConflictLocked(activeTab.id, true);

    setDocMarkdown(activeTab.id, mineContent);
    updateLatestMarkdownForTab(activeTab.id, mineContent);
    setDocDirty(activeTab.id, true);

    await cleanupMergeSessionForTab(activeTab.id);

    addToast('Merge abortado. Sua versão foi restaurada. Use Salvar para resolver a modificação externa.', 'info');
    focusEditorSoon();
  };

  const saveFile = async () => {
    if (!activeTab) return;
    try {
      if (activeTab.mode === 'rich') flushActiveRichMarkdownNow();
      const content = getCachedMarkdownForTab(activeTab);
      updateLatestMarkdownForTab(activeTab.id, content);

      if (activeTab.filePath) {
        if (isExternalConflictLocked(activeTab.id)) {
          const mergeSession = getMergeSession(activeTab.id);
          if (mergeSession) {
            if (hasConflictMarkers(content)) {
              addToast('Ainda existem marcadores de conflito (<<<<<<< / >>>>>>>). Resolva antes de salvar no arquivo real.', 'warning');
              return;
            }
            markSelfWrite(activeTab.filePath);
            await EditorWriteFile(activeTab.filePath, content);
            setDiskBaselineForTab(activeTab.id, content);
            setDocDirty(activeTab.id, false);
            void refreshDiskInfoForTab(activeTab);
            setExternalConflictLocked(activeTab.id, false);
            await cleanupMergeSessionForTab(activeTab.id);
            addToast('Conflito resolvido e salvo no disco', 'success');
            focusEditorSoon();
            return;
          }

          addToast('Salvamento travado: resolva a modificação externa primeiro.', 'warning');
          void promptResolveExternalChangeForTab(activeTab.id, String(activeTab.filePath));
          return;
        }
        markSelfWrite(activeTab.filePath);
        await EditorWriteFile(activeTab.filePath, content);
        setDiskBaselineForTab(activeTab.id, content);
        setDocDirty(activeTab.id, false);
        void refreshDiskInfoForTab(activeTab);
        addToast('Arquivo salvo', 'success');
        focusEditorSoon();
        return;
      }

      // Ainda não tem destino: pedir path
      const suggested = (activeTab.title || 'documento') + '.md';
      const path = String(await EditorSaveFileDialog(suggested) || '').trim();
      if (!path) return;

      markSelfWrite(path);
      await EditorWriteFile(path, content);
      setDiskBaselineForTab(activeTab.id, content);
      const title = basenameFromPath(path);
      setDocFilePath(activeTab.id, path);
      renameDocument(activeTab.id, title);
      setDocDirty(activeTab.id, false);

      // filePath+title são sincronizados pelo useWorkspaceEditorBridge

      void refreshDiskInfoForTab({ ...activeTab, filePath: path });

      const draftId = activeTab.draftId || activeTab.id;
      setDocDraftId(activeTab.id, null);
      await EditorDeleteDraft(draftId);

      addToast('Arquivo salvo', 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      console.error('[EditorPage] saveFile error:', e);
      addToast(getErrorMessage(e) || 'Erro ao salvar', 'error');
    }
  };

  const saveFileAsCopy = async () => {
    if (!activeTab?.filePath) return;
    try {
      if (activeTab.mode === 'rich') flushActiveRichMarkdownNow();
      const suggested = basenameFromPath(activeTab.filePath);
      const path = String(await EditorSaveFileDialog(suggested) || '').trim();
      if (!path) return;
      const content = getCachedMarkdownForTab(activeTab);
      updateLatestMarkdownForTab(activeTab.id, content);
      markSelfWrite(path);
      await EditorWriteFile(path, content);
      addToast('Cópia salva', 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      console.error('[EditorPage] saveAs error:', e);
      addToast(getErrorMessage(e) || 'Erro ao salvar como', 'error');
    }
  };

  const openMermaidEditorByIndex = (index: number, opts?: { insertText?: string }) => {
    if (!activeTab) return;
    const fence = findMermaidFenceByIndex(activeTab.markdown, index);
    if (!fence) {
      addToast(t('editor.chatModal.mermaidBlockNotFound'), 'error');
      return;
    }
    setActiveMermaidIndex(index);
    setMermaidInitialCode(fence.code);
    setMermaidInsertText(opts?.insertText ? String(opts.insertText) : '');
  };

  const applyMermaidCode = (code: string) => {
    if (!activeTab) return;
    if (activeMermaidIndex === null) return;
    const fence = findMermaidFenceByIndex(activeTab.markdown, activeMermaidIndex);
    if (!fence) {
      addToast('O bloco Mermaid não foi encontrado (talvez o documento tenha mudado).', 'error');
      return;
    }
    const nextMarkdown = replaceMermaidFenceCode(activeTab.markdown, fence, code);
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    addToast('Bloco Mermaid atualizado', 'success');
    setActiveMermaidIndex(null);
  };

  const removeMermaidBlockByIndex = async (index: number, reopenOnCancel?: { code: string }) => {
    if (!activeTab) return;

    const confirm = await requestQuestionnaire({
      id: `ui-editor-mermaid-remove-${Date.now()}`,
      title: 'Remover diagrama Mermaid',
      description: 'Tem certeza que deseja remover este bloco Mermaid do documento?',
      submitLabel: 'Remover',
      cancelLabel: 'Cancelar',
      allowCancel: true,
      questions: [
        {
          id: 'note',
          type: 'readonly_code',
          prompt: 'Dica',
          content: 'Essa ação remove o bloco ```mermaid``` inteiro.',
        },
      ],
    });

    if (confirm.cancelled) {
      if (reopenOnCancel) {
        setActiveMermaidIndex(index);
        setMermaidInitialCode(reopenOnCancel.code);
      }
      return;
    }

    const fence = findMermaidFenceByIndex(activeTab.markdown, index);
    if (!fence) {
      addToast('O bloco Mermaid não foi encontrado (talvez o documento tenha mudado).', 'error');
      return;
    }

    const nextMarkdown = removeMermaidFence(activeTab.markdown, fence);
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    addToast('Bloco Mermaid removido', 'success');
  };

  // Limpa timers de autosave ao desmontar
  useEffect(() => {
    return () => {
      const timers = autosaveTimersByTabRef.current;
      for (const k of Object.keys(timers)) {
        try {
          window.clearTimeout(timers[k]);
        } catch {
          // best-effort
        }
      }
      autosaveTimersByTabRef.current = {};
    };
  }, []);

  const fileMenuItems = useMemo(() => {
    const canSave = !!activeTab && (!activeTab.filePath || isExternalConflictLocked(activeTab.id));
    const canSaveAs = !!activeTab?.filePath;
    const hasMergeSession = !!activeTab && !!getMergeSession(activeTab.id);

    const items = [
      { value: 'new', label: 'Novo', sublabel: 'Ctrl+N' },
      { value: 'open', label: 'Abrir', sublabel: 'Ctrl+O' },
      { value: 'save', label: 'Salvar', sublabel: 'Ctrl+S', disabled: !canSave },
      ...(hasMergeSession
        ? [{ value: 'abort-merge', label: 'Abortar merge (Git)', sublabel: 'Descarta marcadores de conflito' }]
        : []),
      { value: 'saveas', label: 'Salvar como...', sublabel: 'Ctrl+Shift+S', disabled: !canSaveAs },
    ];

    return items;
  }, [activeTab]);

  const onFileMenuSelect = useCallback(
    async (value: string) => {
      const v = String(value || '').trim();
      if (!v) return;

      switch (v) {
        case 'new': {
          const draftId = (typeof crypto !== 'undefined' && crypto.randomUUID) ? crypto.randomUUID() : `editor-${Date.now()}`;
          const draftPath = String(await EditorGetDraftPath(draftId) ?? '');
          const tabId = await addWorkspaceTab('editor', 'Novo documento', { filePath: draftPath, draftId });
          createDocument({ id: tabId, draftId, filePath: draftPath });
          focusEditorSoon();
          return;
        }
        case 'open':
          await openFile();
          return;
        case 'save':
          await saveFile();
          return;
        case 'abort-merge':
          await abortMerge();
          return;
        case 'saveas':
          await saveFileAsCopy();
          return;
        default:
          return;
      }
    },
    [createDocument, addWorkspaceTab, openFile, saveFile, abortMerge, saveFileAsCopy, activeTab]
  );

  const {
    menu: toolbarMenu,
    openForTrigger: openToolbarMenu,
    closeMenu: closeToolbarMenu,
    onSelectItem: handleToolbarMenuSelect,
  } = useAnchoredContextMenu();

  const setActiveTabMode = useCallback(
    (nextMode: EditorMode) => {
      if (!activeTab) return;

      if (activeTab.mode === 'rich' && nextMode !== 'rich') {
        flushActiveRichMarkdownNow();
      }

      useEditorStore.getState().setDocMode(activeTab.id, nextMode);

      // Se for arquivo real, memoriza preferência apenas de modos de edição.
      if (activeTab.filePath && (nextMode === 'markdown' || nextMode === 'rich')) {
        fileModeByPathRef.current[normalizePathKey(String(activeTab.filePath))] = nextMode;
      }

      focusEditorSoon();
    },
    [activeTab]
  );

  const fileMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildFileMenuItemsForContextMenu({
      ctx: {
        fileMenuItems,
        onSelect: onFileMenuSelect,
      },
    });
  }, [fileMenuItems, onFileMenuSelect]);

  const insertMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildInsertMenuItemsForContextMenu({
      ctx: {
        activeTab,
        isAsking,
        editorReadyNonce,
        richEditorRef,
        applyInsertRequest,
        focusEditorSoon,
        addToast,
      },
    });
  }, [activeTab, isAsking, editorReadyNonce, addToast, applyInsertRequest]);

  const formatMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildFormatMenuItemsForContextMenu({
      ctx: {
        activeTab,
        isAsking,
        editorReadyNonce,
        richEditorRef,
        richEditorHandleRef,
      },
    });
  }, [activeTab, isAsking, editorReadyNonce]);

  const modeMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildModeMenuItemsForContextMenu({
      ctx: {
        activeTab,
        isAsking,
        setActiveTabMode,
      },
    });
  }, [activeTab, isAsking, setActiveTabMode]);

  const actions = useMemo(() => {
    return [
      {
        key: 'ask',
        label: 'Perguntar ao chat',
        icon: <MessageOutlined />,
        shortcut: 'Ctrl+Shift+I',
        onClick: async () => {
          if (isAsking) return;
          if (activeTab?.mode === 'view') {
            addToast(t('editor.chatModal.prepareNeedCodeOrRich'), 'info');
            return;
          }
          await useWorkspaceChatModalStore.getState().requestOpen();
        },
        disabled: !activeTab || isAsking,
      },
    ];
  }, [activeTab, isAsking, addToast, t]);

  // Atalhos de arquivos
  useEffect(() => {
    const onKeyDown = async (e: KeyboardEvent) => {
      if (!activeTab) return;
      if (isModalOpen()) return;

      if (e.ctrlKey && !e.shiftKey && (e.key === 's' || e.key === 'S') && !e.altKey) {
        e.preventDefault();
        await saveFile();
        return;
      }

      if (e.ctrlKey && e.shiftKey && (e.key === 's' || e.key === 'S') && !e.altKey) {
        e.preventDefault();
        await saveFileAsCopy();
        return;
      }

      if (e.ctrlKey && !e.shiftKey && (e.key === 'o' || e.key === 'O') && !e.altKey) {
        e.preventDefault();
        await openFile();
        return;
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [activeTab]);

  return (
    <div className="editor-page" ref={pageRootRef}>
      <Toolbar
        className="editor-page__toolbar ws-content-toolbar"
        left={<div className="editor-page__title">{activeTab?.title || t('editor.fallback.title')}</div>}
        right={
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <ToolbarButton
              label={t('editor.buttons.file')}
              icon={<FileOutlined />}
              onClick={(e) => openToolbarMenu(e.currentTarget, 'Menu Arquivo', fileMenuItemsForContextMenu)}
              aria-haspopup="menu"
            />

            <ToolbarButton
              label={t('editor.buttons.format')}
              icon={<SlidersOutlined />}
              disabled={!activeTab || isAsking || activeTab.mode !== 'rich' || !richEditorRef.current}
              onClick={(e) => openToolbarMenu(e.currentTarget, 'Menu Formatar', formatMenuItemsForContextMenu)}
              aria-haspopup="menu"
            />

            <ToolbarButton
              label={t('editor.buttons.insert')}
              icon={<PlusOutlined />}
              disabled={!activeTab || isAsking || activeTab.mode === 'view'}
              onClick={(e) => openToolbarMenu(e.currentTarget, 'Menu Inserir', insertMenuItemsForContextMenu)}
              aria-haspopup="menu"
            />

            <ToolbarButton
              label={t('editor.buttons.mode')}
              icon={<CompassOutlined />}
              disabled={!activeTab || isAsking}
              onClick={(e) => openToolbarMenu(e.currentTarget, 'Menu Modo', modeMenuItemsForContextMenu)}
              aria-haspopup="menu"
            />

          </div>
        }
        actions={actions}
        ariaLabel={t('editor.aria.toolbar')}
      />

      <div className="editor-page__content ws-content-area">
        {!activeTab ? (
          <div className="editor-page__empty">{t('editor.empty.noTabs')}</div>
        ) : activeTab.mode === 'markdown' ? (
          <div className={'editor-page__single'}>
            <div className="editor-page__pane" role="region" aria-label="Editor Markdown">
              <div className="editor-page__pane-title">{t('editor.panes.markdown')}</div>
              <div className="editor-page__pane-body">
                <CodeEditor
                  height="100%"
                  language="markdown"
                  ariaLabel={t('editor.aria.markdownEditor')}
                  value={activeTab.markdown}
                  pasteUrlAsMarkdownLink={true}
                  onChange={(v) => {
                    setDocMarkdown(activeTab.id, v);
                    updateLatestMarkdownForTab(activeTab.id, v);
                    schedulePersistForTab(activeTab.id);
                  }}
                  placeholder={t('editor.placeholders.markdown')}
                  readOnly={isAsking}
                  onMount={(editor, monaco) => {
                    editorRef.current = editor;
                    monacoRef.current = monaco;
                    setEditorReadyNonce((n) => n + 1);
                  }}
                />
              </div>
            </div>
          </div>
        ) : activeTab.mode === 'view' ? (
          <div className="editor-page__single">
            <div
              className="editor-page__pane"
              role="region"
              aria-label={t('editor.aria.preview')}
              onDoubleClick={(e) => {
                const target = e.target as HTMLElement | null;
                const wrapper = target?.closest?.('.mermaid-diagram') as HTMLElement | null;
                if (!wrapper) return;
                const raw = wrapper.dataset.mermaidIndex;
                const index = raw ? Number(raw) : NaN;
                if (!Number.isFinite(index)) return;
                openMermaidEditorByIndex(index);
              }}
              onKeyDown={(e) => {
                const target = e.target as HTMLElement | null;
                const wrapper = target?.closest?.('.mermaid-diagram') as HTMLElement | null;
                if (!wrapper) return;

                const raw = wrapper.dataset.mermaidIndex;
                const index = raw ? Number(raw) : NaN;
                if (!Number.isFinite(index)) return;

                if (e.key === 'Enter') {
                  e.preventDefault();
                  openMermaidEditorByIndex(index);
                  return;
                }

                if (e.key === 'Backspace' || e.key === 'Delete') {
                  e.preventDefault();
                  removeMermaidBlockByIndex(index);
                  return;
                }

                // Type-to-edit: abre o editor de Mermaid e injeta o primeiro caractere.
                if (
                  e.key.length === 1 &&
                  !e.ctrlKey &&
                  !e.metaKey &&
                  !e.altKey &&
                  !e.shiftKey
                ) {
                  e.preventDefault();
                  openMermaidEditorByIndex(index, { insertText: e.key });
                }
              }}
            >
              <div className="editor-page__pane-title">{t('editor.panes.preview')}</div>
              <div className="editor-page__preview">
                <div className="editor-page__preview-hint">
                  {t('editor.hints.previewMermaid')}
                </div>
                <MarkdownRenderer
                  content={debouncedMarkdownForPreview}
                  interactiveButtons={false}
                  focusableMermaid={true}
                />
              </div>
            </div>
          </div>
        ) : (
          <div className="editor-page__single">
            <div className="editor-page__pane" role="region" aria-label={t('editor.aria.richEditor')}>
              <div className="editor-page__pane-title">{t('editor.panes.rich')}</div>
              <div className="editor-page__pane-body">
                <RichTextEditor
                  ref={richEditorHandleRef}
                  ariaLabel={t('editor.richText.label')}
                  markdown={activeTab.markdown}
                  onMarkdownChange={(md) => {
                    setDocMarkdown(activeTab.id, md);
                    updateLatestMarkdownForTab(activeTab.id, md);
                    schedulePersistForTab(activeTab.id);
                  }}
                  readOnly={isAsking}
                  placeholder={t('editor.placeholders.rich')}
                  onEditorReady={(ed) => {
                    richEditorRef.current = ed;
                    setEditorReadyNonce((n) => n + 1);
                  }}
                  onRequestEditMermaid={(ctx) => {
                    const mermaidCtx = ctx as {
                      mermaidBlockId?: string;
                      insertText?: string;
                      code?: string;
                      apply: (nextCode: string) => void;
                      remove: () => void;
                    };
                    const mermaidBlockId = String(mermaidCtx.mermaidBlockId || '').trim();
                    const api = richEditorHandleRef.current;
                    setRichMermaidSession({
                      mermaidBlockId,
                      initialCode: String(mermaidCtx.code || ''),
                      insertText: String(mermaidCtx.insertText || ''),
                      apply: (nextCode: string) => {
                        if (mermaidBlockId && api?.applyMermaidById?.(mermaidBlockId, nextCode)) return;
                        mermaidCtx.apply(nextCode);
                      },
                      remove: () => {
                        if (mermaidBlockId && api?.removeMermaidById?.(mermaidBlockId)) return;
                        mermaidCtx.remove();
                      },
                    });
                  }}
                />
              </div>
            </div>
          </div>
        )}
      </div>

      <MermaidEditorModal
        isOpen={activeMermaidIndex !== null || richMermaidSession !== null}
        title="Editar diagrama Mermaid"
        initialCode={
          activeMermaidIndex !== null
            ? mermaidInitialCode
            : richMermaidSession?.initialCode || ''
        }
        initialInsertText={
          activeMermaidIndex !== null
            ? mermaidInsertText
            : String(richMermaidSession?.insertText || '')
        }
        onConsumeInsertText={() => {
          if (activeMermaidIndex !== null) setMermaidInsertText('');
          if (richMermaidSession) {
            setRichMermaidSession((prev) => (prev ? { ...prev, insertText: '' } : prev));
          }
        }}
        onCancel={() => {
          if (activeMermaidIndex !== null) setActiveMermaidIndex(null);
          if (richMermaidSession) setRichMermaidSession(null);
        }}
        onApply={(code) => {
          if (activeMermaidIndex !== null) {
            applyMermaidCode(code);
            return;
          }
          if (richMermaidSession) {
            richMermaidSession.apply(code);
            addToast('Bloco Mermaid atualizado', 'success');
            setRichMermaidSession(null);
          }
        }}
        onRemove={async () => {
          if (activeTab?.mode === 'markdown') {
            if (activeMermaidIndex === null) return;
            const index = activeMermaidIndex;
            const code = mermaidInitialCode;
            setActiveMermaidIndex(null);
            await removeMermaidBlockByIndex(index, { code });
            return;
          }

          if (richMermaidSession) {
            const confirm = await requestQuestionnaire({
              id: `ui-editor-rich-mermaid-remove-${Date.now()}`,
              title: 'Remover diagrama Mermaid',
              description: 'Tem certeza que deseja remover este bloco Mermaid do documento? ',
              submitLabel: 'Remover',
              cancelLabel: 'Cancelar',
              allowCancel: true,
              questions: [
                {
                  id: 'note',
                  type: 'readonly_code',
                  prompt: 'Dica',
                  content: 'Essa ação remove o bloco ```mermaid``` inteiro.',
                },
              ],
            });

            if (confirm.cancelled) return;
            richMermaidSession.remove();
            addToast('Bloco Mermaid removido', 'success');
            setRichMermaidSession(null);
          }
        }}
      />

      <Menu
        items={toolbarMenu.items}
        x={toolbarMenu.x}
        y={toolbarMenu.y}
        visible={toolbarMenu.visible}
        ariaLabel={toolbarMenu.ariaLabel}
        onClose={closeToolbarMenu}
        onSelect={handleToolbarMenuSelect}
      />
    </div>
  );
}
