import { useCallback, useEffect, useMemo, useState } from 'react';
import { useRef, type MutableRefObject, type RefObject } from 'react';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { type MenuItem } from '../components/menu';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import type { RenderedReadingRequest } from '../components/editor/EditorContentArea';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { useEditorStore, type EditorDocument, type EditorInsertRequest, type EditorMode } from '../store/editorStore';
import { useWorkspaceStore, type WorkspaceTab } from '../store/workspaceStore';
import { normalizePathKey } from '../utils/path';
import { isModalOpen } from '../components/ui/Modal';
import {
  buildFileMenuItemsForContextMenu,
  buildFormatMenuItemsForContextMenu,
  buildInsertMenuItemsForContextMenu,
  buildModeMenuItemsForContextMenu,
} from './editorMenus';
import { EditorGetDraftPath } from '@wailsjs/go/wailsapi/Editor';
import type { AddToastFn } from './editorMenus/types';
import type { TipTapEditor } from './editorTypes';
import type { ParsedRevealDeck } from '../lib/revealMarkdown';

interface UseEditorMenusArgs {
  activeTab: EditorDocument | null;
  workspaceTab?: WorkspaceTab;
  isPanelActive: boolean;
  isAsking: boolean;
  editorReadyNonce: number;
  richEditorRef: MutableRefObject<TipTapEditor | null>;
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
  insertMenuButtonRef: MutableRefObject<HTMLButtonElement | null>;
  revealSlidePickerButtonRef: MutableRefObject<HTMLButtonElement | null>;
  fileModeByPathRef: MutableRefObject<Record<string, EditorMode>>;
  revealToolbarDeck: ParsedRevealDeck;
  isRevealToolbarDocument: boolean;
  currentRevealSlideIndex: number;
  mergeStateRevision: number;
  getMergeSession: (tabId: string) => unknown;
  createDocument: (initial?: Partial<Pick<EditorDocument, 'id' | 'title' | 'markdown' | 'mode' | 'filePath' | 'draftId'>>) => string;
  addWorkspaceTab: (type: 'editor', title: string, initialState?: Record<string, unknown>) => Promise<string>;
  openFile: () => Promise<void>;
  saveFile: () => Promise<void>;
  saveFileAsCopy: () => Promise<void>;
  abortMerge: () => Promise<void>;
  applyInsertRequest: (req: EditorInsertRequest) => Promise<boolean>;
  flushActiveRichMarkdownNow: () => void;
  setDocMarkdown: (tabId: string, markdown: string) => void;
  updateLatestMarkdownForTab: (tabId: string, markdown: string) => void;
  schedulePersistForTab: (tabId: string) => void;
  rememberCurrentExplicitSelection: () => void;
  focusEditorSoon: () => void;
  addToast: AddToastFn;
}

/**
 * Hook que concentra menus, toolbar e atalhos do editor:
 * - itens dos menus Arquivo/Inserir/Formatar/Modo (via builders de `editorMenus`);
 * - ações da toolbar (ex.: "Perguntar ao chat");
 * - navegação/criação de slides Reveal e pedido de fullscreen;
 * - atalhos de teclado globais do editor (F5, Alt+1/2/3, Alt+I/S, Ctrl+S/O...).
 */
export function useEditorMenus({
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
}: UseEditorMenusArgs) {
  const { t } = useTranslation();

  const [revealSlideNavigationRequest, setRevealSlideNavigationRequest] = useState<{ index: number; nonce: number } | null>(null);
  const [revealFullscreenRequestNonce, setRevealFullscreenRequestNonce] = useState(0);
  const [revealAppendNonce, setRevealAppendNonce] = useState(0);
  const renderedReadingRequestNonceRef = useRef(0);
  const modePersistenceQueueRef = useRef<Promise<void>>(Promise.resolve());
  const [renderedReadingRequest, setRenderedReadingRequest] = useState<RenderedReadingRequest | null>(null);
  const updateWorkspaceTab = useWorkspaceStore((state) => state.updateTab);
  const consumeRenderedReadingRequest = useCallback((nonce: number) => {
    setRenderedReadingRequest((current) => current?.nonce === nonce ? null : current);
  }, []);
  const requestRenderedReadingFocus = useCallback(() => {
    renderedReadingRequestNonceRef.current += 1;
    setRenderedReadingRequest({ nonce: renderedReadingRequestNonceRef.current });
  }, []);

  const fileMenuItems = useMemo(() => {
    // "Salvar" funciona em qualquer aba ativa: grava o arquivo quando há
    // filePath, pede destino quando é rascunho sem path, ou resolve o conflito
    // externo quando está locked (ver saveFile). Por isso fica habilitado
    // sempre que houver aba ativa — não só nos casos sem path/locked.
    const canSave = !!activeTab && !activeTab.readOnly;
    const canSaveAs = !!activeTab?.filePath && !activeTab.readOnly;
    const hasMergeSession = !!activeTab && !!getMergeSession(activeTab.id);

    const items = [
      { value: 'new', label: t('editor.menuItems.new'), sublabel: 'Ctrl+N' },
      { value: 'open', label: t('editor.menuItems.open'), sublabel: 'Ctrl+O' },
      { value: 'save', label: t('editor.menuItems.save'), sublabel: 'Ctrl+S', disabled: !canSave },
      ...(hasMergeSession
        ? [{ value: 'abort-merge', label: t('editor.menuItems.abortMerge'), sublabel: t('editor.menuItems.abortMergeHint') }]
        : []),
      { value: 'saveas', label: t('editor.menuItems.saveAs'), sublabel: 'Ctrl+Shift+S', disabled: !canSaveAs },
    ];

    return items;
    // `mergeStateRevision` força recomputo quando a merge session muda (lida
    // via ref em `getMergeSession` para o item "Abortar merge"), já que esse
    // estado não deriva de `activeTab`.
  }, [activeTab, mergeStateRevision, t]);

  const onFileMenuSelect = useCallback(
    async (value: string) => {
      const v = String(value || '').trim();
      if (!v) return;

      switch (v) {
        case 'new': {
          const draftId = (typeof crypto !== 'undefined' && crypto.randomUUID) ? crypto.randomUUID() : `editor-${Date.now()}`;
          const draftPath = String(await EditorGetDraftPath(draftId) ?? '');
          const tabId = await addWorkspaceTab('editor', t('editor.fallback.newDoc'), { filePath: draftPath, draftId });
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
    [createDocument, addWorkspaceTab, openFile, saveFile, abortMerge, saveFileAsCopy, activeTab, t]
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
    triggerElementRef: toolbarMenuTriggerRef,
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
    (nextMode: EditorMode, source: 'shortcut' | 'menu' = 'shortcut') => {
      if (!activeTab) return;

      if (activeTab.mode === 'rich' && nextMode !== 'rich') {
        flushActiveRichMarkdownNow();
      }

      useEditorStore.getState().setDocMode(activeTab.id, nextMode);
      const effectiveMode: EditorMode = activeTab.readOnly ? 'view' : nextMode;

      modePersistenceQueueRef.current = modePersistenceQueueRef.current
        .catch(() => undefined)
        .then(() => updateWorkspaceTab(activeTab.id, {
          state: { displayMode: effectiveMode },
        }))
        .catch(() => {
          addToast(t('editor.modePersistenceFailed'), 'error');
        });

      // Se for arquivo real, memoriza preferência apenas de modos de edição.
      if (activeTab.filePath && (effectiveMode === 'markdown' || effectiveMode === 'rich')) {
        fileModeByPathRef.current[normalizePathKey(String(activeTab.filePath))] = effectiveMode;
      }

      if (effectiveMode === 'view') {
        // Ao escolher o modo pelo menu, evita que o fechamento restaure o
        // gatilho depois de a ilha documental receber foco.
        if (source === 'menu') toolbarMenuTriggerRef.current = null;
        requestRenderedReadingFocus();
        return;
      }

      focusEditorSoon();
    },
    [activeTab, addToast, requestRenderedReadingFocus, t, updateWorkspaceTab]
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
        setActiveTabMode: (nextMode) => setActiveTabMode(nextMode, 'menu'),
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
        disabled: !activeTab || isAsking || activeTab.readOnly,
      },
    ];
  }, [activeTab, isAsking, addToast, t, workspaceTab?.id]);

  // Atalhos do editor
  useEffect(() => {
    if (!isPanelActive || !activeTab?.id) return;

    const onKeyDown = async (e: KeyboardEvent) => {
      if (isModalOpen()) return;
      if (
        toolbarMenu.visible
        || (e.target instanceof Element && e.target.closest('[role="menu"]'))
      ) return;

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
        if (activeTab.readOnly && key !== '3') return;

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
    activeTab?.readOnly,
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
    toolbarMenu.visible,
  ]);

  return {
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
  };
}
