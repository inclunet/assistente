import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Menu } from '../components/menu';
import { MermaidEditorModal } from '../components/editor/MermaidEditorModal';
import { EditorExternalChangeDialog } from '../components/editor/EditorExternalChangeDialog';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import { EditorToolbar } from '../components/editor/EditorToolbar';
import { EditorContentArea } from '../components/editor/EditorContentArea';
import { useRichEditorFlushEvents } from './useRichEditorFlushEvents';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { useEditorStore, type EditorMode } from '../store/editorStore';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceTab } from '../store/workspaceStore';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useUIStore } from '../store/uiStore';
import { parseRevealMarkdown } from '../lib/revealMarkdown';
import { isModalOpen } from '../components/ui/Modal';
import { useInlineChatSelectionRestore } from './useInlineChatSelectionRestore';
import { useEditorSelectionSnapshots } from './useEditorSelectionSnapshots';
import { useEditorInsert } from './useEditorInsert';
import { useEditorInlineChat } from './useEditorInlineChat';
import { useEditorFileActions } from './useEditorFileActions';
import { useMermaidSession } from './useMermaidSession';
import { requestEditorMermaidCommand } from '../lib/commandEditorMermaid';
import { useEditorMenus } from './useEditorMenus';
import { useEditorMerge } from './useEditorMerge';
import { useEditorDocument } from './useEditorDocument';
import { useEditorPersistence } from './useEditorPersistence';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import { useWorkspaceCommandSurface } from '../components/workspace/useWorkspaceCommandSurface';
import { createEditorSurfaceContext } from '../lib/commandEditorSurface';
import {
  registerEditorModeSurface,
} from '../lib/commandEditorMode';
import { normalizePathKey } from '../utils/path';
import {
  EDITOR_PRESENTATION_COMMAND_IDS,
  registerEditorPresentationSurface,
  type EditorPresentationCommandID,
} from '../lib/commandEditorPresentation';
import {
  canNavigateCell,
  isEditorCellNavigationCommand,
  navigateEditorCell,
} from '../lib/commandEditorCellNavigation';
import { registerEditorFileSurface } from '../lib/commandEditorFile';
import { useEditorFormattingSurface } from './useEditorFormattingSurface';
import { useEditorMarkdownSurface } from './useEditorMarkdownSurface';
import { useEditorSlideSurface } from './useEditorSlideSurface';
import type {
  MonacoCodeEditor,
  MonacoNamespace,
  TipTapEditor,
} from './editorTypes';
import './EditorPage.css';

interface EditorPageProps {
  documentId?: string;
  workspaceTab?: WorkspaceTab;
  isPanelActive?: boolean;
}

type EditorSurfaceResource = {
  exists: boolean;
  id: string | null;
  mode: string | null;
  readOnly: boolean;
  loadError: boolean;
};

type EditorStoreSnapshot = ReturnType<typeof useEditorStore.getState>;

type CapturedRichEditorTarget = {
  kind: 'rich-editor';
  editor: TipTapEditor;
  doc: TipTapEditor['state']['doc'];
  selection: TipTapEditor['state']['selection'];
};

function isCapturedRichEditorTarget(value: unknown): value is CapturedRichEditorTarget {
  if (typeof value !== 'object' || value === null || !('kind' in value) || !('editor' in value) ||
      !('doc' in value) || !('selection' in value)) return false;
  const candidate = value as { kind?: unknown; editor?: unknown; doc?: unknown; selection?: unknown };
  return candidate.kind === 'rich-editor' && candidate.editor !== null &&
    typeof candidate.editor === 'object' && candidate.doc !== undefined && candidate.selection !== undefined;
}

function editorSurfaceResource(
  state: EditorStoreSnapshot,
  documentId: string | null,
): EditorSurfaceResource {
  const document = documentId ? state.documents[documentId] : undefined;
  return {
    exists: document !== undefined,
    id: document?.id ?? null,
    mode: document?.mode ?? null,
    readOnly: document?.readOnly === true,
    loadError: document?.loadError === true,
  };
}

function sameEditorSurfaceResource(
  left: EditorSurfaceResource,
  right: EditorSurfaceResource,
): boolean {
  return left.exists === right.exists &&
    left.id === right.id &&
    left.mode === right.mode &&
    left.readOnly === right.readOnly &&
    left.loadError === right.loadError;
}

export default function EditorPage({ documentId, workspaceTab, isPanelActive = true }: EditorPageProps = {}) {
  const addToast = useUIStore((s) => s.addToast);

  const documents = useEditorStore((s) => s.documents);
  const createDocument = useEditorStore((s) => s.createDocument);
  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const addWorkspaceTab = useWorkspaceStore((s) => s.addTab);
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);

  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const tabProfileSlug = workspaceTab?.profileOverride?.slug as string | undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || 'editor-texto';

  const currentDocumentId = documentId ?? workspaceTab?.id ?? null;
  const activeTab = currentDocumentId ? documents[currentDocumentId] ?? null : null;

  const pageRootRef = useRef<HTMLDivElement>(null);
  const fileMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const formatMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const insertMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const modeMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const revealSlidePickerButtonRef = useRef<HTMLButtonElement | null>(null);
  const fullscreenButtonRef = useRef<HTMLButtonElement | null>(null);
  const presentationInstanceIdRef = useRef(`editor-presentation-${Math.random().toString(36).slice(2)}`);
  const modeInstanceIdRef = useRef(`editor-mode-${Math.random().toString(36).slice(2)}`);
  const isComposingRef = useRef(false);
  const editorRef = useRef<MonacoCodeEditor | null>(null);
  const monacoRef = useRef<MonacoNamespace | null>(null);
  const richEditorRef = useRef<TipTapEditor | null>(null);
  const richEditorHandleRef = useRef<RichTextEditorHandle | null>(null);
  const currentRevealSlideIndexRef = useRef(0);
  const activeTabRef = useRef(activeTab);
  activeTabRef.current = activeTab;
  const isPanelActiveRef = useRef(isPanelActive);
  isPanelActiveRef.current = isPanelActive;
  const isAskingRef = useRef(false);
  const toolbarMenuVisibleRef = useRef(false);
  const flushModeRef = useRef<() => void>(() => undefined);
  const applyModeRef = useRef<(mode: EditorMode, restoreFocus?: boolean) => boolean>(() => false);

  const [currentRevealSlideIndex, setCurrentRevealSlideIndex] = useState(0);
  const [editorReadyNonce, setEditorReadyNonce] = useState(0);
  const [workspaceFocusRequestNonce, setWorkspaceFocusRequestNonce] = useState(0);
  const consumedWorkspaceFocusRequestRef = useRef(0);

  const chatModalOpen = useWorkspaceChatModalStore((s) => s.isOpen);

  // ----- Hooks de lógica extraída -----
  const merge = useEditorMerge();
  const {
    mergeStateRevision,
    getMergeSession,
    updateLatestMarkdownForTab,
    externalChangeDecision,
    resolveExternalChangeDecision,
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
  const sessionLoadedRef = useRef(sessionLoaded);
  sessionLoadedRef.current = sessionLoaded;

  const canFocusWorkspacePanelImmediately = useCallback(() => {
    if (
      !isPanelActiveRef.current
      || isModalOpen()
      || useWorkspaceChatModalStore.getState().isOpen
      || !sessionLoadedRef.current
    ) return false;
    const tab = activeTabRef.current;
    if (!tab) return false;
    if (tab.mode === 'markdown') {
      const editor = editorRef.current;
      return Boolean(editor && editor.getDomNode?.()?.isConnected);
    }
    if (tab.mode === 'rich') {
      const editor = richEditorRef.current;
      return Boolean(editor && editor.view.dom.isConnected);
    }
    if (tab.mode === 'view') {
      const anchor = pageRootRef.current?.querySelector<HTMLElement>(
        '[data-editor-rendered-anchor="true"][data-reading-active="true"]',
      );
      const documentTarget = anchor?.querySelector<HTMLElement>(
        '[data-editor-rendered-document="true"]',
      );
      return Boolean(documentTarget?.isConnected);
    }
    return false;
  }, []);

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
    if (!isPanelActive) return;
    if (!sessionLoaded) return;
    if (!activeTab) return;
    if (chatModalOpen) return;
    if (isModalOpen()) return;
    if (activeTab.mode === 'markdown' && !editorRef.current) return;

    const el = document.activeElement as HTMLElement | null;
    const tag = el?.tagName || '';
    const isTypingTarget =
      !!el &&
      (tag === 'INPUT' ||
        tag === 'TEXTAREA' ||
        el.isContentEditable ||
        el.getAttribute?.('role') === 'textbox');

    // Primeira entrada: foca o editor se nenhum campo de digitação estiver ativo.
    if (!didInitialEditorAutofocusRef.current) {
      didInitialEditorAutofocusRef.current = true;
      if (!isTypingTarget) {
        focusEditorSoon({ preserveFocusedField: true });
      }
      return;
    }

    // Mudança de aba/modo: só foca automaticamente se não houver um alvo de foco claro.
    // (Evita “puxar” o foco de tabs/toolbar, o que quebra navegação por teclado, ex: F6.)
    const isEditorZone =
      !!el &&
      (!!el.closest?.('.rich-text-editor__content') || !!el.closest?.('.monaco-editor'));
    const isDocumentBody = !el || el === document.body;

    if (!isTypingTarget && (isDocumentBody || isEditorZone)) {
      focusEditorSoon({ preserveExternalFocus: true });
    }
  }, [sessionLoaded, activeTab?.id, activeTab?.mode, chatModalOpen, editorReadyNonce, isPanelActive]);

  const { rememberCurrentExplicitSelection, getPreparedSelectionSnapshot } = useEditorSelectionSnapshots({
    activeTab,
    editorReadyNonce,
    editorRef,
    monacoRef,
    richEditorRef,
  });

  function focusEditorSoon(options?: {
    preserveFocusedField?: boolean;
    preserveExternalFocus?: boolean;
  }) {
    window.setTimeout(() => {
      try {
        const currentTab = activeTabRef.current;
        if (!currentTab || !isPanelActiveRef.current) return;
        if (isModalOpen() || useWorkspaceChatModalStore.getState().isOpen) return;

        if (options?.preserveFocusedField || options?.preserveExternalFocus) {
          const focused = document.activeElement as HTMLElement | null;
          const focusedTag = focused?.tagName || '';
          const isFocusedField =
            !!focused
            && (
              focusedTag === 'INPUT'
              || focusedTag === 'TEXTAREA'
              || focused.isContentEditable
              || focused.getAttribute?.('role') === 'textbox'
            );
          if (isFocusedField) return;

          if (options.preserveExternalFocus) {
            const isEditorZone =
              !!focused
              && (
                !!focused.closest?.('.rich-text-editor__content')
                || !!focused.closest?.('.monaco-editor')
              );
            const isDocumentBody = !focused || focused === document.body;
            if (!isDocumentBody && !isEditorZone) return;
          }
        }

        if (currentTab.mode === 'markdown') {
          editorRef.current?.focus?.();
        } else if (currentTab.mode === 'rich') {
          richEditorRef.current?.commands?.focus?.();
          richEditorRef.current?.view?.focus?.();
        }
      } catch {
        // best-effort
      }
    }, 20);
  }

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
  isAskingRef.current = isAsking;
  useEditorInsert({
    commandSurface: { root: pageRootRef, active: isPanelActive, asking: isAsking },
    activeTab, currentDocumentId, sessionLoaded, editorReadyNonce,
    editorRef, monacoRef, richEditorRef, flushActiveRichMarkdownNow,
  });
  useEditorFormattingSurface(pageRootRef, richEditorRef, currentDocumentId, isPanelActive, editorReadyNonce, isAsking);
  useEditorMarkdownSurface(pageRootRef, editorRef, monacoRef, currentDocumentId, isPanelActive, editorReadyNonce, isAsking);

  useEffect(() => {
    const onCompositionStart = () => { isComposingRef.current = true; };
    const onCompositionEnd = () => { isComposingRef.current = false; };
    window.addEventListener('compositionstart', onCompositionStart, true);
    window.addEventListener('compositionend', onCompositionEnd, true);
    return () => {
      window.removeEventListener('compositionstart', onCompositionStart, true);
      window.removeEventListener('compositionend', onCompositionEnd, true);
    };
  }, []);

  const { abortMerge, prepareFileCommand, confirmOverwrite, applyCommittedFileCommand } = useEditorFileActions({
    merge,
    activeTab,
    flushActiveRichMarkdownNow,
    focusEditorSoon,
  });

  const filePrepareRef = useRef(prepareFileCommand);
  const fileConfirmRef = useRef(confirmOverwrite);
  const fileApplyRef = useRef(applyCommittedFileCommand);
  filePrepareRef.current = prepareFileCommand;
  fileConfirmRef.current = confirmOverwrite;
  fileApplyRef.current = applyCommittedFileCommand;

  useEffect(() => {
    const root = pageRootRef.current;
    const user = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    const tabId = workspaceTab?.id;
    const documentId = activeTab?.id;
    if (!root || !user || !workspace || !tabId || !documentId || !isPanelActive) return undefined;
    const ownerId = user.userId;
    const sessionId = user.sessionId;
    const workspaceId = workspace.id;
    const instanceId = `editor-file-${tabId}-${documentId}`;
    const generation = `${tabId}:${documentId}:${activeTab?.mode ?? 'unknown'}`;
    let identityRevoked = false;
    const isCurrent = () => {
      const auth = useAuthStore.getState();
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      const currentDoc = useEditorStore.getState().documents[documentId];
      return !identityRevoked && auth.isAuthenticated && auth.user?.userId === ownerId && auth.user.sessionId === sessionId &&
        currentWorkspace?.id === workspaceId && currentWorkspace.activeTabId === tabId &&
        currentWorkspace.tabs.some((candidate) => candidate.id === tabId && candidate.type === 'editor') &&
        currentDoc?.id === documentId && isPanelActive && !isAskingRef.current;
    };
    const unregister = registerEditorFileSurface({
      root, ownerId, sessionId, workspaceId, tabId, documentId, instanceId, generation,
      isActive: isCurrent,
      isCurrent,
      canExecute: (commandID) => {
        const currentDoc = useEditorStore.getState().documents[documentId];
        return commandID === 'editor.file.open' ||
          (commandID === 'editor.file.save' && currentDoc?.readOnly !== true) ||
          (commandID === 'editor.file.save_copy' && Boolean(currentDoc?.filePath) && currentDoc?.readOnly !== true);
      },
      canCommit: () => !isComposingRef.current && !isModalOpen(),
      prepare: (commandID) => filePrepareRef.current(commandID),
      confirmOverwrite: (path) => fileConfirmRef.current(path),
      applyCommitted: (commandID, result) => fileApplyRef.current(commandID, result),
      canApplyCommittedResult: (_commandID, result) => {
        if (identityRevoked || !root.isConnected || !result || typeof result !== 'object') return false;
        const returnedTabId = (result as { tabId?: unknown }).tabId;
        const currentWorkspace = useWorkspaceStore.getState().workspace;
        const auth = useAuthStore.getState();
        return typeof returnedTabId === 'string' && returnedTabId === tabId &&
          auth.isAuthenticated && auth.user?.userId === ownerId &&
          useAuthStore.getState().user?.sessionId === sessionId &&
          currentWorkspace?.id === workspaceId &&
          currentWorkspace.activeTabId === tabId &&
          currentWorkspace.tabs.some((candidate) => candidate.id === tabId && candidate.type === 'editor');
      },
      subscribe: (onChange) => {
        const initialAuth = useAuthStore.getState();
        let previousAuth = `${initialAuth.user?.userId ?? ''}:${initialAuth.user?.sessionId ?? ''}:${initialAuth.isAuthenticated}`;
        let previousWorkspace = useWorkspaceStore.getState().workspace;
        let previousDoc = useEditorStore.getState().documents[documentId];
        const a = useAuthStore.subscribe((state) => {
          const next = `${state.user?.userId ?? ''}:${state.user?.sessionId ?? ''}:${state.isAuthenticated}`;
          if (next !== previousAuth) { previousAuth = next; identityRevoked = true; onChange(); }
        });
        const w = useWorkspaceStore.subscribe((state) => {
          const next = state.workspace;
          const changed = next?.id !== previousWorkspace?.id || next?.activeTabId !== previousWorkspace?.activeTabId ||
            next?.tabs.find((candidate) => candidate.id === tabId)?.type !== previousWorkspace?.tabs.find((candidate) => candidate.id === tabId)?.type;
          previousWorkspace = next;
          if (changed) { identityRevoked = true; onChange(); }
        });
        const e = useEditorStore.subscribe((state) => {
          const next = state.documents[documentId];
          const identityChanged = next?.id !== previousDoc?.id || next?.readOnly !== previousDoc?.readOnly || next?.mode !== previousDoc?.mode;
          const filePathChanged = next?.filePath !== previousDoc?.filePath;
          previousDoc = next;
          if (identityChanged) identityRevoked = true;
          if (identityChanged || filePathChanged) onChange();
        });
        return () => { a(); w(); e(); };
      },
    });
    return unregister;
  }, [activeTab?.id, activeTab?.mode, activeTab?.readOnly, workspaceTab?.id, isPanelActive]);

  const {
    openMermaidEditorByIndex,
    removeMermaidBlockByIndex,
    requestEditRichMermaid,
    isMermaidModalOpen,
    mermaidModalTitle,
    mermaidModalInitialCode,
    mermaidModalInitialInsertText,
    consumeMermaidInsertText,
    cancelMermaidModal,
    mermaidModalSessionKey,
    setMermaidModalId,
    updateMermaidDraft,
  } = useMermaidSession({
    activeTab,
    rootRef: pageRootRef,
    active: isPanelActive,
    asking: isAsking,
    editorRef,
    monacoRef,
    richEditorRef,
    richEditorHandleRef,
    setDocMarkdown,
    updateLatestMarkdownForTab,
    schedulePersistForTab,
  });

  const {
    notifyRevealAppend,
    revealSlideNavigationRequest,
    revealFullscreenRequestNonce,
    revealAppendNonce,
    renderedReadingRequest,
    consumeRenderedReadingRequest,
    requestRenderedReadingFocus,
    requestRevealSlideNavigation,
    createRevealSlideFromToolbar,
    requestRevealFullscreen,
    toolbarMenu,
    openToolbarMenu,
    closeToolbarMenu,
    handleToolbarMenuSelect,
    fileMenuItemsForContextMenu,
    insertMenuItemsForContextMenu,
    formatMenuItemsForContextMenu,
    modeMenuItemsForContextMenu,
    actions,
  } = useEditorMenus({
    activeTab,
    workspaceTab,
    isAsking,
    editorReadyNonce,
    richEditorRef,
    richEditorHandleRef,
    mergeStateRevision,
    getMergeSession,
    createDocument,
    addWorkspaceTab,
    abortMerge,
    rememberCurrentExplicitSelection,
    focusEditorSoon,
    addToast,
  });
  toolbarMenuVisibleRef.current = toolbarMenu.visible;
  useEditorSlideSurface({
    root: pageRootRef, rich: richEditorRef, markdown: editorRef, monaco: monacoRef,
    documentId: currentDocumentId, active: isPanelActive, ready: editorReadyNonce,
    asking: isAsking, composing: isComposingRef,
    flushRich: () => {
      if (!richEditorHandleRef.current) return false;
      richEditorHandleRef.current.flushMarkdown();
      return true;
    },
    commitRich: (id, content) => {
      setDocMarkdown(id, content);
      updateLatestMarkdownForTab(id, content);
      schedulePersistForTab(id);
    },
    appended: notifyRevealAppend,
  });

  useEffect(() => {
    if (!currentDocumentId) return;
    return registerWorkspacePanelFocus(currentDocumentId, () => {
      if (
        !isPanelActiveRef.current
        || isModalOpen()
        || useWorkspaceChatModalStore.getState().isOpen
      ) return false;
      setWorkspaceFocusRequestNonce((nonce) => nonce + 1);
      return true;
    }, () => {
      if (!canFocusWorkspacePanelImmediately()) return false;
      const tab = activeTabRef.current;
      if (!tab) return false;

      if (tab.mode === 'markdown') {
        const editor = editorRef.current;
        if (!editor) return false;
        const editorRoot = editor.getDomNode?.();
        if (!editorRoot) return false;
        editor.focus();
        const active = document.activeElement;
        return active === editorRoot || editorRoot.contains(active);
      }

      if (tab.mode === 'rich') {
        const editor = richEditorRef.current;
        const editorRoot = editor?.view?.dom;
        if (!editor || !editorRoot) return false;
        editor.view.focus();
        const active = document.activeElement;
        return active === editorRoot || editorRoot.contains(active);
      }

      if (tab.mode === 'view') {
        const readingAnchor = pageRootRef.current?.querySelector<HTMLElement>(
          '[data-editor-rendered-anchor="true"][data-reading-active="true"]',
        );
        const readingDocument = readingAnchor?.querySelector<HTMLElement>(
          '[data-editor-rendered-document="true"]',
        );
        if (!readingDocument) return false;
        readingDocument.focus();
        return document.activeElement === readingDocument;
      }

      return false;
    }, canFocusWorkspacePanelImmediately);
  }, [canFocusWorkspacePanelImmediately, currentDocumentId]);

  useEffect(() => {
    if (
      workspaceFocusRequestNonce === 0
      || consumedWorkspaceFocusRequestRef.current === workspaceFocusRequestNonce
      || !isPanelActive
      || !sessionLoaded
      || !activeTab
      || isModalOpen()
      || chatModalOpen
    ) return;

    if (activeTab.mode === 'markdown' && !editorRef.current) return;
    if (activeTab.mode === 'rich' && !richEditorRef.current) return;

    consumedWorkspaceFocusRequestRef.current = workspaceFocusRequestNonce;
    if (activeTab.mode === 'view') {
      requestRenderedReadingFocus();
    } else {
      focusEditorSoon();
    }
  }, [
    activeTab?.id,
    activeTab?.mode,
    chatModalOpen,
    editorReadyNonce,
    isPanelActive,
    requestRenderedReadingFocus,
    sessionLoaded,
    workspaceFocusRequestNonce,
  ]);

  const getEditorCommandSurface = useCallback(() => {
    const editorState = useEditorStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    const tabID = workspaceTab?.id ?? null;
    const tab = workspace?.tabs.find((candidate) => candidate.id === tabID) ?? null;
    const document = currentDocumentId ? editorState.documents[currentDocumentId] ?? null : null;
    return createEditorSurfaceContext({
      surfaceType: 'editor',
      surfaceId: tabID ?? null,
      workspaceTabId: tab?.id ?? null,
      activeTabId: workspace?.activeTabId ?? null,
      documentId: currentDocumentId,
      document,
    });
  }, [currentDocumentId, workspaceTab?.id]);

  const subscribeEditorCommandSurface = useCallback((invalidate: () => void) => {
    let previous = editorSurfaceResource(useEditorStore.getState(), currentDocumentId);
    return useEditorStore.subscribe((state) => {
      const next = editorSurfaceResource(state, currentDocumentId);
      if (sameEditorSurfaceResource(previous, next)) return;
      previous = next;
      invalidate();
    });
  }, [currentDocumentId]);

  useWorkspaceCommandSurface('editor', getEditorCommandSurface, subscribeEditorCommandSurface);

  const applyCommittedEditorMode = useCallback((nextMode: EditorMode, restoreFocus = false) => {
    const state = useEditorStore.getState();
    const current = currentDocumentId ? state.documents[currentDocumentId] ?? null : null;
    if (!current || current.id !== currentDocumentId || current.readOnly && nextMode !== 'view') return false;
    state.setDocMode(currentDocumentId, nextMode);
    if (current.filePath && (nextMode === 'markdown' || nextMode === 'rich')) {
      fileModeByPathRef.current[normalizePathKey(String(current.filePath))] = nextMode;
    }
    if (restoreFocus) {
      if (nextMode === 'view') requestRenderedReadingFocus();
      else focusEditorSoon();
    }
    return true;
  }, [currentDocumentId, focusEditorSoon, requestRenderedReadingFocus]);
  flushModeRef.current = flushActiveRichMarkdownNow;
  applyModeRef.current = applyCommittedEditorMode;

  useEffect(() => {
    const user = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    const root = pageRootRef.current;
    const tabId = workspaceTab?.id ?? null;
    const documentIdForSurface = currentDocumentId;
    const capturedMode = activeTab?.mode;
    if (!user || !workspace || !root || !tabId || !documentIdForSurface || !capturedMode) return;
    const ownerId = user.userId;
    const sessionId = user.sessionId;
    const workspaceId = workspace.id;
    const capturedResource = editorSurfaceResource(useEditorStore.getState(), documentIdForSurface);
    const currentSource = () => {
      const auth = useAuthStore.getState().user;
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      const resource = editorSurfaceResource(useEditorStore.getState(), documentIdForSurface);
      const currentTab = currentWorkspace?.tabs.find((tab) => tab.id === tabId);
      return auth?.userId === ownerId && auth.sessionId === sessionId &&
        useAuthStore.getState().isAuthenticated === true &&
        currentWorkspace?.id === workspaceId && currentWorkspace.activeTabId === tabId &&
        currentTab?.type === 'editor' && sameEditorSurfaceResource(resource, capturedResource) &&
        activeTabRef.current?.id === documentIdForSurface && isPanelActiveRef.current &&
        !isAskingRef.current;
    };
    const blocked = () => {
      if (isComposingRef.current || isModalOpen() || toolbarMenuVisibleRef.current) return true;
      if (Array.from(document.querySelectorAll<HTMLElement>('[role="menu"], [role="listbox"], [data-overlay]'))
        .some((element) => element.isConnected && !element.hidden && window.getComputedStyle(element).display !== 'none')) return true;
      const active = document.activeElement as HTMLElement | null;
      if (!active || active === document.body || active === document.documentElement) return false;
      if (root.contains(active)) return false;
      return !(active.closest('.topbar') && active.closest('button,[role="button"],input'));
    };
    return registerEditorModeSurface({
      root,
      ownerId,
      sessionId,
      workspaceId,
      tabId,
      documentId: documentIdForSurface,
      instanceId: modeInstanceIdRef.current,
      generation: `${tabId}:${documentIdForSurface}:${capturedMode}`,
      mode: capturedMode,
      readOnly: activeTab?.readOnly === true,
      isAsking: () => isAskingRef.current,
      isActive: () => isPanelActiveRef.current,
      isCurrent: currentSource,
      isBlocked: blocked,
      flushRichMarkdownNow: () => flushModeRef.current(),
      applyCommitted: (mode, restoreFocus) => applyModeRef.current(mode, restoreFocus),
      subscribe: (onChange) => {
        const unsubAuth = useAuthStore.subscribe(onChange);
        let previousWorkspace = useWorkspaceStore.getState().workspace;
        const unsubWorkspace = useWorkspaceStore.subscribe((state) => {
          const next = state.workspace;
          const previousTab = previousWorkspace?.tabs.find((tab) => tab.id === tabId);
          const nextTab = next?.tabs.find((tab) => tab.id === tabId);
          if (next?.id === previousWorkspace?.id && next?.activeTabId === previousWorkspace?.activeTabId &&
              nextTab?.type === previousTab?.type) return;
          previousWorkspace = next;
          onChange();
        });
        const unsubEditor = useEditorStore.subscribe((state) => {
          const next = editorSurfaceResource(state, documentIdForSurface);
          if (!sameEditorSurfaceResource(capturedResource, next)) onChange();
        });
        return () => { unsubAuth(); unsubWorkspace(); unsubEditor(); };
      },
    });
  }, [activeTab?.mode, activeTab?.readOnly, currentDocumentId, isPanelActive, workspaceTab?.id]);

  useEffect(() => {
    const user = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    const tabId = workspaceTab?.id ?? null;
    const documentIdForSurface = currentDocumentId;
    if (!user || !workspace || !tabId || !documentIdForSurface || !pageRootRef.current) return;

    const ownerId = user.userId;
    const sessionId = user.sessionId;
    const workspaceId = workspace.id;
    const instanceId = presentationInstanceIdRef.current;
    const generation = `${tabId}:${documentIdForSurface}:${activeTab?.mode ?? 'unknown'}`;
    const capturedResource = editorSurfaceResource(useEditorStore.getState(), documentIdForSurface);
    const isTargetAllowed = (target: EventTarget | null) => {
      const root = pageRootRef.current;
      if (!root) return false;
      const element = target instanceof Element ? target : null;
      if (element && (
        element.closest('[role="dialog"], [aria-modal="true"], [data-overlay], .modal-overlay, [role="menu"], [role="listbox"], .xterm, [role="terminal"]')
        || element.closest('[data-tab-type]:not([data-tab-type="editor"])')
      )) return false;
      if (!element || element === document.body || element === document.documentElement) return true;
      if (element.closest('.topbar') && element.closest('button,[role="button"]')) return true;
      return root.contains(element);
    };
    const buttonFor = (commandID: EditorPresentationCommandID) => {
      switch (commandID) {
        case 'editor.menu.file.open': return fileMenuButtonRef.current;
        case 'editor.menu.format.open': return formatMenuButtonRef.current;
        case 'editor.menu.insert.open': return insertMenuButtonRef.current;
        case 'editor.menu.mode.open': return modeMenuButtonRef.current;
        case 'editor.slides.open': return revealSlidePickerButtonRef.current;
        case 'editor.presentation.fullscreen': return fullscreenButtonRef.current;
        case 'editor.table.cell.next':
        case 'editor.table.cell.previous':
          return null;
        default: return null;
      }
    };
    const captureTarget = (): CapturedRichEditorTarget | undefined => {
      const editor = richEditorRef.current;
      if (!editor || editor.isDestroyed || activeTabRef.current?.mode !== 'rich') return undefined;
      return { kind: 'rich-editor', editor, doc: editor.state.doc, selection: editor.state.selection };
    };
    const isCapturedTargetCurrent = (target: unknown, commandID: EditorPresentationCommandID) => {
      if (!isEditorCellNavigationCommand(commandID)) return true;
      const capturedTarget = isCapturedRichEditorTarget(target) ? target : undefined;
      const editor = richEditorRef.current;
      return Boolean(capturedTarget && editor === capturedTarget.editor && !editor.isDestroyed &&
        editor.state.doc === capturedTarget.doc && editor.state.selection.eq(capturedTarget.selection));
    };
    const subscribeCapturedTarget = (target: unknown, onChange: () => void) => {
      if (!isCapturedRichEditorTarget(target)) return () => undefined;
      const editor = target.editor;
      const onTransaction = () => {
        if (editor.isDestroyed || editor.state.doc !== target.doc || !editor.state.selection.eq(target.selection)) onChange();
      };
      editor.on('transaction', onTransaction);
      return () => editor.off('transaction', onTransaction);
    };
    const isCurrentSource = () => {
      const auth = useAuthStore.getState().user;
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      const editor = useEditorStore.getState();
      const currentTab = currentWorkspace?.tabs.find((tab) => tab.id === tabId);
      return auth?.userId === ownerId && auth.sessionId === sessionId &&
        currentWorkspace?.id === workspaceId &&
        currentWorkspace.activeTabId === tabId && currentTab?.type === 'editor' &&
        editor.documents[documentIdForSurface]?.id === documentIdForSurface &&
        sameEditorSurfaceResource(editorSurfaceResource(editor, documentIdForSurface), capturedResource) &&
        activeTabRef.current?.id === documentIdForSurface && isPanelActiveRef.current &&
        !isAskingRef.current &&
        !isModalOpen() && useAuthStore.getState().isAuthenticated === true;
    };
    const isActiveSource = () => isPanelActiveRef.current && !isModalOpen() && isCurrentSource();
    const canOpen = (commandID: EditorPresentationCommandID, target: EventTarget | null) => {
      if (!isTargetAllowed(target)) return false;
      const visibleMenu = Array.from(document.querySelectorAll<HTMLElement>('[role="menu"], [role="listbox"], [data-overlay]'))
        .some((menu) => {
          if (!menu.isConnected || menu.hidden) return false;
          const style = window.getComputedStyle(menu);
          return style.display !== 'none' && style.visibility !== 'hidden' &&
            (menu.getClientRects().length > 0 || style.display !== 'none');
        });
      if (visibleMenu) return false;
      if (!isActiveSource()) return false;
      if (isEditorCellNavigationCommand(commandID)) {
        const editor = richEditorRef.current;
        return activeTabRef.current?.mode === 'rich' && activeTabRef.current.readOnly !== true &&
          activeTabRef.current.loadError !== true && !isComposingRef.current &&
          Boolean(editor && canNavigateCell(editor, commandID));
      }
      const button = buttonFor(commandID);
      return Boolean(button && !button.disabled);
    };

    return registerEditorPresentationSurface({
      root: pageRootRef.current,
      ownerId,
      sessionId,
      workspaceId,
      tabId,
      documentId: documentIdForSurface,
      instanceId,
      generation,
      isRouteCurrent: (routePathname) => routePathname === '/' || routePathname === '',
      allowedCommandIds: EDITOR_PRESENTATION_COMMAND_IDS,
      isActive: isActiveSource,
      isCurrent: isCurrentSource,
      captureTarget,
      isCapturedTargetCurrent,
      subscribeCapturedTarget,
      subscribe: (onChange) => {
        const unsubAuth = useAuthStore.subscribe(onChange);
        let previousWorkspace = useWorkspaceStore.getState().workspace;
        const unsubWorkspace = useWorkspaceStore.subscribe((state) => {
          const next = state.workspace;
          if (next?.id === previousWorkspace?.id && next?.activeTabId === previousWorkspace?.activeTabId &&
              next?.tabs.find((tab) => tab.id === tabId)?.type === previousWorkspace?.tabs.find((tab) => tab.id === tabId)?.type) return;
          previousWorkspace = next;
          onChange();
        });
        let previousResource = capturedResource;
        const unsubEditor = useEditorStore.subscribe((state) => {
          const next = editorSurfaceResource(state, documentIdForSurface);
          if (sameEditorSurfaceResource(previousResource, next)) return;
          previousResource = next;
          onChange();
        });
        return () => { unsubAuth(); unsubWorkspace(); unsubEditor(); };
      },
      canOpen,
      open: (commandID) => {
        if (isEditorCellNavigationCommand(commandID)) {
          const editor = richEditorRef.current;
          return Boolean(editor && canOpen(commandID, document.activeElement) && navigateEditorCell(editor, commandID));
        }
        const button = buttonFor(commandID);
        if (!button || button.disabled) return false;
        button.click();
        return true;
      },
    });
  }, [activeTab?.id, activeTab?.mode, activeTab?.readOnly, activeTab?.loadError, currentDocumentId, editorReadyNonce, isPanelActive, workspaceTab?.id]);

  return (
    <div className="editor-page" ref={pageRootRef}>
      <EditorToolbar
        activeTab={activeTab}
        isAsking={isAsking}
        richEditorRef={richEditorRef}
        shortcutRefs={{
          fileMenu: fileMenuButtonRef,
          formatMenu: formatMenuButtonRef,
          insertMenu: insertMenuButtonRef,
          modeMenu: modeMenuButtonRef,
          revealSlidePicker: revealSlidePickerButtonRef,
          fullscreen: fullscreenButtonRef,
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
        isPanelActive={isPanelActive}
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
        renderedReadingRequest={renderedReadingRequest}
        onRenderedReadingRequestConsumed={consumeRenderedReadingRequest}
        isEditorMenuOpen={toolbarMenu.visible}
        richEditorHandleRef={richEditorHandleRef}
        onRequestEditMermaid={requestEditRichMermaid}
        onOpenMermaid={openMermaidEditorByIndex}
        onRemoveMermaid={(index) => {
          void removeMermaidBlockByIndex(index);
        }}
      />

      <MermaidEditorModal
        isOpen={isMermaidModalOpen}
        title={mermaidModalTitle}
        initialCode={mermaidModalInitialCode}
        initialInsertText={mermaidModalInitialInsertText}
        onConsumeInsertText={consumeMermaidInsertText}
        onCancel={cancelMermaidModal}
        commandOwnerKey={mermaidModalSessionKey}
        onModalIdChange={setMermaidModalId}
        onDraftChange={updateMermaidDraft}
        onCommand={(action, code, shortcut) => {
          if (!activeTab) return;
          requestEditorMermaidCommand(action === 'apply' ? 'editor.mermaid.apply' : 'editor.mermaid.remove', { code }, activeTab, shortcut);
        }}
        canRemove
      />

      <EditorExternalChangeDialog
        decision={externalChangeDecision}
        onAction={resolveExternalChangeDecision}
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
