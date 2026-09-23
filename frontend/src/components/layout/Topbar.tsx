import { logger } from '../../utils/logger';
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { apidto } from '@wailsjs/go/models';
import type { Editor } from '@tiptap/core';
import { flushWorkspaceNavigation, useWorkspaceStore } from '../../store/workspaceStore';
import { useTerminalStore } from '../../store/terminalStore';
import { GetActiveWorkspace } from '@wailsjs/go/wailsapi/Workspace';
import {
  canPrepareWorkspaceChatOpen,
  prepareWorkspaceChatOpen,
  registerWorkspaceChatCommandDispatcher,
  type PreparedWorkspaceChatOpen,
} from '../../store/workspaceChatModalStore';
import { parseWorkspaceSnapshot } from '../../lib/workspaceSnapshot';
import type { BackendWorkspacePayload } from '../../store/workspaceStore';
import { useShortcutsHelpStore } from '../../store/shortcutsHelpStore';
import { useAuthStore } from '../../store/authStore';
import { useUIStore } from '../../store/uiStore';
import { isModalOpen } from '../ui/Modal';
import { useShallow } from 'zustand/shallow';
import { MenuButton, type MenuItem as MenuButtonItem, type MenuButtonRef } from './MenuButton';
import { ConnectionStatusIndicator } from './ConnectionStatusIndicator';
import { Menu, type MenuItem } from '../menu';
import { Combobox, type ComboboxItem } from '../pickers/Combobox';
import { KeyboardShortcutsHelp } from '../ui/KeyboardShortcutsHelp';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { listCommandCatalog } from '../../services/commandCatalog';
import { isCommandLayerAction } from '../../lib/commandLayerActions';
import { createCommandUIEffectGuard, type CommandUIEffect, type CommandUIEffectToken } from '../../lib/commandUIEffect';
import { createTrustedCommandContextSession, type TrustedCommandContextSession } from '../../lib/commandContextSession';
import { ReadFocusContext, ReadProfileContext } from '../../lib/commandContextProviders';
import { useCommandContextScope } from '../../lib/commandContextReact';
import { isEditableKeyboardTarget } from '../../lib/decisionMnemonic';
import { createCommandUIExecution, type CommandUIExecution } from '../../lib/commandUIExecution';
import { captureEditorFormatting, isEditorFormatCommand, type EditorFormatTarget } from '../../lib/commandEditorFormatting';
import { captureEditorMermaidTarget, captureContextualEditorMermaidTarget, isEditorMermaidMutation, EDITOR_MERMAID_COMMAND_IDS, EDITOR_MERMAID_COMMAND_EVENT, isEditorMermaidCommand, type EditorMermaidTarget, type EditorMermaidCommandRequest } from '../../lib/commandEditorMermaid';
import { captureEditorMermaidKeyboard } from '../../lib/commandEditorMermaidWails';
import { CHAT_CLEAR_COMMAND, CHAT_CLEAR_EVENT, captureChatClearTarget, executeChatClear, type ChatClearTarget } from '../../lib/commandChatClear';
import { TERMINAL_INTERRUPT_COMMAND, TERMINAL_SESSION_CLOSE_COMMAND, TERMINAL_SESSION_CREATE_COMMAND, TERMINAL_OPERATION_EVENT, captureTerminalOperationTarget, executeTerminalInterrupt, executeTerminalSessionOperation, isTerminalSessionOperationCommand, type TerminalOperationCommand, type TerminalOperationTarget } from '../../lib/commandTerminalOperation';
import { createCommandTerminalOperationWailsPort } from '../../lib/commandTerminalOperationWails';
import { PAGE_MUTATION_IDS, PAGE_MUTATION_EVENT, capturePageMutationTarget, executePageMutation, isPageMutationCommand, type PageMutationTarget, type PageMutationEventDetail, type PageMutationOutcome } from '../../lib/commandPageMutation';
import { createPageMutationWailsPort } from '../../lib/commandPageMutationWails';
import { CHAT_MESSAGING_COMMAND_IDS, CHAT_MESSAGING_COMMAND_EVENT, captureChatMessagingTarget, executeChatMessaging, isChatMessagingCommand, type ChatMessagingTarget } from '../../lib/commandChatMessaging';
import { EDITOR_CELL_NAVIGATION_COMMAND_EVENT, isEditorCellNavigationCommand } from '../../lib/commandEditorCellNavigation';
import { createCommandUIExecutionWailsPort } from '../../lib/commandUIExecutionWails';
import { createCommandVoiceInputWailsPort } from '../../lib/commandVoiceInputWails';
import { executeGlobalVoiceReservation, parseVoiceInputReservation, VOICE_INPUT_COMMAND } from '../../lib/commandVoiceInput';
import { COMMAND_NAVIGATION_EVENT, createCommandNavigationHandlers, isCommandNavigation, isCommandNavigationRoute, isCommandNavigationTextField } from '../../lib/commandNavigation';
import { createCommandBackendExecution, type CommandBackendExecution, type WorkspaceListCommandOutput } from '../../lib/commandBackendExecution';
import { createCommandBackendExecutionWailsPort } from '../../lib/commandBackendExecutionWails';
import { createContextualPaletteLayerWailsPort } from '../../lib/commandContextualPaletteLayerWails';
import { createContextualDeckLease, isContextualDeckCommand, selectContextualDeckCommand } from '../../lib/commandContextualDeck';
import { createContextualDeckLayerWailsPort } from '../../lib/commandContextualDeckLayerWails';
import { createLocalCommandKeyboard, type LocalCommandKeyboardBinding, type LocalCommandKeyboardController } from '../../lib/commandLocalKeyboard';
import { createLocalPaletteConditionResolver, createLocalPaletteConditionResolverFromParsed, parseLocalPaletteConditions, type LocalCommandPaletteVisualContext } from '../../lib/commandLocalPaletteConditions';
import { resolveLocalDeckConditionCommandFromParsed } from '../../lib/commandLocalDeckConditions';
import { acquireGlobalCommandOwnership } from '../../lib/commandGlobalOwnershipWails';
import { formatCommandKeyboardTrigger } from '../../lib/commandShortcut';
import { publishCommandShortcutHints, useCommandShortcutHints } from '../../lib/commandShortcutHints';
import { getModalRegistrySnapshot } from '../../lib/modalRegistry';
import { createCommandLocalKeyboardWailsPort } from '../../lib/commandLocalKeyboardWails';
import { isLocalUICommand } from '../../lib/commandLocalUI';
import { CHAT_PRESENTATION_COMMAND_EVENT, captureChatPickerTarget, isChatPickerCommand, type ChatPickerTargetLease } from '../../lib/commandChatPickers';
import { captureEditorPresentationTarget, isEditorPresentationCommand, type EditorPresentationTargetLease } from '../../lib/commandEditorPresentation';
import { captureLandmarkNavigationTarget, isLandmarkNavigationCommand, LANDMARK_COMMAND_EVENT } from '../../lib/commandLandmarkNavigation';
import { captureChatNavigationTarget, isChatNavigationCommand, CHAT_NAVIGATION_COMMAND_IDS, CHAT_NAVIGATION_COMMAND_EVENT, type ChatNavigationTarget } from '../../lib/commandChatNavigation';
import { capturePagePresentationTarget, isPagePresentationCommand, PAGE_PRESENTATION_COMMAND_IDS, PAGE_PRESENTATION_COMMAND_EVENT, type PagePresentationTarget } from '../../lib/commandPagePresentation';

import { captureEditorModeTarget, isEditorModeCommand, EDITOR_MODE_COMMAND_IDS, EDITOR_MODE_COMMAND_EVENT, type EditorModeTargetLease } from '../../lib/commandEditorMode';
import { captureEditorFileTarget, isEditorFileCommand, type EditorFileTargetLease } from '../../lib/commandEditorFile';
import { executeEditorFileCommand, type EditorFileBegin } from '../../lib/commandEditorFileExecution';
import { createEditorFileCommandWailsPort } from '../../lib/commandEditorFileWails';
import { captureWorkspacePanelTarget } from '../../lib/commandWorkspacePanel';
import { captureWorkspaceTabTarget } from '../../lib/commandWorkspaceTabTarget';
import { captureWorkspaceTabCloseFocus } from '../../lib/commandWorkspaceTabCloseFocus';
import { captureWorkspaceTabActivationRollbackFocus } from '../../lib/commandWorkspaceTabRollbackFocus';
import {
  captureWorkspaceTabNavigationFocus,
  captureWorkspaceTabNavigationTarget,
  resolveWorkspaceTabNavigationTarget,
  isWorkspaceTabNavigationCommand,
} from '../../lib/commandWorkspaceTabNavigation';
import {
  isWorkspaceTabMutationCommand,
  isWorkspaceTabCreateCommand,
  WORKSPACE_TAB_CREATE_COMMAND_IDS,
  WORKSPACE_TAB_CLOSE_COMMAND_ID,
  isWorkspaceMutationCommand,
  WORKSPACE_CREATE_COMMAND_ID,
  WORKSPACE_CHAT_OPEN_COMMAND_ID,
  createCommandContextualBackendExecution,
} from '../../lib/commandContextualBackendExecution';
import { createCommandWorkspaceTabWailsPort, type ContextualPaletteCommandLease } from '../../lib/commandWorkspaceTabWails';
import { isContextualPaletteCommand, isContextualPagePaletteCommand, isContextualPagePaletteSurface } from '../../lib/commandContextualPalette';
import { captureWorkspaceCreateTarget, type WorkspaceCreateTargetLease } from '../../lib/commandWorkspaceCreateTarget';
import { useWorkspaceTabCreationMenu, type WorkspaceTabCreationIntent } from '../../lib/workspaceTabCreationMenu';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  ArrowLeftOutlined,
  FolderOutlined,
  HistoryOutlined,
  ReadOutlined,
  CheckSquareOutlined,
  UserSwitchOutlined,
  ThunderboltOutlined,
  SettingOutlined,
  QuestionCircleOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  CheckOutlined,
  DownOutlined,
  KeyOutlined,
} from '@ant-design/icons';
import './Topbar.css';

const isCapturedPresentationCommand = (id: string) => isChatNavigationCommand(id) || isPagePresentationCommand(id);
const capturePresentationTarget = (path: () => string, id: string, instance?: string) => isPagePresentationCommand(id)
  ? capturePagePresentationTarget(path, id, instance) : captureChatNavigationTarget(path, id, instance);

const PAGE_TITLE_KEYS: Record<string, string> = {
  '/history': 'menu.history',
  '/memories': 'menu.memories',
  '/tasklists': 'menu.tasklists',
  '/jobs': 'menu.jobs',
  '/profiles': 'menu.profiles',
  '/settings': 'menu.settings',
  '/help': 'menu.help',
  '/about': 'menu.about',
  '/update': 'menu.about',
};

const ROUTE_IDS: Record<string, string> = {
  '/history': 'history',
  '/memories': 'memories',
  '/tasklists': 'tasklists',
  '/jobs': 'jobs',
  '/profiles': 'profiles',
  '/settings': 'settings',
  '/help': 'help',
  '/about': 'about',
  '/update': 'update',
};

const HELP_SHORTCUTS_COMMAND_ID = 'help.shortcuts.show';
const WORKSPACE_LIST_COMMAND_ID = 'workspace.list';
const WORKSPACE_PANEL_FOCUS_COMMAND_ID = 'workspace.panel.focus';
const COMMAND_TOOLBAR_SURFACE_ID = 'command-toolbar';
const PALETTE_OPEN_COMMAND_ID = 'navigation.palette.open';
const HELP_NAVIGATION_COMMAND_ID = 'navigation.help.open';

interface PendingCommandIntent {
  readonly contextualPalette?: ContextualPaletteCommandLease;
  readonly commandID: string;
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly activeTabId: string | null;
  readonly routeIdentity: string;
}

async function readPreparedWorkspaceChatConversation(
  workspaceID: string,
  tabID: string,
): Promise<string | null> {
  try {
    const snapshot = await GetActiveWorkspace();
    const parsed = parseWorkspaceSnapshot(snapshot);
    if (!parsed) return null;
    const payload = parsed.payload as BackendWorkspacePayload;
    if (payload.id !== workspaceID || payload.tabs?.active !== tabID) return null;
    const tab = payload.tabs?.items?.find((candidate) => candidate.id === tabID);
    if (!tab) return null;
    // A chat tab already owns its conversation and only needs focus. Other
    // tab surfaces receive the conversation bound by the backend after commit.
    if (tab.type === 'chat') return '';
    return typeof tab.conversation_id === 'string' && tab.conversation_id.trim()
      ? tab.conversation_id
      : null;
  } catch {
    return null;
  }
}

export function Topbar() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { pathname, search, hash, key: locationKey } = useLocation();
  const pathnameRef = useRef(pathname);
  pathnameRef.current = pathname;
  const commandRouteIdentity = JSON.stringify([pathname, search, hash, locationKey]);
  const commandRouteIdentityRef = useRef(commandRouteIdentity);
  commandRouteIdentityRef.current = commandRouteIdentity;
  const commandOwner = useAuthStore((s) => s.isAuthenticated && s.user
    ? JSON.stringify([s.user.userId, s.user.sessionId]) : null);
  const commandSurfaceRef = useRef<HTMLElement>(null);
  const commandScope = useCommandContextScope();
  const tabCreationMenu = useWorkspaceTabCreationMenu();
  const { announce } = useAnnouncer();
  const creationPresentationRef = useRef({ locale: i18n.language, t, announce });
  creationPresentationRef.current = { locale: i18n.language, t, announce };
  const addToast = useUIStore((s) => s.addToast);
  const { workspace, workspaces, switchWorkspace, renameWorkspace } = useWorkspaceStore(
    useShallow((s) => ({ workspace: s.workspace, workspaces: s.workspaces, switchWorkspace: s.switchWorkspace, renameWorkspace: s.renameWorkspace }))
  );
  const shortcutsHelpOpen = useShortcutsHelpStore((s) => s.isOpen);
  const shortcutSurface = pathname === '/' ? workspace?.tabs.find(tab => tab.id === workspace.activeTabId)?.type
    : pathname === '/tasklists' ? 'tasklists' : pathname === '/profiles' ? 'profiles' : 'toolbar';
  const shortcutHint = useCommandShortcutHints(shortcutSurface);
  const shortcutHintRef = useRef(shortcutHint);
  shortcutHintRef.current = shortcutHint;
  const openShortcutsHelp = useShortcutsHelpStore((s) => s.open);
  const closeShortcutsHelp = useShortcutsHelpStore((s) => s.close);
  const isWorkspaceRoute = pathname === '/' || pathname === '';
  const toolbarRef = useToolbarKeyboardNav();
  const menuButtonRef = useRef<MenuButtonRef>(null);
  const commandPickerButtonRef = useRef<HTMLButtonElement>(null);
  const handleOpenCommandPickerRef = useRef<(fromKeyboard?: boolean) => void>(() => undefined);
  const closeCommandPaletteRef = useRef<((action?: 'restore' | 'open-main-menu' | 'ignore') => void) | null>(null);
  const commandPaletteDismissActionRef = useRef<'restore' | 'open-main-menu' | 'ignore'>('restore');
  const commandPaletteOpenRef = useRef(false);
  const commandPaletteMenuItemsRef = useRef<MenuItem[]>([]);
  const newTabMenuIntentRef = useRef<WorkspaceTabCreationIntent | null>(null);
  const commandUIEffectGuardRef = useRef<ReturnType<typeof createCommandUIEffectGuard> | null>(null);
  const pendingCommandCatalogRef = useRef<CommandUIEffectToken | null>(null);
  const commandPickerMountedRef = useRef(false);
  const commandPickerAfterMainMenuRef = useRef(false);
  const pendingCommandExecutionRef = useRef<PendingCommandIntent | null>(null);
  // Mantém o alvo capturado pelo menu até o fechamento/dispatch. O executor
  // reutiliza esta lease; não recapturamos uma origem depois do timeout do menu.
  const pendingWorkspaceCreateTargetRef = useRef<WorkspaceCreateTargetLease | null>(null);
  const workspaceChatOpenLeaseRef = useRef<PreparedWorkspaceChatOpen | null>(null);
  const workspaceChatPreparationRef = useRef<{
    generation: number;
    tabID: string;
    routeIdentity: string;
    activeElement: Element | null;
  } | null>(null);
  const executePendingCommandRef = useRef<(() => void) | null>(null);
  const activeCommandIntentRef = useRef<PendingCommandIntent | null>(null);
  const workspacePickerIntentRef = useRef<PendingCommandIntent | null>(null);
  const openWorkspacePickerRef = useRef<((trigger: HTMLElement, ariaLabel: string, items: MenuItem[]) => void) | null>(null);
  const commandContextualExecutionRef = useRef<CommandUIExecution | null>(null);
  const commandCancellationGenerationRef = useRef(0);
  const commandBackendExecutionRef = useRef<CommandBackendExecution | null>(null);
  const localCommandKeyboardRef = useRef<LocalCommandKeyboardController | null>(null);
  const localCommandExecutionRef = useRef<CommandBackendExecution | null>(null);
  const localCommandContextualExecutionRef = useRef<CommandUIExecution | null>(null);
  const localPaletteCommandsRef = useRef<ReadonlySet<string>>(new Set());
  const contextualPaletteCommandsRef = useRef<ReadonlySet<string> | null>(new Set());
  const contextualPaletteResolverRef = useRef<ReturnType<typeof createLocalPaletteConditionResolver>>(() => false);
  const contextualPaletteExecutorsRef = useRef(new Set<CommandUIExecution>());
  const localPaletteConditionResolverRef = useRef<(commandId: string, context: LocalCommandPaletteVisualContext | null | undefined) => boolean>(() => false);
  const localPaletteDeadlineRef = useRef(0);
  const localPaletteTrustedSessionRef = useRef<TrustedCommandContextSession | null>(null);
  const localPaletteProfileRevisionRef = useRef(0);
  const localPaletteSourceRef = useRef<{
    ownerId: string;
    sessionId: string;
    workspaceId: string;
    routeIdentity: string;
    surfaceLease: object | null;
    surfaceType: string;
    surfaceId: string;
    snapshotVersion: string;
    profile?: string;
    profileRevision: number;
    generation: string;
    activeTabId?: string;
    explicitVisualOrigin: boolean;
    explicitWorkspaceOrigin: boolean;
  } | null>(null);
  const hasCurrentLocalKeyboardMapCommand = useCallback((id: string) => {
    if (localPaletteDeadlineRef.current !== 0 && Date.now() >= localPaletteDeadlineRef.current) return false;
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    const owner = localKeyboardOwnerRef.current;
    return !!(auth.isAuthenticated && auth.user && currentWorkspace && owner &&
      owner.ownerId === auth.user.userId && owner.sessionId === auth.user.sessionId &&
      owner.workspaceId === currentWorkspace.id && localPaletteCommandsRef.current.has(id));
  }, []);
  const hasCurrentLocalPaletteCommand = useCallback((id: string, resolver?: ReturnType<typeof createLocalPaletteConditionResolver>) => {
    if (localPaletteDeadlineRef.current !== 0 && Date.now() >= localPaletteDeadlineRef.current) return false;
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    const owner = localKeyboardOwnerRef.current;
    if (!auth.isAuthenticated || !auth.user || !currentWorkspace || !owner ||
        owner.ownerId !== auth.user.userId || owner.sessionId !== auth.user.sessionId || owner.workspaceId !== currentWorkspace.id) return false;
    if (!resolver && localPaletteCommandsRef.current.has(id)) return true;
    const source = localPaletteSourceRef.current;
    const session = localPaletteTrustedSessionRef.current;
    if (!source || !session || source.routeIdentity !== commandRouteIdentityRef.current ||
        source.generation !== localKeyboardGenerationRef.current ||
        source.profileRevision !== localPaletteProfileRevisionRef.current ||
        source.workspaceId !== currentWorkspace.id ||
        (source.activeTabId !== undefined && currentWorkspace.activeTabId !== source.activeTabId) ||
        !document.hasFocus()) return false;
    const current = session.readOwnedCommandContextFrame(source.surfaceId);
    const surface = current?.frame.surface;
    if (!current || current.owner.userId !== source.ownerId || current.owner.sessionId !== source.sessionId ||
        current.owner.workspaceId !== source.workspaceId || current.surfaceLease !== source.surfaceLease ||
        !surface || surface.surfaceId !== source.surfaceId || surface.surfaceType !== source.surfaceType ||
        surface.snapshotVersion !== source.snapshotVersion || !current.frame.focus.hasFocus ||
        current.frame.profile?.slug !== source.profile) return false;
    // Durable palette conditions cannot use the toolbar as a substitute for a
    // missing workspace provider. Recheck epochs after the provider read too.
    if (resolver) {
      if (isCommandLayerAction(id) && !source.explicitWorkspaceOrigin) return false;
      if (isContextualPagePaletteCommand(id) && !isContextualPagePaletteSurface(id, source.surfaceType)) return false;
      const finalAuth = useAuthStore.getState();
      const finalWorkspace = useWorkspaceStore.getState().workspace;
      const finalOwner = localKeyboardOwnerRef.current;
      if (!source.explicitVisualOrigin || localPaletteSourceRef.current !== source ||
          localPaletteTrustedSessionRef.current !== session ||
          source.generation !== localKeyboardGenerationRef.current ||
          source.routeIdentity !== commandRouteIdentityRef.current ||
          source.profileRevision !== localPaletteProfileRevisionRef.current ||
          (localPaletteDeadlineRef.current !== 0 && Date.now() >= localPaletteDeadlineRef.current) ||
          !finalAuth.isAuthenticated || finalAuth.user?.userId !== source.ownerId ||
          finalAuth.user.sessionId !== source.sessionId || finalWorkspace?.id !== source.workspaceId ||
          (isCommandLayerAction(id) && finalWorkspace.activeTabId !== source.surfaceId) ||
          finalOwner?.ownerId !== source.ownerId || finalOwner.sessionId !== source.sessionId ||
          finalOwner.workspaceId !== source.workspaceId || !document.hasFocus()) return false;
    }
    return (resolver ?? localPaletteConditionResolverRef.current)(id, {
      surfaceType: source.surfaceType,
      surfaceId: source.surfaceId,
      ...(source.profile !== undefined ? { profile: source.profile } : {}),
    });
  }, []);
  const contextualPaletteAvailable = useCallback((id: string) => {
    if (!isContextualPaletteCommand(id)) return true;
    const ids = contextualPaletteCommandsRef.current;
    return ids !== null && (!ids.has(id) || hasCurrentLocalPaletteCommand(id, contextualPaletteResolverRef.current));
  }, [hasCurrentLocalPaletteCommand]);
  const captureContextualPaletteLease = useCallback((commandId: string): ContextualPaletteCommandLease | undefined => {
    const source = localPaletteSourceRef.current;
    const resolver = contextualPaletteResolverRef.current;
    if (!source || !contextualPaletteCommandsRef.current?.has(commandId) || !hasCurrentLocalPaletteCommand(commandId, resolver)) return undefined;
    const isCurrent = () => localPaletteSourceRef.current === source && contextualPaletteResolverRef.current === resolver &&
      hasCurrentLocalPaletteCommand(commandId, resolver);
    let nativeContinuationPrepared = false;
    return Object.freeze({
      commandId, generation: source.generation,
      observed: Object.freeze({ surfaceId: source.surfaceId, surfaceType: source.surfaceType,
        ...(source.profile !== undefined ? { profile: source.profile } : {}) }),
      isCurrent,
      ...(isEditorFileCommand(commandId) ? { prepareNativeFileContinuation: () => {
        if (nativeContinuationPrepared || !isCurrent()) return undefined;
        const session = localPaletteTrustedSessionRef.current;
        const deadline = localPaletteDeadlineRef.current;
        if (!session) return undefined;
        nativeContinuationPrepared = true;
        // Native UI may invalidate the map. It cannot rebase the source lease;
        // host configuration/version validation remains owned by the backend.
        return () => {
          const scalarCurrent = () => {
            const auth = useAuthStore.getState();
            const workspace = useWorkspaceStore.getState().workspace;
            return auth.isAuthenticated && auth.user?.userId === source.ownerId &&
              auth.user.sessionId === source.sessionId && workspace?.id === source.workspaceId &&
              (source.activeTabId === undefined || workspace.activeTabId === source.activeTabId) &&
              commandRouteIdentityRef.current === source.routeIdentity &&
              localPaletteProfileRevisionRef.current === source.profileRevision &&
              (deadline === 0 || Date.now() < deadline) && document.hasFocus();
          };
          if (!scalarCurrent()) return false;
          const current = session.readOwnedCommandContextFrame(source.surfaceId);
          return !!current && current.owner.userId === source.ownerId && current.owner.sessionId === source.sessionId &&
            current.owner.workspaceId === source.workspaceId && current.surfaceLease === source.surfaceLease &&
            current.frame.surface?.surfaceId === source.surfaceId && current.frame.surface.surfaceType === source.surfaceType &&
            current.frame.surface.snapshotVersion === source.snapshotVersion && current.frame.profile?.slug === source.profile &&
            current.frame.focus.hasFocus && scalarCurrent();
        };
      } } : {}),
    });
  }, [hasCurrentLocalPaletteCommand]);
  const localKeyboardGenerationRef = useRef<string | null>(null);
  const localKeyboardOwnerRef = useRef<{ ownerId: string; sessionId: string; workspaceId: string } | null>(null);
  const localNavigationFocusRef = useRef<ReturnType<typeof captureWorkspaceTabNavigationFocus> | null>(null);
  const rollbackFocusRef = useRef<ReturnType<typeof captureWorkspaceTabActivationRollbackFocus> | null>(null);
  const localKeyboardMapReadyRef = useRef<Promise<void> | null>(null);
  const paletteSurfaceTargetRef = useRef<Pick<ChatPickerTargetLease, 'isCurrent' | 'canOpen' | 'open' | 'dispose'> | undefined>(undefined);
  const paletteLandmarkTargetRef = useRef<ReturnType<typeof captureLandmarkNavigationTarget>>(undefined);
  const palettePresentationCommandsRef = useRef(new Map<string, ChatNavigationTarget | PagePresentationTarget>());
  const pointerPresentationCommandsRef = useRef<Map<string, ChatNavigationTarget | PagePresentationTarget> | undefined>(undefined);
  const pointerLandmarkTargetRef = useRef<{ target: ReturnType<typeof captureLandmarkNavigationTarget> } | undefined>(undefined);
  const paletteEditorModeTargetsRef = useRef(new Map<string, EditorModeTargetLease>());
  const paletteEditorFileTargetRef = useRef<EditorFileTargetLease | null>(null);
  const paletteEditorFormatTargetRef = useRef<EditorFormatTarget | null>(null);
  const pendingEditorFormatTargetRef = useRef<EditorFormatTarget | null>(null);
  const activeEditorFormatTargetRef = useRef<EditorFormatTarget | null>(null);
  const editorFormatBusyRef = useRef(false);
  const paletteMermaidRef = useRef(new Map<string, EditorMermaidTarget>());
  const pendingMermaidRef = useRef<EditorMermaidTarget | null>(null);
  const activeMermaidRef = useRef<EditorMermaidTarget | null>(null);
  const mermaidBusyRef = useRef(false);
  const mermaidDeckTicketsRef = useRef(new Set<string>());
  const globalVoiceTicketsRef = useRef(new Set<string>());
  const pendingEditorFileTargetRef = useRef<EditorFileTargetLease | null>(null);
  const editorFileBusyRef = useRef(false);
  const pendingEditorModeTargetRef = useRef<EditorModeTargetLease | null>(null);
  const activeEditorModeTargetRef = useRef<EditorModeTargetLease | null>(null);
  const clearPaletteEditorModeTargets = useCallback(() => {
    pointerPresentationCommandsRef.current?.forEach(target => target.dispose());
    pointerPresentationCommandsRef.current = undefined;
    palettePresentationCommandsRef.current.forEach(target => target.dispose());
    palettePresentationCommandsRef.current.clear();
    pointerLandmarkTargetRef.current?.target?.dispose();
    pointerLandmarkTargetRef.current = undefined;
    paletteLandmarkTargetRef.current?.dispose();
    paletteLandmarkTargetRef.current = undefined;
    paletteMermaidRef.current.forEach(target => target.dispose());
    paletteMermaidRef.current.clear();
    paletteEditorModeTargetsRef.current.forEach(target => target.dispose());
    paletteEditorModeTargetsRef.current.clear();
    paletteEditorFileTargetRef.current?.dispose();
    paletteEditorFileTargetRef.current = null;
    paletteEditorFormatTargetRef.current?.dispose();
    paletteEditorFormatTargetRef.current = null;
  }, []);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const [commandPaletteItems, setCommandPaletteItems] = useState<ComboboxItem[]>([]);
  commandPaletteOpenRef.current = commandPaletteOpen;
  const pickerButtonRef = useRef<HTMLButtonElement>(null);
  const workspacePickerTriggerRef = useRef<HTMLElement | null>(null);
  const renameInputRef = useRef<HTMLInputElement>(null);
  const commandNavigationHandlers = useMemo(() => {
    const handlers = new Map(createCommandNavigationHandlers(navigate));
    handlers.set('navigation.menu.open', () => {
      if (isModalOpen()) return undefined;
      if (!commandPaletteOpenRef.current) {
        menuButtonRef.current?.toggleMenu();
        return undefined;
      }
      closeCommandPaletteRef.current?.('open-main-menu');
      return undefined;
    });
    return handlers;
  }, [navigate]);
  const commandUIHandlers = useMemo((): Map<string, CommandUIEffect> => new Map([
    [HELP_SHORTCUTS_COMMAND_ID, () => { useShortcutsHelpStore.getState().open(); return undefined; }],
    [PALETTE_OPEN_COMMAND_ID, () => {
      if (!commandPaletteOpenRef.current) handleOpenCommandPickerRef.current(true);
      return undefined;
    }],
    ...commandNavigationHandlers,
    // O efeito real é preparado a partir do painel atualmente montado.
    [WORKSPACE_PANEL_FOCUS_COMMAND_ID, () => {
      throw new Error('workspace.panel.focus requires a prepared effect');
    }],
  ]), [commandNavigationHandlers]);

  const workspacePanelFocusAvailable = useCallback(() => {
    const target = captureWorkspacePanelTarget(() => pathnameRef.current);
    if (!target) return false;
    try {
      return target.isCurrent();
    } finally {
      target.dispose();
    }
  }, []);

  const workspaceTabNavigationAvailable = useCallback((commandID: string) => {
    if (!isWorkspaceTabNavigationCommand(commandID)) return true;
    const target = captureWorkspaceTabNavigationTarget(() => pathnameRef.current, commandID);
    if (!target) return false;
    try { return target.isCurrent(); } finally { target.dispose(); }
  }, []);

  const chatPickerAvailable = useCallback((commandID: string, eventTarget?: EventTarget | null): boolean => {
    if (!isChatPickerCommand(commandID)) return false;
    const target = captureChatPickerTarget(() => pathnameRef.current);
    if (!target) return false;
    try { return target.isCurrent() && target.canOpen(commandID, eventTarget); }
    finally { target.dispose(); }
  }, []);

  const editorPresentationAvailable = useCallback((commandID: string, eventTarget?: EventTarget | null): boolean => {
    if (!isEditorPresentationCommand(commandID)) return false;
    const target = captureEditorPresentationTarget(() => pathnameRef.current);
    if (!target) return false;
    try { return target.isCurrent() && target.canOpen(commandID, eventTarget); }
    finally { target.dispose(); }
  }, []);

  const executeLocalUICommand = useCallback((commandID: string, allowModalHelp = false, chatTarget?: Pick<ChatPickerTargetLease, 'isCurrent' | 'canOpen' | 'open' | 'dispose'>): boolean => {
    const isHelpCommand = commandID === HELP_NAVIGATION_COMMAND_ID;
    const isChatPicker = isChatPickerCommand(commandID);
    const isEditorPresentation = isEditorPresentationCommand(commandID);
    const isLandmark = isLandmarkNavigationCommand(commandID);
    const isCapturedPresentation = isCapturedPresentationCommand(commandID);
    if (!isLocalUICommand(commandID) || (!(allowModalHelp && isHelpCommand) && !isChatPicker && !isLandmark && !isCapturedPresentation && isModalOpen()) || !document.hasFocus()) return false;
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    const mapOwner = localKeyboardOwnerRef.current;
    if (!auth.isAuthenticated || !auth.user || !currentWorkspace || !mapOwner ||
        mapOwner.ownerId !== auth.user.userId || mapOwner.sessionId !== auth.user.sessionId ||
        mapOwner.workspaceId !== currentWorkspace.id || localKeyboardGenerationRef.current === null) return false;
    const isTabNavigation = isWorkspaceTabNavigationCommand(commandID);
    const focusSnapshot = ReadFocusContext();
    if (!focusSnapshot.hasFocus || focusSnapshot.detached || !focusSnapshot.control || focusSnapshot.control.capabilities.disabled) return false;
    const unknownCompositionAllowed = ((isTabNavigation || isChatPicker || isCommandNavigation(commandID) || commandUIHandlers.has(commandID)) &&
      isCommandNavigationTextField(document.activeElement)) ||
      (isCommandNavigation(commandID) && focusSnapshot.control.capabilities.button) ||
      (isEditorPresentation && editorPresentationAvailable(commandID)) || commandID === 'editor.mermaid.open' || isLandmark || isCapturedPresentation;
    if (focusSnapshot.composition === 'active' ||
        (focusSnapshot.composition !== 'inactive' && !unknownCompositionAllowed)) return false;
    if (commandID === 'editor.mermaid.open') {
      const target = pendingMermaidRef.current ?? captureEditorMermaidTarget(commandID);
      pendingMermaidRef.current = null;
      try { return target?.isCurrent() === true && target.canExecute(commandID) && target.execute(commandID); }
      finally { target?.dispose(); }
    }
    if (isChatPicker || isEditorPresentation || isLandmark || isCapturedPresentation) {
      const target = chatTarget ?? (isCapturedPresentation ? capturePresentationTarget(() => pathnameRef.current, commandID) : isLandmark ? captureLandmarkNavigationTarget(() => pathnameRef.current) : isChatPicker
        ? captureChatPickerTarget(() => pathnameRef.current)
        : captureEditorPresentationTarget(() => pathnameRef.current));
      if (!target) return false;
      try { return target.isCurrent() && target.canOpen(commandID) && target.open(commandID); }
      finally { if (!chatTarget) target.dispose(); }
    }
    if (commandID === WORKSPACE_PANEL_FOCUS_COMMAND_ID) {
      const target = captureWorkspacePanelTarget(() => pathnameRef.current);
      if (!target) return false;
      try {
        if (!target.isCurrent()) return false;
        target.focus();
        return true;
      } finally { target.dispose(); }
    }
    if (commandID === HELP_SHORTCUTS_COMMAND_ID) {
      useShortcutsHelpStore.getState().open();
      return true;
    }
    if (isCommandNavigation(commandID) || commandUIHandlers.has(commandID)) {
      const handler = commandUIHandlers.get(commandID);
      if (!handler) return false;
      handler();
      return true;
    }
    const workspaceState = useWorkspaceStore.getState();
    if (!isWorkspaceTabNavigationCommand(commandID)) return false;
    const target = resolveWorkspaceTabNavigationTarget(currentWorkspace, commandID);
    if (!target) return false;
    rollbackFocusRef.current?.dispose();
    rollbackFocusRef.current = null;
    localNavigationFocusRef.current?.dispose();
    localNavigationFocusRef.current = null;
    const focus = captureWorkspaceTabNavigationFocus(() => pathnameRef.current, commandID);
    if (!focus) return false;
    workspaceState.setActiveTab(target.id);
    focus.apply();
    localNavigationFocusRef.current = focus;
    return true;
  }, [commandNavigationHandlers, commandUIHandlers, editorPresentationAvailable]);

  const landmarkAvailable = useCallback((commandID: string) => {
    if (!isLandmarkNavigationCommand(commandID)) return false;
    const target = captureLandmarkNavigationTarget(() => pathnameRef.current);
    try { return target?.isCurrent() === true && target.canOpen(commandID); }
    finally { target?.dispose(); }
  }, []);

  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandId?: unknown }>).detail;
      if (typeof detail?.commandId !== 'string' || !isCommandNavigationRoute(detail.commandId) ||
          !hasCurrentLocalKeyboardMapCommand(detail.commandId)) return;
      if (executeLocalUICommand(detail.commandId)) event.preventDefault();
    };
    window.addEventListener(COMMAND_NAVIGATION_EVENT, request);
    return () => window.removeEventListener(COMMAND_NAVIGATION_EVENT, request);
  }, [executeLocalUICommand, hasCurrentLocalKeyboardMapCommand]);

  const presentationCommandAvailable = useCallback((commandID: string) => {
    if (!isCapturedPresentationCommand(commandID)) return false;
    const target = capturePresentationTarget(() => pathnameRef.current, commandID);
    try { return target?.isCurrent() === true && target.canOpen(commandID); }
    finally { target?.dispose(); }
  }, []);

  const capturePresentationCommands = useCallback(() => {
    const targets = new Map<string, ChatNavigationTarget | PagePresentationTarget>();
    for (const commandID of [...CHAT_NAVIGATION_COMMAND_IDS, ...PAGE_PRESENTATION_COMMAND_IDS]) {
      const target = capturePresentationTarget(() => pathnameRef.current, commandID);
      if (target) targets.set(commandID, target);
    }
    return targets;
  }, []);

  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandID?: unknown; instanceId?: unknown }>).detail;
      if (!detail || typeof detail.commandID !== 'string' || !isCapturedPresentationCommand(detail.commandID) ||
          typeof detail.instanceId !== 'string' || !detail.instanceId || !hasCurrentLocalKeyboardMapCommand(detail.commandID)) return;
      const target = capturePresentationTarget(() => pathnameRef.current, detail.commandID, detail.instanceId);
      if (!target) return;
      try { if (executeLocalUICommand(detail.commandID, false, target)) event.preventDefault(); }
      finally { target.dispose(); }
    };
    window.addEventListener(CHAT_NAVIGATION_COMMAND_EVENT, request);
    window.addEventListener(PAGE_PRESENTATION_COMMAND_EVENT, request);
    return () => {
      window.removeEventListener(CHAT_NAVIGATION_COMMAND_EVENT, request);
      window.removeEventListener(PAGE_PRESENTATION_COMMAND_EVENT, request);
    };
  }, [executeLocalUICommand, hasCurrentLocalKeyboardMapCommand]);

  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandID?: unknown; instanceId?: unknown }>).detail;
      if (!detail || typeof detail.commandID !== 'string' || !isLandmarkNavigationCommand(detail.commandID) ||
          typeof detail.instanceId !== 'string' || !detail.instanceId || !hasCurrentLocalKeyboardMapCommand(detail.commandID)) return;
      const target = captureLandmarkNavigationTarget(() => pathnameRef.current, detail.instanceId);
      if (!target) return;
      try { if (executeLocalUICommand(detail.commandID, false, target)) event.preventDefault(); }
      finally { target.dispose(); }
    };
    window.addEventListener(LANDMARK_COMMAND_EVENT, request);
    return () => window.removeEventListener(LANDMARK_COMMAND_EVENT, request);
  }, [executeLocalUICommand, hasCurrentLocalKeyboardMapCommand]);

  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandID?: unknown; instanceId?: unknown }>).detail;
      if (!detail || typeof detail.commandID !== 'string' || !isChatPickerCommand(detail.commandID) ||
          typeof detail.instanceId !== 'string' || !detail.instanceId ||
          !hasCurrentLocalKeyboardMapCommand(detail.commandID)) return;
      const target = captureChatPickerTarget(() => pathnameRef.current, detail.instanceId);
      if (!target) return;
      try {
        if (executeLocalUICommand(detail.commandID, false, target)) event.preventDefault();
      } finally { target.dispose(); }
    };
    window.addEventListener(CHAT_PRESENTATION_COMMAND_EVENT, request);
    return () => window.removeEventListener(CHAT_PRESENTATION_COMMAND_EVENT, request);
  }, [executeLocalUICommand, hasCurrentLocalKeyboardMapCommand]);

  const prepareWorkspaceTabTarget = useCallback((_commandID: string) =>
    captureWorkspaceTabTarget(() => pathnameRef.current), []);

  const editorFileAvailable = useCallback((commandID: string) => {
    if (editorFileBusyRef.current || (pathnameRef.current !== '/' && pathnameRef.current !== '')) return false;
    const target = captureEditorFileTarget();
    try { return target?.canExecute(commandID) === true; }
    finally { target?.dispose(); }
  }, []);

  const paletteChatClearRef = useRef<ChatClearTarget | null>(null);
  const paletteChatMessagingRef = useRef(new Map<string, ChatMessagingTarget>());
  const pendingChatMessagingRef = useRef<ChatMessagingTarget | null>(null);
  const activeChatMessagingRef = useRef(new Set<ChatMessagingTarget>());
  const clearPaletteChatMessaging = useCallback(() => {
    paletteChatMessagingRef.current.forEach(target => target.dispose());
    paletteChatMessagingRef.current.clear();
  }, []);
  const chatMessagingAvailable = useCallback((commandID: string, keyboardTarget?: EventTarget | null) => {
    if (!isChatMessagingCommand(commandID) || !document.hasFocus() ||
        ReadFocusContext().composition === 'active') return false;
    const target = captureChatMessagingTarget(() => pathnameRef.current, commandID, undefined, keyboardTarget);
    try { return target?.canCommit() === true; } finally { target?.dispose(); }
  }, []);
  const runChatMessaging = useCallback(async (target: ChatMessagingTarget, begin?: () => Promise<EditorFileBegin>, lease?: ContextualPaletteCommandLease) => {
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    const route = commandRouteIdentityRef.current;
    const owner = auth.user;
    const ownerId = owner?.userId;
    const sessionId = owner?.sessionId;
    const workspaceId = workspace?.id;
    const tabId = workspace?.activeTabId;
    let invalidated = false;
    const port = createCommandWorkspaceTabWailsPort({ contextualPalette: lease });
    const current = () => {
      const now = useAuthStore.getState();
      const nextWorkspace = useWorkspaceStore.getState().workspace;
      const valid = !invalidated && now.isAuthenticated && now.user?.userId === ownerId && now.user?.sessionId === sessionId &&
        nextWorkspace?.id === workspaceId && nextWorkspace?.activeTabId === tabId &&
        commandRouteIdentityRef.current === route && target.isCurrent();
      if (!valid) invalidated = true;
      return valid;
    };
    if (!auth.isAuthenticated || !owner || !workspace || !target.canCommit() ||
        !document.hasFocus() || ReadFocusContext().composition === 'active' ||
        [...activeChatMessagingRef.current].some(active => active.instanceId === target.instanceId && active.commandId === target.commandId)) {
      target.dispose();
      return 'cancelled';
    }
    activeChatMessagingRef.current.add(target);
    const unsubscribeAuth = useAuthStore.subscribe(() => { current(); });
    const unsubscribeWorkspace = useWorkspaceStore.subscribe(() => { current(); });
    const report = (status: string) => {
      const now = useAuthStore.getState();
      if (status !== 'succeeded' && status !== 'cancelled' && now.isAuthenticated &&
          now.user?.userId === ownerId && now.user?.sessionId === sessionId &&
          commandRouteIdentityRef.current === route) {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(
          status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed',
        ));
      }
    };
    const guardedTarget: ChatMessagingTarget = {
      ...target,
      isCurrent: current,
      execute: handoff => {
        if (!current() || (lease && !lease.isCurrent())) return Promise.reject(new Error('contextual-palette-stale'));
        return target.execute(handoff);
      },
      canCommit: () => current() && (!lease || lease.isCurrent()) && document.hasFocus() &&
        ReadFocusContext().composition !== 'active' && target.canCommit(),
    };
    try {
      if (target.commandId === 'chat.message.edit.open') {
        const status = await executeChatMessaging(port, guardedTarget);
        report(status);
        return status;
      }
      await flushWorkspaceNavigation();
      if (!guardedTarget.canCommit()) return 'cancelled';
      const status = await executeChatMessaging({ ...port, beginUICommand: lease ? port.beginUICommand : begin ?? port.beginUICommand }, guardedTarget, target.commandId);
      report(status);
      return status;
    } catch {
      report('failed');
      return 'failed';
    } finally {
      unsubscribeAuth(); unsubscribeWorkspace();
      activeChatMessagingRef.current.delete(target);
      target.dispose();
    }
  }, []);
  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandId?: unknown; instanceId?: unknown; target?: ChatMessagingTarget }>).detail;
      if (!detail || !isChatMessagingCommand(detail.commandId) || typeof detail.instanceId !== 'string' || !detail.instanceId) return;
      const target = detail.target ?? captureChatMessagingTarget(() => pathnameRef.current, detail.commandId, detail.instanceId);
      if (!target) return;
      if (target.commandId !== detail.commandId || target.instanceId !== detail.instanceId || !target.canCommit() ||
          ReadFocusContext().composition === 'active' || !document.hasFocus()) { target.dispose(); return; }
      event.preventDefault();
      void runChatMessaging(target).catch(() => undefined);
    };
    window.addEventListener(CHAT_MESSAGING_COMMAND_EVENT, request);
    return () => {
      window.removeEventListener(CHAT_MESSAGING_COMMAND_EVENT, request);
      activeChatMessagingRef.current.forEach(target => target.dispose());
      activeChatMessagingRef.current.clear();
      clearPaletteChatMessaging();
      pendingChatMessagingRef.current?.dispose(); pendingChatMessagingRef.current = null;
    };
  }, [runChatMessaging, clearPaletteChatMessaging]);
  const pendingChatClearRef = useRef<ChatClearTarget | null>(null);
  const pendingTerminalOperationRef = useRef<TerminalOperationTarget | null>(null);
  const activePageMutationRef = useRef<PageMutationTarget | null>(null);
  const pendingPageMutationRef = useRef<PageMutationTarget | null>(null);
  const palettePageMutationsRef = useRef(new Map<string, PageMutationTarget>());
  const pageMutationDeckTicketsRef = useRef(new Set<string>());
  const pageMutationAvailable = useCallback((id: string) => {
    if (activePageMutationRef.current) return false;
    const target = capturePageMutationTarget(() => pathnameRef.current, id);
    try { return target?.canCommit() === true; } finally { target?.dispose(); }
  }, []);
  const runPageMutation = useCallback(async (target: PageMutationTarget, begin?: () => Promise<EditorFileBegin>, lease?: ContextualPaletteCommandLease): Promise<PageMutationOutcome> => {
    if (activePageMutationRef.current) { target.dispose(); return { status: 'cancelled' }; }
    activePageMutationRef.current = target;
    const owner = useAuthStore.getState().user;
    const route = commandRouteIdentityRef.current;
    try {
      if (!await flushWorkspaceNavigation() || !target.canCommit() || (lease && !lease.isCurrent())) return { status: 'cancelled' };
      const port = createPageMutationWailsPort({ contextualPalette: lease });
      const guardedTarget = lease ? { ...target, canCommit: () => lease.isCurrent() && target.canCommit() } : target;
      const outcome = await executePageMutation({ ...port, beginUICommand: lease ? port.beginUICommand : begin ?? port.beginUICommand }, guardedTarget);
      const auth = useAuthStore.getState();
      if (outcome.status !== 'succeeded' && outcome.status !== 'cancelled' && auth.isAuthenticated &&
          owner?.userId === auth.user?.userId && owner?.sessionId === auth.user?.sessionId && route === commandRouteIdentityRef.current) {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(outcome.status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed'));
      }
      return outcome;
    } finally { target.dispose(); if (activePageMutationRef.current === target) activePageMutationRef.current = null; }
  }, []);
  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<PageMutationEventDetail>).detail;
      if (!detail || typeof detail.resolve !== 'function' || !isPageMutationCommand(detail.commandId)) return;
      event.preventDefault();
      const target = capturePageMutationTarget(() => pathnameRef.current, detail.commandId, detail.instanceId);
      if (!target) { detail.resolve({ status: 'cancelled' }); return; }
      void runPageMutation(target).then(detail.resolve, () => detail.resolve({ status: 'failed' }));
    };
    window.addEventListener(PAGE_MUTATION_EVENT, request);
    return () => {
      window.removeEventListener(PAGE_MUTATION_EVENT, request);
      activePageMutationRef.current?.dispose(); pendingPageMutationRef.current?.dispose();
      palettePageMutationsRef.current.forEach(target => target.dispose()); palettePageMutationsRef.current.clear();
    };
  }, [runPageMutation]);
  const paletteTerminalOperationRef = useRef<TerminalOperationTarget | null>(null);
  const paletteTerminalSessionOperationsRef = useRef(new Map<string, TerminalOperationTarget>());
  const activeTerminalOperationRef = useRef<TerminalOperationTarget | null>(null);
  const terminalDeckTicketsRef = useRef(new Set<string>());
  const terminalOperationAvailable = useCallback((commandId: TerminalOperationCommand = TERMINAL_INTERRUPT_COMMAND) => {
    if (activeTerminalOperationRef.current || pathnameRef.current !== '/') return false;
    const target = captureTerminalOperationTarget(() => pathnameRef.current, undefined, commandId);
    try { return target?.canCommit() === true; } finally { target?.dispose(); }
  }, []);
  const runTerminalOperation = useCallback(async (commandId: TerminalOperationCommand, begin?: () => Promise<EditorFileBegin>, captured?: TerminalOperationTarget, lease?: ContextualPaletteCommandLease) => {
    const target = captured ?? pendingTerminalOperationRef.current ?? (lease ? undefined : captureTerminalOperationTarget(() => pathnameRef.current, undefined, commandId));
    pendingTerminalOperationRef.current = null;
    if (!target || target.commandId !== commandId || activeTerminalOperationRef.current) { target?.dispose(); return 'cancelled'; }
    activeTerminalOperationRef.current = target;
    const owner = useAuthStore.getState().user;
    const route = commandRouteIdentityRef.current;
    const originWorkspaceId = target.workspaceId;
    const originTabId = target.tabId;
    const canRefreshOrigin = () => {
      const auth = useAuthStore.getState();
      const workspace = useWorkspaceStore.getState().workspace;
      return auth.isAuthenticated && auth.user?.userId === owner?.userId &&
        auth.user?.sessionId === owner?.sessionId && workspace?.id === originWorkspaceId &&
        workspace.activeTabId === originTabId && route === commandRouteIdentityRef.current;
    };
    try {
      const flushed = await flushWorkspaceNavigation();
      if (!flushed || route !== commandRouteIdentityRef.current || !target.isCurrent() || (lease && !lease.isCurrent())) return 'cancelled';
      const port = createCommandTerminalOperationWailsPort({ contextualPalette: lease });
      const guardedTarget = lease ? { ...target,
        canCommit: () => lease.isCurrent() && target.canCommit(),
      } : target;
      const status = commandId === TERMINAL_INTERRUPT_COMMAND
        ? await executeTerminalInterrupt({ ...port, beginUICommand: lease ? port.beginUICommand : begin ?? port.beginUICommand }, guardedTarget)
        : await executeTerminalSessionOperation({ ...port, beginUICommand: lease ? port.beginUICommand : begin ?? port.beginUICommand }, guardedTarget);
      if (status === 'succeeded' && isTerminalSessionOperationCommand(commandId) && canRefreshOrigin()) {
        await useTerminalStore.getState().loadSessions();
        if (canRefreshOrigin()) await useWorkspaceStore.getState().reconcileActiveSelection();
      }
      const auth = useAuthStore.getState();
      if (status !== 'succeeded' && status !== 'cancelled' && auth.isAuthenticated &&
          auth.user?.userId === owner?.userId && auth.user?.sessionId === owner?.sessionId && route === commandRouteIdentityRef.current) {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(
          status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed'));
      }
      return status;
    } finally { target.dispose(); if (activeTerminalOperationRef.current === target) activeTerminalOperationRef.current = null; }
  }, []);
  const runTerminalInterrupt = useCallback((begin?: () => Promise<EditorFileBegin>, captured?: TerminalOperationTarget, lease?: ContextualPaletteCommandLease) =>
    runTerminalOperation(TERMINAL_INTERRUPT_COMMAND, begin, captured, lease), [runTerminalOperation]);
  useEffect(() => {
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ instanceId?: unknown; commandId?: unknown }>).detail;
      const instance = detail?.instanceId;
      if (typeof instance !== 'string' || !instance) return;
      const commandId = (detail?.commandId ?? TERMINAL_INTERRUPT_COMMAND) as TerminalOperationCommand;
      if (commandId !== TERMINAL_INTERRUPT_COMMAND && !isTerminalSessionOperationCommand(commandId)) return;
      const target = captureTerminalOperationTarget(() => pathnameRef.current, instance, commandId);
      if (!target?.canCommit()) { target?.dispose(); return; }
      event.preventDefault();
      void runTerminalOperation(commandId, undefined, target).catch(() => undefined);
    };
    window.addEventListener(TERMINAL_OPERATION_EVENT, request);
    return () => {
      window.removeEventListener(TERMINAL_OPERATION_EVENT, request);
      activeTerminalOperationRef.current?.dispose();
      pendingTerminalOperationRef.current?.dispose(); pendingTerminalOperationRef.current = null;
      paletteTerminalOperationRef.current?.dispose(); paletteTerminalOperationRef.current = null;
      paletteTerminalSessionOperationsRef.current.forEach(target => target.dispose());
      paletteTerminalSessionOperationsRef.current.clear();
    };
  }, [runTerminalOperation]);
  const activeChatClearRef = useRef<ChatClearTarget | null>(null);
  const chatClearAvailable = useCallback((keyboardTarget?: EventTarget | null) => {
    if (activeChatClearRef.current || (pathnameRef.current !== '/' && pathnameRef.current !== '')) return false;
    const target = captureChatClearTarget(() => pathnameRef.current, undefined, keyboardTarget);
    try { return target?.isCurrent() === true; } finally { target?.dispose(); }
  }, []);
  const runChatClear = useCallback(async (begin?: () => Promise<EditorFileBegin>, captured?: ChatClearTarget, lease?: ContextualPaletteCommandLease) => {
    const target = captured ?? pendingChatClearRef.current ?? (lease ? undefined : captureChatClearTarget(() => pathnameRef.current));
    pendingChatClearRef.current = null;
    if (!target || activeChatClearRef.current) { target?.dispose(); return 'cancelled'; }
    activeChatClearRef.current = target;
    const owner = useAuthStore.getState().user;
    const route = commandRouteIdentityRef.current;
    const port = createCommandWorkspaceTabWailsPort({ contextualPalette: lease });
    const report = (status: string) => {
      const auth = useAuthStore.getState();
      if (status !== 'succeeded' && status !== 'cancelled' && auth.isAuthenticated &&
          auth.user?.userId === owner?.userId && auth.user?.sessionId === owner?.sessionId && route === commandRouteIdentityRef.current) {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(
          status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed',
        ));
      }
    };
    try {
      await flushWorkspaceNavigation();
      if (route !== commandRouteIdentityRef.current || !target.isCurrent() || (lease && !lease.isCurrent())) { target.dispose(); return 'cancelled'; }
      const guardedTarget = lease ? { ...target,
        canCommit: () => lease.isCurrent() && target.canCommit(),
      } : target;
      const status = await executeChatClear({ ...port, beginUICommand: lease ? port.beginUICommand : begin ?? port.beginUICommand }, guardedTarget);
      report(status);
      return status;
    } catch {
      report('failed');
      return 'failed';
    } finally { target.dispose(); if (activeChatClearRef.current === target) activeChatClearRef.current = null; }
  }, []);
  useEffect(() => {
    const request = (event: Event) => {
      const instance = (event as CustomEvent<{ instanceId?: string }>).detail?.instanceId;
      if (!instance) return;
      const target = captureChatClearTarget(() => pathnameRef.current, instance);
      if (!target) return;
      event.preventDefault();
      void runChatClear(undefined, target);
    };
    window.addEventListener(CHAT_CLEAR_EVENT, request);
    return () => {
      window.removeEventListener(CHAT_CLEAR_EVENT, request);
      activeChatClearRef.current?.dispose();
      paletteChatClearRef.current?.dispose(); paletteChatClearRef.current = null;
      pendingChatClearRef.current?.dispose(); pendingChatClearRef.current = null;
    };
  }, [runChatClear]);

  const mermaidAvailable = useCallback((commandID: string) => {
    if (!isEditorMermaidCommand(commandID) || mermaidBusyRef.current || (pathnameRef.current !== '/' && pathnameRef.current !== '')) return false;
    const target = captureEditorMermaidTarget(commandID);
    try { return target?.isCurrent() === true && target.canExecute(commandID); }
    finally { target?.dispose(); }
  }, []);

  const runMermaidCommand = useCallback(async (commandID: string, begin?: () => Promise<EditorFileBegin>, captured?: EditorMermaidTarget, lease?: ContextualPaletteCommandLease) => {
    const target = captured ?? pendingMermaidRef.current ?? captureEditorMermaidTarget(commandID);
    pendingMermaidRef.current = null;
    const cancelled = { invocationId: '', status: 'cancelled', errorCode: 'ui-context-stale' };
    if (!target || !isEditorMermaidCommand(commandID) || mermaidBusyRef.current) { target?.dispose(); return cancelled; }
    mermaidBusyRef.current = true; activeMermaidRef.current = target;
    const route = commandRouteIdentityRef.current;
    const owner = useAuthStore.getState().user;
    let executor: CommandUIExecution | undefined;
    let earlyReservation: EditorFileBegin | undefined;
    let reservationTransferred = false;
    const sourceCurrent = () => !lease || lease.isCurrent();
    try {
      const flushed = await flushWorkspaceNavigation();
      if (lease && !flushed || route !== commandRouteIdentityRef.current || !sourceCurrent() || !target.isCurrent() || !target.canExecute(commandID)) return cancelled;
      if (commandID === 'editor.mermaid.open') {
        pendingMermaidRef.current = target;
        return { ...cancelled, status: executeLocalUICommand(commandID) ? 'succeeded' : 'cancelled' };
      }
      // Physical offers expire in 10 seconds, independently of the destructive
      // confirmation budget. Consume once before any asynchronous preparation.
      if (lease) {
        if (lease.source !== 'deck' || !isEditorMermaidMutation(commandID) || !lease.beginUICommand) return cancelled;
        earlyReservation = await lease.beginUICommand();
        if (!sourceCurrent() || !target.canExecute(commandID)) return cancelled;
      }
      if (!await target.prepare(commandID) || route !== commandRouteIdentityRef.current || !sourceCurrent() || !target.isCurrent() ||
          lease && target.hasPreparedFocus?.() !== true || isModalOpen()) return cancelled;
      const port = createCommandUIExecutionWailsPort();
      const effect = () => { if (!target.execute(commandID)) throw new Error('mermaid-not-applied'); return undefined; };
      executor = createCommandUIExecution({ ...port, beginUICommand: earlyReservation ? async () => {
        reservationTransferred = true; return earlyReservation!;
      } : begin ?? port.beginUICommand }, new Map([[commandID, effect]]), {
        canCommitEffect: () => route === commandRouteIdentityRef.current && sourceCurrent() && target.isCurrent() && target.canExecute(commandID) &&
          (!lease || target.hasPreparedFocus?.() === true) && !isModalOpen(),
        prepareEffect: () => ({ effect, isCurrent: target.isCurrent, dispose: target.dispose }),
      });
      const result = await executor.execute(commandID);
      const auth = useAuthStore.getState();
      if (result.status !== 'succeeded' && result.status !== 'cancelled' && auth.isAuthenticated &&
          auth.user?.userId === owner?.userId && auth.user?.sessionId === owner?.sessionId && route === commandRouteIdentityRef.current) {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(result.status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed'));
      }
      return result;
    } finally {
      if (earlyReservation && !reservationTransferred) await createCommandUIExecutionWailsPort().cancelUICommand(earlyReservation.ticket).catch(() => undefined);
      executor?.dispose(); target.dispose();
      if (activeMermaidRef.current === target) activeMermaidRef.current = null;
      if (pendingMermaidRef.current === target) pendingMermaidRef.current = null;
      mermaidBusyRef.current = false;
    }
  }, [executeLocalUICommand]);

  useEffect(() => {
    const pending = new Map<number, EditorMermaidTarget>();
    const keyboardLifetimes = new Set<() => void>();
    const request = (event: Event) => {
      const detail = (event as CustomEvent<EditorMermaidCommandRequest>).detail;
      if (!detail || !isEditorMermaidCommand(detail.commandID) || mermaidBusyRef.current) return;
      const target = captureEditorMermaidTarget(detail.commandID, detail.expectedDocument, detail.input);
      if (!target) return;
      const shortcut = detail.shortcut ? { ...detail.shortcut, modifiers: [...detail.shortcut.modifiers] } : undefined;
      const generation = localKeyboardGenerationRef.current;
      if (shortcut && (detail.commandID !== 'editor.mermaid.apply' || !generation)) { target.dispose(); return; }
      const keyboard = shortcut ? captureEditorMermaidKeyboard(generation!, shortcut) : undefined;
      if (keyboard) keyboardLifetimes.add(keyboard.dispose);
      event.preventDefault();
      const timer = window.setTimeout(() => {
        pending.delete(timer);
        void runMermaidCommand(detail.commandID, keyboard?.begin, target).catch(() => undefined).finally(() => {
          keyboard?.dispose(); if (keyboard) keyboardLifetimes.delete(keyboard.dispose);
        });
      }, 0);
      pending.set(timer, target);
    };
    window.addEventListener(EDITOR_MERMAID_COMMAND_EVENT, request);
    return () => {
      window.removeEventListener(EDITOR_MERMAID_COMMAND_EVENT, request);
      pending.forEach((target, timer) => { window.clearTimeout(timer); target.dispose(); });
      keyboardLifetimes.forEach(dispose => dispose());
      activeMermaidRef.current?.dispose(); pendingMermaidRef.current?.dispose();
    };
  }, [runMermaidCommand]);

  const editorFormatAvailable = useCallback((commandID: string) => {
    if (editorFormatBusyRef.current || (pathnameRef.current !== '/' && pathnameRef.current !== '')) return false;
    const target = captureEditorFormatting();
    try { return target?.canExecute(commandID) === true; }
    finally { target?.dispose(); }
  }, []);

  const executeFormatCommand = useCallback(async (commandID: string, begin?: () => Promise<EditorFileBegin>, captured?: EditorFormatTarget, input?: unknown, lease?: ContextualPaletteCommandLease) => {
    const target = captured ?? pendingEditorFormatTargetRef.current ?? (lease ? undefined : captureEditorFormatting());
    pendingEditorFormatTargetRef.current = null;
    const cancelled = { invocationId: '', status: 'cancelled', errorCode: 'ui-context-stale' };
    if (!target || !isEditorFormatCommand(commandID) || editorFormatBusyRef.current) { target?.dispose(); return cancelled; }
    editorFormatBusyRef.current = true;
    activeEditorFormatTargetRef.current = target;
    const route = commandRouteIdentityRef.current;
    const owner = useAuthStore.getState().user;
    let executor: CommandUIExecution | undefined;
    try {
      await flushWorkspaceNavigation();
      if (route !== commandRouteIdentityRef.current || !target.isCurrent() || !target.canExecute(commandID) || (lease && !lease.isCurrent())) return cancelled;
      const prepared = await target.prepare(commandID, input);
      if (!prepared || route !== commandRouteIdentityRef.current || !target.isCurrent() || !target.canExecute(commandID) || (lease && !lease.isCurrent())) return cancelled;
      const port = lease ? createCommandWorkspaceTabWailsPort({ contextualPalette: lease }) : createCommandUIExecutionWailsPort();
      const current = () => (!lease || lease.isCurrent()) && route === commandRouteIdentityRef.current && target.isCurrent() && !isModalOpen();
      const effect = () => { if (!current() || !target.execute(commandID)) throw new Error('editor-format-not-applied'); return undefined; };
      executor = createCommandUIExecution({ ...port, beginUICommand: lease ? port.beginUICommand : begin ?? port.beginUICommand }, new Map([[commandID, effect]]), {
        canCommitEffect: current,
        prepareEffect: () => ({ effect, isCurrent: current, dispose: target.dispose }),
      });
      const result = await executor.execute(commandID);
      const auth = useAuthStore.getState();
      if (result.status !== 'succeeded' && result.status !== 'cancelled' &&
          auth.isAuthenticated && auth.user?.userId === owner?.userId && auth.user?.sessionId === owner?.sessionId &&
          route === commandRouteIdentityRef.current) {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(
          result.status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed',
        ));
      }
      return result;
    } finally {
      executor?.dispose(); target.dispose();
      if (activeEditorFormatTargetRef.current === target) activeEditorFormatTargetRef.current = null;
      editorFormatBusyRef.current = false;
    }
  }, []);

  useEffect(() => {
    const pending = new Map<number, EditorFormatTarget>();
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandID?: string; input?: unknown; expectedEditor?: object }>).detail;
      const id = detail?.commandID;
      if (!id || !isEditorFormatCommand(id) || !editorFormatAvailable(id)) return;
      const target = captureEditorFormatting(detail?.expectedEditor);
      if (!target) return;
      event.preventDefault();
      const timer = window.setTimeout(() => {
        pending.delete(timer);
        void executeFormatCommand(id, undefined, target, detail?.input).catch(() => undefined);
      }, 0);
      pending.set(timer, target);
    };
    window.addEventListener('commands:editor-format', request);
    return () => {
      window.removeEventListener('commands:editor-format', request);
      pending.forEach((target, timer) => { window.clearTimeout(timer); target.dispose(); });
      pendingEditorFormatTargetRef.current?.dispose(); pendingEditorFormatTargetRef.current = null;
    };
  }, [editorFormatAvailable, executeFormatCommand]);

  useEffect(() => {
    const pending = new Map<number, EditorPresentationTargetLease>();
    const request = (event: Event) => {
      const detail = (event as CustomEvent<{ commandID?: string; expectedEditor?: Editor }>).detail;
      const id = detail?.commandID;
      if (!id || !isEditorCellNavigationCommand(id)) return;
      if (detail?.expectedEditor !== undefined) {
        const source = captureEditorFormatting(detail.expectedEditor);
        const current = source?.isCurrent() === true;
        source?.dispose();
        if (!current) return;
      }
      // A menu is still open here. Capture now, but evaluate presentation
      // eligibility only after it closes; never recapture another selection.
      const target = captureEditorPresentationTarget(() => pathnameRef.current);
      if (!target) return;
      event.preventDefault();
      const timer = window.setTimeout(() => {
        pending.delete(timer);
        try {
          executeLocalUICommand(id, false, target);
        } finally {
          target.dispose();
        }
      }, 0);
      pending.set(timer, target);
    };
    window.addEventListener(EDITOR_CELL_NAVIGATION_COMMAND_EVENT, request);
    return () => {
      window.removeEventListener(EDITOR_CELL_NAVIGATION_COMMAND_EVENT, request);
      pending.forEach((target, timer) => { window.clearTimeout(timer); target.dispose(); });
      pending.clear();
    };
  }, [executeLocalUICommand]);

  const executeFileCommand = useCallback(async (commandID: string, begin?: () => Promise<EditorFileBegin>, lease?: ContextualPaletteCommandLease, captured?: EditorFileTargetLease) => {
    const target = captured ?? pendingEditorFileTargetRef.current ?? (lease ? undefined : captureEditorFileTarget());
    pendingEditorFileTargetRef.current = null;
    const cancelled = { invocationId: '', status: 'cancelled', errorCode: 'ui-context-stale' };
    if (!target || !isEditorFileCommand(commandID) || editorFileBusyRef.current ||
        (pathnameRef.current !== '/' && pathnameRef.current !== '')) { target?.dispose(); return cancelled; }
    editorFileBusyRef.current = true;
    const route = commandRouteIdentityRef.current;
    const guard = createCommandUIEffectGuard();
    try {
      await flushWorkspaceNavigation();
      if (!target.isCurrent() || route !== commandRouteIdentityRef.current || (lease && !lease.isCurrent())) return cancelled;
      const ui = lease ? createCommandWorkspaceTabWailsPort({ contextualPalette: lease }) : createCommandUIExecutionWailsPort();
      const files = createEditorFileCommandWailsPort();
      const authorize = () => {
        if (route !== commandRouteIdentityRef.current || !target.isCurrent() || isModalOpen() || !document.hasFocus()) return false;
        const token = guard.capture(undefined, commandID);
        return !!token && guard.commit(token, () => undefined);
      };
      let nativeContinuation: (() => boolean) | undefined;
      const result = await executeEditorFileCommand({
        begin: lease ? ui.beginUICommand : begin ?? ui.beginUICommand,
        take: ui.takeUICommand,
        prepare: files.prepare,
        commit: files.commit,
        completeCancelled: (ticket, handoff) => ui.completeUICommand(ticket, handoff, 'cancelled'),
        getResult: ui.getUICommandResult,
        cancel: ui.cancelUICommand,
      }, commandID, { target,
        authorizeInitial: () => (!lease || lease.isCurrent()) && authorize(),
        ...(lease ? { authorizePreparation: () => {
          if (!authorize()) return false;
          nativeContinuation = lease.prepareNativeFileContinuation?.();
          return nativeContinuation !== undefined;
        } } : {}),
        authorizeCommit: () => (!lease || nativeContinuation?.() === true) && authorize(),
      });
      if (target.isCurrent() && result.status !== 'succeeded' && result.status !== 'cancelled') {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(
          result.status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed',
        ));
      }
      return result;
    } catch {
      return { invocationId: '', status: 'outcome_unknown', errorCode: 'file-command-outcome-unknown' };
    } finally {
      guard.dispose(); target.dispose(); editorFileBusyRef.current = false;
    }
  }, []);

  const workspaceTabMutationAvailable = useCallback((commandID: string) => {
    const target = prepareWorkspaceTabTarget(commandID);
    if (!target) return false;
    try {
      return target.isCurrent();
    } finally {
      target.dispose();
    }
  }, [prepareWorkspaceTabTarget]);

  const prepareWorkspaceMutationTarget = useCallback((commandID: string) => {
    if (isEditorModeCommand(commandID)) {
      const active = activeEditorModeTargetRef.current;
      if (active && active.commandID !== commandID) return undefined;
      if (active) return { isCurrent: (): boolean => active.isCurrent(), prepare: (): boolean => active.prepare(), dispose: (): void => undefined };
      return captureEditorModeTarget(() => pathnameRef.current, commandID);
    }
    if (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID) {
      const prepared = workspaceChatOpenLeaseRef.current;
      if (!prepared) return undefined;
      // A prepared chat lease is owned by executeContextualCommand. The
      // contextual executor may probe it more than once; these probes must not
      // dispose the shared lease.
      return {
        isCurrent: () => prepared.isCurrent(),
        dispose: () => undefined,
      };
    }
    if (commandID === WORKSPACE_CREATE_COMMAND_ID) {
      const pendingTarget = activeCommandIntentRef.current?.commandID === WORKSPACE_CREATE_COMMAND_ID
        ? pendingWorkspaceCreateTargetRef.current
        : null;
      const freshTarget = captureWorkspaceCreateTarget(() => commandRouteIdentityRef.current);
      if (!freshTarget) return undefined;
      if (!pendingTarget) return freshTarget;
      // A menu selection keeps its original lease across menu dismissal. A
      // fresh lease still owns this probe/execution call; both origins must
      // remain current, and only the fresh lease is disposed here.
      return {
        isCurrent: () => pendingTarget.isCurrent() && freshTarget.isCurrent(),
        dispose: () => freshTarget.dispose(),
      };
    }
    return isWorkspaceTabMutationCommand(commandID)
      ? prepareWorkspaceTabTarget(commandID)
      : undefined;
  }, [prepareWorkspaceTabTarget]);

  const workspaceMutationAvailable = useCallback((commandID: string) => {
    if (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID) {
      const activeTabID = useWorkspaceStore.getState().workspace?.activeTabId;
      return Boolean(activeTabID && canPrepareWorkspaceChatOpen(activeTabID));
    }
    const target = prepareWorkspaceMutationTarget(commandID);
    if (!target) return false;
    try {
      return target.isCurrent();
    } finally {
      target.dispose();
    }
  }, [prepareWorkspaceMutationTarget]);

  const executeContextualCommand = useCallback(async (executor: CommandUIExecution, commandID: string, capturedTarget?: ReturnType<typeof prepareWorkspaceMutationTarget>, sourceLease?: ContextualPaletteCommandLease) => {
    if (isEditorModeCommand(commandID) && activeEditorModeTargetRef.current) {
      return { invocationId: '', status: 'cancelled' as const, errorCode: 'editor-mode-busy' };
    }
    let editorModeTarget: EditorModeTargetLease | undefined;
    const cancellationGeneration = commandCancellationGenerationRef.current;
    const pendingTargetForExecution = commandID === WORKSPACE_CREATE_COMMAND_ID &&
      activeCommandIntentRef.current?.commandID === WORKSPACE_CREATE_COMMAND_ID
      ? pendingWorkspaceCreateTargetRef.current
      : null;
    const completionFocus = commandID === WORKSPACE_TAB_CLOSE_COMMAND_ID
      ? captureWorkspaceTabCloseFocus(() => pathnameRef.current)
      : undefined;
    let chatLease: PreparedWorkspaceChatOpen | null = null;
    let preparationToken: {
      generation: number;
      tabID: string;
      routeIdentity: string;
      activeElement: Element | null;
    } | null = null;
    let originalTarget: ReturnType<typeof prepareWorkspaceMutationTarget>;
    try {
      if (isEditorModeCommand(commandID)) {
        if (pendingEditorModeTargetRef.current && pendingEditorModeTargetRef.current.commandID !== commandID) {
          pendingEditorModeTargetRef.current.dispose();
          pendingEditorModeTargetRef.current = null;
          return { invocationId: '', status: 'cancelled' as const, errorCode: 'ui-context-stale' };
        }
        editorModeTarget = pendingEditorModeTargetRef.current ?? captureEditorModeTarget(() => pathnameRef.current, commandID);
        pendingEditorModeTargetRef.current = null;
        if (!editorModeTarget?.isCurrent()) return { invocationId: '', status: 'cancelled' as const, errorCode: 'ui-context-stale' };
        activeEditorModeTargetRef.current = editorModeTarget;
      }
      if (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID) {
        const capturedRouteIdentity = commandRouteIdentityRef.current;
        const capturedWorkspace = useWorkspaceStore.getState().workspace;
        const activeTabID = capturedWorkspace?.activeTabId;
        if (!activeTabID || !canPrepareWorkspaceChatOpen(activeTabID)) {
          return { invocationId: '', status: 'cancelled' as const, errorCode: 'ui-context-stale' };
        }
        preparationToken = {
          generation: cancellationGeneration,
          tabID: activeTabID,
          routeIdentity: capturedRouteIdentity,
          activeElement: document.activeElement,
        };
        const currentPreparationToken = preparationToken;
        workspaceChatPreparationRef.current = currentPreparationToken;
        chatLease = await prepareWorkspaceChatOpen(activeTabID, () => {
          const current = useWorkspaceStore.getState().workspace;
          return workspaceChatPreparationRef.current === currentPreparationToken &&
            (!sourceLease || sourceLease.isCurrent()) &&
            document.activeElement === currentPreparationToken.activeElement &&
            commandPickerMountedRef.current &&
            cancellationGeneration === commandCancellationGenerationRef.current &&
            commandRouteIdentityRef.current === capturedRouteIdentity &&
            current?.id === capturedWorkspace?.id &&
            current?.activeTabId === activeTabID &&
            !isModalOpen() && document.hasFocus();
        }) ?? null;
        if (!chatLease || !chatLease.isCurrent() ||
            cancellationGeneration !== commandCancellationGenerationRef.current ||
            !commandPickerMountedRef.current) {
          return { invocationId: '', status: 'cancelled' as const, errorCode: 'ui-context-stale' };
        }
        workspaceChatOpenLeaseRef.current = chatLease;
      }
      originalTarget = capturedTarget ?? (isWorkspaceMutationCommand(commandID)
        ? prepareWorkspaceMutationTarget(commandID)
        : undefined);
      if (isWorkspaceMutationCommand(commandID)) {
        // Persistent tab mutations must wait for
        // the store's active-tab projection before committing the captured
        // source. This is a barrier, never a retargeting operation.
        if (!originalTarget) {
          return { invocationId: '', status: 'cancelled' as const, errorCode: 'ui-context-stale' };
        }
        const flushed = await flushWorkspaceNavigation();
        if (!flushed || cancellationGeneration !== commandCancellationGenerationRef.current || !originalTarget.isCurrent() || (sourceLease && !sourceLease.isCurrent())) {
          return { invocationId: '', status: 'cancelled' as const, errorCode: 'ui-context-stale' };
        }
      }
      const result = await executor.execute(commandID);
      if (editorModeTarget && result.status === 'succeeded' &&
          cancellationGeneration === commandCancellationGenerationRef.current && commandPickerMountedRef.current) {
        editorModeTarget.applyCommitted();
      } else if (editorModeTarget && editorModeTarget.isCurrent() &&
          cancellationGeneration === commandCancellationGenerationRef.current && commandPickerMountedRef.current &&
          result.status !== 'cancelled' && result.status !== 'cancelled_stale' && result.status !== 'rejected_stale') {
        creationPresentationRef.current.announce(creationPresentationRef.current.t(
          result.status === 'failed' ? 'commandPalette.executionFailed' :
            result.status === 'denied' ? 'commandPalette.unavailable' : 'commandPalette.executionUnknown',
        ));
      }
      if (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID && result.status === 'succeeded' &&
          chatLease?.isCurrent() && originalTarget?.isCurrent()) {
        const workspace = useWorkspaceStore.getState().workspace;
        const tabID = workspace?.activeTabId;
        let presented = false;
        if (workspace && tabID) {
          const conversationID = await readPreparedWorkspaceChatConversation(workspace.id, tabID);
          if (conversationID !== null &&
              cancellationGeneration === commandCancellationGenerationRef.current &&
              commandPickerMountedRef.current && chatLease.isCurrent() && originalTarget.isCurrent()) {
            presented = chatLease.present(conversationID);
          }
        }
        if (!presented && cancellationGeneration === commandCancellationGenerationRef.current &&
            commandPickerMountedRef.current && chatLease.isCurrent() && originalTarget.isCurrent()) {
          creationPresentationRef.current.announce(
            creationPresentationRef.current.t('workspace.chatModal.prepareFailed'),
          );
        }
      } else if (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID &&
          result.status !== 'succeeded' && result.status !== 'cancelled' &&
          result.status !== 'cancelled_stale' && result.status !== 'rejected_stale' &&
          cancellationGeneration === commandCancellationGenerationRef.current &&
          commandPickerMountedRef.current && originalTarget?.isCurrent()) {
        creationPresentationRef.current.announce(
          creationPresentationRef.current.t('workspace.chatModal.prepareFailed'),
        );
      }
      if (commandID === WORKSPACE_CREATE_COMMAND_ID &&
          cancellationGeneration === commandCancellationGenerationRef.current &&
          originalTarget?.isCurrent()) {
        const presentation = creationPresentationRef.current;
        if (result.status === 'succeeded') {
          presentation.announce(presentation.t('workspace.announce.workspaceCreatedCommand'));
        } else if (result.status === 'failed') {
          presentation.announce(presentation.t('commandPalette.executionFailed'));
        } else if (result.status === 'denied') {
          presentation.announce(presentation.t('commandPalette.unavailable'));
        } else if (result.status === 'outcome_unknown' || result.status === 'timed_out' || result.status === 'suppressed') {
          presentation.announce(presentation.t('commandPalette.executionUnknown'));
        }
      }
      // Foco é apresentação pós-escrita: nunca submete outra mutação nem
      // transforma um resultado backend em uma confirmação visual.
      if (result.status === 'succeeded' && commandPickerMountedRef.current) {
        try { completionFocus?.apply(); }
        catch (error) { logger.warn('[Commands] Falha ao restaurar foco após comando de aba', error); }
      }
      return result;
    } finally {
      editorModeTarget?.dispose();
      if (editorModeTarget && activeEditorModeTargetRef.current === editorModeTarget) activeEditorModeTargetRef.current = null;
      completionFocus?.dispose();
      originalTarget?.dispose();
      if (chatLease) {
        chatLease.dispose();
        if (workspaceChatOpenLeaseRef.current === chatLease) {
          workspaceChatOpenLeaseRef.current = null;
        }
      }
      if (preparationToken && workspaceChatPreparationRef.current === preparationToken) {
        workspaceChatPreparationRef.current = null;
      }
      if (pendingTargetForExecution && pendingWorkspaceCreateTargetRef.current === pendingTargetForExecution) {
        pendingWorkspaceCreateTargetRef.current?.dispose();
        pendingWorkspaceCreateTargetRef.current = null;
      }
    }
  }, [prepareWorkspaceMutationTarget]);

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const [commandCatalogLoading, setCommandCatalogLoading] = useState(false);

  const intentIsCurrent = useCallback((intent: PendingCommandIntent) => {
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    return commandPickerMountedRef.current && auth.isAuthenticated &&
      auth.user?.userId === intent.userId && auth.user?.sessionId === intent.sessionId &&
      currentWorkspace?.id === intent.workspaceId &&
      (currentWorkspace?.activeTabId ?? null) === intent.activeTabId &&
      commandRouteIdentityRef.current === intent.routeIdentity && !isModalOpen();
  }, []);

  const presentWorkspaceList = useCallback((intent: PendingCommandIntent, output: WorkspaceListCommandOutput): undefined => {
    if (activeCommandIntentRef.current !== intent || !intentIsCurrent(intent)) return undefined;
    const trigger = commandPickerButtonRef.current;
    if (!trigger) return undefined;
    const items: MenuItem[] = output.workspaces.map((ws) => ({
      id: `ws-command-${ws.id}`,
      label: ws.name,
      icon: ws.is_active ? <CheckOutlined /> : undefined,
      shortcut: `${ws.tab_count} ${ws.tab_count === 1 ? t('workspace.tabSingular') : t('workspace.tabPlural')}`,
      checked: ws.is_active,
      action: () => {
        if (workspacePickerIntentRef.current !== intent || !intentIsCurrent(intent)) return;
        // Consumir antes do efeito: o mesmo callback não pode trocar duas vezes.
        workspacePickerIntentRef.current = null;
        if (!ws.is_active) void switchWorkspace(ws.id);
      },
    }));
    workspacePickerIntentRef.current = intent;
    openWorkspacePickerRef.current?.(trigger, t('workspace.workspaceList'), items);
    return undefined;
  }, [intentIsCurrent, switchWorkspace, t]);
  const presentWorkspaceListRef = useRef(presentWorkspaceList);
  presentWorkspaceListRef.current = presentWorkspaceList;

  useLayoutEffect(() => {
    commandPickerMountedRef.current = true;
    setCommandCatalogLoading(false);
    const trustedSession = commandScope?.session ?? createTrustedCommandContextSession();
    localPaletteTrustedSessionRef.current = trustedSession;
    const surfaceElement = commandSurfaceRef.current;
    const unregisterSurface = trustedSession.registerSurfaceContext(COMMAND_TOOLBAR_SURFACE_ID, () => {
      if (!surfaceElement?.isConnected || commandSurfaceRef.current !== surfaceElement) return null;
      return {
        surfaceType: 'toolbar',
        surfaceId: COMMAND_TOOLBAR_SURFACE_ID,
        snapshotVersion: JSON.stringify(['command-toolbar-v1', commandRouteIdentityRef.current]),
      };
    });
    const contextOptions = {
      trustedSession,
      surfaceID: COMMAND_TOOLBAR_SURFACE_ID,
    };
    const guard = createCommandUIEffectGuard(trustedSession);
    const contextualExecutor = createCommandContextualBackendExecution(
      createCommandWorkspaceTabWailsPort(),
      {
        trustedSession,
        surfaceID: COMMAND_TOOLBAR_SURFACE_ID,
        prepareTarget: prepareWorkspaceMutationTarget,
      },
    );
    commandContextualExecutionRef.current = contextualExecutor;
    const editorModeTimers = new Set<ReturnType<typeof setTimeout>>();
    const requestEditorMode = (event: Event) => {
      const commandID = (event as CustomEvent<{ commandID?: string }>).detail?.commandID;
      if (!commandID || !isEditorModeCommand(commandID) || activeEditorModeTargetRef.current || pendingEditorModeTargetRef.current || isModalOpen()) return;
      const target = captureEditorModeTarget(() => pathnameRef.current, commandID);
      if (!target) return;
      pendingEditorModeTargetRef.current = target;
      event.preventDefault();
      const requestGeneration = commandCancellationGenerationRef.current;
      // O menu fecha/restaura foco primeiro; a lease mantém a origem, sem recaptura.
      const timer = setTimeout(() => {
        editorModeTimers.delete(timer);
        if (pendingEditorModeTargetRef.current !== target) return;
        if (!target.isCurrent() || !commandPickerMountedRef.current || isModalOpen() || !document.hasFocus()) {
          target.dispose(); pendingEditorModeTargetRef.current = null; return;
        }
        void executeContextualCommand(contextualExecutor, commandID).catch(() => {
          if (commandPickerMountedRef.current && commandCancellationGenerationRef.current === requestGeneration) {
            creationPresentationRef.current.announce(creationPresentationRef.current.t('commandPalette.executionFailed'));
          }
        });
      }, 0);
      editorModeTimers.add(timer);
    };
    window.addEventListener(EDITOR_MODE_COMMAND_EVENT, requestEditorMode);
    const requestEditorFile = (event: Event) => {
      const id = (event as CustomEvent<{ commandID?: string }>).detail?.commandID;
      if (!id || !isEditorFileCommand(id) || !editorFileAvailable(id) || pendingEditorFileTargetRef.current) return;
      const target = captureEditorFileTarget();
      if (!target) return;
      pendingEditorFileTargetRef.current = target;
      event.preventDefault();
      const timer = setTimeout(() => {
        editorModeTimers.delete(timer);
        if (pendingEditorFileTargetRef.current !== target) return;
        if (!commandPickerMountedRef.current || !target.isCurrent()) {
          target.dispose(); pendingEditorFileTargetRef.current = null; return;
        }
        void executeFileCommand(id);
      }, 0);
      editorModeTimers.add(timer);
    };
    window.addEventListener('commands:editor-file', requestEditorFile);
    const unregisterWorkspaceChatDispatcher = registerWorkspaceChatCommandDispatcher(async (tabID) => {
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      if (!currentWorkspace || currentWorkspace.activeTabId !== tabID || isModalOpen() || !document.hasFocus()) return;
      await executeContextualCommand(contextualExecutor, WORKSPACE_CHAT_OPEN_COMMAND_ID);
    });
    const backendExecutor = createCommandBackendExecution(createCommandBackendExecutionWailsPort(), contextOptions);
    commandBackendExecutionRef.current = backendExecutor;
    const localPort = createCommandLocalKeyboardWailsPort();
    let disposed = false;
    let localIntent: PendingCommandIntent | null = null;
    const clearLocalMapRefs = () => {
      publishCommandShortcutHints(null);
      clearPaletteEditorModeTargets();
      paletteSurfaceTargetRef.current?.dispose();
      paletteSurfaceTargetRef.current = undefined;
      localPaletteCommandsRef.current = new Set();
      contextualPaletteCommandsRef.current = new Set();
      contextualPaletteResolverRef.current = () => false;
      localPaletteConditionResolverRef.current = () => false;
      localPaletteSourceRef.current = null;
      localKeyboardGenerationRef.current = null;
      localKeyboardOwnerRef.current = null;
    };
    const invalidateLocalPresentation = () => {
      localCommandExecutionRef.current?.cancelPresentation();
      void localCommandContextualExecutionRef.current?.cancel();
      if (localIntent && activeCommandIntentRef.current === localIntent) activeCommandIntentRef.current = null;
      if (localIntent && workspacePickerIntentRef.current === localIntent) workspacePickerIntentRef.current = null;
      localIntent = null;
    };
    const creationIntentCurrent = (intent: WorkspaceTabCreationIntent) =>
      newTabMenuIntentRef.current === intent &&
      intentIsCurrent({ ...intent, commandID: '' }) &&
      intent.modalGeneration === getModalRegistrySnapshot().generationNumber &&
      document.hasFocus();
    const closeCreationMenu = (restoreFocus = false) => {
      newTabMenuIntentRef.current = null;
      tabCreationMenu?.close({ restoreFocus });
    };
    const cancelCreationMenu = (restoreFocus = false) => {
      closeCreationMenu(restoreFocus);
      localCommandKeyboardRef.current?.cancelSequence();
    };
    const openCreationMenu = (bindings?: readonly LocalCommandKeyboardBinding[]) => {
      const auth = useAuthStore.getState();
      const current = useWorkspaceStore.getState().workspace;
      if (disposed || !auth.isAuthenticated || !auth.user || !current || isModalOpen() || !document.hasFocus()) return;
      const canShowCreationMenu = Boolean(tabCreationMenu) && workspaceTabMutationAvailable('workspace.tab.chat.create');
      if (!bindings && !canShowCreationMenu) return;
      const intent: WorkspaceTabCreationIntent = {
        userId: auth.user.userId, sessionId: auth.user.sessionId,
        workspaceId: current.id, activeTabId: current.activeTabId ?? null,
        routeIdentity: commandRouteIdentityRef.current,
        modalGeneration: getModalRegistrySnapshot().generationNumber,
      };
      newTabMenuIntentRef.current = intent;
      // The sequence recognizer is generic. Only workspace origins have a
      // creation-menu host; other v2 bindings still use their normal ingress.
      if (!tabCreationMenu || !canShowCreationMenu) return;
      const openingFocus = document.activeElement;
      const ids = bindings ? new Set(bindings.map((binding) => binding.commandId)) : new Set<string>(WORKSPACE_TAB_CREATE_COMMAND_IDS);
      void listCommandCatalog({ locale: creationPresentationRef.current.locale, source: 'palette' }).then((catalog) => {
        if (!creationIntentCurrent(intent)) {
          if (newTabMenuIntentRef.current === intent) cancelCreationMenu();
          return;
        }
        if (document.activeElement !== openingFocus) { cancelCreationMenu(); return; }
        const items = catalog.filter((item) => ids.has(item.id)).map((item) => ({
          commandID: item.id,
          label: item.name || item.id,
          disabled: !item.available,
          shortcut: bindings ? bindings.filter((binding) => binding.commandId === item.id)
            .map((binding) => formatCommandKeyboardTrigger(binding.shortcut)).join(', ') : shortcutHintRef.current(item.id),
        }));
        const shown = tabCreationMenu.show({ origin: bindings ? 'keyboard' : 'toolbar', intent, items }, (commandID, selectedIntent) => {
          if (selectedIntent !== intent || !creationIntentCurrent(intent) ||
              !items.some((item) => item.commandID === commandID && !item.disabled)) {
            cancelCreationMenu();
            return;
          }
          closeCreationMenu(true);
          localCommandKeyboardRef.current?.cancelSequence();
          // Click/Enter are palette selections, never synthetic key events.
          pendingCommandExecutionRef.current = { ...intent, commandID };
          executePendingCommandRef.current?.();
        });
        if (!shown) cancelCreationMenu();
      }).catch((error) => {
        if (!creationIntentCurrent(intent)) return;
        cancelCreationMenu();
        logger.error('[Commands] Falha ao carregar comandos de criação', error);
        creationPresentationRef.current.announce(creationPresentationRef.current.t('commandPalette.error'));
      });
    };
    const unregisterCreationActions = tabCreationMenu?.registerActions({
      openFromToolbar: () => {
        if (newTabMenuIntentRef.current) { cancelCreationMenu(true); return; }
        openCreationMenu();
      },
      cancel: (reason) => cancelCreationMenu(reason === 'escape'),
      requestWorkspaceCreate: queueWorkspaceCreateCommand,
      completeWorkspaceCreate: (restoreFocus) => {
        const intent = pendingCommandExecutionRef.current;
        const target = pendingWorkspaceCreateTargetRef.current;
        if (intent?.commandID !== WORKSPACE_CREATE_COMMAND_ID || !target ||
            !commandPickerMountedRef.current || isModalOpen() || !target.isCurrent()) {
          if (intent?.commandID === WORKSPACE_CREATE_COMMAND_ID) {
            target?.dispose();
            pendingWorkspaceCreateTargetRef.current = null;
            pendingCommandExecutionRef.current = null;
          }
          return intent?.commandID === WORKSPACE_CREATE_COMMAND_ID;
        }
        restoreFocus();
        executePendingCommandRef.current?.();
        return true;
      },
    });
    const globalOwnership = acquireGlobalCommandOwnership({
      subscribe: (listener) => EventsOn('command:global-ownership', listener),
      onChange: () => { void localCommandKeyboardRef.current?.refresh(); },
    });
    // Observe transitions, not just the final slug: A -> B -> A invalidates
    // an outstanding keyboard lease without rebuilding the command map.
    const keyboardProfileKey = () => {
      const current = useWorkspaceStore.getState().workspace;
      return JSON.stringify([current?.id, current?.activeTabId, ReadProfileContext()?.slug]);
    };
    let observedProfileKey = keyboardProfileKey();
    let keyboardProfileRevision = 0;
    const unsubscribeKeyboardProfile = useWorkspaceStore.subscribe(() => {
      const next = keyboardProfileKey();
      if (next !== observedProfileKey) {
        observedProfileKey = next;
        keyboardProfileRevision++;
        localPaletteProfileRevisionRef.current++;
      }
    });
    const captureKeyboardContext = () => {
      if (disposed || pathnameRef.current !== '/' || isModalOpen()) return undefined;
      const activeID = useWorkspaceStore.getState().workspace?.activeTabId;
      if (!activeID) return undefined;
      const owned = trustedSession.readOwnedCommandContextFrame(activeID);
      const surface = owned?.frame.surface;
      if (!owned || !surface || surface.surfaceId !== activeID ||
          !['chat', 'editor', 'terminal', 'tasklist'].includes(surface.surfaceType) || !owned.frame.focus.hasFocus) return undefined;
      const route = commandRouteIdentityRef.current;
      const element = document.activeElement;
      const modalGeneration = getModalRegistrySnapshot().generation;
      const profile = owned.frame.profile?.slug;
      const profileRevision = keyboardProfileRevision;
      return {
        surfaceId: surface.surfaceId,
        surfaceType: surface.surfaceType,
        ...(profile ? { profile } : {}),
        isCurrent: () => {
          if (disposed || pathnameRef.current !== '/' || commandRouteIdentityRef.current !== route ||
              isModalOpen() || getModalRegistrySnapshot().generation !== modalGeneration ||
              useWorkspaceStore.getState().workspace?.activeTabId !== activeID ||
              keyboardProfileRevision !== profileRevision) return false;
          const current = trustedSession.readOwnedCommandContextFrame(activeID);
          const menuIntent = newTabMenuIntentRef.current;
          const ownMenuFocus = !!menuIntent && creationIntentCurrent(menuIntent) &&
            !!document.activeElement?.closest('[data-workspace-tab-creation-menu]');
          return !!current && current.surfaceLease === owned.surfaceLease &&
            current.owner.userId === owned.owner.userId && current.owner.sessionId === owned.owner.sessionId &&
            current.owner.workspaceId === owned.owner.workspaceId &&
            current.frame.profile?.slug === profile &&
            current.frame.surface?.surfaceId === surface.surfaceId && current.frame.surface.surfaceType === surface.surfaceType &&
            current.frame.surface.snapshotVersion === surface.snapshotVersion && current.frame.focus.hasFocus &&
            (document.activeElement === element || ownMenuFocus);
        },
      };
    };
    const localKeyboard = createLocalCommandKeyboard({
      target: window,
      ownedGlobally: globalOwnership.owns,
      readContext: captureKeyboardContext,
      readSurfaceType: () => {
        if (isModalOpen()) return undefined;
        if (pathnameRef.current === '/tasklists') return 'tasklists';
        if (pathnameRef.current === '/profiles') return 'profiles';
        if (pathnameRef.current === '/history') return 'history';
        if (pathnameRef.current !== '/') {
          return trustedSession.readSurfaceContext(COMMAND_TOOLBAR_SURFACE_ID)?.surfaceType;
        }
        const activeID = useWorkspaceStore.getState().workspace?.activeTabId;
        const context = activeID ? trustedSession.readSurfaceContext(activeID) : undefined;
        if (context) return context.surfaceType;
        // O registro do editor também contém fonte viva; ausência não significa
        // "outro contexto" e nunca libera o fallback global.
        const editor = captureEditorPresentationTarget(() => pathnameRef.current);
        if (!editor) return undefined;
        try { return editor.isCurrent() ? 'editor' : undefined; }
        finally { editor.dispose(); }
      },
      loadMap: localPort.loadMap,
      acceptMap: (map) => {
        const auth = useAuthStore.getState();
        const currentWorkspace = useWorkspaceStore.getState().workspace;
        return typeof map.ownerId === 'string' && typeof map.sessionId === 'string' && typeof map.workspaceId === 'string' &&
          auth.isAuthenticated && map.ownerId === auth.user?.userId && map.sessionId === auth.user?.sessionId &&
          currentWorkspace?.id === map.workspaceId;
      },
      onMapAccepted: (map) => {
        localPaletteDeadlineRef.current = map.validUntil ?? 0;
        publishCommandShortcutHints(map);
        localPaletteCommandsRef.current = new Set(map.localPaletteCommands ?? []);
        const contextualConditions = map.contextualPaletteConditions === undefined ? [] : parseLocalPaletteConditions(map.contextualPaletteConditions);
        const validContextualConditions = contextualConditions?.every(condition => isContextualPaletteCommand(condition.commandId) &&
          (!isContextualPagePaletteCommand(condition.commandId) || (condition.bySurfaceId === undefined &&
            Object.values(condition.byProfile ?? {}).every(profile => profile.bySurfaceId === undefined)))) === true;
        contextualPaletteCommandsRef.current = validContextualConditions ? new Set(contextualConditions!.map(condition => condition.commandId)) : null;
        contextualPaletteResolverRef.current = createLocalPaletteConditionResolver(validContextualConditions ? contextualConditions : null);
        localPaletteConditionResolverRef.current = createLocalPaletteConditionResolver(map.localPaletteConditions);
        localKeyboardGenerationRef.current = map.generation;
        localKeyboardOwnerRef.current = {
          ownerId: map.ownerId!, sessionId: map.sessionId!, workspaceId: map.workspaceId!,
        };
      },
      onMapInvalidated: () => {
        clearLocalMapRefs();
      },
      onDown: async (request) => {
        if (request.shortcut.version === 2) {
          const intent = newTabMenuIntentRef.current;
          if ((intent && !creationIntentCurrent(intent)) || (!intent &&
              (isWorkspaceTabCreateCommand(request.commandId) || request.commandId === WORKSPACE_CREATE_COMMAND_ID))) {
            closeCreationMenu();
            return;
          }
          if (intent) closeCreationMenu(true);
        }
        const keyboardContext = request.context ? captureKeyboardContext() : undefined;
        const beginKeyboard = () => keyboardContext
          ? localPort.beginLocalCommandUIKey(request.generation, request.shortcut, request.repeat, keyboardContext)
          : localPort.beginLocalCommandUIKey(request.generation, request.shortcut, request.repeat);
        if (request.context && (!keyboardContext || !keyboardContext.isCurrent() ||
            keyboardContext.surfaceId !== request.context.surfaceId || keyboardContext.surfaceType !== request.context.surfaceType ||
            keyboardContext.profile !== request.context.profile)) return;
        const auth = useAuthStore.getState();
        const currentWorkspace = useWorkspaceStore.getState().workspace;
        const mapOwner = localKeyboardOwnerRef.current;
        const isHelpCommand = request.commandId === HELP_NAVIGATION_COMMAND_ID;
        if (disposed || !auth.isAuthenticated || !auth.user || !currentWorkspace || !mapOwner ||
            mapOwner.ownerId !== auth.user.userId || mapOwner.sessionId !== auth.user.sessionId ||
            mapOwner.workspaceId !== currentWorkspace.id || (!isHelpCommand && !isChatPickerCommand(request.commandId) && request.commandId !== CHAT_CLEAR_COMMAND && !isChatMessagingCommand(request.commandId) && !isTerminalSessionOperationCommand(request.commandId) && !pageMutationAvailable(request.commandId) && !mermaidAvailable(request.commandId) && !landmarkAvailable(request.commandId) && !presentationCommandAvailable(request.commandId) && isModalOpen())) return;
        if (isPageMutationCommand(request.commandId)) {
          if (request.repeat) return;
          const target = capturePageMutationTarget(() => pathnameRef.current, request.commandId);
          if (!target) return;
          await runPageMutation(target, async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('page-mutation-denied');
            return reservation;
          });
          return;
        }
        if (request.commandId === CHAT_CLEAR_COMMAND) {
          await runChatClear(async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('chat-clear-denied');
            return reservation;
          });
          return;
        }
        if (request.commandId === TERMINAL_INTERRUPT_COMMAND) {
          await runTerminalInterrupt(async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('terminal-interrupt-denied');
            return reservation;
          });
          return;
        }
        if (isTerminalSessionOperationCommand(request.commandId)) {
          await runTerminalOperation(request.commandId, async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('terminal-session-denied');
            return reservation;
          });
          return;
        }
        if (isChatMessagingCommand(request.commandId)) {
          if (request.repeat) return;
          const target = captureChatMessagingTarget(() => pathnameRef.current, request.commandId);
          if (!target) return;
          await runChatMessaging(target, async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('chat-messaging-denied');
            return reservation;
          });
          return;
        }
        if (isEditorMermaidCommand(request.commandId)) {
          await runMermaidCommand(request.commandId, async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('mermaid-command-denied');
            return reservation;
          });
          return;
        }
        if (request.handler === 'local_ui') {
          executeLocalUICommand(request.commandId, isHelpCommand);
          return;
        }
        if (isEditorFormatCommand(request.commandId)) {
          await executeFormatCommand(request.commandId, async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('local-format-duplicate');
            return reservation;
          });
          return;
        }
        if (request.handler !== 'contextual' && request.handler !== 'backend') return;
        if (isEditorFileCommand(request.commandId)) {
          await executeFileCommand(request.commandId, async () => {
            const reservation = await beginKeyboard();
            if (!reservation) throw new Error('local-contextual-duplicate');
            return reservation;
          });
          return;
        }
        const intent: PendingCommandIntent = {
          commandID: request.commandId,
          userId: auth.user.userId,
          sessionId: auth.user.sessionId,
          workspaceId: currentWorkspace.id,
          activeTabId: currentWorkspace.activeTabId ?? null,
          routeIdentity: commandRouteIdentityRef.current,
        };
        invalidateLocalPresentation();
        localCommandExecutionRef.current?.dispose();
        localIntent = intent;
        activeCommandIntentRef.current = intent;
        if (request.handler === 'contextual') {
          localCommandContextualExecutionRef.current?.dispose();
          const localContextualExecutor = createCommandContextualBackendExecution(
            {
              ...createCommandWorkspaceTabWailsPort(),
              beginUICommand: async () => {
                const begin = await beginKeyboard();
                if (begin === null) throw new Error('local-contextual-duplicate');
                return begin;
              },
            },
            {
              ...contextOptions,
              canCommitEffect: () => !keyboardContext || keyboardContext.isCurrent(),
              prepareTarget: prepareWorkspaceMutationTarget,
            },
          );
          localCommandContextualExecutionRef.current = localContextualExecutor;
          try {
            await executeContextualCommand(localContextualExecutor, request.commandId);
          } finally {
            if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
            if (localCommandContextualExecutionRef.current === localContextualExecutor) localCommandContextualExecutionRef.current = null;
            localContextualExecutor.dispose();
          }
          return;
        }
        // A porta fecha sobre o request tipado; execute captura o guard antes do primeiro await.
        const localExecutor = createCommandBackendExecution({
          executeCommand: () => localPort.dispatchLocalCommandKey(
            request.generation, request.shortcut, request.kind, request.repeat, ...(keyboardContext ? [keyboardContext] : []),
          ),
        }, keyboardContext ? { ...contextOptions, surfaceID: keyboardContext.surfaceId } : contextOptions);
        localCommandExecutionRef.current = localExecutor;
        try {
          const result = await localExecutor.execute(request.commandId, (output) =>
            presentWorkspaceListRef.current(intent, output));
          if (isCommandLayerAction(request.commandId) && result.presented && intentIsCurrent(intent)) {
            creationPresentationRef.current.announce(creationPresentationRef.current.t('commandSettings.layerActionCompleted'));
          }
        } finally {
          if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
          if (localCommandExecutionRef.current === localExecutor) localCommandExecutionRef.current = null;
          localExecutor.dispose();
        }
      },
      onUp: async (request) => {
        // O host também precisa observar o release de bindings UI; a navegação
        // pode permanecer na mesma superfície e não desmontar o adapter.
        if (request.handler === 'local_ui') return;
        await localPort.dispatchLocalCommandKey(request.generation, request.shortcut, request.kind, request.repeat);
      },
      reset: localPort.resetLocalCommandKeyboard,
      onSequenceStarted: (bindings) => {
        const creationBindings = bindings.filter(binding =>
          isWorkspaceTabCreateCommand(binding.commandId) || binding.commandId === WORKSPACE_CREATE_COMMAND_ID);
        if (creationBindings.length > 0) openCreationMenu(creationBindings);
      },
      onSequenceCancelled: (reason) => {
        if (reason !== 'menu-navigation') {
          const focusInMenu = Boolean(document.activeElement?.closest('[data-workspace-tab-creation-menu]'));
          closeCreationMenu((reason === 'escape' || reason === 'timeout') && focusInMenu);
        }
      },
      canHandle: (commandID, event) => {
        if (isLandmarkNavigationCommand(commandID)) return !event.isComposing && event.keyCode !== 229 && landmarkAvailable(commandID);
        if (isCapturedPresentationCommand(commandID)) return !event.isComposing && event.keyCode !== 229 && presentationCommandAvailable(commandID);
        if (isEditorMermaidCommand(commandID)) return !event.repeat && !event.isComposing && event.keyCode !== 229 && mermaidAvailable(commandID);
        if (isChatMessagingCommand(commandID)) return !event.repeat && !event.isComposing && event.keyCode !== 229 && chatMessagingAvailable(commandID, event.target);
        if (commandID === CHAT_CLEAR_COMMAND) return chatClearAvailable(event.target);
        if (isPageMutationCommand(commandID)) return !event.repeat && !event.isComposing && pageMutationAvailable(commandID);
        if (commandID === TERMINAL_INTERRUPT_COMMAND || isTerminalSessionOperationCommand(commandID)) return !event.repeat && !event.isComposing && terminalOperationAvailable(commandID);
        if (isEditorFormatCommand(commandID)) return event.target instanceof Element &&
          !!event.target.closest('.rich-text-editor, .monaco-editor') && editorFormatAvailable(commandID);
        if (isEditorFileCommand(commandID)) return editorFileAvailable(commandID);
        if (isEditorModeCommand(commandID)) return workspaceMutationAvailable(commandID);
        if (isEditorPresentationCommand(commandID)) return editorPresentationAvailable(commandID, event.target);
        if (isChatPickerCommand(commandID)) return chatPickerAvailable(commandID, event.target);
        const creationIntent = newTabMenuIntentRef.current;
        if (creationIntent && !creationIntentCurrent(creationIntent)) return false;
        if ((isWorkspaceTabCreateCommand(commandID) || commandID === WORKSPACE_CREATE_COMMAND_ID) &&
            event.target instanceof Element && event.target.closest('.datagrid-container, [data-tab-scope]')) return false;
        if (isWorkspaceTabNavigationCommand(commandID) && event.target instanceof Element &&
            event.target.closest('.datagrid-container, [data-tab-scope]')) return false;
        if (commandID === WORKSPACE_TAB_CLOSE_COMMAND_ID && event.target instanceof Element &&
            event.target.closest('.datagrid-container')) return false;
        return commandID !== WORKSPACE_PANEL_FOCUS_COMMAND_ID
          ? (isWorkspaceTabNavigationCommand(commandID)
            ? workspaceTabNavigationAvailable(commandID)
            : !isWorkspaceMutationCommand(commandID) || workspaceMutationAvailable(commandID))
          : workspacePanelFocusAvailable();
      },
      canHandleEditable: (commandID, event) => {
        if (isPageMutationCommand(commandID)) return !event.repeat && !event.isComposing && pageMutationAvailable(commandID);
        if (commandID === TERMINAL_INTERRUPT_COMMAND || isTerminalSessionOperationCommand(commandID)) return !event.repeat && !event.isComposing && terminalOperationAvailable(commandID);
        if (commandID === 'tasklists.create.open' || commandID === 'profiles.create.open') return false;
        if (isLandmarkNavigationCommand(commandID)) return !event.isComposing && event.keyCode !== 229 && landmarkAvailable(commandID);
        if (isCapturedPresentationCommand(commandID)) return !event.isComposing && event.keyCode !== 229 && presentationCommandAvailable(commandID);
        if (isEditorMermaidCommand(commandID)) return !event.repeat && !event.isComposing && event.keyCode !== 229 && mermaidAvailable(commandID);
        if (isChatMessagingCommand(commandID)) return !event.repeat && !event.isComposing && event.keyCode !== 229 &&
          event.target instanceof Element &&
          (commandID === 'chat.message.edit.save'
            ? event.target instanceof HTMLTextAreaElement && !!event.target.closest('.message-node')
            : !!event.target.closest('[data-testid="chat-input"]')) && chatMessagingAvailable(commandID, event.target);
        if (commandID === CHAT_CLEAR_COMMAND) return event.target instanceof Element &&
          !!event.target.closest('[data-testid="chat-input"]') && chatClearAvailable();
        const target = event.target;
        const isSupportedEditable = isCommandNavigationTextField(target);
        const isChatContextEditable = target instanceof Element &&
          !!target.closest('.monaco-editor, [contenteditable="true"]');
        if (event.isComposing || event.keyCode === 229) return false;
        if (isEditorFormatCommand(commandID)) return target instanceof Element &&
          !!target.closest('.rich-text-editor, .monaco-editor') && editorFormatAvailable(commandID);
        if (isEditorFileCommand(commandID)) return editorFileAvailable(commandID);
        if (isEditorModeCommand(commandID)) return workspaceMutationAvailable(commandID);
        if (isEditorPresentationCommand(commandID)) return editorPresentationAvailable(commandID, target);
        if (isChatPickerCommand(commandID)) return chatPickerAvailable(commandID, target);
        if (isCommandLayerAction(commandID)) return isSupportedEditable;
        if (isCommandNavigation(commandID) || isWorkspaceTabNavigationCommand(commandID)) {
          return isSupportedEditable && workspaceTabNavigationAvailable(commandID);
        }
        return isWorkspaceMutationCommand(commandID) &&
          (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID
            ? (isSupportedEditable || isChatContextEditable)
            : isSupportedEditable) &&
          workspaceMutationAvailable(commandID);
      },
      canRepeat: (commandID) => isWorkspaceTabNavigationCommand(commandID) || isEditorCellNavigationCommand(commandID),
      blocked: (commandID?: string) => disposed || !globalOwnership.isReady() || !useAuthStore.getState().isAuthenticated ||
        (commandID !== HELP_NAVIGATION_COMMAND_ID && isModalOpen() &&
          !(commandID && (pageMutationAvailable(commandID) || presentationCommandAvailable(commandID) || landmarkAvailable(commandID) || mermaidAvailable(commandID) || chatPickerAvailable(commandID) || (isChatMessagingCommand(commandID) && chatMessagingAvailable(commandID)) || (commandID === CHAT_CLEAR_COMMAND && chatClearAvailable())))),
    });
    localCommandKeyboardRef.current = localKeyboard;
    clearLocalMapRefs();
    localKeyboardMapReadyRef.current = localKeyboard.refresh();
    const unsubscribeKeyboardMap = EventsOn('command:keyboard-map-changed', () => {
      if (disposed) return;
      cancelCreationMenu();
      invalidateLocalPresentation();
      clearLocalMapRefs();
      localKeyboardMapReadyRef.current = localKeyboard.refresh();
    });
    const voiceInputPort = createCommandVoiceInputWailsPort({
      isGlobalJobAdmissionCurrent: (capturedModalGeneration) => {
        if (disposed) return false;
        const modal = getModalRegistrySnapshot();
        if (modal.topID !== null || modal.generation !== capturedModalGeneration) return false;
        const auth = useAuthStore.getState();
        return auth.isAuthenticated && !!auth.user && globalOwnership.isReady();
      },
    });
    const cancelGlobalVoiceReservation = (ticket: string) => {
      void voiceInputPort.cancelUICommand(ticket).catch(() => undefined);
    };
    const unsubscribeGlobalVoiceReservation = EventsOn('command:global-ui-reservation', (payload: unknown) => {
      const reservation = parseVoiceInputReservation(payload);
      if (!reservation || reservation.commandId !== VOICE_INPUT_COMMAND) {
        return;
      }
      const auth = useAuthStore.getState();
      if (disposed || isModalOpen() || !auth.isAuthenticated || !auth.user) {
        cancelGlobalVoiceReservation(reservation.ticket);
        return;
      }
      if (globalVoiceTicketsRef.current.has(reservation.ticket)) return;
      globalVoiceTicketsRef.current.add(reservation.ticket);
      void executeGlobalVoiceReservation(reservation, voiceInputPort)
        .catch(() => undefined)
        .finally(() => globalVoiceTicketsRef.current.delete(reservation.ticket));
    });
    const unsubscribeGlobalJobAdmission = EventsOn('command:global-job-admission', (payload: unknown) => {
      if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return;
      const invocationId = (payload as { invocationId?: unknown }).invocationId;
      if (typeof invocationId !== 'string' || invocationId.length === 0 || invocationId.trim() !== invocationId) return;
      const modal = getModalRegistrySnapshot();
      const auth = useAuthStore.getState();
      const admitted = !disposed && modal.topID === null && auth.isAuthenticated && !!auth.user && globalOwnership.isReady();
      void voiceInputPort.admitGlobalCommandOccurrence(invocationId, admitted, modal.generation).catch(() => undefined);
    });
    const unsubscribeDeckReservation = EventsOn('command:deck-ui-reservation', (payload: unknown) => {
      const reservation = payload as Partial<{ ticket: string; invocationId: string; commandId: string }> | null;
      if (reservation && typeof reservation.commandId === 'string' && isEditorMermaidCommand(reservation.commandId) &&
          reservation.commandId !== 'editor.mermaid.open' && typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        const admitted = { ticket: reservation.ticket, invocationId: reservation.invocationId, commandId: reservation.commandId };
        if (mermaidDeckTicketsRef.current.has(admitted.ticket)) return;
        mermaidDeckTicketsRef.current.add(admitted.ticket);
        void runMermaidCommand(admitted.commandId, async () => admitted).then(result => {
          if (!result.invocationId) void createCommandUIExecutionWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined);
        }).catch(() => { void createCommandUIExecutionWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined); })
          .finally(() => mermaidDeckTicketsRef.current.delete(admitted.ticket));
        return;
      }
      if (reservation && isChatMessagingCommand(reservation.commandId) && typeof reservation.ticket === 'string' &&
          reservation.ticket && typeof reservation.invocationId === 'string' && reservation.invocationId) {
        const admitted = { ticket: reservation.ticket, invocationId: reservation.invocationId, commandId: reservation.commandId };
        const target = captureChatMessagingTarget(() => pathnameRef.current, admitted.commandId);
        if (!target || !target.canCommit()) {
          target?.dispose();
          void createCommandWorkspaceTabWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined);
          return;
        }
        void runChatMessaging(target, async () => admitted).then(status => {
          if (status === 'cancelled') void createCommandWorkspaceTabWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined);
        }).catch(() => undefined);
        return;
      }
      if (reservation && typeof reservation.commandId === 'string' && isPageMutationCommand(reservation.commandId) && typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        const admitted = { ticket: reservation.ticket, invocationId: reservation.invocationId, commandId: reservation.commandId };
        if (pageMutationDeckTicketsRef.current.has(admitted.ticket)) return;
        pageMutationDeckTicketsRef.current.add(admitted.ticket);
        const target = capturePageMutationTarget(() => pathnameRef.current, admitted.commandId);
        const operation = target ? runPageMutation(target, async () => admitted) : Promise.resolve({ status: 'cancelled' });
        void operation.then(result => { if (result.status === 'cancelled') return createPageMutationWailsPort().cancelUICommand(admitted.ticket); })
          .catch(() => undefined).finally(() => pageMutationDeckTicketsRef.current.delete(admitted.ticket));
        return;
      }
      if (reservation?.commandId === CHAT_CLEAR_COMMAND && typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        void runChatClear(async () => ({ ticket: reservation.ticket!, invocationId: reservation.invocationId!, commandId: CHAT_CLEAR_COMMAND }))
          .then(status => { if (status === 'cancelled') return createCommandUIExecutionWailsPort().cancelUICommand(reservation.ticket!); }).catch(() => undefined);
        return;
      }
      if (reservation?.commandId === TERMINAL_INTERRUPT_COMMAND && typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        const ticket = reservation.ticket;
        if (terminalDeckTicketsRef.current.has(ticket)) return;
        terminalDeckTicketsRef.current.add(ticket);
        void runTerminalInterrupt(async () => ({ ticket: reservation.ticket!, invocationId: reservation.invocationId!, commandId: TERMINAL_INTERRUPT_COMMAND }))
          .then(status => { if (status === 'cancelled') return createCommandUIExecutionWailsPort().cancelUICommand(ticket); })
          .catch(() => undefined).finally(() => terminalDeckTicketsRef.current.delete(ticket));
        return;
      }
      if (typeof reservation?.commandId === 'string' && isTerminalSessionOperationCommand(reservation.commandId) &&
          typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        const ticket = reservation.ticket;
        if (terminalDeckTicketsRef.current.has(ticket)) return;
        terminalDeckTicketsRef.current.add(ticket);
        const target = captureTerminalOperationTarget(() => pathnameRef.current, undefined, reservation.commandId);
        const operation = target
          ? runTerminalOperation(reservation.commandId, async () => ({ ticket, invocationId: reservation.invocationId!, commandId: reservation.commandId! }), target)
          : Promise.resolve('cancelled' as const);
        void operation.then(status => {
          if (status === 'cancelled') return createCommandUIExecutionWailsPort().cancelUICommand(ticket);
        }).catch(() => undefined).finally(() => terminalDeckTicketsRef.current.delete(ticket));
        return;
      }
      if (reservation && typeof reservation.commandId === 'string' && isEditorFormatCommand(reservation.commandId) &&
          typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        const admitted = { ticket: reservation.ticket, invocationId: reservation.invocationId, commandId: reservation.commandId };
        void executeFormatCommand(admitted.commandId, async () => admitted).then(result => {
          if (!result.invocationId) void createCommandUIExecutionWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined);
        }).catch(() => { void createCommandUIExecutionWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined); });
        return;
      }
      if (reservation && typeof reservation.commandId === 'string' && isEditorFileCommand(reservation.commandId) &&
          typeof reservation.ticket === 'string' && typeof reservation.invocationId === 'string') {
        const admitted = { ticket: reservation.ticket, invocationId: reservation.invocationId, commandId: reservation.commandId };
        void executeFileCommand(admitted.commandId, async () => admitted).then(result => {
          if (!result.invocationId) void createCommandUIExecutionWailsPort().cancelUICommand(admitted.ticket).catch(() => undefined);
        });
        return;
      }
      const isContextualReservation = isWorkspaceMutationCommand(reservation?.commandId);
      if (!reservation || typeof reservation.ticket !== 'string' || typeof reservation.invocationId !== 'string' ||
          typeof reservation.commandId !== 'string' || !isContextualReservation ||
          !workspaceMutationAvailable(reservation.commandId) ||
          isModalOpen() || !useAuthStore.getState().isAuthenticated || !useWorkspaceStore.getState().workspace ||
          !commandPickerMountedRef.current || !document.hasFocus()) {
        if (reservation && typeof reservation.ticket === 'string') {
          void createCommandUIExecutionWailsPort().cancelUICommand(reservation.ticket);
        }
        return;
      }
      const deckExecutor = createCommandContextualBackendExecution({
            ...createCommandWorkspaceTabWailsPort(),
            // A Stream Deck reservation is already admitted by the host. The
            // frontend must only Take/execute/commit it; never Begin again.
            beginUICommand: async () => ({
              ticket: reservation.ticket!, invocationId: reservation.invocationId!, commandId: reservation.commandId!,
            }),
          }, {
            trustedSession,
            surfaceID: COMMAND_TOOLBAR_SURFACE_ID,
            canCommitEffect: () => !isModalOpen() && document.hasFocus(),
            prepareTarget: prepareWorkspaceMutationTarget,
          });
      localCommandContextualExecutionRef.current?.dispose();
      localCommandContextualExecutionRef.current = deckExecutor;
      void executeContextualCommand(deckExecutor, reservation.commandId).then(async (result) => {
        // A reserva física já existe mesmo se o preparo local recusar antes de
        // chamar o Begin emprestado. Nesse caso, liberar a reserva sem Take.
        if (!result.invocationId) {
          await Promise.resolve(createCommandUIExecutionWailsPort().cancelUICommand(reservation.ticket!)).catch(() => undefined);
        }
      }).finally(() => {
        if (localCommandContextualExecutionRef.current === deckExecutor) localCommandContextualExecutionRef.current = null;
        deckExecutor.dispose();
      });
    });
    const readDeckVisualSnapshot = (explicitSurfaceId?: string) => {
      const focusedElement = document.activeElement;
      const registeredSource = commandScope?.surfaceForElement(focusedElement);
      if (!registeredSource && focusedElement?.closest('.ws-content__panel')) return null;
      const surfaceId = explicitSurfaceId ?? registeredSource ?? COMMAND_TOOLBAR_SURFACE_ID;
      const modalId = getModalRegistrySnapshot().topID;
      const owned = explicitSurfaceId && modalId
        ? commandScope?.readOwnedCommandOriginMetadata(surfaceId, modalId)
        : trustedSession.readOwnedCommandContextFrame(surfaceId);
      const surface = owned?.frame.surface;
      if (!owned || !surface || surface.surfaceId !== surfaceId || !owned.frame.focus.hasFocus || !document.hasFocus()) return null;
      const profile = owned.frame.profile?.slug;
      return {
        owner: owned.owner,
        surfaceLease: owned.surfaceLease,
        routeIdentity: commandRouteIdentityRef.current,
        generation: localKeyboardGenerationRef.current,
        profileRevision: localPaletteProfileRevisionRef.current,
        focusedElement,
        modalGeneration: getModalRegistrySnapshot().generationNumber,
        surface,
        context: {
          surfaceType: surface.surfaceType,
          surfaceId: surface.surfaceId,
          ...(profile ? { profile } : {}),
        } satisfies LocalCommandPaletteVisualContext,
      };
    };
    const consumedDeckOffers = new Set<string>();
    let deckOfferGeneration: string | null = null;
    const unsubscribeDeckContextualUI = EventsOn('command:deck-contextual-ui', (payload: unknown) => {
      if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return;
      const value = payload as Record<string, unknown>;
      if (typeof value.offerId !== 'string' || !value.offerId || typeof value.generation !== 'string' || !value.generation ||
          typeof value.userId !== 'string' || typeof value.sessionId !== 'string' || typeof value.workspaceId !== 'string') return;
      const offerId = value.offerId;
      const generation = value.generation;
      let mermaidCaptured: ReturnType<typeof captureContextualEditorMermaidTarget>;
      let mermaidHandedOff = false;
      try {
      const startedInModal = isModalOpen();
      if (startedInModal) mermaidCaptured = captureContextualEditorMermaidTarget('editor.mermaid.apply');
      if (startedInModal && !mermaidCaptured) return;
      const first = readDeckVisualSnapshot(mermaidCaptured?.documentId);
      if (!first) return;
      const deadline = localPaletteDeadlineRef.current;
      const scalarCurrent = (native = false) => {
        const auth = useAuthStore.getState();
        const workspace = useWorkspaceStore.getState().workspace;
        const owner = localKeyboardOwnerRef.current;
        return !disposed && commandPickerMountedRef.current && auth.isAuthenticated &&
          auth.user?.userId === value.userId && auth.user?.sessionId === value.sessionId &&
          first.owner.userId === value.userId && first.owner.sessionId === value.sessionId && first.owner.workspaceId === value.workspaceId &&
          workspace?.id === value.workspaceId && commandRouteIdentityRef.current === first.routeIdentity &&
          localPaletteProfileRevisionRef.current === first.profileRevision &&
          (deadline === 0 || Date.now() < deadline) && document.hasFocus() &&
          (native || (owner?.ownerId === value.userId && owner.sessionId === value.sessionId && owner.workspaceId === value.workspaceId &&
            localKeyboardGenerationRef.current === generation && first.generation === generation));
      };
      const sameSource = (native = false) => {
        if (!scalarCurrent(native) || isModalOpen() && !mermaidCaptured) return false;
        const next = readDeckVisualSnapshot(isModalOpen() ? mermaidCaptured?.documentId : undefined);
        return !!next && next.surfaceLease === first.surfaceLease && next.owner.userId === first.owner.userId &&
          next.owner.sessionId === first.owner.sessionId && next.owner.workspaceId === first.owner.workspaceId &&
          next.surface.surfaceId === first.surface.surfaceId && next.surface.surfaceType === first.surface.surfaceType &&
          next.surface.snapshotVersion === first.surface.snapshotVersion && next.context.profile === first.context.profile &&
          (mermaidCaptured ? mermaidCaptured.target.isCurrent() &&
            (isModalOpen() ? !!commandId && mermaidCaptured.target.canExecute(commandId) : commandScope?.surfaceForElement(next.focusedElement) === first.surface.surfaceId) :
            next.focusedElement === first.focusedElement) && scalarCurrent(native);
      };
      if (!scalarCurrent()) return;
      const commandId = selectContextualDeckCommand(value.conditions, first.context);
      if (startedInModal && !isEditorMermaidMutation(commandId)) return;
      if (isEditorMermaidMutation(commandId)) {
        if (!startedInModal || commandId !== 'editor.mermaid.apply') {
          mermaidCaptured?.target.dispose();
          mermaidCaptured = captureContextualEditorMermaidTarget(commandId);
        }
        if (!mermaidCaptured || mermaidCaptured.documentId !== first.surface.surfaceId || first.surface.surfaceType !== 'editor') return;
      }
      if (!commandId || !sameSource() || getModalRegistrySnapshot().generationNumber !== first.modalGeneration) return;
      if (deckOfferGeneration !== generation) { consumedDeckOffers.clear(); deckOfferGeneration = generation; }
      if (consumedDeckOffers.has(offerId) || consumedDeckOffers.size >= 4096) return;
      consumedDeckOffers.add(offerId);
      if (isLocalUICommand(commandId)) {
        if (commandId === 'chat.message.edit.open') {
          const target = captureChatMessagingTarget(() => pathnameRef.current, commandId);
          if (target && sameSource()) void runChatMessaging(target).catch(() => undefined);
          else target?.dispose();
        } else executeLocalUICommand(commandId);
        return;
      }
      const workspace = useWorkspaceStore.getState().workspace;
      const pageCommand = isContextualPagePaletteCommand(commandId);
      const standalonePage = pageCommand && (first.surface.surfaceType === 'tasklists' || first.surface.surfaceType === 'profiles');
      if (!isContextualDeckCommand(commandId) || !startedInModal && commandScope?.surfaceForElement(first.focusedElement) !== first.surface.surfaceId) return;
      if (standalonePage) {
        if (pathnameRef.current !== `/${first.surface.surfaceType}` || !isContextualPagePaletteSurface(commandId, first.surface.surfaceType)) return;
      } else if ((pathnameRef.current !== '/' && pathnameRef.current !== '') ||
          !['chat', 'editor', 'terminal', 'tasklist'].includes(first.surface.surfaceType) || workspace?.activeTabId !== first.surface.surfaceId ||
          (pageCommand && !isContextualPagePaletteSurface(commandId, first.surface.surfaceType))) return;
      const isCurrent = () => {
        if (!sameSource() || !standalonePage && useWorkspaceStore.getState().workspace?.activeTabId !== first.surface.surfaceId) return false;
        const composition = ReadFocusContext().composition;
        // The captured Mermaid target alone can prove its own focus restoration.
        // Initial unknown is refused; the physical target permanently revokes
        // this continuation on any genuine window blur or composition start.
        return composition === 'inactive' || composition === 'unknown' && isEditorMermaidMutation(commandId) &&
          mermaidCaptured?.target.hasPreparedFocus?.() === true;
      };
      if (!isCurrent()) return;
      if (isCommandLayerAction(commandId)) {
        const executor = createCommandBackendExecution(createContextualDeckLayerWailsPort({
          offerId, generation, commandId, observed: first.context,
          isCurrent: () => isCurrent() && getModalRegistrySnapshot().generationNumber === first.modalGeneration,
        }), { trustedSession, surfaceID: first.surface.surfaceId });
        const canPresent = () => {
          const sameOwnerAndOrigin = () => {
            const auth = useAuthStore.getState();
            const currentWorkspace = useWorkspaceStore.getState().workspace;
            return !disposed && commandPickerMountedRef.current && auth.isAuthenticated &&
              auth.user?.userId === first.owner.userId && auth.user.sessionId === first.owner.sessionId &&
              currentWorkspace?.id === first.owner.workspaceId && currentWorkspace.activeTabId === first.surface.surfaceId &&
              commandRouteIdentityRef.current === first.routeIdentity && localPaletteProfileRevisionRef.current === first.profileRevision;
          };
          if (!sameOwnerAndOrigin()) return false;
          const owned = trustedSession.readOwnedCommandContextFrame(first.surface.surfaceId);
          return !!owned && owned.owner.userId === first.owner.userId && owned.owner.sessionId === first.owner.sessionId &&
            owned.owner.workspaceId === first.owner.workspaceId && owned.surfaceLease === first.surfaceLease &&
            owned.frame.surface?.surfaceId === first.surface.surfaceId && owned.frame.surface.surfaceType === first.surface.surfaceType &&
            owned.frame.surface.snapshotVersion === first.surface.snapshotVersion && owned.frame.profile?.slug === first.context.profile &&
            sameOwnerAndOrigin();
        };
        void executor.execute(commandId, () => undefined).then(result => {
          // Layer publication retires this offer's map. It cannot undo a
          // succeeded backend result or cause re-submission to another source.
          if (!canPresent()) return;
          if (result.execution.status === 'succeeded') {
            creationPresentationRef.current.announce(creationPresentationRef.current.t('commandSettings.layerActionCompleted'));
          } else if (result.reason !== 'context-stale' && result.reason !== 'disposed') {
            const key = result.execution.status === 'denied' || result.execution.status === 'rejected_stale'
              ? 'commandPalette.unavailable' : 'commandPalette.executionUnknown';
            creationPresentationRef.current.announce(creationPresentationRef.current.t(key));
          }
        }).finally(() => executor.dispose());
        return;
      }
      let nativePrepared = false;
      const lease = createContextualDeckLease({ offerId, generation, commandId, observed: first.context, isCurrent,
        ...(isEditorFileCommand(commandId) ? { prepareNativeFileContinuation: () => {
          if (nativePrepared || !isCurrent()) return undefined;
          nativePrepared = true;
          return () => sameSource(true) && useWorkspaceStore.getState().workspace?.activeTabId === first.surface.surfaceId &&
            ReadFocusContext().composition === 'inactive';
        } } : {}),
      });
      // Every domain captures its original target now, before flush/Begin/native
      // preparation. The physical lease is passed explicitly, never via palette refs.
      if (isEditorMermaidMutation(commandId) && mermaidCaptured) {
        mermaidHandedOff = true;
        void runMermaidCommand(commandId, undefined, mermaidCaptured.target, lease).catch(() => undefined);
      } else if (pageCommand) {
        const target = capturePageMutationTarget(() => pathnameRef.current, commandId, standalonePage ? first.surface.surfaceId : undefined);
        if (target) void runPageMutation(target, undefined, lease).catch(() => undefined);
      } else if (commandId === CHAT_CLEAR_COMMAND) {
        const target = captureChatClearTarget(() => pathnameRef.current);
        if (target) void runChatClear(undefined, target, lease).catch(() => undefined);
      } else if (commandId === TERMINAL_INTERRUPT_COMMAND || isTerminalSessionOperationCommand(commandId)) {
        const target = captureTerminalOperationTarget(() => pathnameRef.current, undefined, commandId);
        if (target) void runTerminalOperation(commandId, undefined, target, lease).catch(() => undefined);
      } else if (isChatMessagingCommand(commandId)) {
        const target = captureChatMessagingTarget(() => pathnameRef.current, commandId);
        if (target) void runChatMessaging(target, undefined, lease).catch(() => undefined);
      } else if (isEditorFormatCommand(commandId)) {
        const target = captureEditorFormatting();
        if (target) void executeFormatCommand(commandId, undefined, target, undefined, lease).catch(() => undefined);
      } else if (isEditorFileCommand(commandId)) {
        const target = captureEditorFileTarget();
        if (target) void executeFileCommand(commandId, undefined, lease, target).catch(() => undefined);
      } else if (isWorkspaceMutationCommand(commandId)) {
        const target = commandId === WORKSPACE_CHAT_OPEN_COMMAND_ID ? undefined : prepareWorkspaceMutationTarget(commandId);
        if (!target && commandId !== WORKSPACE_CHAT_OPEN_COMMAND_ID) return;
        const executor = createCommandContextualBackendExecution(createCommandWorkspaceTabWailsPort({ contextualPalette: lease }), {
          trustedSession, surfaceID: first.surface.surfaceId, canCommitEffect: isCurrent,
          prepareTarget: id => {
            if (id !== commandId || !isCurrent()) return undefined;
            const captured = target ?? prepareWorkspaceMutationTarget(id);
            return captured && { ...captured, isCurrent: () => isCurrent() && captured.isCurrent() };
          },
        });
        contextualPaletteExecutorsRef.current.add(executor);
        void executeContextualCommand(executor, commandId, target, lease).catch(() => undefined).finally(() => {
          executor.dispose(); target?.dispose(); contextualPaletteExecutorsRef.current.delete(executor);
        });
      }
      } finally { if (!mermaidHandedOff) mermaidCaptured?.target.dispose(); }
    });
    const unsubscribeDeckLocalUI = EventsOn('command:deck-local-ui', (payload: unknown) => {
      const value = payload as Partial<{
        commandId: string;
        generation: string;
        userId: string;
        sessionId: string;
        workspaceId: string;
        conditions: unknown;
      }> | null;
      const auth = useAuthStore.getState();
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      if (!value || typeof value.commandId !== 'string' || typeof value.generation !== 'string' ||
          !auth.isAuthenticated || value.userId !== auth.user?.userId || value.sessionId !== auth.user?.sessionId ||
          !currentWorkspace || value.workspaceId !== currentWorkspace.id ||
          !localKeyboardOwnerRef.current || localKeyboardOwnerRef.current.ownerId !== value.userId ||
          localKeyboardOwnerRef.current.sessionId !== value.sessionId || localKeyboardOwnerRef.current.workspaceId !== value.workspaceId ||
          value.generation !== localKeyboardGenerationRef.current ||
          (localPaletteDeadlineRef.current !== 0 && Date.now() >= localPaletteDeadlineRef.current) || !document.hasFocus()) return;

      let commandId: string | null = null;
      if (value.conditions !== undefined) {
        if (value.commandId !== '') return;
        const first = readDeckVisualSnapshot();
        if (!first) return;
        const parsedConditions = parseLocalPaletteConditions(value.conditions);
        if (!parsedConditions) return;
        const resolveConditions = createLocalPaletteConditionResolverFromParsed(parsedConditions);
        const firstCommand = resolveLocalDeckConditionCommandFromParsed(parsedConditions, first.context);
        if (!firstCommand) return;
        const second = readDeckVisualSnapshot();
        if (!second || first.owner.userId !== second.owner.userId || first.owner.sessionId !== second.owner.sessionId ||
            first.owner.workspaceId !== second.owner.workspaceId || first.surfaceLease !== second.surfaceLease ||
            first.routeIdentity !== second.routeIdentity || first.generation !== second.generation ||
            first.profileRevision !== second.profileRevision || first.surface.surfaceId !== second.surface.surfaceId ||
            first.surface.surfaceType !== second.surface.surfaceType || first.surface.snapshotVersion !== second.surface.snapshotVersion ||
            first.context.profile !== second.context.profile || first.focusedElement !== second.focusedElement ||
            first.modalGeneration !== second.modalGeneration || second.owner.userId !== value.userId ||
            second.owner.sessionId !== value.sessionId || second.owner.workspaceId !== value.workspaceId ||
            second.owner.userId !== localKeyboardOwnerRef.current?.ownerId ||
            second.owner.sessionId !== localKeyboardOwnerRef.current?.sessionId ||
            second.owner.workspaceId !== localKeyboardOwnerRef.current?.workspaceId ||
            second.generation !== value.generation || second.generation !== localKeyboardGenerationRef.current ||
            firstCommand !== (resolveConditions(firstCommand, second.context) ? firstCommand : null)) return;
        if (localPaletteDeadlineRef.current !== 0 && Date.now() >= localPaletteDeadlineRef.current) return;
        commandId = firstCommand;
      } else {
        if (!isLocalUICommand(value.commandId)) return;
        commandId = value.commandId;
      }
      if (!commandId || (isModalOpen() && !presentationCommandAvailable(commandId) && !landmarkAvailable(commandId) && !chatPickerAvailable(commandId) && !(commandId === 'chat.message.edit.open' && chatMessagingAvailable(commandId)))) return;
      if (commandId === 'chat.message.edit.open') {
        const target = captureChatMessagingTarget(() => pathnameRef.current, commandId);
        if (target) void runChatMessaging(target).catch(() => undefined);
        return;
      }
      executeLocalUICommand(commandId);
    });
    const refreshKeyboardOnFocus = (event: FocusEvent) => {
      if (event.target !== event.currentTarget || disposed) return;
      invalidateLocalPresentation();
      localKeyboardMapReadyRef.current = localKeyboard.refresh();
    };
    const cancelKeyboardOnBlur = (event: FocusEvent) => {
      if (event.target === event.currentTarget) {
        cancelCreationMenu();
        invalidateLocalPresentation();
        clearLocalMapRefs();
      }
    };
    window.addEventListener('focus', refreshKeyboardOnFocus, true);
    window.addEventListener('blur', cancelKeyboardOnBlur, true);
    const invalidateIntentIfContextChanged = () => {
      const creationIntent = newTabMenuIntentRef.current;
      if (creationIntent && !creationIntentCurrent(creationIntent)) cancelCreationMenu();
      const intent = pendingCommandExecutionRef.current ?? activeCommandIntentRef.current ?? workspacePickerIntentRef.current;
      const auth = useAuthStore.getState();
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      if (
        intent && (
          !auth.isAuthenticated ||
          auth.user?.userId !== intent.userId ||
          auth.user?.sessionId !== intent.sessionId ||
          currentWorkspace?.id !== intent.workspaceId ||
          (currentWorkspace?.activeTabId ?? null) !== intent.activeTabId ||
          commandRouteIdentityRef.current !== intent.routeIdentity
        )
      ) {
        pendingCommandExecutionRef.current = null;
        activeCommandIntentRef.current = null;
        workspacePickerIntentRef.current = null;
        pendingCommandCatalogRef.current = null;
        commandPaletteDismissActionRef.current = 'ignore';
        setCommandPaletteOpen(false);
        commandPaletteMenuItemsRef.current = [];
        setCommandPaletteItems([]);
        backendExecutor.cancelPresentation();
        invalidateLocalPresentation();
      }
    };
    const unsubscribeAuth = useAuthStore.subscribe(invalidateIntentIfContextChanged);
    const unsubscribeWorkspace = useWorkspaceStore.subscribe(invalidateIntentIfContextChanged);
    commandUIEffectGuardRef.current = guard;
    return () => {
      disposed = true;
      activeChatMessagingRef.current.forEach(target => target.dispose());
      activeChatMessagingRef.current.clear();
      clearPaletteChatMessaging();
      pendingChatMessagingRef.current?.dispose(); pendingChatMessagingRef.current = null;
      window.removeEventListener(EDITOR_MODE_COMMAND_EVENT, requestEditorMode);
      window.removeEventListener('commands:editor-file', requestEditorFile);
      activeEditorFormatTargetRef.current?.dispose();
      activeMermaidRef.current?.dispose(); activeMermaidRef.current = null;
      pendingMermaidRef.current?.dispose(); pendingMermaidRef.current = null;
      activeEditorFormatTargetRef.current = null;
      pendingEditorFormatTargetRef.current?.dispose();
      pendingEditorFormatTargetRef.current = null;
      pendingEditorFileTargetRef.current?.dispose();
      pendingEditorFileTargetRef.current = null;
      editorModeTimers.forEach(timer => clearTimeout(timer));
      clearPaletteEditorModeTargets();
      pendingEditorModeTargetRef.current?.dispose();
      pendingEditorModeTargetRef.current = null;
      activeEditorModeTargetRef.current?.dispose();
      activeEditorModeTargetRef.current = null;
      cancelCreationMenu();
      unregisterCreationActions?.();
      commandCancellationGenerationRef.current += 1;
      commandPickerMountedRef.current = false;
      unsubscribeAuth();
      unsubscribeWorkspace();
      guard.dispose();
      contextualPaletteExecutorsRef.current.forEach(executor => executor.dispose());
      contextualPaletteExecutorsRef.current.clear();
      contextualExecutor.dispose();
      backendExecutor.dispose();
      localCommandExecutionRef.current?.dispose();
      localCommandContextualExecutionRef.current?.dispose();
      localKeyboard.dispose();
      unsubscribeKeyboardProfile();
      globalOwnership.dispose();
      paletteSurfaceTargetRef.current?.dispose();
      paletteSurfaceTargetRef.current = undefined;
      localNavigationFocusRef.current?.dispose();
      localNavigationFocusRef.current = null;
      clearLocalMapRefs();
      localKeyboardMapReadyRef.current = null;
      unsubscribeKeyboardMap();
      unsubscribeGlobalVoiceReservation();
      unsubscribeGlobalJobAdmission();
      globalVoiceTicketsRef.current.clear();
      unsubscribeDeckReservation();
      unsubscribeDeckLocalUI();
      unsubscribeDeckContextualUI();
      window.removeEventListener('focus', refreshKeyboardOnFocus, true);
      window.removeEventListener('blur', cancelKeyboardOnBlur, true);
      unregisterSurface();
      if (!commandScope) trustedSession.dispose();
      if (localPaletteTrustedSessionRef.current === trustedSession) localPaletteTrustedSessionRef.current = null;
      commandBackendExecutionRef.current = null;
      localCommandKeyboardRef.current = null;
      localCommandExecutionRef.current = null;
      localCommandContextualExecutionRef.current = null;
      commandContextualExecutionRef.current = null;
      unregisterWorkspaceChatDispatcher();
      workspaceChatOpenLeaseRef.current?.dispose();
      workspaceChatOpenLeaseRef.current = null;
      commandUIEffectGuardRef.current = null;
      pendingCommandCatalogRef.current = null;
      commandPaletteDismissActionRef.current = 'ignore';
      setCommandPaletteOpen(false);
      commandPaletteMenuItemsRef.current = [];
      setCommandPaletteItems([]);
      commandPickerAfterMainMenuRef.current = false;
      pendingCommandExecutionRef.current = null;
      activeCommandIntentRef.current = null;
      workspacePickerIntentRef.current = null;
      pendingWorkspaceCreateTargetRef.current?.dispose();
      pendingWorkspaceCreateTargetRef.current = null;
    };
  }, [commandOwner, workspace?.id, commandRouteIdentity, commandScope, prepareWorkspaceTabTarget, prepareWorkspaceMutationTarget, workspaceTabMutationAvailable, workspaceMutationAvailable, workspaceTabNavigationAvailable, workspacePanelFocusAvailable, executeContextualCommand, executeLocalUICommand, chatPickerAvailable, editorPresentationAvailable, tabCreationMenu, intentIsCurrent, clearPaletteEditorModeTargets, executeFileCommand, editorFileAvailable, executeFormatCommand, editorFormatAvailable, chatMessagingAvailable, runChatMessaging, clearPaletteChatMessaging, mermaidAvailable, runMermaidCommand, landmarkAvailable, presentationCommandAvailable]);

  const executePendingCommand = useCallback(() => {
    const intent = pendingCommandExecutionRef.current;
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    const intentStillCurrent = intent && auth.isAuthenticated &&
      auth.user?.userId === intent.userId && auth.user?.sessionId === intent.sessionId &&
      currentWorkspace?.id === intent.workspaceId &&
      (currentWorkspace?.activeTabId ?? null) === intent.activeTabId &&
      commandRouteIdentityRef.current === intent.routeIdentity;
    if (!intentStillCurrent || !commandPickerMountedRef.current || isModalOpen()) {
      pendingPageMutationRef.current?.dispose(); pendingPageMutationRef.current = null;
      pendingMermaidRef.current?.dispose(); pendingMermaidRef.current = null;
      pendingEditorFormatTargetRef.current?.dispose();
      pendingEditorFormatTargetRef.current = null;
      pendingEditorFileTargetRef.current?.dispose();
      pendingEditorFileTargetRef.current = null;
      pendingEditorModeTargetRef.current?.dispose();
      pendingEditorModeTargetRef.current = null;
      clearPaletteEditorModeTargets();
      paletteSurfaceTargetRef.current?.dispose();
      paletteSurfaceTargetRef.current = undefined;
      if (intent?.commandID === WORKSPACE_CREATE_COMMAND_ID) {
        pendingWorkspaceCreateTargetRef.current?.dispose();
        pendingWorkspaceCreateTargetRef.current = null;
      }
      pendingCommandExecutionRef.current = null;
      return;
    }
    // Toda ação localUI passa pela prova da paleta antes de qualquer branch
    // específico (inclusive Mermaid); um alvo capturado não substitui essa
    // revalidação final.
    if (isLocalUICommand(intent.commandID) && !hasCurrentLocalPaletteCommand(intent.commandID)) {
      pendingMermaidRef.current?.dispose(); pendingMermaidRef.current = null;
      pendingChatMessagingRef.current?.dispose(); pendingChatMessagingRef.current = null;
      pendingCommandExecutionRef.current = null;
      return;
    }
    pendingCommandExecutionRef.current = null;
    activeCommandIntentRef.current = intent;
    const executionStillCurrent = () => {
      const currentAuth = useAuthStore.getState();
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      return commandPickerMountedRef.current && activeCommandIntentRef.current === intent && currentAuth.isAuthenticated &&
        currentAuth.user?.userId === intent.userId &&
        currentAuth.user?.sessionId === intent.sessionId &&
        currentWorkspace?.id === intent.workspaceId &&
        (currentWorkspace?.activeTabId ?? null) === intent.activeTabId &&
        commandRouteIdentityRef.current === intent.routeIdentity;
    };
    if (intent.commandID === WORKSPACE_LIST_COMMAND_ID || isCommandLayerAction(intent.commandID)) {
      const lease = intent.contextualPalette;
      const contextualLayer = isCommandLayerAction(intent.commandID) && lease;
      const backendExecutor = contextualLayer ? createCommandBackendExecution(
        createContextualPaletteLayerWailsPort(lease, () => executionStillCurrent() &&
          !isModalOpen() && document.hasFocus() && ReadFocusContext().composition === 'inactive'),
        { trustedSession: localPaletteTrustedSessionRef.current ?? undefined, surfaceID: COMMAND_TOOLBAR_SURFACE_ID },
      ) : commandBackendExecutionRef.current;
      if (!backendExecutor) {
        activeCommandIntentRef.current = null;
        return;
      }
      void backendExecutor.execute(intent.commandID, (output) =>
        presentWorkspaceList(intent, output)).then((result) => {
        const stillCurrent = executionStillCurrent();
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
        if (contextualLayer && result.execution.status === 'succeeded') {
          if (stillCurrent) announce(t('commandSettings.layerActionCompleted'));
          return;
        }
        if (isCommandLayerAction(intent.commandID) && result.presented && stillCurrent) announce(t('commandSettings.layerActionCompleted'));
        if (result.presented || !stillCurrent) return;
        if (result.reason === 'context-stale' || result.reason === 'disposed') return;
        if (result.execution.status === 'suppressed') {
          announce(t('commandPalette.executionSuppressed'));
          return;
        }
        if (result.execution.status === 'denied' || result.execution.status === 'rejected_stale') {
          announce(t('commandPalette.unavailable'));
          return;
        }
        announce(t('commandPalette.executionUnknown'));
      }, () => {
        const stillCurrent = executionStillCurrent();
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
        if (stillCurrent) announce(t('commandPalette.executionUnknown'));
      }).finally(() => { if (contextualLayer) backendExecutor.dispose(); });
      return;
    }
    if (isEditorMermaidCommand(intent.commandID)) {
      const target = pendingMermaidRef.current;
      pendingMermaidRef.current = null;
      if (!target) { activeCommandIntentRef.current = null; return; }
      void runMermaidCommand(intent.commandID, undefined, target).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      }).catch(() => undefined);
      return;
    }
    if (isLocalUICommand(intent.commandID) && !isChatMessagingCommand(intent.commandID)) {
      const currentAuth = useAuthStore.getState();
      const currentWorkspace = useWorkspaceStore.getState().workspace;
      const mapOwner = localKeyboardOwnerRef.current;
      if (!hasCurrentLocalPaletteCommand(intent.commandID) ||
          !currentAuth.isAuthenticated || !currentAuth.user || !currentWorkspace || !mapOwner ||
          mapOwner.ownerId !== currentAuth.user.userId || mapOwner.sessionId !== currentAuth.user.sessionId ||
          mapOwner.workspaceId !== currentWorkspace.id) {
        activeCommandIntentRef.current = null;
        return;
      }
      // A paleta já capturou a instância e o alvo antes de abrir sua busca.
      // Ausência/invalidação dessa lease nunca permite procurar outra origem.
      if (isCapturedPresentationCommand(intent.commandID)) {
        const target = palettePresentationCommandsRef.current.get(intent.commandID);
        if (target) executeLocalUICommand(intent.commandID, false, target);
      } else if (isLandmarkNavigationCommand(intent.commandID)) {
        if (paletteLandmarkTargetRef.current) executeLocalUICommand(intent.commandID, false, paletteLandmarkTargetRef.current);
      } else if ((!isChatPickerCommand(intent.commandID) && !isEditorPresentationCommand(intent.commandID)) || paletteSurfaceTargetRef.current) {
        executeLocalUICommand(intent.commandID, false, paletteSurfaceTargetRef.current);
      }
      if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      return;
    }
    if (intent.commandID === CHAT_CLEAR_COMMAND) {
      const target = pendingChatClearRef.current;
      if (!target) return;
      void runChatClear(undefined, target, intent.contextualPalette).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      });
      return;
    }
    if (isPageMutationCommand(intent.commandID)) {
      const target = pendingPageMutationRef.current; pendingPageMutationRef.current = null;
      if (!target || target.commandId !== intent.commandID) { target?.dispose(); activeCommandIntentRef.current = null; return; }
      void runPageMutation(target, undefined, intent.contextualPalette).finally(() => { if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null; });
      return;
    }
    if (intent.commandID === TERMINAL_INTERRUPT_COMMAND) {
      const target = pendingTerminalOperationRef.current;
      if (!target) return;
      void runTerminalInterrupt(undefined, target, intent.contextualPalette).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      }).catch(() => undefined);
      return;
    }
    if (isTerminalSessionOperationCommand(intent.commandID)) {
      const target = pendingTerminalOperationRef.current;
      pendingTerminalOperationRef.current = null;
      if (!target || target.commandId !== intent.commandID) {
        target?.dispose();
        activeCommandIntentRef.current = null;
        return;
      }
      void runTerminalOperation(intent.commandID, undefined, target, intent.contextualPalette).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      }).catch(() => undefined);
      return;
    }
    if (isChatMessagingCommand(intent.commandID)) {
      const target = pendingChatMessagingRef.current;
      pendingChatMessagingRef.current = null;
      if (!target || target.commandId !== intent.commandID) { target?.dispose(); activeCommandIntentRef.current = null; return; }
      if (intent.commandID === 'chat.message.edit.open') {
        const owner = localKeyboardOwnerRef.current;
        if (!hasCurrentLocalPaletteCommand(intent.commandID) || owner?.ownerId !== intent.userId ||
            owner?.sessionId !== intent.sessionId || owner?.workspaceId !== intent.workspaceId) {
          target.dispose(); activeCommandIntentRef.current = null; return;
        }
      }
      void runChatMessaging(target, undefined, intent.contextualPalette).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      }).catch(() => undefined);
      return;
    }
    if (isEditorFormatCommand(intent.commandID)) {
      void executeFormatCommand(intent.commandID, undefined, undefined, undefined, intent.contextualPalette).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      }).catch(() => undefined);
      return;
    }
    if (isEditorFileCommand(intent.commandID)) {
      void executeFileCommand(intent.commandID, undefined, intent.contextualPalette).finally(() => {
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      });
      return;
    }
    if (isWorkspaceMutationCommand(intent.commandID)) {
      const lease = intent.contextualPalette;
      const trustedSession = localPaletteTrustedSessionRef.current;
      if (lease && (!trustedSession || !lease.isCurrent())) { activeCommandIntentRef.current = null; return; }
      const contextualExecutor = lease ? createCommandContextualBackendExecution(
        createCommandWorkspaceTabWailsPort({ contextualPalette: lease }), {
          trustedSession: trustedSession!,
          surfaceID: lease.observed.surfaceId,
          canCommitEffect: () => lease.isCurrent() && !isModalOpen(),
          prepareTarget: (id) => {
            if (id !== lease.commandId || !lease.isCurrent()) return undefined;
            const target = prepareWorkspaceMutationTarget(id);
            return target && { ...target, isCurrent: () => lease.isCurrent() && target.isCurrent() };
          },
        }) : commandContextualExecutionRef.current;
      if (!contextualExecutor) {
        activeCommandIntentRef.current = null;
        return;
      }
      const executionGeneration = commandCancellationGenerationRef.current;
      if (lease) contextualPaletteExecutorsRef.current.add(contextualExecutor);
      void executeContextualCommand(contextualExecutor, intent.commandID).then((result) => {
        const stillCurrent = executionStillCurrent() &&
          commandCancellationGenerationRef.current === executionGeneration;
        if (intent.commandID === WORKSPACE_CREATE_COMMAND_ID || intent.commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID || isEditorModeCommand(intent.commandID)) return;
        if (result.status === 'succeeded') {
          return;
        }
        if (!stillCurrent) return;
        if (result.status !== 'denied' && result.status !== 'cancelled_stale' &&
            result.status !== 'rejected_stale' && result.errorCode !== 'ui-context-stale') {
          announce(t(result.status === 'failed' ? 'commandPalette.executionFailed' : 'commandPalette.executionUnknown'));
        }
      }).finally(() => {
        if (lease) { contextualExecutor.dispose(); contextualPaletteExecutorsRef.current.delete(contextualExecutor); }
        if (activeCommandIntentRef.current === intent) activeCommandIntentRef.current = null;
      });
      return;
    }
    // Unknown presentation commands have no legacy audited fallback.
    activeCommandIntentRef.current = null;
  }, [announce, intentIsCurrent, presentWorkspaceList, t, executeContextualCommand, executeLocalUICommand, clearPaletteEditorModeTargets, executeFileCommand, executeFormatCommand, runChatMessaging, runMermaidCommand, hasCurrentLocalPaletteCommand, prepareWorkspaceMutationTarget]);
  executePendingCommandRef.current = executePendingCommand;

  // --- Page title ---
  const resolvedPath = pathname.startsWith('/settings') ? '/settings' : pathname;
  const pageTitle = isWorkspaceRoute
    ? (workspace?.name || t('menu.appTitle'))
    : t(PAGE_TITLE_KEYS[resolvedPath] || 'menu.appTitle');

  // --- Workspace picker (left, workspace route) — only workspace list ---
  const {
    menu: pickerMenu,
    openForTrigger: openPicker,
    closeMenu: closePicker,
    onSelectItem: onPickerSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => requestAnimationFrame(() => restoreDefaultFocus()),
    onAfterDismiss: () => {
      workspacePickerIntentRef.current = null;
      workspacePickerTriggerRef.current?.focus();
      workspacePickerTriggerRef.current = null;
    },
  });
  const openWorkspacePicker = useCallback((trigger: HTMLElement, ariaLabel: string, items: MenuItem[]) => {
    workspacePickerTriggerRef.current = trigger;
    openPicker(trigger, ariaLabel, items);
  }, [openPicker]);
  openWorkspacePickerRef.current = openWorkspacePicker;

  // --- Context menu (right-click) on picker button ---
  const {
    menu: ctxMenu,
    openAtPoint: openCtx,
    closeMenu: closeCtx,
    onSelectItem: onCtxSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => {
      const intent = pendingCommandExecutionRef.current;
      const target = pendingWorkspaceCreateTargetRef.current;
      if (intent?.commandID === WORKSPACE_CREATE_COMMAND_ID) {
        if (!target || !commandPickerMountedRef.current || isModalOpen() || !target.isCurrent()) {
          target?.dispose();
          pendingWorkspaceCreateTargetRef.current = null;
          pendingCommandExecutionRef.current = null;
          return;
        }
        pickerButtonRef.current?.focus();
        executePendingCommandRef.current?.();
        return;
      }
      requestAnimationFrame(() => restoreDefaultFocus());
    },
    onAfterDismiss: () => pickerButtonRef.current?.focus(),
  });

  const handleCommandPaletteSelect = useCallback((value: string) => {
    const item = commandPaletteMenuItemsRef.current.find((candidate) => candidate.id === value);
    if (!item || item.disabled || item.separator) return;
    item.action?.();
  }, []);

  const handleCommandPaletteAfterSelect = useCallback(() => {
    palettePageMutationsRef.current.forEach(target => target.dispose()); palettePageMutationsRef.current.clear();
    clearPaletteChatMessaging();
    paletteChatClearRef.current?.dispose(); paletteChatClearRef.current = null;
    paletteTerminalOperationRef.current?.dispose(); paletteTerminalOperationRef.current = null;
    paletteTerminalSessionOperationsRef.current.forEach(target => target.dispose());
    paletteTerminalSessionOperationsRef.current.clear();
    const intent = pendingCommandExecutionRef.current;
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    const intentStillCurrent = intent && auth.isAuthenticated &&
      auth.user?.userId === intent.userId && auth.user?.sessionId === intent.sessionId &&
      currentWorkspace?.id === intent.workspaceId &&
      (currentWorkspace?.activeTabId ?? null) === intent.activeTabId &&
      commandRouteIdentityRef.current === intent.routeIdentity &&
      commandPickerMountedRef.current && !isModalOpen();
    if (!intentStillCurrent) {
      pendingPageMutationRef.current?.dispose(); pendingPageMutationRef.current = null;
      clearPaletteEditorModeTargets();
      pendingChatMessagingRef.current?.dispose(); pendingChatMessagingRef.current = null;
      pendingChatClearRef.current?.dispose(); pendingChatClearRef.current = null;
      pendingTerminalOperationRef.current?.dispose(); pendingTerminalOperationRef.current = null;
      paletteSurfaceTargetRef.current?.dispose();
      paletteSurfaceTargetRef.current = undefined;
      if (intent?.commandID === WORKSPACE_CREATE_COMMAND_ID) {
        pendingWorkspaceCreateTargetRef.current?.dispose();
        pendingWorkspaceCreateTargetRef.current = null;
      }
      pendingCommandExecutionRef.current = null;
      activeCommandIntentRef.current = null;
      return;
    }
    commandPickerButtonRef.current?.focus();
    executePendingCommand();
    clearPaletteEditorModeTargets();
    paletteSurfaceTargetRef.current?.dispose();
    paletteSurfaceTargetRef.current = undefined;
  }, [executePendingCommand, clearPaletteEditorModeTargets, clearPaletteChatMessaging]);

  const handleCommandPaletteAfterDismiss = useCallback(() => {
    clearPaletteChatMessaging();
    paletteChatClearRef.current?.dispose(); paletteChatClearRef.current = null;
    paletteTerminalOperationRef.current?.dispose(); paletteTerminalOperationRef.current = null;
    paletteTerminalSessionOperationsRef.current.forEach(target => target.dispose());
    paletteTerminalSessionOperationsRef.current.clear();
    clearPaletteEditorModeTargets();
    paletteSurfaceTargetRef.current?.dispose();
    paletteSurfaceTargetRef.current = undefined;
    const action = commandPaletteDismissActionRef.current;
    commandPaletteDismissActionRef.current = 'restore';
    if (action === 'ignore' || !commandPickerMountedRef.current || isModalOpen()) return;
    if (action === 'open-main-menu') {
      menuButtonRef.current?.toggleMenu();
      return;
    }
    commandPickerButtonRef.current?.focus();
  }, [clearPaletteEditorModeTargets, clearPaletteChatMessaging]);

  closeCommandPaletteRef.current = (action = 'restore') => {
    if (!commandPaletteOpenRef.current) {
      commandPaletteDismissActionRef.current = 'restore';
      if (action === 'open-main-menu') menuButtonRef.current?.toggleMenu();
      return;
    }
    commandPaletteDismissActionRef.current = action;
    // Combobox calls onOpenChange(false) before React publishes the next
    // render. Keep the synchronous ref authoritative so selecting
    // navigation.palette.open can reopen exactly once from onAfterSelect.
    commandPaletteOpenRef.current = false;
    setCommandPaletteOpen(false);
  };

  const handleExportWorkspace = useCallback(async () => {
    try {
      const yaml = await useWorkspaceStore.getState().exportWorkspace();
      const blob = new Blob([yaml], { type: 'application/x-yaml' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `workspace-${workspace?.name?.replace(/\s+/g, '-').toLowerCase() || 'export'}.yaml`;
      a.click();
      URL.revokeObjectURL(url);
      announce(t('workspace.exported'));
    } catch (error) {
      logger.error('[Topbar] Export error:', error);
    }
  }, [workspace?.name, announce, t]);

  const handleImportWorkspace = useCallback(async () => {
    try {
      const input = document.createElement('input');
      input.type = 'file';
      input.accept = '.yaml,.yml';
      input.onchange = async () => {
        const file = input.files?.[0];
        if (!file) return;
        const text = await file.text();
        await useWorkspaceStore.getState().importWorkspace(text);
      };
      input.click();
    } catch (error) {
      logger.error('[Topbar] Import error:', error);
    }
  }, []);

  const startRename = useCallback(() => {
    if (!workspace) return;
    setIsRenaming(true);
    setRenameValue(workspace.name);
  }, [workspace]);

  const queueWorkspaceCreateCommand = useCallback(() => {
    const auth = useAuthStore.getState();
    const currentWorkspace = useWorkspaceStore.getState().workspace;
    if (!auth.isAuthenticated || !auth.user || !currentWorkspace || isModalOpen()) return;
    const target = captureWorkspaceCreateTarget(() => commandRouteIdentityRef.current);
    if (!target) return;
    pendingWorkspaceCreateTargetRef.current?.dispose();
    pendingWorkspaceCreateTargetRef.current = target;
    pendingCommandExecutionRef.current = {
      commandID: WORKSPACE_CREATE_COMMAND_ID,
      userId: auth.user.userId,
      sessionId: auth.user.sessionId,
      workspaceId: currentWorkspace.id,
      activeTabId: currentWorkspace.activeTabId ?? null,
      routeIdentity: commandRouteIdentityRef.current,
    };
  }, []);

  const pickerItems = useMemo((): MenuItem[] => {
    return workspaces.map((ws) => ({
      id: `ws-${ws.id}`,
      label: ws.name,
      icon: ws.is_active ? <CheckOutlined /> : undefined,
      shortcut: `${ws.tab_count} ${ws.tab_count === 1 ? t('workspace.tabSingular') : t('workspace.tabPlural')}`,
      checked: ws.is_active,
      action: () => { if (!ws.is_active) void switchWorkspace(ws.id); },
    }));
  }, [workspaces, switchWorkspace, t]);

  const ctxMenuItems = useMemo((): MenuItem[] => [
    {
      id: 'new-workspace',
      label: t('workspace.newWorkspace'),
      icon: <PlusOutlined />,
      shortcut: shortcutHint(WORKSPACE_CREATE_COMMAND_ID),
      action: queueWorkspaceCreateCommand,
    },
    {
      id: 'rename-workspace',
      label: t('workspace.rename'),
      icon: <EditOutlined />,
      shortcut: 'F2',
      action: startRename,
    },
    { id: 'sep-1', separator: true },
    {
      id: 'export-workspace',
      label: t('workspace.export'),
      icon: <ExportOutlined />,
      action: handleExportWorkspace,
    },
    {
      id: 'import-workspace',
      label: t('workspace.import'),
      icon: <ImportOutlined />,
      action: handleImportWorkspace,
    },
  ], [t, queueWorkspaceCreateCommand, startRename, handleExportWorkspace, handleImportWorkspace, shortcutHint]);

  const commandCatalogItemsToMenuItems = useCallback((items: apidto.CommandCatalogItem[]): MenuItem[] => {
    if (items.length === 0) {
      return [{
        id: 'command-palette-empty',
        label: t('commandPalette.empty'),
        disabled: true,
      }];
    }

    return items.map((item) => {
      const localUIAvailable = (!isLocalUICommand(item.id) || hasCurrentLocalPaletteCommand(item.id)) && (!isPageMutationCommand(item.id) || palettePageMutationsRef.current.get(item.id)?.isCurrent() === true);
      const contextualAvailable = item.id !== WORKSPACE_PANEL_FOCUS_COMMAND_ID || workspacePanelFocusAvailable();
      const tabNavigationAvailable = workspaceTabNavigationAvailable(item.id);
      const mutationAvailable = !isWorkspaceMutationCommand(item.id) || (workspaceMutationAvailable(item.id) && contextualPaletteAvailable(item.id));
      const editorModeAvailable = !isEditorModeCommand(item.id) || paletteEditorModeTargetsRef.current.get(item.id)?.isCurrent() === true;
      const editorFileAvailable = !isEditorFileCommand(item.id) || paletteEditorFileTargetRef.current?.canExecute(item.id) === true;
      const editorFormatAvailable = !isEditorFormatCommand(item.id) || paletteEditorFormatTargetRef.current?.canExecute(item.id) === true;
      const mermaidAvailable = !isEditorMermaidCommand(item.id) || paletteMermaidRef.current.get(item.id)?.canExecute(item.id) === true;
      const chatAvailable = (!isChatPickerCommand(item.id) && !isEditorPresentationCommand(item.id)) || Boolean(paletteSurfaceTargetRef.current?.isCurrent() &&
        paletteSurfaceTargetRef.current.canOpen(item.id));
      const clearAvailable = item.id !== CHAT_CLEAR_COMMAND || paletteChatClearRef.current?.isCurrent() === true;
      const terminalAvailable = item.id === TERMINAL_INTERRUPT_COMMAND
        ? paletteTerminalOperationRef.current?.isCurrent() === true
        : isTerminalSessionOperationCommand(item.id)
          ? paletteTerminalSessionOperationsRef.current.get(item.id)?.isCurrent() === true
          : true;
      const messagingAvailable = !isChatMessagingCommand(item.id) || paletteChatMessagingRef.current.get(item.id)?.isCurrent() === true;
      const landmarkAvailable = !isLandmarkNavigationCommand(item.id) || paletteLandmarkTargetRef.current?.canOpen(item.id) === true;
      const presentationCommandAvailable = !isCapturedPresentationCommand(item.id) || palettePresentationCommandsRef.current.get(item.id)?.canOpen(item.id) === true;
      const requiresContextualPalette = isContextualPaletteCommand(item.id) && contextualPaletteCommandsRef.current?.has(item.id);
      const available = item.available && contextualPaletteAvailable(item.id) && localUIAvailable && contextualAvailable && tabNavigationAvailable && mutationAvailable && chatAvailable && editorModeAvailable && editorFileAvailable && editorFormatAvailable && mermaidAvailable && clearAvailable && terminalAvailable && messagingAvailable && landmarkAvailable && presentationCommandAvailable;
      const unavailableReason = !contextualAvailable || !tabNavigationAvailable || !mutationAvailable
        ? t('commandPalette.unavailable')
        : item.readinessReason || item.availabilityReason || t('commandPalette.unavailable');
      return {
        id: `command-${item.id}`,
        label: item.name || item.id,
        searchText: [item.description, item.category, ...(item.aliases ?? [])].filter(Boolean).join(' '),
        shortcut: item.category || item.risk || undefined,
        disabled: !available,
        ariaLabel: [item.name || item.id, item.description, !available ? unavailableReason : '']
          .filter(Boolean).join('. '),
        action: () => {
          if (!available ||
              !contextualPaletteAvailable(item.id) ||
              (isLocalUICommand(item.id) && !hasCurrentLocalPaletteCommand(item.id)) ||
              (isCapturedPresentationCommand(item.id) && !palettePresentationCommandsRef.current.get(item.id)?.canOpen(item.id)) ||
              (isLandmarkNavigationCommand(item.id) && !paletteLandmarkTargetRef.current?.canOpen(item.id)) ||
              ((isChatPickerCommand(item.id) || isEditorPresentationCommand(item.id)) && !paletteSurfaceTargetRef.current?.isCurrent()) ||
              (item.id === WORKSPACE_PANEL_FOCUS_COMMAND_ID && !workspacePanelFocusAvailable()) ||
              !workspaceTabNavigationAvailable(item.id) ||
              (isWorkspaceMutationCommand(item.id) && !workspaceMutationAvailable(item.id))) {
            announce(`${item.name || item.id}: ${unavailableReason}`);
            return;
          }
          if (isPageMutationCommand(item.id) || item.id === WORKSPACE_LIST_COMMAND_ID || isCommandLayerAction(item.id) || item.id === CHAT_CLEAR_COMMAND || item.id === TERMINAL_INTERRUPT_COMMAND || isTerminalSessionOperationCommand(item.id) || isChatMessagingCommand(item.id) || commandUIHandlers.has(item.id) || isWorkspaceMutationCommand(item.id) || isLocalUICommand(item.id) || isEditorFileCommand(item.id) || isEditorFormatCommand(item.id) || isEditorMermaidCommand(item.id)) {
            const auth = useAuthStore.getState();
            const currentWorkspace = useWorkspaceStore.getState().workspace;
            if (!auth.isAuthenticated || !auth.user || !currentWorkspace) return;
            const needsContextualPalette = requiresContextualPalette || (isContextualPaletteCommand(item.id) && contextualPaletteCommandsRef.current?.has(item.id));
            if (needsContextualPalette && isCommandLayerAction(item.id) && ReadFocusContext().composition === 'active') return;
            const contextualPalette = needsContextualPalette ? captureContextualPaletteLease(item.id) : undefined;
            if (needsContextualPalette && !contextualPalette) return;
            if (isPageMutationCommand(item.id)) {
              const target = palettePageMutationsRef.current.get(item.id);
              if (!target?.isCurrent()) return;
              pendingPageMutationRef.current?.dispose(); pendingPageMutationRef.current = target;
              palettePageMutationsRef.current.delete(item.id);
            }
            if (item.id === TERMINAL_INTERRUPT_COMMAND) {
              const target = paletteTerminalOperationRef.current;
              if (!target?.isCurrent()) return;
              pendingTerminalOperationRef.current?.dispose(); pendingTerminalOperationRef.current = target;
              paletteTerminalOperationRef.current = null;
            }
            if (isTerminalSessionOperationCommand(item.id)) {
              const target = paletteTerminalSessionOperationsRef.current.get(item.id);
              if (!target?.isCurrent()) return;
              pendingTerminalOperationRef.current?.dispose();
              pendingTerminalOperationRef.current = target;
              paletteTerminalSessionOperationsRef.current.delete(item.id);
            }
            if (isEditorMermaidCommand(item.id)) {
              const target = paletteMermaidRef.current.get(item.id);
              if (!target?.isCurrent() || !target.canExecute(item.id)) return;
              pendingMermaidRef.current?.dispose(); pendingMermaidRef.current = target;
              paletteMermaidRef.current.delete(item.id);
            }
            if (isChatMessagingCommand(item.id)) {
              const target = paletteChatMessagingRef.current.get(item.id);
              if (!target?.isCurrent()) return;
              pendingChatMessagingRef.current?.dispose(); pendingChatMessagingRef.current = target;
              paletteChatMessagingRef.current.delete(item.id);
            }
            if (item.id === CHAT_CLEAR_COMMAND) {
              const target = paletteChatClearRef.current;
              if (!target?.isCurrent()) return;
              pendingChatClearRef.current?.dispose(); pendingChatClearRef.current = target;
              paletteChatClearRef.current = null;
            }
            if (isEditorFormatCommand(item.id)) {
              const target = paletteEditorFormatTargetRef.current;
              if (!target?.canExecute(item.id)) return;
              pendingEditorFormatTargetRef.current?.dispose();
              pendingEditorFormatTargetRef.current = target;
              paletteEditorFormatTargetRef.current = null;
            }
            if (isEditorFileCommand(item.id)) {
              const target = paletteEditorFileTargetRef.current;
              if (!target?.canExecute(item.id)) return;
              pendingEditorFileTargetRef.current?.dispose();
              pendingEditorFileTargetRef.current = target;
              paletteEditorFileTargetRef.current = null;
            }
            if (isEditorModeCommand(item.id)) {
              const target = paletteEditorModeTargetsRef.current.get(item.id);
              if (!target?.isCurrent()) return;
              pendingEditorModeTargetRef.current?.dispose();
              pendingEditorModeTargetRef.current = target;
              paletteEditorModeTargetsRef.current.delete(item.id);
            }
            pendingCommandExecutionRef.current = {
              commandID: item.id,
              ...(contextualPalette ? { contextualPalette } : {}),
              userId: auth.user.userId,
              sessionId: auth.user.sessionId,
              workspaceId: currentWorkspace.id,
              activeTabId: currentWorkspace.activeTabId ?? null,
              routeIdentity: commandRouteIdentityRef.current,
            };
            return;
          }
          const message = `${item.name || item.id}: ${item.description || t('commandPalette.available')}`;
          announce(message);
        },
      };
    });
  }, [announce, commandUIHandlers, t, workspaceMutationAvailable, workspaceTabNavigationAvailable, workspacePanelFocusAvailable, hasCurrentLocalPaletteCommand, contextualPaletteAvailable, captureContextualPaletteLease]);

  const handleOpenCommandPicker = useCallback((fromKeyboard = false) => {
    const pointerLandmark = pointerLandmarkTargetRef.current;
    pointerLandmarkTargetRef.current = undefined;
    const pointerPresentationCommands = pointerPresentationCommandsRef.current;
    pointerPresentationCommandsRef.current = undefined;
    const disposePointerTargets = () => {
      pointerLandmark?.target?.dispose();
      pointerPresentationCommands?.forEach(target => target.dispose());
    };
    if (fromKeyboard) disposePointerTargets();
    if (isModalOpen()) {
      disposePointerTargets();
      commandPickerAfterMainMenuRef.current = false;
      return;
    }
    if (commandPaletteOpen) {
      disposePointerTargets();
      closeCommandPaletteRef.current?.();
      return;
    }
    const trigger = commandPickerButtonRef.current;
    if (!trigger) { disposePointerTargets(); return; }
    const guard = commandUIEffectGuardRef.current;
    const focusedElement = document.activeElement;
    const registeredSource = commandScope?.surfaceForElement(focusedElement);
    // A workspace panel still loading/mounting its provider is not a toolbar
    // origin. This DOM check only rejects; it never invents a surface snapshot.
    if (commandScope && !registeredSource && focusedElement?.closest('.ws-content__panel')) { disposePointerTargets(); return; }
    const sourceSurface = registeredSource ?? COMMAND_TOOLBAR_SURFACE_ID;
    const ownedSource = localPaletteTrustedSessionRef.current?.readOwnedCommandContextFrame(sourceSurface);
    const sourceFrame = ownedSource?.frame;
    const sourceVisual = sourceFrame?.surface;
    const sourceWorkspace = useWorkspaceStore.getState().workspace;
    if (ownedSource && sourceVisual && sourceVisual.surfaceId === sourceSurface && sourceFrame.focus.hasFocus && sourceWorkspace) {
      localPaletteSourceRef.current = {
        ownerId: ownedSource.owner.userId,
        sessionId: ownedSource.owner.sessionId,
        workspaceId: ownedSource.owner.workspaceId,
        routeIdentity: commandRouteIdentityRef.current,
        surfaceLease: ownedSource.surfaceLease,
        surfaceType: sourceVisual.surfaceType,
        surfaceId: sourceVisual.surfaceId,
        snapshotVersion: sourceVisual.snapshotVersion,
        profileRevision: localPaletteProfileRevisionRef.current,
        generation: localKeyboardGenerationRef.current ?? '',
        explicitVisualOrigin: registeredSource !== undefined || !!commandSurfaceRef.current?.contains(focusedElement),
        explicitWorkspaceOrigin: registeredSource !== undefined && isWorkspaceRoute &&
          sourceVisual.surfaceId === sourceWorkspace.activeTabId &&
          ['chat', 'editor', 'terminal', 'tasklist'].includes(sourceVisual.surfaceType),
        ...(sourceFrame.profile?.slug ? { profile: sourceFrame.profile.slug } : {}),
        ...(sourceSurface !== COMMAND_TOOLBAR_SURFACE_ID && sourceWorkspace.activeTabId
          ? { activeTabId: sourceWorkspace.activeTabId }
          : {}),
      };
    } else {
      // O picker transfere foco, mas nunca vira a origem visual capturada.
      // Sem prova do provider, somente comandos locais incondicionais passam.
      localPaletteSourceRef.current = null;
    }
    const token = fromKeyboard
      ? guard?.capturePalette(sourceSurface)
      : guard?.capture(sourceSurface);
    if (!guard || !token) { disposePointerTargets(); return; }
    paletteSurfaceTargetRef.current?.dispose();
    paletteSurfaceTargetRef.current = captureChatPickerTarget(() => pathnameRef.current) ??
      captureEditorPresentationTarget(() => pathnameRef.current);
    clearPaletteEditorModeTargets();
    paletteEditorFileTargetRef.current = captureEditorFileTarget() ?? null;
    paletteLandmarkTargetRef.current = !fromKeyboard && pointerLandmark
      ? pointerLandmark.target
      : captureLandmarkNavigationTarget(() => pathnameRef.current);
    palettePresentationCommandsRef.current = !fromKeyboard && pointerPresentationCommands
      ? pointerPresentationCommands
      : capturePresentationCommands();
    paletteEditorFormatTargetRef.current = captureEditorFormatting() ?? null;
    for (const commandID of EDITOR_MERMAID_COMMAND_IDS) {
      const target = captureEditorMermaidTarget(commandID);
      if (target) paletteMermaidRef.current.set(commandID, target);
    }
    paletteChatClearRef.current?.dispose();
    paletteChatClearRef.current = captureChatClearTarget(() => pathnameRef.current) ?? null;
    paletteTerminalOperationRef.current?.dispose();
    paletteTerminalOperationRef.current = captureTerminalOperationTarget(() => pathnameRef.current) ?? null;
    paletteTerminalSessionOperationsRef.current.forEach(target => target.dispose());
    paletteTerminalSessionOperationsRef.current.clear();
    for (const commandId of [TERMINAL_SESSION_CREATE_COMMAND, TERMINAL_SESSION_CLOSE_COMMAND] as const) {
      const target = captureTerminalOperationTarget(() => pathnameRef.current, undefined, commandId);
      if (target) paletteTerminalSessionOperationsRef.current.set(commandId, target);
    }
    palettePageMutationsRef.current.forEach(target => target.dispose()); palettePageMutationsRef.current.clear();
    for (const id of PAGE_MUTATION_IDS) {
      const target = capturePageMutationTarget(() => pathnameRef.current, id);
      if (target) palettePageMutationsRef.current.set(id, target);
    }
    clearPaletteChatMessaging();
    for (const commandID of CHAT_MESSAGING_COMMAND_IDS) {
      const target = captureChatMessagingTarget(() => pathnameRef.current, commandID);
      if (target) paletteChatMessagingRef.current.set(commandID, target);
    }
    for (const commandID of EDITOR_MODE_COMMAND_IDS) {
      const target = captureEditorModeTarget(() => pathnameRef.current, commandID);
      if (target) paletteEditorModeTargetsRef.current.set(commandID, target);
    }
    pendingCommandCatalogRef.current = token;
    setCommandCatalogLoading(true);
    const mapReady = localKeyboardMapReadyRef.current ?? Promise.resolve();
    void mapReady.then(() => listCommandCatalog({ locale: i18n.language, source: 'palette' }))
      .then((items) => {
        if (pendingCommandCatalogRef.current !== token) return;
        guard.commit(token, () => {
          pendingCommandCatalogRef.current = null;
          setCommandCatalogLoading(false);
          const currentTrigger = commandPickerButtonRef.current;
          if (currentTrigger) {
            const menuItems = commandCatalogItemsToMenuItems(items);
            commandPaletteMenuItemsRef.current = menuItems;
            commandPaletteDismissActionRef.current = 'restore';
            setCommandPaletteItems(menuItems
              .filter((item): item is MenuItem & { id: string; label: string } => Boolean(item.id && item.label && !item.separator))
              .map((item) => ({
                value: item.id,
                label: item.label,
                searchText: item.searchText,
                accessibleLabel: item.ariaLabel,
                disabled: item.disabled,
              })));
            setCommandPaletteOpen(true);
            announce(items.length === 0 ? t('commandPalette.empty') : t('commandPalette.results', {
              total: items.length,
              available: menuItems.filter((item) => !item.disabled && !item.separator).length,
            }));
          }
        });
      })
      .catch((error) => {
        logger.error('[Topbar] Erro ao carregar Command Palette:', error);
        if (pendingCommandCatalogRef.current !== token) return;
        clearPaletteEditorModeTargets();
        clearPaletteChatMessaging();
        paletteChatClearRef.current?.dispose(); paletteChatClearRef.current = null;
        paletteSurfaceTargetRef.current?.dispose(); paletteSurfaceTargetRef.current = undefined;
        guard.commit(token, () => {
          pendingCommandCatalogRef.current = null;
          const message = t('commandPalette.error');
          addToast(message, 'error');
          announce(message);
          setCommandCatalogLoading(false);
        });
      })
      .finally(() => {
        // Bookkeeping da requisição não é uma prova de contexto nem publica UI.
        // Só o pedido ainda pendente pode limpar o estado de loading.
        if (!commandPickerMountedRef.current || pendingCommandCatalogRef.current !== token) return;
        clearPaletteEditorModeTargets();
        clearPaletteChatMessaging();
        paletteChatClearRef.current?.dispose(); paletteChatClearRef.current = null;
        paletteSurfaceTargetRef.current?.dispose(); paletteSurfaceTargetRef.current = undefined;
        pendingCommandCatalogRef.current = null;
        setCommandCatalogLoading(false);
      });
  }, [addToast, announce, commandCatalogItemsToMenuItems, commandPaletteOpen, i18n.language, t, commandScope, clearPaletteEditorModeTargets, clearPaletteChatMessaging, capturePresentationCommands]);
  handleOpenCommandPickerRef.current = handleOpenCommandPicker;

  const openCommandPickerAfterMainMenuSelect = useCallback(() => {
    if (!commandPickerAfterMainMenuRef.current || !commandPickerMountedRef.current || isModalOpen()) {
      commandPickerAfterMainMenuRef.current = false;
      return;
    }
    commandPickerAfterMainMenuRef.current = false;
    commandPickerButtonRef.current?.focus();
    handleOpenCommandPicker();
  }, [handleOpenCommandPicker]);

  const handleOpenPicker = useCallback(() => {
    if (pickerMenu.visible) { closePicker(); return; }
    if (pickerButtonRef.current) {
      openWorkspacePicker(pickerButtonRef.current, t('workspace.workspaceList'), pickerItems);
    }
  }, [pickerMenu.visible, closePicker, openWorkspacePicker, pickerItems, t]);

  const handlePickerContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openCtx(e.clientX, e.clientY, t('workspace.workspaceOptions'), ctxMenuItems);
  }, [openCtx, t, ctxMenuItems]);

  const handlePickerKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'F2') {
      e.preventDefault();
      startRename();
    }
  }, [startRename]);

  // --- Rename ---
  useEffect(() => {
    if (isRenaming) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [isRenaming]);

  const handleConfirmRename = useCallback(async () => {
    const trimmed = renameValue.trim();
    if (trimmed && trimmed !== workspace?.name) {
      await renameWorkspace(trimmed);
      announce(`${t('workspace.renamed')}: ${trimmed}`);
    }
    setIsRenaming(false);
  }, [renameValue, workspace?.name, renameWorkspace, announce, t]);

  const handleRenameKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleConfirmRename();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setIsRenaming(false);
      pickerButtonRef.current?.focus();
    }
  }, [handleConfirmRename]);

  const currentPage = ROUTE_IDS[resolvedPath] || 'workspace';

  const mainMenuItems: MenuButtonItem[] = useMemo(() => [
    ...(!isWorkspaceRoute ? [
      { id: 'back-to-workspace', label: t('menu.backToWorkspace'), icon: <ArrowLeftOutlined />, shortcut: shortcutHint('navigation.workspace.open'), onClick: () => navigate('/') },
      { id: 'sep-back', separator: true as const },
    ] : []),
    { id: 'history', label: t('menu.history'), icon: <HistoryOutlined />, shortcut: shortcutHint('navigation.history.open'), onClick: () => navigate('/history') },
    { id: 'memories', label: t('menu.memories'), icon: <ReadOutlined />, shortcut: shortcutHint('navigation.memories.open'), onClick: () => navigate('/memories') },
    { id: 'tasklists', label: t('menu.tasklists'), icon: <CheckSquareOutlined />, shortcut: shortcutHint('navigation.tasklists.open'), onClick: () => navigate('/tasklists') },
    { id: 'jobs', label: t('menu.jobs'), icon: <ThunderboltOutlined />, shortcut: shortcutHint('navigation.jobs.open'), onClick: () => navigate('/jobs') },
    { id: 'profiles', label: t('menu.profiles'), icon: <UserSwitchOutlined />, shortcut: shortcutHint('navigation.profiles.open'), onClick: () => navigate('/profiles') },
    { id: 'settings', label: t('menu.settings'), icon: <SettingOutlined />, shortcut: shortcutHint('navigation.settings.open'), onClick: () => navigate('/settings') },
    { id: 'help', label: t('menu.help'), icon: <QuestionCircleOutlined />, shortcut: shortcutHint('navigation.help.open'), onClick: () => navigate('/help') },
    { id: 'command-palette', label: t('commandPalette.title'), icon: <ThunderboltOutlined />, shortcut: shortcutHint('navigation.palette.open'), onClick: () => { commandPickerAfterMainMenuRef.current = true; } },
    { id: 'keyboard-shortcuts', label: t('menu.keyboardShortcuts'), icon: <KeyOutlined />, shortcut: 'Ctrl+?', onClick: () => openShortcutsHelp() },
    { id: 'about', label: t('menu.about'), icon: <InfoCircleOutlined />, onClick: () => navigate('/about') },
  ], [navigate, t, isWorkspaceRoute, openShortcutsHelp, shortcutHint]);

  // --- Keyboard shortcuts ---
  useEffect(() => {
    const handleRollback = (event: Event) => {
      const detail = (event as CustomEvent<{
        failedTabId?: unknown;
        rollbackTabId?: unknown;
      }>).detail;
      if (typeof detail?.failedTabId !== 'string' || typeof detail.rollbackTabId !== 'string') return;
      rollbackFocusRef.current?.dispose();
      rollbackFocusRef.current = captureWorkspaceTabActivationRollbackFocus(
        () => pathnameRef.current,
        { failedTabId: detail.failedTabId, rollbackTabId: detail.rollbackTabId },
      );
      rollbackFocusRef.current?.apply();
    };

    window.addEventListener('workspace:tab-activation-rollback', handleRollback);
    return () => {
      window.removeEventListener('workspace:tab-activation-rollback', handleRollback);
      rollbackFocusRef.current?.dispose();
      rollbackFocusRef.current = null;
    };
  }, [commandOwner, workspace?.id, commandRouteIdentity]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (
        event.key === 'Escape' &&
        (pendingCommandCatalogRef.current !== null || commandPickerAfterMainMenuRef.current ||
          pendingCommandExecutionRef.current || activeCommandIntentRef.current || workspacePickerIntentRef.current ||
          workspaceChatPreparationRef.current || workspaceChatOpenLeaseRef.current)
      ) {
        event.preventDefault();
        commandCancellationGenerationRef.current += 1;
        pendingCommandCatalogRef.current = null;
        commandPickerAfterMainMenuRef.current = false;
        pendingCommandExecutionRef.current = null;
        activeCommandIntentRef.current = null;
        workspacePickerIntentRef.current = null;
        workspaceChatPreparationRef.current = null;
        workspaceChatOpenLeaseRef.current?.dispose();
        workspaceChatOpenLeaseRef.current = null;
        commandBackendExecutionRef.current?.cancelPresentation();
        localCommandExecutionRef.current?.cancelPresentation();
        void commandContextualExecutionRef.current?.cancel();
        void localCommandContextualExecutionRef.current?.cancel();
        setCommandCatalogLoading(false);
        return;
      }

      const isPlainAlt = event.altKey && !event.ctrlKey && !event.shiftKey && !event.metaKey;

      // Alt+Backspace → workspace (alternativa acessível a Alt+W). Tratado antes
      // do guard de modal porque Alt+Backspace tem "voltar" padrão no
      // navegador/WebView: sempre prevenimos (fora de campo editável) para não
      // vazar, inclusive com modal aberto — onde apenas prevenimos, sem navegar.
      // Em campos editáveis Alt+Backspace costuma apagar palavra, então saímos.
      if (isPlainAlt && event.key === 'Backspace') {
        if (isEditableKeyboardTarget(event.target)) return;
        event.preventDefault();
        // Alt+Backspace continua reservado fora de campos editáveis para não
        // vazar como "voltar" do WebView. A navegação para workspace, quando
        // existir, pertence exclusivamente ao binding efetivo do mapa local.
        return;
      }

      // Os demais atalhos de navegação (Alt+…) não devem agir na UI de fundo
      // enquanto qualquer modal está aberto (incl. o painel de atalhos, que se
      // registra no stack via Modal).
      if (isModalOpen()) return;
      if (!isPlainAlt) return;

    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  return (
    <>
      <header className="topbar" ref={commandSurfaceRef} onPointerDownCapture={(event) => {
        pointerLandmarkTargetRef.current?.target?.dispose();
        pointerLandmarkTargetRef.current = undefined;
        pointerPresentationCommandsRef.current?.forEach(target => target.dispose());
        pointerPresentationCommandsRef.current = undefined;
        if (event.button === 0 && !commandPaletteOpen && event.target instanceof Node &&
            commandPickerButtonRef.current?.contains(event.target)) {
          // Pointer focus moves to the trigger before its click handler runs.
          // Keep the original region, including an unavailable capture.
          pointerLandmarkTargetRef.current = { target: captureLandmarkNavigationTarget(() => pathnameRef.current) };
          pointerPresentationCommandsRef.current = capturePresentationCommands();
        }
      }} onPointerCancelCapture={() => {
        pointerLandmarkTargetRef.current?.target?.dispose();
        pointerLandmarkTargetRef.current = undefined;
        pointerPresentationCommandsRef.current?.forEach(target => target.dispose());
        pointerPresentationCommandsRef.current = undefined;
      }}>
        <div
          className="topbar__toolbar"
          role="toolbar"
          aria-label={t('landmarks.topbar')}
          ref={toolbarRef as React.RefObject<HTMLDivElement>}
        >
        <div className="topbar__left">
          <MenuButton
            ref={menuButtonRef}
            items={mainMenuItems}
            currentItemId={currentPage}
            buttonLabel={[t('menu.navLabelPlain'), shortcutHint('navigation.menu.open')].filter(Boolean).join(' — ')}
            tabIndex={-1}
            onAfterSelect={openCommandPickerAfterMainMenuSelect}
          />
          <ConnectionStatusIndicator />
          <Combobox
            icon={<ThunderboltOutlined />}
            label={t('commandPalette.shortTitle')}
            triggerAriaLabel={t('commandPalette.title')}
            items={commandPaletteItems}
            selected=""
            placeholder={t('commandPalette.searchPlaceholder')}
            onSelect={handleCommandPaletteSelect}
            open={commandPaletteOpen}
            onOpenChange={(open) => {
              if (!open) closeCommandPaletteRef.current?.();
              else handleOpenCommandPicker();
            }}
            triggerRef={commandPickerButtonRef}
            onAfterSelect={handleCommandPaletteAfterSelect}
            onAfterDismiss={handleCommandPaletteAfterDismiss}
            shortcut={shortcutHint('navigation.palette.open')}
            busy={commandCatalogLoading}
          />
        </div>

        <h1 className="topbar__title">{pageTitle}</h1>

        <div className="topbar__right">
          {isWorkspaceRoute ? (
            isRenaming ? (
              <input
                ref={renameInputRef}
                className="topbar__rename-input"
                value={renameValue}
                onChange={(e) => setRenameValue(e.target.value)}
                onKeyDown={handleRenameKeyDown}
                onBlur={() => void handleConfirmRename()}
                aria-label={t('workspace.renamePlaceholder')}
              />
            ) : (
              <button
                ref={pickerButtonRef}
                className="topbar__picker"
                onClick={handleOpenPicker}
                onContextMenu={handlePickerContextMenu}
                onKeyDown={handlePickerKeyDown}
                onDoubleClick={startRename}
                aria-expanded={pickerMenu.visible}
                aria-label={t('workspace.workspaceList')}
                tabIndex={-1}
              >
                <span className="topbar__picker-icon" aria-hidden="true"><FolderOutlined /></span>
                <span className="topbar__picker-name">{workspace?.name || 'Workspace'}</span>
                <span className="topbar__picker-arrow" aria-hidden="true"><DownOutlined /></span>
              </button>
            )
          ) : (
            <button
              className="topbar__back"
              onClick={() => navigate('/')}
              aria-label={[t('menu.backToWorkspace'), shortcutHint('navigation.workspace.open')].filter(Boolean).join(' — ')}
              title={[t('menu.backToWorkspace'), shortcutHint('navigation.workspace.open')].filter(Boolean).join(' — ')}
              tabIndex={-1}
            >
              <ArrowLeftOutlined aria-hidden="true" />
              <span>{t('menu.backToWorkspace')}</span>
            </button>
          )}
        </div>
        </div>
      </header>

      <Menu
        items={pickerMenu.items}
        x={pickerMenu.x}
        y={pickerMenu.y}
        visible={pickerMenu.visible}
        ariaLabel={pickerMenu.ariaLabel || t('workspace.workspaceList')}
        searchable
        searchPlaceholder={t('workspace.searchWorkspaces')}
        onClose={closePicker}
        onSelect={onPickerSelect}
      />

      <Menu
        items={ctxMenu.items}
        x={ctxMenu.x}
        y={ctxMenu.y}
        visible={ctxMenu.visible}
        ariaLabel={ctxMenu.ariaLabel || t('workspace.workspaceOptions')}
        onClose={closeCtx}
        onSelect={onCtxSelect}
      />

      <KeyboardShortcutsHelp isOpen={shortcutsHelpOpen} onClose={closeShortcutsHelp} surfaceType={shortcutSurface} />
    </>
  );
}
