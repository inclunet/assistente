import { useCallback, useMemo, useState } from 'react';
import { useRef, type MutableRefObject, type RefObject } from 'react';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { type MenuItem } from '../components/menu';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import type { RenderedReadingRequest } from '../components/editor/EditorContentArea';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { type EditorDocument, type EditorMode } from '../store/editorStore';
import type { WorkspaceTab } from '../store/workspaceStore';
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
import { editorModeCommandForMode, requestEditorModeCommand } from '../lib/commandEditorMode';
import { requestEditorFormatCommand } from '../lib/commandEditorFormatting';
import { useCommandShortcutHints } from '../lib/commandShortcutHints';

function requestFileCommand(commandID: string) {
  if (!isModalOpen()) window.dispatchEvent(new CustomEvent('commands:editor-file', { detail: { commandID }, cancelable: true }));
}

interface UseEditorMenusArgs {
  activeTab: EditorDocument | null;
  workspaceTab?: WorkspaceTab;
  isAsking: boolean;
  editorReadyNonce: number;
  richEditorRef: MutableRefObject<TipTapEditor | null>;
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
  mergeStateRevision: number;
  getMergeSession: (tabId: string) => unknown;
  createDocument: (initial?: Partial<Pick<EditorDocument, 'id' | 'title' | 'markdown' | 'mode' | 'filePath' | 'draftId'>>) => string;
  addWorkspaceTab: (type: 'editor', title: string, initialState?: Record<string, unknown>) => Promise<string>;
  abortMerge: () => Promise<void>;
  rememberCurrentExplicitSelection: () => void;
  focusEditorSoon: () => void;
  addToast: AddToastFn;
}

/**
 * Hook que concentra menus, toolbar e atalhos do editor:
 * - itens dos menus Arquivo/Inserir/Formatar/Modo (via builders de `editorMenus`);
 * - ações da toolbar (ex.: "Perguntar ao chat");
 * - navegação/criação de slides Reveal e pedido de fullscreen;
 * - atalhos de teclado globais do editor (Ctrl+S/O...).
 */
export function useEditorMenus({
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
}: UseEditorMenusArgs) {
  const { t } = useTranslation();
  const commandShortcutHint = useCommandShortcutHints('editor');

  const [revealSlideNavigationRequest, setRevealSlideNavigationRequest] = useState<{ index: number; nonce: number } | null>(null);
  const [revealFullscreenRequestNonce, setRevealFullscreenRequestNonce] = useState(0);
  const [revealAppendNonce, setRevealAppendNonce] = useState(0);
  const renderedReadingRequestNonceRef = useRef(0);
  const [renderedReadingRequest, setRenderedReadingRequest] = useState<RenderedReadingRequest | null>(null);
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
      // Ctrl+N is a contextual sequence for creating an editor tab
      // (workspace.tab.editor.create), not this local menu action. Do not
      // advertise it here until this action has a canonical command ID.
      { value: 'new', label: t('editor.menuItems.new') },
      { value: 'open', label: t('editor.menuItems.open'), sublabel: commandShortcutHint('editor.file.open') },
      { value: 'save', label: t('editor.menuItems.save'), sublabel: commandShortcutHint('editor.file.save'), disabled: !canSave },
      ...(hasMergeSession
        ? [{ value: 'abort-merge', label: t('editor.menuItems.abortMerge'), sublabel: t('editor.menuItems.abortMergeHint') }]
        : []),
      { value: 'saveas', label: t('editor.menuItems.saveAs'), sublabel: commandShortcutHint('editor.file.save_copy'), disabled: !canSaveAs },
    ];

    return items;
    // `mergeStateRevision` força recomputo quando a merge session muda (lida
    // via ref em `getMergeSession` para o item "Abortar merge"), já que esse
    // estado não deriva de `activeTab`.
  }, [activeTab, commandShortcutHint, mergeStateRevision, t]);

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
          requestFileCommand('editor.file.open');
          return;
        case 'save':
          requestFileCommand('editor.file.save');
          return;
        case 'abort-merge':
          await abortMerge();
          return;
        case 'saveas':
          requestFileCommand('editor.file.save_copy');
          return;
        default:
          return;
      }
    },
    [createDocument, addWorkspaceTab, abortMerge, activeTab, t]
  );

  const notifyRevealAppend = useCallback(() => setRevealAppendNonce(n => n + 1), []);

  const requestRevealSlideNavigation = useCallback((index: number) => {
    setRevealSlideNavigationRequest((prev) => ({
      index,
      nonce: (prev?.nonce ?? 0) + 1,
    }));
  }, []);

  const createRevealSlideFromToolbar = useCallback(() => {
    if (!activeTab || isAsking || activeTab.readOnly || activeTab.loadError || activeTab.mode === 'view') return;
    requestEditorFormatCommand('editor.slide.insert.basic', undefined, activeTab);
  }, [activeTab, isAsking]);

  const requestRevealFullscreen = useCallback(() => {
    setRevealFullscreenRequestNonce((nonce) => nonce + 1);
  }, []);


  const {
    menu: toolbarMenu,
    openForTrigger: openToolbarMenu,
    closeMenu: closeToolbarMenu,
    onSelectItem: handleToolbarMenuSelect,
  } = useAnchoredContextMenu();

  const requestModeChange = useCallback((nextMode: EditorMode) => {
    if (!activeTab || isAsking) return;
    requestEditorModeCommand(editorModeCommandForMode(nextMode));
  }, [activeTab, isAsking]);

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
        addToast,
      },
    });
  }, [activeTab, isAsking, editorReadyNonce, addToast]);

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
        setActiveTabMode: requestModeChange,
      },
    });
  }, [activeTab, isAsking, requestModeChange]);

  const actions = useMemo(() => {
    return [
      {
        key: 'ask',
        label: t('editor.actions.askChat'),
        icon: <MessageOutlined />,
        // A ação abre exatamente o chat contextual da aba; seu executor é
        // workspace.chat.open, não um atalho local paralelo.
        shortcut: commandShortcutHint('workspace.chat.open'),
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

  // Atalhos de arquivo pertencem ao mapa central; não há listener legado.

  return {
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
  };
}
