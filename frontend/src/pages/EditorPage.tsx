import { useEffect, useMemo, useRef, useState } from 'react';
import { Toolbar } from '../components/ui/Toolbar';
import { ProfilePicker } from '../components/pickers/ProfilePicker';
import { EditorTabs } from '../components/editor/EditorTabs';
import { CodeEditor } from '../components/ui/CodeEditor';
import { MarkdownRenderer } from '../components/ui/MarkdownRenderer';
import { MermaidEditorModal } from '../components/editor/MermaidEditorModal';
import { RichTextEditor } from '../components/editor/RichTextEditor';
import { EditorInlineChatModal } from '@/components/editor/EditorInlineChatModal';
import { useEditorStore, type EditorMode, type EditorTab } from '../store/editorStore';
import { useEditorTabsKeyboardShortcuts } from '../hooks/useEditorTabsKeyboardShortcuts';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { createTwoFilesPatch } from 'diff';
import { buildEditorPatchPrompt, extractEditorPatch } from '../lib/editorPatch';
import { markdownToHtml } from '../lib/markdownToHtml';
import { findMermaidFenceByIndex, removeMermaidFence, replaceMermaidFenceCode } from '../lib/mermaidFence';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import {
  EditorDeleteDraft,
  EditorGetFileInfo,
  EditorLoadSession,
  EditorOpenFile,
  EditorReadDraft,
  EditorReadFile,
  EditorSaveFileDialog,
  EditorSaveSession,
  EditorUnwatchFile,
  EditorWatchFile,
  EditorWriteDraft,
  EditorWriteFile,
} from '@wailsjs/go/main/App';
import './EditorPage.css';

export default function EditorPage() {
  const { addToast } = useUIStore();
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);


  const tabs = useEditorStore((s) => s.tabs);
  const activeTabId = useEditorStore((s) => s.activeTabId);
  const createTab = useEditorStore((s) => s.createTab);
  const toggleTabMode = useEditorStore((s) => s.toggleTabMode);
  const setTabMarkdown = useEditorStore((s) => s.setTabMarkdown);
  const renameTab = useEditorStore((s) => s.renameTab);
  const setTabFilePath = useEditorStore((s) => s.setTabFilePath);
  const setTabDraftId = useEditorStore((s) => s.setTabDraftId);
  const setTabDirty = useEditorStore((s) => s.setTabDirty);
  const autoSaveEnabled = useEditorStore((s) => s.autoSaveEnabled);
  const toggleAutoSave = useEditorStore((s) => s.toggleAutoSave);
  const editorProfileSlug = useEditorStore((s) => s.editorProfileSlug);
  const setEditorProfileSlug = useEditorStore((s) => s.setEditorProfileSlug);
  const hydrate = useEditorStore((s) => s.hydrate);

  const activeTab = useMemo(() => tabs.find((t) => t.id === activeTabId) || null, [tabs, activeTabId]);

  // Atalhos globais das abas do editor (Ctrl+T/Ctrl+W/Ctrl+Tab...)
  useEditorTabsKeyboardShortcuts();

  const pageRootRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<any>(null);
  const monacoRef = useRef<any>(null);
  const richEditorRef = useRef<any>(null);

  const [isAsking, setIsAsking] = useState(false);
  const [showPreview, setShowPreview] = useState(true);

  const [activeMermaidIndex, setActiveMermaidIndex] = useState<number | null>(null);
  const [mermaidInitialCode, setMermaidInitialCode] = useState('');
  const [mermaidInsertText, setMermaidInsertText] = useState('');
  const [richMermaidSession, setRichMermaidSession] = useState<any | null>(null);

  type InlineChatSelection =
    | {
        mode: 'markdown';
        tabId: string;
        selectedText: string;
        /** True quando não há seleção (inserção no cursor) */
        selectionIsEmpty?: boolean;
        /** Contexto ao redor do cursor para orientar inserção */
        cursorContext?: string;
        /** Texto a exibir no painel "Contexto" do mini-chat */
        displayText?: string;
        startOffset: number;
        endOffset: number;
        snapshot: string;
      }
    | {
        mode: 'rich';
        tabId: string;
        selectedText: string;
        selectionIsEmpty?: boolean;
        cursorContext?: string;
        displayText?: string;
        from: number;
        to: number;
        snapshot: string;
      };

  const [inlineChatOpen, setInlineChatOpen] = useState(false);
  const [inlineChatSelection, setInlineChatSelection] = useState<InlineChatSelection | null>(null);
  const [inlineChatError, setInlineChatError] = useState<string | null>(null);
  const inlineChatRunIdRef = useRef(0);
  const [inlineChatFocusNonce, setInlineChatFocusNonce] = useState(0);

  const [sessionLoaded, setSessionLoaded] = useState(false);

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

  const refreshDiskInfoForTab = async (tab: EditorTab): Promise<DiskInfo | null> => {
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

  const getCachedMarkdownForTab = (tab: EditorTab): string => {
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

    setTabMarkdown(tabId, conflictText);
    updateLatestMarkdownForTab(tabId, conflictText);
    setTabDirty(tabId, true);

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

    const { tabs: currentTabs } = useEditorStore.getState();
    const tab = currentTabs.find((t) => t.id === tabId) || null;
    if (!tab || !tab.filePath) return;

    setExternalPromptInFlight(tabId, true);
    try {
      setExternalConflictLocked(tabId, true);
      setTabDirty(tabId, true);

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
        setTabDirty(tabId, false);
        const { tabs: afterTabs } = useEditorStore.getState();
        const afterTab = afterTabs.find((t) => t.id === tabId) || tab;
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
        title: 'Arquivo modificado fora do Assistente',
        description:
          'Este arquivo mudou no disco enquanto estava aberto aqui. Para evitar sobrescrever sem querer, o autosave deste arquivo fica travado até você decidir.',
        submitLabel: 'Aplicar',
        cancelLabel: 'Agora não',
        allowCancel: true,
        questions: [
          {
            id: 'path',
            type: 'readonly_code' as const,
            prompt: 'Arquivo',
            content: String(filePath || ''),
          },
          ...(diffPreview.preview
            ? [
                {
                  id: 'diff',
                  type: 'readonly_code' as const,
                  prompt: 'Diff (disco → minha versão)',
                  content: diffPreview.preview,
                },
              ]
            : []),
          {
            id: 'disk',
            type: 'readonly_code' as const,
            prompt: 'Versão do disco (preview)',
            content: diskPreview.preview,
          },
          {
            id: 'local',
            type: 'readonly_code' as const,
            prompt: 'Sua versão (preview)',
            content: localPreview.preview,
          },
          {
            id: 'choice',
            type: 'single_choice' as const,
            prompt: 'Ação',
            required: true,
            options: [
              'Usar versão do disco',
              'Resolver conflitos (estilo Git)',
              'Usar minha versão',
              'Salvar como…',
            ],
            default: 'Usar versão do disco',
          },
        ],
      });

      if (resp.cancelled) {
        addToast('Arquivo mudou fora do Assistente. Autosave travado até você decidir.', 'warning');
        return;
      }

      const choice = String(resp.answers?.choice || '').trim();

      if (choice.startsWith('Resolver conflitos')) {
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

      if (choice.startsWith('Usar versão do disco')) {
        if (diskReadError) {
          addToast('Não foi possível ler do disco. Tente novamente.', 'error');
          return;
        }

        try {
          setTabMarkdown(tabId, diskContent);
          updateLatestMarkdownForTab(tabId, diskContent);
          setDiskBaselineForTab(tabId, diskContent);
          setTabDirty(tabId, false);
          const { tabs: afterTabs } = useEditorStore.getState();
          const afterTab = afterTabs.find((t) => t.id === tabId) || tab;
          void refreshDiskInfoForTab(afterTab);
          setExternalConflictLocked(tabId, false);
          addToast('Recarregado do disco', 'success');
        } catch (e: any) {
          addToast(e?.message || 'Erro ao recarregar arquivo', 'error');
        }
        return;
      }

      if (choice.startsWith('Salvar como')) {
        const suggested = basenameFromPath(filePath) || 'documento.md';
        const newPath = String(await EditorSaveFileDialog(suggested) || '').trim();
        if (!newPath) return;

        updateLatestMarkdownForTab(tabId, localContent);
        markSelfWrite(newPath);
        await EditorWriteFile(newPath, localContent);
        setDiskBaselineForTab(tabId, localContent);

        const title = basenameFromPath(newPath);
        setTabFilePath(tabId, newPath);
        renameTab(tabId, title);
        setTabDraftId(tabId, null);
        setTabDirty(tabId, false);

        const { tabs: afterTabs } = useEditorStore.getState();
        const afterTab = afterTabs.find((t) => t.id === tabId) || tab;
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
        setTabDirty(tabId, false);
        const { tabs: afterTabs } = useEditorStore.getState();
        const afterTab = afterTabs.find((t) => t.id === tabId) || tab;
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
    const { tabs: currentTabs, autoSaveEnabled: currentAutoSaveEnabled } = useEditorStore.getState();
    const tab = currentTabs.find((t) => t.id === tabId) || null;
    if (!tab) return;

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
          setTabDirty(tabId, true);
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

      if (!currentAutoSaveEnabled) {
        // Sem autosave: não persiste no disco automaticamente.
        return;
      }

      markSelfWrite(filePath);
      await EditorWriteFile(filePath, markdown);
      setDiskBaselineForTab(tab.id, markdown);
      setTabDirty(tab.id, false);

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

  // Restaura sessão (guias abertas + autosave) de ~/.assistente/editor/session.json
  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const sess = await EditorLoadSession();
        if (cancelled) return;

        // Preferências por arquivo (se existir)
        try {
          const fromSess = (sess as any)?.fileModeByPath as Record<string, any> | undefined;
          if (fromSess && typeof fromSess === 'object') {
            const next: Record<string, 'markdown' | 'rich'> = {};
            for (const [k, v] of Object.entries(fromSess)) {
              const key = normalizePathKey(String(k || ''));
              if (!key) continue;
              next[key] = v === 'rich' ? 'rich' : 'markdown';
            }
            fileModeByPathRef.current = next;
          }
        } catch {
          // best-effort
        }

        const autoSaveEnabledFromSess = typeof (sess as any)?.autoSaveEnabled === 'boolean' ? !!(sess as any).autoSaveEnabled : true;
        const editorProfileSlugFromSess = String((sess as any)?.profileSlug || '').trim();

        const lockedFromSess = ((sess as any)?.externalConflictLockedByTabId || {}) as Record<string, any>;
        const mergeFromSess = ((sess as any)?.mergeSessionsByTabId || {}) as Record<string, any>;

        const rawTabs = Array.isArray((sess as any)?.tabs) ? ((sess as any).tabs as any[]) : [];
        const loadedTabs: EditorTab[] = [];

        for (const t of rawTabs) {
          const tabId = String(t?.id || '').trim();
          const filePath = String(t?.filePath || '').trim();
          const savedDraftId = String(t?.draftId || '').trim();
          const draftId = filePath ? '' : (savedDraftId || tabId);

          const mergeSessRaw = tabId ? (mergeFromSess[tabId] as any) : null;
          const hasMergeSess = !!mergeSessRaw && typeof mergeSessRaw === 'object' && String(mergeSessRaw?.conflictDraftId || '').trim();
          const isLocked = !!(tabId && lockedFromSess && (lockedFromSess as any)[tabId]);

          let markdown = '';
          try {
            if (filePath) {
              // Se havia merge em andamento, reabre do draft de conflito em vez do arquivo real.
              if (hasMergeSess) {
                const conflictDraftId = String(mergeSessRaw?.conflictDraftId || '').trim();
                const resDraft = await EditorReadDraft(conflictDraftId);
                markdown = String((resDraft as any)?.content ?? (resDraft as any) ?? '');
              } else {
                const res = await EditorReadFile(filePath);
                markdown = String((res as any)?.content ?? (res as any) ?? '');
              }
            } else if (draftId) {
              const res = await EditorReadDraft(draftId);
              markdown = String((res as any)?.content ?? (res as any) ?? '');
            }
          } catch {
            markdown = '';
          }

          const mode: EditorMode = t?.mode === 'rich' ? 'rich' : 'markdown';
          const title = String(t?.title || (filePath ? basenameFromPath(filePath) : 'Novo documento')) || 'Novo documento';

          loadedTabs.push({
            id: tabId,
            title,
            markdown,
            mode,
            filePath: filePath || null,
            draftId: filePath ? null : (draftId || null),
            isDirty: !!isLocked || !!hasMergeSess,
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

        // Baseline do disco para arquivos reais (best-effort; não bloqueia a UI)
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

        // Se não houver tabs na sessão, mantém vazio (usuário pode criar com Ctrl+N)
        const nextActiveTabId = String((sess as any)?.activeTabId || '').trim();
        const activeExists = !!loadedTabs.find((t) => t.id === nextActiveTabId);

        hydrate({
          tabs: loadedTabs,
          activeTabId: activeExists ? nextActiveTabId : (loadedTabs[0]?.id ?? null),
          autoSaveEnabled: autoSaveEnabledFromSess,
          editorProfileSlug: editorProfileSlugFromSess || 'editor-texto',
        });

        // Restaura locks e merge sessions em refs antes de liberar autosave.
        try {
          for (const t of loadedTabs) {
            if (!t?.id) continue;
            if (t.filePath && lockedFromSess && (lockedFromSess as any)[t.id]) {
              setExternalConflictLocked(t.id, true);
            }
            const raw = mergeFromSess && t.id ? (mergeFromSess as any)[t.id] : null;
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
        // Se falhar, ainda marca como carregado para permitir uso normal
        setSessionLoaded(true);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [hydrate]);

  const debouncedMarkdownForPreview = useDebouncedValue(activeTab?.markdown || '', 120);

  // Mantém cache do markdown atual da aba ativa para flush/abas.
  useEffect(() => {
    if (!sessionLoaded) return;
    if (!activeTab) return;
    updateLatestMarkdownForTab(activeTab.id, String(activeTab.markdown ?? ''));
  }, [sessionLoaded, activeTab?.id]);

  // Persiste a sessão (abas abertas) em ~/.assistente/editor/session.json
  useEffect(() => {
    if (!sessionLoaded) return;

    const timer = window.setTimeout(() => {
      // Atualiza preferências por arquivo com o estado atual das tabs
      try {
        for (const t of tabs) {
          if (t.filePath) {
            fileModeByPathRef.current[normalizePathKey(String(t.filePath))] = t.mode;
          }
        }
      } catch {
        // best-effort
      }

      const externalConflictLockedByTabId: Record<string, boolean> = {};
      const mergeSessionsByTabId: Record<string, any> = {};
      try {
        for (const t of tabs) {
          if (!t?.id) continue;
          if (isExternalConflictLocked(t.id)) externalConflictLockedByTabId[t.id] = true;
          const ms = getMergeSession(t.id);
          if (ms) mergeSessionsByTabId[t.id] = ms;
        }
      } catch {
        // best-effort
      }

      const payload = {
        version: 2,
        autoSaveEnabled: !!autoSaveEnabled,
        activeTabId: activeTabId || '',
        profileSlug: editorProfileSlug,
        fileModeByPath: fileModeByPathRef.current,
        externalConflictLockedByTabId,
        mergeSessionsByTabId,
        tabs: tabs.map((t) => ({
          id: t.id,
          title: t.title,
          mode: t.mode,
          filePath: t.filePath || '',
          draftId: t.filePath ? '' : (t.draftId || t.id),
        })),
      };
      EditorSaveSession(payload as any).catch((e) => {
        console.warn('[EditorPage] falha ao salvar sessão:', e);
      });
    }, 500);

    return () => window.clearTimeout(timer);
  }, [sessionLoaded, tabs, activeTabId, autoSaveEnabled, editorProfileSlug]);

  // Flush imediato ao fechar/minimizar para reduzir chance de perder a sessão
  useEffect(() => {
    if (!sessionLoaded) return;

    const persistNow = () => {
      // Flush best-effort do conteúdo da aba ativa antes de persistir sessão.
      try {
        const { activeTabId: currentActive } = useEditorStore.getState();
        if (currentActive) {
          void persistTabContentNow(currentActive);
        }
      } catch {
        // best-effort
      }

      // Atualiza preferências por arquivo com o estado atual das tabs
      try {
        for (const t of tabs) {
          if (t.filePath) {
            fileModeByPathRef.current[normalizePathKey(String(t.filePath))] = t.mode;
          }
        }
      } catch {
        // best-effort
      }

      const externalConflictLockedByTabId: Record<string, boolean> = {};
      const mergeSessionsByTabId: Record<string, any> = {};
      try {
        for (const t of tabs) {
          if (!t?.id) continue;
          if (isExternalConflictLocked(t.id)) externalConflictLockedByTabId[t.id] = true;
          const ms = getMergeSession(t.id);
          if (ms) mergeSessionsByTabId[t.id] = ms;
        }
      } catch {
        // best-effort
      }

      const payload = {
        version: 2,
        autoSaveEnabled: !!autoSaveEnabled,
        activeTabId: activeTabId || '',
        profileSlug: editorProfileSlug,
        fileModeByPath: fileModeByPathRef.current,
        externalConflictLockedByTabId,
        mergeSessionsByTabId,
        tabs: tabs.map((t) => ({
          id: t.id,
          title: t.title,
          mode: t.mode,
          filePath: t.filePath || '',
          draftId: t.filePath ? '' : (t.draftId || t.id),
        })),
      };
      EditorSaveSession(payload as any).catch(() => null);
    };

    const onBeforeUnload = () => persistNow();
    const onPageHide = () => persistNow();
    const checkActiveFileExternalChange = async () => {
      const { tabs: currentTabs, activeTabId: currentActiveTabId } = useEditorStore.getState();
      const tab = currentTabs.find((t) => t.id === currentActiveTabId) || null;
      if (!tab?.filePath) return;
      if (isExternalConflictLocked(tab.id)) return;

      const lastDisk = diskInfoByTabRef.current[String(tab.id)];
      const currentDisk = await refreshDiskInfoForTab(tab);
      if (!currentDisk) return;

      if (lastDisk && !diskInfoEquals(lastDisk, currentDisk)) {
        setExternalConflictLocked(tab.id, true);
        setTabDirty(tab.id, true);
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
  }, [sessionLoaded, tabs, activeTabId, autoSaveEnabled]);

  // Watcher de mudanças externas (backend emite editor:fileChanged)
  const watchedFilesRef = useRef<Record<string, { path: string; count: number }>>({});

  useEffect(() => {
    if (!sessionLoaded) return;

    const next: Record<string, { path: string; count: number }> = {};
    for (const t of tabs) {
      if (!t.filePath) continue;
      const p = String(t.filePath || '').trim();
      const key = normalizePathKey(p);
      if (!key) continue;
      if (!next[key]) next[key] = { path: p, count: 0 };
      next[key].count += 1;
    }

    const prev = watchedFilesRef.current;

    // Unwatch (reduções)
    for (const [key, entry] of Object.entries(prev)) {
      const prevCount = entry.count;
      const nextCount = next[key]?.count ?? 0;
      const diff = prevCount - nextCount;
      if (diff <= 0) continue;
      // Backend tem refcount também; mantemos o nosso para evitar chamadas excessivas.
      for (let i = 0; i < diff; i++) {
        EditorUnwatchFile(entry.path).catch(() => null);
      }
    }

    // Watch (aumentos)
    for (const [key, entry] of Object.entries(next)) {
      const prevCount = prev[key]?.count ?? 0;
      const diff = entry.count - prevCount;
      if (diff <= 0) continue;
      for (let i = 0; i < diff; i++) {
        EditorWatchFile(entry.path).catch(() => null);
      }
    }

    watchedFilesRef.current = next;
  }, [sessionLoaded, tabs]);

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

      const { tabs: currentTabs } = useEditorStore.getState();
      const affected = currentTabs.filter((t) => t.filePath && normalizePathKey(String(t.filePath)) === key);
      if (affected.length === 0) return;

      // Lê o disco uma única vez por evento (mesmo arquivo pode estar em múltiplas abas)
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
            setTabDirty(t.id, false);
            void refreshDiskInfoForTab(t);
            // Não abre prompt.
            continue;
          }

          // Aba limpa: recarrega automaticamente, mas só se realmente mudou
          if (!t.isDirty) {
            try {
              setTabMarkdown(t.id, diskContent);
              updateLatestMarkdownForTab(t.id, diskContent);
              setDiskBaselineForTab(t.id, diskContent);
              setTabDirty(t.id, false);
              void refreshDiskInfoForTab(t);
              if (t.id === activeTabId) addToast('Arquivo recarregado do disco (mudança externa)', 'info');
            } catch {
              // Se não der pra aplicar automaticamente, cai pro fluxo existente
              setExternalConflictLocked(t.id, true);
              setTabDirty(t.id, true);
              if (!isExternalPromptInFlight(t.id)) {
                void promptResolveExternalChangeForTab(t.id, String(t.filePath), { diskContent, diskReadError });
              }
            }
            continue;
          }
        }
        // Aba dirty (ou falha ao ler o disco): pede decisão explícita
        setExternalConflictLocked(t.id, true);
        setTabDirty(t.id, true);
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
  }, [sessionLoaded, activeTabId]);

  // F6: circular foco entre abas, toolbar principal e toolbar de formatação (quando existir)
  useEffect(() => {
    const focusFirstEnabledButton = (root: Element | null) => {
      if (!root) return false;
      const btn = root.querySelector('button:not([disabled])') as HTMLButtonElement | null;
      if (!btn) return false;
      btn.focus();
      return true;
    };

    const focusTabs = () => {
      const root = pageRootRef.current;
      if (!root) return false;
      const active = root.querySelector('.editor-tabs [role="tab"][aria-selected="true"]') as HTMLButtonElement | null;
      const anyTab = root.querySelector('.editor-tabs [role="tab"]') as HTMLButtonElement | null;
      (active || anyTab)?.focus();
      return !!(active || anyTab);
    };

    const focusMainToolbar = () => {
      const root = pageRootRef.current;
      if (!root) return false;
      const toolbar = root.querySelector('.editor-page__toolbar') as Element | null;
      if (!toolbar) return false;
      return focusFirstEnabledButton(toolbar);
    };

    const focusFormatToolbar = () => {
      const root = pageRootRef.current;
      if (!root) return false;
      const toolbar = root.querySelector('.rich-text-editor__format-toolbar') as Element | null;
      if (!toolbar) return false;
      return focusFirstEnabledButton(toolbar);
    };

    const focusEditor = () => {
      if (!activeTab) return false;

      if (activeTab.mode === 'markdown') {
        try {
          const monacoEditor = editorRef.current;
          monacoEditor?.focus?.();
          return true;
        } catch {
          return false;
        }
      }

      if (activeTab.mode === 'rich') {
        try {
          const tiptap = richEditorRef.current;
          tiptap?.commands?.focus?.();
          tiptap?.view?.focus?.();
          return true;
        } catch {
          return false;
        }
      }

      return false;
    };

    const getCurrentZone = () => {
      const el = document.activeElement as HTMLElement | null;
      if (!el) return 'unknown' as const;
      if (el.closest?.('.editor-tabs')) return 'tabs' as const;
      if (el.closest?.('.editor-page__toolbar')) return 'main' as const;
      if (el.closest?.('.rich-text-editor__format-toolbar')) return 'format' as const;
      if (el.closest?.('.rich-text-editor__content')) return 'editor' as const;
      if (el.closest?.('.monaco-editor')) return 'editor' as const;
      return 'unknown' as const;
    };

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== 'F6') return;
      if (document.querySelector('.simple-modal-overlay')) return;

      e.preventDefault();
      e.stopPropagation();

      const zones = ['tabs', 'main', 'format', 'editor'] as const;
      const available = zones.filter((z) => {
        if (z === 'format') return activeTab?.mode === 'rich';
        if (z === 'editor') return !!activeTab;
        return true;
      });
      const current = getCurrentZone();
      const idx = Math.max(0, available.indexOf(current as any));
      const dir = e.shiftKey ? -1 : 1;
      const next = available[(idx + dir + available.length) % available.length];

      if (next === 'tabs') {
        focusTabs();
        return;
      }
      if (next === 'main') {
        focusMainToolbar();
        return;
      }
      if (next === 'format') {
        if (!focusFormatToolbar()) {
          // fallback
          focusMainToolbar() || focusTabs();
        }
        return;
      }

      if (next === 'editor') {
        if (!focusEditor()) {
          // fallback
          focusMainToolbar() || focusTabs();
        }
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [activeTab?.id, activeTab?.mode]);

  // Limpa drafts quando uma aba (rascunho) é fechada
  const prevTabsRef = useRef<typeof tabs>([]);
  useEffect(() => {
    if (!sessionLoaded) return;

    const prev = prevTabsRef.current;
    prevTabsRef.current = tabs;

    const prevById = new Map(prev.map((t) => [t.id, t] as const));
    const nextIds = new Set(tabs.map((t) => t.id));

    const removed = prev.filter((t) => !nextIds.has(t.id));
    for (const tab of removed) {
      const was = prevById.get(tab.id);
      if (!was) continue;
      if (was.filePath) continue;
      const draftId = was.draftId || was.id;
      EditorDeleteDraft(draftId).catch(() => null);
    }
  }, [sessionLoaded, tabs]);

  const waitForChatDone = (expectedConversationId?: number, timeoutMs = 5 * 60 * 1000) => {
    return new Promise<number>((resolve, reject) => {
      let timer: number;
      const unsub = EventsOn('chat:done', (data: any) => {
        const convId = data?.conversationId;
        if (typeof convId !== 'number') return;
        if (expectedConversationId && expectedConversationId > 0 && convId !== expectedConversationId) return;
        window.clearTimeout(timer);
        unsub();
        resolve(convId);
      });

      timer = window.setTimeout(() => {
        unsub();
        reject(new Error('Timeout aguardando chat:done'));
      }, timeoutMs);
    });
  };

  const parseToolCalls = (toolCallsJson: any): any[] => {
    if (!toolCallsJson) return [];

    // Banco/Wails normalmente entrega string JSON em message.toolCalls.
    // Mas aceitamos também o caso de já ser um array/objeto.
    if (Array.isArray(toolCallsJson)) return toolCallsJson;
    if (typeof toolCallsJson === 'object') return [toolCallsJson];

    if (typeof toolCallsJson !== 'string') return [];
    const raw = toolCallsJson.trim();
    if (!raw) return [];

    try {
      const parsed = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed : [parsed];
    } catch {
      return [];
    }
  };

  const extractTextEditToolCallIds = (toolCallsJson: any): string[] => {
    const calls = parseToolCalls(toolCallsJson);
    const ids: string[] = [];
    for (const c of calls) {
      const name = String(c?.function?.name || c?.name || '').trim();
      const id = String(c?.id || c?.callId || '').trim();
      if (name === 'text_edit' && id) ids.push(id);
    }
    return ids;
  };

  const parseEditorPatchFromToolResultContent = (toolContent: string): any => {
    const raw = String(toolContent || '').trim();
    if (!raw) return null;
    try {
      const parsed = JSON.parse(raw);
      // Pode vir direto como patch, ou embrulhado (não deveria, mas aceitamos).
      const candidate = parsed?.patch && typeof parsed?.patch === 'object' ? parsed.patch : parsed;
      if (candidate?.v !== 1 || candidate?.op !== 'replace_selection') return null;
      if (candidate?.format !== 'markdown' && candidate?.format !== 'plain') return null;
      if (typeof candidate?.replacement !== 'string') return null;
      return candidate;
    } catch {
      return null;
    }
  };

  const getMaxNumericMessageId = (messages: any[]): number => {
    let maxId = 0;
    for (const m of messages) {
      const n = typeof m?.id === 'number' ? m.id : parseInt(String(m?.id || ''), 10);
      if (!isNaN(n) && n > maxId) maxId = n;
    }
    return maxId;
  };

  const findLatestEditorPatch = (
    chatTabId: string,
    opts?: {
      /** Ignora mensagens com id <= afterMessageId (evita pegar patches antigos) */
      afterMessageId?: number;
      /** Se true, prioriza tool calling (text_edit) em vez de patch no corpo */
      preferToolCalling?: boolean;
      /** Se false, NUNCA tenta extrair patch do corpo (modo tool-only) */
      allowBodyFallback?: boolean;
    }
  ): any => {
    const afterState = useChatStore.getState();
    const allMessages = afterState.getTabMessages(chatTabId);
    const afterMessageId = opts?.afterMessageId || 0;
    const preferToolCalling = opts?.preferToolCalling !== false;

    const messages = afterMessageId > 0
      ? allMessages.filter((m: any) => {
          const n = typeof m?.id === 'number' ? m.id : parseInt(String(m?.id || ''), 10);
          return !isNaN(n) && n > afterMessageId;
        })
      : allMessages;

    const allowBodyFallback = opts?.allowBodyFallback !== false;

    // Indexa resultados de tools por toolCallId
    const toolResultsByCallId = new Map<string, string>();
    for (const m of messages) {
      if (m?.role !== 'tool') continue;
      const callId = String(m?.toolCallId || '').trim();
      if (!callId) continue;
      toolResultsByCallId.set(callId, String(m?.content || ''));
    }

    // Passo 1 (preferido): assistant tool_calls(text_edit) + tool_result correspondente.
    for (let i = messages.length - 1; i >= 0; i--) {
      const msg: any = messages[i];
      if (msg?.role !== 'assistant') continue;

      const textEditCallIds = extractTextEditToolCallIds(msg?.toolCalls);
      if (textEditCallIds.length > 0) {
        // Pega o último text_edit deste assistant
        for (let j = textEditCallIds.length - 1; j >= 0; j--) {
          const callId = textEditCallIds[j];
          const toolContent = toolResultsByCallId.get(callId);
          if (!toolContent) continue;

          const patch = parseEditorPatchFromToolResultContent(toolContent);
          if (patch) return { ok: true, patch, source: 'tool' } as any;

          // Caso a tool tenha retornado mensagem de rejeição/erro, propaga isso.
          const toolText = String(toolContent || '').trim();
          if (toolText) {
            return { ok: false, error: toolText } as any;
          }
        }

        // Tem tool_calls, mas ainda não apareceu o tool_result correspondente.
        return { ok: false, error: 'Aguardando resultado da ferramenta…' } as any;
      }
    }

    // Passo 2 (fallback): patch no corpo da resposta (apenas quando não há tool calling)
    if (!preferToolCalling) {
      for (let i = messages.length - 1; i >= 0; i--) {
        const msg: any = messages[i];
        if (msg?.role !== 'assistant') continue;
        const content = String(msg?.content || '');
        const extracted = extractEditorPatch(content);
        if (extracted.ok) return { ...extracted, source: 'body' } as any;
      }
      return { ok: false, error: 'Nenhum patch encontrado' } as any;
    }

    // preferToolCalling=true: só aceita patch do corpo se explicitamente permitido.
    if (allowBodyFallback) {
      for (let i = messages.length - 1; i >= 0; i--) {
        const msg: any = messages[i];
        if (msg?.role !== 'assistant') continue;
        const content = String(msg?.content || '');
        const extracted = extractEditorPatch(content);
        if (extracted.ok) return { ...extracted, source: 'body' } as any;
      }
    }

    return {
      ok: false,
      error: allowBodyFallback
        ? 'Nenhum patch encontrado'
        : 'Tool calling está ativo: aguardando um text_edit (nenhum tool_call foi recebido).',
    } as any;
  };

  const waitForEditorPatch = async (
    chatTabId: string,
    opts?: {
      afterMessageId?: number;
      preferToolCalling?: boolean;
      allowBodyFallback?: boolean;
      timeoutMs?: number;
    }
  ) => {
    const timeoutMs = typeof opts?.timeoutMs === 'number' ? opts.timeoutMs : 5000;
    const startedAt = Date.now();
    while (Date.now() - startedAt < timeoutMs) {
      const found = findLatestEditorPatch(chatTabId, opts);
      if (found?.ok) return found;
      await new Promise((r) => setTimeout(r, 120));
    }

    // Última tentativa: retorna o melhor erro que tivermos
    return findLatestEditorPatch(chatTabId, opts);
  };

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
    const displayText = selectionIsEmpty ? (cursorContext || '(cursor)') : selectedText;

    return { selectedText, selectionIsEmpty, cursorContext, displayText, from, to };
  };

  const focusEditorSoon = () => {
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
  };

  const closeInlineChatModal = () => {
    inlineChatRunIdRef.current += 1;
    setInlineChatOpen(false);
    setInlineChatSelection(null);
    setInlineChatError(null);
    setIsAsking(false);
    focusEditorSoon();
  };

  const askInlineChat = async () => {
    if (!activeTab) return;

    const chatState = useChatStore.getState();
    if (!chatState.activeTabId) {
      addToast('Nenhuma aba de chat ativa para enviar a mensagem.', 'error');
      return;
    }

    const selectionRaw =
      activeTab.mode === 'markdown'
        ? getSelectionSnapshot()
        : activeTab.mode === 'rich'
          ? getRichSelectionSnapshot()
          : null;

    if (!selectionRaw) {
      addToast('Não foi possível capturar a seleção do editor.', 'error');
      return;
    }

    if (selectionRaw.selectedText.length > 20000) {
      addToast('Seleção muito grande para enviar ao chat (limite: 20.000 caracteres).', 'error');
      return;
    }

    const snapshot =
      activeTab.mode === 'markdown'
        ? (editorRef.current?.getModel?.()?.getValue?.() ?? activeTab.markdown)
        : activeTab.markdown;
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
            selectionIsEmpty: !!(selectionRaw as any).selectionIsEmpty,
            cursorContext: (selectionRaw as any).cursorContext,
            displayText: (selectionRaw as any).displayText,
            from: (selectionRaw as any).from,
            to: (selectionRaw as any).to,
            snapshot,
          };

    setInlineChatError(null);
    setInlineChatSelection(selection);
    setInlineChatOpen(true);
    setInlineChatFocusNonce((n) => n + 1);
  };

  const askInlineChatRef = useRef(askInlineChat);
  useEffect(() => {
    askInlineChatRef.current = askInlineChat;
  }, [askInlineChat]);

  const sendInlineChatInstruction = async (instruction: string, mediaFiles?: any[]) => {
    if (!activeTab) return;
    if (!inlineChatSelection) {
      addToast('Seleção do editor não está disponível.', 'error');
      return;
    }

    const chatState = useChatStore.getState();
    if (chatState.isLoading) {
      addToast('O chat já está respondendo. Aguarde terminar.', 'info');
      return;
    }

    const chatTabId = chatState.activeTabId;
    const chatTab = chatTabId ? chatState.tabs.find((t) => t.id === chatTabId) : undefined;
    const expectedConversationId = chatTab?.conversationId;
    if (!chatTabId) {
      addToast('Nenhuma aba de chat ativa para enviar a mensagem.', 'error');
      return;
    }

    // Marca o estado atual para não capturar patches antigos do histórico.
    const beforeMessages = chatState.getTabMessages(chatTabId);
    const afterMessageId = getMaxNumericMessageId(beforeMessages as any);

    const trimmed = String(instruction || '').trim();
    if (!trimmed) return;

    const prompt = buildEditorPatchPrompt({
      instruction: trimmed,
      selectedText: inlineChatSelection.selectedText,
      format: 'markdown',
      selectionIsEmpty: !!(inlineChatSelection as any)?.selectionIsEmpty,
      cursorContext: (inlineChatSelection as any)?.cursorContext,
    });

    const runId = (inlineChatRunIdRef.current += 1);
    setInlineChatError(null);

    const normalizeReplacementForEditor = (raw: string, patchFormat: any, selectedText: string) => {
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

    const applyInlinePatchNow = (selection: InlineChatSelection, patch: any) => {
      const replacement = normalizeReplacementForEditor(String(patch?.replacement || ''), patch?.format, selection?.selectedText);
      const { tabs: currentTabs, activeTabId: currentActiveTabId } = useEditorStore.getState();
      const tab = currentTabs.find((t) => t.id === selection.tabId) || null;
      if (!tab) {
        addToast('Aba do editor não encontrada para aplicar a alteração.', 'error');
        setIsAsking(false);
        focusEditorSoon();
        return;
      }

      if (selection.mode === 'markdown') {
        const s = selection;

        // Para offsets de Markdown, garantimos que a aba original esteja ativa
        // e usamos o texto do model do Monaco (onde os offsets foram calculados).
        if (currentActiveTabId !== s.tabId) {
          addToast('Abra a aba original do editor para aplicar esta alteração.', 'info');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const model = editorRef.current?.getModel?.();
        const current = model?.getValue?.() ?? String(tab.markdown ?? '');

        // Se o conteúdo mudou desde o snapshot, evita aplicar offsets errados.
        const selectedInCurrent = current.slice(s.startOffset, s.endOffset);
        if (selectedInCurrent !== s.selectedText) {
          addToast('O texto selecionado mudou desde que você abriu o mini-chat. Refazer a seleção e tentar novamente.', 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const nextMarkdown = current.slice(0, s.startOffset) + replacement + current.slice(s.endOffset);
        setTabMarkdown(s.tabId, nextMarkdown);
        updateLatestMarkdownForTab(s.tabId, nextMarkdown);
        schedulePersistForTab(s.tabId);
        addToast('Alteração aplicada', 'success');

        requestAnimationFrame(() => {
          try {
            const editor = editorRef.current;
            const m = editor?.getModel?.();
            if (!editor || !m) return;
            if (currentActiveTabId !== s.tabId) return;
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
        if (currentActiveTabId !== s.tabId) {
          addToast('Abra a aba original do editor para aplicar esta alteração.', 'info');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }
        const rich = richEditorRef.current;
        if (!rich) {
          addToast('Editor rico não está pronto.', 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        // Evita aplicar em um range errado caso a seleção tenha mudado enquanto o mini-chat estava aberto.
        try {
          const currentSel = rich.state?.selection;
          if (!currentSel) {
            addToast('Não foi possível ler a seleção atual do editor rico. Refazer a seleção e tentar novamente.', 'error');
            setIsAsking(false);
            focusEditorSoon();
            return;
          }

          const expectedEmpty = !!(s as any).selectionIsEmpty;
          const expectedFrom = Number((s as any).from);
          const expectedTo = Number((s as any).to);

          if (currentSel.from !== expectedFrom || currentSel.to !== expectedTo) {
            addToast('A seleção mudou desde que você abriu o mini-chat. Refazer a seleção e tentar novamente.', 'error');
            setIsAsking(false);
            focusEditorSoon();
            return;
          }

          if (expectedEmpty && !currentSel.empty) {
            addToast('A seleção mudou desde que você abriu o mini-chat. Refazer a seleção e tentar novamente.', 'error');
            setIsAsking(false);
            focusEditorSoon();
            return;
          }

          if (!expectedEmpty) {
            const currentSelectedText = rich.state.doc.textBetween(currentSel.from, currentSel.to, '\n');
            if (String(currentSelectedText) !== String((s as any).selectedText || '')) {
              addToast('O texto selecionado mudou desde que você abriu o mini-chat. Refazer a seleção e tentar novamente.', 'error');
              setIsAsking(false);
              focusEditorSoon();
              return;
            }
          }
        } catch {
          addToast('Não foi possível validar a seleção do editor rico. Refazer a seleção e tentar novamente.', 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const wasEditable = !!rich.isEditable;
        try {
          if (!wasEditable) rich.setEditable?.(true);
          const contentToInsert = patch?.format === 'markdown' ? markdownToHtml(replacement) : replacement;
          rich
            .chain()
            .focus()
            .setTextSelection({ from: s.from, to: s.to })
            .insertContent(contentToInsert)
            .run();
          addToast('Alteração aplicada', 'success');
        } finally {
          if (!wasEditable) rich.setEditable?.(false);
        }
      }

      setInlineChatError(null);
      setInlineChatSelection(null);
      setInlineChatOpen(false);
      setIsAsking(false);
      focusEditorSoon();
    };

    const confirmInlinePatch = async (selection: InlineChatSelection, patch: any) => {
      const before = String(selection?.selectedText || '');
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

      addToast('Alteração rejeitada', 'info');
      setIsAsking(false);
      // Mantém o mini-chat aberto para você criticar/explicar detalhes.
      // Apenas devolve o foco para o input do mini-chat.
      setInlineChatFocusNonce((n) => n + 1);
    };

    try {
      setIsAsking(true);
      const donePromise = waitForChatDone(expectedConversationId);
      await useChatStore.getState().sendMessageWithParams(prompt, mediaFiles, { profileSlug: editorProfileSlug });
      await donePromise;

      if (runId !== inlineChatRunIdRef.current) return;

      const extracted = await waitForEditorPatch(chatTabId, {
        afterMessageId,
        preferToolCalling: true,
        allowBodyFallback: false,
        timeoutMs: 8000,
      });
      if (!extracted.ok) {
        const errText = String(extracted.error || '').trim();

        // Se a própria tool foi rejeitada/cancelada pelo usuário, não trata como erro.
        if (/rejeitad|cancelad/i.test(errText)) {
          addToast('Alteração rejeitada', 'info');
          setIsAsking(false);
          setInlineChatFocusNonce((n) => n + 1);
          return;
        }

        setInlineChatError(errText || 'Nenhum patch encontrado');
        setIsAsking(false);
        return;
      }

      // Se veio de tool calling (text_edit), o usuário já confirmou na tool.
      // Evita dupla confirmação e evita aplicar algo vindo do corpo da resposta.
      if (extracted.source === 'tool') {
        applyInlinePatchNow(inlineChatSelection, extracted.patch as any);
        return;
      }

      // Fallback (sem tool calling): confirma antes de aplicar.
      await confirmInlinePatch(inlineChatSelection, extracted.patch as any);
    } catch (e: any) {
      console.error('[EditorPage] inline chat error:', e);
      setInlineChatError(e?.message || 'Erro ao pedir alteração ao chat');
      setIsAsking(false);
    }
  };

  const openFile = async () => {
    try {
      const res = await EditorOpenFile();
      const path = String(res?.path || '').trim();
      if (!path) return;

      const key = normalizePathKey(path);
      const preferredMode = fileModeByPathRef.current[key] || tabs.find((t) => t.filePath && normalizePathKey(String(t.filePath)) === key)?.mode || 'markdown';

      const title = basenameFromPath(path);
      const id = createTab({ title, markdown: String(res?.content || ''), mode: preferredMode });
      renameTab(id, title);
      setTabFilePath(id, path);
      setTabDraftId(id, null);
      setTabDirty(id, false);

      updateLatestMarkdownForTab(id, String(res?.content || ''));
      setDiskBaselineForTab(id, String(res?.content || ''));
      void refreshDiskInfoForTab({ id, title, markdown: String(res?.content || ''), mode: preferredMode, filePath: path } as any);

      fileModeByPathRef.current[key] = preferredMode;

      // createTab cria um draftId por padrão; como agora é um arquivo real, limpamos.
      EditorDeleteDraft(id).catch(() => null);
      addToast('Arquivo aberto', 'success');
      focusEditorSoon();
    } catch (e: any) {
      console.error('[EditorPage] openFile error:', e);
      addToast(e?.message || 'Erro ao abrir arquivo', 'error');
    }
  };

  const abortMerge = async () => {
    if (!activeTab?.filePath) return;

    const sess = getMergeSession(activeTab.id);
    if (!sess) return;

    let mineContent = '';
    try {
      const res = await EditorReadDraft(sess.mineDraftId);
      mineContent = String((res as any)?.content ?? (res as any) ?? '');
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

    setTabMarkdown(activeTab.id, mineContent);
    updateLatestMarkdownForTab(activeTab.id, mineContent);
    setTabDirty(activeTab.id, true);

    await cleanupMergeSessionForTab(activeTab.id);

    addToast('Merge abortado. Sua versão foi restaurada. Use Salvar para resolver a modificação externa.', 'info');
    focusEditorSoon();
  };

  const saveFile = async () => {
    if (!activeTab) return;
    try {
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
            setTabDirty(activeTab.id, false);
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
        setTabDirty(activeTab.id, false);
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
      setTabFilePath(activeTab.id, path);
      renameTab(activeTab.id, title);
      setTabDirty(activeTab.id, false);

      void refreshDiskInfoForTab({ ...activeTab, filePath: path } as any);

      const draftId = activeTab.draftId || activeTab.id;
      setTabDraftId(activeTab.id, null);
      await EditorDeleteDraft(draftId);

      addToast('Arquivo salvo', 'success');
      focusEditorSoon();
    } catch (e: any) {
      console.error('[EditorPage] saveFile error:', e);
      addToast(e?.message || 'Erro ao salvar', 'error');
    }
  };

  const saveFileAsCopy = async () => {
    if (!activeTab?.filePath) return;
    try {
      const suggested = basenameFromPath(activeTab.filePath);
      const path = String(await EditorSaveFileDialog(suggested) || '').trim();
      if (!path) return;
      const content = getCachedMarkdownForTab(activeTab);
      updateLatestMarkdownForTab(activeTab.id, content);
      markSelfWrite(path);
      await EditorWriteFile(path, content);
      addToast('Cópia salva', 'success');
      focusEditorSoon();
    } catch (e: any) {
      console.error('[EditorPage] saveAs error:', e);
      addToast(e?.message || 'Erro ao salvar como', 'error');
    }
  };

  const openMermaidEditorByIndex = (index: number, opts?: { insertText?: string }) => {
    if (!activeTab) return;
    const fence = findMermaidFenceByIndex(activeTab.markdown, index);
    if (!fence) {
      addToast('Não foi possível localizar o bloco Mermaid no Markdown.', 'error');
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
    setTabMarkdown(activeTab.id, nextMarkdown);
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
    setTabMarkdown(activeTab.id, nextMarkdown);
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

  // Ctrl+Shift+I: pedir alteração ao chat
  useEffect(() => {
    const onKeyDown = async (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && (e.code === 'KeyI' || e.key === 'i' || e.key === 'I') && !e.altKey) {
        e.preventDefault();
        if (isAsking) return;
        if (document.querySelector('.simple-modal-overlay')) return;
        await askInlineChatRef.current();
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [isAsking]);

  const actions = useMemo(() => {
    const modeLabel = activeTab?.mode === 'markdown' ? 'Modo rico' : 'Modo Markdown';
    const previewLabel = showPreview ? 'Ocultar preview' : 'Mostrar preview';

    const canSave = !!activeTab && (!activeTab.filePath || !autoSaveEnabled || isExternalConflictLocked(activeTab.id));
    const canSaveAs = !!activeTab?.filePath;
    const autoSaveLabel = autoSaveEnabled ? 'AutoSave: ligado' : 'AutoSave: desligado';

    const hasMergeSession = !!activeTab && !!getMergeSession(activeTab.id);

    return [
      {
        key: 'new',
        label: 'Novo',
        icon: '+',
        shortcut: 'Ctrl+N',
        onClick: () => {
          createTab();
          focusEditorSoon();
        },
      },
      {
        key: 'open',
        label: 'Abrir',
        icon: '📂',
        shortcut: 'Ctrl+O',
        onClick: openFile,
      },
      {
        key: 'save',
        label: 'Salvar',
        icon: '💾',
        shortcut: 'Ctrl+S',
        onClick: saveFile,
        disabled: !canSave,
      },
      ...(hasMergeSession
        ? [
            {
              key: 'abort-merge',
              label: 'Abortar merge',
              icon: '⟲',
              onClick: abortMerge,
            },
          ]
        : []),
      {
        key: 'saveas',
        label: 'Salvar como',
        icon: '📄',
        shortcut: 'Ctrl+Shift+S',
        onClick: saveFileAsCopy,
        disabled: !canSaveAs,
      },
      {
        key: 'autosave',
        label: autoSaveLabel,
        icon: autoSaveEnabled ? '🟢' : '⚪',
        onClick: () => {
          toggleAutoSave();

          // Se acabou de ligar e já tem destino, tenta persistir imediatamente.
          const nextEnabled = !autoSaveEnabled;
          if (nextEnabled && activeTab?.filePath) {
            void persistTabContentNow(activeTab.id);
          }

          focusEditorSoon();
        },
        disabled: !activeTab,
      },
      {
        key: 'toggle',
        label: modeLabel,
        icon: '⇄',
        onClick: () => {
          if (!activeTab) return;
          toggleTabMode(activeTab.id);

          // Se for arquivo real, memoriza preferência para reabrir no mesmo modo
          if (activeTab.filePath) {
            const nextMode = activeTab.mode === 'markdown' ? 'rich' : 'markdown';
            fileModeByPathRef.current[normalizePathKey(String(activeTab.filePath))] = nextMode;
          }

          focusEditorSoon();
        },
        disabled: !activeTab,
      },
      {
        key: 'ask',
        label: 'Perguntar ao chat',
        icon: '💬',
        shortcut: 'Ctrl+Shift+I',
        onClick: async () => {
          if (isAsking) return;
          await askInlineChat();
        },
        disabled: !activeTab || isAsking,
      },
      {
        key: 'preview',
        label: previewLabel,
        icon: '👁',
        onClick: () => {
          setShowPreview((v) => !v);
          focusEditorSoon();
        },
        disabled: !activeTab || activeTab.mode !== 'markdown',
      },
    ];
  }, [activeTab, createTab, toggleTabMode, askInlineChat, isAsking, showPreview, autoSaveEnabled, toggleAutoSave]);

  // Atalhos de arquivos
  useEffect(() => {
    const onKeyDown = async (e: KeyboardEvent) => {
      if (!activeTab) return;
      if (document.querySelector('.simple-modal-overlay')) return;

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
  }, [activeTab, autoSaveEnabled]);

  return (
    <div className="editor-page" ref={pageRootRef}>
      <EditorTabs />

      <Toolbar
        className="editor-page__toolbar"
        left={<div className="editor-page__title">{activeTab?.title || 'Editor'}</div>}
        center={
          <ProfilePicker
            value={editorProfileSlug}
            onChange={(slug) => setEditorProfileSlug(slug)}
            label="Perfil (editor)"
            icon="✍️"
            maxWidth="280px"
          />
        }
        actions={actions}
        ariaLabel="Barra de ferramentas do editor"
      />

      <div className="editor-page__content">
        {!activeTab ? (
          <div className="editor-page__empty">Nenhuma aba aberta</div>
        ) : activeTab.mode === 'markdown' ? (
          <div className={showPreview ? 'editor-page__split' : 'editor-page__single'}>
            <div className="editor-page__pane" role="region" aria-label="Editor Markdown">
              <div className="editor-page__pane-title">Markdown</div>
              <div className="editor-page__pane-body">
                <CodeEditor
                  height="100%"
                  language="markdown"
                  ariaLabel="Editor Markdown"
                  value={activeTab.markdown}
                  onChange={(v) => {
                    setTabMarkdown(activeTab.id, v);
                    updateLatestMarkdownForTab(activeTab.id, v);
                    if (!activeTab.filePath || autoSaveEnabled) {
                      schedulePersistForTab(activeTab.id);
                    }
                    if (activeTab.filePath && !autoSaveEnabled) {
                      setTabDirty(activeTab.id, true);
                    }
                  }}
                  placeholder="Escreva em Markdown..."
                  readOnly={isAsking}
                  onMount={(editor, monaco) => {
                    editorRef.current = editor;
                    monacoRef.current = monaco;
                  }}
                />
              </div>
            </div>

            {showPreview && (
              <div
                className="editor-page__pane"
                role="region"
                aria-label="Preview Markdown"
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

                  // Type-to-edit: ao digitar qualquer caractere "imprimível" com o diagrama focado,
                  // abre o editor e injeta o primeiro caractere no final (best-effort).
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
                <div className="editor-page__pane-title">Preview</div>
                <div className="editor-page__preview">
                  <div className="editor-page__preview-hint">
                    Dê duplo clique (ou Enter) no diagrama Mermaid para editar.
                  </div>
                  <MarkdownRenderer
                    content={debouncedMarkdownForPreview}
                    interactiveButtons={false}
                    focusableMermaid={true}
                  />
                </div>
              </div>
            )}
          </div>
        ) : (
          <div className="editor-page__single">
            <div className="editor-page__pane" role="region" aria-label="Editor rico">
              <div className="editor-page__pane-title">Rico</div>
              <div className="editor-page__pane-body">
                <RichTextEditor
                  ariaLabel="Editor rico"
                  markdown={activeTab.markdown}
                  onMarkdownChange={(md) => {
                    setTabMarkdown(activeTab.id, md);
                    updateLatestMarkdownForTab(activeTab.id, md);
                    if (!activeTab.filePath || autoSaveEnabled) {
                      schedulePersistForTab(activeTab.id);
                    }
                    if (activeTab.filePath && !autoSaveEnabled) {
                      setTabDirty(activeTab.id, true);
                    }
                  }}
                  readOnly={isAsking}
                  placeholder="Escreva…"
                  onEditorReady={(ed) => {
                    richEditorRef.current = ed;
                  }}
                  onRequestEditMermaid={(ctx) => {
                    setRichMermaidSession({
                      initialCode: String(ctx.code || ''),
                      apply: ctx.apply,
                      remove: ctx.remove,
                    });
                  }}
                />
              </div>
            </div>
          </div>
        )}
      </div>

      <EditorInlineChatModal
        isOpen={inlineChatOpen}
        title="Perguntar ao chat"
        selectedText={(inlineChatSelection as any)?.displayText || inlineChatSelection?.selectedText || ''}
        error={inlineChatError}
        focusNonce={inlineChatFocusNonce}
        onClose={closeInlineChatModal}
        onSend={sendInlineChatInstruction}
      />

      <MermaidEditorModal
        isOpen={activeMermaidIndex !== null || richMermaidSession !== null}
        title="Editar diagrama Mermaid"
        initialCode={
          activeMermaidIndex !== null
            ? mermaidInitialCode
            : richMermaidSession?.initialCode || ''
        }
        initialInsertText={activeMermaidIndex !== null ? mermaidInsertText : ''}
        onConsumeInsertText={() => {
          if (activeMermaidIndex !== null) setMermaidInsertText('');
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
    </div>
  );
}
