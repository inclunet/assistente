import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Menu } from '../components/menu';
import { MermaidEditorModal } from '../components/editor/MermaidEditorModal';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import { EditorToolbar } from '../components/editor/EditorToolbar';
import { EditorContentArea } from '../components/editor/EditorContentArea';
import { useRichEditorFlushEvents } from './useRichEditorFlushEvents';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { useEditorStore } from '../store/editorStore';
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
import { useEditorMenus } from './useEditorMenus';
import { useEditorMerge } from './useEditorMerge';
import { useEditorDocument } from './useEditorDocument';
import { useEditorPersistence } from './useEditorPersistence';
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
  const insertMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const modeMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const revealSlidePickerButtonRef = useRef<HTMLButtonElement | null>(null);
  const editorRef = useRef<MonacoCodeEditor | null>(null);
  const monacoRef = useRef<MonacoNamespace | null>(null);
  const richEditorRef = useRef<TipTapEditor | null>(null);
  const richEditorHandleRef = useRef<RichTextEditorHandle | null>(null);
  const currentRevealSlideIndexRef = useRef(0);
  const activeTabRef = useRef(activeTab);
  activeTabRef.current = activeTab;

  const [currentRevealSlideIndex, setCurrentRevealSlideIndex] = useState(0);
  const [editorReadyNonce, setEditorReadyNonce] = useState(0);

  const chatModalOpen = useWorkspaceChatModalStore((s) => s.isOpen);

  // ----- Hooks de lógica extraída -----
  const merge = useEditorMerge();
  const { mergeStateRevision, getMergeSession, updateLatestMarkdownForTab } = merge;

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
  }, [sessionLoaded, activeTab?.id, activeTab?.mode, chatModalOpen, editorReadyNonce]);

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
        if (!currentTab) return;

        if (options?.preserveFocusedField || options?.preserveExternalFocus) {
          if (isModalOpen() || useWorkspaceChatModalStore.getState().isOpen) return;

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

  const { openFile, abortMerge, saveFile, saveFileAsCopy } = useEditorFileActions({
    merge,
    activeTab,
    documents,
    fileModeByPathRef,
    flushActiveRichMarkdownNow,
    focusEditorSoon,
  });

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
    applyMermaidModal,
    removeMermaidFromModal,
  } = useMermaidSession({
    activeTab,
    richEditorHandleRef,
    setDocMarkdown,
    updateLatestMarkdownForTab,
    schedulePersistForTab,
    focusEditorSoon,
  });

  const {
    revealSlideNavigationRequest,
    revealFullscreenRequestNonce,
    revealAppendNonce,
    renderedReadingRequest,
    consumeRenderedReadingRequest,
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
    isPanelActive,
    isAsking,
    editorReadyNonce,
    richEditorRef,
    richEditorHandleRef,
    insertMenuButtonRef,
    revealSlidePickerButtonRef,
    fileModeByPathRef,
    revealToolbarDeck,
    isRevealToolbarDocument,
    currentRevealSlideIndex,
    mergeStateRevision,
    getMergeSession,
    createDocument,
    addWorkspaceTab,
    openFile,
    saveFile,
    saveFileAsCopy,
    abortMerge,
    applyInsertRequest,
    flushActiveRichMarkdownNow,
    setDocMarkdown,
    updateLatestMarkdownForTab,
    schedulePersistForTab,
    rememberCurrentExplicitSelection,
    focusEditorSoon,
    addToast,
  });

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
        onApply={applyMermaidModal}
        onRemove={removeMermaidFromModal}
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
