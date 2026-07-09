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
import { useEditorStore, type EditorMode } from '../store/editorStore';
import { useWorkspaceStore, type WorkspaceTab } from '../store/workspaceStore';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useUIStore } from '../store/uiStore';
import { parseRevealMarkdown } from '../lib/revealMarkdown';
import { normalizePathKey } from '../utils/path';
import { isModalOpen } from '../components/ui/Modal';
import {
  buildFileMenuItemsForContextMenu,
  buildFormatMenuItemsForContextMenu,
  buildInsertMenuItemsForContextMenu,
  buildModeMenuItemsForContextMenu,
} from './editorMenus';
import { EditorGetDraftPath } from '@wailsjs/go/app/App';
import { useInlineChatSelectionRestore } from './useInlineChatSelectionRestore';
import { useEditorSelectionSnapshots } from './useEditorSelectionSnapshots';
import { useEditorInsert } from './useEditorInsert';
import { useEditorInlineChat } from './useEditorInlineChat';
import { useEditorFileActions } from './useEditorFileActions';
import { useMermaidSession } from './useMermaidSession';
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
  const { t } = useTranslation();
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

  const [currentRevealSlideIndex, setCurrentRevealSlideIndex] = useState(0);
  const [revealSlideNavigationRequest, setRevealSlideNavigationRequest] = useState<{ index: number; nonce: number } | null>(null);
  const [revealFullscreenRequestNonce, setRevealFullscreenRequestNonce] = useState(0);

  const [editorReadyNonce, setEditorReadyNonce] = useState(0);
  const [revealAppendNonce, setRevealAppendNonce] = useState(0);

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
        onRequestEditMermaid={requestEditRichMermaid}
        onOpenMermaid={openMermaidEditorByIndex}
        onRemoveMermaid={(index) => {
          void removeMermaidBlockByIndex(index);
        }}
      />

      <MermaidEditorModal
        isOpen={isMermaidModalOpen}
        title="Editar diagrama Mermaid"
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
