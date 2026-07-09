import { logger } from '../utils/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Menu, type MenuItem } from '../components/menu';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import { MermaidEditorModal } from '../components/editor/MermaidEditorModal';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import { EditorToolbar } from '../components/editor/EditorToolbar';
import { EditorContentArea } from '../components/editor/EditorContentArea';
import { useRichEditorFlushEvents } from './useRichEditorFlushEvents';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { useEditorStore, DEFAULT_MD, type EditorMode } from '../store/editorStore';
import { useWorkspaceStore, type WorkspaceTab } from '../store/workspaceStore';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import { findMermaidFenceByIndex, removeMermaidFence, replaceMermaidFenceCode } from '../lib/mermaidFence';
import { parseRevealMarkdown } from '../lib/revealMarkdown';
import { getErrorMessage, getMaybeContent } from '../lib/editorContent';
import { composePreviewText, hasConflictMarkers } from '../lib/editorMergeUtils';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import { isModalOpen } from '../components/ui/Modal';
import {
  buildFileMenuItemsForContextMenu,
  buildFormatMenuItemsForContextMenu,
  buildInsertMenuItemsForContextMenu,
  buildModeMenuItemsForContextMenu,
} from './editorMenus';
import {
  EditorDeleteDraft,
  EditorGetDraftPath,
  EditorOpenFile,
  EditorReadDraft,
  EditorSaveFileDialog,
  EditorWriteFile,
} from '@wailsjs/go/app/App';
import { useInlineChatSelectionRestore } from './useInlineChatSelectionRestore';
import { useEditorSelectionSnapshots } from './useEditorSelectionSnapshots';
import { useEditorInsert } from './useEditorInsert';
import { useEditorInlineChat } from './useEditorInlineChat';
import { useEditorMerge } from './useEditorMerge';
import { useEditorDocument } from './useEditorDocument';
import { useEditorPersistence } from './useEditorPersistence';
import type {
  MonacoCodeEditor,
  MonacoNamespace,
  RichMermaidSession,
  TipTapEditor,
} from './editorTypes';
import './EditorPage.css';

interface EditorPageProps {
  documentId?: string;
  workspaceTab?: WorkspaceTab;
  isPanelActive?: boolean;
}

export default function EditorPage({ documentId, workspaceTab, isPanelActive = true }: EditorPageProps = {}) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const documents = useEditorStore((s) => s.documents);
  const createDocument = useEditorStore((s) => s.createDocument);
  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const renameDocument = useEditorStore((s) => s.renameDocument);
  const setDocFilePath = useEditorStore((s) => s.setDocFilePath);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const addWorkspaceTab = useWorkspaceStore((s) => s.addTab);
  const setActiveWsTab = useWorkspaceStore((s) => s.setActiveTab);
  const wsTabs = useWorkspaceStore((s) => s.workspace?.tabs);
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);

  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const tabProfileSlug = workspaceTab?.profileOverride?.slug as string | undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || 'editor-texto';

  const currentDocumentId = documentId ?? workspaceTab?.id ?? null;
  const activeTab = currentDocumentId ? documents[currentDocumentId] ?? null : null;

  const pageRootRef = useRef<HTMLDivElement>(null);
  const insertMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const modeMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const revealSlidePickerButtonRef = useRef<HTMLButtonElement | null>(null);
  const editorRef = useRef<MonacoCodeEditor | null>(null);
  const monacoRef = useRef<MonacoNamespace | null>(null);
  const richEditorRef = useRef<TipTapEditor | null>(null);
  const richEditorHandleRef = useRef<RichTextEditorHandle | null>(null);
  const currentRevealSlideIndexRef = useRef(0);

  const [currentRevealSlideIndex, setCurrentRevealSlideIndex] = useState(0);
  const [revealSlideNavigationRequest, setRevealSlideNavigationRequest] = useState<{ index: number; nonce: number } | null>(null);
  const [revealFullscreenRequestNonce, setRevealFullscreenRequestNonce] = useState(0);

  const [activeMermaidIndex, setActiveMermaidIndex] = useState<number | null>(null);
  const [mermaidInitialCode, setMermaidInitialCode] = useState('');
  const [mermaidInsertText, setMermaidInsertText] = useState('');
  const [richMermaidSession, setRichMermaidSession] = useState<RichMermaidSession | null>(null);

  const [editorReadyNonce, setEditorReadyNonce] = useState(0);
  const [revealAppendNonce, setRevealAppendNonce] = useState(0);

  const chatModalOpen = useWorkspaceChatModalStore((s) => s.isOpen);

  // Foco previsível após fechar o modal Mermaid.
  const prevMermaidOpenRef = useRef(false);
  useEffect(() => {
    const isOpen = activeMermaidIndex !== null;
    if (prevMermaidOpenRef.current && !isOpen) {
      focusEditorSoon();
    }
    prevMermaidOpenRef.current = isOpen;
  }, [activeMermaidIndex]);

  // ----- Hooks de lógica extraída -----
  const merge = useEditorMerge();
  const {
    mergeStateRevision,
    getMergeSession,
    getCachedMarkdownForTab,
    updateLatestMarkdownForTab,
    markSelfWrite,
    isExternalConflictLocked,
    setExternalConflictLocked,
    setDiskBaselineForTab,
    refreshDiskInfoForTab,
    cleanupMergeSessionForTab,
    promptResolveExternalChangeForTab,
  } = merge;

  const allDocs = useMemo(() => Object.values(documents), [documents]);

  const { sessionLoaded, fileModeByPathRef, saveEditorState } = useEditorDocument({
    merge,
    isWsInitialized,
    currentDocumentId,
    activeTab,
    allDocs,
    documents,
  });

  const flushActiveRichMarkdownNow = useCallback(() => {
    try {
      const st = useEditorStore.getState();
      const tab = currentDocumentId ? st.documents[currentDocumentId] ?? null : null;
      if (!tab || tab.mode !== 'rich') return;
      richEditorHandleRef.current?.flushMarkdown?.();
    } catch {
      // best-effort
    }
  }, [currentDocumentId]);

  const { persistTabContentNow, schedulePersistForTab, syncAssistedChangeForTab } = useEditorPersistence({
    merge,
    sessionLoaded,
    currentDocumentId,
    allDocs,
    flushActiveRichMarkdownNow,
    saveEditorState,
  });

  useRichEditorFlushEvents({ flushNow: flushActiveRichMarkdownNow });

  const debouncedMarkdownForPreview = useDebouncedValue(activeTab?.markdown || '', 120);
  const revealToolbarDeck = useMemo(
    () => parseRevealMarkdown(activeTab?.markdown || ''),
    [activeTab?.markdown]
  );
  const isRevealToolbarDocument = revealToolbarDeck.detection.kind === 'reveal' && revealToolbarDeck.slides.length > 0;

  const {
    clearPendingInlineChatEditorRestore,
    queueMarkdownEditorRestore,
    queueRichEditorRestore,
    queueEditorRestoreForInlineSelection,
  } = useInlineChatSelectionRestore({
    activeTab,
    chatModalOpen,
    editorReadyNonce,
    editorRef,
    richEditorRef,
    focusEditorSoon,
  });

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

  const { rememberCurrentExplicitSelection, getPreparedSelectionSnapshot } = useEditorSelectionSnapshots({
    activeTab,
    editorReadyNonce,
    editorRef,
    monacoRef,
    richEditorRef,
  });

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

  const { applyInsertRequest } = useEditorInsert({
    activeTab,
    currentDocumentId,
    sessionLoaded,
    editorReadyNonce,
    editorRef,
    monacoRef,
    richEditorRef,
    addWorkspaceTab,
    setDocMarkdown,
    updateLatestMarkdownForTab,
    schedulePersistForTab,
    flushActiveRichMarkdownNow,
    focusEditorSoon,
    addToast,
  });

  const { isAsking } = useEditorInlineChat({
    activeTab,
    workspaceTab,
    currentDocumentId,
    effectiveProfileSlug,
    editorReadyNonce,
    editorRef,
    richEditorRef,
    currentRevealSlideIndexRef,
    flushActiveRichMarkdownNow,
    persistTabContentNow,
    syncAssistedChangeForTab,
    setDocMarkdown,
    updateLatestMarkdownForTab,
    schedulePersistForTab,
    focusEditorSoon,
    getPreparedSelectionSnapshot,
    clearPendingInlineChatEditorRestore,
    queueMarkdownEditorRestore,
    queueRichEditorRestore,
    queueEditorRestoreForInlineSelection,
  });

  const openFile = async () => {
    try {
      const res = await EditorOpenFile();
      const path = String(res?.path || '').trim();
      if (!path) return;

      const key = normalizePathKey(path);
      const content = String(res?.content || '');

      // Se o arquivo já está aberto em outra aba, apenas ativa essa aba.
      const existingDoc = Object.values(documents).find(
        (tab) => tab.filePath && normalizePathKey(String(tab.filePath)) === key,
      );
      if (existingDoc) {
        const wsTab = (wsTabs || []).find(
          (tab) => tab.type === 'editor' && tab.id === existingDoc.id,
        );
        if (wsTab) {
          await setActiveWsTab(wsTab.id);
          addToast(t('editor.toast.fileAlreadyOpen'), 'info');
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
        // filePath+title são sincronizados pelo controller do painel de editor.
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
      const diskTab = {
        id,
        title,
        markdown: content,
        mode: preferredMode,
        filePath: path,
      };
      void refreshDiskInfoForTab(diskTab);

      fileModeByPathRef.current[key] = preferredMode === 'rich' ? 'rich' : 'markdown';

      EditorDeleteDraft(id).catch(() => null);
      addToast(t('editor.toast.fileOpened'), 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      logger.error('[EditorPage] openFile error:', e);
      addToast(getErrorMessage(e) || t('editor.toast.openFailed'), 'error');
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

    const minePreviewText = composePreviewText(mineContent, t);

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
          content: minePreviewText || '(vazio)',
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

    addToast(t('editor.toast.mergeAborted'), 'info');
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
              addToast(t('editor.toast.conflictMarkersRemain'), 'warning');
              return;
            }
            markSelfWrite(activeTab.filePath);
            await EditorWriteFile(activeTab.filePath, content);
            setDiskBaselineForTab(activeTab.id, content);
            setDocDirty(activeTab.id, false);
            void refreshDiskInfoForTab(activeTab);
            setExternalConflictLocked(activeTab.id, false);
            await cleanupMergeSessionForTab(activeTab.id);
            addToast(t('editor.toast.conflictResolvedSaved'), 'success');
            focusEditorSoon();
            return;
          }

          addToast(t('editor.toast.saveLockedExternal'), 'warning');
          void promptResolveExternalChangeForTab(activeTab.id, String(activeTab.filePath));
          return;
        }
        markSelfWrite(activeTab.filePath);
        await EditorWriteFile(activeTab.filePath, content);
        setDiskBaselineForTab(activeTab.id, content);
        setDocDirty(activeTab.id, false);
        void refreshDiskInfoForTab(activeTab);
        addToast(t('editor.toast.fileSaved'), 'success');
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

      // filePath+title são sincronizados pelo controller do painel de editor.

      void refreshDiskInfoForTab({ ...activeTab, filePath: path });

      const draftId = activeTab.draftId || activeTab.id;
      setDocDraftId(activeTab.id, null);
      await EditorDeleteDraft(draftId);

      addToast(t('editor.toast.fileSaved'), 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      logger.error('[EditorPage] saveFile error:', e);
      addToast(getErrorMessage(e) || t('editor.toast.saveFailed'), 'error');
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
      addToast(t('editor.toast.copySaved'), 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      logger.error('[EditorPage] saveAs error:', e);
      addToast(getErrorMessage(e) || t('editor.toast.saveAsFailed'), 'error');
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
      addToast(t('editor.toast.mermaidBlockGone'), 'error');
      return;
    }
    const nextMarkdown = replaceMermaidFenceCode(activeTab.markdown, fence, code);
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    addToast(t('editor.toast.mermaidUpdated'), 'success');
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
      addToast(t('editor.toast.mermaidBlockGone'), 'error');
      return;
    }

    const nextMarkdown = removeMermaidFence(activeTab.markdown, fence);
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    addToast(t('editor.toast.mermaidRemoved'), 'success');
  };

  const fileMenuItems = useMemo(() => {
    // "Salvar" funciona em qualquer aba ativa: grava o arquivo quando há
    // filePath, pede destino quando é rascunho sem path, ou resolve o conflito
    // externo quando está locked (ver saveFile). Por isso fica habilitado
    // sempre que houver aba ativa — não só nos casos sem path/locked.
    const canSave = !!activeTab;
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
    // `mergeStateRevision` força recomputo quando a merge session muda (lida
    // via ref em `getMergeSession` para o item "Abortar merge"), já que esse
    // estado não deriva de `activeTab`.
  }, [activeTab, mergeStateRevision]);

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

  const appendMarkdownToDocument = useCallback((content: string) => {
    if (!activeTab) return;
    if (activeTab.mode === 'rich') {
      flushActiveRichMarkdownNow();
    }

    const latestTab = useEditorStore.getState().documents[activeTab.id] ?? activeTab;
    const current = String(latestTab.markdown ?? '');
    const trimmedContent = String(content || '').trim();
    const currentWithoutTrailingNewlines = current.replace(/[\r\n]+$/, '');
    const hasTrailingSlideSeparator = /(^|\r?\n)\s*-{3,4}\s*$/.test(currentWithoutTrailingNewlines);
    const separator = current.trim()
      ? hasTrailingSlideSeparator
        ? '\n\n'
        : '\n\n---\n\n'
      : '';
    const nextMarkdown = `${currentWithoutTrailingNewlines}${separator}${trimmedContent}\n`;
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    setRevealAppendNonce((n) => n + 1);
  }, [activeTab, flushActiveRichMarkdownNow, setDocMarkdown, updateLatestMarkdownForTab, schedulePersistForTab]);

  const requestRevealSlideNavigation = useCallback((index: number) => {
    setRevealSlideNavigationRequest((prev) => ({
      index,
      nonce: (prev?.nonce ?? 0) + 1,
    }));
  }, []);

  const createRevealSlideFromToolbar = useCallback(() => {
    appendMarkdownToDocument(`<!-- .slide: class="content-slide" -->

## ${t('editor.presentation.newSlideTitle')}`);
  }, [appendMarkdownToDocument, t]);

  const requestRevealFullscreen = useCallback(() => {
    setRevealFullscreenRequestNonce((nonce) => nonce + 1);
  }, []);

  const showRevealSlidePicker = !!activeTab && activeTab.mode === 'rich' && isRevealToolbarDocument;
  const getRevealToolbarSlideLabel = useCallback(
    (index: number) => revealToolbarDeck.slides[index]?.label || t('editor.presentation.slideOption', { index: index + 1 }),
    [revealToolbarDeck.slides, t]
  );
  const revealSlideMenuItemsForShortcut = useMemo((): MenuItem[] => {
    if (!showRevealSlidePicker) return [];
    return [
      ...revealToolbarDeck.slides.map((_, index) => ({
        id: `reveal-slide-${index}`,
        label: getRevealToolbarSlideLabel(index),
        checked: index === Math.min(currentRevealSlideIndex, Math.max(0, revealToolbarDeck.slides.length - 1)),
        action: () => requestRevealSlideNavigation(index),
      })),
      { id: 'reveal-slide-separator', separator: true },
      {
        id: 'reveal-slide-new',
        label: t('editor.presentation.newSlide'),
        action: createRevealSlideFromToolbar,
      },
    ];
  }, [
    createRevealSlideFromToolbar,
    currentRevealSlideIndex,
    getRevealToolbarSlideLabel,
    requestRevealSlideNavigation,
    revealToolbarDeck.slides,
    showRevealSlidePicker,
    t,
  ]);

  const {
    menu: toolbarMenu,
    openForTrigger: openToolbarMenu,
    closeMenu: closeToolbarMenu,
    onSelectItem: handleToolbarMenuSelect,
  } = useAnchoredContextMenu();

  const openToolbarMenuFromShortcut = useCallback(
    (anchor: HTMLButtonElement | null, ariaLabel: string, items: MenuItem[]) => {
      if (!anchor || anchor.disabled) return false;
      anchor.focus();
      openToolbarMenu(anchor, ariaLabel, items);
      return true;
    },
    [openToolbarMenu]
  );

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
        appendMarkdownToDocument,
        focusEditorSoon,
        addToast,
      },
    });
  }, [activeTab, isAsking, editorReadyNonce, addToast, applyInsertRequest, appendMarkdownToDocument]);

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
        label: t('editor.actions.askChat'),
        icon: <MessageOutlined />,
        shortcut: 'Ctrl+Shift+I',
        onMouseDown: () => {
          rememberCurrentExplicitSelection();
        },
        onClick: async () => {
          if (isAsking) return;
          if (activeTab?.mode === 'view') {
            addToast(t('editor.chatModal.prepareNeedCodeOrRich'), 'info');
            return;
          }
          if (!workspaceTab?.id) return;
          await useWorkspaceChatModalStore.getState().requestOpen(workspaceTab.id);
        },
        disabled: !activeTab || isAsking,
      },
    ];
  }, [activeTab, isAsking, addToast, t, workspaceTab?.id]);

  // Atalhos do editor
  useEffect(() => {
    if (!isPanelActive || !activeTab?.id) return;

    const onKeyDown = async (e: KeyboardEvent) => {
      if (isModalOpen()) return;

      if (
        e.key === 'F5' &&
        !e.ctrlKey &&
        !e.shiftKey &&
        !e.altKey &&
        !e.metaKey &&
        activeTab?.mode === 'view' &&
        isRevealToolbarDocument &&
        !isAsking
      ) {
        e.preventDefault();
        e.stopPropagation();
        requestRevealFullscreen();
        return;
      }

      if (e.altKey && !e.ctrlKey && !e.metaKey) {
        const key = e.key.toLowerCase();

        if (!e.shiftKey) {
          const modesByShortcut: Record<string, EditorMode> = {
            '1': 'markdown',
            '2': 'rich',
            '3': 'view',
          };
          const modeShortcut = modesByShortcut[key];
          if (modeShortcut && !isAsking) {
            e.preventDefault();
            e.stopPropagation();
            setActiveTabMode(modeShortcut);
            return;
          }

          if (key === 'i') {
            const didOpen = openToolbarMenuFromShortcut(
              insertMenuButtonRef.current,
              t('editor.aria.insertMenu'),
              insertMenuItemsForContextMenu
            );
            if (didOpen) {
              e.preventDefault();
              e.stopPropagation();
            }
            return;
          }

          if (key === 's') {
            const didOpen = openToolbarMenuFromShortcut(
              revealSlidePickerButtonRef.current,
              t('editor.presentation.goToSlide'),
              showRevealSlidePicker ? revealSlideMenuItemsForShortcut : []
            );
            if (didOpen) {
              e.preventDefault();
              e.stopPropagation();
            }
            return;
          }
        }
      }

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
  }, [
    activeTab?.id,
    activeTab?.mode,
    isPanelActive,
    isAsking,
    isRevealToolbarDocument,
    insertMenuItemsForContextMenu,
    openFile,
    openToolbarMenuFromShortcut,
    requestRevealFullscreen,
    saveFile,
    saveFileAsCopy,
    setActiveTabMode,
    showRevealSlidePicker,
    revealSlideMenuItemsForShortcut,
    t,
  ]);

  return (
    <div className="editor-page" ref={pageRootRef}>
      <EditorToolbar
        activeTab={activeTab}
        isAsking={isAsking}
        richEditorRef={richEditorRef}
        shortcutRefs={{
          insertMenu: insertMenuButtonRef,
          modeMenu: modeMenuButtonRef,
          revealSlidePicker: revealSlidePickerButtonRef,
        }}
        actions={actions}
        onOpenMenu={openToolbarMenu}
        fileMenuItems={fileMenuItemsForContextMenu}
        formatMenuItems={formatMenuItemsForContextMenu}
        insertMenuItems={insertMenuItemsForContextMenu}
        modeMenuItems={modeMenuItemsForContextMenu}
        revealSlidePicker={{
          enabled: !!activeTab && activeTab.mode === 'rich' && isRevealToolbarDocument,
          slideCount: revealToolbarDeck.slides.length,
          currentSlideIndex: Math.min(currentRevealSlideIndex, Math.max(0, revealToolbarDeck.slides.length - 1)),
          slideLabels: revealToolbarDeck.slides.map((slide) => slide.label),
          onSelectSlide: requestRevealSlideNavigation,
          onCreateSlide: createRevealSlideFromToolbar,
        }}
        revealFullscreen={{
          enabled: !!activeTab && activeTab.mode === 'view' && isRevealToolbarDocument,
          onRequest: requestRevealFullscreen,
        }}
      />

      <EditorContentArea
        activeTab={activeTab}
        isAsking={isAsking}
        debouncedMarkdownForPreview={debouncedMarkdownForPreview}
        onMarkdownChange={(v) => {
          if (!activeTab) return;
          setDocMarkdown(activeTab.id, v);
          updateLatestMarkdownForTab(activeTab.id, v);
          schedulePersistForTab(activeTab.id);
        }}
        onMonacoMount={(editor, monaco) => {
          editorRef.current = editor as unknown as MonacoCodeEditor;
          monacoRef.current = monaco as MonacoNamespace;
          setEditorReadyNonce((n) => n + 1);
        }}
        onRichMarkdownChange={(md) => {
          if (!activeTab) return;
          setDocMarkdown(activeTab.id, md);
          updateLatestMarkdownForTab(activeTab.id, md);
          schedulePersistForTab(activeTab.id);
        }}
        onRichEditorReady={(ed) => {
          richEditorRef.current = ed;
          setEditorReadyNonce((n) => n + 1);
        }}
        onRevealSlideIndexChange={(index) => {
          currentRevealSlideIndexRef.current = index;
          setCurrentRevealSlideIndex(index);
        }}
        revealAppendNonce={revealAppendNonce}
        revealSlideNavigationRequest={revealSlideNavigationRequest}
        revealFullscreenRequestNonce={revealFullscreenRequestNonce}
        richEditorHandleRef={richEditorHandleRef}
        onRequestEditMermaid={(ctx) => {
          const mermaidBlockId = String(ctx.mermaidBlockId || '').trim();
          const api = richEditorHandleRef.current;
          setRichMermaidSession({
            mermaidBlockId,
            initialCode: String(ctx.code || ''),
            insertText: String(ctx.insertText || ''),
            apply: (nextCode: string) => {
              if (mermaidBlockId && api?.applyMermaidById?.(mermaidBlockId, nextCode)) return;
              ctx.apply(nextCode);
            },
            remove: () => {
              if (mermaidBlockId && api?.removeMermaidById?.(mermaidBlockId)) return;
              ctx.remove();
            },
          });
        }}
        onOpenMermaid={openMermaidEditorByIndex}
        onRemoveMermaid={(index) => {
          void removeMermaidBlockByIndex(index);
        }}
      />

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
            addToast(t('editor.toast.mermaidUpdated'), 'success');
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
            addToast(t('editor.toast.mermaidRemoved'), 'success');
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
